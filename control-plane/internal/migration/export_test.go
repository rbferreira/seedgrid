package migration

import (
	"context"
	"time"
)

// SetClock injects the recheck watchdog clock. It only exists in tests: the watchdog
// measures minutes, and the integration tests (migration_test package) need to advance
// time without sleeping.
func (s *Service) SetClock(fn func() time.Time) { s.now = fn }

// AbortForTest and CompleteForTest execute both terminations with a STALE SNAPSHOT of the
// migration — what the reconciler (or cancellation handler) read before the other end
// finished the migration. They deterministically reproduce the race that conditional
// transitions exist to win, without depending on scheduling luck.
func (s *Service) AbortForTest(ctx context.Context, m Migration) (bool, error) {
	return s.abort(ctx, m.ID, m.SourceNodeID, m.DestNodeID, m.TorrentID, CancelReason, true)
}

func (s *Service) CompleteForTest(ctx context.Context, m Migration) error {
	return s.complete(ctx, m.ID, m.TorrentID, m.SourceNodeID, m.DestNodeID)
}
