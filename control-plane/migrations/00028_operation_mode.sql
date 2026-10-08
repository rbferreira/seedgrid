-- +goose Up
-- Manual/Automatic operation mode (ADR-024): gives the operator explicit control
-- over download admission (ADR-021). Two state axes, both in the database
-- (ADR-006), read per pass by the worker (natural hot-reload):
--
--  * clusters.operation_mode — global "Full auto / Flex" gate.
--      full_auto: SG controls everything (per-Node modes are locked in the UI);
--      flex:      unlocks; each Node becomes SG (automatic) or Manual (operator).
--  * nodes.operation_mode — per-Node "SG / Manual" mode (only applies under flex).
--      sg:     the system has priority (manages holds/rotations — ADR-021).
--      manual: the operator is sovereign; the controller never pauses/promotes/rotates/
--              re-pauses torrents for this Node (but its assets COUNT towards the L cap).
--
-- Defaults reproduce the pre-ADR-024 behavior: full_auto gate + all Nodes sg
-- ⇒ effective rule "Node is Manual iff gate=flex AND node=manual" never triggers, and the
-- worker acts as before. A Node is only untouched when the operator chooses Manual under Flex.
ALTER TABLE clusters
    ADD COLUMN operation_mode TEXT NOT NULL DEFAULT 'full_auto'
        CHECK (operation_mode IN ('full_auto', 'flex'));

ALTER TABLE nodes
    ADD COLUMN operation_mode TEXT NOT NULL DEFAULT 'sg'
        CHECK (operation_mode IN ('sg', 'manual'));

-- +goose Down
ALTER TABLE nodes    DROP COLUMN IF EXISTS operation_mode;
ALTER TABLE clusters DROP COLUMN IF EXISTS operation_mode;
