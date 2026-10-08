-- +goose Up
-- CPU usage per Node (#151).
--
-- The Node card already showed bars for Disk, Memory and disk I/O, but CPU
-- appeared only as loose text ("4 cores · load 2.99") — no visual sense of load.
-- In a heterogeneous fleet this hides a lot: measured on 06/09/2026, qBittorrent on
-- node-04 (Atom x5-Z8350) runs at 50,7% CPU while the other three (i5) stay between
-- 0,5% and 2,9% with the same workload.
--
-- Mirrors the disk_io_util_pct column (migration 00017): sampled by the Agent between
-- heartbeats, NOT NULL with default 0 so the old Agent does not generate spurious NULL — 0
-- here means "not reported or idle", and the bar at 0 is an honest reading in both
-- cases (unlike storage_ok, where unknown NEEDS to be distinguishable from false).
--
-- Deliberately NOT derived from load1/cpu_cores: Linux load counts processes in
-- I/O wait (state D), so on a Node with a saturated disk it rises without the CPU being
-- busy — the Node where the metric matters most would be exactly where it would lie.
ALTER TABLE nodes
    ADD COLUMN cpu_util_pct double precision NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN IF EXISTS cpu_util_pct;
