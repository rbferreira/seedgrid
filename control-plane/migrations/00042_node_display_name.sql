-- +goose Up
-- Node alias within SeedGrid (#145).
--
-- `nodes.display_name` already existed since 00002 and was never written: the Node joined
-- with the OS hostname and there was no way to name it without touching the machine. The alias lives
-- ONLY in SeedGrid — the OS hostname is technical data and remains untouched (the operator
-- prefers numbered hostnames, like seedgrid-node-01).
--
-- Uniqueness: the alias is what the operator reads and selects in action
-- pickers for destructive operations (migrate, drain, remove Node) and is the series key in graphs.
-- Two Nodes with the same name is ambiguity in a place where mistakes cost data. It applies
-- between NON-removed Nodes (a removed Node's name returns to the pool) and is
-- case-insensitive. The index is the safeguard against races; the friendly check
-- (409 display_name_taken) and the comparison with the HOSTNAMES of other Nodes are done
-- by the Control Plane, which can explain the conflict.
CREATE UNIQUE INDEX uk_nodes_display_name_per_cluster
    ON nodes (cluster_id, lower(display_name))
    WHERE deleted_at IS NULL AND display_name IS NOT NULL;

-- The name chosen in the "Add Node" modal travels in the join token: the token already
-- binds the Node coming up to the operator's intent (reserved BT port, single
-- use). This way the name reaches the record without bootstrap-node.sh needing to know it
-- and without the Agent loading anything new.
ALTER TABLE enroll_tokens ADD COLUMN display_name text;

-- +goose Down
ALTER TABLE enroll_tokens DROP COLUMN IF EXISTS display_name;
DROP INDEX IF EXISTS uk_nodes_display_name_per_cluster;
