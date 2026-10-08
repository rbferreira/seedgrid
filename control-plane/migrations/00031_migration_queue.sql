-- +goose Up
-- ADR-031: migrations are now QUEUED before execution.
--
-- Before, Migrate() emitted the token and enqueued serve+pull immediately: a drain of N
-- torrents turned into N simultaneous transfers, thrashing the disk I/O right on
-- the Node being drained. Now the migration starts as QUEUED and the reconciler
-- promotes it to PENDING respecting the migration.max_concurrent tunable (ADR-023).
--
-- Schema consequence: the token only exists from PROMOTION onwards (generating it at queue
-- entry would make the 30min TTL run while the migration still waits its turn, expiring
-- before starting). Therefore token_sha256/token_expires_at now accept NULL.

ALTER TABLE torrent_migrations ALTER COLUMN token_sha256 DROP NOT NULL;
ALTER TABLE torrent_migrations ALTER COLUMN token_expires_at DROP NOT NULL;

-- Progress floor the destination must prove on recheck (ADR-031). It's a SNAPSHOT of the
-- source progress when the migration was requested — which is why it stays on the migration
-- row and is not read from the torrent at promotion time (the source may have progressed or
-- regressed while the migration waited in the queue). 1.0 = complete torrent, which requires
-- SEEDING at the destination exactly as in ADR-018.
ALTER TABLE torrent_migrations ADD COLUMN min_progress DOUBLE PRECISION NOT NULL DEFAULT 1.0;

-- "In flight" now includes queue wait: a torrent cannot have two migrations
-- competing for it, whether it is waiting (QUEUED) or executing (PENDING).
DROP INDEX IF EXISTS torrent_migrations_one_inflight;
CREATE UNIQUE INDEX torrent_migrations_one_inflight
    ON torrent_migrations (torrent_id)
    WHERE status IN ('QUEUED', 'PENDING');

-- Queue processing order (FIFO) and count of those running.
CREATE INDEX torrent_migrations_queue ON torrent_migrations (status, created_at);

COMMENT ON COLUMN torrent_migrations.status IS
    'QUEUED: aguardando vaga sob migration.max_concurrent (ADR-031). '
    'PENDING: comandos enfileirados (origem serve, destino puxa). '
    'COMPLETED: destino comprovou ter ao menos o que a origem tinha; origem removida. '
    'FAILED: transfer/recheck failed or token expired (the torrent stays at the source).';

-- +goose Down
ALTER TABLE torrent_migrations DROP COLUMN IF EXISTS min_progress;
DROP INDEX IF EXISTS torrent_migrations_queue;
DROP INDEX IF EXISTS torrent_migrations_one_inflight;
CREATE UNIQUE INDEX torrent_migrations_one_inflight
    ON torrent_migrations (torrent_id)
    WHERE status = 'PENDING';
-- The columns become NOT NULL again only if there is no QUEUED row (without token).
DELETE FROM torrent_migrations WHERE status = 'QUEUED';
ALTER TABLE torrent_migrations ALTER COLUMN token_sha256 SET NOT NULL;
ALTER TABLE torrent_migrations ALTER COLUMN token_expires_at SET NOT NULL;
