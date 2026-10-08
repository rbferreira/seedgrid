-- +goose Up
-- Cluster (Database.md §9), Nodes (§10-14) and Configuration (§71) domains.

-- clusters
CREATE TABLE clusters (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uuid        UUID NOT NULL DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT,
    timezone    TEXT NOT NULL DEFAULT 'UTC',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    deleted_by  UUID,
    CONSTRAINT uk_clusters_uuid UNIQUE (uuid)
);

-- nodes (Seed Nodes). Detailed hardware belongs to the Inventory domain.
CREATE TABLE nodes (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cluster_id           UUID NOT NULL,
    uuid                 UUID NOT NULL DEFAULT gen_random_uuid(),
    hostname             TEXT NOT NULL,
    display_name         TEXT,
    description          TEXT,
    fqdn                 TEXT,
    management_ip        INET,
    management_port      INTEGER,
    agent_version        TEXT,
    engine               TEXT,
    engine_version       TEXT,
    os_name              TEXT,
    os_version           TEXT,
    kernel_version       TEXT,
    architecture         TEXT,
    timezone             TEXT,
    status               TEXT NOT NULL DEFAULT 'UNKNOWN',
    health_score         INTEGER,
    inventory_version    BIGINT NOT NULL DEFAULT 0,
    inventory_updated_at TIMESTAMPTZ,
    registered_at        TIMESTAMPTZ,
    last_seen_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ,
    deleted_by           UUID,
    CONSTRAINT uk_nodes_uuid UNIQUE (uuid),
    CONSTRAINT uk_nodes_hostname_per_cluster UNIQUE (cluster_id, hostname),
    CONSTRAINT uk_nodes_management_ip UNIQUE (management_ip),
    CONSTRAINT fk_nodes_cluster FOREIGN KEY (cluster_id) REFERENCES clusters (id),
    CONSTRAINT ck_nodes_status CHECK (status IN ('ONLINE','OFFLINE','DRAIN','MAINTENANCE','FAILED','UNKNOWN','DISABLED')),
    CONSTRAINT ck_nodes_health_score CHECK (health_score IS NULL OR (health_score BETWEEN 0 AND 100))
);

-- node_labels: defined by the operator (Database.md §12)
CREATE TABLE node_labels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id     UUID NOT NULL,
    key         TEXT NOT NULL,
    value       TEXT,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_node_labels_node_key UNIQUE (node_id, key),
    CONSTRAINT fk_node_labels_node FOREIGN KEY (node_id) REFERENCES nodes (id)
);

-- node_capabilities: automatically detected by the Agent (Database.md §13)
CREATE TABLE node_capabilities (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id     UUID NOT NULL,
    capability  TEXT NOT NULL,
    value       TEXT,
    source      TEXT,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_node_capabilities_node_cap UNIQUE (node_id, capability),
    CONSTRAINT fk_node_capabilities_node FOREIGN KEY (node_id) REFERENCES nodes (id)
);

-- node_tags: automatically generated, transient (Database.md §14)
CREATE TABLE node_tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id    UUID NOT NULL,
    tag        TEXT NOT NULL,
    source     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_node_tags_node_tag UNIQUE (node_id, tag),
    CONSTRAINT fk_node_tags_node FOREIGN KEY (node_id) REFERENCES nodes (id)
);

-- settings: global settings (Database.md §71)
CREATE TABLE settings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category    TEXT NOT NULL,
    key         TEXT NOT NULL,
    value       TEXT,
    description TEXT,
    updated_by  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_settings_category_key UNIQUE (category, key)
);

CREATE INDEX idx_nodes_hostname ON nodes (hostname);
CREATE INDEX idx_nodes_management_ip ON nodes (management_ip);
CREATE INDEX idx_nodes_status ON nodes (status);
CREATE INDEX idx_nodes_health_score ON nodes (health_score);
CREATE INDEX idx_nodes_last_seen_at ON nodes (last_seen_at);
CREATE INDEX idx_node_labels_node ON node_labels (node_id);
CREATE INDEX idx_node_capabilities_node ON node_capabilities (node_id);
CREATE INDEX idx_node_tags_node ON node_tags (node_id);

-- +goose Down
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS node_tags;
DROP TABLE IF EXISTS node_capabilities;
DROP TABLE IF EXISTS node_labels;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS clusters;
