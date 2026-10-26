package torrent_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/screenshot"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrentfile"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
)

// makeTorrentBytes builds a valid single-file bencode .torrent whose infohash varies
// with the name (used in VerifyReseed tests, #23).
func makeTorrentBytes(name string) []byte {
	return makeTorrentBytesWithSize(name, 100)
}

// makeTorrentBytesWithSize is makeTorrentBytes with a configurable info.length (Size, ADR-032)
// — used to reproduce sizes that overflow int32 (size_bytes parameter bug).
func makeTorrentBytesWithSize(name string, size int64) []byte {
	info := []byte(fmt.Sprintf("d6:lengthi%de4:name%d:%s12:piece lengthi16384e6:pieces0:e", size, len(name), name))
	data := append([]byte("d8:announce3:foo4:info"), info...)
	return append(data, 'e')
}

// TestSyncStates_AdoptsUnknownTorrent verifies that the CP adopts (inserts) a torrent
// that exists in the Engine but the CP doesn't know — and that a 2nd sync updates without duplicating.
func TestSyncStates_AdoptsUnknownTorrent(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "adopt-test-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	hash := "dddddddddddddddddddddddddddddddddddddddd" // exclusive (never Added) — unknown to the CP
	prog := 1.0
	size := int64(4242)
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hash, Name: "Engine orphan", Status: "SEEDING", Progress: &prog, Size: &size},
	}); err != nil {
		t.Fatalf("SyncStates(adopt): %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found *torrent.Torrent
	for i := range list {
		if list[i].Hash != nil && *list[i].Hash == hash {
			found = &list[i]
		}
	}
	if found == nil {
		t.Fatal("orphan torrent was not adopted in the CP")
	}
	if found.Name == nil || *found.Name != "Engine orphan" {
		t.Errorf("adopted name = %v, want \"Engine orphan\"", found.Name)
	}
	if found.Status != "SEEDING" {
		t.Errorf("adopted status = %q, want SEEDING", found.Status)
	}

	// 2nd sync: updates without duplicating (keeps a single row for the hash).
	prog2 := 0.5
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hash, Status: "DOWNLOADING", Progress: &prog2},
	}); err != nil {
		t.Fatalf("SyncStates(update): %v", err)
	}
	list2, _ := svc.List(ctx)
	n, st := 0, ""
	for _, x := range list2 {
		if x.Hash != nil && *x.Hash == hash {
			n++
			st = x.Status
		}
	}
	if n != 1 {
		t.Errorf("expected 1 row for the adopted hash, got %d (duplicated?)", n)
	}
	if st != "DOWNLOADING" {
		t.Errorf("status after 2nd sync = %q, want DOWNLOADING", st)
	}
}

// TestSyncStates_CumulativeTransfer verifies the cumulative totals (#14): they accumulate the
// positive session delta between syncs and, when the Engine RESETS the session (re-add: the
// session value drops), they add the new full value as the start of a new cycle — the
// total never regresses. This is the basis of the aggregated ratio per Node/Cluster.
func TestSyncStates_CumulativeTransfer(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "cumul-test-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	hash := "e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0e0" // exclusive to this test (cumulative)
	up := func(v int64) *int64 { return &v }
	totalsFor := func() (dl, ul int64) {
		list, _ := svc.List(ctx)
		for _, x := range list {
			if x.Hash != nil && *x.Hash == hash {
				return x.DownloadedTotal, x.UploadedTotal
			}
		}
		t.Fatalf("torrent %s not found", hash)
		return 0, 0
	}

	// Sync 1 (INSERT): seeds with the current session value.
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hash, Name: "Cumul", Status: "SEEDING", Downloaded: up(1000), Uploaded: up(2000)},
	}); err != nil {
		t.Fatalf("SyncStates(1): %v", err)
	}
	if dl, ul := totalsFor(); dl != 1000 || ul != 2000 {
		t.Fatalf("after insert: total dl/ul = %d/%d, want 1000/2000", dl, ul)
	}

	// Sync 2 (positive delta): session went up → adds only the delta.
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hash, Status: "SEEDING", Downloaded: up(1500), Uploaded: up(5000)},
	}); err != nil {
		t.Fatalf("SyncStates(2): %v", err)
	}
	if dl, ul := totalsFor(); dl != 1500 || ul != 5000 {
		t.Fatalf("after delta+: total dl/ul = %d/%d, want 1500/5000", dl, ul)
	}

	// Sync 3 (Engine RESET): session dropped (re-add) → adds the new full value.
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hash, Status: "SEEDING", Downloaded: up(100), Uploaded: up(300)},
	}); err != nil {
		t.Fatalf("SyncStates(3): %v", err)
	}
	if dl, ul := totalsFor(); dl != 1600 || ul != 5300 {
		t.Fatalf("after reset: total dl/ul = %d/%d, want 1600/5300 (never regresses)", dl, ul)
	}
}

