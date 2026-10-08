package qbittorrent

import "testing"

// TestSlotLimitRegex covers the extraction of the DL slots limit from the tracker
// UNIT3D (error 164), including text variations and cases that should not match.
func TestSlotLimitRegex(t *testing.T) {
	cases := []struct {
		msg      string
		want     string // "" = should not match
		wantSlot bool
	}{
		{"Your slot limit is reached! You may at most download 4 torrents at the same time", "4", true},
		{"You may at most download 10 torrents", "10", true},
		{"YOU MAY AT MOST DOWNLOAD 2 TORRENTS", "2", true}, // case-insensitive
		{"Working", "", false},
		{"", "", false},
		{"You may at most download some torrents", "", false}, // no number
	}
	for _, c := range cases {
		m := slotLimitRe.FindStringSubmatch(c.msg)
		if c.wantSlot {
			if m == nil {
				t.Errorf("msg %q: expected to match, did not match", c.msg)
				continue
			}
			if m[1] != c.want {
				t.Errorf("msg %q: extracted %q, want %q", c.msg, m[1], c.want)
			}
		} else if m != nil {
			t.Errorf("msg %q: should not match, matched %q", c.msg, m[1])
		}
	}
}
