-- +goose Up
-- Site profile of a tracker (ADR-037, #190/#196): the knowledge that is of the SITE and not the
-- operator. Today it is the taxonomy (the category/type/resolution IDs); name format,
-- rules and image host policy come in as new columns, one section per issue.
--
-- The three layers of ADR-037, and why this is a SEPARATE table from `trackers`:
--   1. family (API shape)             — Go code, not in the database;
--   2. site profile (this table)      — exportable data (#197): whoever has a site account
--                                        discovers, adjusts and shares;
--   3. operator config                — `trackers`: api_token, announce with passkey (and in the
--                                        future, anon/modq). NEVER exported.
-- This table HAS NO credential column, on purpose: #197 export only reads from here, and
-- layer 3 is kept out by design, not by a filter that someone needs to remember.
--
-- Why JSONB and not a relational table per term:
--   * the export/import/validation unit is the DOCUMENT — one column per profile section
--     is the contributed file format, without assembling/disassembling;
--   * new axis (audio, language, tag, flag) and new section (rules, image host) enter without
--     table migration, only with schema_version;
--   * the taxonomy is never queried by content ("which trackers have category 5?") —
--     it is read entirely, it's a few dozen terms, and the Portal edits it as a whole.
-- The integrity that relational would give for free (unique id per axis, range, name
-- length) lives in internal/trackerprofile, at every write point, and the write is
-- serialized by SELECT … FOR UPDATE on the profile row.
--
-- Shape of `taxonomy`: {"category": [{"id":1,"name":"Movies","kind":"movie","origin":"seed"}],
--                       "type": [...], "resolution": [...]}
--   origin: seed (migration) | discovered (discovered in the catalog) | manual (operator).
--           It decides who can change the name: discovered never overwrites `manual`.
--   kind:   semantic role of category (movie | tv | game); only in category axis. It is what
--           the Portal uses to show season/episode and game search, instead of the
--           category number that was hardcoded in the frontend.
CREATE TABLE tracker_profiles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tracker_id     UUID NOT NULL REFERENCES trackers (id),
    -- Tracker family (ADR-037). V1 only knows 'unit3d'; the value is validated in Go
    -- (a new family is new code), here it only ensures it is not empty.
    family         TEXT NOT NULL DEFAULT 'unit3d',
    -- Version of the profile FORMAT (what export stamps and import checks).
    schema_version INTEGER NOT NULL DEFAULT 1,
    taxonomy       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ,
    CONSTRAINT ck_tracker_profiles_family CHECK (family <> ''),
    CONSTRAINT ck_tracker_profiles_schema_version CHECK (schema_version >= 1),
    CONSTRAINT ck_tracker_profiles_taxonomy_object CHECK (jsonb_typeof(taxonomy) = 'object')
);

-- One live profile per tracker. The row is created on demand (first edit or discovery);
-- tracker without a row is read as an empty profile.
CREATE UNIQUE INDEX uk_tracker_profiles_tracker
    ON tracker_profiles (tracker_id)
    WHERE deleted_at IS NULL;

-- The table starts empty: no tracker gets a profile here. Whoever already has their site profile
-- imports the file (profiles/<family>/<site>.json, ADR-037); the others run "Discover
-- taxonomy" in the Portal.

-- +goose Down
DROP TABLE IF EXISTS tracker_profiles;
