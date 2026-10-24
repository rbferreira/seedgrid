// Package queue is the download queue the Agent runs for an Engine that has none (ADR-045 §4). It
// applies the engine.* settings (ADR-039) the way qBittorrent's queue does, so they mean the same
// on every Node: at most so many downloads, uploads and torrents active, the oldest first, and a
// torrent slow for longer than the timer not counting when dont_count_slow is on.
//
// It only ever starts torrents it queued: a torrent the operator or the admission controller
// (ADR-021) stopped is not the queue's.
//
// The package decides; the adapter that runs it stops, marks and starts.
package queue

import (
	"sort"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
)

// Item is a torrent as the queue sees it.
type Item struct {
	Hash     string
	Complete bool
	Running  bool // started and active
	Queued   bool // stopped by the queue (and not started since)
	Checking bool // being hashed: never counted, never stopped
	DlRate   int64
	UpRate   int64
	Loaded   int64 // when the Engine got it: the queue's order, oldest first
}

// Plan is what the queue wants done now.
type Plan struct {
	Stop  []string // stop and mark queued
	Start []string // queued, with a slot free
	// Unmark: running torrents that still carry the mark (started outside SeedGrid, e.g. from
	// ruTorrent). Stopped again later by someone else, they are not the queue's to start.
	Unmark []string
}

// Queue keeps, between rounds, since when each torrent has been slow.
type Queue struct {
	slowSince map[string]time.Time
}

// New returns an empty queue.
func New() *Queue {
	return &Queue{slowSince: map[string]time.Time{}}
}

// Round decides one round of the queue over the torrents the Engine holds now.
func (q *Queue) Round(items []Item, c engine.Config, now time.Time) Plan {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Loaded < items[j].Loaded })
	if !c.QueueingEnabled {
		// No queue: what it held goes back to running, as qBittorrent does when queueing is
		// turned off.
		var p Plan
		for _, it := range items {
			switch {
			case it.Queued && it.Running:
				p.Unmark = append(p.Unmark, it.Hash)
			case it.Queued:
				p.Start = append(p.Start, it.Hash)
			}
		}
		q.slowSince = map[string]time.Time{}
		return p
	}

	seen := make(map[string]bool, len(items))
	dl, ul, all := newSlots(c.MaxActiveDownloads), newSlots(c.MaxActiveUploads), newSlots(c.MaxActiveTorrents)
	var p Plan
	for _, it := range items {
		seen[it.Hash] = true
		if it.Running && it.Queued {
			p.Unmark = append(p.Unmark, it.Hash)
		}
		if !it.Running || it.Checking {
			continue
		}
		if c.DontCountSlowTorrents && q.slow(it, c, now) {
			continue
		}
		kind := dl
		if it.Complete {
			kind = ul
		}
		inKind, inAll := kind.take(), all.take()
		if inKind && inAll {
			continue
		}
		kind.give() // the one that does not fit gives back what it took
		all.give()
		p.Stop = append(p.Stop, it.Hash)
	}
	for h := range q.slowSince {
		if !seen[h] {
			delete(q.slowSince, h)
		}
	}
	for _, it := range items {
		if !it.Queued || it.Running || it.Checking {
			continue
		}
		kind := dl
		if it.Complete {
			kind = ul
		}
		if kind.free() && all.free() {
			kind.take()
			all.take()
			p.Start = append(p.Start, it.Hash)
		}
	}
	return p
}

// slow tells whether a running torrent has been below the slow threshold for longer than the timer.
func (q *Queue) slow(it Item, c engine.Config, now time.Time) bool {
	rate, threshold := it.DlRate, c.SlowTorrentDlRateThreshold
	if it.Complete {
		rate, threshold = it.UpRate, c.SlowTorrentUlRateThreshold
	}
	if rate >= int64(threshold)*1024 { // thresholds are KiB/s
		delete(q.slowSince, it.Hash)
		return false
	}
	since, ok := q.slowSince[it.Hash]
	if !ok {
		q.slowSince[it.Hash] = now
		return false
	}
	return now.Sub(since) > time.Duration(c.SlowTorrentInactiveTimer)*time.Second
}

// slots counts what a limit still allows; a negative limit is no limit, as in qBittorrent.
type slots struct {
	limit, used int
}

func newSlots(limit int) *slots { return &slots{limit: limit} }

func (s *slots) free() bool { return s.limit < 0 || s.used < s.limit }

// take uses a slot, and says whether it was within the limit.
func (s *slots) take() bool {
	s.used++
	return s.limit < 0 || s.used <= s.limit
}

func (s *slots) give() { s.used-- }