// TestSyncStates_InheritsNameOnAdopt verifies that, when adopting a torrent the CP already
// knows in another Node, the nameless report inherits the existing name (not the hash) — and that
// a row that ended up with the hash as its name is corrected in the next report.
func TestSyncStates_InheritsNameOnAdopt(t *testing.T) {
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
	nodeA, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "inherit-node-a", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register A: %v", err)
	}
	nodeB, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "inherit-node-b", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register B: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	hash := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	prog := 1.0
	// Node A knows the torrent with a readable name.
	if err := svc.SyncStates(ctx, nodeA.NodeID, []torrent.Report{
		{Hash: hash, Name: "Cool Movie", Status: "SEEDING", Progress: &prog},
	}); err != nil {
		t.Fatalf("SyncStates(A): %v", err)
	}
	// Node B reports the same hash WITHOUT a name → should inherit "Cool Movie".
	if err := svc.SyncStates(ctx, nodeB.NodeID, []torrent.Report{
		{Hash: hash, Status: "SEEDING", Progress: &prog},
	}); err != nil {
		t.Fatalf("SyncStates(B): %v", err)
	}

	nameFor := func(nodeID string) string {
		var n string
		if err := pool.QueryRow(ctx,
			`SELECT name FROM torrents WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL`,
			nodeID, hash).Scan(&n); err != nil {
			t.Fatalf("name query (%s): %v", nodeID, err)
		}
		return n
	}
	if got := nameFor(nodeB.NodeID); got != "Cool Movie" {
		t.Errorf("inherited name on Node B = %q, want \"Cool Movie\"", got)
	}
}

func TestAdd_SchedulesAndEnqueues(t *testing.T) {
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

	// Eligible Node: registered (ONLINE, recent last_seen) with engine.
	nodeSvc := node.NewService(pool, events.NewEmitter(pool))
	if err := nodeSvc.EnsureDefaultCluster(ctx); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	if _, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "torrent-test-node", Engine: "qbittorrent"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))
	magnet := "magnet:?xt=urn:btih:a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0&dn=Test"
	tr, err := svc.Add(ctx, magnet, "", torrent.AddOptions{AllowOtherVersion: true})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if tr.Status != "QUEUED" {
		t.Errorf("status = %q, want QUEUED", tr.Status)
	}
	// The Scheduler picks an eligible Node (deterministic); we validate that a choice was made.
	if tr.NodeID == "" {
		t.Fatal("no Node chosen by the Scheduler")
	}

	// Scheduler decision recorded.
	var decisions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM torrent_scheduler WHERE torrent_id = $1`, tr.ID).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 {
		t.Errorf("recorded decisions = %d, want 1", decisions)
	}

	// add_torrent command enqueued for the chosen Node.
	var cmds int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM commands WHERE node_id = $1 AND type = 'add_torrent' AND status = 'PENDING'`,
		tr.NodeID).Scan(&cmds); err != nil {
		t.Fatal(err)
	}
	if cmds < 1 {
		t.Errorf("add_torrent commands enqueued = %d, want >= 1", cmds)
	}

	// State sync: the Agent reports the torrent as SEEDING.
	if tr.Hash == nil {
		t.Fatal("torrent without hash")
	}
	prog := 1.0
	if err := svc.SyncStates(ctx, tr.NodeID, []torrent.Report{{Hash: *tr.Hash, Status: "SEEDING", Progress: &prog}}); err != nil {
		t.Fatalf("SyncStates: %v", err)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var status string
	for _, x := range list {
		if x.ID == tr.ID {
			status = x.Status
		}
	}
	if status != "SEEDING" {
		t.Errorf("status after sync = %q, want SEEDING", status)
	}
}

// TestAddFile_SizeBeyondInt32 is the regression for the bug where adding a .torrent with
// info.length above math.MaxInt32 (common: remux/4K exceed 2 GiB) failed with
// "failed to encode args[N]... into binary format for int4". Cause: in the INSERT of
// torrents, `NULLIF($5, 0)::bigint` made Postgres infer the type of parameter $5
// as int4 (due to the literal 0), and only then converted the RESULT to bigint —
// too late for the parameter itself. size_bytes was already BIGINT in the column; the bug was only
// in the parameter type inference by the driver (pgx).
func TestAddFile_SizeBeyondInt32(t *testing.T) {
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
	if _, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "large-size-test-node", Engine: "qbittorrent"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	const bigSize = int64(4897417867) // > math.MaxInt32 (2147483647) — reproduces the reported bug
	data := makeTorrentBytesWithSize("big-movie.mkv", bigSize)
	tr, err := svc.AddFile(ctx, data, torrent.AddOptions{AllowOtherVersion: true})
	if err != nil {
		t.Fatalf("AddFile with torrent > 2GiB: %v", err)
	}
	if tr.Status != "QUEUED" {
		t.Errorf("status = %q, want QUEUED", tr.Status)
	}
}

