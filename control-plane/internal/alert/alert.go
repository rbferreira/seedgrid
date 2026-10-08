// Package alert implements the Control Plane's Alert Service: opens and resolves
// deterministic alerts (ADR-008). An alert is a condition with a lifecycle
// (opens and closes), unlike an event (a point-in-time fact — see package events).
package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Alert is the representation of an alert exposed by the API.
type Alert struct {
	ID       string  `json:"id"`
	Code     string  `json:"code"`
	Severity string  `json:"severity"`
	Status   string  `json:"status"`
	NodeID   *string `json:"node_id,omitempty"`
	Message  *string `json:"message,omitempty"`
	// Payload: the alert data, from which the reader builds the phrase in their language (ADR-041 §3;
	// the keys for each code are in messages.Registry). Message is the fallback.
	Payload    json.RawMessage `json:"payload,omitempty"`
	OpenedAt   time.Time       `json:"opened_at"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty"`
	// Expired: closed due to VALIDITY (#211), not because the condition ceased to exist. The
	// Portal needs to distinguish this, otherwise it says "resolved" about something nobody fixed.
	Expired bool `json:"expired"`
}

// Notifier receives alerts that OPEN (already deduplicated — only what's new) and those that
// RESOLVE, for external telemetry (ADR-028). It is satisfied by notify.AlertHook
// structurally, without this package needing to import notify.
//
// Contract: the implementation is best-effort and DOES NOT block. An offline external channel must
// not prevent the alert from being recorded — the database/Portal is the source of truth; notifying is
// a convenience.
type Notifier interface {
	AlertOpened(code, severity, nodeID, message string, payload map[string]any)
	AlertResolved(code, severity, nodeID, message string, payload map[string]any)
}

// Service centralizes operations on alerts.
type Service struct {
	pool     *pgxpool.Pool
	notifier Notifier // nil = no external telemetry (the alert still exists in the Portal)
}

// NewService creates the Alert Service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// SetNotifier enables external telemetry (ADR-028). Optional; nil-safe.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// Open opens an alert if there isn't already an OPEN one for the same condition
// (code + node_id). It is idempotent: repeating the same condition doesn't duplicate it.
// An empty nodeID records the alert without an associated Node. Returns true if a new alert
// was opened (false if one was already open).
func (s *Service) Open(ctx context.Context, code, severity, nodeID, message string, payload map[string]any) (bool, error) {
	return s.OpenEpisode(ctx, code, severity, nodeID, "", message, payload)
}

