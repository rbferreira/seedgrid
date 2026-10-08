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

// setupIPv6Worker prepares an ONLINE Node whose IPv6 changed `changedAgo` ago (SQL interval;
// empty = never changed) and whose last incoming connection was `lastIncoming` ago (empty = never).
func setupIPv6Worker(t *testing.T, prev, cur, changedAgo, lastIncoming, grace string) (*Worker, *pgxpool.Pool, string) {
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
		`INSERT INTO clusters (name, description) VALUES ('ipv6-itest', 'IPv6 rotation test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at, bt_port,
		                    previous_public_ipv6, public_ipv6, public_ipv6_changed_at, last_incoming_at)
		 VALUES ($1, 'ipv6-itest-node', 'ONLINE', now(), 6881, NULLIF($2,''), NULLIF($3,''),
		         CASE WHEN $4 = '' THEN NULL ELSE now() - $4::interval END,
		         CASE WHEN $5 = '' THEN NULL ELSE now() - $5::interval END)
		 RETURNING id::text`,
		clusterID, prev, cur, changedAgo, lastIncoming).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alerts WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	return ipv6Worker(pool, grace), pool, nodeID
}

// ipv6Worker sets up a Worker over `pool` with the given grace period. It is separate from setup
// because the zero grace period test needs a SECOND Worker on the SAME Node — the
// test Store is immutable once created.
func ipv6Worker(pool *pgxpool.Pool, grace string) *Worker {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := settings.NewForTest(map[string]string{
		"alerting.worker_interval":    "1s",
		"alerting.node_offline_after": "90s",
		"alerting.sample_retention":   "0s",
		"alerting.disk_saturated_pct": "90",
		"alerting.firewalled_grace":   "30m",
		"alerting.incoming_silence":   "12h",
		"alerting.ipv6_change_grace":  grace,
	})
	return NewWorker(pool, NewService(pool), logger, cfg)
}

// ipv6RowCount counts ALL alert rows for the Node, open or resolved. It is the
// metric that reveals flapping: each reopen creates a new row, so counting only
// OPEN ones yields zero while the worker opens and closes every run, notifying on all.
func ipv6RowCount(t *testing.T, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alerts WHERE code = 'NODE_IPV6_CHANGED' AND node_id = $1`,
		nodeID).Scan(&n); err != nil {
		t.Fatalf("count alert lines: %v", err)
	}
	return n
}

// ipv6CurrentIPv6 reads the current_ipv6 recorded in the OPEN alert payload of the Node. It
// reveals whether a subsequent rotation was actually propagated, or silently swallowed (#209).
func ipv6CurrentIPv6(t *testing.T, pool *pgxpool.Pool, nodeID string) string {
	t.Helper()
	var v string
	if err := pool.QueryRow(context.Background(),
		`SELECT payload->>'current_ipv6' FROM alerts
		  WHERE code = 'NODE_IPV6_CHANGED' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&v); err != nil {
		t.Fatalf("read current_ipv6 from payload: %v", err)
	}
	return v
}

func ipv6OpenCount(t *testing.T, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alerts WHERE code = 'NODE_IPV6_CHANGED' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

const (
	oldPrefix = "2001:db8:601:d300:be24:11ff:fe12:dee1"
	newPrefix = "2001:db8:60b:c000:be24:11ff:fe12:dee1"
)

// Sustained change opens the alert and does not duplicate. This is the 05-06/09/2026 incident: without this
// rule, three Nodes were unreachable for weeks with the Portal green.
func TestEvaluateIPv6Changed_SustainedChange(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()

	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 NODE_IPV6_CHANGED alert, got %d", got)
	}
	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("dedup failed: expected 1, got %d", got)
	}
}

// Anti-flapping: privacy extensions rotate the suffix on their own. Within the grace period
// the change is not yet a verdict.
func TestEvaluateIPv6Changed_WithinGracePeriodDoesNotAlert(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 minutes", "", "15m")
	w.evaluateIPv6Changed(context.Background())
	if got := ipv6OpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts within cooldown, got %d", got)
	}
}

// First boot (there was never a previous address) is DISCOVERY, not rotation. Alerting here
// would trigger an alarm on every new Node joining the Cluster.
func TestEvaluateIPv6Changed_FirstAddressDoesNotAlert(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, "", newPrefix, "2 hours", "", "15m")
	w.evaluateIPv6Changed(context.Background())
	if got := ipv6OpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts on the first address, got %d", got)
	}
}

