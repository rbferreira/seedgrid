package migration_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// Tests for #87: migrate_pull's DONE no longer means "the destination is verified".
// The reconciler completes, aborts or waits, watching the torrent on the destination Node.

// fakeClock is the watchdog clock: the recheck measures minutes and the test cannot sleep.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// hash87 generates infohashes specific to these tests (the database is shared and `add` refuses
// duplicate hash in the Cluster).
func hash87(n int) string { return fmt.Sprintf("87%038d", n) }

// verifyHarness prepares a harness with the fake clock and the watchdog with known limits
// (stagnation 10 min, ceiling 1 h) — independent of production defaults.
func verifyHarness(t *testing.T) (harness, *fakeClock) {
	t.Helper()
	h := newHarness(t)
	clk := &fakeClock{t: time.Now()}
	h.mig.SetClock(clk.now)
	h.mig.SetRecheckWatchdog(
		func() time.Duration { return 10 * time.Minute },
		func() time.Duration { return time.Hour })
	return h, clk
}

// pullDone marks the pull as DONE with the end ONE MINUTE in the past — enough so that a
// sync "now" is later than the DONE (only a later read counts as an observation).
func (h harness) pullDone(t *testing.T, migID string) {
	t.Helper()
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE commands SET status='DONE', finished_at = now() - interval '1 minute' WHERE id = $1`,
		h.pullCmdID(t, migID)); err != nil {
		t.Fatalf("mark pull DONE: %v", err)
	}
}

// destReport simulates the inventory that the destination Agent sends to the CP (the real path:
// SyncStates adopts the hash row on the Node as soon as the Engine has it).
func (h harness) destReport(t *testing.T, dest, hash, status string, progress float64) {
	t.Helper()
	h.destReports(t, dest, torrent.Report{Hash: hash, Name: "destination", Status: status, Progress: &progress})
}

func (h harness) destReports(t *testing.T, dest string, reports ...torrent.Report) {
	t.Helper()
	if err := h.tor.SyncStates(h.ctx, dest, reports); err != nil {
		t.Fatalf("SyncStates: %v", err)
	}
}

func (h harness) liveRows(t *testing.T, nodeID, hash string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM torrents WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL`,
		nodeID, hash).Scan(&n); err != nil {
		t.Fatalf("liveRows: %v", err)
	}
	return n
}

func (h harness) migError(t *testing.T, migID string) string {
	t.Helper()
	var e *string
	if err := h.pool.QueryRow(h.ctx, `SELECT error FROM torrent_migrations WHERE id = $1`, migID).Scan(&e); err != nil {
		t.Fatalf("migError: %v", err)
	}
	if e == nil {
		return ""
	}
	return *e
}

// reconcile runs one pass and requires that it not fail.
func (h harness) reconcile(t *testing.T) {
	t.Helper()
	if err := h.mig.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
}

// seedTorrent creates a torrent with the .torrent persisted and returns it with the destination Node.
func (h harness) seedTorrent(t *testing.T, n int, name string) (torrent.Torrent, string) {
	t.Helper()
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:"+hash87(n)+"&dn="+name)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	return tr, h.other(tr.NodeID)
}

// endsAtEnd tears down in-flight torrent migrations at the end of the test: the database is
// shared and a forgotten PENDING migration would count against the concurrency ceiling and the
// admission of the following tests.
func (h harness) endsAtEnd(t *testing.T, torrentIDs ...string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = h.pool.Exec(h.ctx,
			`UPDATE torrent_migrations SET status = 'FAILED', error = 'encerrada pelo teste'
			  WHERE torrent_id = ANY($1::uuid[]) AND status IN ('QUEUED','PENDING')`, torrentIDs)
	})
}

