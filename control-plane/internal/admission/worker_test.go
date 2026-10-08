package admission

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// fakeCommander records the enqueued commands (without touching a real Agent).
type fakeCommander struct {
	mu    sync.Mutex
	calls []cmdCall
}

type cmdCall struct{ nodeID, cmdType, hash string }

func (f *fakeCommander) Enqueue(_ context.Context, nodeID, cmdType string, payload json.RawMessage) (string, error) {
	var p map[string]string
	_ = json.Unmarshal(payload, &p)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, cmdCall{nodeID, cmdType, p["hash"]})
	return "cmd-id", nil
}

func (f *fakeCommander) count(cmdType string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.cmdType == cmdType {
			n++
		}
	}
	return n
}

// fakeSlots returns a fixed L (ok=false simulates "no observation").
type fakeSlots struct {
	limit int
	ok    bool
}

func (f fakeSlots) ObservedDLSlotLimit(context.Context, time.Duration) (int, bool) {
	return f.limit, f.ok
}

func setup(t *testing.T, slots fakeSlots, fallback int) (*Worker, *fakeCommander, *pgxpool.Pool, string) {
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
		`INSERT INTO clusters (name, description) VALUES ('adm-itest', 'admission test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at)
		 VALUES ($1, 'adm-itest-node', 'ONLINE', now()) RETURNING id::text`, clusterID).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM torrent_stat_samples WHERE torrent_id IN (SELECT id FROM torrents WHERE node_id = $1)`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM torrents WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cmds := &fakeCommander{}
	// Rate-based (#22) and dead-based rotation are off by default (min_dl_rate=0, dead_grace=0)
	// — ceiling tests must not be interfered with; TestRotateSlow/TestRotateDead
	// enable the tunables via w.cfg (white-box). In-memory store (no DB), ADR-023.
	cfg := settings.NewForTest(map[string]string{
		"admission.interval":              "1s",
		"admission.max_cluster_downloads": strconv.Itoa(fallback),
		"admission.min_dl_rate":           "0",
		"admission.dead_grace":            "0",
	})
	w := NewWorker(pool, slots, cmds, logger, cfg)
	return w, cmds, pool, nodeID
}

// setSwarm sets the SWARM peers (tracker scrape) for a torrent.
func setSwarm(t *testing.T, pool *pgxpool.Pool, id string, seeds, leechers int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE torrents SET num_seeds = $2, num_leechers = $3 WHERE id = $1`, id, seeds, leechers); err != nil {
		t.Fatalf("set swarm for %s: %v", id, err)
	}
}

// setConnected sets the CONNECTED peers for a torrent (refinement of ADR-021).
func setConnected(t *testing.T, pool *pgxpool.Pool, id string, seeds, leechers int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE torrents SET num_seeds_connected = $2, num_leechers_connected = $3 WHERE id = $1`,
		id, seeds, leechers); err != nil {
		t.Fatalf("set connected for %s: %v", id, err)
	}
}

// insertTorrent creates a test torrent and returns its id.
func insertTorrent(t *testing.T, pool *pgxpool.Pool, nodeID, hash, status string, progress float64, hold bool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO torrents (node_id, hash, name, status, progress, admission_paused_at)
		 VALUES ($1, $2, $2, $3, $4, CASE WHEN $5 THEN now() ELSE NULL END) RETURNING id::text`,
		nodeID, hash, status, progress, hold).Scan(&id); err != nil {
		t.Fatalf("insert torrent %s: %v", hash, err)
	}
	return id
}

func isHeld(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var held bool
	if err := pool.QueryRow(context.Background(),
		`SELECT admission_paused_at IS NOT NULL FROM torrents WHERE id = $1`, id).Scan(&held); err != nil {
		t.Fatalf("read hold for %s: %v", id, err)
	}
	return held
}

// clusterOf returns the cluster of a Node.
func clusterOf(t *testing.T, pool *pgxpool.Pool, nodeID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`SELECT cluster_id::text FROM nodes WHERE id = $1`, nodeID).Scan(&id); err != nil {
		t.Fatalf("node cluster: %v", err)
	}
	return id
}

