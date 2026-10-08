-- +goose Up
-- Fields for integration with external UNIT3D tracker (ADR-020). When the tracker is a
-- UNIT3D site (e.g., tracker.example), publishing isn't just announcing: the release needs
-- to be registered in the site's catalog via API. These fields enable automatic
-- registration:
--   source    = tag embedded in the .torrent (e.g., EXAMPLE); without it the infohash
--               won't match the site's. NULL/'' = regular tracker (no integration).
--   api_url   = UNIT3D site base (e.g., https://tracker.example), for the POST /api.
--   api_token = operator's SECRET (text, same decision as announce in ADR-019);
--               NEVER returned in the API — listing only exposes a has_api_token boolean.
ALTER TABLE trackers
    ADD COLUMN source    TEXT,
    ADD COLUMN api_url   TEXT,
    ADD COLUMN api_token TEXT;

-- +goose Down
ALTER TABLE trackers
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS api_url,
    DROP COLUMN IF EXISTS api_token;