// The pull only promises "copy done, recheck triggered": the CP cannot complete or remove
// the origin until the destination proves it. It is the heart of #87 — with the new Agent the DONE
// arrives before verification.
func TestVerify_PullDoneAloneDoesNotComplete(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 1, "Verify1")
	source := tr.NodeID
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.pullDone(t, m.ID)

	// 1) No read from the destination yet: wait.
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "PENDING" {
		t.Fatalf("without destination read: status = %s, want PENDING", got)
	}

	// 2) The destination appears in recheck, halfway: it keeps waiting.
	h.destReport(t, dest, *tr.Hash, "CHECKING", 0.4)
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "PENDING" {
		t.Fatalf("destination in recheck: status = %s, want PENDING", got)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 0 {
		t.Fatalf("remove_torrent on source = %d, want 0 — the source only leaves after proof", got)
	}

	// 3) The destination proves it (SEEDING, 100%): completes, and only now removes the origin.
	h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "COMPLETED" {
		t.Fatalf("destination verified: status = %s (%s), want COMPLETED", got, h.migError(t, m.ID))
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 1 {
		t.Errorf("remove_torrent at source = %d, want 1", got)
	}
	if got := h.liveRows(t, dest, *tr.Hash); got != 1 {
		t.Errorf("live rows at destination = %d, want 1", got)
	}

	// Subsequent passes requeue nothing (the invariant from the 2026-07-18 incident).
	h.reconcile(t)
	h.reconcile(t)
	if got := h.removeCountForHash(t, *tr.Hash); got != 1 {
		t.Errorf("remove_torrent after repeated passes = %d, want 1", got)
	}
}

// During recheck qBittorrent reports the VERIFICATION advance as progress. A
// partial at 60% whose recheck goes past 60% has not finished verifying — completing there
// would remove the origin without proof. Only progress OUTSIDE CHECKING counts.
func TestVerify_RecheckProgressDoesNotCountAsFloor(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 2, "Verify2")
	if _, err := h.pool.Exec(h.ctx, `UPDATE torrents SET progress = 0.6 WHERE id = $1`, tr.ID); err != nil {
		t.Fatalf("source progress: %v", err)
	}
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.pullDone(t, m.ID)

	h.destReport(t, dest, *tr.Hash, "CHECKING", 0.9) // above 0,6, but still verifying
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "PENDING" {
		t.Fatalf("CHECKING at 0.9 for a 0.6 partial: status = %s, want PENDING", got)
	}

	h.destReport(t, dest, *tr.Hash, "DOWNLOADING", 0.6) // checked and continues downloading the rest
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "COMPLETED" {
		t.Fatalf("partial verified: status = %s (%s), want COMPLETED", got, h.migError(t, m.ID))
	}
}

