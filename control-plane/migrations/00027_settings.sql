-- +goose Up
-- Dynamic configuration of tunables (ADR-023). 00002 created a `settings` table
-- for scaffolding (id/category/key/value/description/updated_by, Database.md §71) that
-- was NEVER used by code. ADR-023 gives it a real purpose and a simpler form:
-- pure KV whose metadata (type/default/description/limits) lives in the code catalog
-- (internal/settings/catalog.go), not in the DB. We replace the empty table.
--
-- It is the EDITABLE source of truth for behavior tunables (admission, alerts,
-- worker intervals) — the DB is in charge; the env var only seeds on 1st boot. Changes
-- here are applied at runtime by the Store (hot-reload), no redeploy. Reset to default
-- = remove the row.
DROP TABLE IF EXISTS settings;
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,                       -- namespaced key (e.g.: admission.dead_grace); validated against the catalog
    value      TEXT NOT NULL,                          -- value serialized as a string (parsed by the catalog type)
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by TEXT                                    -- operator who changed it (light audit); 'env-seed' on boot seed
);

-- +goose Down
-- Reverts to the original 00002 scaffolding table (maintains migration symmetry).
DROP TABLE IF EXISTS settings;
CREATE TABLE settings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category    TEXT NOT NULL,
    key         TEXT NOT NULL,
    value       TEXT,
    description TEXT,
    updated_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_settings_category_key UNIQUE (category, key)
);
