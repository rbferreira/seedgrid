-- +goose Up
-- Agent Authentication (Authentication.md §8,§17-20), Monitoring (Database.md §60-62)
-- and Events (§63-65).

-- node_credentials: credential per Agent, issued at registration.
-- We store the SHA-256 of the token (high entropy) for O(1) indexable lookup.
CREATE TABLE node_credentials (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id      UUID NOT NULL,
    token_sha256 TEXT NOT NULL,
    state        TEXT NOT NULL DEFAULT 'TRUSTED',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    CONSTRAINT uk_node_credentials_token UNIQUE (token_sha256),
    CONSTRAINT fk_node_credentials_node FOREIGN KEY (node_id) REFERENCES nodes (id),
    CONSTRAINT ck_node_credentials_state CHECK (state IN ('PENDING','TRUSTED','REVOKED','DISABLED'))
);

-- heartbeats: only the last known heartbeat per Node (no history — Database.md §61).
CREATE TABLE heartbeats (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id        UUID NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    agent_version  TEXT,
    engine_version TEXT,
    status         TEXT,
    uptime_seconds BIGINT,
    health_score   INTEGER,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_heartbeats_node UNIQUE (node_id),
    CONSTRAINT fk_heartbeats_node FOREIGN KEY (node_id) REFERENCES nodes (id)
);

-- health_scores: current health of a Node (Database.md §62).
CREATE TABLE health_scores (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id           UUID NOT NULL,
    overall_score     INTEGER,
    cpu_score         INTEGER,
    memory_score      INTEGER,
    storage_score     INTEGER,
    network_score     INTEGER,
    temperature_score INTEGER,
    engine_score      INTEGER,
    calculated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_health_scores_node UNIQUE (node_id),
    CONSTRAINT fk_health_scores_node FOREIGN KEY (node_id) REFERENCES nodes (id)
);

-- event_types: catalog of event types (Database.md §64).
CREATE TABLE event_types (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT NOT NULL,
    name        TEXT,
    severity    TEXT NOT NULL DEFAULT 'INFO',
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_event_types_code UNIQUE (code)
);

-- events: relevant events (Database.md §65).
CREATE TABLE events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type_id UUID NOT NULL,
    node_id       UUID,
    torrent_id    UUID,
    user_id       UUID,
    message       TEXT,
    payload       JSONB,
    severity      TEXT NOT NULL DEFAULT 'INFO',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_events_event_type FOREIGN KEY (event_type_id) REFERENCES event_types (id),
    CONSTRAINT fk_events_node FOREIGN KEY (node_id) REFERENCES nodes (id)
);

CREATE INDEX idx_node_credentials_node ON node_credentials (node_id);
CREATE INDEX idx_events_created_at ON events (created_at);
CREATE INDEX idx_events_node ON events (node_id);
CREATE INDEX idx_events_payload ON events USING gin (payload);

-- +goose Down
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS event_types;
DROP TABLE IF EXISTS health_scores;
DROP TABLE IF EXISTS heartbeats;
DROP TABLE IF EXISTS node_credentials;
