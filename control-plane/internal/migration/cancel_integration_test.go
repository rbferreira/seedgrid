package migration_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// Cancellation by the operator (#87): the same safe shutdown of the watchdog, in any
// phase of the migration.

// cmdStatusesOf returns the status of the serve and pull of the migration.
func (h harness) cmdStatusesOf(t *testing.T, migID string) (serve, pull string) {
	t.Helper()
	if err := h.pool.QueryRow(h.ctx,
		`SELECT s.status, p.status
		   FROM torrent_migrations m
		   JOIN commands s ON s.id = m.serve_command_id
		   JOIN commands p ON p.id = m.pull_command_id
		  WHERE m.id = $1`, migID).Scan(&serve, &pull); err != nil {
		t.Fatalf("migration command statuses: %v", err)
	}
	return serve, pull
}

func (h harness) setCmd(t *testing.T, cmdID, status string) {
	t.Helper()
	if _, err := h.pool.Exec(h.ctx, `UPDATE commands SET status = $2 WHERE id = $1`, cmdID, status); err != nil {
		t.Fatalf("setCmd: %v", err)
	}
}

func (h harness) serveCmdID(t *testing.T, migID string) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT serve_command_id::text FROM torrent_migrations WHERE id = $1`, migID).Scan(&id); err != nil {
		t.Fatalf("serveCmdID: %v", err)
	}
	return id
}

func (h harness) mustCancel(t *testing.T, migID string) migration.Migration {
	t.Helper()
	m, err := h.mig.Cancel(h.ctx, migID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if m.Status != "FAILED" || !m.Cancelled || m.Error == nil || *m.Error != migration.CancelReason {
		t.Fatalf("cancelled migration = %+v, want FAILED/cancelled with reason %q", m, migration.CancelReason)
	}
	return m
}

// Migration still in QUEUE: nothing queued or paused — canceling just drops it from the queue, and
// it is never promoted later.
func TestCancel_InQueueLeavesNoTrace(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 20, "Cancel20")
	m, err := h.mig.Migrate(h.ctx, tr.ID, dest)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	h.endsAtEnd(t, tr.ID)

	h.mustCancel(t, m.ID)

	if err := h.mig.ReconcileAndPromote(h.ctx, 10); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Errorf("queued cancelled migration was revived: status = %s", got)
	}
	for _, typ := range []string{"migrate_serve", "migrate_pull", "resume_torrent", "remove_torrent"} {
		if got := h.cmdCountForHash(t, typ, *tr.Hash); got != 0 {
			t.Errorf("%s queued = %d, want 0 — cancelled before any effect", typ, got)
		}
	}
	if got := h.liveRows(t, tr.NodeID, *tr.Hash); got != 1 {
		t.Errorf("live rows at source = %d, want 1", got)
	}
}

// Promoted but the Agent hasn't claimed serve/pull yet: they are taken down to never
// run (a pull forgotten in the queue would copy GBs in vain), and since nothing ran there's no origin
// to resume nor copy to discard — sending a remove to the destination would risk deleting a legitimate
// copy it already had.
func TestCancel_UndeliveredCommandsNeverRun(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 21, "Cancel21")
	source := tr.NodeID
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)

	h.mustCancel(t, m.ID)

	serve, pull := h.cmdStatusesOf(t, m.ID)
	if serve != "FAILED" || pull != "FAILED" {
		t.Errorf("serve=%s pull=%s, want both FAILED (downloaded before the Agent claims them)", serve, pull)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest); got != 0 {
		t.Errorf("remove_torrent at destination = %d, want 0 — pull never ran", got)
	}
	if got := h.cmdCountForHashOnNode(t, "resume_torrent", *tr.Hash, source); got != 0 {
		t.Errorf("resume_torrent at source = %d, want 0 — serve never paused anything", got)
	}
	if got := h.torrentNodeID(t, tr.ID); got != source {
		t.Errorf("node_id = %s, want source %s", got, source)
	}
}

// Cancellation DURING THE COPY: the destination Agent is in migrate_pull and the command can't
// be interrupted. The migration turns FAILED instantly, the origin is resumed, and the cleanup of the
// copy is queued behind the pull (serial Poller) — the copy finishes and is discarded. The
// in-progress pull is NOT touched.
func TestCancel_DuringCopy(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 22, "Cancel22")
	source := tr.NodeID
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	// The origin has already stopped and authorized (serve DONE); the destination is copying (pull DISPATCHED).
	h.setCmd(t, h.serveCmdID(t, m.ID), "DONE")
	h.setCmd(t, h.pullCmdID(t, m.ID), "DISPATCHED")

	h.mustCancel(t, m.ID)

	if _, pull := h.cmdStatusesOf(t, m.ID); pull != "DISPATCHED" {
		t.Errorf("pull = %s, want DISPATCHED: the in-progress command is not interrupted", pull)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest); got != 1 {
		t.Errorf("remove_torrent at destination = %d, want 1 (discards the copy as soon as pull finishes)", got)
	}
	var deleteFiles bool
	if err := h.pool.QueryRow(h.ctx,
		`SELECT (payload->>'delete_files')::bool FROM commands
		  WHERE type='remove_torrent' AND payload->>'hash' = $1 AND node_id = $2`,
		*tr.Hash, dest).Scan(&deleteFiles); err != nil || !deleteFiles {
		t.Errorf("cleanup must delete the data (delete_files=%v, err=%v)", deleteFiles, err)
	}
	if got := h.cmdCountForHashOnNode(t, "resume_torrent", *tr.Hash, source); got != 1 {
		t.Errorf("resume_torrent at source = %d, want 1", got)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 0 {
		t.Errorf("remove_torrent on SOURCE = %d, want 0 — it is the good copy", got)
	}

	// The pull finishes after cancellation (the Agent is unaware of the cancellation): the reconciler
	// cannot resurrect the migration or remove the origin.
	h.pullDone(t, m.ID)
	h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
	h.reconcile(t)
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Errorf("status after late pull = %s, want FAILED", got)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 0 {
		t.Errorf("remove_torrent on source after late pull = %d, want 0", got)
	}
}

// Cancellation during the recheck VERIFICATION: the case that motivates the endpoint (hours of
// waiting with torrent actions refused). It exits via the same path as the watchdog, and the
// torrent accepts actions again.
func TestCancel_DuringVerification(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 23, "Cancel23")
	source := tr.NodeID
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.setCmd(t, h.serveCmdID(t, m.ID), "DONE")
	h.pullDone(t, m.ID)
	h.destReport(t, dest, *tr.Hash, "CHECKING", 0.4)

	// Stuck: the action is refused, and the refusal points to the exit.
	_, err := h.tor.Command(h.ctx, tr.ID, "resume")
	var inFlight *torrent.MigrationInFlightError
	if !errors.As(err, &inFlight) || inFlight.MigrationID != m.ID {
		t.Fatalf("resume during migration = %v, want MigrationInFlightError for migration %s", err, m.ID)
	}

	h.mustCancel(t, m.ID)

	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest); got != 1 {
		t.Errorf("remove_torrent at destination = %d, want 1 (stops recheck and frees disk)", got)
	}
	if got := h.cmdCountForHashOnNode(t, "resume_torrent", *tr.Hash, source); got != 1 {
		t.Errorf("resume_torrent at source = %d, want 1", got)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 0 {
		t.Errorf("remove_torrent at SOURCE = %d, want 0", got)
	}
	// Free again.
	if _, err := h.tor.Command(h.ctx, tr.ID, "pause"); err != nil {
		t.Errorf("pause after cancellation = %v, want allowed", err)
	}
}

// Repeating the cancellation is safe: nothing is re-executed, and the response says why. Migration
// that finished via another path is also not cancelable, with a clear error.
func TestCancel_Idempotent(t *testing.T) {
	h, _ := verifyHarness(t)
	tr, dest := h.seedTorrent(t, 24, "Cancel24")
	m := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.setCmd(t, h.serveCmdID(t, m.ID), "DONE")
	h.setCmd(t, h.pullCmdID(t, m.ID), "DISPATCHED")

	h.mustCancel(t, m.ID)
	removes, resumes := h.cmdCountForHash(t, "remove_torrent", *tr.Hash), h.cmdCountForHash(t, "resume_torrent", *tr.Hash)

	for i := 0; i < 3; i++ {
		if _, err := h.mig.Cancel(h.ctx, m.ID); !errors.Is(err, migration.ErrMigrationAlreadyCancelled) {
			t.Fatalf("repeated cancellation (%d) = %v, want ErrMigrationAlreadyCancelled", i, err)
		}
	}
	if got := h.cmdCountForHash(t, "remove_torrent", *tr.Hash); got != removes {
		t.Errorf("remove_torrent after retry = %d, want %d (nothing reruns)", got, removes)
	}
	if got := h.cmdCountForHash(t, "resume_torrent", *tr.Hash); got != resumes {
		t.Errorf("resume_torrent after retry = %d, want %d", got, resumes)
	}
}

func TestCancel_MigrationFinishedOrNonexistent(t *testing.T) {
	h, _ := verifyHarness(t)

	// Completed.
	tr, dest := h.seedTorrent(t, 25, "Cancel25")
	done := h.start(t, tr.ID, dest)
	h.endsAtEnd(t, tr.ID)
	h.pullDone(t, done.ID)
	h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
	h.reconcile(t)
	if got := h.migStatus(t, done.ID); got != "COMPLETED" {
		t.Fatalf("precondition: status = %s, want COMPLETED", got)
	}
	if _, err := h.mig.Cancel(h.ctx, done.ID); !errors.Is(err, migration.ErrMigrationFinished) {
		t.Errorf("cancel completed migration = %v, want ErrMigrationFinished", err)
	}
	if got := h.migStatus(t, done.ID); got != "COMPLETED" {
		t.Errorf("cancel attempt changed the completed migration: %s", got)
	}

	// Failed on its own (it wasn't the operator).
	tr2, dest2 := h.seedTorrent(t, 26, "Cancel26")
	failed := h.start(t, tr2.ID, dest2)
	h.endsAtEnd(t, tr2.ID)
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE commands SET status='FAILED', error='recheck stuck' WHERE id = $1`, h.pullCmdID(t, failed.ID)); err != nil {
		t.Fatalf("fail the pull: %v", err)
	}
	h.reconcile(t)
	if _, err := h.mig.Cancel(h.ctx, failed.ID); !errors.Is(err, migration.ErrMigrationFinished) {
		t.Errorf("cancel migration that failed on its own = %v, want ErrMigrationFinished (not a cancellation)", err)
	}

	// Nonexistent and malformed id.
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid", ""} {
		if _, err := h.mig.Cancel(h.ctx, id); !errors.Is(err, migration.ErrMigrationNotFound) {
			t.Errorf("Cancel(%q) = %v, want ErrMigrationNotFound", id, err)
		}
	}
}

