-- +goose Up
-- A Node can join with the qBittorrent the machine already runs (ADR-043, #32). The Engine
-- stays the operator's: the Agent never writes its preferences, SeedGrid does not restart it,
-- and the Node joins in Manual. This column records it, from the registration, so the
-- Portal can show it and the Control Plane can choose the first mode.
ALTER TABLE nodes ADD COLUMN engine_adopted BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE nodes DROP COLUMN engine_adopted;