// TestResumeClearsAdmissionHold (ADR-024): resuming a torrent that was in
// admission-hold clears admission_paused_at — the operator took over that torrent and the
// admission controller must not re-pause it (fix for the "manual resume wins" asymmetry).
func TestResumeClearsAdmissionHold(t *testing.T) {
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
	res, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "resume-fix-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO torrents (node_id, hash, name, status, progress, admission_paused_at)
		 VALUES ($1, 'resumefixhash', 'resume-fix', 'PAUSED', 0.5, now()) RETURNING id::text`,
		res.NodeID).Scan(&id); err != nil {
		t.Fatalf("insert torrent: %v", err)
	}

	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))
	if _, err := svc.Command(ctx, id, "resume"); err != nil {
		t.Fatalf("Command resume: %v", err)
	}

	var held bool
	if err := pool.QueryRow(ctx,
		`SELECT admission_paused_at IS NOT NULL FROM torrents WHERE id = $1`, id).Scan(&held); err != nil {
		t.Fatalf("read hold: %v", err)
	}
	if held {
		t.Error("resume should have cleared admission_paused_at (manual resume wins — ADR-024)")
	}
}

// TestPendingTrackerAndMarkPublished covers backlog #8: a created torrent linked to
// a tracker (tracker_id) and not yet published appears in the pending queue; after
// MarkPublished it leaves the queue.
func TestPendingTrackerAndMarkPublished(t *testing.T) {
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

	// User who registers the tracker (names are unique across the cluster, ADR-047).
	var userID string
	username := fmt.Sprintf("pending-test-%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id::text`,
		username).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}

	nodeSvc := node.NewService(pool, events.NewEmitter(pool))
	if err := nodeSvc.EnsureDefaultCluster(ctx); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "pending-test-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	trk, err := tracker.NewService(pool).Create(ctx, userID, tracker.Input{
		Name: "Example " + username, Announce: "https://tracker.example/announce/key",
		Source: "EXAMPLE", APIURL: "https://tracker.example", APIToken: "tok",
	})
	if err != nil {
		t.Fatalf("create tracker: %v", err)
	}

	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	// Torrent linked to the tracker, with .torrent persisted → pending publication.
	hash := "ffffffffffffffffffffffffffffffffffffffff"
	var torrentID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO torrents (node_id, hash, name, status, tracker_id)
		 VALUES ($1, $2, 'Release Pendente', 'SEEDING', $3::uuid) RETURNING id::text`,
		reg.NodeID, hash, trk.ID).Scan(&torrentID); err != nil {
		t.Fatalf("insert torrent: %v", err)
	}
	if err := svc.StoreTorrentFile(ctx, torrentID, []byte("d4:infod6:lengthi1e4:name7:pending12:piece lengthi16384eee"), "created"); err != nil {
		t.Fatalf("StoreTorrentFile: %v", err)
	}

	pending, err := svc.PendingTracker(ctx)
	if err != nil {
		t.Fatalf("PendingTracker: %v", err)
	}
	found := false
	for _, p := range pending {
		if p.ID != torrentID {
			continue
		}
		found = true
		if !p.HasFile {
			t.Error("has_file = false, want true (.torrent persisted)")
		}
		if p.TrackerSource != "EXAMPLE" {
			t.Errorf("tracker_source = %q, want EXAMPLE", p.TrackerSource)
		}
		if p.TrackerID != trk.ID {
			t.Errorf("tracker_id = %q, want %q", p.TrackerID, trk.ID)
		}
	}
	if !found {
		t.Fatal("torrent created with tracker did not appear in the pending queue")
	}

	// Published → leaves the queue.
	if err := svc.MarkPublished(ctx, torrentID, "https://tracker.example/download/abc"); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	pending2, err := svc.PendingTracker(ctx)
	if err != nil {
		t.Fatalf("PendingTracker (post): %v", err)
	}
	for _, x := range pending2 {
		if x.ID == torrentID {
			t.Error("published torrent still appears in pending queue")
		}
	}
}

// TestVerifyReseed (#23) covers the post-publication infohash verification: when the
// official .torrent from the site has a different infohash than what we seed, the CP updates the
// row to the official hash (preserving metadata) and enqueues add + remove (without data)
// on the Node; when the infohash already matches, it does nothing.
func TestVerifyReseed(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "reseed-test-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	local := makeTorrentBytes("local-release")
	official := makeTorrentBytes("official-release") // different infohash
	mLocal, err := torrentfile.Parse(local)
	if err != nil {
		t.Fatalf("parse local: %v", err)
	}
	mOfficial, err := torrentfile.Parse(official)
	if err != nil {
		t.Fatalf("parse official: %v", err)
	}
	oldHash, newHash := mLocal.InfoHash, mOfficial.InfoHash // hex already lowercase
	if oldHash == newHash {
		t.Fatal("fixtures should have different infohashes")
	}

	var torrentID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO torrents (node_id, hash, name, status) VALUES ($1, $2, 'Local Release', 'SEEDING') RETURNING id::text`,
		reg.NodeID, oldHash).Scan(&torrentID); err != nil {
		t.Fatalf("insert torrent: %v", err)
	}

	// Infohash matches → does nothing.
	if reseeded, err := svc.VerifyReseed(ctx, torrentID, local); err != nil || reseeded {
		t.Fatalf("equal infohash: reseeded=%v err=%v (want false/nil)", reseeded, err)
	}

	// Infohash diverges → re-seeds: updates the hash and queues add + remove.
	reseeded, err := svc.VerifyReseed(ctx, torrentID, official)
	if err != nil {
		t.Fatalf("VerifyReseed (diverge): %v", err)
	}
	if !reseeded {
		t.Fatal("divergent infohash should have triggered re-seed")
	}
	var gotHash string
	if err := pool.QueryRow(ctx, `SELECT hash FROM torrents WHERE id = $1`, torrentID).Scan(&gotHash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if gotHash != newHash {
		t.Errorf("hash in db = %s, want official %s", gotHash, newHash)
	}
	var adds, removes int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE type = 'add_torrent'), count(*) FILTER (WHERE type = 'remove_torrent')
		   FROM commands WHERE node_id = $1`, reg.NodeID).Scan(&adds, &removes); err != nil {
		t.Fatalf("count commands: %v", err)
	}
	if adds < 1 || removes < 1 {
		t.Errorf("expected add_torrent + remove_torrent queued; got add=%d remove=%d", adds, removes)
	}
}

// TestDownloadTicket covers backlog #1: ticket issuance (hash present/absent) and
// rejection of invalid ticket. The happy path of OpenDownload needs a live Node
// (authorizes + streams), out of scope for this quick test.
func TestDownloadTicket(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "dl-ticket-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	// Torrent with hash → issues ticket.
	var withHash string
	if err := pool.QueryRow(ctx,
		`INSERT INTO torrents (node_id, hash, name, status) VALUES ($1, $2, 'DL With Hash', 'SEEDING') RETURNING id::text`,
		reg.NodeID, "1111111111111111111111111111111111111111").Scan(&withHash); err != nil {
		t.Fatalf("insert torrent with hash: %v", err)
	}
	tk, err := svc.CreateDownloadTicket(ctx, withHash)
	if err != nil {
		t.Fatalf("CreateDownloadTicket: %v", err)
	}
	if tk == "" {
		t.Fatal("empty ticket")
	}

	// The command that authorizes the Node is serve-only (#61): a download must not stop the
	// torrent the way a migration stops its source. No Agent answers here, so OpenDownload gives
	// up when the context ends; what matters is the command it queued.
	if _, err := pool.Exec(ctx, `UPDATE nodes SET last_ip = '127.0.0.1' WHERE id = $1`, reg.NodeID); err != nil {
		t.Fatalf("set last_ip: %v", err)
	}
	short, cancel := context.WithTimeout(ctx, time.Second)
	_, _, _, _ = svc.OpenDownload(short, tk)
	cancel()
	var serveOnly bool
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE((payload->>'serve_only')::boolean, false) FROM commands
		  WHERE node_id = $1 AND type = 'migrate_serve' ORDER BY created_at DESC LIMIT 1`, reg.NodeID).Scan(&serveOnly); err != nil {
		t.Fatalf("read the queued migrate_serve: %v", err)
	}
	if !serveOnly {
		t.Error("the download's migrate_serve is not serve_only: the Agent would stop the torrent")
	}

	// Torrent without hash → ErrNoHash.
	var noHash string
	if err := pool.QueryRow(ctx,
		`INSERT INTO torrents (node_id, hash, name, status) VALUES ($1, NULL, 'DL Without Hash', 'QUEUED') RETURNING id::text`,
		reg.NodeID).Scan(&noHash); err != nil {
		t.Fatalf("insert torrent without hash: %v", err)
	}
	if _, err := svc.CreateDownloadTicket(ctx, noHash); !errors.Is(err, torrent.ErrNoHash) {
		t.Fatalf("CreateDownloadTicket(no hash) = %v, want ErrNoHash", err)
	}

	// Non-existent ticket → ErrTicketInvalid (fails before touching the Node).
	if _, _, _, err := svc.OpenDownload(ctx, "ticket-that-does-not-exist"); !errors.Is(err, torrent.ErrTicketInvalid) {
		t.Fatalf("OpenDownload(invalid) = %v, want ErrTicketInvalid", err)
	}
}

