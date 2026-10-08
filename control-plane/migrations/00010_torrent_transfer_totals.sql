-- +goose Up
-- Cumulative transfer totals per torrent (reported by the Agent from
-- qBittorrent). Basis for per-Node metrics and rankings (downloaded/uploaded volume).
ALTER TABLE torrents
    ADD COLUMN downloaded_bytes BIGINT,
    ADD COLUMN uploaded_bytes   BIGINT;

-- +goose Down
ALTER TABLE torrents
    DROP COLUMN downloaded_bytes,
    DROP COLUMN uploaded_bytes;
