package rtorrent

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
)

// TestCodec_RoundTrip: what the encoder writes, the decoder reads back the same.
func TestCodec_RoundTrip(t *testing.T) {
	in := []any{
		"a <b> & c", int64(7), int64(1 << 40), true, false,
		[]any{"x", int64(-1)},
		map[string]any{"methodName": "d.name", "params": []any{"ABC"}},
	}
	body, err := encodeCall("m", in...)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(body, []byte("<i8>1099511627776</i8>")) || !bytes.Contains(body, []byte("<i4>7</i4>")) {
		t.Errorf("ints should be i4 when they fit and i8 otherwise: %s", body)
	}
	method, out := decodeCall(t, body)
	if method != "m" || !reflect.DeepEqual(out, in) {
		t.Errorf("round trip: %s %#v, want m %#v", method, out, in)
	}
}

// TestDecode_RTorrentShapes: an untyped value is a string, i8 and double are read, and a fault
// comes back as *Fault.
func TestDecode_RTorrentShapes(t *testing.T) {
	v, err := decodeResponse(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<methodResponse><params><param><value><array><data>
 <value>0.9.8</value><value><i8>12</i8></value><value><double>1.5</double></value><value><string/></value>
</data></array></value></param></params></methodResponse>`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if want := []any{"0.9.8", int64(12), 1.5, ""}; !reflect.DeepEqual(v, want) {
		t.Errorf("decode = %#v, want %#v", v, want)
	}

	_, err = decodeResponse(strings.NewReader(`<methodResponse><fault><value><struct>
<member><name>faultCode</name><value><i4>-501</i4></value></member>
<member><name>faultString</name><value><string>Could not find info-hash.</string></value></member>
</struct></value></fault></methodResponse>`))
	var f *Fault
	if !errors.As(err, &f) || f.Code != -501 || f.Message != "Could not find info-hash." {
		t.Errorf("fault = %v, want -501 Could not find info-hash.", err)
	}
}

func TestNewClient_URLs(t *testing.T) {
	for _, c := range []struct{ url, network, addr string }{
		{"scgi:///home/u/.rtorrent/rpc.socket", "unix", "/home/u/.rtorrent/rpc.socket"},
		{"scgi://127.0.0.1:5000", "tcp", "127.0.0.1:5000"},
	} {
		cl, err := newClient(c.url)
		if err != nil || cl.network != c.network || cl.addr != c.addr {
			t.Errorf("newClient(%q) = %+v, %v", c.url, cl, err)
		}
	}
	cl, err := newClient("https://u:p%40ss@box.example/RPC2")
	if err != nil || cl.httpURL != "https://box.example/RPC2" || cl.user != "u" || cl.pass != "p@ss" {
		t.Errorf("https URL = %+v, %v; want the credentials out of the URL", cl, err)
	}
	for _, bad := range []string{"scgi://", "ftp://x/RPC2", "127.0.0.1:5000"} {
		if _, err := newClient(bad); err == nil {
			t.Errorf("newClient(%q) should fail", bad)
		}
	}
}

// torrentRow builds a d.multicall2 row in the order of fields.
func torrentRow(hash, name string, state, active, complete, hashing int, message string, size, done int64,
	down, up int64, peersComplete, peersConnected int, dir string, multi int, custom string) []any {
	return []any{hash, name, state, active, complete, hashing, message, size, done, down, up,
		1500, done, up * 10, peersComplete, peersConnected, dir, multi, custom}
}

func fakeWithTorrents(rows ...[]any) *fakeRTorrent {
	f := newFake()
	f.on("d.multicall2", func(params []any) (any, error) {
		if len(params) != 2+len(fields) || params[0] != "" || params[1] != "main" {
			return nil, errors.New("bad d.multicall2 params")
		}
		out := make([]any, len(rows))
		for i, r := range rows {
			out[i] = r
		}
		return out, nil
	})
	f.on("t.multicall", func(params []any) (any, error) {
		switch params[0] {
		case "AAAA":
			return []any{
				[]any{1, 40, 7, 3},
				[]any{0, 900, 900, 0}, // disabled: ignored
			}, nil
		case "BBBB":
			return []any{[]any{1, 0, 0, 0}}, nil
		}
		return nil, errors.New("could not find info-hash")
	})
	return f
}

// TestTorrents_OverSCGI reads torrents through a Unix socket and maps them to SeedGrid's view.
func TestTorrents_OverSCGI(t *testing.T) {
	f := fakeWithTorrents(
		torrentRow("AAAA", "debian.iso", 1, 1, 0, 0, "", 1000, 250, 50, 10, 2, 5, "/downloads", 0, ""),
		torrentRow("BBBB", "album", 1, 1, 1, 0, "", 500, 500, 0, 30, 0, 0, "/downloads/album", 1, ""),
	)
	pm, err := pathmap.Parse("/downloads=/srv/torrents")
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(f.serveSCGI(t))
	if err != nil {
		t.Fatal(err)
	}
	a.WithPaths(pm)
	got, err := a.Torrents(context.Background())
	if err != nil {
		t.Fatalf("Torrents: %v", err)
	}
	want := []engine.Torrent{{
		Hash: "aaaa", Name: "debian.iso", Status: "DOWNLOADING", Progress: 0.25,
		NumSeeds: 40, NumLeechs: 7, NumSeedsConnected: 2, NumLeechsConnected: 3,
		DlSpeed: 50, UpSpeed: 10, Size: 1000, Ratio: 1.5, ETA: 15, Downloaded: 250, Uploaded: 100,
		ContentPath: "/srv/torrents/debian.iso",
	}, {
		Hash: "bbbb", Name: "album", Status: "SEEDING", Progress: 1,
		DlSpeed: 0, UpSpeed: 30, Size: 500, Ratio: 1.5, ETA: etaInfinite, Downloaded: 500, Uploaded: 300,
		ContentPath: "/srv/torrents/album",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Torrents =\n%+v\nwant\n%+v", got, want)
	}

	p, st, err := a.State(context.Background(), "AAAA")
	if err != nil || p != 0.25 || st != "DOWNLOADING" {
		t.Errorf("State = %v %q %v", p, st, err)
	}
	if p, st, err := a.State(context.Background(), "cccc"); err != nil || p != 0 || st != "" {
		t.Errorf("State of a hash rTorrent does not hold = %v %q %v, want 0 \"\" nil", p, st, err)
	}
	if cp, err := a.ContentPathOf(context.Background(), "bbbb"); err != nil || cp != "/srv/torrents/album" {
		t.Errorf("ContentPathOf = %q %v", cp, err)
	}
}

// TestVersion_OverHTTP goes through a web server's /RPC2 with basic auth.
func TestVersion_OverHTTP(t *testing.T) {
	f := newFake()
	f.on("system.client_version", func([]any) (any, error) { return "0.9.8", nil })
	a, err := New(f.serveHTTP(t, "op", "secret"))
	if err != nil {
		t.Fatal(err)
	}
	if v, err := a.Version(context.Background()); err != nil || v != "0.9.8" {
		t.Errorf("Version = %q %v", v, err)
	}

	wrong, _ := New(strings.Replace(f.serveHTTP(t, "op", "secret"), "op:secret", "op:nope", 1))
	if _, err := wrong.Version(context.Background()); err == nil {
		t.Error("a refused login must be an error: the Engine is unreachable for the Agent")
	}
}

func TestMapState(t *testing.T) {
	cases := []struct {
		name string
		r    row
		want string
	}{
		{"hashing", row{started: true, active: true, hashing: 1}, "CHECKING"},
		{"hashing a stopped torrent", row{hashing: 3}, "CHECKING"},
		{"storage error", row{started: true, active: true, message: "Storage error: [File not found]"}, "ERROR"},
		{"tracker message is not the torrent's state", row{started: true, active: true, complete: true, message: `Tracker: [Failure reason "Unregistered torrent"]`}, "SEEDING"},
		{"stopped and complete", row{complete: true}, "PAUSED"},
		{"stopped by the Agent's queue", row{custom: queuedMark}, "QUEUED"},
		{"stopped by the operator", row{}, "PAUSED"},
		{"complete keeps no queue mark", row{complete: true, custom: queuedMark}, "PAUSED"},
		{"paused (started, not active)", row{started: true}, "PAUSED"},
		{"seeding", row{started: true, active: true, complete: true}, "SEEDING"},
		{"downloading", row{started: true, active: true}, "DOWNLOADING"},
	}
	for _, c := range cases {
		if got := mapState(c.r); got != c.want {
			t.Errorf("%s: mapState = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestTrackerVerdicts(t *testing.T) {
	f := fakeWithTorrents(
		torrentRow("AAAA", "ok", 1, 1, 1, 0, "", 1, 1, 0, 0, 0, 0, "/d", 0, ""),
		torrentRow("BBBB", "never answered", 1, 1, 1, 0, "", 1, 1, 0, 0, 0, 0, "/d", 0, ""),
		torrentRow("CCCC", "deleted", 1, 1, 1, 0, `Tracker: [Failure reason "Torrent has been deleted."]`, 1, 1, 0, 0, 0, 0, "/d", 0, ""),
		torrentRow("DDDD", "timeout", 1, 1, 1, 0, "Tracker: [Timeout was reached]", 1, 1, 0, 0, 0, 0, "/d", 0, ""),
	)
	a, _ := New(f.serveSCGI(t))
	got, err := a.TrackerVerdicts(context.Background())
	if err != nil {
		t.Fatalf("TrackerVerdicts: %v", err)
	}
	want := map[string]engine.TrackerVerdict{
		"aaaa": {Status: trackerWorking},
		"bbbb": {Status: trackerNotContacted},
		"cccc": {Status: trackerNotWorking, Message: "Torrent has been deleted."},
		// CCCC and DDDD have no trackers in the fake (t.multicall faults): the message still counts.
		"dddd": {Status: trackerNotWorking, Message: "Timeout was reached"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("TrackerVerdicts = %+v, want %+v", got, want)
	}
}

func TestSlotLimit(t *testing.T) {
	f := fakeWithTorrents(
		torrentRow("AAAA", "a", 1, 1, 0, 0, "", 1, 0, 0, 0, 0, 0, "/d", 0, ""),
		torrentRow("BBBB", "b", 1, 1, 0, 0, `Tracker: [Failure reason "Your slot limit is reached! You may at most download 3 torrents at the same time"]`, 1, 0, 0, 0, 0, 0, "/d", 0, ""),
	)
	a, _ := New(f.serveSCGI(t))
	if n, ok := a.SlotLimit(context.Background(), []string{"aaaa", "bbbb"}); !ok || n != 3 {
		t.Errorf("SlotLimit = %d %t, want 3 true", n, ok)
	}
	if _, ok := a.SlotLimit(context.Background(), []string{"aaaa"}); ok {
		t.Error("SlotLimit must only read the torrents it is given")
	}
}

func TestIncomingPeers(t *testing.T) {
	f := fakeWithTorrents(
		torrentRow("AAAA", "a", 1, 1, 1, 0, "", 1, 1, 0, 0, 1, 3, "/d", 0, ""),
		torrentRow("BBBB", "b", 1, 1, 1, 0, "", 1, 1, 0, 0, 0, 0, "/d", 0, ""),
	)
	f.on("p.multicall", func(params []any) (any, error) {
		if params[0] != "AAAA" {
			return nil, errors.New("only torrents with peers are asked")
		}
		return []any{[]any{1}, []any{0}, []any{1}}, nil
	})
	a, _ := New(f.serveSCGI(t))
	if n, err := a.IncomingPeers(context.Background()); err != nil || n != 2 {
		t.Errorf("IncomingPeers = %d %v, want 2", n, err)
	}
	for _, c := range f.calls {
		if c == "p.multicall" {
			return
		}
	}
	t.Error("p.multicall was not called")
}

func TestConnectionStatus_Unknown(t *testing.T) {
	a, _ := New("scgi://127.0.0.1:1")
	if s, err := a.ConnectionStatus(context.Background()); s != "" || err != nil {
		t.Errorf("ConnectionStatus = %q %v, want \"\" nil: rTorrent has no verdict", s, err)
	}
}