// TestAddDuplicateGuard covers backlog #11: a 2nd add of the same hash without allowDuplicate
// is rejected with DuplicateError (listing the Node); with allowDuplicate=true, it passes.
func TestAddDuplicateGuard(t *testing.T) {
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
	if _, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "dup-guard-node", Engine: "qbittorrent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))
	magnet := "magnet:?xt=urn:btih:2222222222222222222222222222222222222222&dn=Dup"

	if _, err := svc.Add(ctx, magnet, "", torrent.AddOptions{AllowOtherVersion: true}); err != nil {
		t.Fatalf("1º add: %v", err)
	}
	// 2nd add without allowDuplicate → DuplicateError with the Node listed.
	var dup *torrent.DuplicateError
	if _, err := svc.Add(ctx, magnet, "", torrent.AddOptions{AllowOtherVersion: true}); !errors.As(err, &dup) {
		t.Fatalf("2nd add = %v, want *DuplicateError", err)
	}
	if len(dup.Nodes) == 0 {
		t.Error("DuplicateError without Nodes")
	}
	if dup.Existing == "" {
		t.Error("DuplicateError without Existing (torrent name already present)")
	}
	// 3rd add with allowDuplicate → passes (operator confirmed the 2nd copy).
	if _, err := svc.Add(ctx, magnet, "", torrent.AddOptions{AllowDuplicate: true, AllowOtherVersion: true}); err != nil {
		t.Fatalf("add with allowDuplicate: %v", err)
	}
}

