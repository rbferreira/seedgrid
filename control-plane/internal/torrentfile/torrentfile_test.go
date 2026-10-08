package torrentfile_test

import (
	"crypto/sha1" //nolint:gosec // G505: same reason as the tested package — BitTorrent infohash
	"fmt"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/torrentfile"
)

func TestParse(t *testing.T) {
	// info dict with keys in bencode order (length, name, piece length, pieces).
	info := []byte("d6:lengthi100e4:name3:bar12:piece lengthi16384e6:pieces0:e")
	data := append([]byte("d8:announce3:foo13:announce-listll3:baree4:info"), info...)
	data = append(data, 'e')

	m, err := torrentfile.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := fmt.Sprintf("%x", sha1.Sum(info)) //nolint:gosec // G401: idem
	if m.InfoHash != want {
		t.Errorf("InfoHash = %s, want %s", m.InfoHash, want)
	}
	if m.Name != "bar" {
		t.Errorf("Name = %q, want bar", m.Name)
	}
	// announce "foo" + announce-list [["bar"]] → dedup keeps foo, bar.
	if len(m.Trackers) != 2 || m.Trackers[0] != "foo" {
		t.Errorf("Trackers = %v", m.Trackers)
	}
	// Size single-file: info.length (ADR-032).
	if m.Size != 100 {
		t.Errorf("Size = %d, want 100 (info.length)", m.Size)
	}
}

// TestParseSizeMultiFile: multi-file torrent sums files[].length (ADR-032).
func TestParseSizeMultiFile(t *testing.T) {
	// info with files=[{length:10,path:[a]},{length:32,path:[b]}] (no length at the top).
	info := []byte("d5:filesld6:lengthi10e4:pathl1:aeed6:lengthi32e4:pathl1:beee4:name3:dir12:piece lengthi16384e6:pieces0:e")
	data := append([]byte("d4:info"), info...)
	data = append(data, 'e')

	m, err := torrentfile.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Size != 42 {
		t.Errorf("Size = %d, want 42 (10+32)", m.Size)
	}
}

// TestParseSizeAbsent: no length or files, Size=0 → disk check skips it (ADR-032).
func TestParseSizeAbsent(t *testing.T) {
	info := []byte("d4:name3:bar12:piece lengthi16384e6:pieces0:e")
	data := append([]byte("d4:info"), info...)
	data = append(data, 'e')
	m, err := torrentfile.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Size != 0 {
		t.Errorf("Size = %d, want 0 (absent)", m.Size)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := torrentfile.Parse([]byte("not bencode")); err == nil {
		t.Error("expected error for invalid bencode")
	}
	if _, err := torrentfile.Parse(nil); err == nil {
		t.Error("expected error for empty")
	}
}
