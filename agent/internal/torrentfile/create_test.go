package torrentfile

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // G505: BitTorrent v1 pieces are SHA-1
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPieceSize(t *testing.T) {
	for _, c := range []struct{ total, want int64 }{
		{11, 16 << 10},
		{1 << 20, 16 << 10},
		{1 << 30, 512 << 10},
		{100 << 30, 16 << 20},
	} {
		if got := PieceSize(c.total); got != c.want {
			t.Errorf("PieceSize(%d) = %d, want %d", c.total, got, c.want)
		}
	}
}

func TestCreate_SingleFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Create(context.Background(), p, Options{Trackers: []string{"https://t/announce"}, Private: true, Source: "SGX"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sum := sha1.Sum([]byte("hello world")) //nolint:gosec // G401: BitTorrent v1 pieces are SHA-1
	info := "4:infod6:lengthi11e4:name5:a.txt12:piece lengthi16384e6:pieces20:" + string(sum[:]) + "7:privatei1e6:source3:SGXee"
	if !strings.HasPrefix(string(out), "d8:announce18:https://t/announce10:created by8:SeedGrid13:creation datei") || !strings.HasSuffix(string(out), info) {
		t.Errorf("Create =\n%q\nwant the announce, then the info dictionary\n%q", out, info)
	}
	if strings.Contains(string(out), "announce-list") {
		t.Error("one tracker needs no announce-list")
	}
}

// TestCreate_Folder: the files are hashed end to end as one stream (a piece spans the two), listed
// by path, and the trackers each get a tier.
func TestCreate_Folder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "album")
	a := bytes.Repeat([]byte{'a'}, 10000)
	b := bytes.Repeat([]byte{'b'}, 10000)
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b", "2.bin"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.bin"), a, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(root, "a.bin"), filepath.Join(root, "link.bin")) // left out
	out, err := Create(context.Background(), root, Options{Trackers: []string{"http://x/1", "http://y/2"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stream := append(append([]byte{}, a...), b...)
	p1 := sha1.Sum(stream[:16384]) //nolint:gosec // G401: BitTorrent v1 pieces are SHA-1
	p2 := sha1.Sum(stream[16384:]) //nolint:gosec // G401: BitTorrent v1 pieces are SHA-1
	info := "4:infod5:filesld6:lengthi10000e4:pathl5:a.bineed6:lengthi10000e4:pathl1:b5:2.bineee" +
		"4:name5:album12:piece lengthi16384e6:pieces40:" + string(p1[:]) + string(p2[:]) + "ee"
	if !strings.HasSuffix(string(out), info) {
		t.Errorf("Create =\n%q\nwant the info dictionary\n%q", out, info)
	}
	if !strings.Contains(string(out), "8:announce10:http://x/113:announce-listll10:http://x/1el10:http://y/2ee") {
		t.Errorf("Create = %q, want announce and one tier per tracker", out)
	}
}

func TestCreate_Refuses(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create(context.Background(), dir, Options{}); err == nil {
		t.Error("a folder without files must be refused")
	}
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(context.Background(), empty, Options{}); err == nil {
		t.Error("an empty file must be refused")
	}
	if _, err := Create(context.Background(), filepath.Join(dir, "missing"), Options{}); err == nil {
		t.Error("a missing path must be refused")
	}
	full := filepath.Join(dir, "full")
	if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Create(ctx, full, Options{}); err == nil {
		t.Error("a cancelled creation must fail")
	}
}
