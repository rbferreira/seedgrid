-- +goose Up
-- The operator's trackers for publishing local content (ADR-019). Each line is
-- an announce credential (private tracker): name + announce URL containing the
-- passkey. The announce is stored in plaintext (operator's decision — no encryption at
-- rest) and is NOT returned in the API listing (only name + masked host); the CP
-- injects the announce only into the command to the Agent at publish time.
-- "Public" (no tracker, DHT/PeX) is not a line here — it is the absence of a tracker.
CREATE TABLE trackers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users (id),
    name          TEXT NOT NULL,
    announce      TEXT NOT NULL,                -- full announce URL (with passkey)
    host          TEXT NOT NULL,                -- announce host, only for display
    is_default    BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

-- Unique name per operator (among active ones).
CREATE UNIQUE INDEX uk_trackers_user_name
    ON trackers (user_id, name)
    WHERE deleted_at IS NULL;

-- At most one default tracker per operator (among active ones).
CREATE UNIQUE INDEX uk_trackers_one_default
    ON trackers (user_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX ix_trackers_user ON trackers (user_id) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS trackers;
