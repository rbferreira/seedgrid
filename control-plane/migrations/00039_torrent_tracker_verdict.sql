-- +goose Up
-- Tracker verdict per torrent (#160).
--
-- Until then nothing in the product measured whether the tracker still recognized the torrent. A torrent
-- deleted there keeps SEEDING, green in the Portal, announcing to those who no longer know it —
-- and the only way to find out was to open the Nodes over SSH and query torrent by torrent.
-- Measured on 06/09/2026: of 983 torrents in the cluster, 3 replied "Torrent has been
-- deleted." and nobody knew.
--
-- tracker_status is the libtorrent status on announce (2 = working, 4 = not
-- working, 5 = refused). NULL = Agent that does not report the field: UNKNOWN, and it should never
-- be read as "dead" — same semantics as storage_ok and incoming_peers.
--
-- tracker_message stores the RAW message from the tracker instead of a derived boolean: it is
-- what distinguishes "deleted" from "not registered" from "invalid passkey", and grouping by
-- it is what makes bulk cleanup possible in the analytics panel.
ALTER TABLE torrents
    ADD COLUMN tracker_status  integer,
    ADD COLUMN tracker_message text;

-- Partial index: the query that matters is "which torrents the tracker is refusing",
-- and they are the minority (3 of 983 in the measurement). Indexing only the problematic ones keeps the
-- index small and the panel scan cheap.
CREATE INDEX idx_torrents_tracker_problema
    ON torrents (node_id)
    WHERE deleted_at IS NULL AND tracker_status IS NOT NULL AND tracker_status >= 4;

-- +goose Down
DROP INDEX IF EXISTS idx_torrents_tracker_problema;
ALTER TABLE torrents
    DROP COLUMN IF EXISTS tracker_status,
    DROP COLUMN IF EXISTS tracker_message;
