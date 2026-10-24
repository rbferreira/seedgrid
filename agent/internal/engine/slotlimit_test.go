package engine

import "testing"

// TestSlotLimitIn covers the extraction of the DL slots limit from the tracker
// UNIT3D (error 164), including text variations and cases that should not match.
func TestSlotLimitIn(t *testing.T) {
	cases := []struct {
		msg    string
		want   int
		wantOK bool
	}{
		{"Your slot limit is reached! You may at most download 4 torrents at the same time", 4, true},
		{"You may at most download 10 torrents", 10, true},
		{"YOU MAY AT MOST DOWNLOAD 2 TORRENTS", 2, true}, // case-insensitive
		// rTorrent puts the tracker's answer inside its own message (ADR-045).
		{`Tracker: [Failure reason "Your slot limit is reached! You may at most download 3 torrents at the same time"]`, 3, true},
		{"Working", 0, false},
		{"", 0, false},
		{"You may at most download some torrents", 0, false}, // no number
		{"You may at most download 0 torrents", 0, false},    // not a limit
	}
	for _, c := range cases {
		n, ok := SlotLimitIn(c.msg)
		if ok != c.wantOK || n != c.want {
			t.Errorf("SlotLimitIn(%q) = %d, %t; want %d, %t", c.msg, n, ok, c.want, c.wantOK)
		}
	}
}
