package rtorrent

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"
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
