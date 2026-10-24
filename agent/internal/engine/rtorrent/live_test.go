package rtorrent

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // G505: the BitTorrent v1 info hash is SHA-1
	"fmt"
	"os"
	"regexp"
	"strings"
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
	if got := infoHash(t, data); got != strings.ToLower(hash) {
		t.Errorf("exported info hash %s, want %s", got, hash)
	}
	if bytes.Contains(data, []byte("libtorrent_resume")) || bytes.Contains(data, []byte("8:rtorrent")) {
		t.Error("the export still has rTorrent's keys")
	}
}

// infoHash is the SHA-1 of a .torrent's info dictionary, as it is in the file.
func infoHash(t *testing.T, data []byte) string {
	t.Helper()
	i := 1
	for i < len(data) && data[i] != 'e' {
		keyEnd, err := skipValue(data, i)
		if err != nil {
			t.Fatal(err)
		}
		valEnd, err := skipValue(data, keyEnd)
		if err != nil {
			t.Fatal(err)
		}
		if string(data[i:keyEnd]) == "4:info" {
			return fmt.Sprintf("%x", sha1.Sum(data[keyEnd:valEnd]))
		}
		i = valEnd
	}
	t.Fatal("no info dictionary")
	return ""
}
