package alert

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// spyNotifier logs what the Alert Service sends to telemetry (ADR-028).
type spyNotifier struct {
	mu       sync.Mutex
	open     []string
	closed   []string
	payloads []map[string]any
}

func (s *spyNotifier) AlertOpened(code, _, _, _ string, _ map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = append(s.open, code)
}

func (s *spyNotifier) AlertResolved(code, _, _, _ string, payload map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = append(s.closed, code)
	s.payloads = append(s.payloads, payload)
}

// TestResolveNotifiesTelemetry is the test requested by #161.
//
// The bug: the Alert Worker closed alerts with a direct UPDATE on the table, bypassing
// alert.Service — the only place that calls AlertResolved. Result: the tunable
// `notify.alerts_on_resolve` was enabled by default, promised "notify on resolve",
// and never fired. No error in log and nothing in the Portal indicating this.
//
// Node goes down (opens NODE_OFFLINE) and returns (resolves): BOTH ends must notify.
func TestResolveNotifiesTelemetry(t *testing.T) {
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
		`INSERT INTO clusters (name, description) VALUES ('res-itest', 'resolve test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	// Node without heartbeat for a long time → the worker opens NODE_OFFLINE.
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at)
		 VALUES ($1, 'res-itest-node', 'ONLINE', now() - interval '2 hours')
		 RETURNING id::text`, clusterID).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alerts WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	spy := &spyNotifier{}
	svc := NewService(pool)
	svc.SetNotifier(spy)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := settings.NewForTest(map[string]string{
		"alerting.worker_interval":    "1s",
		"alerting.node_offline_after": "90s",
		"alerting.sample_retention":   "0s",
		"alerting.disk_saturated_pct": "90",
		"alerting.firewalled_grace":   "30m",
		"alerting.incoming_silence":   "12h",
		"alerting.ipv6_change_grace":  "15m",
	})
	w := NewWorker(pool, svc, logger, cfg)

	w.evaluateNodeOffline(ctx)
	if len(spy.open) == 0 {
		t.Fatal("opening did not notify — this path already worked before #161")
	}

	// Node returns to ONLINE → the worker must resolve AND notify.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET status = 'ONLINE', last_seen_at = now() WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark online: %v", err)
	}
	w.evaluateNodeOffline(ctx)

	if len(spy.closed) == 0 {
		t.Fatal("RESOLVE DID NOT NOTIFY — it is exactly bug #161: the Worker closed the " +
			"alert outside the Service and the AlertResolved hook never ran")
	}
	if spy.closed[0] != "NODE_OFFLINE" {
		t.Fatalf("notified code = %q, expected NODE_OFFLINE", spy.closed[0])
	}

	// The alert must actually be closed in the database, not just notified.
	var open int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM alerts WHERE code = 'NODE_OFFLINE' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&open); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	if open != 0 {
		t.Fatalf("alert remained open (%d) — notifying without closing would be worse than the bug", open)
	}
}
