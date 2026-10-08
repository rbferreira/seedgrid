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

// setupDiskFull creates an ONLINE Node with heartbeat (disk) and returns the Worker + ids.
// diskFree/diskTotal in bytes; minFree is the margin for the admission.disk_min_free tunable.
func setupDiskFull(t *testing.T, diskFree, diskTotal, minFree int64) (*Worker, *pgxpool.Pool, string, context.Context) {
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
		`INSERT INTO clusters (name, description) VALUES ('diskfull-itest', 'teste disco cheio') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at)
		 VALUES ($1, 'diskfull-node', 'ONLINE', now()) RETURNING id::text`,
		clusterID).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO heartbeats (node_id, disk_total_bytes, disk_free_bytes)
		 VALUES ($1, $2, $3)`, nodeID, diskTotal, diskFree); err != nil {
		t.Fatalf("insert heartbeat: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alerts WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM torrents WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM heartbeats WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := settings.NewForTest(map[string]string{
		"alerting.worker_interval":    "1s",
		"alerting.node_offline_after": "90s",
		"admission.disk_min_free":     itoa(minFree),
	})
	return NewWorker(pool, NewService(pool), logger, cfg), pool, nodeID, ctx
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func diskFullOpen(t *testing.T, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alerts WHERE code = 'NODE_DISK_FULL' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

const gib = int64(1) << 30

// TestEvaluateDiskFull_OpensAndResolves: Node with low projected space opens NODE_DISK_FULL;
// freeing space resolves it. Covers the core of the ADR-032 alert.
func TestEvaluateDiskFull_OpensAndResolves(t *testing.T) {
	// 10 GiB free, 5 GiB margin. An 8 GiB torrent downloaded only 50% will still write
	// 4 GiB → projected = 10-4 = 6 GiB. 6 > 5 → does NOT alert yet.
	w, pool, nodeID, ctx := setupDiskFull(t, 10*gib, 100*gib, 5*gib)
	if _, err := pool.Exec(ctx,
		`INSERT INTO torrents (node_id, name, status, size_bytes, progress)
		 VALUES ($1, 'meio', 'DOWNLOADING', $2, 0.5)`, nodeID, 8*gib); err != nil {
		t.Fatalf("insert torrent: %v", err)
	}
	w.evaluateDiskFull(ctx)
	if got := diskFullOpen(t, pool, nodeID); got != 0 {
		t.Fatalf("projected 6 GiB > margin 5 GiB: expected 0 alerts, got %d", got)
	}

	// Now a 2nd 10 GiB torrent at 0% → +10 GiB to write → projected = 10-4-10 = -4 GiB.
	// -4 < 5 → alert opens.
	if _, err := pool.Exec(ctx,
		`INSERT INTO torrents (node_id, name, status, size_bytes, progress)
		 VALUES ($1, 'large', 'QUEUED', $2, 0)`, nodeID, 10*gib); err != nil {
		t.Fatalf("insert torrent 2: %v", err)
	}
	w.evaluateDiskFull(ctx)
	if got := diskFullOpen(t, pool, nodeID); got != 1 {
		t.Fatalf("projected negative: expected 1 alert, got %d", got)
	}
	// Dedup: reevaluating does not duplicate.
	w.evaluateDiskFull(ctx)
	if got := diskFullOpen(t, pool, nodeID); got != 1 {
		t.Fatalf("dedup failed: expected 1, got %d", got)
	}

	// Remove the large torrent → projected returns to 6 GiB > 5 → resolves.
	if _, err := pool.Exec(ctx, `UPDATE torrents SET deleted_at = now() WHERE node_id = $1 AND name = 'large'`, nodeID); err != nil {
		t.Fatalf("remove large torrent: %v", err)
	}
	w.evaluateDiskFull(ctx)
	if got := diskFullOpen(t, pool, nodeID); got != 0 {
		t.Fatalf("expected resolved alert (0), got %d", got)
	}
}

// TestEvaluateDiskFull_Disabled: minFree=0 disables the rule (never opens).
func TestEvaluateDiskFull_Disabled(t *testing.T) {
	w, pool, nodeID, ctx := setupDiskFull(t, 1*gib, 100*gib, 0) // almost out of space, but rule off
	if _, err := pool.Exec(ctx,
		`INSERT INTO torrents (node_id, name, status, size_bytes, progress)
		 VALUES ($1, 'large', 'QUEUED', $2, 0)`, nodeID, 50*gib); err != nil {
		t.Fatalf("insert torrent: %v", err)
	}
	w.evaluateDiskFull(ctx)
	if got := diskFullOpen(t, pool, nodeID); got != 0 {
		t.Fatalf("rule disabled (minFree=0): expected 0 alerts, got %d", got)
	}
}