// OLD Agent: it only responds DONE after the recheck, so at the moment of the DONE the destination is
// already verified. The CP completes anyway — by the torrent state, without knowing which Agent it was —,
// as soon as a read later than the DONE arrives.
func TestVerify_OldAgentDestinationAlreadyVerifiedCompletes(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 3, "Verify3")
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)

	// The destination was synchronized SEEDING during the old Agent's recheck...
	h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
	// ...and the DONE arrives later. That read is earlier than the DONE: it does not count yet.
	h.pullDone(t, m.ID)
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE torrents SET last_synced_at = now() - interval '5 minutes' WHERE node_id = $1 AND hash = $2`,
		dest, *tr.Hash); err != nil {
		t.Fatalf("age the read: %v", err)
	}
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "PENDING" {
		t.Fatalf("read before DONE: status = %s, want PENDING", got)
	}

	// Next destination sync (≤15 s later): new read → completes.
	h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "COMPLETED" {
		t.Fatalf("status = %s (%s), want COMPLETED", got, h.migError(t, m.ID))
	}
}

// Stuck recheck: the progress stops rising. The CP aborts as #79 did in the Agent —
// same message, same effect: the torrent remains on the origin, which is resumed, and the
// destination copy is discarded.
func TestVerify_StuckRecheckFailsNoLoss(t *testing.T) {
	h, clk := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 4, "Verify4")
	source := tr.NodeID
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.pullDone(t, m.ID)

	h.destReport(t, dest, *tr.Hash, "CHECKING", 0.2)
	h.reconcile(t) // records the advance (0 → 0,2)
	clk.advance(9 * time.Minute)
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "PENDING" {
		t.Fatalf("9 min stalled (window of 10): status = %s, want PENDING", got)
	}

	clk.advance(2 * time.Minute) // 11 min without advance
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Fatalf("11 min stalled: status = %s, want FAILED", got)
	}
	if msg := h.migError(t, m.ID); !strings.Contains(msg, "destination recheck stalled") {
		t.Errorf("error = %q, want the stuck recheck diagnosis (#79)", msg)
	}
	// No loss: the origin keeps the torrent, is resumed, and the destination is cleaned.
	if got := h.torrentNodeID(t, tr.ID); got != source {
		t.Errorf("node_id = %s, want source %s", got, source)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 0 {
		t.Errorf("remove_torrent at SOURCE = %d, want 0", got)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest); got != 1 {
		t.Errorf("remove_torrent on destination = %d, want 1 (stuck recheck copy)", got)
	}
	if got := h.cmdCountForHash(t, "resume_torrent", *tr.Hash); got != 1 {
		t.Errorf("resume_torrent = %d, want 1", got)
	}
}

// A slow but MOVING recheck cannot fail (the case of the USB HD on node-03, #79): the
// stagnation window restarts on every advance. The total here (40 min) exceeds 4× the window.
func TestVerify_SlowRecheckProgressingDoesNotFail(t *testing.T) {
	h, clk := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 5, "Verify5")
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.pullDone(t, m.ID)

	for i := 1; i <= 5; i++ {
		h.destReport(t, dest, *tr.Hash, "CHECKING", 0.15*float64(i))
		h.reconcile(t)
		if got := h.migStatus(t, m.ID); got != "PENDING" {
			t.Fatalf("step %d (advancing): status = %s (%s), want PENDING", i, got, h.migError(t, m.ID))
		}
		clk.advance(8 * time.Minute) // each step fits in the window; the sum does not
	}
	h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "COMPLETED" {
		t.Fatalf("status = %s (%s), want COMPLETED", got, h.migError(t, m.ID))
	}
}

// Absolute ceiling: progress that always rises but never arrives must not occupy the
// migration slot (migration.max_concurrent) forever. Counted from the end of the pull.
func TestVerify_WaitCeilingFails(t *testing.T) {
	h, clk := verifyHarness(t) // ceiling of 1 h
	tr, dest := h.seedTorrent(t, 6, "Verify6")
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.pullDone(t, m.ID)

	for i := 1; i <= 14; i++ { // 70 min in steps of 5
		h.destReport(t, dest, *tr.Hash, "CHECKING", 0.05+0.01*float64(i))
		h.reconcile(t)
		if h.migStatus(t, m.ID) == "FAILED" {
			break
		}
		clk.advance(5 * time.Minute)
	}
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Fatalf("status = %s, want FAILED due to the 1 h ceiling", got)
	}
	msg := h.migError(t, m.ID)
	if !strings.Contains(msg, "did not reach source progress") || strings.Contains(msg, "stalled") {
		t.Errorf("error = %q, want the CEILING one (progress was advancing, not stuck)", msg)
	}
}

// The destination never appears in the inventory (Agent crashed after the DONE, broken sync): the
// absence of reads counts as stagnation, not as "it will arrive".
func TestVerify_DestinationNoReadFailsByStagnation(t *testing.T) {
	h, clk := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 7, "Verify7")
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.pullDone(t, m.ID)

	h.reconcile(t)
	clk.advance(11 * time.Minute)
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Fatalf("status = %s, want FAILED", got)
	}
	if msg := h.migError(t, m.ID); !strings.Contains(msg, "unread") {
		t.Errorf("error = %q, want to indicate no read from destination", msg)
	}
}

// Restarting the CP in the middle of a verification (the watchdog state is in memory) cannot
// take down a healthy migration: the 1st time the process sees it opens a new window. Here
// the pull DONE was 2 h ago — within the 6 h ceiling of the defaults — and the destination still advances.
func TestVerify_FirstSightAfterRestartRenewsWindow(t *testing.T) {
	h := newHarness(t)
	clk := &fakeClock{t: time.Now()}
	h.mig.SetClock(clk.now) // without SetRecheckWatchdog: the defaults apply (10 min / 6 h)
	tr, dest := h.seedTorrent(t, 8, "Verify8")
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE commands SET status='DONE', finished_at = now() - interval '2 hours' WHERE id = $1`,
		h.pullCmdID(t, m.ID)); err != nil {
		t.Fatalf("mark pull DONE: %v", err)
	}
	h.destReport(t, dest, *tr.Hash, "CHECKING", 0.5)

	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "PENDING" {
		t.Fatalf("1st pass after restart: status = %s (%s), want PENDING", got, h.migError(t, m.ID))
	}
	clk.advance(11 * time.Minute) // now yes: 11 min without advance
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Fatalf("status = %s, want FAILED due to stall", got)
	}
}

