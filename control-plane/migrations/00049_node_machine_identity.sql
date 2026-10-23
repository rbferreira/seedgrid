-- +goose Up
-- A removed Node that joins again comes back as itself only when it is the same machine (ADR-044,
-- #62). machine_id is /etc/machine-id as the Agent reads it; NULL for Nodes registered before it.
ALTER TABLE nodes ADD COLUMN machine_id TEXT;

-- The hostname is unique among live Nodes only: a removed row keeps its hostname for history and
-- no longer holds it. The registration upserts on this index (ON CONFLICT ... WHERE deleted_at IS NULL).
ALTER TABLE nodes DROP CONSTRAINT uk_nodes_hostname_per_cluster;
CREATE UNIQUE INDEX uk_nodes_hostname_per_cluster ON nodes (cluster_id, hostname) WHERE deleted_at IS NULL;

-- +goose Down
-- Fails when a removed row and a live one share a hostname: remove one of them first.
DROP INDEX uk_nodes_hostname_per_cluster;
ALTER TABLE nodes ADD CONSTRAINT uk_nodes_hostname_per_cluster UNIQUE (cluster_id, hostname);
ALTER TABLE nodes DROP COLUMN machine_id;
