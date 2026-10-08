// Package activity records the Control Plane orchestration activity trail
// (ADR-022): torrent decisions (hold/promotion by the admission controller,
// Scheduler scheduling) and lifecycle transitions (started downloading,
// completed, started seeding). It is its OWN, append-only trail, SEPARATE from
// Alerts (problems) and Events (infra lifecycle) — the "under the
// hood" view of the orchestration happening.
package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Trail Kinds (also used by the Portal to choose icon/color).
const (
	KindAdmissionHold   = "admission_hold"   // admission controller placed a hold (pause) on a download
	KindAdmissionResume = "admission_resume" // admission controller promoted (resumed) a hold
	KindScheduled       = "scheduled"        // Scheduler chose a Node for a new torrent
	KindDownloading     = "downloading"      // observed transition: started downloading
	KindSeeding         = "seeding"          // observed transition: completed and started seeding
	// KindNodeBlocked: scheduling was blocked because the only Node candidate(s)
	// has /data failing. Recorded at the moment the Scheduler's exclusion
	// COSTS something — never on the I/O hiccup that didn't prevent anything (#180).
	KindNodeBlocked = "node_blocked"
	// KindScreenshotHostFailed (#193, ADR-040): a host in the upload chain of
	// screenshots failed. Each failure becomes a Feed entry with the sanitized reason
	// (ADR-040 closed set) and the next host in the queue.
	KindScreenshotHostFailed = "screenshot_host_failed"
)

// AllKinds is the canonical vocabulary of the trail — used by the notification catalog
// (ADR-028) to offer options to the operator with labels. A test in the api package
// ensures the catalog does not fall behind this list.
func AllKinds() []string {
	return []string{KindAdmissionHold, KindAdmissionResume, KindScheduled, KindDownloading, KindSeeding, KindNodeBlocked, KindScreenshotHostFailed}
}

// Recorder writes and reads the activity trail in the Cluster State.
type Recorder struct {
	pool     *pgxpool.Pool
	notifier Notifier // nil = no external telemetry (activity still goes to the Feed)
}

// Notifier receives each recorded activity, for external telemetry (ADR-028). Satisfied
// by notify.ActivityHook — structurally, without this package importing notify. detail follows
// along for rich templates (e.g.: Node "hostname", size_bytes/added_at for
// "completed and started seeding") — notify decides what to do with it; here the
// content is not interpreted. Best-effort: it doesn't block and never makes Record fail.
type Notifier interface {
	ActivityRecorded(kind, message, torrentName, nodeID string, detail map[string]any)
}

// NewRecorder creates the activity feed recorder.
func NewRecorder(pool *pgxpool.Pool) *Recorder {
	return &Recorder{pool: pool}
}

// SetNotifier enables external telemetry (ADR-028). Optional; nil-safe.
func (r *Recorder) SetNotifier(n Notifier) { r.notifier = n }

// Entry is a record to write. Empty TorrentID/NodeID write without the reference;
// nil Detail writes without the "why".
type Entry struct {
	Kind        string
	TorrentID   string
	TorrentName string
	NodeID      string
	Message     string
	Detail      map[string]any
}

// Record writes an activity. It is best-effort on the caller (see ADR-022): a failure
// here must never take down the orchestration — the caller ignores the error (or logs it).
func (r *Recorder) Record(ctx context.Context, e Entry) error {
	var detailJSON any
	if e.Detail != nil {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("serialize detail: %w", err)
		}
		detailJSON = string(b)
	}
	var torrentArg, nodeArg, nameArg any
	if e.TorrentID != "" {
		torrentArg = e.TorrentID
	}
	if e.NodeID != "" {
		nodeArg = e.NodeID
	}
	if e.TorrentName != "" {
		nameArg = e.TorrentName
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO activities (kind, torrent_id, torrent_name, node_id, message, detail)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb)`,
		e.Kind, torrentArg, nameArg, nodeArg, e.Message, detailJSON); err != nil {
		return fmt.Errorf("record activity: %w", err)
	}
	// External telemetry (ADR-028). Detail carries the "hostname" when the caller
	// resolved it (same pattern as Alert) — without it, notify falls back to the raw id. The filter by
	// kind is what contains the volume.
	if r.notifier != nil {
		r.notifier.ActivityRecorded(e.Kind, e.Message, e.TorrentName, e.NodeID, e.Detail)
	}
	return nil
}

// Activity is the representation exposed by the API. NodeHostname and NodeDisplayName (the alias,
// #145) are resolved via JOIN (nodes is soft-delete, so the name persists); TorrentName
// comes denormalized.
type Activity struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	TorrentID       *string         `json:"torrent_id,omitempty"`
	TorrentName     *string         `json:"torrent_name,omitempty"`
	NodeID          *string         `json:"node_id,omitempty"`
	NodeHostname    *string         `json:"node_hostname,omitempty"`
	NodeDisplayName *string         `json:"node_display_name,omitempty"`
	Message         string          `json:"message"`
	Detail          json.RawMessage `json:"detail,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// List returns the most recent activities (timeline). torrentID != "" filters by
// torrent. limit <= 0 or > 500 uses 100.
func (r *Recorder) List(ctx context.Context, torrentID string, limit int) ([]Activity, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx,
		`SELECT a.id::text, a.kind, a.torrent_id::text, a.torrent_name,
		        a.node_id::text, n.hostname, n.display_name, a.message, a.detail, a.created_at
		   FROM activities a
		   LEFT JOIN nodes n ON n.id = a.node_id
		  WHERE ($1 = '' OR a.torrent_id = $1::uuid)
		  ORDER BY a.created_at DESC
		  LIMIT $2`, torrentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	out := make([]Activity, 0)
	for rows.Next() {
		var a Activity
		var detail []byte // jsonb → []byte (nil when NULL)
		if err := rows.Scan(&a.ID, &a.Kind, &a.TorrentID, &a.TorrentName,
			&a.NodeID, &a.NodeHostname, &a.NodeDisplayName, &a.Message, &detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		if len(detail) > 0 {
			a.Detail = json.RawMessage(detail)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
