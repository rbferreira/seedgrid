package rtorrent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
)

// record makes the fake answer method with 0 and remember its params.
func record(f *fakeRTorrent, got *[][]any, methods ...string) {
	for _, m := range methods {
		f.on(m, func(params []any) (any, error) {
			*got = append(*got, append([]any{m}, params...))
			return 0, nil
		})
	}
}

// TestStopStart: the hash goes uppercase, Stop also closes the files, both clear the queue mark,
// and Start does not fail on the d.open rTorrent refuses for an open torrent.
func TestStopStart(t *testing.T) {
	f := newFake()
	var got [][]any
	record(f, &got, "d.custom.set", "d.stop", "d.close", "d.start", "d.resume")
	f.on("d.open", func([]any) (any, error) { return nil, errors.New("already open") })
	a, _ := New(f.serveSCGI(t))

	if err := a.Stop(context.Background(), "abcd"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := a.Start(context.Background(), "abcd"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	want := [][]any{
		{"d.custom.set", "ABCD", customKey, ""}, {"d.stop", "ABCD"}, {"d.close", "ABCD"},
		{"d.custom.set", "ABCD", customKey, ""}, {"d.start", "ABCD"}, {"d.resume", "ABCD"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}

	f.on("d.stop", func([]any) (any, error) { return nil, errors.New("could not find info-hash") })
	if err := a.Stop(context.Background(), "ffff"); err == nil {
		t.Error("Stop of a hash rTorrent does not hold must fail")
	}
}

// TestAddFileTo: the data is where the migration put it, as rTorrent sees that folder.
func TestAddFileTo(t *testing.T) {
	f := newFake()
	var got [][]any
	record(f, &got, "load.raw_start")
	f.on("d.hash", func(p []any) (any, error) { return p[0], nil })
	pm, _ := pathmap.Parse("/downloads=/srv/torrents")
	a, _ := New(f.serveSCGI(t))
	a.WithPaths(pm)
	if err := a.AddFileTo(context.Background(), []byte("d4:infod4:name1:xee"), "/srv/torrents/movies"); err != nil {
		t.Fatalf("AddFileTo: %v", err)
	}
	want := [][]any{{"load.raw_start", "", []byte("d4:infod4:name1:xee"), "d.directory.set=/downloads/movies"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %#v, want %#v", got, want)
	}
}

// TestDelete_Files: rTorrent lets go of the torrent, then the Agent removes the data; a path that
// cannot be a torrent's data is refused.
func TestDelete_Files(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "album")
	if err := os.MkdirAll(filepath.Join(data, "cd1"), 0o750); err != nil {
		t.Fatal(err)
	}
	f := fakeWithTorrents(torrentRow("AAAA", "album", 1, 1, 1, 0, "", 1, 1, 0, 0, 0, 0, data, 1, ""))
	var got [][]any
	record(f, &got, "d.erase")
	a, _ := New(f.serveSCGI(t))
	if err := a.Delete(context.Background(), "aaaa", true); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Errorf("data still there: %v", err)
	}
	if len(got) != 1 || got[0][1] != "AAAA" {
		t.Errorf("d.erase calls = %v", got)
	}

	f2 := fakeWithTorrents(torrentRow("AAAA", "x", 1, 1, 1, 0, "", 1, 1, 0, 0, 0, 0, "/", 1, ""))
	record(f2, &got, "d.erase")
	a2, _ := New(f2.serveSCGI(t))
	got = nil
	if err := a2.Delete(context.Background(), "aaaa", true); err == nil {
		t.Error("deleting / must be refused")
	}
	if len(got) != 0 {
		t.Errorf("a refused delete must leave the torrent in rTorrent: d.erase was called %v", got)
	}
}

// TestExport_FromSession: the .torrent comes from the session folder, without rTorrent's keys and
// with the info dictionary byte for byte.
func TestExport_FromSession(t *testing.T) {
	session := t.TempDir()
	info := "d6:lengthi5e4:name5:a.iso12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaae"
	stored := "d8:announce9:http://t/7:comment1:c4:info" + info + "17:libtorrent_resumed5:fileslee8:rtorrentd5:statei1eee"
	if err := os.WriteFile(filepath.Join(session, "ABCD.torrent"), []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFake()
	f.on("session.path", func([]any) (any, error) { return "/session/", nil })
	pm, _ := pathmap.Parse("/session=" + session)
	a, _ := New(f.serveSCGI(t))
	a.WithPaths(pm)
	got, err := a.Export(context.Background(), "abcd")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if want := "d8:announce9:http://t/7:comment1:c4:info" + info + "e"; string(got) != want {
		t.Errorf("Export =\n%s\nwant\n%s", got, want)
	}
}

// TestConfigure: the machine's port and folder go to rTorrent; the queue stays with the Agent, and
// ReadQueue reports it off with the limits the Agent starts from (ADR-045 §8).
func TestConfigure(t *testing.T) {
	f := newFake()
	var got [][]any
	// The pre-0.16 names: the new ones are not defined in the fake, as in rTorrent 0.9.8.
	record(f, &got, "network.port_random.set", "network.port_range.set", "directory.default.set")
	pm, _ := pathmap.Parse("/downloads=/srv/torrents")
	a, _ := New(f.serveSCGI(t))
	a.WithPaths(pm).WithQueue(engine.Config{QueueingEnabled: true, MaxActiveDownloads: 2, MaxActiveCheckingTorrents: 1})

	q, _ := a.ReadQueue(context.Background())
	if q.QueueingEnabled || q.MaxActiveDownloads != 2 || q.MaxActiveCheckingTorrents != 1 {
		t.Errorf("ReadQueue = %+v, want the baseline with the queue off", q)
	}
	c := engine.Config{BTPort: 51413, SavePath: "/srv/torrents", QueueingEnabled: true, MaxActiveDownloads: 5}
	if err := a.Configure(context.Background(), c); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	want := [][]any{
		{"network.port_random.set", "", int64(0)},
		{"network.port_range.set", "", "51413-51413"},
		{"directory.default.set", "", "/downloads"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if a.queue.cfg.MaxActiveDownloads != 5 || !a.queue.cfg.QueueingEnabled {
		t.Errorf("queue = %+v, want what Configure was given", a.queue.cfg)
	}
}

// TestShutdown: rTorrent dropping the connection as it quits is success.
func TestShutdown(t *testing.T) {
	a, _ := New("scgi://" + filepath.Join(t.TempDir(), "gone.socket"))
	if err := a.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown with rTorrent gone = %v, want nil", err)
	}
	f := newFake()
	f.on("system.shutdown.normal", func([]any) (any, error) { return nil, errors.New("not allowed") })
	a, _ = New(f.serveSCGI(t))
	if err := a.Shutdown(context.Background()); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("Shutdown refused = %v, want the fault", err)
	}
}

// TestCreateTorrent: the Agent creates the .torrent and rTorrent loads it started, pointing at the
// folder that holds the data, as rTorrent sees it.
func TestCreateTorrent(t *testing.T) {
	host := t.TempDir()
	if err := os.WriteFile(filepath.Join(host, "show.mkv"), []byte("episode"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFake()
	var got [][]any
	record(f, &got, "load.raw_start")
	f.on("d.hash", func(p []any) (any, error) { return p[0], nil })
	pm, _ := pathmap.Parse("/downloads=" + host)
	a, _ := New(f.serveSCGI(t))
	a.WithPaths(pm)
	data, err := a.CreateTorrent(context.Background(), filepath.Join(host, "show.mkv"), []string{"https://t/a"}, true, "SGX")
	if err != nil {
		t.Fatalf("CreateTorrent: %v", err)
	}
	if !strings.Contains(string(data), "4:name8:show.mkv") || !strings.Contains(string(data), "7:privatei1e6:source3:SGX") {
		t.Errorf(".torrent = %q", data)
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0][2], data) || got[0][3] != "d.directory.set=/downloads" {
		t.Errorf("calls = %v, want the .torrent loaded into /downloads", got)
	}
}

// TestLoadRaw_Dropped: a .torrent rTorrent answers 0 to but does not keep is an error, not a
// torrent that silently never appears.
func TestLoadRaw_Dropped(t *testing.T) {
	f := newFake()
	var got [][]any
	record(f, &got, "load.raw_start")
	f.on("d.hash", func([]any) (any, error) { return nil, errors.New("could not find info-hash") })
	a, _ := New(f.serveSCGI(t))
	err := a.AddFile(context.Background(), []byte("d4:infod4:name1:xee"))
	if err == nil || !strings.Contains(err.Error(), "was not taken") {
		t.Errorf("AddFile of a dropped .torrent = %v, want it reported", err)
	}
}
