package commands

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
)

// stepEngine is an Engine whose State() changes on each query — the common fakeEngine
// returns a fixed value and is not suitable for exercising the recheck watchdog (#79).
type stepEngine struct {
	fakeEngine
	steps  []float64 // progress returned on each call; the last one repeats
	calls  int
	stateS string
}

func (s *stepEngine) State(_ context.Context, _ string) (float64, string, error) {
	p := s.steps[len(s.steps)-1]
	if s.calls < len(s.steps) {
		p = s.steps[s.calls]
	}
	s.calls++
	st := s.stateS
	if st == "" {
		st = "SEEDING"
	}
	return p, st, nil
}

// timeEngine advances progress based on TIME, not the number of queries: it is the
// only way to truly exercise the stagnation window. An engine that advances on
// each call empties the steps in a few milliseconds and the test would pass even with
// the window reset removed (verified by mutation — the first pair of tests I
// wrote here were decorative because of this).
type timeEngine struct {
	fakeEngine
	start   time.Time
	step    time.Duration // how often the progress increases
	delta   float64       // how much it goes up per step
	ceiling float64
	stateS  string
}

func (e *timeEngine) State(_ context.Context, _ string) (float64, string, error) {
	p := float64(int(time.Since(e.start)/e.step)) * e.delta
	if p > e.ceiling {
		p = e.ceiling
	}
	st := e.stateS
	if st == "" {
		st = "SEEDING"
	}
	return p, st, nil
}

func fastExecutor(e Engine) *Executor {
	ex := NewExecutor(e)
	ex.recheckPoll = time.Millisecond
	ex.recheckStall = 60 * time.Millisecond
	ex.recheckMax = 3 * time.Second
	return ex
}

// A slow but MOVING Recheck must not fail: this was the case of #79 (10 GB on a USB HD
// takes longer than the old fixed deadline of 10 min, with a perfect copy at the destination).
//
// The progress here is SLOWER than the entire stagnation window only when summed: each
// step (40 ms) fits inside the window (100 ms), but the total wait (~240 ms) exceeds it
// by a wide margin. Without the window resetting on every advance, this fails.
func TestWaitProgress_SlowAdvanceDoesNotFail(t *testing.T) {
	e := &timeEngine{start: time.Now(), step: 40 * time.Millisecond, delta: 0.2, ceiling: 1.0}
	ex := fastExecutor(e)
	ex.recheckStall = 100 * time.Millisecond
	ex.recheckPoll = 5 * time.Millisecond
	start := time.Now()
	if err := ex.waitProgress(context.Background(), "h1", 0.999); err != nil {
		t.Fatalf("progress advancing should not fail: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("wait only lasted %s — the test did not cross the stall window", elapsed)
	}
}

// Stuck progress = real stall (the symptom of #79: 0/1333 pieces, idle disk).
// It needs to fail, and fast, so the CP aborts the migration instead of waiting for the cap.
func TestWaitProgress_StagnationFails(t *testing.T) {
	e := &stepEngine{steps: []float64{0}, stateS: "CHECKING"}
	ex := fastExecutor(e)
	start := time.Now()
	err := ex.waitProgress(context.Background(), "h1", 0.999)
	if err == nil {
		t.Fatal("stalled progress should fail")
	}
	if !strings.Contains(err.Error(), "stalled") {
		t.Errorf("error should indicate stall, got %q", err)
	}
	// Failed due to stagnation (60ms), not the cap (3s).
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("should fail in the stall window; took %s", elapsed)
	}
}

// A recheck that stalls AFTER already advancing still needs to be cut off: the window
// resets on advance, but is not infinite. It covers the middle ground between the two tests above.
func TestWaitProgress_StallsAfterAdvancing(t *testing.T) {
	// Goes up to 0.4 in the first 80 ms and freezes (cap below the requirement).
	e := &timeEngine{start: time.Now(), step: 40 * time.Millisecond, delta: 0.2, ceiling: 0.4}
	ex := fastExecutor(e)
	ex.recheckStall = 100 * time.Millisecond
	ex.recheckPoll = 5 * time.Millisecond
	err := ex.waitProgress(context.Background(), "h1", 0.999)
	if err == nil || !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("recheck that freezes after advancing should fail due to stall, got %v", err)
	}
}

// Safety cap: progress that always goes up, but never reaches the requirement, must not
// hold the Poller (which executes Node commands in series) forever.
func TestWaitProgress_MaxCeiling(t *testing.T) {
	e := &stepEngine{steps: []float64{0.1}}
	ex := fastExecutor(e)
	ex.recheckStall = time.Hour // disables stagnation to isolate the cap
	ex.recheckMax = 80 * time.Millisecond
	err := ex.waitProgress(context.Background(), "h1", 0.999)
	if err == nil || !strings.Contains(err.Error(), "did not reach source progress") {
		t.Fatalf("should fail at the maximum ceiling, got %v", err)
	}
}

// With min < 0.999 (partial migration, ADR-031) the state doesn't need to be SEEDING:
// it is enough for the recheck to confirm the progress the source had.
func TestWaitProgress_PartialDoesNotRequireSeeding(t *testing.T) {
	e := &stepEngine{steps: []float64{0.2, 0.6}, stateS: "DOWNLOADING"}
	ex := fastExecutor(e)
	if err := ex.waitProgress(context.Background(), "h1", 0.5); err != nil {
		t.Fatalf("partial should not require SEEDING: %v", err)
	}
}

// The central bug of #79: without the explicit start, qBittorrent accepts the Recheck but doesn't
// verify a single piece. migrate_pull needs to start the torrent at the destination.
func TestMigratePull_StartsTorrentAtDestination(t *testing.T) {
	e := &stepEngine{steps: []float64{1.0}}
	fm := &fakeMigrator{}
	ex := fastExecutor(e).EnableMigration(fm, "/data")
	tf := base64.StdEncoding.EncodeToString([]byte("torrent"))
	res := ex.Execute(context.Background(), client.Command{
		Type:    "migrate_pull",
		Payload: []byte(`{"hash":"h1","token":"t","source_host":"1.2.3.4","source_port":6900,"torrent_file":"` + tf + `","min_progress":1}`),
	})
	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s), want DONE", res.Status, res.Error)
	}
	if e.rechecked != "h1" {
		t.Errorf("recheck was not triggered (rechecked=%q)", e.rechecked)
	}
	if e.started != "h1" {
		t.Errorf("torrent was not started at the destination (started=%q) — without this recheck stays at 0 (#79)", e.started)
	}
}
