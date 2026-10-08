// Package command implements the durable command queue (ADR-014). The Control
// Plane queues commands per Node; the Agent polls, executes, and acknowledges.
package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound indicates a nonexistent command for the given Node.
var ErrNotFound = errors.New("command not found")

// Command is a queued command, in the form delivered to the Agent.
type Command struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Result is the outcome of a command reported by the Agent.
type Result struct {
	Status string          `json:"status"` // DONE ou FAILED
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Service groups the operations on the command queue.
type Service struct {
	pool *pgxpool.Pool
	// redispatchAfter/maxAttempts (#111): window to redeliver a DISPATCHED whose
	// Agent never acknowledged (died between Claim and SubmitResult) and the delivery
	// cap before giving up (FAILED "abandoned"). Read per call → hot-reload
	// (ADR-023). nil or <= 0 disables redelivery (pre-#111 behavior).
	redispatchAfter func() time.Duration
	maxAttempts     func() int
}

// NewService creates the command service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// SetRedispatch enables redelivery of orphaned commands (#111). The window must be LARGER
// than the longest expected command: the Agent executes the claimed batch serially, and a
// large-torrent migrate_pull keeps commands legitimately DISPATCHED for hours
// (#87) — redelivering too early would duplicate execution. nil-safe.
func (s *Service) SetRedispatch(after func() time.Duration, maxAttempts func() int) {
	s.redispatchAfter = after
	s.maxAttempts = maxAttempts
}

// redispatchWindow returns the current window (0 = disabled).
func (s *Service) redispatchWindow() time.Duration {
	if s.redispatchAfter == nil {
		return 0
	}
	return s.redispatchAfter()
}

// attemptCap returns the current delivery cap (min. 1 when redelivery is enabled).
func (s *Service) attemptCap() int {
	if s.maxAttempts == nil {
		return 1
	}
	if n := s.maxAttempts(); n > 0 {
		return n
	}
	return 1
}

// rowQuerier is the minimum that Enqueue needs — satisfied by both *pgxpool.Pool
// and pgx.Tx, to enqueue within a caller's transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Enqueue adds a PENDING command for a Node and returns its id.
func (s *Service) Enqueue(ctx context.Context, nodeID, cmdType string, payload json.RawMessage) (string, error) {
	return enqueue(ctx, s.pool, nodeID, cmdType, payload)
}

// EnqueueTx is like Enqueue, but within a caller's transaction — the command only
// exists if the tx commits. Used when enqueuing and changing the state that justifies
// it need to be atomic (e.g.: remove from source + complete the migration; without it,
// a mid-way error leaves the command enqueued and re-executes every cycle — incident
// 2026-07-18, 27k remove_torrent).
func (s *Service) EnqueueTx(ctx context.Context, tx pgx.Tx, nodeID, cmdType string, payload json.RawMessage) (string, error) {
	return enqueue(ctx, tx, nodeID, cmdType, payload)
}

func enqueue(ctx context.Context, q rowQuerier, nodeID, cmdType string, payload json.RawMessage) (string, error) {
	var payloadArg any
	if len(payload) > 0 {
		payloadArg = string(payload)
	}
	var id string
	if err := q.QueryRow(ctx,
		`INSERT INTO commands (node_id, type, payload, status)
		 VALUES ($1, $2, $3::jsonb, 'PENDING')
		 RETURNING id::text`,
		nodeID, cmdType, payloadArg).Scan(&id); err != nil {
		return "", fmt.Errorf("enqueue command: %w", err)
	}
	return id, nil
}

// Status is the state of a command for tracking by the operator (Portal).
type Status struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Status       string     `json:"status"` // PENDING | DISPATCHED | DONE | FAILED
	Error        *string    `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	DispatchedAt *time.Time `json:"dispatched_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

// Get returns the state of a command (for the Portal to display the action's progress).
func (s *Service) Get(ctx context.Context, id string) (Status, error) {
	var st Status
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, type, status, error, created_at, dispatched_at, finished_at
		   FROM commands WHERE id = $1`, id).
		Scan(&st.ID, &st.Type, &st.Status, &st.Error, &st.CreatedAt, &st.DispatchedAt, &st.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, ErrNotFound
	}
	return st, err
}

// Claim atomically claims the PENDING commands of a Node, marking them as
// DISPATCHED, and returns them for execution by the Agent.
//
// With redelivery enabled (#111), it also returns DISPATCHED whose Agent never acknowledged
// within the window (died between Claim and SubmitResult) while attempts < cap, and
// marks FAILED ("abandoned by the Agent") those that exceed the cap — the command appears
// failed in the Portal instead of vanishing in an eternal DISPATCHED.
func (s *Service) Claim(ctx context.Context, nodeID string, limit int) ([]Command, error) {
	if limit <= 0 {
		limit = 50
	}
	window := s.redispatchWindow()
	if window > 0 {
		if _, err := s.pool.Exec(ctx,
			`UPDATE commands SET status = 'FAILED',
			        error = 'abandoned: the Agent did not acknowledge the command after '||retry_count||' delivery(ies)',
			        finished_at = now()
			  WHERE node_id = $1 AND status = 'DISPATCHED'
			    AND dispatched_at < now() - make_interval(secs => $2)
			    AND retry_count >= $3`,
			nodeID, window.Seconds(), s.attemptCap()); err != nil {
			return nil, fmt.Errorf("expire abandoned commands: %w", err)
		}
	}
	// windowSecs <= 0 disables the redelivery branch in the predicate (only PENDING).
	// Order matters and must hold TWICE (#28): in batch selection (subquery) and
	// in the return — the RETURNING of an UPDATE does not guarantee order, so the final
	// read reorders. The Agent executes serially, in the order received: this is where the
	// queue becomes a sequence. Orders by seq (insertion order), not by created_at, which ties
	// within a transaction.
	rows, err := s.pool.Query(ctx,
		`WITH claimed AS (
		   UPDATE commands SET status = 'DISPATCHED', dispatched_at = now(), retry_count = retry_count + 1
		    WHERE id IN (
		       SELECT id FROM commands
		        WHERE node_id = $1 AND (
		              status = 'PENDING'
		              OR ($3::float8 > 0 AND status = 'DISPATCHED'
		                  AND dispatched_at < now() - make_interval(secs => $3)
		                  AND retry_count < $4)
		        )
		        ORDER BY seq
		        LIMIT $2
		        FOR UPDATE SKIP LOCKED
		    )
		    RETURNING id, type, payload, seq
		 )
		 SELECT id::text, type, payload FROM claimed ORDER BY seq`,
		nodeID, limit, window.Seconds(), s.attemptCap())
	if err != nil {
		return nil, fmt.Errorf("claim commands: %w", err)
	}
	defer rows.Close()

	cmds := make([]Command, 0)
	for rows.Next() {
		var c Command
		var payload []byte
		if err := rows.Scan(&c.ID, &c.Type, &payload); err != nil {
			return nil, err
		}
		if len(payload) > 0 {
			c.Payload = json.RawMessage(payload)
		}
		cmds = append(cmds, c)
	}
	return cmds, rows.Err()
}

// ResultOf returns the result payload of a completed command (e.g.: screenshot frames
// extracted by the Agent, ADR-026) — for flows that need the return value,
// not just the status (unlike Get/Status).
func (s *Service) ResultOf(ctx context.Context, id string) (json.RawMessage, error) {
	var result []byte
	err := s.pool.QueryRow(ctx, `SELECT result FROM commands WHERE id = $1`, id).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(result), nil
}

// WaitDone waits for a command to reach a terminal state (DONE or FAILED) via
// polling (1s), or returns a timeout/cancellation error. Generalizes what was
// previously private in torrent.Service (prepare_upload, ADR-019) for reuse by other
// blocking flows (e.g.: extract_screenshots, ADR-026).
func (s *Service) WaitDone(ctx context.Context, id string, timeout time.Duration) (Status, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		st, err := s.Get(ctx, id)
		if err == nil && (st.Status == "DONE" || st.Status == "FAILED") {
			return st, nil
		}
		if time.Now().After(deadline) {
			return Status{}, errors.New("timeout waiting for Node")
		}
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// SubmitResult records the outcome of a command, ensuring it belongs to the Node.
func (s *Service) SubmitResult(ctx context.Context, nodeID, commandID string, res Result) error {
	status := res.Status
	if status != "DONE" && status != "FAILED" {
		return fmt.Errorf("invalid status: %q", status)
	}
	var resultArg any
	if len(res.Result) > 0 {
		resultArg = string(res.Result)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE commands SET status = $3, result = $4::jsonb, error = NULLIF($5,''), finished_at = now()
		  WHERE id = $1 AND node_id = $2`,
		commandID, nodeID, status, resultArg, res.Error)
	if err != nil {
		return fmt.Errorf("record result: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
