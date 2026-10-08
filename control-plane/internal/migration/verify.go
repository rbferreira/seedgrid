package migration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// Destination recheck verification (#87).
//
// Before #87 the Agent was the one waiting for the recheck: migrate_pull stayed in a
// State() loop until the destination hit the floor and only then replied DONE — and, since the Poller
// executes Node commands in series, the entire Node went deaf (pause/resume/
// remove of OTHER torrents) for up to 2h. Now the Agent replies DONE as soon as the copy
// finishes and the recheck has been triggered, and WHO OBSERVES the outcome is the CP, which now decides the
// migration outcome (ADR-018): the pull's DONE no longer means "the destination is
// verified".
//
// The observation doesn't depend on knowing which Agent replied. The old Agent only replies
// DONE after the recheck; the new one, before. In both cases the CP looks at the SAME thing — the
// torrent line on the destination Node, fed by SyncStates (which "adopts" it as soon as
// the destination Engine has the hash) — and only concludes when it proves the floor.

const (
	// defaultRecheckStall/defaultRecheckMax apply when the catalog (ADR-023) doesn't inject a
	// provider — tests and nil-safety. In production they come from migration.recheck_stall_window and
	// migration.recheck_max_wait.
	defaultRecheckStall = 10 * time.Minute
	defaultRecheckMax   = 6 * time.Hour

	// pieceTolerance is the margin deducted from the progress required at the destination (ADR-031): the
	// recheck invalidates the BORDER piece when the download stopped in the middle of it. Same
	// value that the Agent applied (agent/internal/commands.pieceTolerance) — the criterion
	// just changed address, not rule.
	pieceTolerance = 0.005

	// progressEpsilon prevents Engine rounding noise from counting as progress.
	progressEpsilon = 0.0001
)

// SetRecheckWatchdog injects the recheck watchdog limits (migration.recheck_stall_window
// and migration.recheck_max_wait), read per pass (hot-reload, ADR-023). nil-safe: without a
// provider, or with a value <= 0, the defaults apply — the watchdog is never disabled.
func (s *Service) SetRecheckWatchdog(stall, ceiling func() time.Duration) {
	s.recheckStall, s.recheckMax = stall, ceiling
}

func (s *Service) stallWindow() time.Duration {
	if s.recheckStall != nil {
		if d := s.recheckStall(); d > 0 {
			return d
		}
	}
	return defaultRecheckStall
}

func (s *Service) maxWait() time.Duration {
	if s.recheckMax != nil {
		if d := s.recheckMax(); d > 0 {
			return d
		}
	}
	return defaultRecheckMax
}

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// destView is what the CP knows about the torrent on the destination Node: the line adopted by SyncStates.
type destView struct {
	found    bool
	status   string
	progress float64
	syncedAt time.Time // zero = the line has not been synced yet
}

// verifyState is the EPHEMERAL state of a migration's watchdog: the best progress ever
// observed and when it last went up. It stays in memory, like the Admission
// Worker's — restarting the CP only renews the stall window (it never drops a healthy
// migration); the absolute ceiling is measured from the pull's DONE, which is in the database.
type verifyState struct {
	best       float64
	advancedAt time.Time
}

type verdict int

const (
	verdictWait verdict = iota // still verifying: nothing to do on this pass
	verdictDone                // the destination confirmed the floor: complete
	verdictFail                // stalled or hit the ceiling: abort without loss
)

// provesProgress tells if the Engine state makes the reported progress reliable as
// "what the torrent has". During the recheck (CHECKING) qBittorrent reports the
// VERIFICATION progress as progress, and in transient or error states (MOVING, UNKNOWN, ERROR —
// adapter.MapState) there is no read that proves anything. Allowlist, not "!= CHECKING":
// a state that the Agent couldn't map cannot become proof by omission.
func provesProgress(status string) bool {
	switch status {
	case "SEEDING", "DOWNLOADING", "QUEUED", "PAUSED":
		return true
	}
	return false
}

// progressFloor is the progress floor that the destination needs to prove: the progress from the
// source minus the border piece tolerance. A value outside of (0,1] (it never leaves
// promoteOne, but it's the defense against an "empty" migration passing as success) requires the
// complete torrent.
func progressFloor(minProgress float64) float64 {
	if minProgress <= 0 || minProgress > 1 {
		return 0.999
	}
	if floor := minProgress - pieceTolerance; floor > 0 {
		return floor
	}
	return 0
}

