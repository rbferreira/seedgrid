-- +goose Up
-- last_synced_at (backlog #19): marks the time of the last SyncStates that SAW this
-- torrent in the Node inventory. It serves deletion reconciliation: SyncStates
-- soft-deletes the Node's live rows whose hash didn't come in the current inventory — but only
-- those ALREADY seen at least once (last_synced_at IS NOT NULL), protecting
-- newly added torrents that haven't yet appeared in the Engine. Fixes the removal
-- race (immediate soft-delete + async remove_torrent → the sync upsert recreates
-- an orphaned live row, since the unique index is partial WHERE deleted_at IS NULL).
-- Backfill with updated_at: already synced torrents become reconcilable right
-- away (clears pre-existing orphans on the 1st sync post-deploy). Those that still lack
-- a consistent updated_at get last_synced_at on the next heartbeat.
ALTER TABLE torrents ADD COLUMN last_synced_at timestamptz;

UPDATE torrents SET last_synced_at = updated_at WHERE deleted_at IS NULL;

-- +goose Down
ALTER TABLE torrents DROP COLUMN IF EXISTS last_synced_at;
