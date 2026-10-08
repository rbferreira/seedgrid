// Package migration orchestrates the migration of torrents between Seed Nodes (ADR-018):
// physical copy with direct pull destination←source, authorized by a one-time token
// issued by the CP. The CP never intermediates the bytes (doesn't become data-plane); it only
// commands the source to serve and the destination to pull, and finalizes when the destination
// proves it has what the source had — observed by the CP itself (verify.go, #87).
// If it fails, the torrent remains at the source (no loss).
package migration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/capacity"
	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
)

var (
	// ErrTorrentNotFound: torrent missing/removed.
	ErrTorrentNotFound = errors.New("torrent not found")
	// ErrNoTorrentFile: the CP does not yet have the .torrent persisted (waiting for backfill).
	ErrNoTorrentFile = errors.New("torrent without .torrent persisted in the CP (wait for backfill)")
	// ErrSameNode: destination is the same as the source.
	ErrSameNode = errors.New("destination Node is the same as the source")
	// ErrDestNotEligible: destination missing or offline.
	ErrDestNotEligible = errors.New("destination node not found or offline")
	// ErrSourceNoEndpoint: source has no known IP (the destination pulls from it).
	ErrSourceNoEndpoint = errors.New("source Node has no known endpoint for transfer")
	// ErrAlreadyMigrating: there is already an in-flight migration for this torrent.
	ErrAlreadyMigrating = errors.New("a migration is already in progress for this torrent")
	// ErrDestNoSpace: the destination Node has no space for the torrent (ADR-032).
	ErrDestNoSpace = errors.New("destination Node has no disk space for the torrent")
)

// tokenTTL is the validity of the one-time transfer token (short window: the
// copy is over LAN and starts immediately).
const tokenTTL = 30 * time.Minute

