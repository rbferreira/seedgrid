-- +goose Up
-- Torrent link to the tracker it should be (or was) published to (ADR-020,
-- backlog #8). When the operator creates local content by choosing a UNIT3D tracker, the
-- torrent is created with tracker_id filled but published_tracker_at NULL — it is a
-- "pending publication" (the queue the Portal exposes on the Publish tab). Upon completing the
-- POST to the tracker, the CP sets published_tracker_at and stores the returned download URL.
ALTER TABLE torrents
    ADD COLUMN tracker_id           UUID,
    ADD COLUMN published_tracker_at TIMESTAMPTZ,
    ADD COLUMN tracker_torrent_url  TEXT;

ALTER TABLE torrents
    ADD CONSTRAINT fk_torrents_tracker FOREIGN KEY (tracker_id) REFERENCES trackers (id);

-- Partial index: the pending queue is the hot query (only torrents with a tracker
-- not yet published).
CREATE INDEX ix_torrents_pending_publish
    ON torrents (tracker_id)
    WHERE tracker_id IS NOT NULL AND published_tracker_at IS NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS ix_torrents_pending_publish;
ALTER TABLE torrents DROP CONSTRAINT IF EXISTS fk_torrents_tracker;
ALTER TABLE torrents
    DROP COLUMN IF EXISTS tracker_torrent_url,
    DROP COLUMN IF EXISTS published_tracker_at,
    DROP COLUMN IF EXISTS tracker_id;
