-- +goose Up
-- Validity of reachability alerts (#211).
--
-- NODE_FIREWALLED, NODE_INBOUND_SILENT and NODE_IPV6_CHANGED deduce reachability by the
-- ABSENCE of inbound connections, and only resolve when one arrives. A Node with no demand
-- never receives this connection, even with the port open. Measured on 09/24/2026: node-01
-- with 0 leechers across 116 torrents and two open alerts, the port verified from outside and
-- open. The alert stayed open forever.
--
-- The solution is for the alert to EXPIRE: the outbound call already happened at opening; after the
-- validity period (`alerting.reachability_ttl`) it closes by itself, without notifying.
--
-- `expired` distinguishes "closed due to validity" from "the condition no longer exists" — without
-- this the Portal would say "resolved" about something nobody fixed.
--
-- `episode_key` is the IDENTITY of the condition that opened the alert (the IPv6 rotation, the
-- start of silence, the Engine verdict). It is what prevents the worker from reopening the
-- SAME episode — both after expiring and after a reboot, which closes the
-- alert due to "left ONLINE" and would reopen it upon return, notifying again. NULL = alert
-- without an episode (all other rules), which continue deduplicating only by OPEN.
ALTER TABLE alerts
    ADD COLUMN episode_key text,
    ADD COLUMN expired boolean NOT NULL DEFAULT false;

CREATE INDEX idx_alerts_episode ON alerts (code, node_id, episode_key)
    WHERE episode_key IS NOT NULL;

-- Backfill of alerts that are already open, so that the first expiration after the
-- deploy does not reopen them (without a key, the episode would seem new and notify again).
-- The expression MUST be identical to the one in `episodeSQL` in internal/alert/worker.go: UTC
-- explicit and microseconds, so as not to depend on the session's TimeZone.
UPDATE alerts a
   SET episode_key = to_char(n.public_ipv6_changed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
  FROM nodes n
 WHERE a.node_id = n.id AND a.status = 'OPEN' AND a.code = 'NODE_IPV6_CHANGED'
   AND n.public_ipv6_changed_at IS NOT NULL;

UPDATE alerts a
   SET episode_key = to_char(n.last_incoming_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
  FROM nodes n
 WHERE a.node_id = n.id AND a.status = 'OPEN' AND a.code = 'NODE_INBOUND_SILENT'
   AND n.last_incoming_at IS NOT NULL;

UPDATE alerts a
   SET episode_key = to_char(n.connectable_since AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
  FROM nodes n
 WHERE a.node_id = n.id AND a.status = 'OPEN' AND a.code = 'NODE_FIREWALLED'
   AND n.connectable_since IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_alerts_episode;
ALTER TABLE alerts
    DROP COLUMN IF EXISTS expired,
    DROP COLUMN IF EXISTS episode_key;
