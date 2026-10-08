-- +goose Up
-- Swarm statistics per torrent, reported by the Agent (snapshot of the latest
-- SyncStates). They feed the per-Node console in the Portal.
ALTER TABLE torrents
    ADD COLUMN num_seeds    INTEGER,
    ADD COLUMN num_leechers INTEGER,
    ADD COLUMN dl_speed     BIGINT,
    ADD COLUMN up_speed     BIGINT,
    ADD COLUMN size_bytes   BIGINT,
    ADD COLUMN ratio        DOUBLE PRECISION,
    ADD COLUMN eta_seconds  BIGINT;

-- +goose Down
ALTER TABLE torrents
    DROP COLUMN num_seeds,
    DROP COLUMN num_leechers,
    DROP COLUMN dl_speed,
    DROP COLUMN up_speed,
    DROP COLUMN size_bytes,
    DROP COLUMN ratio,
    DROP COLUMN eta_seconds;
