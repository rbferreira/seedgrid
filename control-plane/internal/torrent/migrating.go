package torrent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrMigrationInFlight: the action was refused because the torrent is in the middle of a
// migration between Nodes (#87). The API maps it to 409. It is always returned as a
// *MigrationInFlightError, which says WHICH migration — and therefore which one to cancel.
var ErrMigrationInFlight = errors.New("torrent in migration")

// MigrationInFlightError is the ErrMigrationInFlight with the id of the migration holding the
// torrent. The operator isn't left without an out: the message says it can be canceled.
type MigrationInFlightError struct{ MigrationID string }

func (e *MigrationInFlightError) Error() string {
	return "the torrent is in migration; wait for completion or cancel the migration " +
		"(Portal: Alerts & Events → Recent migrations → Cancel migration; " +
		"API: POST /api/v1/migrations/" + e.MigrationID + "/cancel)"
}

// Is makes errors.Is(err, ErrMigrationInFlight) work for the typed error.
func (e *MigrationInFlightError) Is(target error) bool { return target == ErrMigrationInFlight }

// inMigration returns the id of the PENDING migration (transferring or with destination recheck
// in progress) that the torrent is part of — "" if it isn't part of any:
//   - as the migrated torrent — the source row, which the source Agent stopped serving; or
//   - as the COPY of the same hash on the destination Node, which SyncStates adopts once the
//     destination Engine gets the torrent. For the CP it's another row (another id), but it's
//     exactly the one being verified.
//
// Why it exists (#87): the Agent no longer holds the destination command queue while
// waiting for the recheck, so a pause/resume/remove on this copy would now be EXECUTED
// in the middle of verification (before it queued behind migrate_pull) — pausing drops
// the recheck and removing deletes the copy the CP is about to validate. On source, the
// same goes for resume (copies in-flight data, the reason for the #8 pause) or remove
// (migration completes later and resurrects the torrent at the destination).
//
// Only PENDING: a QUEUED migration hasn't changed anything, and blocking the torrent during
// a drain queue (hours) leaves the operator unable to act on it needlessly. And the way out
// exists: the in-flight migration can be canceled (migration.Cancel).
func (s *Service) inMigration(ctx context.Context, id, nodeID, hash string) (string, error) {
	var migID string
	err := s.pool.QueryRow(ctx,
		`SELECT m.id::text FROM torrent_migrations m
		   JOIN torrents src ON src.id = m.torrent_id
		  WHERE m.status = 'PENDING'
		    AND (m.torrent_id = $1 OR (m.dest_node_id = $2 AND src.hash = $3))
		  ORDER BY m.created_at
		  LIMIT 1`,
		id, nodeID, hash).Scan(&migID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return migID, err
}
