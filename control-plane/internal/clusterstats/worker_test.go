package clusterstats

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

func setup(t *testing.T) (*Worker, *pgxpool.Pool, string) {
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
		`INSERT INTO clusters (name, description) VALUES ('cs-itest', 'clusterstats test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at)
		 VALUES ($1, 'cs-itest-node', 'ONLINE', now()) RETURNING id::text`, clusterID).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM cluster_stat_samples`)
		_, _ = pool.Exec(ctx, `DELETE FROM torrents WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	cfg := settings.NewForTest(map[string]string{
		"metrics.cluster_sample_interval":  "5m",
		"metrics.cluster_sample_retention": "0s",
	})
	w := NewWorker(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	return w, pool, nodeID
}

func insertTorrent(t *testing.T, pool *pgxpool.Pool, nodeID, hash string, dl, ul int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO torrents (node_id, hash, name, status, downloaded_total, uploaded_total)
		 VALUES ($1, $2, $2, 'SEEDING', $3, $4)`, nodeID, hash, dl, ul); err != nil {
		t.Fatalf("insert torrent %s: %v", hash, err)
	}
}

// TestSampleAggregatesClusterTotals: Sample records the SUM of the cumulative totals of all
// torrents. It measures against the baseline (other packages may leave torrents in the shared database).
func TestSampleAggregatesClusterTotals(t *testing.T) {
	w, pool, nodeID := setup(t)
	ctx := context.Background()

	var baseD, baseU int64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(downloaded_total), 0), COALESCE(SUM(uploaded_total), 0) FROM torrents`).
		Scan(&baseD, &baseU); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	insertTorrent(t, pool, nodeID, "cs01", 100, 10)
	insertTorrent(t, pool, nodeID, "cs02", 50, 5)

	w.Sample(ctx)

	var d, u int64
	if err := pool.QueryRow(ctx,
		`SELECT downloaded_total, uploaded_total FROM cluster_stat_samples ORDER BY sampled_at DESC, id DESC LIMIT 1`).
		Scan(&d, &u); err != nil {
		t.Fatalf("read sample: %v", err)
	}
	if d != baseD+150 || u != baseU+15 {
		t.Errorf("sample = (%d, %d), want (%d, %d)", d, u, baseD+150, baseU+15)
	}
}

// TestThroughputWindowAndRetention: Throughput respects the window (excludes a sample outside it)
// and returns the increasing series; the retention purge removes expired samples.
func TestThroughputWindowAndRetention(t *testing.T) {
	w, pool, _ := setup(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM cluster_stat_samples`); err != nil {
		t.Fatalf("clear samples: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO cluster_stat_samples (sampled_at, downloaded_total, uploaded_total) VALUES
		 (now() - interval '10 days', 1000, 100),
		 (now() - interval '2 hours', 2000, 200),
		 (now() - interval '1 hour',  3000, 300)`); err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	pts, err := Throughput(ctx, pool, 24*time.Hour)
	if err != nil {
		t.Fatalf("throughput: %v", err)
	}
	if len(pts) < 2 {
		t.Fatalf("expected >=2 points in the 24h window (the 10-day one is outside), got %d", len(pts))
	}
	if pts[len(pts)-1].Down < pts[0].Down || pts[len(pts)-1].Up < pts[0].Up {
		t.Error("cumulative curve should be non-decreasing")
	}

	// 7-day retention → the 10-day-old sample is purged on the next Sample.
	w.cfg = settings.NewForTest(map[string]string{
		"metrics.cluster_sample_interval":  "5m",
		"metrics.cluster_sample_retention": "168h",
	})
	w.Sample(ctx)

	var old int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM cluster_stat_samples WHERE sampled_at < now() - interval '8 days'`).Scan(&old); err != nil {
		t.Fatalf("count old: %v", err)
	}
	if old != 0 {
		t.Errorf("the 10-day sample should have been purged; %d remained", old)
	}
}
