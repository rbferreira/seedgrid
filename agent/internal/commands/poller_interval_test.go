package commands

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
)

// The polling interval changes on the fly via Control Plane configuration (ADR-039): the Poller
// that woke up hourly now wakes up every 30 ms WITHOUT restarting and without waiting for the
// old tick to expire — on a Node with a 30s interval, waiting would mean 30s of stale behavior.
func TestPollerSetIntervalAppliesImmediately(t *testing.T) {
	var polls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := NewPoller(client.New(srv.URL), func() string { return "tok" }, NewExecutor(&fakeEngine{}),
		time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()

	time.Sleep(50 * time.Millisecond)
	if n := polls.Load(); n != 0 {
		t.Fatalf("with 1h interval it should not have polled yet (polled %d times)", n)
	}

	p.SetInterval(20 * time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for polls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := polls.Load(); n < 3 {
		t.Fatalf("after SetInterval(20ms) the Poller polled only %d times in 5s — the new interval was not adopted", n)
	}
	if got := p.Interval(); got != 20*time.Millisecond {
		t.Errorf("Interval = %v, want 20ms", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit with canceled context")
	}
}

// A non-positive value would crash the ticker (time.NewTicker/Reset panic on ≤ 0): the
// Poller ignores it and keeps the previous one.
func TestPollerSetIntervalIgnoresInvalidValue(t *testing.T) {
	p := NewPoller(client.New("http://127.0.0.1:1"), func() string { return "tok" }, NewExecutor(&fakeEngine{}),
		5*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.SetInterval(0)
	p.SetInterval(-time.Second)
	if got := p.Interval(); got != 5*time.Second {
		t.Errorf("Interval = %v, want 5s (invalid value ignored)", got)
	}
	p.SetInterval(7 * time.Second)
	if got := p.Interval(); got != 7*time.Second {
		t.Errorf("Interval = %v, want 7s", got)
	}
}
