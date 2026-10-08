// Package audit logs the audit trail of the operator's actions (ADR-025): who
// did what, when, from where. It is a DEDICATED, append-only trail, SEPARATE from Alerts
// (issues), Events (domain facts) and Activity (orchestration). Recorded by a
// middleware at the boundary over mutating protected routes + login. Best-effort and
// decoupled from the request: an audit failure never crashes or blocks the response.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Recorder writes and reads the audit trail in the Cluster State.
type Recorder struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewRecorder creates the audit trail recorder.
func NewRecorder(pool *pgxpool.Pool, logger *slog.Logger) *Recorder {
	return &Recorder{pool: pool, logger: logger}
}

// RecordAction logs an audited action. Satisfies httpx.AuditSink. It is DECOUPLED from
// the request: uses a detached context with a short timeout (logs even if the client
// has disconnected) and NEVER propagates errors — just logs (best-effort, ADR-025).
func (r *Recorder) RecordAction(ctx context.Context, actor, action, code string, params map[string]any, method, path string, status int, ip string) {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	var methodArg, pathArg, ipArg, statusArg any
	if method != "" {
		methodArg = method
	}
	if path != "" {
		pathArg = path
	}
	if ip != "" {
		ipArg = ip
	}
	if status != 0 {
		statusArg = status
	}
	var codeArg, paramsArg any
	if code != "" {
		codeArg = code
	}
	if params != nil {
		if b, err := json.Marshal(params); err == nil {
			paramsArg = string(b)
		}
	}
	if _, err := r.pool.Exec(dctx,
		`INSERT INTO audit_log (actor, action, action_code, action_params, method, path, status_code, source_ip)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)`,
		actor, action, codeArg, paramsArg, methodArg, pathArg, statusArg, ipArg); err != nil && r.logger != nil {
		r.logger.Warn("record audit", "error", err, "actor", actor, "action", action)
	}
}

// Entry is the representation exposed by the API (one line of the trail).
type Entry struct {
	ID     string `json:"id"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	// ActionCode and ActionParams: from which the reader builds the phrase in their language (#57); Action
	// is the fallback. Empty for records prior to #57.
	ActionCode   *string         `json:"action_code,omitempty"`
	ActionParams json.RawMessage `json:"action_params,omitempty"`
	Method       *string         `json:"method,omitempty"`
	Path         *string         `json:"path,omitempty"`
	StatusCode   *int            `json:"status_code,omitempty"`
	SourceIP     *string         `json:"source_ip,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// List returns the most recent actions (timeline). actor != "" filters by operator;
// limit <= 0 or > 500 uses 100.
func (r *Recorder) List(ctx context.Context, actor string, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id::text, actor, action, action_code, action_params, method, path, status_code, source_ip, created_at
		   FROM audit_log
		  WHERE ($1 = '' OR actor = $1)
		  ORDER BY created_at DESC
		  LIMIT $2`, actor, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	out := make([]Entry, 0)
	for rows.Next() {
		var e Entry
		var params []byte
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.ActionCode, &params, &e.Method, &e.Path,
			&e.StatusCode, &e.SourceIP, &e.CreatedAt); err != nil {
			return nil, err
		}
		if params != nil {
			e.ActionParams = json.RawMessage(params)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