// judgeRecheck decides what to do with a migration whose pull has already replied DONE. Pure
// (no database, no own clock) to be testable via table.
//
// Rules, in order:
//  1. Only a read AFTER the DONE is valid (v.syncedAt > pullDoneAt). A line synced
//     before might be from before the recheck — the new Agent replies DONE right after
//     triggering it, and the same applies to an old line left by another copy.
//  2. Proven = state that proves progress (provesProgress) and progress >= floor. The
//     state condition is not cosmetic: during the recheck qBittorrent reports the
//     VERIFICATION progress as progress, not what the torrent has — a 70% recheck of a
//     60% partial torrent would already "pass" the floor without having finished verifying the rest.
//  3. Not proven, but progress went up: renew the stall window.
//  4. Stopped going up due to stall → failure ("recheck stalled", the same diagnosis as before).
//  5. Passed the ceiling since the DONE → failure (oscillating progress that never arrives).
func judgeRecheck(v destView, minProgress float64, st *verifyState, pullDoneAt, now time.Time, stall, ceiling time.Duration) (verdict, string) {
	floor := progressFloor(minProgress)
	fresh := v.found && v.syncedAt.After(pullDoneAt)
	if fresh {
		if provesProgress(v.status) && v.progress >= floor {
			return verdictDone, ""
		}
		if v.progress > st.best+progressEpsilon {
			st.best = v.progress
			st.advancedAt = now
		}
	}
	state, progress := v.status, v.progress
	if !fresh {
		state, progress = "unread", 0
	}
	if stalled := now.Sub(st.advancedAt); stalled >= stall {
		return verdictFail, fmt.Sprintf("destination recheck stalled: no progress for %s (state=%q progress=%.3f, required=%.3f)",
			stalled.Round(time.Second), state, progress, floor)
	}
	if waited := now.Sub(pullDoneAt); waited >= ceiling {
		return verdictFail, fmt.Sprintf("destination did not reach source progress in %s (state=%q progress=%.3f, required=%.3f)",
			ceiling, state, progress, floor)
	}
	return verdictWait, ""
}

// verifyStates stores the verifyState per migration. Mutex because the reconciler runs in a
// goroutine but tests (and a potential second caller) might invoke Reconcile directly.
type verifyStates struct {
	mu sync.Mutex
	m  map[string]*verifyState
}

// of returns the migration state, creating it the first time THIS process sees it: the
// stall window starts there (best=-1 so the 1st read always counts as progress).
func (v *verifyStates) of(migID string, now time.Time) *verifyState {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.m == nil {
		v.m = map[string]*verifyState{}
	}
	st, ok := v.m[migID]
	if !ok {
		st = &verifyState{best: -1, advancedAt: now}
		v.m[migID] = st
	}
	return st
}

func (v *verifyStates) forget(migID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.m, migID)
}

// keepOnly discards the state of migrations that are no longer in flight (finished via another
// path, e.g.: promote failure) — without this the map would only grow.
func (v *verifyStates) keepOnly(inFlight map[string]bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for id := range v.m {
		if !inFlight[id] {
			delete(v.m, id)
		}
	}
}

// destViewOf reads the live line of the hash on the destination Node — the one SyncStates adopted when
// the destination Engine got the torrent.
func (s *Service) destViewOf(ctx context.Context, destID, hash string) (destView, error) {
	var v destView
	var syncedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT status, COALESCE(progress, 0), last_synced_at
		   FROM torrents
		  WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL
		  ORDER BY created_at DESC
		  LIMIT 1`, destID, hash).Scan(&v.status, &v.progress, &syncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return destView{}, nil
	}
	if err != nil {
		return destView{}, err
	}
	v.found = true
	if syncedAt != nil {
		v.syncedAt = *syncedAt
	}
	return v, nil
}

// verifyAfterPull is the reconciler step for a migration whose pull replied DONE:
// it observes the destination and concludes, aborts or waits. It's here — and no longer in the Agent — that the
// recheck watchdog lives (#79/#87).
func (s *Service) verifyAfterPull(ctx context.Context, migID, torrentID, sourceID, destID string, minProgress float64, pullDoneAt time.Time) error {
	var hash string
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(hash,'') FROM torrents WHERE id = $1`, torrentID).Scan(&hash); err != nil {
		return err
	}
	view, err := s.destViewOf(ctx, destID, hash)
	if err != nil {
		return fmt.Errorf("read torrent on destination: %w", err)
	}
	now := s.clock()
	v, reason := judgeRecheck(view, minProgress, s.verify.of(migID, now), pullDoneAt, now, s.stallWindow(), s.maxWait())
	switch v {
	case verdictDone:
		s.verify.forget(migID)
		return s.complete(ctx, migID, torrentID, sourceID, destID)
	case verdictFail:
		s.verify.forget(migID)
		return s.fail(ctx, migID, sourceID, destID, torrentID, reason)
	}
	return nil
}
