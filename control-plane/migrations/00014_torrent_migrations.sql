-- +goose Up
-- Torrent migration between Seed Nodes (ADR-018): physical copy with direct pull
-- destination←source, orchestrated and authorized by the CP. One line per attempt to
-- migrate a torrent to a destination Node (idempotent by torrent+destination
-- while in flight). The transfer token is one-time and stored only as a hash.
CREATE TABLE torrent_migrations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    torrent_id       UUID NOT NULL REFERENCES torrents (id) ON DELETE CASCADE,
    source_node_id   UUID NOT NULL REFERENCES nodes (id),
    dest_node_id     UUID NOT NULL REFERENCES nodes (id),
    -- PENDING: commands queued (source serves, destination pulls).
    -- COMPLETED: destination confirmed SEEDING, node_id changed, source removed.
    -- FAILED: transfer/recheck failed or token expired (torrent stays at source).
    status           TEXT NOT NULL DEFAULT 'PENDING',
    token_sha256     TEXT NOT NULL,                 -- one-time transfer token (hash)
    token_expires_at TIMESTAMPTZ NOT NULL,
    serve_command_id UUID REFERENCES commands (id), -- migrate_serve (source)
    pull_command_id  UUID REFERENCES commands (id), -- migrate_pull (destination)
    error            TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at     TIMESTAMPTZ
);

-- At most one in-flight (PENDING) migration per torrent — prevents races of
-- multiple destinations competing for the same torrent.
CREATE UNIQUE INDEX torrent_migrations_one_inflight
    ON torrent_migrations (torrent_id)
    WHERE status = 'PENDING';

CREATE INDEX torrent_migrations_status ON torrent_migrations (status);

-- +goose Down
DROP TABLE IF EXISTS torrent_migrations;
