package queue

import (
	"reflect"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
)

var on = engine.Config{
	QueueingEnabled: true, MaxActiveDownloads: 2, MaxActiveUploads: -1, MaxActiveTorrents: -1,
}

func dl(hash string, loaded int64, running bool) Item {
	return Item{Hash: hash, Loaded: loaded, Running: running, DlRate: 100 << 10}
}

func queued(hash string, loaded int64) Item {
	return Item{Hash: hash, Loaded: loaded, Queued: true}
}

func TestRound_KeepsTheOldestRunning(t *testing.T) {
	// Three downloads running with a limit of two: the newest is queued, whatever the input order.
	p := New().Round([]Item{dl("c", 3, true), dl("a", 1, true), dl("b", 2, true)}, on, time.Now())
	if want := (Plan{Stop: []string{"c"}}); !reflect.DeepEqual(p, want) {
		t.Errorf("plan = %+v, want %+v", p, want)
	}
}

func TestRound_StartsTheOldestQueuedWhenASlotFrees(t *testing.T) {
	p := New().Round([]Item{dl("a", 1, true), queued("d", 4), queued("c", 3)}, on, time.Now())
	if want := (Plan{Start: []string{"c"}}); !reflect.DeepEqual(p, want) {
		t.Errorf("plan = %+v, want %+v", p, want)
	}
}

// TestRound_NeverStartsWhatItDidNotQueue: stopped by the operator or by admission (ADR-021), a
// torrent stays stopped even with every slot free.
func TestRound_NeverStartsWhatItDidNotQueue(t *testing.T) {
	p := New().Round([]Item{{Hash: "x", Loaded: 1}, {Hash: "y", Loaded: 2, Complete: true}}, on, time.Now())
	if !reflect.DeepEqual(p, Plan{}) {
		t.Errorf("plan = %+v, want nothing", p)
	}
}

// TestRound_UploadsAndTotal: complete torrents count against the uploads, everything against the
// total.
func TestRound_UploadsAndTotal(t *testing.T) {
	c := engine.Config{QueueingEnabled: true, MaxActiveDownloads: 5, MaxActiveUploads: 1, MaxActiveTorrents: 2}
	items := []Item{
		{Hash: "s1", Loaded: 1, Running: true, Complete: true},
		{Hash: "s2", Loaded: 2, Running: true, Complete: true}, // second upload: over 1
		{Hash: "d1", Loaded: 3, Running: true},
		{Hash: "d2", Loaded: 4, Running: true}, // third torrent: over 2
		{Hash: "q", Loaded: 5, Queued: true},
	}
	p := New().Round(items, c, time.Now())
	if want := (Plan{Stop: []string{"s2", "d2"}}); !reflect.DeepEqual(p, want) {
		t.Errorf("plan = %+v, want %+v", p, want)
	}
}

func TestRound_CheckingIsLeftAlone(t *testing.T) {
	items := []Item{dl("a", 1, true), dl("b", 2, true), {Hash: "c", Loaded: 3, Running: true, Checking: true}}
	if p := New().Round(items, on, time.Now()); !reflect.DeepEqual(p, Plan{}) {
		t.Errorf("plan = %+v, want nothing: a torrent being checked is not counted nor stopped", p)
	}
}

// TestRound_DontCountSlow: a download below the threshold for longer than the timer stops
// counting, and the next queued one starts.
func TestRound_DontCountSlow(t *testing.T) {
	c := on
	c.MaxActiveDownloads = 1
	c.DontCountSlowTorrents = true
	c.SlowTorrentDlRateThreshold = 2 // KiB/s
	c.SlowTorrentInactiveTimer = 60
	q := New()
	t0 := time.Now()
	stalled := Item{Hash: "a", Loaded: 1, Running: true, DlRate: 1 << 10}
	items := []Item{stalled, queued("b", 2)}
	if p := q.Round(items, c, t0); len(p.Start) != 0 {
		t.Fatalf("slow for no time yet: plan = %+v", p)
	}
	if p := q.Round(items, c, t0.Add(30*time.Second)); len(p.Start) != 0 {
		t.Fatalf("slow for less than the timer: plan = %+v", p)
	}
	if p := q.Round(items, c, t0.Add(61*time.Second)); !reflect.DeepEqual(p, Plan{Start: []string{"b"}}) {
		t.Fatalf("slow past the timer: plan = %+v, want b started", p)
	}
	// Fast again, it counts again: with b running, one of the two is over the limit.
	stalled.DlRate = 10 << 10
	items = []Item{stalled, dl("b", 2, true)}
	if p := q.Round(items, c, t0.Add(62*time.Second)); !reflect.DeepEqual(p, Plan{Stop: []string{"b"}}) {
		t.Fatalf("fast again: plan = %+v, want b queued", p)
	}
}

// TestRound_Off: with the queue turned off, what it held runs again.
func TestRound_Off(t *testing.T) {
	items := []Item{dl("a", 1, true), dl("b", 2, true), dl("c", 3, true), queued("d", 4), {Hash: "e", Loaded: 5}}
	p := New().Round(items, engine.Config{QueueingEnabled: false, MaxActiveDownloads: 1}, time.Now())
	if want := (Plan{Start: []string{"d"}}); !reflect.DeepEqual(p, want) {
		t.Errorf("plan = %+v, want %+v", p, want)
	}
}

// TestRound_Unmark: a torrent started outside SeedGrid loses the mark, so that a later stop by
// someone else is not undone by the queue.
func TestRound_Unmark(t *testing.T) {
	items := []Item{{Hash: "a", Loaded: 1, Running: true, Queued: true, DlRate: 100 << 10}}
	if p := New().Round(items, on, time.Now()); !reflect.DeepEqual(p, Plan{Unmark: []string{"a"}}) {
		t.Errorf("plan = %+v, want a unmarked", p)
	}
}

func TestRound_ZeroLimitAllowsNone(t *testing.T) {
	c := on
	c.MaxActiveDownloads = 0
	if p := New().Round([]Item{dl("a", 1, true), queued("b", 2)}, c, time.Now()); !reflect.DeepEqual(p, Plan{Stop: []string{"a"}}) {
		t.Errorf("plan = %+v, want a queued and b left queued", p)
	}
}
