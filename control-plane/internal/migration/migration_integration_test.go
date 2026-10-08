package migration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

type harness struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	mig   *migration.Service
	tor   *torrent.Service
	nodeA string
	nodeB string
}

// newHarness registers 2 ONLINE Nodes with IP and returns the migration/torrent services.
func newHarness(t *testing.T) harness {
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

	nodeSvc := node.NewService(pool, events.NewEmitter(pool))
	if err := nodeSvc.EnsureDefaultCluster(ctx); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	a, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "mig-node-a", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	b, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "mig-node-b", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register b: %v", err)
	}
	// last_ip is filled in the heartbeat; here we set it directly to resolve the endpoint.
	if _, err := pool.Exec(ctx, `UPDATE nodes SET last_ip = '10.0.0.9' WHERE id IN ($1,$2)`, a.NodeID, b.NodeID); err != nil {
		t.Fatalf("set last_ip: %v", err)
	}

	return harness{
		ctx:   ctx,
		pool:  pool,
		mig:   migration.NewService(pool, command.NewService(pool), events.NewEmitter(pool), 6900),
		tor:   torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool)),
		nodeA: a.NodeID,
		nodeB: b.NodeID,
	}
}

func (h harness) addTorrent(t *testing.T, magnet string) torrent.Torrent {
	t.Helper()
	tr, err := h.tor.Add(h.ctx, magnet, "", false)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	return tr
}

func (h harness) other(nodeID string) string {
	if nodeID == h.nodeA {
		return h.nodeB
	}
	return h.nodeA
}

// removeCountForHash counts the queued remove_torrent for a specific hash. The
// tests share the mig-node-a/b Nodes (same IDs across the entire suite), so counting
// remove_torrent per Node accumulates across tests; filtering by payload hash isolates
// the test's own torrent — essential for idempotency assertions (1 remove).
func (h harness) removeCountForHash(t *testing.T, hash string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM commands WHERE type = 'remove_torrent' AND payload->>'hash' = $1`,
		hash).Scan(&n); err != nil {
		t.Fatalf("removeCountForHash: %v", err)
	}
	return n
}

// cmdCountForHash counts commands of a type queued for a specific hash —
// same reason as removeCountForHash (Nodes shared across tests in the suite).
func (h harness) cmdCountForHash(t *testing.T, cmdType, hash string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM commands WHERE type = $1 AND payload->>'hash' = $2`,
		cmdType, hash).Scan(&n); err != nil {
		t.Fatalf("cmdCountForHash(%s): %v", cmdType, err)
	}
	return n
}

func (h harness) pullCmdID(t *testing.T, migID string) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT pull_command_id::text FROM torrent_migrations WHERE id = $1`, migID).Scan(&id); err != nil {
		t.Fatalf("pullCmdID: %v", err)
	}
	return id
}

func (h harness) torrentNodeID(t *testing.T, torrentID string) string {
	t.Helper()
	var id string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT node_id::text FROM torrents WHERE id = $1`, torrentID).Scan(&id); err != nil {
		t.Fatalf("torrentNodeID: %v", err)
	}
	return id
}

// countByStatus counts migrations in a status, restricted to given torrents — the
// harness does NOT clear the DB between tests, so global counting would add up the
// migrations left behind by previous tests.
func (h harness) countByStatus(t *testing.T, status string, torrentIDs []string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM torrent_migrations WHERE status = $1 AND torrent_id = ANY($2::uuid[])`,
		status, torrentIDs).Scan(&n); err != nil {
		t.Fatalf("countByStatus: %v", err)
	}
	return n
}

func (h harness) migStatus(t *testing.T, migID string) string {
	t.Helper()
	var s string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT status FROM torrent_migrations WHERE id = $1`, migID).Scan(&s); err != nil {
		t.Fatalf("migStatus: %v", err)
	}
	return s
}

// sampleTorrent is a minimal .torrent (bencode) to persist in the tests.
func sampleTorrent() []byte {
	return []byte("d4:infod6:lengthi1e4:name4:demo12:piece lengthi16384eee")
}

