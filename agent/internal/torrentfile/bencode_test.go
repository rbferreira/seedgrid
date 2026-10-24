package torrentfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStripKeys(t *testing.T) {
	in := "d8:announce1:a4:infod4:name1:xe17:libtorrent_resumed5:fileslee8:rtorrentd5:statei1eee"
	got, err := StripKeys([]byte(in), "rtorrent", "libtorrent_resume")
	if err != nil || string(got) != "d8:announce1:a4:infod4:name1:xee" {
		t.Errorf("StripKeys = %q %v", got, err)
	}
	for _, bad := range []string{"", "l1:ae", "d4:info", "d3:key5:ab", "di1ei2ee", "d1:a1:bextra"} {
		if _, err := StripKeys([]byte(bad), "rtorrent"); err == nil {
			t.Errorf("StripKeys(%q) should fail", bad)
		}
	}
}

// TestInfoHash: the info hash survives StripKeys, and is what Create's info dictionary hashes to.
func TestInfoHash(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := Create(context.Background(), p, Options{Trackers: []string{"http://t/a"}})
	if err != nil {
		t.Fatal(err)
	}
	h1, err := InfoHash(data)
	if err != nil || len(h1) != 40 {
		t.Fatalf("InfoHash = %q %v", h1, err)
	}
	withKeys := append(append([]byte{}, data[:len(data)-1]...), []byte("8:rtorrentd5:statei1eee")...)
	stripped, err := StripKeys(withKeys, "rtorrent")
	if err != nil {
		t.Fatal(err)
	}
	if h2, _ := InfoHash(stripped); h2 != h1 {
		t.Errorf("info hash after StripKeys = %s, want %s", h2, h1)
	}
	if _, err := InfoHash([]byte("d8:announce1:ae")); err == nil {
		t.Error("a .torrent without info must fail")
	}
}
