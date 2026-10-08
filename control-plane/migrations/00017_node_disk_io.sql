-- +goose Up
-- Storage disk I/O utilization (busy%) reported in the heartbeat — current snapshot
-- on the nodes. Feeds the DISK_IO_SATURATED alert and the live number on the
-- Portal. Trends (read/write/util) come from Prometheus (node-exporter),
-- not from here. Default 0 = idle disk / old Agent that doesn't report yet.
ALTER TABLE nodes
    ADD COLUMN disk_io_util_pct DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN disk_io_util_pct;
