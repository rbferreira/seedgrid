package health

import (
	"testing"
	"time"
)

func TestRegistry_RegisteredButNeverRan(t *testing.T) {
	r := NewRegistry()
	r.Register("alert-worker", 30*time.Second)

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
	w := snap[0]
	if w.Name != "alert-worker" {
		t.Errorf("name = %q, want alert-worker", w.Name)
	}
	if w.LastRun != nil {
		t.Errorf("last_run = %v, want nil (never ran)", w.LastRun)
	}
	if w.Healthy {
		t.Error("healthy = true, want false (never ran)")
	}
	if w.Interval != 30 {
		t.Errorf("interval = %d, want 30", w.Interval)
	}
}

func TestRegistry_BeatMarksHealthy(t *testing.T) {
	r := NewRegistry()
	beat := r.Register("admission-worker", 15*time.Second)
	beat()

	w := r.Snapshot()[0]
	if w.LastRun == nil {
		t.Fatal("last_run = nil after beat, want populated")
	}
	if !w.Healthy {
		t.Error("healthy = false right after beat, want true")
	}
}

func TestRegistry_StaleWhenOverdue(t *testing.T) {
	r := NewRegistry()
	r.Register("migration-reconciler", 10*time.Second)
	// Simulates a last execution well in the past (beyond staleFactor * interval).
	r.mu.Lock()
	r.m["migration-reconciler"].lastRun = time.Now().Add(-5 * time.Minute)
	r.mu.Unlock()

	w := r.Snapshot()[0]
	if w.LastRun == nil {
		t.Fatal("last_run = nil, want filled")
	}
	if w.Healthy {
		t.Error("healthy = true for run 5min ago (interval 10s), want false")
	}
}

func TestRegistry_SnapshotSortedByName(t *testing.T) {
	r := NewRegistry()
	r.Register("zeta", time.Second)
	r.Register("alpha", time.Second)
	r.Register("mid", time.Second)

	snap := r.Snapshot()
	if snap[0].Name != "alpha" || snap[1].Name != "mid" || snap[2].Name != "zeta" {
		t.Errorf("order = %v, want [alpha mid zeta]", []string{snap[0].Name, snap[1].Name, snap[2].Name})
	}
}