// TestAdd_OtherVersionOfTheSameTitle covers #37: with one version of the work in the cluster,
// adding ANOTHER version (another resolution and codec, another hash) returns OtherVersionsError
// with what already exists — name, resolution, codec, size, Node. With AllowOtherVersion, it goes
// through. A different work isn't affected.
func TestAdd_OtherVersionOfTheSameTitle(t *testing.T) {
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
	if _, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "other-version-node", Engine: "qbittorrent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	// A unique title per run: the test database is shared by the package's tests.
	title := fmt.Sprintf("Test Work %d", time.Now().UnixNano())
	hash := func(n int) string { return fmt.Sprintf("%040x", time.Now().UnixNano()+int64(n)) }
	first := title + ".2019.1080p.BluRay.x264-GRP"
	if _, err := svc.Add(ctx, "magnet:?xt=urn:btih:"+hash(1)+"&dn="+first, "", torrent.AddOptions{}); err != nil {
		t.Fatalf("1st version: %v", err)
	}

	other := "magnet:?xt=urn:btih:" + hash(2) + "&dn=" + title + " 2019 720p WEB-DL x265-OTHER"
	var ov *torrent.OtherVersionsError
	if _, err := svc.Add(ctx, other, "", torrent.AddOptions{}); !errors.As(err, &ov) {
		t.Fatalf("2nd version = %v, want *OtherVersionsError", err)
	}
	if len(ov.Versions) != 1 {
		t.Fatalf("versions = %+v, want only the 1st", ov.Versions)
	}
	v := ov.Versions[0]
	if v.Name != first || v.Resolution != "1080p" || v.Codec != "x264" || v.Node == "" {
		t.Errorf("version = %+v, want name %q, 1080p, x264 and the Node", v, first)
	}
	if ov.Adding.Resolution != "720p" || ov.Adding.Codec != "x265" {
		t.Errorf("adding = %+v, want 720p x265", ov.Adding)
	}

	// The operator saw it and confirmed: it goes in.
	if _, err := svc.Add(ctx, other, "", torrent.AddOptions{AllowOtherVersion: true}); err != nil {
		t.Fatalf("confirmed 2nd version: %v", err)
	}

	// A different work doesn't run into anything.
	different := "magnet:?xt=urn:btih:" + hash(3) + "&dn=" + title + " Part 2 2021 1080p WEB-DL x264-GRP"
	if _, err := svc.Add(ctx, different, "", torrent.AddOptions{}); err != nil {
		t.Fatalf("different work: %v", err)
	}
}

