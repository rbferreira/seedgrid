package migration

import (
	"strings"
	"testing"
	"time"
)

func TestProgressFloor(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"complete deducts tolerance", 1.0, 0.995},
		{"partial deducts tolerance", 0.6, 0.595},
		{"missing/zero requires complete", 0, 0.999},
		{"negative requires complete", -0.2, 0.999},
		{"out of range requires complete", 1.5, 0.999},
		{"floor is never negative", 0.004, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := progressFloor(c.in); got < c.want-1e-9 || got > c.want+1e-9 {
				t.Errorf("progressFloor(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestJudgeRecheck(t *testing.T) {
	const (
		stall   = 10 * time.Minute
		ceiling = time.Hour
	)
	done := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	after := func(d time.Duration) time.Time { return done.Add(d) }

	cases := []struct {
		name     string
		view     destView
		min      float64
		st       verifyState
		now      time.Time
		want     verdict
		contains string
		wantBest float64
		wantAdv  *time.Time
	}{
		{
			name: "no target read: waits",
			view: destView{}, min: 1, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictWait,
		},
		{
			name: "read before DONE does not count, even if SEEDING at 100%",
			view: destView{found: true, status: "SEEDING", progress: 1, syncedAt: after(-time.Second)},
			min:  1, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictWait,
		},
		{
			name: "complete: SEEDING at 100% done after DONE completes",
			view: destView{found: true, status: "SEEDING", progress: 1, syncedAt: after(time.Second)},
			min:  1, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictDone,
		},
		{
			name: "complete: 99,6% (boundary piece) is within tolerance",
			view: destView{found: true, status: "DOWNLOADING", progress: 0.996, syncedAt: after(time.Second)},
			min:  1, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictDone,
		},
		{
			name: "CHECKING above floor does NOT complete: it is the checking progress, not what the torrent has",
			view: destView{found: true, status: "CHECKING", progress: 0.9, syncedAt: after(time.Second)},
			min:  0.6, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictWait, wantBest: 0.9,
		},
		{
			name: "transient or error state proves nothing, even above floor",
			view: destView{found: true, status: "UNKNOWN", progress: 1, syncedAt: after(time.Second)},
			min:  1, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictWait,
		},
		{
			name: "partial: outside CHECKING and at floor completes, following download",
			view: destView{found: true, status: "DOWNLOADING", progress: 0.6, syncedAt: after(time.Second)},
			min:  0.6, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictDone,
		},
		{
			name: "below floor outside CHECKING waits (can still go up)",
			view: destView{found: true, status: "DOWNLOADING", progress: 0.3, syncedAt: after(time.Second)},
			min:  0.6, st: verifyState{best: -1, advancedAt: done},
			now: after(time.Minute), want: verdictWait,
		},
		{
			name: "stagnant for longer than window fails as stuck recheck",
			view: destView{found: true, status: "CHECKING", progress: 0.2, syncedAt: after(11 * time.Minute)},
			min:  1, st: verifyState{best: 0.2, advancedAt: done},
			now: after(11 * time.Minute), want: verdictFail, contains: "destination recheck stalled",
		},
		{
			name: "stagnant, but still within window: waits",
			view: destView{found: true, status: "CHECKING", progress: 0.2, syncedAt: after(9 * time.Minute)},
			min:  1, st: verifyState{best: 0.2, advancedAt: done},
			now: after(9 * time.Minute), want: verdictWait,
		},
		{
			name: "progress renews stagnation window",
			view: destView{found: true, status: "CHECKING", progress: 0.5, syncedAt: after(11 * time.Minute)},
			min:  1, st: verifyState{best: 0.2, advancedAt: done},
			now: after(11 * time.Minute), want: verdictWait, wantBest: 0.5, wantAdv: ptr(after(11 * time.Minute)),
		},
		{
			name: "Engine rounding noise does not count as progress",
			view: destView{found: true, status: "CHECKING", progress: 0.20005, syncedAt: after(11 * time.Minute)},
			min:  1, st: verifyState{best: 0.2, advancedAt: done},
			now: after(11 * time.Minute), want: verdictFail, contains: "stalled",
		},
		{
			name: "ceiling exceeded with progress still rising fails as did not reach",
			view: destView{found: true, status: "CHECKING", progress: 0.9, syncedAt: after(2 * time.Hour)},
			min:  1, st: verifyState{best: 0.8, advancedAt: after(2*time.Hour - time.Minute)},
			now: after(2 * time.Hour), want: verdictFail, contains: "did not reach source progress",
		},
		{
			name: "no read for longer than window: reason says there was no read",
			view: destView{}, min: 1, st: verifyState{best: -1, advancedAt: done},
			now: after(11 * time.Minute), want: verdictFail, contains: "unread",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := c.st
			got, reason := judgeRecheck(c.view, c.min, &st, done, c.now, stall, ceiling)
			if got != c.want {
				t.Fatalf("verdict = %v (%q), want %v", got, reason, c.want)
			}
			if c.contains != "" && !strings.Contains(reason, c.contains) {
				t.Errorf("reason = %q, want to contain %q", reason, c.contains)
			}
			if c.want != verdictFail && reason != "" {
				t.Errorf("reason = %q, want empty outside of failure", reason)
			}
			if c.wantBest != 0 && st.best != c.wantBest {
				t.Errorf("best = %v, want %v", st.best, c.wantBest)
			}
			if c.wantAdv != nil && !st.advancedAt.Equal(*c.wantAdv) {
				t.Errorf("advancedAt = %v, want %v", st.advancedAt, *c.wantAdv)
			}
		})
	}
}

func TestProvesProgress(t *testing.T) {
	// The states produced by the Agent adapter (adapter.MapState). Only the four where the
	// progress is "what the torrent has" prove it; the rest wait.
	for status, want := range map[string]bool{
		"SEEDING": true, "DOWNLOADING": true, "QUEUED": true, "PAUSED": true,
		"CHECKING": false, "MOVING": false, "ERROR": false, "UNKNOWN": false, "": false,
	} {
		if got := provesProgress(status); got != want {
			t.Errorf("provesProgress(%q) = %v, want %v", status, got, want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// The watchdog state is created the 1st time the process sees the migration (best=-1: the first
// read, whatever it is, counts as progress) and is discarded when it exits flight.
func TestVerifyStates_Lifecycle(t *testing.T) {
	var v verifyStates
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	st := v.of("m1", now)
	if st.best != -1 || !st.advancedAt.Equal(now) {
		t.Fatalf("new state = %+v, want best=-1 and window opened now", st)
	}
	st.best = 0.4
	if again := v.of("m1", now.Add(time.Hour)); again.best != 0.4 || !again.advancedAt.Equal(now) {
		t.Errorf("second query = %+v; should return the SAME state, without renewing the window", again)
	}

	v.of("m2", now)
	v.keepOnly(map[string]bool{"m1": true}) // m2 exited flight through another path
	if _, ok := v.m["m2"]; ok {
		t.Error("m2 should have been discarded")
	}
	if _, ok := v.m["m1"]; !ok {
		t.Error("m1 is still in flight and cannot be discarded")
	}
	v.forget("m1")
	if len(v.m) != 0 {
		t.Errorf("map = %v, want empty", v.m)
	}
}
