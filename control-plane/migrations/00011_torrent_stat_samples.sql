-- +goose Up
-- Time-series samples per torrent (history of cumulative totals and speeds
-- reported by the Agent in each SyncStates). Unlike the snapshot in
-- torrents.* (migrations 00009/00010), here we store the history to plot
-- charts on the torrent detail page. High per-torrent cardinality → stays
-- in Postgres, not in Prometheus (ADR-016).
CREATE TABLE torrent_stat_samples (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    torrent_id       UUID NOT NULL,
    sampled_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    downloaded_bytes BIGINT,
    uploaded_bytes   BIGINT,
    dl_speed         BIGINT,
    up_speed         BIGINT,
    CONSTRAINT fk_torrent_stat_samples_torrent
        FOREIGN KEY (torrent_id) REFERENCES torrents (id) ON DELETE CASCADE
);

-- Endpoint query (per torrent, ordered in time) and retention purge.
CREATE INDEX idx_torrent_stat_samples_torrent_time
    ON torrent_stat_samples (torrent_id, sampled_at DESC);
CREATE INDEX idx_torrent_stat_samples_sampled_at
    ON torrent_stat_samples (sampled_at);

-- +goose Down
DROP TABLE IF EXISTS torrent_stat_samples;