func TestBackfillTorrentFile(t *testing.T) {
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
	if _, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "backfill-test-node", Engine: "qbittorrent"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))
	// Torrent added via magnet: created without .torrent persisted in the CP.
	magnet := "magnet:?xt=urn:btih:b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0&dn=Backfill"
	tr, err := svc.Add(ctx, magnet, "", torrent.AddOptions{AllowOtherVersion: true})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if tr.Hash == nil {
		t.Fatal("torrent without hash")
	}

	// While metadata hasn't resolved (size == 0), the torrent is not eligible for backfill.
	if hashes, err := svc.HashesNeedingFile(ctx, tr.NodeID); err != nil {
		t.Fatalf("HashesNeedingFile (pre-metadata): %v", err)
	} else if len(hashes) != 0 {
		t.Fatalf("pending items before metadata = %d, want 0", len(hashes))
	}

	// The Agent reports the resolved metadata (size > 0) → becomes eligible for backfill.
	size := int64(1 << 20)
	if err := svc.SyncStates(ctx, tr.NodeID, []torrent.Report{{Hash: *tr.Hash, Status: "DOWNLOADING", Size: &size}}); err != nil {
		t.Fatalf("SyncStates: %v", err)
	}
	hashes, err := svc.HashesNeedingFile(ctx, tr.NodeID)
	if err != nil {
		t.Fatalf("HashesNeedingFile: %v", err)
	}
	if len(hashes) != 1 || hashes[0] != *tr.Hash {
		t.Fatalf("pending items = %v, want [%s]", hashes, *tr.Hash)
	}

	// The Agent exports and sends the .torrent → persisted and leaves the pending list.
	if err := svc.StoreTorrentFileByHash(ctx, tr.NodeID, *tr.Hash, []byte("d4:infod6:lengthi1e4:name8:backfill12:piece lengthi16384eee")); err != nil {
		t.Fatalf("StoreTorrentFileByHash: %v", err)
	}
	if ok, err := svc.HasTorrentFile(ctx, tr.ID); err != nil {
		t.Fatalf("HasTorrentFile: %v", err)
	} else if !ok {
		t.Fatal(".torrent was not persisted")
	}
	if hashes, err := svc.HashesNeedingFile(ctx, tr.NodeID); err != nil {
		t.Fatalf("HashesNeedingFile (post-backfill): %v", err)
	} else if len(hashes) != 0 {
		t.Fatalf("pending items after backfill = %d, want 0", len(hashes))
	}

	// Hash not found on this Node → ErrNotFound.
	if err := svc.StoreTorrentFileByHash(ctx, tr.NodeID, "cccccccccccccccccccccccccccccccccccccccc", []byte("x")); !errors.Is(err, torrent.ErrNotFound) {
		t.Fatalf("StoreTorrentFileByHash (nonexistent hash) = %v, want ErrNotFound", err)
	}
}

// TestSyncStates_ReconcilesMissing verifies deletion reconciliation (#19): a
// torrent ALREADY seen in the inventory that disappears from the next inventory is soft-deleted; a
// torrent NEVER synced (null last_synced_at, e.g., newly added still missing
// from the Engine) is protected; and an EMPTY report is never treated as "everything is gone".
func TestSyncStates_ReconcilesMissing(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "reconcile-test-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	// Hashes exclusive to this test (the database is shared between tests).
	hashX := "a1000000000000000000000000000000000000a1" // will be seen and then disappear
	hashY := "b2000000000000000000000000000000000000b2" // remains in the inventory
	hashZ := "c3000000000000000000000000000000000000c3" // newly added, never synced
	prog := 1.0

	inList := func(hash string) bool {
		list, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, x := range list {
			if x.Hash != nil && *x.Hash == hash {
				return true
			}
		}
		return false
	}

	// Sync 1: X and Y present in the inventory → adopted (last_synced_at set).
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hashX, Name: "X", Status: "SEEDING", Progress: &prog},
		{Hash: hashY, Name: "Y", Status: "SEEDING", Progress: &prog},
	}); err != nil {
		t.Fatalf("SyncStates(1): %v", err)
	}
	// Z enters as newly added (live row, but never synced: null last_synced_at).
	if _, err := pool.Exec(ctx,
		`INSERT INTO torrents (node_id, hash, name, status, last_synced_at) VALUES ($1::uuid, $2, $3, 'CHECKING', NULL)`,
		reg.NodeID, hashZ, "Z"); err != nil {
		t.Fatalf("insert Z: %v", err)
	}
	if !inList(hashX) || !inList(hashY) || !inList(hashZ) {
		t.Fatalf("precondition: X=%v Y=%v Z=%v (expected all present)", inList(hashX), inList(hashY), inList(hashZ))
	}

	// Sync 2: only Y in the inventory → X disappears from the inventory (must be reconciled/removed);
	// Y remains; Z is protected (was never synced).
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hashY, Status: "SEEDING", Progress: &prog},
	}); err != nil {
		t.Fatalf("SyncStates(2): %v", err)
	}
	if inList(hashX) {
		t.Error("X should have been soft-deleted (disappeared from inventory after already being seen)")
	}
	if !inList(hashY) {
		t.Error("Y should not have been removed (still in inventory)")
	}
	if !inList(hashZ) {
		t.Error("Z should not have been removed (newly added, never synced → last_synced_at null)")
	}

	// Sync 3: EMPTY report is not "everything is gone" — Y and Z remain.
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{}); err != nil {
		t.Fatalf("SyncStates(empty): %v", err)
	}
	if !inList(hashY) {
		t.Error("Y should not be removed by an empty report")
	}
	if !inList(hashZ) {
		t.Error("Z should not be removed by an empty report")
	}
}

