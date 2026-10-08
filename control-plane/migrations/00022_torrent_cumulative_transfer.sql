-- +goose Up
-- Cumulative transfer counters per torrent (backlog #14). The fields
-- uploaded_bytes/downloaded_bytes reflect the current Engine (qBittorrent) SESSION,
-- which RESETS whenever the torrent is re-added — useless for a stable aggregate ratio.
-- These columns only accumulate positive deltas between syncs (see torrent.SyncStates):
-- when the Engine resets the session, the new session value is added as the start of a
-- new cycle, so the total never regresses. Base of the aggregate ratio per Node and Cluster.
-- No backfill: they start at 0 and begin accumulating from the next heartbeat (the
-- first delta against the already observed session is ~0, avoiding double counting).
ALTER TABLE torrents
    ADD COLUMN uploaded_total   BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN downloaded_total BIGINT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE torrents
    DROP COLUMN IF EXISTS downloaded_total,
    DROP COLUMN IF EXISTS uploaded_total;
