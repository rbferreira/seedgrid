package command_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

// registerNode creates a test Node and returns its id (the queue references nodes).
func registerNode(t *testing.T, dsn string) string {
	t.Helper()
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	svc := node.NewService(pool, events.NewEmitter(pool))
	if err := svc.EnsureDefaultCluster(context.Background()); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	res, err := svc.Register(context.Background(), node.RegisterInput{Hostname: "cmd-test-node"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return res.NodeID
}

func TestCommandQueueLifecycle(t *testing.T) {
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	nodeID := registerNode(t, dsn)

	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	svc := command.NewService(pool)
	ctx := context.Background()

	id, err := svc.Enqueue(ctx, nodeID, "add_torrent", json.RawMessage(`{"magnet":"magnet:?xt=abc"}`))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Claim must return the enqueued command.
	cmds, err := svc.Claim(ctx, nodeID, 10)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	var found *command.Command
	for i := range cmds {
		if cmds[i].ID == id {
			found = &cmds[i]
		}
	}
	if found == nil {
		t.Fatalf("queued command not returned by Claim")
	}
	if found.Type != "add_torrent" {
		t.Errorf("type = %q", found.Type)
	}

	// A second Claim must not return the same command (already DISPATCHED).
	again, err := svc.Claim(ctx, nodeID, 10)
	if err != nil {
		t.Fatalf("Claim 2: %v", err)
	}
	for _, c := range again {
		if c.ID == id {
			t.Error("already claimed command returned again")
		}
	}

	// SubmitResult completes the command.
	if err := svc.SubmitResult(ctx, nodeID, id, command.Result{Status: "DONE", Result: json.RawMessage(`{"hash":"abc"}`)}); err != nil {
		t.Fatalf("SubmitResult: %v", err)
	}

	// Result for non-existent command → ErrNotFound.
	if err := svc.SubmitResult(ctx, nodeID, "00000000-0000-0000-0000-000000000000", command.Result{Status: "DONE"}); err != command.ErrNotFound {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// TestClaim_RedeliversDispatchedOrphan (#111): a DISPATCHED command whose Agent never
// confirmed returns for redelivery after the window — and disappears from Claim when the window is off.
func TestClaim_RedeliversDispatchedOrphan(t *testing.T) {
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	nodeID := registerNode(t, dsn)

	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	svc := command.NewService(pool)
	svc.SetRedispatch(func() time.Duration { return time.Hour }, func() int { return 3 })
	ctx := context.Background()

	id, err := svc.Enqueue(ctx, nodeID, "pause_torrent", json.RawMessage(`{"hash":"redelivery-test"}`))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := svc.Claim(ctx, nodeID, 50); err != nil {
		t.Fatalf("Claim 1: %v", err)
	}

	// Agent "died": no SubmitResult. Inside the window, nothing returns.
	cmds, err := svc.Claim(ctx, nodeID, 50)
	if err != nil {
		t.Fatalf("Claim 2: %v", err)
	}
	for _, c := range cmds {
		if c.ID == id {
			t.Fatal("recent DISPATCHED command should not be redelivered within the window")
		}
	}

	// Ages the delivery beyond the window → the next Claim redelivers (attempts=2).
	if _, err := pool.Exec(ctx,
		`UPDATE commands SET dispatched_at = now() - interval '2 hours' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	cmds, err = svc.Claim(ctx, nodeID, 50)
	if err != nil {
		t.Fatalf("Claim 3: %v", err)
	}
	found := false
	for _, c := range cmds {
		if c.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("orphaned command beyond the window was not redelivered")
	}
	var attempts int // retry_count = completed deliveries (#111)
	if err := pool.QueryRow(ctx, `SELECT retry_count FROM commands WHERE id = $1`, id).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}

	// With redelivery TURNED OFF (window 0), an aged orphan does not return.
	svc.SetRedispatch(func() time.Duration { return 0 }, func() int { return 3 })
	if _, err := pool.Exec(ctx,
		`UPDATE commands SET dispatched_at = now() - interval '2 hours' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	cmds, err = svc.Claim(ctx, nodeID, 50)
	if err != nil {
		t.Fatalf("Claim 4: %v", err)
	}
	for _, c := range cmds {
		if c.ID == id {
			t.Error("redelivery disabled should not return the orphan")
		}
	}
}

// TestClaim_ExpiresAfterAttemptCap (#111): orphan that hit the max delivery attempts
// becomes FAILED ("abandoned") instead of spinning forever — and is no longer redelivered.
func TestClaim_ExpiresAfterAttemptCap(t *testing.T) {
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	nodeID := registerNode(t, dsn)

	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	svc := command.NewService(pool)
	svc.SetRedispatch(func() time.Duration { return time.Hour }, func() int { return 3 })
	ctx := context.Background()

	id, err := svc.Enqueue(ctx, nodeID, "resume_torrent", json.RawMessage(`{"hash":"cap-test"}`))
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Simulates an orphan that has already reached the max delivery attempts, all aged.
	if _, err := pool.Exec(ctx,
		`UPDATE commands SET status = 'DISPATCHED', retry_count = 3,
		        dispatched_at = now() - interval '2 hours' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	cmds, err := svc.Claim(ctx, nodeID, 50)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	for _, c := range cmds {
		if c.ID == id {
			t.Fatal("command beyond the cap should not be redelivered")
		}
	}
	st, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if st.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED", st.Status)
	}
	if st.Error == nil || *st.Error == "" {
		t.Error("abandoned command should carry the error message")
	}
}

// REGRESSION #28 — Claim delivers in the order commands entered the queue.
//
// The Agent executes serially, in the received order; Claim was the one shuffling, picking
// with ORDER BY in a subquery and returning via an UPDATE's RETURNING (without guaranteed
// order). Seen in the field: add → pause → resume → remove executed as resume, remove,
// add, pause — the "removed" torrent stayed in the Engine. The test covers both ways the
// order gets lost: tied created_at (multiple commands in one transaction) and rows
// physically out of order in the heap (an UPDATE moves the tuple to the end).
func TestClaim_DeliversInQueuingOrder(t *testing.T) {
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	nodeID := registerNode(t, dsn)

	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	svc := command.NewService(pool)
	ctx := context.Background()

	// The test Node is shared with other tests in the package: clears out whatever
	// they left in the queue.
	if _, err := pool.Exec(ctx,
		`UPDATE commands SET status = 'DONE', finished_at = now()
		  WHERE node_id = $1 AND status IN ('PENDING','DISPATCHED')`, nodeID); err != nil {
		t.Fatalf("clear queue: %v", err)
	}

	var want []string
	// 1) Twelve commands in a single transaction: created_at (now() = transaction start) ties.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for i := 0; i < 12; i++ {
		id, err := svc.EnqueueTx(ctx, tx, nodeID, "pause_torrent", json.RawMessage(`{"hash":"order"}`))
		if err != nil {
			t.Fatalf("EnqueueTx %d: %v", i, err)
		}
		want = append(want, id)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	// 2) Nine more, one per transaction.
	for i := 0; i < 9; i++ {
		id, err := svc.Enqueue(ctx, nodeID, "resume_torrent", json.RawMessage(`{"hash":"order"}`))
		if err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
		want = append(want, id)
	}
	// 3) Takes the rows out of physical order: the UPDATE writes a new version of each tuple at the end
	// of the heap, so the first commands end up AFTER the last ones on disk.
	for _, i := range []int{0, 3, 4, 13, 1} {
		if _, err := pool.Exec(ctx, `UPDATE commands SET payload = payload WHERE id = $1`, want[i]); err != nil {
			t.Fatalf("reorder heap: %v", err)
		}
	}

	// Delivery in batches smaller than the queue: the CHOICE of each batch and the ORDER within it must
	// follow the queue.
	var got []string
	for len(got) < len(want) {
		cmds, err := svc.Claim(ctx, nodeID, 5)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if len(cmds) == 0 {
			break
		}
		for _, c := range cmds {
			got = append(got, c.ID)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("Claim delivered %d commands, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order swapped at position %d: command enqueued in position %d was delivered out of place\n want %v\n got %v",
				i, i+1, want, got)
		}
	}
}
