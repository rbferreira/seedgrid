-- +goose Up
-- Audit Log (ADR-025): dedicated, append-only trail of the operator's ACTIONS on the API —
-- "who did what, when, from where". SEPARATE from Alerts (problems), Events (domain
-- facts) and Activity Feed (orchestration). Recorded by a middleware at the boundary over
-- the protected mutating routes (POST/PUT/DELETE) + login. It is the accountability lens.

CREATE TABLE audit_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor        TEXT NOT NULL,                       -- authenticated username (or the attempted username, on login)
    action       TEXT NOT NULL,                       -- short semantic label, e.g.: login, login_failed, POST /api/v1/torrents
    method       TEXT,                                -- HTTP method (NULL for non-HTTP events like login)
    path         TEXT,                                -- concrete path, with ids (e.g.: /api/v1/torrents/<id>/pause)
    status_code  INTEGER,                             -- HTTP response status (NULL when not applicable)
    source_ip    TEXT,                                -- request source IP
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Default read: most recent first (timeline) and by actor (filter).
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at DESC);
CREATE INDEX idx_audit_log_actor ON audit_log (actor);

-- +goose Down
DROP TABLE audit_log;