// setGate sets the global gate (full_auto/flex) for the cluster (ADR-024).
func setGate(t *testing.T, pool *pgxpool.Pool, clusterID, mode string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE clusters SET operation_mode = $2 WHERE id = $1`, clusterID, mode); err != nil {
		t.Fatalf("set gate %s: %v", mode, err)
	}
}

// setNodeMode sets the per-Node mode (sg/manual) (ADR-024).
func setNodeMode(t *testing.T, pool *pgxpool.Pool, nodeID, mode string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE nodes SET operation_mode = $2 WHERE id = $1`, nodeID, mode); err != nil {
		t.Fatalf("set node mode %s: %v", mode, err)
	}
}

// addNode creates a second Node in the cluster with a given mode and schedules its cleanup.
func addNode(t *testing.T, pool *pgxpool.Pool, clusterID, hostname, mode string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at, operation_mode)
		 VALUES ($1, $2, 'ONLINE', now(), $3) RETURNING id::text`, clusterID, hostname, mode).Scan(&id); err != nil {
		t.Fatalf("create node %s: %v", hostname, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM torrents WHERE node_id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM nodes WHERE id = $1`, id)
	})
	return id
}

// setPriority sets/unsets manual prioritization (F3) for a torrent.
func setPriority(t *testing.T, pool *pgxpool.Pool, id string, on bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE torrents SET prioritized_at = CASE WHEN $2 THEN now() ELSE NULL END WHERE id = $1`, id, on); err != nil {
		t.Fatalf("set priority de %s: %v", id, err)
	}
}

// isPrioritized reads whether the torrent is marked as prioritized (F3).
func isPrioritized(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var p bool
	if err := pool.QueryRow(context.Background(),
		`SELECT prioritized_at IS NOT NULL FROM torrents WHERE id = $1`, id).Scan(&p); err != nil {
		t.Fatalf("read priority for %s: %v", id, err)
	}
	return p
}

// TestManualNodeUntouched (ADR-024): under Flex, a Manual Node is sovereign to the operator —
// the controller does not hold the excess nor re-pause (drift) torrents on this Node, even
// if the cluster is above L. This fixes the asymmetry: manual resume is not undone.
func TestManualNodeUntouched(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	setGate(t, pool, clusterOf(t, pool, nodeID), "flex")
	setNodeMode(t, pool, nodeID, "manual")

	a1 := insertTorrent(t, pool, nodeID, "mm01", "DOWNLOADING", 0.10, false)
	a2 := insertTorrent(t, pool, nodeID, "mm02", "DOWNLOADING", 0.20, false)
	// Pre-existing hold that resumed running (operator's manual resume on a Manual Node):
	// reconcileDrift MUST NOT re-pause it.
	drift := insertTorrent(t, pool, nodeID, "mm03", "DOWNLOADING", 0.30, true)

	w.Evaluate(context.Background())

	if isHeld(t, pool, a1) || isHeld(t, pool, a2) {
		t.Error("Node Manual: controller should not hold anything (sovereign operator)")
	}
	if got := cmds.count("pause_torrent"); got != 0 {
		t.Errorf("Node Manual: no pause should be enqueued; got %d", got)
	}
	// The drift marker remains (clearing it is the operator's resume role in the API, not
	// the worker's) — what matters is that the worker did not re-enqueue the pause.
	if !isHeld(t, pool, drift) {
		t.Error("Node Manual: worker should not touch the pre-existing hold")
	}
}

// TestManualCountsTowardL (ADR-024): downloads on a Manual Node COUNT towards the L ceiling.
// With the single slot (L=1) occupied by an active on a Manual Node, a viable hold on an
// SG Node is NOT promoted — otherwise the cluster would exceed L.
func TestManualCountsTowardL(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	clusterID := clusterOf(t, pool, nodeID)
	setGate(t, pool, clusterID, "flex")
	setNodeMode(t, pool, nodeID, "sg") // the setup node is SG
	manualNode := addNode(t, pool, clusterID, "adm-itest-manual", "manual")

	insertTorrent(t, pool, manualNode, "nn01", "DOWNLOADING", 0.40, false) // active in Manual (counts for L)
	sgHold := insertTorrent(t, pool, nodeID, "nn02", "PAUSED", 0.60, true) // viable hold on SG

	w.Evaluate(context.Background())

	if !isHeld(t, pool, sgHold) {
		t.Error("Manual counts toward L: slot occupied by Node Manual → Node SG hold must not be promoted")
	}
	if got := cmds.count("resume_torrent"); got != 0 {
		t.Errorf("no resume expected (ceiling reached via Manual Node); got %d", got)
	}
}

// TestHoldExcess: with L=2 and 4 non-stopped, the 2 with LOWEST progress go into hold.
func TestHoldExcess(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 2, ok: true}, 0)
	lo1 := insertTorrent(t, pool, nodeID, "aa01", "DOWNLOADING", 0.10, false)
	lo2 := insertTorrent(t, pool, nodeID, "aa02", "QUEUED", 0.20, false)
	hi1 := insertTorrent(t, pool, nodeID, "aa03", "DOWNLOADING", 0.80, false)
	hi2 := insertTorrent(t, pool, nodeID, "aa04", "DOWNLOADING", 0.90, false)

	w.Evaluate(context.Background())

	if !isHeld(t, pool, lo1) || !isHeld(t, pool, lo2) {
		t.Error("the two with lowest progress should be on hold")
	}
	if isHeld(t, pool, hi1) || isHeld(t, pool, hi2) {
		t.Error("highest-progress ones should NOT be on hold")
	}
	if got := cmds.count("pause_torrent"); got != 2 {
		t.Errorf("pause_torrent enqueued = %d, want 2", got)
	}
}

// TestPromote: with L=3 and only 1 active + 2 holds, the 2 holds (highest progress first)
// are promoted (resume + hold cleared).
func TestPromote(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 3, ok: true}, 0)
	insertTorrent(t, pool, nodeID, "bb01", "DOWNLOADING", 0.50, false) // 1 active
	h1 := insertTorrent(t, pool, nodeID, "bb02", "PAUSED", 0.30, true) // holds
	h2 := insertTorrent(t, pool, nodeID, "bb03", "PAUSED", 0.60, true)

	w.Evaluate(context.Background())

	if isHeld(t, pool, h1) || isHeld(t, pool, h2) {
		t.Error("both holds should have been promoted (room for 2)")
	}
	if got := cmds.count("resume_torrent"); got != 2 {
		t.Errorf("resume_torrent enqueued = %d, want 2", got)
	}
}

// TestUserPauseAdoptedOnSGNode: on an SG Node the SYSTEM has priority (ADR-024) — a
// download paused by the operator is ADOPTED as a hold and returns to the queue; if there's a slot,
// it is promoted in the same cycle. This is the case that left the torrent out of orchestration
// forever (neither active nor promotable).
func TestUserPauseAdoptedOnSGNode(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 5, ok: true}, 0)
	userPaused := insertTorrent(t, pool, nodeID, "cc01", "PAUSED", 0.40, false) // user-pause: hold=false

	w.Evaluate(context.Background())

	// Adopted and, with L=5 and no active, promoted next (hold cleared + resume).
	if isHeld(t, pool, userPaused) {
		t.Error("adopted pause should have been promoted (slot available)")
	}
	if got := cmds.count("resume_torrent"); got != 1 {
		t.Errorf("resume_torrent = %d, want 1 (manual pause adopted and promoted)", got)
	}
}

// insertMigration puts a migration in the given state for the torrent, with any destination
// Node in the same cluster (the destination doesn't matter for the "in-flight" predicate).
func insertMigration(t *testing.T, pool *pgxpool.Pool, torrentID, sourceNodeID, status string) {
	t.Helper()
	ctx := context.Background()
	var destID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status)
		 SELECT cluster_id, 'adm-mig-dest-' || substr(md5(random()::text),1,8), 'ONLINE'
		   FROM nodes WHERE id = $1 RETURNING id::text`, sourceNodeID).Scan(&destID); err != nil {
		t.Fatalf("create destination node: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO torrent_migrations (torrent_id, source_node_id, dest_node_id, status)
		 VALUES ($1, $2, $3, $4)`, torrentID, sourceNodeID, destID, status); err != nil {
		t.Fatalf("insert migration %s: %v", status, err)
	}
}

// Issue #8: the migration STOPS the torrent at the source before copying. To admission this
// looks like an orphaned manual pause — it would adopt and promote in the next cycle, resuming the
// download IN THE MIDDLE of the transfer (observed live on 07/08/2026). While the
// migration is in flight, the torrent is not theirs.
func TestMigratingTorrentNotAdoptedNorResumed(t *testing.T) {
	for _, status := range []string{"QUEUED", "PENDING"} {
		w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 5, ok: true}, 0)
		paused := insertTorrent(t, pool, nodeID, "mg0"+status[:1], "PAUSED", 0.40, false)
		insertMigration(t, pool, paused, nodeID, status)

		w.Evaluate(context.Background())

		if isHeld(t, pool, paused) {
			t.Errorf("%s: migrating torrent should not become an admission hold", status)
		}
		if got := cmds.count("resume_torrent"); got != 0 {
			t.Errorf("%s: resume_torrent = %d, want 0 (would resume download during copy)", status, got)
		}
	}
}

// Once the migration is finished (COMPLETED/FAILED), the torrent becomes a normal candidate again —
// the predicate cannot turn into a permanent exclusion.
func TestFinishedMigrationDoesNotBlockAdmission(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 5, ok: true}, 0)
	paused := insertTorrent(t, pool, nodeID, "mg9", "PAUSED", 0.40, false)
	insertMigration(t, pool, paused, nodeID, "FAILED")

	w.Evaluate(context.Background())

	if got := cmds.count("resume_torrent"); got != 1 {
		t.Errorf("resume_torrent = %d, want 1 (finished migration releases the torrent)", got)
	}
}

// TestUserPauseAdoptedStaysHeldWithoutSlot: adoption does NOT promote on its own — at the
// ceiling (L=1, already occupied) the adopted pause stays in hold, waiting its turn.
func TestUserPauseAdoptedStaysHeldWithoutSlot(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	insertTorrent(t, pool, nodeID, "cc10", "DOWNLOADING", 0.50, false) // occupies the single slot
	userPaused := insertTorrent(t, pool, nodeID, "cc11", "PAUSED", 0.40, false)

	w.Evaluate(context.Background())

	if !isHeld(t, pool, userPaused) {
		t.Error("with no free slot, the adopted pause should remain on hold")
	}
	if got := cmds.count("resume_torrent"); got != 0 {
		t.Errorf("no resume should leave at ceiling; got %d", got)
	}
}

// TestUserPauseUntouchedOnManualNode: on a Manual Node the OPERATOR is sovereign — the pause
// is neither adopted nor promoted (the other end of the ADR-024 priority rule).
func TestUserPauseUntouchedOnManualNode(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 5, ok: true}, 0)
	setGate(t, pool, clusterOf(t, pool, nodeID), "flex")
	setNodeMode(t, pool, nodeID, "manual")
	userPaused := insertTorrent(t, pool, nodeID, "cc02", "PAUSED", 0.40, false)

	w.Evaluate(context.Background())

	if isHeld(t, pool, userPaused) {
		t.Error("on Node Manual the operator pause should not be adopted")
	}
	if got := cmds.count("resume_torrent"); got != 0 {
		t.Errorf("no resume should be enqueued on Node Manual; got %d", got)
	}
}

// TestManualBackToSGReadopts: when switching back from Manual to SG, what was stopped manually is
// re-adopted — no hook on mode change, because adoption runs on the SG Node predicate
// every cycle. This is the "or the system returns to auto" part of the rule.
func TestManualBackToSGReadopts(t *testing.T) {
	w, _, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	clusterID := clusterOf(t, pool, nodeID)
	setGate(t, pool, clusterID, "flex")
	setNodeMode(t, pool, nodeID, "manual")
	insertTorrent(t, pool, nodeID, "cc20", "DOWNLOADING", 0.50, false) // occupies the slot, prevents promotion
	userPaused := insertTorrent(t, pool, nodeID, "cc21", "PAUSED", 0.40, false)

	w.Evaluate(context.Background())
	if isHeld(t, pool, userPaused) {
		t.Fatal("on Manual it should not have been adopted yet")
	}

	setNodeMode(t, pool, nodeID, "sg") // operator hands back the wheel
	w.Evaluate(context.Background())

	if !isHeld(t, pool, userPaused) {
		t.Error("upon switching back to SG, the pause should have been re-adopted as a hold")
	}
}

// TestCompletedPauseNotAdopted: PAUSED at 100% is a SEEDER that the operator stopped, not a
// queued download — adopting it would make the controller send resume to something it didn't hold.
func TestCompletedPauseNotAdopted(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 5, ok: true}, 0)
	done := insertTorrent(t, pool, nodeID, "cc30", "PAUSED", 1.0, false)

	w.Evaluate(context.Background())

	if isHeld(t, pool, done) {
		t.Error("paused seeder (progress=1) should not be adopted")
	}
	if got := cmds.count("resume_torrent"); got != 0 {
		t.Errorf("no resume should be issued for paused seeder; got %d", got)
	}
}

// TestNoLimitNoAction: with no observed L and no fallback, the controller takes no action.
func TestNoLimitNoAction(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{ok: false}, 0)
	a := insertTorrent(t, pool, nodeID, "dd01", "DOWNLOADING", 0.10, false)
	insertTorrent(t, pool, nodeID, "dd02", "DOWNLOADING", 0.20, false)

	w.Evaluate(context.Background())

	if isHeld(t, pool, a) {
		t.Error("with no known L, nothing should enter hold")
	}
	if got := cmds.count("pause_torrent"); got != 0 {
		t.Errorf("with no L, no pause should be enqueued; got %d", got)
	}
}

// TestFallbackLimit: with no observation but with a config fallback, the fallback applies.
func TestFallbackLimit(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{ok: false}, 1) // fallback L=1
	insertTorrent(t, pool, nodeID, "ee01", "DOWNLOADING", 0.90, false)
	lo := insertTorrent(t, pool, nodeID, "ee02", "DOWNLOADING", 0.10, false)

	w.Evaluate(context.Background())

	if !isHeld(t, pool, lo) {
		t.Error("with fallback L=1 and 2 active, the one with lowest progress should enter hold")
	}
	if got := cmds.count("pause_torrent"); got != 1 {
		t.Errorf("pause_torrent = %d, want 1", got)
	}
}

// TestRotateSlow: at the ceiling (L=1) with 1 slow active (dl_speed=0) and 1 hold with progress,
// after the grace the slow active is rotated (hold) and the hold is promoted. Before the grace,
// nothing happens (it just marks the clock).
func TestRotateSlow(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	// Turns on rotation (white-box) with a controlled clock.
	base := time.Unix(1_780_000_000, 0)
	clock := base
	w.now = func() time.Time { return clock }
	w.cfg = settings.NewForTest(map[string]string{
		"admission.max_cluster_downloads": "0",
		"admission.min_dl_rate":           "51200",
		"admission.slow_grace":            "5m",
		"admission.slow_cooldown":         "10m",
		"admission.dead_grace":            "0", // dead-based rotation OFF in this test
	})

	slow := insertTorrent(t, pool, nodeID, "ff01", "DOWNLOADING", 0.20, false) // dl_speed NULL→0 (slow)
	held := insertTorrent(t, pool, nodeID, "ff02", "PAUSED", 0.70, true)       // hold with progress

	// 1st pass: marks slowSince, but the grace hasn't passed yet → nothing rotated.
	w.Evaluate(context.Background())
	if isHeld(t, pool, slow) {
		t.Fatal("should not rotate before grace")
	}
	if cmds.count("pause_torrent") != 0 {
		t.Fatalf("no pause expected before grace; got %d", cmds.count("pause_torrent"))
	}

	// Advances past the grace and re-evaluates → rotates: slow becomes hold, hold is promoted.
	clock = base.Add(6 * time.Minute)
	w.Evaluate(context.Background())

	if !isHeld(t, pool, slow) {
		t.Error("the slow active should have been rotated (on hold)")
	}
	if isHeld(t, pool, held) {
		t.Error("the hold with progress should have been promoted")
	}
	if cmds.count("pause_torrent") != 1 || cmds.count("resume_torrent") != 1 {
		t.Errorf("expected 1 pause + 1 resume on rotation; got pause=%d resume=%d",
			cmds.count("pause_torrent"), cmds.count("resume_torrent"))
	}
	// The rotated one enters cooldown → is not re-promoted immediately.
	if _, ok := w.cooldown[slow]; !ok {
		t.Error("the rotated one should be on cooldown")
	}
}

// TestRotateDead: at the ceiling (L=1) with 1 DEAD active (0 seeds/0 peers) and two holds — a
// VIABLE one with lower progress (has seeds) and one with no peers with HIGHER progress — after the grace
// the dead one is rotated (hold + long cooldown) and the peer-aware promotion picks the VIABLE one
// (not the one with highest %). Before the grace, nothing happens.
func TestRotateDead(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	base := time.Unix(1_780_000_000, 0)
	clock := base
	w.now = func() time.Time { return clock }
	w.cfg = settings.NewForTest(map[string]string{
		"admission.max_cluster_downloads": "0",
		"admission.min_dl_rate":           "0", // rate-based rotation OFF in this test
		"admission.dead_grace":            "3m",
		"admission.dead_cooldown":         "30m",
	})

	dead := insertTorrent(t, pool, nodeID, "gg01", "DOWNLOADING", 0.50, false)  // 0 seeds/peers → dead
	viable := insertTorrent(t, pool, nodeID, "gg02", "PAUSED", 0.20, true)      // VIABLE hold (lower %)
	stalledHold := insertTorrent(t, pool, nodeID, "gg03", "PAUSED", 0.90, true) // hold with no peers (higher %)
	// Only the viable one has seeds in the swarm.
	if _, err := pool.Exec(context.Background(), `UPDATE torrents SET num_seeds = 4 WHERE id = $1`, viable); err != nil {
		t.Fatalf("set seeds for viable: %v", err)
	}

	// 1st pass: marks deadSince, grace hasn't passed yet → nothing rotated.
	w.Evaluate(context.Background())
	if isHeld(t, pool, dead) {
		t.Fatal("should not rotate the dead one before grace")
	}

	// Advances past the grace → rotates: dead becomes hold; promotes the VIABLE one (peer-aware).
	clock = base.Add(4 * time.Minute)
	w.Evaluate(context.Background())

	if !isHeld(t, pool, dead) {
		t.Error("the dead active (0 seeds/0 peers) should have been rotated (on hold)")
	}
	if isHeld(t, pool, viable) {
		t.Error("the VIABLE hold (has seeds) should have been promoted")
	}
	if !isHeld(t, pool, stalledHold) {
		t.Error("the hold without peers (higher %) should NOT be promoted — peer-aware prefers viable")
	}
	if until, ok := w.cooldown[dead]; !ok || !until.After(clock.Add(20*time.Minute)) {
		t.Error("the dead one should go to the back of the queue (long cooldown ~30min)")
	}
	if cmds.count("pause_torrent") != 1 || cmds.count("resume_torrent") != 1 {
		t.Errorf("expected 1 pause (dead) + 1 resume (viable); got pause=%d resume=%d",
			cmds.count("pause_torrent"), cmds.count("resume_torrent"))
	}
}

// TestRotateDeadGhostSeeder (refinement of ADR-021) — the case the SWARM criterion
// let slip: the tracker announces 1 seeder (scrape), but it never responds, so the
// Engine reports 0 CONNECTED peers. Judged by the swarm, the torrent looked alive and was never
// rotated; judged by connected, it is dead and yields the slot. Real case:
// Blade.Runner.2049 at 98,79% and 0 connections for 5 days, burning a slot every ~15 min.
func TestRotateDeadGhostSeeder(t *testing.T) {
	w, _, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	base := time.Unix(1_780_000_000, 0)
	clock := base
	w.now = func() time.Time { return clock }
	w.cfg = settings.NewForTest(map[string]string{
		"admission.max_cluster_downloads": "0",
		"admission.min_dl_rate":           "0", // rate-based rotation OFF: isolates the dead trigger
		"admission.dead_grace":            "3m",
		"admission.dead_cooldown":         "30m",
		"admission.dead_backoff_max":      "4h",
	})

	// Active with GHOST seeder: swarm says 1 seed, but 0 connected.
	ghost := insertTorrent(t, pool, nodeID, "ph01", "DOWNLOADING", 0.98, false)
	setSwarm(t, pool, ghost, 1, 1)
	setConnected(t, pool, ghost, 0, 0)
	// Viable hold to receive the slot (otherwise rotation doesn't act, by design).
	viable := insertTorrent(t, pool, nodeID, "ph02", "PAUSED", 0.10, true)
	setSwarm(t, pool, viable, 5, 2)

	w.Evaluate(context.Background()) // marks deadSince; before the grace nothing happens
	if isHeld(t, pool, ghost) {
		t.Fatal("should not rotate before grace")
	}

	clock = base.Add(4 * time.Minute)
	w.Evaluate(context.Background())

	if !isHeld(t, pool, ghost) {
		t.Error("active with ghost seeder (swarm>0, 0 connected) should be marked dead and rotated")
	}
	if isHeld(t, pool, viable) {
		t.Error("the viable hold should have been promoted to the freed slot")
	}
	if until, ok := w.cooldown[ghost]; !ok || !until.After(clock.Add(20*time.Minute)) {
		t.Error("the ghost should go to the back of the queue (long cooldown)")
	}
}

// TestRotateDeadRetrocompat — Agent prior to the field doesn't report connected (NULL
// columns). COALESCE must fall back to the swarm and preserve the pre-refinement behavior: with swarm
// > 0 and connected NULL, the torrent is NOT marked as dead (there is no evidence that it is).
func TestRotateDeadRetrocompat(t *testing.T) {
	w, _, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	base := time.Unix(1_780_000_000, 0)
	clock := base
	w.now = func() time.Time { return clock }
	w.cfg = settings.NewForTest(map[string]string{
		"admission.max_cluster_downloads": "0",
		"admission.min_dl_rate":           "0",
		"admission.dead_grace":            "3m",
		"admission.dead_cooldown":         "30m",
	})

	// Old Agent: swarm reported, connected stays NULL.
	legacy := insertTorrent(t, pool, nodeID, "lg01", "DOWNLOADING", 0.50, false)
	setSwarm(t, pool, legacy, 3, 1)
	viable := insertTorrent(t, pool, nodeID, "lg02", "PAUSED", 0.10, true)
	setSwarm(t, pool, viable, 5, 2)

	clock = base.Add(10 * time.Minute) // well past the grace
	w.Evaluate(context.Background())
	w.Evaluate(context.Background())

	if isHeld(t, pool, legacy) {
		t.Error("with connected NULL (legacy Agent) and swarm > 0, should not mark the torrent as dead")
	}
}

// TestDeadCooldownBackoff — the cooldown doubles on each CONSECUTIVE dead rotation of the same
// torrent, capped by the ceiling; connecting to a peer resets the count. This is what makes a chronic
// dead back off from the queue instead of returning every fixed cooldown.
func TestDeadCooldownBackoff(t *testing.T) {
	w, _, pool, _ := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	_ = pool
	w.cfg = settings.NewForTest(map[string]string{
		"admission.dead_cooldown":    "30m",
		"admission.dead_backoff_max": "2h",
	})

	for _, tc := range []struct {
		n    int
		want time.Duration
	}{
		{1, 30 * time.Minute},
		{2, time.Hour},
		{3, 2 * time.Hour},
		{4, 2 * time.Hour}, // cap
		{9, 2 * time.Hour}, // ceiling (doesn't overflow)
	} {
		if got := w.deadCooldownFor(tc.n); got != tc.want {
			t.Errorf("deadCooldownFor(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}

	// Backoff disabled (ceiling 0) → always the base cooldown.
	w.cfg = settings.NewForTest(map[string]string{
		"admission.dead_cooldown":    "30m",
		"admission.dead_backoff_max": "0",
	})
	if got := w.deadCooldownFor(5); got != 30*time.Minute {
		t.Errorf("with backoff disabled deadCooldownFor(5) = %v, want 30m", got)
	}
}

// TestPriorityJumpsQueue (F3): a PRIORITIZED hold jumps the queue — it is promoted before a
// non-prioritized viable one (with seeds) with higher progress, which the peer-aware order would pick.
func TestPriorityJumpsQueue(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	// L=1, none active, 2 holds. Without priority, the peer-aware would promote the viable one (pj02).
	prio := insertTorrent(t, pool, nodeID, "pj01", "PAUSED", 0.10, true) // low %, no seeds
	setPriority(t, pool, prio, true)
	viable := insertTorrent(t, pool, nodeID, "pj02", "PAUSED", 0.90, true) // high %, with seeds
	setSwarm(t, pool, viable, 5, 2)

	w.Evaluate(context.Background())

	if isHeld(t, pool, prio) {
		t.Error("the prioritized one should skip the queue and be promoted, even without seeds and with lower progress")
	}
	if !isHeld(t, pool, viable) {
		t.Error("with only 1 slot, the non-prioritized one should remain in hold")
	}
	if got := cmds.count("resume_torrent"); got != 1 {
		t.Errorf("expected 1 resume (only the prioritized one); got %d", got)
	}
}

// TestPriorityIgnoresCooldown (F3): a prioritized torrent is promoted even while in rotation
// cooldown — manual priority jumps ahead of the "end of the queue".
func TestPriorityIgnoresCooldown(t *testing.T) {
	w, _, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	base := time.Unix(1_780_000_000, 0)
	w.now = func() time.Time { return base }

	prio := insertTorrent(t, pool, nodeID, "pc01", "PAUSED", 0.10, true)
	setPriority(t, pool, prio, true)
	setSwarm(t, pool, prio, 3, 0)
	w.cooldown[prio] = base.Add(30 * time.Minute) // parked (end of the queue) — but it is prioritized

	w.Evaluate(context.Background())

	if isHeld(t, pool, prio) {
		t.Error("prioritized should be promoted even in cooldown (ignores end of queue)")
	}
}

// TestPriorityPersistentProtectsFromDead (F3): in persistent mode, a prioritized torrent
// that went dead (0 seeds/0 peers) is NOT rotated — it holds the slot until it downloads.
func TestPriorityPersistentProtectsFromDead(t *testing.T) {
	w, cmds, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	base := time.Unix(1_780_000_000, 0)
	clock := base
	w.now = func() time.Time { return clock }
	w.cfg = settings.NewForTest(map[string]string{
		"admission.max_cluster_downloads": "0",
		"admission.min_dl_rate":           "0",
		"admission.dead_grace":            "3m",
		"admission.dead_cooldown":         "30m",
		"admission.priority_persistent":   "true",
	})
	deadPrio := insertTorrent(t, pool, nodeID, "pp01", "DOWNLOADING", 0.50, false)
	setConnected(t, pool, deadPrio, 0, 0)
	setPriority(t, pool, deadPrio, true)
	viable := insertTorrent(t, pool, nodeID, "pp02", "PAUSED", 0.10, true)
	setSwarm(t, pool, viable, 5, 2)

	clock = base.Add(4 * time.Minute) // past grace
	w.Evaluate(context.Background())

	if isHeld(t, pool, deadPrio) {
		t.Error("persistent mode: dead prioritized should NOT be rotated")
	}
	if !isPrioritized(t, pool, deadPrio) {
		t.Error("persistent mode: priority should be preserved")
	}
	if !isHeld(t, pool, viable) {
		t.Error("slot remains occupied by protected prioritized → viable remains in hold")
	}
	if got := cmds.count("pause_torrent"); got != 0 {
		t.Errorf("no pause expected (prioritized protected); got %d", got)
	}
}

// TestPriorityNonPersistentDeadClears (F3): in standard mode (non-persistent), a prioritized
// dead torrent is rotated normally and, upon demotion, LOSES priority (it goes back to being
// a normal torrent).
func TestPriorityNonPersistentDeadClears(t *testing.T) {
	w, _, pool, nodeID := setup(t, fakeSlots{limit: 1, ok: true}, 0)
	base := time.Unix(1_780_000_000, 0)
	clock := base
	w.now = func() time.Time { return clock }
	w.cfg = settings.NewForTest(map[string]string{
		"admission.max_cluster_downloads": "0",
		"admission.min_dl_rate":           "0",
		"admission.dead_grace":            "3m",
		"admission.dead_cooldown":         "30m",
		"admission.priority_persistent":   "false",
	})
	deadPrio := insertTorrent(t, pool, nodeID, "np01", "DOWNLOADING", 0.50, false)
	setConnected(t, pool, deadPrio, 0, 0)
	setPriority(t, pool, deadPrio, true)
	viable := insertTorrent(t, pool, nodeID, "np02", "PAUSED", 0.10, true)
	setSwarm(t, pool, viable, 5, 2)

	w.Evaluate(context.Background()) // marks deadSince; grace hasn't passed yet
	clock = base.Add(4 * time.Minute)
	w.Evaluate(context.Background()) // past grace → rotates

	if !isHeld(t, pool, deadPrio) {
		t.Error("normal mode: dead prioritized should be rotated (in hold)")
	}
	if isPrioritized(t, pool, deadPrio) {
		t.Error("normal mode: upon demotion, the torrent should lose priority")
	}
	if isHeld(t, pool, viable) {
		t.Error("the viable one should have been promoted to the freed slot")
	}
}

// TestCompletedPriorityCleared (F3): a prioritized torrent that has completed (SEEDING/100%)
// is no longer prioritized — there is no longer a queue to jump.
func TestCompletedPriorityCleared(t *testing.T) {
	w, _, pool, nodeID := setup(t, fakeSlots{limit: 5, ok: true}, 0)
	seed := insertTorrent(t, pool, nodeID, "cp01", "SEEDING", 1.0, false)
	setPriority(t, pool, seed, true)

	w.Evaluate(context.Background())

	if isPrioritized(t, pool, seed) {
		t.Error("torrent that completed (SEEDING) should lose priority")
	}
}
