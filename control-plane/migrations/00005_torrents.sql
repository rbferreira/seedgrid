-- +goose Up
-- Torrent Domain (Database.md §35) and Scheduler decisions (§45).

CREATE TABLE torrents (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uuid           UUID NOT NULL DEFAULT gen_random_uuid(),
    node_id        UUID NOT NULL,
    hash           TEXT,
    name           TEXT,
    size           BIGINT,
    status         TEXT NOT NULL DEFAULT 'QUEUED',
    progress       DOUBLE PRECISION,
    engine         TEXT,
    magnet         TEXT,
    created_by     UUID,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ,
    deleted_by     UUID,
    CONSTRAINT uk_torrents_uuid UNIQUE (uuid),
    CONSTRAINT uk_torrents_node_hash UNIQUE (node_id, hash),
    CONSTRAINT fk_torrents_node FOREIGN KEY (node_id) REFERENCES nodes (id),
    CONSTRAINT ck_torrents_status CHECK (status IN
        ('QUEUED','DOWNLOADING','SEEDING','PAUSED','CHECKING','MOVING','ERROR','ARCHIVED','DELETED','UNKNOWN'))
);

-- Scheduler decisions (auditable, Scheduler.md §21,§30).
CREATE TABLE torrent_scheduler (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    torrent_id        UUID NOT NULL,
    selected_node     UUID NOT NULL,
    decision_score    INTEGER,
    decision_reason   TEXT,
    candidates        INTEGER,
    algorithm_version TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_torrent_scheduler_torrent FOREIGN KEY (torrent_id) REFERENCES torrents (id),
    CONSTRAINT fk_torrent_scheduler_node FOREIGN KEY (selected_node) REFERENCES nodes (id)
);

CREATE INDEX idx_torrents_node ON torrents (node_id);
CREATE INDEX idx_torrents_hash ON torrents (hash);
CREATE INDEX idx_torrents_status ON torrents (status);
CREATE INDEX idx_torrent_scheduler_torrent ON torrent_scheduler (torrent_id);

-- +goose Down
DROP TABLE IF EXISTS torrent_scheduler;
DROP TABLE IF EXISTS torrents;
