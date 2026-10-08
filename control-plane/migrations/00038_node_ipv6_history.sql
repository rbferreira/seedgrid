-- +goose Up
-- IPv6 history per Node, to detect provider prefix rotation (#147).
--
-- Incident on 05-06/09/2026: the provider rotated the delegated prefix
-- (2001:db8:601:d300::/64 -> 2001:db8:60b:c000::/64) at some point between 31/07 and
-- 01/09. The gateway firewall rules used LITERAL ADDRESS, so they started to
-- allow addresses that no longer exist. Three Nodes were unreachable for WEEKS,
-- with 553 torrents receiving no connection, and the Portal showed them ONLINE and green.
--
-- The rule fix is on the gateway side (swap literal for network object). What is
-- SeedGrid's fault is the failure being SILENT: nothing in the product noticed the swap. The
-- public_ipv6 (00035) stores the CURRENT address and is overwritten on heartbeat — without
-- storing the previous one, there is no way to know it changed, only that it's different from yesterday.
--
-- previous_public_ipv6 is only written when the value CHANGES from a non-empty to another
-- non-empty one: first boot (NULL -> address) is not rotation, it is discovery.
ALTER TABLE nodes
    ADD COLUMN previous_public_ipv6 text,
    ADD COLUMN public_ipv6_changed_at timestamptz;

-- +goose Down
ALTER TABLE nodes
    DROP COLUMN IF EXISTS previous_public_ipv6,
    DROP COLUMN IF EXISTS public_ipv6_changed_at;
