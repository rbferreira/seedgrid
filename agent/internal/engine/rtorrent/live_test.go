package rtorrent

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
	"github.com/rbferreira/seedgrid/agent/internal/engine/queue"
	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
	"github.com/rbferreira/seedgrid/agent/internal/torrentfile"
)

// TestLive_ReadSide reads a real rTorrent when SEEDGRID_TEST_RTORRENT_URL points at one (the bench,
// ADR-045): the fake can only answer what we believed rTorrent says.
func TestLive_ReadSide(t *testing.T) {
	u := os.Getenv("SEEDGRID_TEST_RTORRENT_URL")
	if u == "" {
		t.Skip("SEEDGRID_TEST_RTORRENT_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a, err := New(u)
	if err != nil {
		t.Fatal(err)
	}
	v, err := a.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	t.Logf("rTorrent %s", v)
	ts, err := a.Torrents(ctx)
	if err != nil {
		t.Fatalf("Torrents: %v", err)
	}
	hashRe := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, tr := range ts {
		t.Logf("%+v", tr)
		if !hashRe.MatchString(tr.Hash) || tr.Status == "" || tr.Size <= 0 || tr.ContentPath == "" {
			t.Errorf("implausible torrent: %+v", tr)
		}
		p, st, err := a.State(ctx, tr.Hash)
		if err != nil || st != tr.Status || p != tr.Progress {
			t.Errorf("State(%s) = %v %q %v, want %v %q", tr.Hash, p, st, err, tr.Progress, tr.Status)
		}
	}
	vs, err := a.TrackerVerdicts(ctx)
	if err != nil {
		t.Fatalf("TrackerVerdicts: %v", err)
	}
	t.Logf("verdicts: %+v", vs)
	n, err := a.IncomingPeers(ctx)
	if err != nil {
		t.Fatalf("IncomingPeers: %v", err)
	}
	t.Logf("incoming peers: %d", n)
}

// TestLive_WriteSide stops, starts and exports SEEDGRID_TEST_RTORRENT_HASH on a real rTorrent: the
// torrent ends started, and the export's info dictionary hashes to the torrent's hash.
func TestLive_WriteSide(t *testing.T) {
	u, hash := os.Getenv("SEEDGRID_TEST_RTORRENT_URL"), os.Getenv("SEEDGRID_TEST_RTORRENT_HASH")
	if u == "" || hash == "" {
		t.Skip("SEEDGRID_TEST_RTORRENT_URL and SEEDGRID_TEST_RTORRENT_HASH not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a, err := New(u)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(ctx, hash); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, st, _ := a.State(ctx, hash); st != "PAUSED" {
		t.Errorf("after Stop: %s, want PAUSED", st)
	}
	if err := a.Start(ctx, hash); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, st, _ := a.State(ctx, hash); st != "SEEDING" && st != "DOWNLOADING" && st != "CHECKING" {
		t.Errorf("after Start: %s, want it running", st)
	}
	data, err := a.Export(ctx, hash)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got, _ := torrentfile.InfoHash(data); got != strings.ToLower(hash) {
		t.Errorf("exported info hash %s, want %s", got, hash)
	}
	if bytes.Contains(data, []byte("libtorrent_resume")) || bytes.Contains(data, []byte("8:rtorrent")) {
		t.Error("the export still has rTorrent's keys")
	}
}

// TestLive_Queue runs the Agent's queue on a real rTorrent over the two incomplete torrents of
// SEEDGRID_TEST_RTORRENT_DOWNLOADS (oldest first): with one download allowed the newer is queued,
// with two it runs again. Both end stopped.
func TestLive_Queue(t *testing.T) {
	u, dls := os.Getenv("SEEDGRID_TEST_RTORRENT_URL"), strings.Split(os.Getenv("SEEDGRID_TEST_RTORRENT_DOWNLOADS"), ",")
	if u == "" || len(dls) != 2 {
		t.Skip("SEEDGRID_TEST_RTORRENT_URL and SEEDGRID_TEST_RTORRENT_DOWNLOADS (two hashes) not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a, err := New(u)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, h := range dls {
			_ = a.Stop(context.Background(), h)
		}
	}()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	q := queue.New()
	for _, h := range dls {
		if err := a.Start(ctx, h); err != nil {
			t.Fatalf("Start %s: %v", h, err)
		}
	}
	one := engine.Config{QueueingEnabled: true, MaxActiveDownloads: 1, MaxActiveUploads: -1, MaxActiveTorrents: -1}
	_ = a.ConfigureQueue(ctx, one)
	if err := a.queueRound(ctx, q, time.Now(), log); err != nil {
		t.Fatalf("round: %v", err)
	}
	_, older, _ := a.State(ctx, dls[0])
	_, newer, _ := a.State(ctx, dls[1])
	if older != "DOWNLOADING" || newer != "QUEUED" {
		t.Errorf("one download allowed: %s %s, want DOWNLOADING QUEUED", older, newer)
	}
	two := one
	two.MaxActiveDownloads = 2
	_ = a.ConfigureQueue(ctx, two)
	if err := a.queueRound(ctx, q, time.Now(), log); err != nil {
		t.Fatalf("round: %v", err)
	}
	if _, st, _ := a.State(ctx, dls[1]); st != "DOWNLOADING" {
		t.Errorf("two downloads allowed: %s, want DOWNLOADING", st)
	}
}

// TestLive_CreateTorrent publishes a folder of generated data on a real rTorrent: the Agent creates
// the .torrent, rTorrent checks the data against it and seeds. SEEDGRID_TEST_RTORRENT_DATA is a
// host folder rTorrent sees through SEEDGRID_TEST_RTORRENT_PATHMAP.
func TestLive_CreateTorrent(t *testing.T) {
	u, dir := os.Getenv("SEEDGRID_TEST_RTORRENT_URL"), os.Getenv("SEEDGRID_TEST_RTORRENT_DATA")
	if u == "" || dir == "" {
		t.Skip("SEEDGRID_TEST_RTORRENT_URL and SEEDGRID_TEST_RTORRENT_DATA not set")
	}
	pm, err := pathmap.Parse(os.Getenv("SEEDGRID_TEST_RTORRENT_PATHMAP"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	a, err := New(u)
	if err != nil {
		t.Fatal(err)
	}
	a.WithPaths(pm)
	root := filepath.Join(dir, fmt.Sprintf("sg-create-%d", time.Now().UnixNano()))
	chunk := bytes.Repeat([]byte("seedgrid"), 300000) // 2.4 MB: pieces span the files
	for i, name := range []string{"a.bin", "sub/b.bin", "sub/c.txt"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // G301: rTorrent runs as another user and must read it
			t.Fatal(err)
		}
		if err := os.WriteFile(p, chunk[:len(chunk)-i*1000], 0o644); err != nil { //nolint:gosec // G306: rTorrent runs as another user and must read it
			t.Fatal(err)
		}
	}
	// A tracker that answers nothing: rTorrent 0.9.8 drops a .torrent without one.
	data, err := a.CreateTorrent(ctx, root, []string{"http://127.0.0.1:1/announce"}, true, "SGTEST")
	if err != nil {
		t.Fatalf("CreateTorrent: %v", err)
	}
	hash, _ := torrentfile.InfoHash(data)
	defer func() {
		if !t.Failed() {
			_ = a.Delete(context.Background(), hash, true)
		}
	}()
	for {
		if rows, err := a.rows(ctx, false); err == nil {
			for _, r := range rows {
				if r.hash == hash {
					t.Logf("%+v", r)
				}
			}
		}
		_, st, err := a.State(ctx, hash)
		if err != nil {
			t.Fatalf("State: %v", err)
		}
		if st == "SEEDING" {
			break
		}
		if st == "ERROR" { // stopped for a moment before the check starts: not a verdict
			t.Fatalf("after creation: %s, want it checked and seeding", st)
		}
		select {
		case <-ctx.Done():
			t.Fatal("rTorrent never seeded the created torrent: the data does not match it, or is not where it looked")
		case <-time.After(time.Second):
		}
	}
	if cp, _ := a.ContentPathOf(ctx, hash); cp != root {
		t.Errorf("content path %q, want %q", cp, root)
	}
}