// start queues the migration and promotes it next (ADR-031: Migrate now only
// QUEUES — the reconciler issues the token and triggers serve/pull). Returns the
// migration already in PENDING, which is the state the outcome tests start from.
func (h harness) start(t *testing.T, torrentID, dest string) migration.Migration {
	t.Helper()
	m, err := h.mig.Migrate(h.ctx, torrentID, dest)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if m.Status != "QUEUED" {
		t.Fatalf("status at creation = %q, want QUEUED", m.Status)
	}
	if err := h.mig.ReconcileAndPromote(h.ctx, 10); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got := h.migStatus(t, m.ID); got != "PENDING" {
		t.Fatalf("status after promotion = %q, want PENDING", got)
	}
	m.Status = "PENDING"
	return m
}

// minProgressOfPull reads the min_progress sent in the migrate_pull payload (ADR-031).
func (h harness) minProgressOfPull(t *testing.T, migID string) float64 {
	t.Helper()
	var payload []byte
	if err := h.pool.QueryRow(h.ctx,
		`SELECT c.payload FROM torrent_migrations m JOIN commands c ON c.id = m.pull_command_id
		  WHERE m.id = $1`, migID).Scan(&payload); err != nil {
		t.Fatalf("read pull payload: %v", err)
	}
	var p struct {
		MinProgress float64 `json:"min_progress"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return p.MinProgress
}

func TestMigrate_CompletesOnPullDone(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&dn=Mig1")
	source := tr.NodeID
	dest := h.other(source)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}

	m := h.start(t, tr.ID, dest)
	// Counted by hash: Nodes are shared across tests in the suite, and counting by
	// Node sums the commands from all tests that ran before.
	if got := h.cmdCountForHashOnNode(t, "migrate_serve", *tr.Hash, source); got != 1 {
		t.Errorf("migrate_serve at source = %d, want 1", got)
	}
	if got := h.cmdCountForHashOnNode(t, "migrate_pull", *tr.Hash, dest); got != 1 {
		t.Errorf("migrate_pull at destination = %d, want 1", got)
	}

	// Already in flight → second migration rejected.
	if _, err := h.mig.Migrate(h.ctx, tr.ID, dest); !errors.Is(err, migration.ErrAlreadyMigrating) {
		t.Fatalf("second Migrate = %v, want ErrAlreadyMigrating", err)
	}

	// Destination completes pull and recheck: destination Agent reports torrent SEEDING and the
	// reconcile finishes. Pull DONE alone isn't enough (#87) — the CP completes based on
	// the torrent state at the destination.
	h.pullDone(t, m.ID)
	h.destReport(t, dest, *tr.Hash, "SEEDING", 1.0)
	if err := h.mig.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Torrent now lives at the destination: source row is soft-deleted and the destination
	// (the one SyncStates adopted during verification) is the live one.
	if got := h.liveRows(t, dest, *tr.Hash); got != 1 {
		t.Errorf("live rows at destination = %d, want 1", got)
	}
	if got := h.liveRows(t, source, *tr.Hash); got != 0 {
		t.Errorf("live rows at source = %d, want 0", got)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 1 {
		t.Errorf("remove_torrent at source = %d, want 1", got)
	}
	if got := h.migStatus(t, m.ID); got != "COMPLETED" {
		t.Errorf("migration status = %s, want COMPLETED", got)
	}

	// Regression of incident 2026-07-18 (27.293 remove_torrent): rerunning the reconciler
	// with pull still DONE MUST NOT requeue the remove. Migration is already COMPLETED and
	// leaves the PENDING filter; remove remains exactly 1. This is the invariant the loop
	// violated — complete() rerun every cycle queued one remove at a time.
	for i := 0; i < 3; i++ {
		if err := h.mig.Reconcile(h.ctx); err != nil {
			t.Fatalf("idempotent Reconcile (%d): %v", i, err)
		}
	}
	if got := h.removeCountForHash(t, *tr.Hash); got != 1 {
		t.Errorf("remove_torrent after repeated reconciles = %d, want 1 (incident loop)", got)
	}
}

// TestMigrate_CompletesWhenDestAlreadyHasTorrent reproduces incident 2026-07-18: the
// destination ALREADY has a live row for the hash (SyncStates adopted a copy that the
// destination downloaded). Before, complete() did blind UPDATE node_id → collided with
// uk_torrents_node_hash → error repeated every 15s locked the ENTIRE queue. Now it must
// soft-delete the source and complete without colliding.
func TestMigrate_CompletesWhenDestAlreadyHasTorrent(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&dn=Dup")
	source := tr.NodeID
	dest := h.other(source)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	m := h.start(t, tr.ID, dest)
	h.pullDone(t, m.ID)
	// Destination adopts its own copy of the same hash (live row, different torrent_id).
	// Since #87 this is the NORMAL path — verification only completes looking at this row —,
	// but was how the incident manifested: replacement collided with uk_torrents_node_hash.
	if _, err := h.pool.Exec(h.ctx,
		`INSERT INTO torrents (node_id, hash, name, status, progress, last_synced_at)
		 VALUES ($1, $2, 'Dup (adotado)', 'SEEDING', 1, now())`,
		dest, tr.Hash); err != nil {
		t.Fatalf("insert adopted copy into destination: %v", err)
	}
	// Pull DONE → reconcile must complete WITHOUT colliding.
	if err := h.mig.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile should not fail (destination already has the torrent): %v", err)
	}
	if got := h.migStatus(t, m.ID); got != "COMPLETED" {
		t.Errorf("status = %s, want COMPLETED", got)
	}
	// The SOURCE row was soft-deleted (the torrent is already at the destination).
	var srcDeleted bool
	if err := h.pool.QueryRow(h.ctx,
		`SELECT deleted_at IS NOT NULL FROM torrents WHERE id = $1`, tr.ID).Scan(&srcDeleted); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !srcDeleted {
		t.Error("the source row should have been soft-deleted")
	}
	// The destination copy remains live (it's the torrent that stayed).
	var liveAtDest int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM torrents WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL`,
		dest, tr.Hash).Scan(&liveAtDest); err != nil {
		t.Fatalf("count at destination: %v", err)
	}
	if liveAtDest != 1 {
		t.Errorf("live rows at destination = %d, want 1", liveAtDest)
	}
	// This is the EXACT incident scenario: the replacement collision made complete() fail
	// every cycle, endlessly requeuing the remove. Repeated reconciles must keep the
	// remove at 1 (the migration is already COMPLETED and no longer reprocessed).
	for i := 0; i < 3; i++ {
		if err := h.mig.Reconcile(h.ctx); err != nil {
			t.Fatalf("idempotent Reconcile (%d): %v", i, err)
		}
	}
	if got := h.removeCountForHash(t, *tr.Hash); got != 1 {
		t.Errorf("remove_torrent after repeated reconciles = %d, want 1 (incident loop)", got)
	}
}