// A migration waiting for the recheck does not hold the others: the reconciler continues (the lesson of the
// 2026-07-18 incident, now applied to verification).
func TestVerify_MigrationInRecheckDoesNotBlockOthers(t *testing.T) {
	h, _ := verifyHarness(t)
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE torrent_migrations SET status = 'FAILED', error = 'encerrada pelo setup do teste'
		  WHERE status IN ('QUEUED', 'PENDING')`); err != nil {
		t.Fatalf("clear in-flight migrations: %v", err)
	}
	source, dest := h.nodeA, h.nodeB
	var ids []string
	var migs []migration.Migration
	for _, n := range []int{9, 10} {
		tr := h.addTorrent(t, "magnet:?xt=urn:btih:"+hash87(n)+"&dn=Par")
		if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
			t.Fatalf("StoreTorrentFile: %v", err)
		}
		if _, err := h.pool.Exec(h.ctx, `UPDATE torrents SET node_id = $2 WHERE id = $1`, tr.ID, source); err != nil {
			t.Fatalf("fix source: %v", err)
		}
		m, err := h.mig.Migrate(h.ctx, tr.ID, dest)
		if err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		ids = append(ids, tr.ID)
		migs = append(migs, m)
	}
	h.endsAtEnd(t, ids...)
	if err := h.mig.ReconcileAndPromote(h.ctx, 10); err != nil {
		t.Fatalf("promote: %v", err)
	}
	for _, m := range migs {
		h.pullDone(t, m.ID)
	}

	p0, p1 := 0.3, 1.0
	h.destReports(t, dest,
		torrent.Report{Hash: hash87(9), Name: "slow", Status: "CHECKING", Progress: &p0},
		torrent.Report{Hash: hash87(10), Name: "ready", Status: "SEEDING", Progress: &p1})
	h.reconcile(t)

	if got := h.migStatus(t, migs[0].ID); got != "PENDING" {
		t.Errorf("a in recheck: status = %s, want PENDING", got)
	}
	if got := h.migStatus(t, migs[1].ID); got != "COMPLETED" {
		t.Errorf("the verified one: status = %s (%s), want COMPLETED — waiting for the other cannot hold it", got, h.migError(t, migs[1].ID))
	}
}

// The CP tells the Agent that it is the one watching the recheck (async_recheck). That is what allows
// the new Agent to return DONE early WITHOUT an old CP (which treats DONE as "verified")
// ever seeing a premature DONE: the notice only exists in this CP.
func TestVerify_PullAnnouncesAsyncRecheck(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 11, "Verify11")
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)

	var flag *string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT c.payload->>'async_recheck' FROM torrent_migrations m JOIN commands c ON c.id = m.pull_command_id WHERE m.id = $1`,
		m.ID).Scan(&flag); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if flag == nil || *flag != "true" {
		t.Errorf("async_recheck in migrate_pull payload = %v, want true", flag)
	}
}

