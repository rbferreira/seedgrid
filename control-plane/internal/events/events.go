// Package events records relevant platform events (Glossary:
// Event). Events describe facts; they do not replace logs or alerts.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NotifiableCodes is the canonical vocabulary of Event codes that the operator can
// choose to notify (ADR-028). These are lifecycle facts emitted via Emit — it DOES NOT
// include ALERT codes (NODE_OFFLINE, NODE_STORAGE, …), which have their own channel,
// filtered by severity. The notification catalog mirrors this list with labels in
// PT-BR; a test in the api package ensures they do not fall out of sync.
func NotifiableCodes() []string {
	return []string{
		"NODE_REGISTERED", "NODE_DELETED",
		"TORRENT_CREATED", "TORRENT_REMOVED",
		"TORRENT_PAUSED", "TORRENT_RESUMED", "TORRENT_RECHECK",
		"TORRENT_MIGRATION_STARTED", "TORRENT_MIGRATED", "TORRENT_MIGRATION_FAILED",
		"TORRENT_RESEEDED", "TORRENT_PUBLISHED_EXTERNAL",
		"ENGINE_RESTART", "AGENT_UPDATE", "STORAGE_RECOVERY",
	}
}

// Emitter records events in the Cluster State.
type Emitter struct {
	pool     *pgxpool.Pool
	notifier Notifier // nil = no external telemetry (the event still goes to the Portal)
}

// Notifier receives each emitted event for external telemetry (ADR-028). Satisfied
// by notify.EventHook — structurally, without this package importing notify. payload goes
// along for the rich templates (e.g.: Node "hostname", aborted migration "reason",
// removed torrent "actor") — notify decides what to do with it; here we do not
// interpret the content. Best-effort: it does not block and never makes Emit fail.
type Notifier interface {
	EventEmitted(code, severity, nodeID, message string, payload map[string]any)
}

// NewEmitter creates the event emitter.
func NewEmitter(pool *pgxpool.Pool) *Emitter {
	return &Emitter{pool: pool}
}

// SetNotifier enables external telemetry (ADR-028). Optional; nil-safe.
func (e *Emitter) SetNotifier(n Notifier) { e.notifier = n }

// Emit records an event, ensuring the type (event_types) idempotently.
// empty nodeID records event without associated Node. nil payload records without payload.
func (e *Emitter) Emit(ctx context.Context, code, severity, nodeID, message string, payload map[string]any) error {
	var eventTypeID string
	if err := e.pool.QueryRow(ctx,
		`INSERT INTO event_types (code, name, severity)
		 VALUES ($1, $1, $2)
		 ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
		 RETURNING id::text`,
		code, severity).Scan(&eventTypeID); err != nil {
		return fmt.Errorf("ensure event_type %s: %w", code, err)
	}

	var payloadJSON any
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("serialize payload: %w", err)
		}
		payloadJSON = string(b)
	}

	var nodeArg any
	if nodeID != "" {
		nodeArg = nodeID
	}

	if _, err := e.pool.Exec(ctx,
		`INSERT INTO events (event_type_id, node_id, message, payload, severity)
		 VALUES ($1, $2, $3, $4::jsonb, $5)`,
		eventTypeID, nodeArg, message, payloadJSON, severity); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	// External telemetry (ADR-028) only AFTER the event is persisted: we notify what is
	// a fact. It is notify that filters by type — here we do not decide what matters.
	if e.notifier != nil {
		e.notifier.EventEmitted(code, severity, nodeID, message, payload)
	}
	return nil
}

// Event is the representation of an event exposed by the API.
type Event struct {
	ID       string  `json:"id"`
	Code     string  `json:"code"`
	Severity string  `json:"severity"`
	NodeID   *string `json:"node_id,omitempty"`
	Message  *string `json:"message,omitempty"`
	// Payload: the event data, from which the reader builds the phrase in their language (ADR-041 §3;
	// the keys for each code are in messages.Registry). The Message is the fallback.
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// List returns the most recent events (limited). limit <= 0 uses 100.
func (e *Emitter) List(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := e.pool.Query(ctx,
		`SELECT e.id::text, et.code, e.severity, e.node_id::text, e.message, e.payload, e.created_at
		   FROM events e JOIN event_types et ON et.id = e.event_type_id
		  ORDER BY e.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	out := make([]Event, 0)
	for rows.Next() {
		var ev Event
		var payload []byte
		if err := rows.Scan(&ev.ID, &ev.Code, &ev.Severity, &ev.NodeID, &ev.Message, &payload, &ev.CreatedAt); err != nil {
			return nil, err
		}
		if payload != nil {
			ev.Payload = json.RawMessage(payload)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