func TestMigrate_FailsOnPullFailedNoDataLoss(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&dn=Mig2")
	source := tr.NodeID
	dest := h.other(source)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	m := h.start(t, tr.ID, dest)
	if _, err := h.pool.Exec(h.ctx, `UPDATE commands SET status='FAILED', error='recheck failed' WHERE id = $1`, h.pullCmdID(t, m.ID)); err != nil {
		t.Fatalf("mark pull FAILED: %v", err)
	}
	if err := h.mig.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// No loss: torrent remains at the source; migration FAILED.
	if got := h.torrentNodeID(t, tr.ID); got != source {
		t.Errorf("node_id = %s, want source %s (no loss)", got, source)
	}
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Errorf("status = %s, want FAILED", got)
	}
	// Issue #8: source Agent STOPS the torrent when serving. "Remains at source" is only
	// true if it resumes running when the migration aborts — otherwise a failed drain
	// leaves torrents stopped in silence.
	if got := h.cmdCountForHash(t, "resume_torrent", *tr.Hash); got != 1 {
		t.Errorf("resume_torrent on source = %d, want 1 (torrent would stay stopped after abort)", got)
	}
}

// A torrent held by admission (ADR-021/024) is already paused BY DECISION of the
// controller. Resuming it when aborting migration would bypass the hold — the queue
// is managed by the Admission Worker, not the migration reconciler.
func TestMigrate_FailureDoesNotResumeTorrentInAdmissionHold(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:c3aa000000000000000000000000000000000001&dn=EmHold")
	dest := h.other(tr.NodeID)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	m := h.start(t, tr.ID, dest)
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE torrents SET admission_paused_at = now() WHERE id = $1`, tr.ID); err != nil {
		t.Fatalf("mark admission hold: %v", err)
	}
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE commands SET status='FAILED', error='recheck failed' WHERE id = $1`, h.pullCmdID(t, m.ID)); err != nil {
		t.Fatalf("mark pull FAILED: %v", err)
	}
	if err := h.mig.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := h.cmdCountForHash(t, "resume_torrent", *tr.Hash); got != 0 {
		t.Errorf("resume_torrent = %d, want 0 (admission hold dictates)", got)
	}
}