// RESOLVE — the point that depended on #155.
//
// The right question is not "did the address stop changing", it is "is the firewall letting
// traffic in again". Only last_incoming_at, which DECAYS, answers this: with a sticky
// engine_connectable the Node would keep saying 'connected' and the alert would never clear on its own.
func TestEvaluateIPv6Changed_ResolvesWhenIncomingResumes(t *testing.T) {
	// An incoming connection OLDER than the change does not resolve it: it is a connection from before the firewall broke.
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "3 hours", "15m")
	ctx := context.Background()
	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("entry before the change should not resolve; open = %d", got)
	}

	// An incoming connection AFTER the change proves that the unblocking was redone.
	if _, err := pool.Exec(ctx, `UPDATE nodes SET last_incoming_at = now() WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark recent incoming: %v", err)
	}
	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected resolved alert, got %d open", got)
	}
}

// Grace period 0 disables the rule, like the other alert tunables.
func TestEvaluateIPv6Changed_ZeroGraceDisables(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "0s")
	w.evaluateIPv6Changed(context.Background())
	if got := ipv6OpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("grace 0 should disable the rule, got %d", got)
	}
}

// REGRESSION #204 — once resolved, the alert MUST NOT reopen.
//
// The bug: nothing clears previous_public_ipv6, so the open condition remains true
// forever. When the Node started receiving incoming connections again, open and resolve applied on the
// same run; Open dedup only works while the alert is OPEN, so the next
// run found nothing open and opened it again — a notification every cycle.
// Counting OPEN alerts does not catch this (it yields 0 in both versions): we must count ROWS.
func TestEvaluateIPv6Changed_DoesNotReopenAfterResolving(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()

	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected the open alert, got %d", got)
	}

	// The firewall was fixed and incoming connections returned: the alert resolves.
	if _, err := pool.Exec(ctx, `UPDATE nodes SET last_incoming_at = now() WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark recent incoming: %v", err)
	}
	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected resolved alert, got %d open", got)
	}

	// From here on nothing else can happen, no matter how many runs execute.
	for i := 0; i < 3; i++ {
		w.evaluateIPv6Changed(ctx)
		if got := ipv6OpenCount(t, pool, nodeID); got != 0 {
			t.Fatalf("pass %d reopened the alert (%d open)", i+1, got)
		}
	}
	if got := ipv6RowCount(t, pool, nodeID); got != 1 {
		t.Fatalf("flapping: expected 1 alert row in total, got %d", got)
	}
}

// A Node whose incoming connection is already AFTER the change never opens: the unblocking is already
// in place, there is nothing to warn about. This is the case of those who fix the firewall before the grace period expires.
func TestEvaluateIPv6Changed_IncomingAfterChangeDoesNotOpen(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "5 minutes", "15m")
	w.evaluateIPv6Changed(context.Background())
	if got := ipv6RowCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected no alerts, got %d", got)
	}
}

// REGRESSION #209 — a SUBSEQUENT rotation, while the first alert is still OPEN
// (Node with no demand, days without receiving incoming connections), must not be silently swallowed by
// the Open() dedup. The payload must become the MOST RECENT address, otherwise whoever consumes the
// alert (n8n via webhook) will keep reacting forever to an address that is no longer the issue.
func TestEvaluateIPv6Changed_SecondChangeUpdatesOpenAlert(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()
	spy := &spyNotifier{}
	w.alerts.SetNotifier(spy)

	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 open alert, got %d", got)
	}
	if got := ipv6CurrentIPv6(t, pool, nodeID); got != newPrefix {
		t.Fatalf("payload should contain %q, got %q", newPrefix, got)
	}

	// Repeating the SAME run must not update or notify again — otherwise the
	// flapping from #204 returns, but now via the UpdatePayload path instead of Open().
	w.evaluateIPv6Changed(ctx)
	if got := len(spy.open); got != 1 {
		t.Fatalf("repeated pass without real change notified again: %d openings", got)
	}

	// The prefix rotates AGAIN while the first change alert is still OPEN.
	const newestPrefix = "2001:db8:60f:200:be24:11ff:fe12:dee1"
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET previous_public_ipv6 = public_ipv6, public_ipv6 = $2,
		                   public_ipv6_changed_at = now() - interval '2 hours'
		   WHERE id = $1`, nodeID, newestPrefix); err != nil {
		t.Fatalf("simulate second rotation: %v", err)
	}

	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("second rotation should not open a second row, got %d open", got)
	}
	if got := ipv6RowCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 row in total (UPDATE, not INSERT), got %d", got)
	}
	if got := ipv6CurrentIPv6(t, pool, nodeID); got != newestPrefix {
		t.Fatalf("payload should have been updated to %q, stayed at %q", newestPrefix, got)
	}
	if got := len(spy.open); got != 2 {
		t.Fatalf("second rotation should notify again (n8n needs to know): %d openings", got)
	}
	if got := len(spy.closed); got != 0 {
		t.Fatalf("nothing was actually resolved; could not notify closure: %d", got)
	}
}

// Disabling the rule (grace period 0) must CLOSE whatever was left open. Before, the grace period
// return happened before the resolve and alerts were left orphaned on the dashboard — with no
// way out, because the only thing that resolved them was the rule being disabled itself.
func TestEvaluateIPv6Changed_ZeroGraceResolvesOrphans(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()

	w.evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected the alert open before shutdown, got %d", got)
	}

	ipv6Worker(pool, "0s").evaluateIPv6Changed(ctx)
	if got := ipv6OpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("grace 0 left %d orphaned open alert(s)", got)
	}
}
