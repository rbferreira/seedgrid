-- +goose Up
-- Join tokens one-time for zero-touch Seed Node onboarding (ADR-017). The
-- operator issues a disposable token; the Node presents it at enroll (reserves
-- the BT port) and at registration (consumes it). We only store the hash.
CREATE TABLE enroll_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_sha256 TEXT NOT NULL UNIQUE,
    bt_port      INTEGER,       -- BT port reserved at enroll (NULL until reserved)
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,   -- consumed in the Node record
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Live reservations (open token with port) — used to calculate the next free port.
CREATE INDEX idx_enroll_tokens_reserved ON enroll_tokens (bt_port) WHERE used_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS enroll_tokens;
