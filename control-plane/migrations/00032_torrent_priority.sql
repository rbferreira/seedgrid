-- +goose Up
-- Manual torrent prioritization in the admission queue (F3). The operator can mark a
-- torrent to "skip the queue": it moves ahead in the promotion order and is the next to
-- be admitted when a slot opens. It is a manual override of the peer-aware order of
-- ADR-021, without changing the L ceiling.

-- prioritized_at: when NOT null, the torrent was manually prioritized by the operator.
-- Dual effect in the admission controller:
--   1) promotion order: a NOT null prioritized_at comes FIRST in promotableHolds
--      (before the viable→progress criterion), and ignores the rotation cooldown.
--   2) protection (persistent mode, tunable admission.priority_persistent): when enabled,
--      a prioritized torrent is NOT rotated for dead/slow — holds the slot until download completes.
-- The mark is cleared when the torrent completes (turns SEEDING) and, in NON-persistent mode,
-- when dead/slow rotation demotes it (it becomes a normal torrent again).
ALTER TABLE torrents
    ADD COLUMN prioritized_at TIMESTAMPTZ;

-- Prioritized queue: partial index only over marked and alive ones — the vast
-- majority of torrents never enter here (same pattern as idx_torrents_admission_hold).
CREATE INDEX idx_torrents_prioritized
    ON torrents (prioritized_at)
    WHERE prioritized_at IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_torrents_prioritized;
ALTER TABLE torrents
    DROP COLUMN IF EXISTS prioritized_at;