// OpenEpisode is the Open for rules whose alert can EXPIRE (#211). Besides dedup by
// OPEN, it refuses to open if the same episode — the identity of the condition, e.g., the IPv6
// rotation that originated it — has already generated an alert, in ANY status.
//
// This is what makes the validity safe. The condition for these rules remains valid after
// the alert expires (the idle Node remains without inbound connection); without the episode, the
// worker would reopen it on the next pass and notify again on every validity cycle.
// For the same reason a reboot stops re-notifying: going OFFLINE closes the alert, and
// coming back finds the episode already registered.
//
// An empty episode = no episode, identical to Open.
func (s *Service) OpenEpisode(ctx context.Context, code, severity, nodeID, episode, message string, payload map[string]any) (bool, error) {
	var nodeArg any
	if nodeID != "" {
		nodeArg = nodeID
	}
	var payloadJSON any
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return false, fmt.Errorf("serialize payload: %w", err)
		}
		payloadJSON = string(b)
	}

	// IS NOT DISTINCT FROM handles NULL node_id correctly in deduplication.
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO alerts (code, severity, node_id, message, payload, episode_key)
		 SELECT $1, $2, $3, NULLIF($4,''), $5::jsonb, NULLIF($6::text,'')
		  WHERE NOT EXISTS (
		        SELECT 1 FROM alerts
		         WHERE code = $1 AND status = 'OPEN'
		           AND node_id IS NOT DISTINCT FROM $3::uuid)
		    AND ($6::text = '' OR NOT EXISTS (
		        SELECT 1 FROM alerts
		         WHERE code = $1 AND episode_key = $6::text
		           AND node_id IS NOT DISTINCT FROM $3::uuid))
		 RETURNING id::text`,
		code, severity, nodeArg, message, payloadJSON, episode).Scan(&id)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return false, nil // there was already an open alert
		}
		return false, fmt.Errorf("open alert %s: %w", code, err)
	}
	// Notifies only what is NEW (ADR-028): the deduplication above already ensures the same
	// condition doesn't reopen, so this doesn't become a repetition on every worker pass.
	if s.notifier != nil {
		s.notifier.AlertOpened(code, severity, nodeID, message, payload)
	}
	return true, nil
}

// UpdatePayload updates the message/payload of the OPEN alert for the condition (code + node_id),
// if the message is DIFFERENT from the one already recorded. It exists for cases where the condition
// evolves (e.g., the Node rotated IPv6 again) while the previous alert hasn't
// resolved yet: without this, Open()'s dedup silently discards the new information — whoever
// consumes the alert (Portal, n8n via webhook) never learns about the latest state
// (#209). Returns true if something was actually updated.
//
// It doesn't use Resolve()+Open() for this: closing the old alert would notify "resolved" without the
// condition being fixed — rotating to a new address isn't less broken than the
// previous one, it's just a newer version of the SAME problem. That's why it reuses
// AlertOpened (not AlertResolved) to notify: whoever is on the other end wants to act on the
// current address, not celebrate a closure that didn't happen.
//
// episode, if not empty, becomes the alert's episode (#211): the condition evolved, and
// it's the MOST RECENT episode that cannot reopen after this alert expires.
func (s *Service) UpdatePayload(ctx context.Context, code, nodeID, severity, episode, message string, payload map[string]any) (bool, error) {
	var nodeArg any
	if nodeID != "" {
		nodeArg = nodeID
	}
	var payloadJSON any
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return false, fmt.Errorf("serialize payload: %w", err)
		}
		payloadJSON = string(b)
	}

	var id string
	err := s.pool.QueryRow(ctx,
		`UPDATE alerts SET message = NULLIF($3,''), payload = $4::jsonb,
		                   episode_key = COALESCE(NULLIF($5::text,''), episode_key), updated_at = now()
		   WHERE code = $1 AND status = 'OPEN' AND node_id IS NOT DISTINCT FROM $2::uuid
		     AND message IS DISTINCT FROM NULLIF($3,'')
		 RETURNING id::text`,
		code, nodeArg, message, payloadJSON, episode).Scan(&id)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return false, nil // no OPEN alert, or the message is already the same
		}
		return false, fmt.Errorf("update alert %s: %w", code, err)
	}
	if s.notifier != nil {
		s.notifier.AlertOpened(code, severity, nodeID, message, payload)
	}
	return true, nil
}

// Resolve closes the OPEN alert for the condition (code + node_id), if it exists.
// Returns the number of resolved alerts.
func (s *Service) Resolve(ctx context.Context, code, nodeID string) (int64, error) {
	var nodeArg any
	if nodeID != "" {
		nodeArg = nodeID
	}
	// RETURNING (instead of just counting rows) because the resolution notification needs the
	// text and hostname of the alert that closed (ADR-028) — the payload is the only source of the
	// hostname here, since the worker records it there when opening.
	rows, err := s.pool.Query(ctx,
		`UPDATE alerts
		    SET status = 'RESOLVED', resolved_at = now(), updated_at = now()
		  WHERE code = $1 AND status = 'OPEN'
		    AND node_id IS NOT DISTINCT FROM $2::uuid
		 RETURNING severity, COALESCE(message, ''), COALESCE(payload::text, '')`,
		code, nodeArg)
	if err != nil {
		return 0, fmt.Errorf("resolve alert %s: %w", code, err)
	}
	type resolved struct{ severity, message, payload string }
	var done []resolved
	for rows.Next() {
		var r resolved
		if err := rows.Scan(&r.severity, &r.message, &r.payload); err != nil {
			rows.Close()
			return 0, fmt.Errorf("resolve alert %s: %w", code, err)
		}
		done = append(done, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("resolve alert %s: %w", code, err)
	}
	if s.notifier != nil {
		for _, r := range done {
			var payload map[string]any
			if r.payload != "" {
				_ = json.Unmarshal([]byte(r.payload), &payload) // invalid payload → no hostname, not fatal
			}
			s.notifier.AlertResolved(code, r.severity, nodeID, r.message, payload)
		}
	}
	return int64(len(done)), nil
}

// Expire closes by VALIDITY the OPEN alerts for `code` whose last notification is older
// than ttl (#211) and returns the closed node ids. DOES NOT notify: the outbound call
// already happened on open, and "Resolved: …" would be a lie — nothing was fixed.
//
// The age is counted from updated_at, not opened_at, because that's where the last notification
// is recorded: a new IPv6 rotation with an open alert goes through UpdatePayload,
// notifies again, and renews the validity.
func (s *Service) Expire(ctx context.Context, code string, ttl time.Duration) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`UPDATE alerts
		    SET status = 'RESOLVED', resolved_at = now(), updated_at = now(), expired = true
		  WHERE code = $1 AND status = 'OPEN'
		    AND updated_at < now() - make_interval(secs => $2)
		 RETURNING COALESCE(node_id::text, '')`,
		code, ttl.Seconds())
	if err != nil {
		return nil, fmt.Errorf("expire alerts %s: %w", code, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("expire alerts %s: %w", code, err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// List returns alerts. If onlyOpen is true, only the OPEN ones; otherwise the most
// recent first.
func (s *Service) List(ctx context.Context, onlyOpen bool) ([]Alert, error) {
	q := `SELECT id::text, code, severity, status, node_id::text, message, payload, opened_at, resolved_at, expired
	        FROM alerts`
	if onlyOpen {
		q += ` WHERE status = 'OPEN'`
	}
	q += ` ORDER BY opened_at DESC LIMIT 500`

	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()

	out := make([]Alert, 0)
	for rows.Next() {
		var a Alert
		var payload []byte
		if err := rows.Scan(&a.ID, &a.Code, &a.Severity, &a.Status, &a.NodeID, &a.Message, &payload, &a.OpenedAt, &a.ResolvedAt, &a.Expired); err != nil {
			return nil, err
		}
		if payload != nil {
			a.Payload = json.RawMessage(payload)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
