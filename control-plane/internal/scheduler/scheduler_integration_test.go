package scheduler_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
)

const gib = int64(1) << 30

func newPool(t *testing.T) (*pgxpool.Pool, context.Context) {
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
	return pool, context.Background()
}

// mkNode creates an eligible ONLINE Node with disk heartbeat and returns its id.
func mkNode(t *testing.T, pool *pgxpool.Pool, ctx context.Context, clusterID, host string, health int, diskFree, diskTotal int64) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, engine, status, engine_reachable, last_seen_at, health_score, last_ip)
		 VALUES ($1, $2, 'qbittorrent', 'ONLINE', TRUE, now(), $3, '10.0.0.9') RETURNING id::text`,
		clusterID, host, health).Scan(&id); err != nil {
		t.Fatalf("create node %s: %v", host, err)
	}
	if diskTotal > 0 {
		if _, err := pool.Exec(ctx,
			`INSERT INTO heartbeats (node_id, disk_total_bytes, disk_free_bytes) VALUES ($1, $2, $3)`,
			id, diskTotal, diskFree); err != nil {
			t.Fatalf("heartbeat %s: %v", host, err)
		}
	}
	return id
}

// TestSelect_DiskAware covers the Scheduler's space filter (ADR-032).
func TestSelect_DiskAware(t *testing.T) {
	pool, ctx := newPool(t)
	var clusterID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO clusters (name, description) VALUES ('sched-itest', 'x') RETURNING id::text`).Scan(&clusterID); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	// Full Node (high health, but 2 GiB free) and Roomy Node (lower health, 100 GiB).
	full := mkNode(t, pool, ctx, clusterID, "full", 100, 2*gib, 500*gib)
	slack := mkNode(t, pool, ctx, clusterID, "slack", 50, 100*gib, 500*gib)
	// The test database is shared across packages (orphan nodes from other harnesses
	// would be eligible and, without disk data, would "fit" any size — masking
	// ErrNoNodeWithSpace). It makes all nodes that are not part of this test ineligible.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET last_seen_at = now() - interval '1 hour' WHERE id NOT IN ($1, $2)`,
		full, slack); err != nil {
		t.Fatalf("neutralize other nodes: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM torrent_scheduler WHERE selected_node IN ($1,$2)`, full, slack)
		_, _ = pool.Exec(ctx, `DELETE FROM heartbeats WHERE node_id IN ($1,$2)`, full, slack)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id IN ($1,$2)`, full, slack)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	s := scheduler.NewService(pool)
	minFree := int64(5) * gib

	// 10 GiB Torrent: DOES NOT fit in the full one (2 GiB) → despite higher health, picks the roomy one.
	dec, err := s.Select(ctx, 10*gib, minFree)
	if err != nil {
		t.Fatalf("Select with space on the loose node: %v", err)
	}
	if dec.NodeID != slack {
		t.Errorf("selected %s, want the loose one (the full one cannot fit)", dec.NodeID)
	}

	// 200 GiB Torrent: doesn't fit in any → ErrNoNodeWithSpace (distinct from no eligible).
	_, err = s.Select(ctx, 200*gib, minFree)
	if !errors.Is(err, scheduler.ErrNoNodeWithSpace) {
		t.Errorf("Select 200 GiB = %v, want ErrNoNodeWithSpace", err)
	}
	// The refusal states the numbers: what was requested, the margin, and the best space that actually
	// cannot accommodate the request (#22).
	var noSpace *scheduler.NoNodeWithSpaceError
	if !errors.As(err, &noSpace) {
		t.Fatalf("Select 200 GiB = %v, want a *NoNodeWithSpaceError", err)
	}
	if noSpace.NeedBytes != 200*gib || noSpace.MinFreeBytes != minFree {
		t.Errorf("rejection numbers = %+v, want need=200 GiB and the test margin", noSpace)
	}
	if noSpace.BestFreeBytes <= 0 || noSpace.BestFreeBytes >= 200*gib+minFree {
		t.Errorf("best free space = %d: must be positive and less than requested + margin", noSpace.BestFreeBytes)
	}

	// needBytes=0 (magnet): no disk filter → chooses by health (the full one, health 100).
	dec, err = s.Select(ctx, 0, minFree)
	if err != nil {
		t.Fatalf("Select without size: %v", err)
	}
	if dec.NodeID != full {
		t.Errorf("without size selected %s, want the one with highest health (full)", dec.NodeID)
	}
}

// TestSelect_ExcludesBrokenStorage covers the Scheduler's storage_ok filter (#139).
// A Node whose /data is not mounted writes to the SYSTEM disk, and the heartbeat starts
// measuring the rootfs — so it seems to have space and the disk filter won't block it. Only
// storage_ok excludes it.
func TestSelect_ExcludesBrokenStorage(t *testing.T) {
	pool, ctx := newPool(t)
	var clusterID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO clusters (name, description) VALUES ('sched-storage-itest', 'x') RETURNING id::text`).Scan(&clusterID); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	// The broken one has HIGHER health and spare disk space: without the filter, it would win.
	broken := mkNode(t, pool, ctx, clusterID, "storage-broken", 100, 400*gib, 500*gib)
	healthy := mkNode(t, pool, ctx, clusterID, "storage-healthy", 10, 400*gib, 500*gib)
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET storage_ok = FALSE, storage_detail = 'not a mountpoint' WHERE id = $1`, broken); err != nil {
		t.Fatalf("mark storage broken: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE nodes SET storage_ok = TRUE WHERE id = $1`, healthy); err != nil {
		t.Fatalf("mark storage healthy: %v", err)
	}
	// Test database is shared across packages — neutralizes nodes from other harnesses.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET last_seen_at = now() - interval '1 hour' WHERE id NOT IN ($1, $2)`,
		broken, healthy); err != nil {
		t.Fatalf("neutralize other nodes: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM torrent_scheduler WHERE selected_node IN ($1,$2)`, broken, healthy)
		_, _ = pool.Exec(ctx, `DELETE FROM heartbeats WHERE node_id IN ($1,$2)`, broken, healthy)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id IN ($1,$2)`, broken, healthy)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	s := scheduler.NewService(pool)
	minFree := int64(5) * gib

	dec, err := s.Select(ctx, 10*gib, minFree)
	if err != nil {
		t.Fatalf("Select with a healthy Node: %v", err)
	}
	if dec.NodeID != healthy {
		t.Errorf("selected %s, want the one with healthy storage (the broken one has higher health but /data is down)", dec.NodeID)
	}

	// storage_ok NULL (old Agent that doesn't report the field) CANNOT exclude.
	if _, err := pool.Exec(ctx, `UPDATE nodes SET storage_ok = NULL WHERE id = $1`, healthy); err != nil {
		t.Fatalf("revert storage to NULL: %v", err)
	}
	if _, err := s.Select(ctx, 10*gib, minFree); err != nil {
		t.Errorf("Select with storage_ok NULL = %v, want eligible (old Agent cannot be blocked)", err)
	}

	// With no healthy storage Node left: it's "no eligible", not "out of space".
	if _, err := pool.Exec(ctx, `UPDATE nodes SET storage_ok = FALSE WHERE id = $1`, healthy); err != nil {
		t.Fatalf("break second storage: %v", err)
	}
	if _, err := s.Select(ctx, 10*gib, minFree); !errors.Is(err, scheduler.ErrNoEligibleNode) {
		t.Errorf("Select without healthy storage = %v, want ErrNoEligibleNode", err)
	}
}

// Exclusion due to failed storage is no longer silent (#180). When it is the reason for
// having no eligible Node, the error needs to SAY this — before the operator received "no
// eligible Seed Node at the moment" with all Nodes online in the Portal, with nothing linking
// one thing to the other.
func TestSelect_ErrorReportsWhoLeftDueToStorage(t *testing.T) {
	pool, ctx := newPool(t)
	var clusterID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO clusters (name, description) VALUES ('sched-blocked-itest', 'x') RETURNING id::text`).Scan(&clusterID); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	// Only Node in the cluster, and with /data down: no one is left.
	broken := mkNode(t, pool, ctx, clusterID, "node-with-disk-down", 100, 400*gib, 500*gib)
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET storage_ok = FALSE, storage_detail = 'device not responding' WHERE id = $1`, broken); err != nil {
		t.Fatalf("mark storage broken: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET last_seen_at = now() - interval '1 hour' WHERE id <> $1`, broken); err != nil {
		t.Fatalf("neutralize other nodes: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM heartbeats WHERE node_id = $1`, broken)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, broken)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	_, err := scheduler.NewService(pool).Select(ctx, 10*gib, int64(5)*gib)
	if err == nil {
		t.Fatal("expected error: the only Node has faulty storage")
	}
	// It continues to satisfy the sentinel — handlers use errors.Is and must not break.
	if !errors.Is(err, scheduler.ErrNoEligibleNode) {
		t.Errorf("errors.Is(ErrNoEligibleNode) failed: %v", err)
	}
	var blocked *scheduler.NoEligibleNodeError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected NoEligibleNodeError, got %T: %v", err, err)
	}
	if len(blocked.BlockedByStorage) != 1 || blocked.BlockedByStorage[0] != "node-with-disk-down" {
		t.Errorf("BlockedByStorage = %v, want [node-with-disk-down]", blocked.BlockedByStorage)
	}
	if !strings.Contains(err.Error(), "failing storage") {
		t.Errorf("message does not explain the reason: %q", err.Error())
	}
}

