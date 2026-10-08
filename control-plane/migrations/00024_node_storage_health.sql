-- +goose Up
-- Storage health per Node (backlog #19-storage): the Agent reports in the heartbeat if the
-- StoragePath (/data) is mounted AND writable. NULL = unknown (old agent or without
-- signal) — never generates alerts nor penalizes health. Base of NODE_STORAGE alert and
-- metric seedgrid_node_storage_ok — avoids having to access each Node via SSH to know
-- if the HD is mounted right.
ALTER TABLE nodes
    ADD COLUMN storage_ok     boolean,
    ADD COLUMN storage_detail text;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN IF EXISTS storage_detail,
    DROP COLUMN IF EXISTS storage_ok;
