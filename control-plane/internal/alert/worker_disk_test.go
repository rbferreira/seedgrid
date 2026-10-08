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

// setupWorker prepares a pool + a test Node (ONLINE, recent heartbeat) and returns
// the Worker (white-box) and the Node id. Skips if there is no test database.
// `satFor` is the alerting.disk_saturated_for tunable (#150): "0s" opens on the first read.
func setupWorker(t *testing.T, util float64, satFor string) (*Worker, *pgxpool.Pool, string) {
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
		`INSERT INTO clusters (name, description) VALUES ('disk-itest', 'I/O test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at, disk_io_util_pct)
		 VALUES ($1, 'disk-itest-node', 'ONLINE', now(), $2) RETURNING id::text`,
		clusterID, util).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alerts WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// In-memory Store (ADR-023): interval=30s, offline=90s, retention=0 (purge off), disk=90%.
	cfg := settings.NewForTest(map[string]string{
		"alerting.worker_interval":    "30s", // the production default: it provides the comparison margin
		"alerting.node_offline_after": "90s",
		"alerting.sample_retention":   "0s",
		"alerting.disk_saturated_pct": "90",
		"alerting.disk_saturated_for": satFor,
	})
	w := NewWorker(pool, NewService(pool), logger, cfg)
	return w, pool, nodeID
}

func openCount(t *testing.T, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alerts WHERE code = 'DISK_IO_SATURATED' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

// With no grace period (0s) the rule behaves as before #150: opens on the first read.
func TestEvaluateDiskIOSaturated(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "0s") // above the threshold (90)
	ctx := context.Background()

	// 1) Disk saturated → opens alert.
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 open DISK_IO_SATURATED alert, got %d", got)
	}

	// 2) Reevaluating doesn't duplicate (dedup by code+node).
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("dedup failed: expected 1, got %d", got)
	}

	// 3) Disk eases below the margin (90-15=75) → resolves.
	if _, err := pool.Exec(ctx, `UPDATE nodes SET disk_io_util_pct = 40 WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("lower util: %v", err)
	}
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected resolved alert (0 open), got %d", got)
	}
}

func TestEvaluateDiskIODisabled(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 99, "0s")
	w.cfg = settings.NewForTest(map[string]string{"alerting.disk_saturated_pct": "0"}) // disabled → never opens
	w.evaluateDiskIOSaturated(context.Background())
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("with threshold 0 it should not open an alert, got %d", got)
	}
}
