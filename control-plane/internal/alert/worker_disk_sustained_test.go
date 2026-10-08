package alert

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// SUSTAINED saturation (#150).
//
// DISK_IO_SATURATED triggered with ONE reading of ~10s above the threshold; in the production database,
// 16 of the 18 alerts between 12/09 and 02/10/2026 lasted exactly 30s (one pass); the other
// two lasted 9min and 4m30. The tests advance a fake clock in
// 30s steps (the Alert Worker interval) instead of sleeping 90 seconds. They share
// setupWorker/openCount with worker_disk_test.go.

// fakeClock is the Worker's injectable clock.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.t = c.t.Add(d)
}

// useFakeClock swaps the Worker's clock for one that only advances when the test tells it to.
func useFakeClock(w *Worker) *fakeClock {
	c := &fakeClock{t: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	w.now = c.Now
	return c
}

// inCount tells if the Node has an ongoing saturation sequence. It looks at the Node's
// entry, not the map size: the test database is shared and may have others.
func inCount(w *Worker, nodeID string) bool {
	_, ok := w.satSince[nodeID]
	return ok
}

// setUtil records the Node's I/O reading and renews the heartbeat — as the Agent does every 10s.
func setUtil(t *testing.T, pool *pgxpool.Pool, nodeID string, util float64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE nodes SET disk_io_util_pct = $1, last_seen_at = now() WHERE id = $2`, util, nodeID); err != nil {
		t.Fatalf("write util: %v", err)
	}
}

// Isolated spike — the case of the 16 30s alerts — does not trigger, and doesn't even trigger later: the
// count died when the reading returned to normal.
func TestEvaluateDiskIO_IsolatedSpikeDoesNotOpen(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "90s")
	clk := useFakeClock(w)
	ctx := context.Background()

	w.evaluateDiskIOSaturated(ctx) // ONE pass seeing 95%
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("single-pass spike opened alert (expected 0, got %d)", got)
	}

	setUtil(t, pool, nodeID, 40) // the peak has passed
	clk.advance(30 * time.Second)
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts, got %d", got)
	}
	if inCount(w, nodeID) {
		t.Fatal("count should have reset to zero when returning below threshold")
	}

	// Time passes freely: without a new sequence, nothing triggers.
	clk.advance(10 * time.Minute)
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts, got %d", got)
	}
}

// Continuous saturation for the grace period triggers — and only then. With 30s passes and
// a 90s grace period it's the fourth reading (0, 30, 60, 90s).
func TestEvaluateDiskIO_SustainedOpens(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "90s")
	clk := useFakeClock(w)
	ctx := context.Background()

	for _, at := range []time.Duration{0, 30 * time.Second, 60 * time.Second} {
		if at > 0 {
			clk.advance(30 * time.Second)
		}
		w.evaluateDiskIOSaturated(ctx)
		if got := openCount(t, pool, nodeID); got != 0 {
			t.Fatalf("opened too early, with %v of saturation: %d alert(s)", at, got)
		}
	}

	clk.advance(30 * time.Second) // Continuous 90s
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("saturation of 90s should open 1 alert, got %d", got)
	}

	// Dedup by code+node still applies.
	clk.advance(30 * time.Second)
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("dedup failed: expected 1, got %d", got)
	}
}

// One pass below the threshold in the middle resets it: 95, 95, 40, 95… it's not continuous saturation,
// even if the total time since the first reading exceeds the grace period.
func TestEvaluateDiskIO_DropResetsCount(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "90s")
	clk := useFakeClock(w)
	ctx := context.Background()
	step := func(util float64) int {
		t.Helper()
		setUtil(t, pool, nodeID, util)
		w.evaluateDiskIOSaturated(ctx)
		n := openCount(t, pool, nodeID)
		clk.advance(30 * time.Second)
		return n
	}

	// t=0, 30: saturated. t=60: drops. Without the drop, it would trigger at t=90.
	for i, util := range []float64{95, 95, 40, 95, 95, 95} { // t=0..150
		if got := step(util); got != 0 {
			t.Fatalf("pass %d (t=%ds, util=%.0f) opened alert, but the sequence was interrupted", i, i*30, util)
		}
	}
	// New sequence started at t=90; 90s later, t=180, completes the grace period.
	if got := step(95); got != 1 {
		t.Fatalf("the restarted sequence should open at t=180s, got %d alert(s)", got)
	}
}

// Passes with millisecond jitter do not delay the trigger: the 90s grace period is a multiple
// of the 30s interval and the fourth reading lands on the tie. Without the slack in the comparison, 89.95s
// would be < 90s and the alert would fire a full pass later, about half the time.
func TestEvaluateDiskIO_JitterDoesNotDelayOpening(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "90s")
	clk := useFakeClock(w)
	ctx := context.Background()

	w.evaluateDiskIOSaturated(ctx) // t=0
	for _, d := range []time.Duration{30*time.Second + 20*time.Millisecond, 29*time.Second + 960*time.Millisecond} {
		clk.advance(d)
		w.evaluateDiskIOSaturated(ctx)
	}
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("with 60s of saturation it should not open, got %d", got)
	}
	clk.advance(29*time.Second + 970*time.Millisecond) // 89.95s since the first reading
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("the fourth pass should open despite jitter, got %d", got)
	}
}

// Grace period 0 is the previous behavior: triggers on the first reading, without depending on the clock.
func TestEvaluateDiskIO_ZeroGraceOpensImmediately(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "0s")
	useFakeClock(w)
	w.evaluateDiskIOSaturated(context.Background())
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("cooldown 0 should open immediately, got %d", got)
	}
}

// RESOLUTION hasn't changed: no grace period, and with 15pp hysteresis. A reading between the slack
// (75) and the threshold (90) keeps the alert open, but no longer counts as saturated — reopening
// after resolution requires a whole new sequence.
func TestEvaluateDiskIO_ResolveRemainsSame(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "90s")
	clk := useFakeClock(w)
	ctx := context.Background()

	for i := 0; i < 4; i++ { // 0, 30, 60, 90s
		w.evaluateDiskIOSaturated(ctx)
		clk.advance(30 * time.Second)
	}
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 open alert, got %d", got)
	}

	setUtil(t, pool, nodeID, 80) // within hysteresis: does not resolve
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("reading in hysteresis band should not resolve: %d open(s)", got)
	}

	setUtil(t, pool, nodeID, 40) // below the slack: resolves RIGHT AWAY, no grace period
	clk.advance(30 * time.Second)
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected alert to be resolved immediately, still open: %d", got)
	}

	// Saturates again: it's a new episode and needs to fulfill the grace period again.
	setUtil(t, pool, nodeID, 95)
	clk.advance(30 * time.Second)
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("reopened without completing cooldown: %d", got)
	}
	for i := 0; i < 3; i++ { // +30s, +60s, +90s of the new sequence
		clk.advance(30 * time.Second)
		w.evaluateDiskIOSaturated(ctx)
	}
	if got := openCount(t, pool, nodeID); got != 1 {
		t.Fatalf("after cooldown, the new episode should open: %d", got)
	}
}

// Outside ONLINE there is no reliable reading: the sequence dies, and the return does not inherit the time
// from before.
func TestEvaluateDiskIO_NodeOfflineResetsCount(t *testing.T) {
	w, pool, nodeID := setupWorker(t, 95, "90s")
	clk := useFakeClock(w)
	ctx := context.Background()

	w.evaluateDiskIOSaturated(ctx)
	clk.advance(60 * time.Second)
	w.evaluateDiskIOSaturated(ctx)
	if !inCount(w, nodeID) {
		t.Fatal("saturated Node should be in count")
	}

	if _, err := pool.Exec(ctx, `UPDATE nodes SET status = 'OFFLINE' WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark offline: %v", err)
	}
	clk.advance(30 * time.Second)
	w.evaluateDiskIOSaturated(ctx)
	if inCount(w, nodeID) {
		t.Fatal("Node OFFLINE should leave count")
	}

	// Returns saturated 5 minutes later: starts from scratch.
	if _, err := pool.Exec(ctx, `UPDATE nodes SET status = 'ONLINE', last_seen_at = now() WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark online: %v", err)
	}
	clk.advance(5 * time.Minute)
	w.evaluateDiskIOSaturated(ctx)
	if got := openCount(t, pool, nodeID); got != 0 {
		t.Fatalf("return inherited old count and opened: %d", got)
	}
}

// The sequence is PER NODE: a Node that saturates later does not inherit the other's clock.
func TestEvaluateDiskIO_IndependentNodes(t *testing.T) {
	w, pool, nodeA := setupWorker(t, 95, "90s")
	clk := useFakeClock(w)
	ctx := context.Background()

	var clusterID, nodeB string
	if err := pool.QueryRow(ctx, `SELECT cluster_id::text FROM nodes WHERE id = $1`, nodeA).Scan(&clusterID); err != nil {
		t.Fatalf("read cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at, disk_io_util_pct)
		 VALUES ($1, 'disk-itest-node-b', 'ONLINE', now(), 40) RETURNING id::text`,
		clusterID).Scan(&nodeB); err != nil {
		t.Fatalf("create Node B: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alerts WHERE node_id = $1`, nodeB)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeB)
	})

	// A saturates from t=0; B only from t=60s.
	for _, at := range []int{0, 30, 60, 90} {
		if at == 60 {
			setUtil(t, pool, nodeB, 95)
		}
		w.evaluateDiskIOSaturated(ctx)
		clk.advance(30 * time.Second)
	}
	// At the t=90 pass: A completed 90s; B, only 30s.
	if got := openCount(t, pool, nodeA); got != 1 {
		t.Errorf("Node A (90s continuous) should have an alert, got %d", got)
	}
	if got := openCount(t, pool, nodeB); got != 0 {
		t.Errorf("Node B (30s) should not have an alert, got %d", got)
	}
}

// Turning off the rule (threshold 0) discards the count: turning it back on starts from scratch.
func TestEvaluateDiskIO_ShutdownDiscardsCount(t *testing.T) {
	w, _, nodeID := setupWorker(t, 95, "90s")
	useFakeClock(w)
	ctx := context.Background()

	w.evaluateDiskIOSaturated(ctx)
	if !inCount(w, nodeID) {
		t.Fatal("saturated Node should be in count")
	}
	w.cfg = settings.NewForTest(map[string]string{"alerting.disk_saturated_pct": "0"})
	w.evaluateDiskIOSaturated(ctx)
	if inCount(w, nodeID) {
		t.Fatal("disabled rule should clear count")
	}
}
