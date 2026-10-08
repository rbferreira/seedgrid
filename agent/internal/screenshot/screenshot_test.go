package screenshot

import "testing"

// Only the pure `timestamps` function is testable without the ffmpeg/ffprobe binary
// installed — Extract/probeDuration/extractFrame depend on external processes
// and, like runMediainfo in the transfer package, are best-effort without
// dedicated unit tests.

func TestTimestamps_CountOne(t *testing.T) {
	got := timestamps(100, 1)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	want := (100*0.1 + 100*0.9) / 2 // midpoint between 10% and 90%
	if got[0] != want {
		t.Errorf("timestamp = %v, want %v", got[0], want)
	}
}

func TestTimestamps_CountFour(t *testing.T) {
	got := timestamps(100, 4)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0] != 10 {
		t.Errorf("first point = %v, want 10 (10%% of 100)", got[0])
	}
	if got[3] != 90 {
		t.Errorf("last point = %v, want 90 (90%% of 100)", got[3])
	}
	// Intermediate points uniformly distributed between 10 and 90.
	if got[1] <= got[0] || got[2] <= got[1] || got[3] <= got[2] {
		t.Errorf("points are not in ascending order: %v", got)
	}
}

func TestTimestamps_CountZeroOrNegativeTreatedAsOne(t *testing.T) {
	for _, count := range []int{0, -1, -5} {
		got := timestamps(100, count)
		if len(got) != 1 {
			t.Errorf("count=%d: len = %d, want 1", count, len(got))
		}
	}
}
