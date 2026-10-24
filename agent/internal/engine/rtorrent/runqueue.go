package rtorrent

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/engine/queue"
)

// queueEvery is how often the Agent's queue looks at rTorrent: seconds, as the ADR accepts, not
// inside the Engine.
const queueEvery = 5 * time.Second

// RunQueue runs the Agent's queue for rTorrent (ADR-045 §4) until ctx ends.
func (a *Adapter) RunQueue(ctx context.Context, logger *slog.Logger) {
	q := queue.New()
	t := time.NewTicker(queueEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := a.queueRound(rctx, q, time.Now(), logger); err != nil {
			logger.Debug("Engine queue round", "error", err)
		}
		cancel()
	}
}

func (a *Adapter) queueRound(ctx context.Context, q *queue.Queue, now time.Time, logger *slog.Logger) error {
	if !a.queueApplied() {
		return nil
	}
	rows, err := a.rows(ctx, false)
	if err != nil {
		return err
	}
	items := make([]queue.Item, len(rows))
	for i, r := range rows {
		items[i] = queue.Item{
			Hash: r.hash, Complete: r.complete, Running: r.started && r.active,
			Queued: r.custom == queuedMark, Checking: r.hashing != 0,
			DlRate: r.downRate, UpRate: r.upRate, Loaded: r.loaded,
		}
	}
	a.queue.mu.Lock()
	cfg := a.queue.cfg
	a.queue.mu.Unlock()
	p := q.Round(items, cfg, now)
	var calls []rpcCall
	for _, h := range p.Unmark {
		calls = append(calls, rpcCall{"d.custom.set", []any{strings.ToUpper(h), customKey, ""}})
	}
	for _, h := range p.Stop {
		H := strings.ToUpper(h)
		calls = append(calls,
			rpcCall{"d.stop", []any{H}},
			rpcCall{"d.close", []any{H}},
			rpcCall{"d.custom.set", []any{H, customKey, queuedMark}})
	}
	for _, h := range p.Start {
		H := strings.ToUpper(h)
		calls = append(calls,
			rpcCall{"d.custom.set", []any{H, customKey, ""}},
			rpcCall{"d.open", []any{H}},
			rpcCall{"d.start", []any{H}})
	}
	if len(calls) == 0 {
		return nil
	}
	if len(p.Stop)+len(p.Start) > 0 {
		logger.Info("Engine queue", "queued", p.Stop, "started", p.Start)
	}
	return a.all(ctx, calls)
}

func (a *Adapter) queueApplied() bool {
	a.queue.mu.Lock()
	defer a.queue.mu.Unlock()
	return a.queue.applied
}