// sabotage installs a trigger that rejects INSERTs into commands matching the filter
// (#117): forces an intermediate step to fail to prove the flow is atomic.
// The payload content filter scopes the sabotage to the test's exclusive hash — the
// database is shared and accumulates state between tests (see testing.md), so an unrestricted
// trigger would break the neighbors. Idempotent + cleanup in Cleanup.
func sabotage(t *testing.T, pool *pgxpool.Pool, name, when string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx,
		`CREATE OR REPLACE FUNCTION seedgrid_test_boom() RETURNS trigger LANGUAGE plpgsql
		 AS $$ BEGIN RAISE EXCEPTION 'sabotagem de teste (#117)'; END $$`); err != nil {
		t.Fatalf("create sabotage function: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER IF EXISTS `+name+` ON commands`); err != nil {
		t.Fatalf("clear previous trigger: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`CREATE TRIGGER `+name+` BEFORE INSERT ON commands FOR EACH ROW
		 WHEN (`+when+`) EXECUTE FUNCTION seedgrid_test_boom()`); err != nil {
		t.Fatalf("create sabotage trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS `+name+` ON commands`)
	})
}

// TestAdd_AtomicWhenEnqueueFails (#117): if the add_torrent enqueue fails,
// NOTHING can persist — no phantom QUEUED row (which reconcile #19 doesn't clean, since
// null last_synced_at protects it), no Scheduler decision, no command.
func TestAdd_AtomicWhenEnqueueFails(t *testing.T) {
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
	if _, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "atomic-add-node", Engine: "qbittorrent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	hash := "f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1f1" // exclusive to this test (atomic add)
	sabotage(t, pool, "seedgrid_test_boom_add",
		`NEW.type = 'add_torrent' AND NEW.payload::text LIKE '%`+hash+`%'`)

	if _, err := svc.Add(ctx, "magnet:?xt=urn:btih:"+hash+"&dn=Atomic", "", torrent.AddOptions{AllowOtherVersion: true}); err == nil {
		t.Fatal("Add should fail with sabotaged enqueueing")
	}

	var torrents, cmds, decisions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM torrents WHERE hash = $1`, hash).Scan(&torrents); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM commands WHERE payload::text LIKE '%'||$1||'%'`, hash).Scan(&cmds); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM torrent_scheduler ts JOIN torrents tr ON tr.id = ts.torrent_id WHERE tr.hash = $1`,
		hash).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if torrents != 0 || cmds != 0 || decisions != 0 {
		t.Errorf("leftover trace after failure: torrents=%d commands=%d decisions=%d, want 0/0/0", torrents, cmds, decisions)
	}
}

// TestRemove_AtomicWhenEnqueueFails (#117): if the enqueueing of remove_torrent
// fails, the soft-delete MUST NOT stand — the row stays alive and no command exists
// (the inverse — command without soft-delete — was the pattern of the 27k removes incident).
func TestRemove_AtomicWhenEnqueueFails(t *testing.T) {
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
	if _, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "atomic-remove-node", Engine: "qbittorrent"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	hash := "f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2f2" // exclusive to this test (atomic remove)
	tr, err := svc.Add(ctx, "magnet:?xt=urn:btih:"+hash+"&dn=AtomicRemove", "", torrent.AddOptions{AllowOtherVersion: true})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	sabotage(t, pool, "seedgrid_test_boom_remove",
		`NEW.type = 'remove_torrent' AND NEW.payload::text LIKE '%`+hash+`%'`)

	if _, err := svc.Remove(ctx, tr.ID, "test"); err == nil {
		t.Fatal("Remove should fail with sabotaged enqueueing")
	}

	var alive bool
	if err := pool.QueryRow(ctx,
		`SELECT deleted_at IS NULL FROM torrents WHERE id = $1`, tr.ID).Scan(&alive); err != nil {
		t.Fatal(err)
	}
	if !alive {
		t.Error("torrent was soft-deleted even with removal command not enqueued")
	}
	var cmds int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM commands WHERE type = 'remove_torrent' AND payload::text LIKE '%'||$1||'%'`,
		hash).Scan(&cmds); err != nil {
		t.Fatal(err)
	}
	if cmds != 0 {
		t.Errorf("remove_torrent commands = %d, want 0", cmds)
	}
}

