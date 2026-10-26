-- +goose Up
-- ADR-047: trackers belong to the cluster and the Administrator manages them. Until now
-- every query filtered on the registering operator; names were unique per operator and
-- each operator had their own default.

-- Names become unique across the cluster. A duplicate keeps its row and gets its owner's
-- username appended; the oldest keeps the plain name.
UPDATE trackers t
   SET name = t.name || ' (' || u.username || ')', updated_at = now()
  FROM users u
 WHERE u.id = t.user_id
   AND t.deleted_at IS NULL
   AND EXISTS (SELECT 1 FROM trackers o
                WHERE o.name = t.name AND o.deleted_at IS NULL
                  AND (o.created_at, o.id) < (t.created_at, t.id));

-- One default for the cluster: the oldest one stays.
UPDATE trackers t
   SET is_default = false, updated_at = now()
 WHERE t.is_default AND t.deleted_at IS NULL
   AND EXISTS (SELECT 1 FROM trackers o
                WHERE o.is_default AND o.deleted_at IS NULL
                  AND (o.created_at, o.id) < (t.created_at, t.id));

DROP INDEX uk_trackers_user_name;
DROP INDEX uk_trackers_one_default;
DROP INDEX ix_trackers_user;

CREATE UNIQUE INDEX uk_trackers_name
    ON trackers (name)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX uk_trackers_one_default
    ON trackers (is_default)
    WHERE is_default AND deleted_at IS NULL;

-- Who registered the tracker: a record only, it grants nothing.
ALTER TABLE trackers RENAME COLUMN user_id TO created_by;
ALTER TABLE trackers RENAME CONSTRAINT trackers_user_id_fkey TO trackers_created_by_fkey;

-- The Publisher picks trackers and seeds from them; it no longer changes them.
DELETE FROM role_permissions rp
 USING roles r, permissions p
 WHERE rp.role_id = r.id AND rp.permission_id = p.id
   AND r.name = 'Publisher' AND p.resource = 'trackers' AND p.action = 'write';

-- +goose Down
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
  FROM roles r, permissions p
 WHERE r.name = 'Publisher' AND p.resource = 'trackers' AND p.action = 'write'
ON CONFLICT DO NOTHING;

ALTER TABLE trackers RENAME CONSTRAINT trackers_created_by_fkey TO trackers_user_id_fkey;
ALTER TABLE trackers RENAME COLUMN created_by TO user_id;

DROP INDEX uk_trackers_one_default;
DROP INDEX uk_trackers_name;

CREATE UNIQUE INDEX uk_trackers_user_name
    ON trackers (user_id, name)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX uk_trackers_one_default
    ON trackers (user_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE INDEX ix_trackers_user ON trackers (user_id) WHERE deleted_at IS NULL;
