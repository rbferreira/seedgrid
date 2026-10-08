-- +goose Up
-- Canonical torrent metadata (.torrent) stored in the CP — source of truth for
-- migration between Nodes and re-provisioning (ADR-018). Small files (KB to
-- a few MB) → bytea in Postgres (ADR-006: Postgres is the official state). Separated
-- from torrents so the blob isn't fetched in listings.
CREATE TABLE torrent_files (
    torrent_id UUID PRIMARY KEY REFERENCES torrents (id) ON DELETE CASCADE,
    data       BYTEA NOT NULL,
    byte_size  INTEGER NOT NULL,
    source     TEXT NOT NULL DEFAULT 'upload', -- upload | engine_export
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS torrent_files;