// TestMigrate_PartialSendsSourceMinProgress covers the core of ADR-031: migrating an
// INCOMPLETE torrent must send source progress to the destination as a floor, and not
// require the whole torrent. Before, every partial failed (destination only responded
// DONE in SEEDING), even though the copy was perfect.
func TestMigrate_PartialSendsSourceMinProgress(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:ad31000000000000000000000000000000000001&dn=Partial")
	dest := h.other(tr.NodeID)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	// Source is at 60% (the Node drain scenario with no space).
	if _, err := h.pool.Exec(h.ctx, `UPDATE torrents SET progress = 0.6 WHERE id = $1`, tr.ID); err != nil {
		t.Fatalf("set progress: %v", err)
	}

	m := h.start(t, tr.ID, dest)
	if got := h.minProgressOfPull(t, m.ID); got != 0.6 {
		t.Errorf("min_progress = %v, want 0.6 (source progress)", got)
	}

	// Destination confirms (recheck done, downloading the rest) → migration completes
	// NORMALLY, even with the torrent at 60%.
	h.pullDone(t, m.ID)
	h.destReport(t, dest, *tr.Hash, "DOWNLOADING", 0.6)
	if err := h.mig.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := h.migStatus(t, m.ID); got != "COMPLETED" {
		t.Errorf("status = %s, want COMPLETED (partial migration is success)", got)
	}
	if got := h.liveRows(t, dest, *tr.Hash); got != 1 {
		t.Errorf("live rows at destination = %d, want 1", got)
	}
}

// TestMigrate_UnknownProgressRequiresComplete: no progress in inventory, the floor
// is 1.0 — when in doubt, the conservative behavior from ADR-018.
func TestMigrate_UnknownProgressRequiresComplete(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:ad31000000000000000000000000000000000002&dn=NoProgress")
	dest := h.other(tr.NodeID)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	if _, err := h.pool.Exec(h.ctx, `UPDATE torrents SET progress = NULL WHERE id = $1`, tr.ID); err != nil {
		t.Fatalf("clear progress: %v", err)
	}
	m := h.start(t, tr.ID, dest)
	if got := h.minProgressOfPull(t, m.ID); got != 1.0 {
		t.Errorf("min_progress = %v, want 1.0 (conservative when progress is unknown)", got)
	}
}

