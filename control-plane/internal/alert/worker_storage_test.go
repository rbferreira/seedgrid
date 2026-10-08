package alert

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// setupStorageWorker prepares an ONLINE Node with failing storage since `sinceInterval`
// ago (e.g., '5 minutes'), or without a timestamp when it comes empty.
func setupStorageWorker(t *testing.T, storageOK any, sinceInterval, grace string) (*Worker, *pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	var clusterID, nodeID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO clusters (name, description) VALUES ('st-itest', 'storage test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at,
		                    storage_ok, storage_detail, storage_failing_since)
		 VALUES ($1, 'st-itest-node', 'ONLINE', now(), $2, 'dispositivo não responde',
		         CASE WHEN $3 = '' THEN NULL ELSE now() - $3::interval END)
		 RETURNING id::text`,
		clusterID, storageOK, sinceInterval).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alerts WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := settings.NewForTest(map[string]string{
		"alerting.worker_interval":    "1s",
		"alerting.node_offline_after": "90s",
		"alerting.sample_retention":   "0s",
		"alerting.disk_saturated_pct": "90",
		"alerting.firewalled_grace":   "30m",
		"alerting.storage_grace":      grace,
	})
	return NewWorker(pool, NewService(pool), logger, cfg), pool, nodeID
}

func stOpenCount(t *testing.T, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alerts WHERE code = 'NODE_STORAGE' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

// It is the false positive from #177: a bad read, 30 seconds old, cannot turn
// CRITICAL. That's how node-02 alerted at 17:37:27 and resolved at 17:37:57.
func TestEvaluateStorage_SolutionDoesNotAlert(t *testing.T) {
	w, pool, nodeID := setupStorageWorker(t, false, "30 seconds", "60s")
	w.evaluateStorageUnhealthy(context.Background())
	if got := stOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("30s failure opened an alert (expected 0, got %d) — grace is not being enforced", got)
	}
}

// Sustained failure beyond the grace period is the real incident: must alert.
func TestEvaluateStorage_SustainedAlerts(t *testing.T) {
	w, pool, nodeID := setupStorageWorker(t, false, "5 minutes", "60s")
	ctx := context.Background()

	w.evaluateStorageUnhealthy(ctx)
	if got := stOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 open NODE_STORAGE alert, got %d", got)
	}

	// Dedup by code+node.
	w.evaluateStorageUnhealthy(ctx)
	if got := stOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("dedup failed: expected 1, got %d", got)
	}

	// Storage is back → resolves WITHOUT grace period. Good news doesn't wait.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET storage_ok = true, storage_failing_since = NULL WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark storage ok: %v", err)
	}
	w.evaluateStorageUnhealthy(ctx)
	if got := stOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected alert resolved without grace, still open: %d", got)
	}
}

// Grace period 0 keeps the old behavior (alerts on first read), which is what the
// tunable's description promises to anyone who wants to disable the wait.
func TestEvaluateStorage_ZeroGraceAlertsImmediately(t *testing.T) {
	w, pool, nodeID := setupStorageWorker(t, false, "1 second", "0s")
	w.evaluateStorageUnhealthy(context.Background())
	if got := stOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("grace 0 should alert immediately, got %d", got)
	}
}

// The message DOES NOT repeat the detail (#178): it travels in the payload and the notifier
// renders it as "Detail:". Before, the operator read the same phrase twice.
func TestEvaluateStorage_MessageDoesNotRepeatDetail(t *testing.T) {
	w, pool, nodeID := setupStorageWorker(t, false, "5 minutes", "60s")
	ctx := context.Background()
	w.evaluateStorageUnhealthy(ctx)

	var msg string
	if err := pool.QueryRow(ctx,
		`SELECT message FROM alerts WHERE code = 'NODE_STORAGE' AND node_id = $1`, nodeID).Scan(&msg); err != nil {
		t.Fatalf("read alert: %v", err)
	}
	if msg != "Node storage is not mounted/writable" {
		t.Errorf("message = %q, want the short phrase without the concatenated detail", msg)
	}
}
