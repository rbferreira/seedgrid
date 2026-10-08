-- +goose Up
-- Inbound connectability signal that DECAYS (#155).
--
-- Migration 00035 introduced engine_connectable, read from libtorrent's
-- has_incoming_connections. This field is STICKY: it turns 'connected' on the first inbound
-- connection of the session and never reverts while the Engine runs. It answers "has
-- this session ever received anyone", and not "is this Node reachable now" — which
-- was the question #141 set out to answer.
--
-- Proof that it's useless: measured on 06/09/2026, node-01 and node-03 reported
-- 'connected' with the Engine up for 1 month and 5 days, spanning the entire period in
-- which they were provably unreachable (firewall rules pointing to the old
-- IPv6 prefix, 553 torrents without a single connection). NODE_FIREWALLED wouldn't have
-- triggered in the incident that motivated it.
--
-- incoming_peers is the count of INBOUND connections open now (flag "I" on peers),
-- sampled by the Agent. NULL = Agent not reporting the field: UNKNOWN, never
-- alerts — same semantics as storage_ok, and deliberately different from cpu_util_pct
-- (00036), where 0 is an honest reading. Here 0 and "I don't know" are distinct things.
--
-- last_incoming_at is what provides the decay: the last moment the count was > 0.
-- The AGE of this timestamp triggers the alert, not the sample value — measured on
-- 07/09/2026, the four Nodes (all healthy) had 0, 1, 1 and 0 inbound connections
-- at the reading time. In an idle seeding swarm, an inbound connection is a rare
-- event, not a continuous state: alerting on the instant value gives false positives on
-- a healthy Node.
ALTER TABLE nodes
    ADD COLUMN incoming_peers   integer,
    ADD COLUMN last_incoming_at timestamptz;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN IF EXISTS incoming_peers,
    DROP COLUMN IF EXISTS last_incoming_at;