// TestMigrate_RespectsMaxConcurrent covers the concurrency lock (ADR-031): a drain
// cannot turn into N simultaneous transfers — that's what breaks the I/O of the Node being
// emptied.
func TestMigrate_RespectsMaxConcurrent(t *testing.T) {
	h := newHarness(t)
	// The concurrency ceiling is GLOBAL (it's the cluster disk to protect), and the
	// harness doesn't clear the DB between tests: in-flight migrations left by previous
	// tests would fill the slots and nothing would be promoted here. Neutralize before.
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE torrent_migrations SET status = 'FAILED', error = 'encerrada pelo setup do teste'
		  WHERE status IN ('QUEUED', 'PENDING')`); err != nil {
		t.Fatalf("clear in-flight migrations: %v", err)
	}
	// All on the SAME source (Scheduler distributes among Nodes and some would have the
	// destination as source): it's the drain scenario, which is what the lock protects.
	source, dest := h.nodeA, h.nodeB
	var ids []string
	for i, magnet := range []string{
		"magnet:?xt=urn:btih:ad31000000000000000000000000000000000011&dn=C1",
		"magnet:?xt=urn:btih:ad31000000000000000000000000000000000012&dn=C2",
		"magnet:?xt=urn:btih:ad31000000000000000000000000000000000013&dn=C3",
	} {
		tr := h.addTorrent(t, magnet)
		if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
			t.Fatalf("StoreTorrentFile: %v", err)
		}
		if _, err := h.pool.Exec(h.ctx, `UPDATE torrents SET node_id = $2 WHERE id = $1`, tr.ID, source); err != nil {
			t.Fatalf("fix source: %v", err)
		}
		if _, err := h.mig.Migrate(h.ctx, tr.ID, dest); err != nil {
			t.Fatalf("Migrate %d: %v", i, err)
		}
		ids = append(ids, tr.ID)
	}

	// Ceiling 1 → only ONE leaves the queue at a time.
	if err := h.mig.ReconcileAndPromote(h.ctx, 1); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got := h.countByStatus(t, "PENDING", ids); got != 1 {
		t.Errorf("PENDING = %d, want 1 (concurrency ceiling)", got)
	}
	if got := h.countByStatus(t, "QUEUED", ids); got != 2 {
		t.Errorf("QUEUED = %d, want 2 (the others wait)", got)
	}

	// Running again without completing anything DOES NOT promote anyone else (the slot remains occupied).
	if err := h.mig.ReconcileAndPromote(h.ctx, 1); err != nil {
		t.Fatalf("promote 2nd time: %v", err)
	}
	if got := h.countByStatus(t, "PENDING", ids); got != 1 {
		t.Errorf("PENDING after 2nd pass = %d, want 1", got)
	}

	// Completing the in-flight one frees the slot for the next one in the queue.
	var inflightID string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT id::text FROM torrent_migrations WHERE status = 'PENDING' AND torrent_id = ANY($1::uuid[])`,
		ids).Scan(&inflightID); err != nil {
		t.Fatalf("find inflight: %v", err)
	}
	h.pullDone(t, inflightID)
	var inflightHash string
	if err := h.pool.QueryRow(h.ctx,
		`SELECT t.hash FROM torrent_migrations m JOIN torrents t ON t.id = m.torrent_id WHERE m.id = $1`,
		inflightID).Scan(&inflightHash); err != nil {
		t.Fatalf("in-flight migration hash: %v", err)
	}
	h.destReport(t, dest, inflightHash, "SEEDING", 1.0) // the destination confirms (#87)
	if err := h.mig.ReconcileAndPromote(h.ctx, 1); err != nil {
		t.Fatalf("promote 3rd time: %v", err)
	}
	if got := h.countByStatus(t, "COMPLETED", ids); got != 1 {
		t.Errorf("COMPLETED = %d, want 1", got)
	}
	if got := h.countByStatus(t, "PENDING", ids); got != 1 {
		t.Errorf("PENDING after freeing a slot = %d, want 1 (next in queue entered)", got)
	}
}

