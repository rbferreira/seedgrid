-- +goose Up
-- media_info stores the `mediainfo` output extracted on the Node at publish time
-- (ADR-020). It's the `mediainfo` field the UNIT3D API expects; kept in the torrent to
-- pre-fill the upload form (and survive a Portal refresh). NULL
-- for torrents that didn't go through publishing with UNIT3D integration.
ALTER TABLE torrents ADD COLUMN media_info TEXT;

-- +goose Down
ALTER TABLE torrents DROP COLUMN IF EXISTS media_info;
