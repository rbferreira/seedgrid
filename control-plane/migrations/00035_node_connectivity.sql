-- +goose Up
-- Incoming connectability per Node (#141, ADR-034).
--
-- A Node may be ONLINE, with the Engine reachable, high health — and completely
-- unreachable from outside, with no indication in the product. It happened with node-01/02/03
-- between 31/07 and 01/09 of 2026: the provider rotated the delegated IPv6 prefix, the
-- gateway firewall rules (anchored to a literal address) started allowing addresses
-- that no longer existed, and three Nodes spent weeks without receiving a single connection —
-- 553 torrents stalled — while the Portal showed them as green. The problem only surfaced
-- because the operator went to check the tracker.
--
-- public_ipv6        : global IPv6 address that the Agent sees on the interfaces. It is what
--                      the operator must allow in the firewall, and whose CHANGE is the sign
--                      that the existing rule has become stale. NULL = not reported
--                      (old Agent) or Node without global IPv6.
-- engine_connectable : Torrent Engine verdict on having already received an outside connection
--                      ('connected' | 'firewalled' | 'disconnected'). It is
--                      has_incoming_connections from libtorrent. NULL = unknown —
--                      never treat as firewalled, at the risk of a false alarm on a Node
--                      whose Agent does not report the field.
-- connectable_since  : when the current value of engine_connectable started. It allows
--                      requiring 'firewalled' to be SUSTAINED before alerting: freshly started,
--                      every Node is firewalled just because nobody has knocked on the door yet.
ALTER TABLE nodes
    ADD COLUMN public_ipv6        text,
    ADD COLUMN engine_connectable text,
    ADD COLUMN connectable_since  timestamptz;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN IF EXISTS connectable_since,
    DROP COLUMN IF EXISTS engine_connectable,
    DROP COLUMN IF EXISTS public_ipv6;