// setHeartbeatDisk writes a heartbeat with disk data for a Node (ADR-032). The
// test DB is shared across packages and the migration harness does not remove its
// Nodes; a heartbeat left here would change the Scheduler's disk sorting in ANOTHER
// package (the `proj_free DESC` would promote this Node). So we remove it in cleanup.
func (h harness) setHeartbeatDisk(t *testing.T, nodeID string, freeBytes, totalBytes int64) {
	t.Helper()
	if _, err := h.pool.Exec(h.ctx,
		`INSERT INTO heartbeats (node_id, disk_total_bytes, disk_free_bytes)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (node_id) DO UPDATE SET disk_total_bytes = EXCLUDED.disk_total_bytes,
		                                     disk_free_bytes = EXCLUDED.disk_free_bytes`,
		nodeID, totalBytes, freeBytes); err != nil {
		t.Fatalf("setHeartbeatDisk: %v", err)
	}
	t.Cleanup(func() { _, _ = h.pool.Exec(context.Background(), `DELETE FROM heartbeats WHERE node_id = $1`, nodeID) })
}

// TestMigrate_BlocksWhenDestNoSpace: migration refused when destination can't fit the
// TOTAL size of the torrent + margin (ADR-032). Without disk data, it does not block.
func TestMigrate_BlocksWhenDestNoSpace(t *testing.T) {
	const gib = int64(1) << 30
	h := newHarness(t)
	h.mig.SetDiskMinFree(func() int64 { return 5 * gib }) // 5 GiB margin

	tr := h.addTorrent(t, "magnet:?xt=urn:btih:ad32000000000000000000000000000000000001&dn=Large")
	dest := h.other(tr.NodeID)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	// Torrent de 20 GiB.
	if _, err := h.pool.Exec(h.ctx, `UPDATE torrents SET size_bytes = $2 WHERE id = $1`, tr.ID, 20*gib); err != nil {
		t.Fatalf("set size: %v", err)
	}

	// 1) Destination without disk data → DOES NOT block (there is no way to judge).
	if _, err := h.mig.Migrate(h.ctx, tr.ID, dest); err != nil {
		t.Fatalf("without disk data should not block: %v", err)
	}
	// Clears the created migration to repeat.
	if _, err := h.pool.Exec(h.ctx, `DELETE FROM torrent_migrations WHERE torrent_id = $1`, tr.ID); err != nil {
		t.Fatalf("clear migration: %v", err)
	}

	// 2) Destination with only 10 GiB free → 20 GiB + 5 margin DOES NOT fit → blocks.
	h.setHeartbeatDisk(t, dest, 10*gib, 500*gib)
	if _, err := h.mig.Migrate(h.ctx, tr.ID, dest); !errors.Is(err, migration.ErrDestNoSpace) {
		t.Fatalf("Migrate = %v, want ErrDestNoSpace", err)
	}

	// 3) Destination with 30 GiB free → 20 + 5 = 25 ≤ 30 → passes.
	h.setHeartbeatDisk(t, dest, 30*gib, 500*gib)
	if _, err := h.mig.Migrate(h.ctx, tr.ID, dest); err != nil {
		t.Fatalf("with space should migrate: %v", err)
	}
}

func TestMigrate_RequiresTorrentFile(t *testing.T) {
	h := newHarness(t)
	// Own hash: the harness doesn't clear the database between tests, and "cccc" was left alive in the
	// destination after TestMigrate_CompletesWhenDestAlreadyHasTorrent (adopted copy).
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&dn=Mig3")
	dest := h.other(tr.NodeID)
	if _, err := h.mig.Migrate(h.ctx, tr.ID, dest); !errors.Is(err, migration.ErrNoTorrentFile) {
		t.Fatalf("Migrate without .torrent = %v, want ErrNoTorrentFile", err)
	}
}

