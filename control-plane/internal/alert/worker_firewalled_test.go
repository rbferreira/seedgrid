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

// setupFirewalledWorker prepares an ONLINE Node whose Engine reports `connectable` since
// `sinceInterval` ago (e.g., '2 hours'). Returns the white-box Worker and the Node id.
func setupFirewalledWorker(t *testing.T, connectable, sinceInterval, grace string) (*Worker, *pgxpool.Pool, string) {
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
		`INSERT INTO clusters (name, description) VALUES ('fw-itest', 'connectability test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at,
		                    engine_connectable, connectable_since, public_ipv6, bt_port)
		 VALUES ($1, 'fw-itest-node', 'ONLINE', now(), $2,
		         now() - $3::interval, '2001:db8:60b:c000:be24:11ff:fe12:dee1', 6881)
		 RETURNING id::text`,
		clusterID, connectable, sinceInterval).Scan(&nodeID); err != nil {
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
		"alerting.firewalled_grace":   grace,
	})
	return NewWorker(pool, NewService(pool), logger, cfg), pool, nodeID
}

func fwOpenCount(t *testing.T, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alerts WHERE code = 'NODE_FIREWALLED' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

// TestEvaluateFirewalled_Sustained: firewalled for longer than the grace period triggers the alert,
// does not duplicate, and resolves as soon as the Engine receives connections again.
func TestEvaluateFirewalled_Sustained(t *testing.T) {
	w, pool, nodeID := setupFirewalledWorker(t, "firewalled", "2 hours", "30m")
	ctx := context.Background()

	w.evaluateFirewalled(ctx)
	if got := fwOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 open NODE_FIREWALLED alert, got %d", got)
	}

	// Dedup by code+node.
	w.evaluateFirewalled(ctx)
	if got := fwOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("dedup failed: expected 1, got %d", got)
	}

	// Received connection → resolves without grace period.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET engine_connectable = 'connected' WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark connected: %v", err)
	}
	w.evaluateFirewalled(ctx)
	if got := fwOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected resolved alert (0 open), got %d", got)
	}
}

// TestEvaluateFirewalled_RecentlyUpDoesNotAlert is why the grace period exists: the
// has_incoming_connections from libtorrent starts false, so every Node is firewalled
// right after startup — just because no one knocked yet. Alerting there would raise an alarm
// on every Engine restart.
func TestEvaluateFirewalled_RecentlyUpDoesNotAlert(t *testing.T) {
	w, pool, nodeID := setupFirewalledWorker(t, "firewalled", "2 minutes", "30m")
	w.evaluateFirewalled(context.Background())
	if got := fwOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts within cooldown, got %d", got)
	}
}

// TestEvaluateFirewalled_UnknownDoesNotAlert: an Agent that doesn't report the field leaves
// engine_connectable NULL. Unknown must never become an accusation.
func TestEvaluateFirewalled_UnknownDoesNotAlert(t *testing.T) {
	w, pool, nodeID := setupFirewalledWorker(t, "firewalled", "2 hours", "30m")
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET engine_connectable = NULL WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("clear connectable: %v", err)
	}
	w.evaluateFirewalled(ctx)
	if got := fwOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts with unknown verdict, got %d", got)
	}
}

// TestEvaluateFirewalled_Disabled: a 0 grace period disables the entire rule.
func TestEvaluateFirewalled_Disabled(t *testing.T) {
	w, pool, nodeID := setupFirewalledWorker(t, "firewalled", "5 hours", "0s")
	w.evaluateFirewalled(context.Background())
	if got := fwOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts with the rule disabled, got %d", got)
	}
}