// The race condition, deterministically: whoever ends a migration with an OLD snapshot of it
// (which was read before the other side finished) cannot overwrite the outcome.
func TestCancel_TerminationWithOldSnapshotDoesNotOverwrite(t *testing.T) {
	h, _ := verifyHarness(t)

	t.Run("cancel after completion", func(t *testing.T) {
		tr, dest := h.seedTorrent(t, 30, "Old30")
		source := tr.NodeID
		m := h.start(t, tr.ID, dest)
		h.endsAtEnd(t, tr.ID)
		h.setCmd(t, h.serveCmdID(t, m.ID), "DONE")
		h.pullDone(t, m.ID)
		h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
		stale, err := h.mig.Get(h.ctx, m.ID) // the snapshot that the cancellation read: PENDING
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		h.reconcile(t) // the reconciler finishes halfway through
		if got := h.migStatus(t, m.ID); got != "COMPLETED" {
			t.Fatalf("precondition: status = %s, want COMPLETED", got)
		}

		did, err := h.mig.AbortForTest(h.ctx, stale)
		if err != nil || did {
			t.Fatalf("abort with stale snapshot = (%v, %v), want (false, nil)", did, err)
		}
		if got := h.migStatus(t, m.ID); got != "COMPLETED" {
			t.Errorf("status = %s, want COMPLETED (late cancellation overwrote completion)", got)
		}
		if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest); got != 0 {
			t.Errorf("remove_torrent on destination = %d, want 0 — it would delete the ONLY copy", got)
		}
		if got := h.cmdCountForHashOnNode(t, "resume_torrent", *tr.Hash, source); got != 0 {
			t.Errorf("resume_torrent on source = %d, want 0 — the source has already been removed", got)
		}
	})

	t.Run("complete after cancellation", func(t *testing.T) {
		tr, dest := h.seedTorrent(t, 31, "Old31")
		source := tr.NodeID
		m := h.start(t, tr.ID, dest)
		h.endsAtEnd(t, tr.ID)
		h.setCmd(t, h.serveCmdID(t, m.ID), "DONE")
		h.pullDone(t, m.ID)
		h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
		stale, err := h.mig.Get(h.ctx, m.ID) // the snapshot that the reconciler read: PENDING
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		h.mustCancel(t, m.ID) // the operator cancels halfway through

		if err := h.mig.CompleteForTest(h.ctx, stale); err != nil {
			t.Fatalf("complete with stale snapshot: %v", err)
		}
		if got := h.migStatus(t, m.ID); got != "FAILED" {
			t.Errorf("status = %s, want FAILED (late completion overwrote cancellation)", got)
		}
		if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 0 {
			t.Errorf("remove_torrent on SOURCE = %d, want 0 — the source is the good copy", got)
		}
		// The rollback undid the replacement: the origin row stays alive, in place.
		if got := h.liveRows(t, source, *tr.Hash); got != 1 {
			t.Errorf("live rows at source = %d, want 1 (late complete soft-deleted the source)", got)
		}
		if got := h.torrentNodeID(t, tr.ID); got != source {
			t.Errorf("node_id = %s, want source %s", got, source)
		}
	})
}

