-- +goose Up
-- Download admission controller (#21). The Control Plane ensures ≤ L torrents
-- downloading across the ENTIRE cluster (L = DL slots per account on the tracker): it
-- puts the excess on "hold" (pause) and promotes the next one when a slot opens.

-- admission_paused_at: when NOT null, the torrent was paused BY the admission
-- controller (hold), not by the operator. It's immune to SyncStates (which overwrites
-- torrents.status with the Engine reality) — so it's a dedicated column and not
-- a status value. Distinguishing admission-hold from user-pause is what prevents
-- the controller from resuming a torrent that the operator paused on purpose.
ALTER TABLE torrents
    ADD COLUMN admission_paused_at TIMESTAMPTZ;

-- Hold queue (promotion candidates): partial index only on those that are in
-- hold and alive — the majority of torrents never enter here.
CREATE INDEX idx_torrents_admission_hold
    ON torrents (admission_paused_at)
    WHERE admission_paused_at IS NOT NULL AND deleted_at IS NULL;

-- Observed L per tracker (dynamic): extracted from the tracker's own message
-- (UNIT3D error 164 "You may at most download N torrents at the same time"), which
-- the Agent reads from Engine and reports. NULL = never observed → falls back to
-- config fallback (SEEDGRID_MAX_CLUSTER_DOWNLOADS). dl_slot_limit_seen_at dates the
-- last observation (the controller only trusts recent observations).
ALTER TABLE trackers
    ADD COLUMN dl_slot_limit         INTEGER,
    ADD COLUMN dl_slot_limit_seen_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE trackers
    DROP COLUMN IF EXISTS dl_slot_limit_seen_at,
    DROP COLUMN IF EXISTS dl_slot_limit;
DROP INDEX IF EXISTS idx_torrents_admission_hold;
ALTER TABLE torrents
    DROP COLUMN IF EXISTS admission_paused_at;
