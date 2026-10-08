-- +goose Up
-- Durable command queue (ADR-014: delivery via polling in V1). The Control Plane
-- enqueues; the Agent polls, executes and confirms the result.

CREATE TABLE commands (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id       UUID NOT NULL,
    type          TEXT NOT NULL,
    payload       JSONB,
    status        TEXT NOT NULL DEFAULT 'PENDING',
    result        JSONB,
    error         TEXT,
    retry_count   INTEGER NOT NULL DEFAULT 0,
    created_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched_at TIMESTAMPTZ,
    finished_at   TIMESTAMPTZ,
    CONSTRAINT fk_commands_node FOREIGN KEY (node_id) REFERENCES nodes (id),
    CONSTRAINT ck_commands_status CHECK (status IN ('PENDING','DISPATCHED','DONE','FAILED'))
);

CREATE INDEX idx_commands_node_status ON commands (node_id, status);
CREATE INDEX idx_commands_created_at ON commands (created_at);

-- +goose Down
DROP TABLE IF EXISTS commands;
