-- +goose Up
-- Raw resources reported by the Agent in the heartbeat (current snapshot, domain).
-- They feed the Health Score calculation (in the Control Plane) and the Portal view.
-- History/trends stay in Prometheus (ADR-016); here is just the current value.
ALTER TABLE heartbeats
    ADD COLUMN disk_total_bytes BIGINT,
    ADD COLUMN disk_free_bytes  BIGINT,
    ADD COLUMN mem_total_bytes  BIGINT,
    ADD COLUMN mem_avail_bytes  BIGINT,
    ADD COLUMN load1            DOUBLE PRECISION,
    ADD COLUMN cpu_cores        INTEGER;

-- +goose Down
ALTER TABLE heartbeats
    DROP COLUMN disk_total_bytes,
    DROP COLUMN disk_free_bytes,
    DROP COLUMN mem_total_bytes,
    DROP COLUMN mem_avail_bytes,
    DROP COLUMN load1,
    DROP COLUMN cpu_cores;