// Migration is a torrent migration exposed by the API.
type Migration struct {
	ID           string     `json:"id"`
	TorrentID    string     `json:"torrent_id"`
	SourceNodeID string     `json:"source_node_id"`
	DestNodeID   string     `json:"dest_node_id"`
	Status       string     `json:"status"`
	Error        *string    `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	// Cancelled distinguishes the migration that the OPERATOR stopped (FAILED with CancelReason) from
	// one that failed on its own — the Portal shows "Cancelled", not "Failed" (#87).
	Cancelled bool `json:"cancelled,omitempty"`
}

// Service orchestrates the migrations.
type Service struct {
	pool    *pgxpool.Pool
	cmds    *command.Service
	events  *events.Emitter
	migPort int          // port of the Agent's transfer server (source listens; destination connects)
	minFree func() int64 // free disk margin (ADR-032), read per call (hot-reload); nil = 0

	// Watchdog for the recheck at the destination (#87) — see verify.go. Providers read per pass
	// (hot-reload, ADR-023); nil = defaults.
	recheckStall func() time.Duration
	recheckMax   func() time.Duration
	verify       verifyStates     // ephemeral state of the watchdog, per migration
	now          func() time.Time // injectable for tests; nil = time.Now
}

// NewService creates the migration service. migPort is the port where the source Agent
// serves the files during the migration (the destination Agent connects to source_ip:migPort).
func NewService(pool *pgxpool.Pool, cmds *command.Service, emitter *events.Emitter, migPort int) *Service {
	if migPort <= 0 {
		migPort = 6900
	}
	return &Service{pool: pool, cmds: cmds, events: emitter, migPort: migPort}
}

// SetDiskMinFree injects the free disk margin provider (ADR-032), read per call
// (hot-reload). nil-safe.
func (s *Service) SetDiskMinFree(fn func() int64) { s.minFree = fn }

// hostname fetches the Node's hostname for the external notification (ADR-028) — same data
// that the Alert Worker already resolves in its alerts. Best-effort: "" if not found (the notify
// falls back to the raw id, harder to read but never breaks the notification).
func (s *Service) hostname(ctx context.Context, nodeID string) string {
	var h string
	_ = s.pool.QueryRow(ctx, `SELECT hostname FROM nodes WHERE id = $1`, nodeID).Scan(&h)
	return h
}

func (s *Service) diskMinFree() int64 {
	if s.minFree == nil {
		return 0
	}
	return s.minFree()
}

// Migrate starts the migration of a torrent to the destination Node. Idempotency: the
// unique index (torrent_id WHERE status='PENDING') prevents two in-flight migrations.
func (s *Service) Migrate(ctx context.Context, torrentID, destNodeID string) (Migration, error) {
	// Torrent + source + hash + name (name goes in the event text — backlog #7) +
	// progress (floor that the destination will have to prove — ADR-031) + size (destination
	// disk check — ADR-032).
	var sourceNodeID, hash, name string
	var progress float64
	var sizeBytes *int64
	err := s.pool.QueryRow(ctx,
		`SELECT node_id::text, COALESCE(hash,''), COALESCE(name,''), COALESCE(progress,0), size_bytes
		   FROM torrents WHERE id = $1 AND deleted_at IS NULL`,
		torrentID).Scan(&sourceNodeID, &hash, &name, &progress, &sizeBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Migration{}, ErrTorrentNotFound
	}
	if err != nil {
		return Migration{}, err
	}
	if hash == "" {
		return Migration{}, ErrNoTorrentFile // without a hash there is nothing to migrate
	}
	if destNodeID == sourceNodeID {
		return Migration{}, ErrSameNode
	}

	// The copy that the destination of an ongoing migration adopted during verification (#87) is
	// of the same hash and lives on the destination Node — to the CP it's ANOTHER torrent, and the "in-
	// flight" index (by torrent_id) doesn't see it. Migrating it back now would make two migrations
	// dispute the same content in opposite directions.
	var isDestCopy bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM torrent_migrations m JOIN torrents src ON src.id = m.torrent_id
		    WHERE m.status = 'PENDING' AND m.dest_node_id = $1 AND src.hash = $2 AND m.torrent_id <> $3)`,
		sourceNodeID, hash, torrentID).Scan(&isDestCopy); err != nil {
		return Migration{}, err
	}
	if isDestCopy {
		return Migration{}, ErrAlreadyMigrating
	}

	// Persisted .torrent (prerequisite): the destination re-adds from it. Here we only
	// CHECK for existence — the bytes are read on promotion (ADR-031), since the migration
	// might wait in the queue and loading them now would be holding them in memory in vain.
	var hasTorrentFile bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM torrent_files WHERE torrent_id = $1)`, torrentID).Scan(&hasTorrentFile); err != nil {
		return Migration{}, err
	}
	if !hasTorrentFile {
		return Migration{}, ErrNoTorrentFile
	}

	// Eligible destination (ONLINE) and source with an IP for the pull.
	var destOnline bool
	if err := s.pool.QueryRow(ctx,
		`SELECT status = 'ONLINE' FROM nodes WHERE id = $1 AND deleted_at IS NULL`,
		destNodeID).Scan(&destOnline); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Migration{}, ErrDestNotEligible
		}
		return Migration{}, err
	}
	if !destOnline {
		return Migration{}, ErrDestNotEligible
	}

	// Destination disk (ADR-032): does not migrate to a Node that cannot hold the TOTAL
	// SIZE of the torrent + margin. Unknown size (size_bytes NULL) or destination disk
	// not yet reported → does not block (there is no way to judge).
	if sizeBytes != nil && *sizeBytes > 0 {
		fits, known, _, ferr := capacity.Fits(ctx, s.pool, destNodeID, *sizeBytes, s.diskMinFree())
		if ferr != nil {
			return Migration{}, ferr
		}
		if known && !fits {
			return Migration{}, ErrDestNoSpace
		}
	}

	var sourceIP *string
	if err := s.pool.QueryRow(ctx,
		`SELECT last_ip FROM nodes WHERE id = $1 AND deleted_at IS NULL`, sourceNodeID).Scan(&sourceIP); err != nil {
		return Migration{}, err
	}
	if sourceIP == nil || *sourceIP == "" {
		return Migration{}, ErrSourceNoEndpoint
	}

	// The migration enters the QUEUE (QUEUED). The one that issues the token and queues the commands is the
	// reconciler, upon promotion, respecting migration.max_concurrent (ADR-031) — without this
	// a drain of N torrents would turn into N simultaneous transfers and break the I/O of the
	// disk of the Node being emptied. The token is NOT generated here on purpose: its
	// 30min TTL would run during the queue wait and expire before starting.
	var m Migration
	err = s.pool.QueryRow(ctx,
		`INSERT INTO torrent_migrations
		   (torrent_id, source_node_id, dest_node_id, status, min_progress)
		 VALUES ($1, $2, $3, 'QUEUED', $4)
		 RETURNING id::text, torrent_id::text, source_node_id::text, dest_node_id::text, status, error, created_at, completed_at`,
		torrentID, sourceNodeID, destNodeID, minProgress(progress)).
		Scan(&m.ID, &m.TorrentID, &m.SourceNodeID, &m.DestNodeID, &m.Status, &m.Error, &m.CreatedAt, &m.CompletedAt)
	if err != nil {
		// Collision in the "in-flight" index → there is already a migration waiting or running.
		return Migration{}, ErrAlreadyMigrating
	}

	if s.events != nil {
		_ = s.events.Emit(ctx, "TORRENT_MIGRATION_STARTED", "INFO", sourceNodeID,
			fmt.Sprintf("Torrent migration started: %s", label(name, hash)),
			map[string]any{
				"hash": hash, "name": name, "dest_node_id": destNodeID,
				"hostname": s.hostname(ctx, sourceNodeID), "dest_hostname": s.hostname(ctx, destNodeID),
			})
	}
	return m, nil
}

// DrainNode migrates all (non-removed) torrents from a Node to the destination.
// Returns the started migrations; torrents without a persisted .torrent yet are
// skipped (the caller can reprocess after the backfill).
func (s *Service) DrainNode(ctx context.Context, sourceNodeID, destNodeID string) ([]Migration, error) {
	if destNodeID == sourceNodeID {
		return nil, ErrSameNode
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text FROM torrents WHERE node_id = $1 AND deleted_at IS NULL AND hash IS NOT NULL`,
		sourceNodeID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Migration, 0, len(ids))
	for _, id := range ids {
		m, err := s.Migrate(ctx, id, destNodeID)
		if err != nil {
			// Skips what cannot be migrated now (no .torrent, already in-flight, doesn't fit in the
			// destination — ADR-032); proceeds with the rest. A destination that cannot hold everything still
			// receives what fits; the rest stays at the source and the alert covers it.
			if errors.Is(err, ErrNoTorrentFile) || errors.Is(err, ErrAlreadyMigrating) || errors.Is(err, ErrDestNoSpace) {
				continue
			}
			return out, err
		}
		out = append(out, m)
	}
	return out, nil
}

