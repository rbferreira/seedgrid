-- +goose Up
-- Deterministic Alerts (ADR-008; Alerting — Database.md). An alert is a
-- condition with a lifecycle (opens and closes), unlike an event (point-in-time fact).
-- Deterministic rules for the Alert Worker: Node Offline, low disk, critical SMART.

CREATE TABLE alerts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT NOT NULL,                 -- ex.: NODE_OFFLINE, DISK_LOW, SMART_CRITICAL
    severity    TEXT NOT NULL DEFAULT 'WARNING',
    status      TEXT NOT NULL DEFAULT 'OPEN',  -- OPEN | RESOLVED
    node_id     UUID,                          -- alert may not have an associated Node
    message     TEXT,
    payload     JSONB,
    opened_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_alerts_node FOREIGN KEY (node_id) REFERENCES nodes (id),
    CONSTRAINT ck_alerts_severity CHECK (severity IN ('INFO','WARNING','CRITICAL')),
    CONSTRAINT ck_alerts_status CHECK (status IN ('OPEN','RESOLVED'))
);

-- Deduplication: at most one OPEN alert per (code, node_id). Reopen only after resolving.
-- COALESCE prevents multiple alerts without a Node (node_id NULL) from escaping the unique index.
CREATE UNIQUE INDEX uk_alerts_open_condition
    ON alerts (code, COALESCE(node_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE status = 'OPEN';

CREATE INDEX idx_alerts_status ON alerts (status);
CREATE INDEX idx_alerts_node ON alerts (node_id);
CREATE INDEX idx_alerts_opened_at ON alerts (opened_at);

-- +goose Down
DROP TABLE IF EXISTS alerts;
