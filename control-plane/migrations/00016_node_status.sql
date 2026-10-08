-- +goose Up
-- Status signals per Node for troubleshooting without SSH (host uptime, engine
-- reachable, Engine container start). The Agent/service uptime already lives in
-- heartbeats.uptime_seconds; here we store the current snapshot in nodes.
ALTER TABLE nodes
    ADD COLUMN host_uptime_seconds BIGINT,
    ADD COLUMN engine_reachable    BOOLEAN,
    ADD COLUMN engine_started_at   TIMESTAMPTZ;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN host_uptime_seconds,
    DROP COLUMN engine_reachable,
    DROP COLUMN engine_started_at;