// List returns the most recent migrations.
func (s *Service) List(ctx context.Context, limit int) ([]Migration, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+migrationColumns+` FROM torrent_migrations ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Migration, 0)
	for rows.Next() {
		m, err := scanMigration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReconcileAndPromote is the worker's full pass (ADR-031): first it closes what
// has finished (freeing up slots), then it promotes from the queue what fits under the ceiling.
// Order matters — reconciling first means that a migration completed in this cycle already
// yields its slot to the next one in the same cycle.
func (s *Service) ReconcileAndPromote(ctx context.Context, maxConcurrent int) error {
	recErr := s.Reconcile(ctx)
	promErr := s.promoteQueued(ctx, maxConcurrent)
	return errors.Join(recErr, promErr)
}

// Reconcile advances PENDING migrations according to the outcome of the pull command
// (destination). It is called periodically by a CP worker.
//
// ⚠️ The pull's DONE does NOT complete the migration (#87). A new Agent replies DONE as soon as the
// copy finishes and the recheck has been triggered — the destination is not yet verified. With
// DONE, the reconciler starts to OBSERVE the torrent at the destination (verifyAfterPull) and only then
// completes, aborts, or waits. The old Agent, which only replies DONE after the recheck,
// falls into the same path: the observation is the same and already finds the destination verified.
func (s *Service) Reconcile(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id::text, m.torrent_id::text, m.source_node_id::text, m.dest_node_id::text,
		        m.min_progress, c.status, COALESCE(c.error,''),
		        COALESCE(c.finished_at, c.created_at), (m.token_expires_at < now())
		   FROM torrent_migrations m
		   JOIN commands c ON c.id = m.pull_command_id
		  WHERE m.status = 'PENDING'`)
	if err != nil {
		return err
	}
	type pending struct {
		id, torrentID, sourceID, destID, cmdStatus, cmdErr string
		minProgress                                        float64
		pullDoneAt                                         time.Time
		expired                                            bool
	}
	var items []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.torrentID, &p.sourceID, &p.destID, &p.minProgress,
			&p.cmdStatus, &p.cmdErr, &p.pullDoneAt, &p.expired); err != nil {
			rows.Close()
			return err
		}
		items = append(items, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// ⚠️ A migration that fails to finalize MUST NOT block the others (head-of-line
	// blocking): previously, a `return err` here aborted the entire pass, and since the error
	// repeated every cycle, a "cork" migration froze the whole queue (incident
	// 2026-07-18). Now we accumulate the errors and CONTINUE; RunReconciler logs the aggregate.
	var errs []error
	inFlight := make(map[string]bool, len(items))
	for _, p := range items {
		inFlight[p.id] = true
		var err error
		switch {
		case p.cmdStatus == "DONE":
			err = s.verifyAfterPull(ctx, p.id, p.torrentID, p.sourceID, p.destID, p.minProgress, p.pullDoneAt)
		case p.cmdStatus == "FAILED":
			msg := p.cmdErr
			if msg == "" {
				msg = "transfer failed on the destination"
			}
			err = s.fail(ctx, p.id, p.sourceID, p.destID, p.torrentID, msg)
		case p.expired:
			// The token's deadline covers the TRANSFER (pull still in progress). After DONE the
			// wait is for the recheck watchdog (verify.go), with its own deadline.
			err = s.fail(ctx, p.id, p.sourceID, p.destID, p.torrentID, "transfer token expired before completion")
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("migration %s: %w", p.id, err))
		}
	}
	s.verify.keepOnly(inFlight)
	return errors.Join(errs...)
}

