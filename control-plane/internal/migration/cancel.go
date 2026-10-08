package migration

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
)

// Migration cancellation by the operator (#87).
//
// With recheck outside the command, a PENDING migration can last hours (the check has
// a 6 h cap), and during all this time the operator's actions on the torrent are rejected
// (torrent.ErrMigrationInFlight). Without an exit, the operator would be stuck. Canceling is that
// exit — and it's the SAME safe watchdog shutdown (abort): migration FAILED with the reason,
// source resumed, destination copy discarded.

// CancelReason is the reason recorded on a migration canceled by the operator. It stays in
// torrent_migrations.error — no new state in the enum, which many consumers read as
// "terminal = COMPLETED or FAILED" (the deletion reconciliation of SyncStates, the Portal) —
// and it's what Migration.Cancelled recognizes.
const CancelReason = "canceled by operator"

var (
	// ErrMigrationNotFound: nonexistent migration (or malformed id).
	ErrMigrationNotFound = errors.New("migration not found")
	// ErrMigrationFinished: the migration has already finished (completed, or failed on its own).
	ErrMigrationFinished = errors.New("migration already finished: nothing to cancel")
	// ErrMigrationAlreadyCancelled: cancellation was already done before. Distinct from
	// ErrMigrationFinished for the client repeating the call (double click, lost
	// response) to know that the requested effect is already in place.
	ErrMigrationAlreadyCancelled = errors.New("migration has already been cancelled")
)

var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// migrationColumns/scanMigration are the single read of a row from torrent_migrations.
const migrationColumns = `id::text, torrent_id::text, source_node_id::text, dest_node_id::text,
	status, error, created_at, completed_at`

func scanMigration(row pgx.Row) (Migration, error) {
	var m Migration
	if err := row.Scan(&m.ID, &m.TorrentID, &m.SourceNodeID, &m.DestNodeID, &m.Status, &m.Error, &m.CreatedAt, &m.CompletedAt); err != nil {
		return Migration{}, err
	}
	m.Cancelled = m.Status == "FAILED" && m.Error != nil && *m.Error == CancelReason
	return m, nil
}

// Get returns a migration by id (ErrMigrationNotFound if it doesn't exist).
func (s *Service) Get(ctx context.Context, migID string) (Migration, error) {
	if !uuidShape.MatchString(migID) {
		return Migration{}, ErrMigrationNotFound
	}
	m, err := scanMigration(s.pool.QueryRow(ctx,
		`SELECT `+migrationColumns+` FROM torrent_migrations WHERE id = $1`, migID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Migration{}, ErrMigrationNotFound
	}
	return m, err
}

// finishedError says why a finished migration cannot be canceled (nil = in flight).
func finishedError(m Migration) error {
	switch {
	case m.Cancelled:
		return ErrMigrationAlreadyCancelled
	case m.Status == "COMPLETED" || m.Status == "FAILED":
		return ErrMigrationFinished
	}
	return nil
}

// Cancel terminates an in-flight migration (QUEUED or PENDING) at the operator's request.
//
// What happens in each phase:
//   - QUEUED: nothing was queued — turns FAILED and disappears from the queue.
//   - PENDING, commands not yet delivered: downloaded as FAILED to never run; nothing
//     to undo, because nothing ran.
//   - PENDING during COPY (the destination Agent is in migrate_pull): the command cannot
//     be interrupted — the Agent has no cancel channel mid-command, and
//     ADR-014 doesn't provide for it. The migration turns FAILED immediately, the source is
//     resumed, and the copy removal (with data) is queued BEHIND the pull in the serial
//     Poller of the destination: the copy finishes, and is discarded right after. It's the
//     same effect as the watchdog.
//   - PENDING during recheck VERIFICATION: same, and the remove interrupts the recheck.
//
// Finished migration (COMPLETED, or FAILED for another reason) responds with
// ErrMigrationFinished; already canceled, ErrMigrationAlreadyCancelled. In both NOTHING
// is re-executed: repeating the cancellation is safe.
func (s *Service) Cancel(ctx context.Context, migID string) (Migration, error) {
	cur, err := s.Get(ctx, migID)
	if err != nil {
		return Migration{}, err
	}
	if err := finishedError(cur); err != nil {
		return cur, err
	}
	did, err := s.abort(ctx, cur.ID, cur.SourceNodeID, cur.DestNodeID, cur.TorrentID, CancelReason, true)
	if err != nil {
		return Migration{}, err
	}
	after, err := s.Get(ctx, cur.ID)
	if err != nil {
		return Migration{}, err
	}
	if !did {
		// Lost the race: the reconciler finished (or failed) the migration between the read and
		// UPDATE. Nothing was done by us.
		if err := finishedError(after); err != nil {
			return after, err
		}
		return after, ErrMigrationFinished
	}
	return after, nil
}
