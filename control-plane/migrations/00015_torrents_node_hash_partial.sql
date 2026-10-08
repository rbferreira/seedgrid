-- +goose Up
-- Makes (node_id, hash) uniqueness sensitive to soft delete. Previously, a torrent
-- removed (deleted_at != NULL) still occupied the (node_id, hash) slot, which
-- (a) prevented re-adding a torrent to a Node it was removed from, and
-- (b) blocked completing a migration whose target Node already had (and
--     removed) that torrent in the past — the node_id UPDATE collided with the
--     dead row (ADR-018).
-- Solution: replace the CONSTRAINT with a PARTIAL unique index on live rows.
ALTER TABLE torrents DROP CONSTRAINT uk_torrents_node_hash;
CREATE UNIQUE INDEX uk_torrents_node_hash ON torrents (node_id, hash) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS uk_torrents_node_hash;
ALTER TABLE torrents ADD CONSTRAINT uk_torrents_node_hash UNIQUE (node_id, hash);
