-- +goose Up
-- Agent/Engine behavior configuration coming from the Control Plane (ADR-039, #144).
--
-- Precedence: Node override > cluster value (`settings`, ADR-023) > Node env >
-- code default. This migration creates the highest level — the PER NODE override — and the
-- columns that record what each Agent applied.

-- Node override. Same KV design as `settings` (00027): the metadata (type, range,
-- options, default) lives in the code catalog (internal/settings/catalog.go) and the value is
-- validated by the SAME Spec, at both levels. Only PerNode keys from the catalog go here
-- (checked by the Control Plane, not the DB — the catalog is not in the DB).
--
-- Removing the override is a row DELETE (reverts to cluster value), like the `settings`
-- reset: the history of who changed what stays in the Audit trail (ADR-025), with the
-- before and after, and not in a dead row here. ON DELETE CASCADE only matters if a Node
-- is actually deleted — Nodes are soft-deleted (deleted_at), and a removed Node's override
-- simply ceases to be read.
CREATE TABLE node_settings (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id    UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,                          -- namespaced catalog key (e.g., engine.max_active_downloads)
    value      TEXT NOT NULL,                          -- value serialized as string, parsed by the catalog type
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by TEXT,                                   -- operator; 'seeding (Node env)' when it comes from seeding (ADR-039 §7)
    CONSTRAINT uk_node_settings_node_key UNIQUE (node_id, key)
);

-- What the Agent applied. The heartbeat carries `config_version` (hash of effective values that
-- it received and successfully applied); the Portal compares with today's effective version to
-- show "up to date" / "waiting for Agent". NULL = the Agent never reported (prior to
-- ADR-039) — it is never read as "up to date".
--
-- agent_config_seeded_at is the "once per Node" for seeding overrides from the env
-- that the Agent reports in the first new heartbeat (ADR-039 §7): once populated, the
-- Control Plane no longer seeds, even if the operator deletes the seeded override.
ALTER TABLE nodes
    ADD COLUMN agent_config_version    TEXT,
    ADD COLUMN agent_config_applied_at TIMESTAMPTZ,
    ADD COLUMN agent_config_seeded_at  TIMESTAMPTZ;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN IF EXISTS agent_config_seeded_at,
    DROP COLUMN IF EXISTS agent_config_applied_at,
    DROP COLUMN IF EXISTS agent_config_version;
DROP TABLE IF EXISTS node_settings;