// Dispute over the SAME torrent (#87): without the Poller lock, a pause/resume/recheck/remove
// that reached the destination during the recheck would be executed IN THE MIDDLE of it. The CP refuses the
// action — on the migrated torrent (origin) and on the copy that the destination adopted — while the
// migration is in flight; other torrents, and the same torrent outside the window, remain free.
func TestDuringMigration_OperatorActionsRejected(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 12, "Verify12")
	other, _ := h.seedTorrent(t, 13, "Free")
	h.endsAtEnd(t, tr.ID, other.ID)

	// QUEUED migration (still in the queue, nothing moved): the torrent remains free.
	m, err := h.mig.Migrate(h.ctx, tr.ID, dest)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := h.tor.Command(h.ctx, tr.ID, "pause"); err != nil {
		t.Fatalf("pause with migration only QUEUED: %v, want allowed", err)
	}

	// Promoted (PENDING) and with the destination already in recheck: the adopted copy exists.
	if err := h.mig.ReconcileAndPromote(h.ctx, 10); err != nil {
		t.Fatalf("promote: %v", err)
	}
	h.pullDone(t, m.ID)
	h.destReport(t, dest, *tr.Hash, "CHECKING", 0.3)
	var copyID string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT id::text FROM torrents WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL`,
		dest, *tr.Hash).Scan(&copyID); err != nil {
		t.Fatalf("find adopted copy at destination: %v", err)
	}

	for _, action := range []string{"pause", "resume", "recheck"} {
		_, err := h.tor.Command(h.ctx, tr.ID, action)
		if !errors.Is(err, torrent.ErrMigrationInFlight) {
			t.Errorf("%s on migrated torrent = %v, want ErrMigrationInFlight", action, err)
		} else if !strings.Contains(err.Error(), "/api/v1/migrations/"+m.ID+"/cancel") {
			// The refusal cannot leave the operator with no way out: it says that it can be canceled, and which one.
			t.Errorf("%s: message does not indicate cancellation of migration %s: %q", action, m.ID, err)
		}
		if _, err := h.tor.Command(h.ctx, copyID, action); !errors.Is(err, torrent.ErrMigrationInFlight) {
			t.Errorf("%s on destination copy = %v, want ErrMigrationInFlight", action, err)
		}
	}
	if _, err := h.tor.Remove(h.ctx, tr.ID, "test"); !errors.Is(err, torrent.ErrMigrationInFlight) {
		t.Errorf("remove on migrated torrent = %v, want ErrMigrationInFlight", err)
	}
	if _, err := h.tor.Remove(h.ctx, copyID, "test"); !errors.Is(err, torrent.ErrMigrationInFlight) {
		t.Errorf("remove on destination copy = %v, want ErrMigrationInFlight", err)
	}
	// ...and the refusal left no trace: the remove did not soft-delete or enqueue anything.
	if got := h.liveRows(t, dest, *tr.Hash); got != 1 {
		t.Errorf("live rows at destination = %d, want 1 (copy must not have been removed)", got)
	}

	// Migrating the copy back while the outbound is in flight would make two migrations dispute the
	// same content in opposite directions.
	if err := h.tor.StoreTorrentFile(h.ctx, copyID, sampleTorrent(), "engine_export"); err != nil {
		t.Fatalf("StoreTorrentFile for copy: %v", err)
	}
	if _, err := h.mig.Migrate(h.ctx, copyID, tr.NodeID); !errors.Is(err, migration.ErrAlreadyMigrating) {
		t.Errorf("Migrate copy back = %v, want ErrAlreadyMigrating", err)
	}

	// Another torrent, same Nodes: free.
	if _, err := h.tor.Command(h.ctx, other.ID, "pause"); err != nil {
		t.Errorf("pause on OTHER torrent = %v, want allowed", err)
	}

	// Once the migration is finished (here, failed), the torrent accepts actions again.
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE commands SET status='FAILED', error='terminated by test' WHERE id = $1`, h.pullCmdID(t, m.ID)); err != nil {
		t.Fatalf("fail the pull: %v", err)
	}
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Fatalf("status = %s, want FAILED", got)
	}
	if _, err := h.tor.Command(h.ctx, tr.ID, "resume"); err != nil {
		t.Errorf("resume after migration finishes = %v, want allowed", err)
	}
}
