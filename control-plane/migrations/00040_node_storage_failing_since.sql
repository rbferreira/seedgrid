-- +goose Up
-- Since when a Node's storage (/data) has been failing (#177).
--
-- NODE_STORAGE opened with ONE read and resolved with ONE. Since `storage_ok` is a
-- boolean overwritten every heartbeat (30s), any I/O hiccup became a pair of
-- open/resolve events 30 seconds apart:
--
--   17:37:27  Node Storage is not mounted/writable ... input/output error
--   17:37:57  Resolved: Node Storage is not mounted/writable ...
--
-- On an underpowered USB drive — the known case of node-03 — intermittent EIO is
-- expected, and a flashing CRITICAL alert goes unread.
--
-- Without history there's no way to require "sustained failure": the boolean doesn't track since
-- when. This column is the same resource that `connectable_since` (00035) gives to
-- NODE_FIREWALLED, and is read the same way — the AGE of the timestamp is what triggers the
-- alert, compared against the tunable grace period `alerting.storage_grace`.
--
-- It only restarts when the verdict CHANGES (ok -> failed). NULL from Agent = unknown
-- (old Agent) and does not zero the history, same semantics as `storage_ok`.
ALTER TABLE nodes
    ADD COLUMN storage_failing_since timestamptz;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN IF EXISTS storage_failing_since;
