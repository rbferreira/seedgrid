-- +goose Up
-- A Node can join with the qBittorrent the machine already runs (ADR-043, #32, #64). The column
-- records it, from the registration: it starts the Node in Manual and makes the seeding of its
-- overrides read the Engine's values (ADR-039 §7). Once in the Cluster it is an ordinary Node.
ALTER TABLE nodes ADD COLUMN engine_adopted BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE nodes DROP COLUMN engine_adopted;
