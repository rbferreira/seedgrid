-- +goose Up
-- Activity Feed (ADR-022): dedicated trail, append-only, of orchestration decisions
-- of the Control Plane and torrent lifecycle transitions — SEPARATE from
-- Alerts (issues) and Events (infra lifecycle). It is the "under the
-- hood" view: admission controller hold/promotion, Scheduler scheduling, and the
-- observed transitions (started downloading, completed, started seeding).

CREATE TABLE activities (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind          TEXT NOT NULL,                         -- admission_hold | admission_resume | scheduled | downloading | seeding
    torrent_id    UUID,                                  -- dangling reference: the torrent may be removed later
    torrent_name  TEXT,                                  -- denormalized: the feed survives a hard-delete of the torrent
    node_id       UUID,                                  -- hostname resolved via JOIN on read (nodes is soft-delete)
    message       TEXT NOT NULL,
    detail        JSONB,                                 -- the "why": reason for the hold, Scheduler score/ratio, etc.
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_activities_torrent FOREIGN KEY (torrent_id) REFERENCES torrents (id),
    CONSTRAINT fk_activities_node FOREIGN KEY (node_id) REFERENCES nodes (id)
);

-- Default read: most recent first (timeline) and by torrent (filter).
CREATE INDEX idx_activities_created_at ON activities (created_at DESC);
CREATE INDEX idx_activities_torrent ON activities (torrent_id);

-- +goose Down
DROP TABLE activities;