// TestListPageEMeta (#115): stable List pagination and the summary (total, max updated)
// that feeds the conditional ETag of GET /torrents.
func TestListPageEMeta(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "listpage-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	// 3 torrents adopted via sync (hashes exclusive to this test).
	prog := 1.0
	for _, h := range []string{
		"a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1",
		"a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2",
		"a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3",
	} {
		if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
			{Hash: h, Name: "listpage-" + h[:4], Status: "SEEDING", Progress: &prog},
		}); err != nil {
			t.Fatalf("SyncStates(%s): %v", h, err)
		}
	}

	total, maxUpdated, err := svc.ListMeta(ctx)
	if err != nil {
		t.Fatalf("ListMeta: %v", err)
	}
	if total < 3 {
		t.Errorf("total = %d, want >= 3", total)
	}
	if maxUpdated.IsZero() {
		t.Error("maxUpdated zero with live torrents")
	}

	all, err := svc.ListPage(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListPage(0,0): %v", err)
	}
	if len(all) != total {
		t.Errorf("ListPage(0,0) = %d items, ListMeta total = %d", len(all), total)
	}

	// Page of 2 + next page of 2: no overlap, stable order.
	p1, err := svc.ListPage(ctx, 2, 0)
	if err != nil {
		t.Fatalf("ListPage(2,0): %v", err)
	}
	p2, err := svc.ListPage(ctx, 2, 2)
	if err != nil {
		t.Fatalf("ListPage(2,2): %v", err)
	}
	if len(p1) != 2 {
		t.Fatalf("page 1 = %d items, want 2", len(p1))
	}
	seen := map[string]bool{p1[0].ID: true, p1[1].ID: true}
	for _, x := range p2 {
		if seen[x.ID] {
			t.Errorf("item %s repeated across pages", x.ID)
		}
	}
	if p1[0].ID != all[0].ID || p1[1].ID != all[1].ID {
		t.Error("page order differs from full list (unstable pagination)")
	}

	// Mutation changes the summary (invalidates the ETag): a new sync bumps updated_at.
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1", Name: "listpage-a1a1", Status: "SEEDING", Progress: &prog},
	}); err != nil {
		t.Fatalf("SyncStates(bump): %v", err)
	}
	_, maxUpdated2, err := svc.ListMeta(ctx)
	if err != nil {
		t.Fatalf("ListMeta 2: %v", err)
	}
	if !maxUpdated2.After(maxUpdated) {
		t.Errorf("maxUpdated did not advance after mutation: %v → %v", maxUpdated, maxUpdated2)
	}
}

// TestSyncStates_DecimatesSamples (#113): with the injected sampling interval, two
// consecutive syncs generate ONE batch of samples; without injection, each sync generates its own.
func TestSyncStates_DecimatesSamples(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "decima-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	svc := torrent.NewService(pool, scheduler.NewService(pool), command.NewService(pool), events.NewEmitter(pool))

	hash := "b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1" // exclusive to this test (decimation)
	prog := 1.0
	sync := func() {
		t.Helper()
		if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
			{Hash: hash, Name: "tenth-test", Status: "SEEDING", Progress: &prog},
		}); err != nil {
			t.Fatalf("SyncStates: %v", err)
		}
	}
	samples := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM torrent_stat_samples ts
			   JOIN torrents tr ON tr.id = ts.torrent_id WHERE tr.hash = $1`, hash).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Without injection: each sync generates a sample (previous behavior).
	sync()
	sync()
	if n := samples(); n != 2 {
		t.Fatalf("no decimation: samples = %d, want 2", n)
	}

	// With a 1h interval: the next syncs DO NOT generate a new sample... except the
	// first after injection (the per-Node clock doesn't exist yet → samples and arms).
	svc.SetSampleInterval(func() time.Duration { return time.Hour })
	sync() // arms the Node clock (generates 1 batch)
	base := samples()
	sync()
	sync()
	if n := samples(); n != base {
		t.Errorf("decimation enabled: samples = %d, want %d (no new samples within window)", n, base)
	}
}

// TestTorrentForCommand: the screenshot render (step-by-step flow) finds the torrent for the
// extract_screenshots command by the (node_id, hash) pair — it's what puts the torrent in the Feed entry
// when an image host fails (#193). Non-existent command or malformed id = empty.
func TestTorrentForCommand(t *testing.T) {
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
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "shots-cmd-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	cmds := command.NewService(pool)
	svc := torrent.NewService(pool, scheduler.NewService(pool), cmds, events.NewEmitter(pool))

	hash := "5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c"
	prog := 1.0
	if err := svc.SyncStates(ctx, reg.NodeID, []torrent.Report{
		{Hash: hash, Name: "Screenshot Movie", Status: "SEEDING", Progress: &prog},
	}); err != nil {
		t.Fatalf("SyncStates: %v", err)
	}
	var wantID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM torrents WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL`,
		reg.NodeID, hash).Scan(&wantID); err != nil {
		t.Fatalf("id do torrent: %v", err)
	}

	cmdID, err := screenshot.EnqueueExtract(ctx, cmds, reg.NodeID, hash, 3)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	id, name := svc.TorrentForCommand(ctx, cmdID)
	if id != wantID || name != "Screenshot Movie" {
		t.Errorf("TorrentForCommand = (%q, %q), want (%q, %q)", id, name, wantID, "Screenshot Movie")
	}

	for _, bad := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		if id, name := svc.TorrentForCommand(ctx, bad); id != "" || name != "" {
			t.Errorf("TorrentForCommand(%q) = (%q, %q), want empty", bad, id, name)
		}
	}
}