// The invariant that conditional complete() and abort() guarantee: completing and canceling at
// the same time NEVER both win. A cancellation that arrives at the exact instant the
// reconciler completes would overwrite COMPLETED and delete the ONLY copy (the one at the
// destination), with the origin already removed. Run both actions in parallel several times and check
// that the outcome is one of the two, intact.
func TestCancel_VersusCompletionNeverWinTogether(t *testing.T) {
	h, _ := verifyHarness(t)
	for i := 0; i < 12; i++ {
		tr, dest := h.seedTorrent(t, 100+i, "Race")
		source := tr.NodeID
		m := h.start(t, tr.ID, dest)
		h.endsAtEnd(t, tr.ID)
		h.setCmd(t, h.serveCmdID(t, m.ID), "DONE")
		h.pullDone(t, m.ID)
		h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0) // ready for the reconciler to complete

		var wg sync.WaitGroup
		var cancelErr, reconcileErr error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, cancelErr = h.mig.Cancel(h.ctx, m.ID)
		}()
		go func() {
			defer wg.Done()
			<-start
			reconcileErr = h.mig.Reconcile(h.ctx)
		}()
		close(start)
		wg.Wait()
		if reconcileErr != nil {
			t.Fatalf("iteration %d: Reconcile: %v", i, reconcileErr)
		}

		removeSource := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source)
		removeDest := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest)
		resume := h.cmdCountForHashOnNode(t, "resume_torrent", *tr.Hash, source)
		switch got := h.migStatus(t, m.ID); got {
		case "COMPLETED":
			if !errors.Is(cancelErr, migration.ErrMigrationFinished) {
				t.Errorf("iteration %d: completed, but Cancel = %v (want ErrMigrationFinished)", i, cancelErr)
			}
			if removeSource != 1 || removeDest != 0 || resume != 0 {
				t.Errorf("iteration %d: COMPLETED with removeSource=%d removeDest=%d resume=%d, want 1/0/0", i, removeSource, removeDest, resume)
			}
		case "FAILED":
			if cancelErr != nil {
				t.Errorf("iteration %d: canceled, but Cancel = %v", i, cancelErr)
			}
			if removeSource != 0 || removeDest != 1 || resume != 1 {
				t.Errorf("iteration %d: FAILED with removeSource=%d removeDest=%d resume=%d, want 0/1/1 — the source is the good copy", i, removeSource, removeDest, resume)
			}
			if got := h.liveRows(t, source, *tr.Hash); got != 1 {
				t.Errorf("iteration %d: live rows on source = %d, want 1 (nothing was restored on destination)", i, got)
			}
		default:
			t.Fatalf("iteration %d: final status = %s", i, got)
		}
		time.Sleep(time.Millisecond)
	}
}
