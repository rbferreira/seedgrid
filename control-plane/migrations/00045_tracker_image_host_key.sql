-- +goose Up
-- Operator key for the tracker's own image host (ADR-040, #193).
-- The key is tied to the host it was registered for: if the operator changes the upload_url
-- of the profile, the key recorded for the old host no longer works for the new one (the upload
-- chain skips it with the reason "key registered for another host"). NEVER returned in the API —
-- the listing only exposes has_image_host_key (boolean) and image_host_key_host (the DNS host).
ALTER TABLE trackers
    ADD COLUMN image_host_key      TEXT,
    ADD COLUMN image_host_key_host TEXT;

-- +goose Down
ALTER TABLE trackers
    DROP COLUMN IF EXISTS image_host_key,
    DROP COLUMN IF EXISTS image_host_key_host;