// With one healthy Node available, excluding the other doesn't cause any error: scheduling
// happens normally and no one needs to be notified.
func TestSelect_SilentDeletionWhenHealthyNodeExists(t *testing.T) {
	pool, ctx := newPool(t)
	var clusterID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO clusters (name, description) VALUES ('sched-quiet-itest', 'x') RETURNING id::text`).Scan(&clusterID); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	broken := mkNode(t, pool, ctx, clusterID, "quiet-broken", 100, 400*gib, 500*gib)
	healthy := mkNode(t, pool, ctx, clusterID, "quiet-healthy", 10, 400*gib, 500*gib)
	if _, err := pool.Exec(ctx, `UPDATE nodes SET storage_ok = FALSE WHERE id = $1`, broken); err != nil {
		t.Fatalf("mark broken: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET last_seen_at = now() - interval '1 hour' WHERE id NOT IN ($1,$2)`, broken, healthy); err != nil {
		t.Fatalf("neutralize other nodes: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM torrent_scheduler WHERE selected_node IN ($1,$2)`, broken, healthy)
		_, _ = pool.Exec(ctx, `DELETE FROM heartbeats WHERE node_id IN ($1,$2)`, broken, healthy)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id IN ($1,$2)`, broken, healthy)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	dec, err := scheduler.NewService(pool).Select(ctx, 10*gib, int64(5)*gib)
	if err != nil {
		t.Fatalf("there was a healthy Node, should not fail: %v", err)
	}
	if dec.NodeID != healthy {
		t.Errorf("selected %s, want the healthy one", dec.NodeID)
	}
}