// RunReconciler runs Reconcile periodically until the context is canceled.
// interval is a PROVIDER read every cycle (ADR-023): the tunable
// workers.reconciler_interval can change in the panel and takes effect without redeploy.
// beat is optional (variadic to not break callers/tests): heartbeat marked every
// cycle, exposed in the health API (see internal/health).
// maxConcurrent is a PROVIDER read every pass (ADR-031/023): the tunable
// migration.max_concurrent takes effect without redeploy. nil = 1 (serial), the safe default.
func (s *Service) RunReconciler(ctx context.Context, interval func() time.Duration, maxConcurrent func() int, logger *slog.Logger, beat ...func()) {
	cur := interval()
	if cur <= 0 {
		cur = 15 * time.Second
	}
	logger.Info("migration reconciler started", "interval", cur.String())
	ticker := time.NewTicker(cur)
	defer ticker.Stop()
	mark := func() {
		for _, b := range beat {
			if b != nil {
				b()
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			logger.Info("migration reconciler stopped")
			return
		case <-ticker.C:
			maxc := 1
			if maxConcurrent != nil {
				if v := maxConcurrent(); v > 0 {
					maxc = v
				}
			}
			if err := s.ReconcileAndPromote(ctx, maxc); err != nil {
				logger.Error("reconcile migrations", "error", err)
			}
			mark()
			if next := interval(); next > 0 && next != cur {
				cur = next
				ticker.Reset(cur)
				logger.Info("reconciler interval updated", "interval", cur.String())
			}
		}
	}
}

// promoteQueued promotes QUEUED → PENDING migrations while there is headroom under the ceiling
// of concurrency (ADR-031), in FIFO order. This is where the one-time token is issued and the
// migrate_serve/migrate_pull commands are enqueued — not at creation — so that the
// token TTL only starts ticking when the transfer actually takes place.
//
// maxConcurrent is read PER PASS (catalog provider, ADR-023): changing the tunable in the
// dashboard takes effect without a redeploy.
func (s *Service) promoteQueued(ctx context.Context, maxConcurrent int) error {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	var running int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM torrent_migrations WHERE status = 'PENDING'`).Scan(&running); err != nil {
		return err
	}
	slots := maxConcurrent - running
	if slots <= 0 {
		return nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id::text, torrent_id::text, source_node_id::text, dest_node_id::text
		   FROM torrent_migrations
		  WHERE status = 'QUEUED'
		  ORDER BY created_at
		  LIMIT $1`, slots)
	if err != nil {
		return err
	}
	type queued struct{ id, torrentID, sourceID, destID string }
	var items []queued
	for rows.Next() {
		var q queued
		if err := rows.Scan(&q.id, &q.torrentID, &q.sourceID, &q.destID); err != nil {
			rows.Close()
			return err
		}
		items = append(items, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// A promotion that fails must not block the others (same lesson from the Reconcile head-of-line
	// blocking — 2026-07-18 incident): accumulate and move on.
	var errs []error
	for _, q := range items {
		if err := s.promoteOne(ctx, q.id, q.torrentID, q.sourceID, q.destID); err != nil {
			errs = append(errs, fmt.Errorf("promote migration %s: %w", q.id, err))
		}
	}
	return errors.Join(errs...)
}

// promoteOne issues the token and enqueues serve+pull for ONE migration from the queue.
func (s *Service) promoteOne(ctx context.Context, migID, torrentID, sourceID, destID string) error {
	var hash string
	var minProg float64
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(t.hash,''), m.min_progress
		   FROM torrent_migrations m JOIN torrents t ON t.id = m.torrent_id
		  WHERE m.id = $1`, migID).Scan(&hash, &minProg); err != nil {
		return err
	}
	if hash == "" {
		return s.fail(ctx, migID, sourceID, destID, torrentID, "torrent without hash at execution time")
	}
	var torrentFile []byte
	if err := s.pool.QueryRow(ctx,
		`SELECT data FROM torrent_files WHERE torrent_id = $1`, torrentID).Scan(&torrentFile); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.fail(ctx, migID, sourceID, destID, torrentID, "the .torrent no longer exists in CP before execution")
		}
		return err
	}
	// The source may have gone down while the migration was waiting its turn.
	var sourceIP *string
	if err := s.pool.QueryRow(ctx,
		`SELECT last_ip FROM nodes WHERE id = $1 AND deleted_at IS NULL`, sourceID).Scan(&sourceIP); err != nil {
		return err
	}
	if sourceIP == nil || *sourceIP == "" {
		return s.fail(ctx, migID, sourceID, destID, torrentID, "source Node has no endpoint at execution time")
	}

	token, err := generateToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(tokenTTL)

	servePayload, _ := json.Marshal(map[string]any{
		"hash": hash, "token": token, "expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
	serveCmdID, err := s.cmds.Enqueue(ctx, sourceID, "migrate_serve", servePayload)
	if err != nil {
		return fmt.Errorf("enqueue migrate_serve: %w", err)
	}
	pullPayload, _ := json.Marshal(map[string]any{
		"hash":         hash,
		"token":        token,
		"source_host":  *sourceIP,
		"source_port":  s.migPort,
		"torrent_file": base64.StdEncoding.EncodeToString(torrentFile),
		// Progress floor that the destination must prove via recheck (ADR-031).
		// Complete Torrent sends ~1.0; partial sends downloaded fraction and the migration completes
		// there, with the destination continuing the rest of the download. The CP checks the floor
		// (verify.go) — the Agent only uses it in the legacy path, without async_recheck.
		"min_progress": minProg,

		// #87: the CP watches the destination's recheck (verify.go). An Agent that understands the field
		// replies DONE as soon as the copy finishes and the recheck is triggered, instead of
		// holding the Node's command queue waiting for it; an older Agent ignores the
		// field and waits as before — the CP still completes it, based on the torrent state.
		"async_recheck": true,
	})
	pullCmdID, err := s.cmds.Enqueue(ctx, destID, "migrate_pull", pullPayload)
	if err != nil {
		return fmt.Errorf("enqueue migrate_pull: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE torrent_migrations
		    SET status = 'PENDING', token_sha256 = $2, token_expires_at = $3,
		        serve_command_id = $4, pull_command_id = $5, updated_at = now()
		  WHERE id = $1 AND status = 'QUEUED'`,
		migID, sha256hex(token), expiresAt, serveCmdID, pullCmdID)
	return err
}

// complete finalizes a successful migration: tells the source to remove the torrent
// (with data), restores the torrent on the destination Node and marks the migration completed.
//
// ⚠️ ATOMICITY (2026-07-18 incident): the three effects — enqueuing the remove from the
// source, restoring/soft-deleting the row and marking COMPLETED — run in a SINGLE transaction,
// and the remove is enqueued LAST. The reconciler only leaves PENDING when the migration
// turns COMPLETED; if any step fails, BEFORE the remove was already enqueued and the
// migration stayed PENDING, so the reconciler re-executed complete() every cycle
// (~15s) and re-enqueued the remove indefinitely — 27.293 remove_torrent in 5 days
// on one migration whose restore collided with uk_torrents_node_hash. With everything in the tx:
// failed → rollback → nothing enqueued, clean retry; succeeded → 1 remove + COMPLETED
// commit together, and the loop is impossible.
func (s *Service) complete(ctx context.Context, migID, torrentID, sourceID, destID string) error {
	var hash, name string
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(hash,''), COALESCE(name,'') FROM torrents WHERE id = $1`, torrentID).Scan(&hash, &name); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Restore the torrent on the destination. Normal case: move the row (same torrent_id) to
	// destination Node. HOWEVER, if the destination ALREADY has a live row for the hash — SyncStates
	// adopted a copy the destination itself downloaded (e.g., the migration fell back to downloading from the
	// swarm) —, the UPDATE would collide with uk_torrents_node_hash (node_id, hash WHERE
	// deleted_at IS NULL) and lock the entire queue (2026-07-18 incident). In this case the
	// torrent is ALREADY at the destination: just soft-delete the source row and finish.
	var destHasRow bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM torrents
		   WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL AND id <> $3)`,
		destID, hash, torrentID).Scan(&destHasRow); err != nil {
		return fmt.Errorf("check torrent on destination: %w", err)
	}
	if destHasRow {
		if _, err := tx.Exec(ctx,
			`UPDATE torrents SET deleted_at = now(), updated_at = now() WHERE id = $1 AND deleted_at IS NULL`,
			torrentID); err != nil {
			return fmt.Errorf("soft-delete of source (destination already has the torrent): %w", err)
		}
	} else if _, err := tx.Exec(ctx,
		`UPDATE torrents SET node_id = $2, updated_at = now() WHERE id = $1`, torrentID, destID); err != nil {
		return fmt.Errorf("restore torrent on destination: %w", err)
	}
	// Conditional: if the migration is no longer PENDING, someone ended it between the reconciler's
	// read and here — the operator canceling (#87). So we DO NOT complete: the rollback undoes
	// the above restore and nothing was enqueued (the source remove comes last, below). Without
	// the condition, COMPLETED would overwrite the cancel, which already told the source to resume and
	// delete the destination copy.
	tag, err := tx.Exec(ctx,
		`UPDATE torrent_migrations SET status = 'COMPLETED', completed_at = now(), updated_at = now()
		  WHERE id = $1 AND status = 'PENDING'`,
		migID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	// Source removes from the Engine (with data) — enqueued LAST, in the same tx, so it
	// only exists if the migration actually completes (see atomicity note above).
	removePayload, _ := json.Marshal(map[string]any{"hash": hash, "delete_files": true})
	if _, err := s.cmds.EnqueueTx(ctx, tx, sourceID, "remove_torrent", removePayload); err != nil {
		return fmt.Errorf("queue remove on source: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if s.events != nil {
		_ = s.events.Emit(ctx, "TORRENT_MIGRATED", "INFO", destID,
			fmt.Sprintf("Torrent migrated to destination Node: %s", label(name, hash)),
			map[string]any{
				"hash": hash, "name": name, "source_node_id": sourceID,
				"hostname": s.hostname(ctx, destID), "source_hostname": s.hostname(ctx, sourceID),
			})
	}
	return nil
}

// fail aborts a migration without loss (the torrent remains on the source) and cleans up the
// partial copy left on the destination (#79). Idempotent: a migration that has already
// finished (COMPLETED/FAILED) is not touched.
func (s *Service) fail(ctx context.Context, migID, sourceID, destID, torrentID, reason string) error {
	_, err := s.abort(ctx, migID, sourceID, destID, torrentID, reason, false)
	return err
}

// abort is the ONLY lossless termination path for an in-flight migration — the watchdog,
// the token deadline, the pull failure and the operator cancellation (#87) go through here, and
// therefore have exactly the same effect: migration FAILED with the reason, source resumed,
// destination copy discarded.
//
// Returns false (no effect at all) when the migration was no longer in flight: someone
// finished it first — the reconciler completing it, or the operator canceling simultaneously. The
// QUEUED/PENDING → FAILED transition is conditional (and PENDING → COMPLETED, in complete(),
// too), so they don't race each other: otherwise a cancellation arriving at the
// exact moment the reconciler completes would overwrite COMPLETED and tell it to delete the ONLY
// copy (the destination's), with the source already removed.
//
// All in one transaction — the transition, the command clearing and the cleanup commands —,
// for the same reason as complete(): failed midway, rollback; nothing gets partially enqueued
// and the migration stays in flight for the next cycle to try again.
//
// byOperator (cancellation) adds a rule: serve/pull that the Agent HAS NOT YET
// claimed (PENDING) are downgraded to FAILED, so they never run — a pull
// "forgotten" in the queue would copy gigabytes of a torrent nobody wants to migrate anymore. And,
// since what never ran left no trace, there is nothing to resume on the source nor to
// discard on the destination because of it. But a claimed command (DISPATCHED, the Agent
// is in the middle of the copy) cannot be interrupted: it runs to the end and the cleanup,
// enqueued behind it in the Node's serial Poller, undoes it.
func (s *Service) abort(ctx context.Context, migID, sourceID, destID, torrentID, reason string, byOperator bool) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var serveID, pullID *string
	err = tx.QueryRow(ctx,
		`UPDATE torrent_migrations
		    SET status = 'FAILED', error = $2, completed_at = now(), updated_at = now()
		  WHERE id = $1 AND status IN ('QUEUED', 'PENDING')
		RETURNING serve_command_id::text, pull_command_id::text`,
		migID, reason).Scan(&serveID, &pullID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// "got to run" = the command was enqueued (as it always was) AND, on cancellation, was not
	// cleared by us before the Agent claimed it.
	served, pulled := serveID != nil, pullID != nil
	if byOperator {
		var ids []string
		for _, id := range []*string{serveID, pullID} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
		if len(ids) > 0 {
			rows, err := tx.Query(ctx,
				`UPDATE commands SET status = 'FAILED', error = $2, finished_at = now()
				  WHERE id = ANY($1::uuid[]) AND status = 'PENDING'
				RETURNING id::text`, ids, reason)
			if err != nil {
				return false, fmt.Errorf("download commands not yet delivered: %w", err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return false, err
				}
				if serveID != nil && id == *serveID {
					served = false
				}
				if pullID != nil && id == *pullID {
					pulled = false
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return false, err
			}
		}
	}
	if pulled {
		if err := s.enqueueDestCleanup(ctx, tx, destID, torrentID); err != nil {
			return false, err
		}
	}
	if served {
		if err := s.enqueueSourceResume(ctx, tx, sourceID, torrentID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	if s.events != nil {
		var hash, name string
		_ = s.pool.QueryRow(ctx, `SELECT COALESCE(hash,''), COALESCE(name,'') FROM torrents WHERE id = $1`, torrentID).Scan(&hash, &name)
		detail := map[string]any{"hash": hash, "name": name, "dest_node_id": destID, "reason": reason,
			"hostname": s.hostname(ctx, sourceID), "cancelled": byOperator}
		_ = s.events.Emit(ctx, "TORRENT_MIGRATION_FAILED", "WARNING", sourceID,
			fmt.Sprintf("Torrent migration aborted (remains on source): %s", label(name, hash)), detail)
	}
	return true, nil
}

// enqueueDestCleanup enqueues, on the destination Node, the removal (with data) of the copy left
// by an aborted migration (#79). Called only when the pull actually ran.
//
// When the pull runs, the data is already in the destination's `/data` — and the source remains the
// official copy (enqueueSourceResume brings it back online). Without this cleanup, the destination
// would accumulate the entire file on disk and a second record for the same hash in the Cluster
// (11 GB orphaned in the case that originated the issue), which nobody removed.
//
// It is safe to remove WITH the data precisely because the migration outcome was FAILED: the
// ADR-018 contract is "the torrent remains at the source", so what is at the destination is a
// draft of a transfer that did not count. The row in `torrents` is not touched here —
// `remove_torrent` makes the destination Engine forget the hash and SyncStates reconciles the
// inventory (#19), without rushing adoption.
//
// Conservative: the caller only triggers if the pull was actually enqueued (and delivered).
// A migration that died before that (no endpoint, no space, token not issued) left nothing
// at the destination, and enqueuing a remove there would tell the Node to delete a torrent that
// it may have for another reason — including a legitimate pre-existing copy.
func (s *Service) enqueueDestCleanup(ctx context.Context, tx pgx.Tx, destID, torrentID string) error {
	var hash string
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(hash,'') FROM torrents WHERE id = $1`, torrentID).Scan(&hash); err != nil {
		return err
	}
	if hash == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"hash": hash, "delete_files": true})
	if err != nil {
		return err
	}
	if _, err := s.cmds.EnqueueTx(ctx, tx, destID, "remove_torrent", payload); err != nil {
		return fmt.Errorf("queue cleanup of the copy on the destination: %w", err)
	}
	return nil
}

// enqueueSourceResume brings the torrent back online at the source after an aborted migration.
//
// The source is STOPPED by the Agent when running migrate_serve (issue #8), to avoid
// copying in-flight data. On the happy path this does not matter — the source is removed in
// complete(). When the migration fails, however, "the torrent remains at the source"
// (ADR-018) is only true if it starts running again: without this, a failed drain
// would leave torrents silently stopped.
//
// Best-effort and deliberately conservative — the caller only triggers if the serve was actually
// enqueued (and delivered; otherwise nothing was paused), and here it does not resume when:
//   - the torrent is in admission-hold (`admission_paused_at`): the admission controller is
//     what controls the queue (ADR-021/024), and resuming here would bypass the hold;
//   - the torrent disappeared/was soft-deleted.
func (s *Service) enqueueSourceResume(ctx context.Context, tx pgx.Tx, sourceID, torrentID string) error {
	var hash string
	var admissionHeld bool
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(hash,''), admission_paused_at IS NOT NULL
		   FROM torrents WHERE id = $1 AND deleted_at IS NULL`, torrentID).Scan(&hash, &admissionHeld); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("read torrent to resume on the source: %w", err)
	}
	if hash == "" || admissionHeld {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{"hash": hash})
	if _, err := s.cmds.EnqueueTx(ctx, tx, sourceID, "resume_torrent", payload); err != nil {
		return fmt.Errorf("queue resume_torrent on source: %w", err)
	}
	return nil
}

// minProgress converts the source progress into the floor sent to the destination (ADR-031).
// Unknown progress (0 — inventory has not reported yet) becomes 1.0: when in doubt
// we require the entire torrent, which is the conservative behavior of ADR-018.
func minProgress(progress float64) float64 {
	if progress <= 0 || progress > 1 {
		return 1.0
	}
	return progress
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// label returns a human-readable torrent label for Event text (backlog #7):
// the name, or the short hash when there is no name.
func label(name, hash string) string {
	if name != "" {
		return name
	}
	if len(hash) >= 8 {
		return hash[:8]
	}
	return hash
}
