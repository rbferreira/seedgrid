-- +goose Up
-- CONNECTED peers vs SWARM peers (ADR-021 refinement, peer-aware admission).
--
-- Up to here torrents.num_seeds/num_leechers stored the SWARM totals (the scrape from
-- tracker: qBittorrent num_complete/num_incomplete) — the number the operator understands
-- as "torrent seeds/peers" and that the Portal displays. The Agent collapsed both sources
-- into a single field (adapter.pick), so the CP never saw how many peers were actually
-- CONNECTED.
--
-- This blinds the dead rotation (admission.rotateDead): a torrent whose tracker announces
-- a GHOST seeder (registered in the scrape, but never responds) has num_seeds > 0 and
-- therefore is never marked as dead — even with 0 connections for days. It returns to the top of the
-- promotion queue every cooldown (it's "viable" and has high progress), burning a download slot
-- from the L ceiling and downloads 0 bytes. Observed live: Blade.Runner.2049 on node-02,
-- 98.79% and 0 connections for 5 days, cycling resume→hold every ~15 min since 14/Jul/2026.
--
-- The new columns store the actually connected peers (qBittorrent num_seeds/num_leechs)
-- WITHOUT touching the semantics of the old ones. NULL = Agent that doesn't yet report the field (previous
-- version): consumers do COALESCE(num_seeds_connected, num_seeds) and fall back to the
-- current behavior — the change is additive and backwards compatible, with no deploy window.
ALTER TABLE torrents
    ADD COLUMN num_seeds_connected    INTEGER,
    ADD COLUMN num_leechers_connected INTEGER;

COMMENT ON COLUMN torrents.num_seeds    IS 'Seeds in the SWARM (tracker scrape). Shown in the Portal.';
COMMENT ON COLUMN torrents.num_leechers IS 'Leechers in the SWARM (tracker scrape). Shown in the Portal.';
COMMENT ON COLUMN torrents.num_seeds_connected    IS 'Seeds actually CONNECTED to the Engine. NULL = old Agent. Basis of the dead-torrent rotation (ADR-021).';
COMMENT ON COLUMN torrents.num_leechers_connected IS 'Leechers actually CONNECTED to the Engine. NULL = old Agent. Basis of the dead-torrent rotation (ADR-021).';

-- +goose Down
ALTER TABLE torrents
    DROP COLUMN IF EXISTS num_leechers_connected,
    DROP COLUMN IF EXISTS num_seeds_connected;
