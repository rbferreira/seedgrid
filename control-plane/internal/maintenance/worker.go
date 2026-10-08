// Package maintenance implements the retention worker (#112): periodic pruning of
// append-only tables that grew unbounded — terminal commands, events,
// activities, audit_log and expired sessions. Sample tables already had their own
// pruning (alert worker / cluster-stats) and are excluded here.
//
// Each retention is a tunable (ADR-023, hot-reload); 0 disables pruning for that
// category. PENDING/DISPATCHED commands are never pruned — only old DONE/FAILED ones —,
// nor those referenced by a migration (#213).
// DELETEs run in batches to avoid holding locks or bloating the WAL all at once.
package maintenance

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// batchSize limits each DELETE; the sweep repeats until the backlog is cleared.
const batchSize = 5000

// sessionRetention is how long an EXPIRED session remains in the database (recent
// history of "who was logged in"); after that it's trash. Fixed — not a tunable.
const sessionRetention = 7 * 24 * time.Hour

// Worker prunes the append-only tables according to the catalog retentions.
type Worker struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	cfg    *settings.Store
}

// NewWorker creates the maintenance worker.
func NewWorker(pool *pgxpool.Pool, logger *slog.Logger, cfg *settings.Store) *Worker {
	return &Worker{pool: pool, logger: logger, cfg: cfg}
}

func (w *Worker) interval() time.Duration { return w.cfg.Duration("workers.maintenance_interval") }

// Run executes the pruning loop until the context is canceled. beat: heartbeat of the
// health API (see internal/health), same pattern as other workers.
func (w *Worker) Run(ctx context.Context, beat ...func()) {
	interval := w.interval()
	w.logger.Info("maintenance worker started", "interval", interval.String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	mark := func() {
		for _, b := range beat {
			if b != nil {
				b()
			}
		}
	}

	w.Sweep(ctx)
	mark()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("maintenance worker stopped")
			return
		case <-ticker.C:
			w.Sweep(ctx)
			mark()
			// Hot-reload of the interval (ADR-023).
			if cur := w.interval(); cur != interval {
				interval = cur
				ticker.Reset(cur)
				w.logger.Info("maintenance worker interval updated", "interval", cur.String())
			}
		}
	}
}

// Sweep runs a pruning pass across all categories. Failures are logged, not
// propagated (a failing category doesn't block the others). Exported for tests.
func (w *Worker) Sweep(ctx context.Context) {
	// A command referenced by torrent_migrations is NEVER pruned (#213). The two FKs don't
	// have ON DELETE, and a single such command in the batch would fail the entire DELETE:
	// measured on 24/09/2026, 64 migration commands blocked pruning 53,732 others, and the
	// table had never been pruned in production.
	//
	// ON DELETE SET NULL was ruled out because the columns are not just history:
	// `serve_command_id IS NOT NULL` means the source was actually paused (resumeSource),
	// `pull_command_id IS NOT NULL` means the pull was actually queued (discardDestCopy),
	// and Reconcile JOINs the pull of PENDING migrations. Nullifying would silently change
	// these decisions. Preserving costs at most two commands per migration.
	//
	// Two NOT EXISTS, not one with OR (which prevents anti-join) nor NOT IN: the columns are
	// NULL in QUEUED migration, and a NULL in the list makes NOT IN prune NOTHING, without error.
	w.prune(ctx, "commands", w.cfg.Duration("retention.commands"),
		`DELETE FROM commands WHERE id IN (
		    SELECT c.id FROM commands c
		     WHERE c.status IN ('DONE', 'FAILED')
		       AND c.finished_at < now() - make_interval(secs => $1)
		       AND NOT EXISTS (SELECT 1 FROM torrent_migrations m WHERE m.serve_command_id = c.id)
		       AND NOT EXISTS (SELECT 1 FROM torrent_migrations m WHERE m.pull_command_id = c.id)
		     LIMIT $2)`)
	w.prune(ctx, "events", w.cfg.Duration("retention.events"),
		`DELETE FROM events WHERE id IN (
		    SELECT id FROM events
		     WHERE created_at < now() - make_interval(secs => $1)
		     LIMIT $2)`)
	w.prune(ctx, "activities", w.cfg.Duration("retention.activities"),
		`DELETE FROM activities WHERE id IN (
		    SELECT id FROM activities
		     WHERE created_at < now() - make_interval(secs => $1)
		     LIMIT $2)`)
	w.prune(ctx, "audit_log", w.cfg.Duration("retention.audit"),
		`DELETE FROM audit_log WHERE id IN (
		    SELECT id FROM audit_log
		     WHERE created_at < now() - make_interval(secs => $1)
		     LIMIT $2)`)
	// Sessions: logical expiration already blocks usage (expires_at > now() in Authenticate);
	// here we only remove the corpse after the history window.
	w.prune(ctx, "sessions", sessionRetention,
		`DELETE FROM sessions WHERE id IN (
		    SELECT id FROM sessions
		     WHERE expires_at < now() - make_interval(secs => $1)
		     LIMIT $2)`)
}

// prune deletes in batches until the category backlog is cleared. retention <= 0 disables.
func (w *Worker) prune(ctx context.Context, label string, retention time.Duration, query string) {
	if retention <= 0 {
		return
	}
	var total int64
	for {
		tag, err := w.pool.Exec(ctx, query, retention.Seconds(), batchSize)
		if err != nil {
			w.logger.Error("retention pruning", "table", label, "error", err)
			return
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < batchSize {
			break
		}
	}
	if total > 0 {
		w.logger.Info("retention applied", "table", label, "removed", total, "retention", retention.String())
	}
}
