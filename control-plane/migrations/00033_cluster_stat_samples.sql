-- +goose Up
-- Cluster throughput rollup (F2). Periodic snapshot of the SUM of cumulative totals
-- of download/upload for ALL managed torrents (includes soft-deleted ones,
-- so the curve is MONOTONIC — "accumulated growth"; a removed torrent freezes its
-- contribution instead of subtracting it from the sum). Unlike torrent_stat_samples (per torrent,
-- SESSION bytes, 7d retention), this has 1 row per interval for the entire cluster, with
-- long retention — this is what allows 6-month / 1-year filters on the Metrics page.
-- Chart basis via GET /api/v1/metrics/cluster-throughput (downsampling via date_bin).
CREATE TABLE cluster_stat_samples (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sampled_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    downloaded_total BIGINT NOT NULL,
    uploaded_total   BIGINT NOT NULL
);

-- Endpoint query (time window + bucketization) and retention purge.
CREATE INDEX idx_cluster_stat_samples_sampled_at
    ON cluster_stat_samples (sampled_at);

-- +goose Down
DROP TABLE IF EXISTS cluster_stat_samples;