// cmdCountForHashOnNode counts commands of a type, for a hash, on a specific Node
// — the cleanup from #79 is distinguished from the source remove precisely by the destination Node.
func (h harness) cmdCountForHashOnNode(t *testing.T, cmdType, hash, nodeID string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM commands WHERE type = $1 AND payload->>'hash' = $2 AND node_id = $3`,
		cmdType, hash, nodeID).Scan(&n); err != nil {
		t.Fatalf("cmdCountForHashOnNode(%s): %v", cmdType, err)
	}
	return n
}

// Migration aborted AFTER the pull leaves the partial copy in the destination /data (11 GB
// orphaned in the case that originated #79). The torrent remains at the source — what was left in the
// destination is a draft and needs to be removed WITH the data.
func TestMigrate_FailureCleansCopyAtDestination(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:79aa000000000000000000000000000000000001&dn=Orphan")
	source := tr.NodeID
	dest := h.other(source)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	m := h.start(t, tr.ID, dest)
	if _, err := h.pool.Exec(h.ctx,
		`UPDATE commands SET status='FAILED', error='recheck stuck' WHERE id = $1`, h.pullCmdID(t, m.ID)); err != nil {
		t.Fatalf("mark pull FAILED: %v", err)
	}
	if err := h.mig.Reconcile(h.ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Fatalf("status = %s, want FAILED", got)
	}
	// The orphaned copy at the destination is removed along with the data...
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest); got != 1 {
		t.Errorf("remove_torrent on destination = %d, want 1 (orphan copy would remain on disk — #79)", got)
	}
	var deleteFiles bool
	if err := h.pool.QueryRow(h.ctx,
		`SELECT (payload->>'delete_files')::bool FROM commands
		  WHERE type='remove_torrent' AND payload->>'hash' = $1 AND node_id = $2`,
		*tr.Hash, dest).Scan(&deleteFiles); err != nil {
		t.Fatalf("read remove payload: %v", err)
	}
	if !deleteFiles {
		t.Error("delete_files = false; cleanup must remove the data too, otherwise the destination disk is not freed")
	}
	// ...and the source remains intact and back online (ADR-018 contract).
	if got := h.torrentNodeID(t, tr.ID); got != source {
		t.Errorf("node_id = %s, want source %s (no loss)", got, source)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, source); got != 0 {
		t.Errorf("remove_torrent on SOURCE = %d, want 0 — the source is the good copy", got)
	}
	if got := h.cmdCountForHash(t, "resume_torrent", *tr.Hash); got != 1 {
		t.Errorf("resume_torrent = %d, want 1", got)
	}
}

// Migration that dies BEFORE the pull (no endpoint, no space, token not issued) did not
// leave anything at the destination. Sending a remove there would be asking the Node to delete a
// torrent it might have for another reason — including a legitimate copy.
func TestMigrate_FailureBeforePullDoesNotTouchDestination(t *testing.T) {
	h := newHarness(t)
	tr := h.addTorrent(t, "magnet:?xt=urn:btih:79bb000000000000000000000000000000000002&dn=NoPull")
	source := tr.NodeID
	dest := h.other(source)
	if err := h.tor.StoreTorrentFile(h.ctx, tr.ID, sampleTorrent(), "upload"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}
	m, err := h.mig.Migrate(h.ctx, tr.ID, dest)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Source loses the endpoint before promotion: the migration fails while still in QUEUED,
	// without getting to queue serve/pull — so nothing was copied to the destination.
	// Nodes are shared across the suite: restore the endpoint at the end, otherwise the
	// following tests would inherit a source without last_ip and fail as a result.
	var ipBefore *string
	if err := h.pool.QueryRow(h.ctx, `SELECT last_ip FROM nodes WHERE id = $1`, source).Scan(&ipBefore); err != nil {
		t.Fatalf("read source last_ip: %v", err)
	}
	t.Cleanup(func() {
		_, _ = h.pool.Exec(h.ctx, `UPDATE nodes SET last_ip = $2 WHERE id = $1`, source, ipBefore)
	})
	if _, err := h.pool.Exec(h.ctx, `UPDATE nodes SET last_ip = NULL WHERE id = $1`, source); err != nil {
		t.Fatalf("clear source last_ip: %v", err)
	}
	if err := h.mig.ReconcileAndPromote(h.ctx, 10); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got := h.migStatus(t, m.ID); got != "FAILED" {
		t.Fatalf("status = %s, want FAILED", got)
	}
	if got := h.cmdCountForHashOnNode(t, "remove_torrent", *tr.Hash, dest); got != 0 {
		t.Errorf("remove_torrent on destination = %d, want 0 (nothing was copied there)", got)
	}
}
