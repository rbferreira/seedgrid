-- +goose Up
-- ADR-035: RBAC authorization for operators — role and permission seeds.
-- Idempotent (ON CONFLICT DO NOTHING): safe to run on existing installations.

-- System roles (ADR-035). Administrator may already exist via Bootstrap (00001+).
INSERT INTO roles (name, description, system) VALUES
    ('Administrator', 'Full administrative access', true),
    ('Publisher',     'Manages content: torrents, trackers, publishing', true),
    ('Downloader',    'Consumes content: reads and downloads torrents', true),
    ('Viewer',        'Read-only: observes the cluster without mutating anything', true)
ON CONFLICT (name) DO NOTHING;

-- Permissions in resource:action format (ADR-035).
INSERT INTO permissions (resource, action, description) VALUES
    ('torrents', 'read',     'List and inspect torrents'),
    ('torrents', 'create',   'Add torrent (magnet/file)'),
    ('torrents', 'build',    'Create a .torrent from local content (publishing)'),
    ('torrents', 'write',    'Pause, resume, recheck, prioritize'),
    ('torrents', 'delete',   'Remove torrent'),
    ('torrents', 'migrate',  'Migrate torrent between Nodes'),
    ('torrents', 'publish',  'Publish to the tracker and generate screenshots'),
    ('torrents', 'download', 'Issue a download ticket'),
    ('nodes',    'read',     'List and inspect Nodes'),
    ('nodes',    'write',    'Remove, drain, restart Engine, update Agent'),
    ('nodes',    'command',  'Queue commands on Nodes'),
    ('nodes',    'operation-mode', 'Change operation mode (cluster/Node)'),
    ('trackers', 'read',     'List trackers and browse the catalog'),
    ('trackers', 'write',    'Create, edit, remove trackers and set the default'),
    ('trackers', 'seed',     'Seed a torrent from the tracker'),
    ('publish',  'read',     'Search games for publishing (IGDB)'),
    ('publish',  'write',    'Infer publishing metadata'),
    ('settings', 'read',     'List settings'),
    ('settings', 'write',    'Change/reset settings'),
    ('alerts',   'read',     'List alerts'),
    ('events',   'read',     'List events'),
    ('activities','read',    'List the activity feed'),
    ('audit',    'read',     'List the audit trail'),
    ('metrics',  'read',     'Query cluster metrics'),
    ('system',   'read',     'Query system health'),
    ('commands', 'read',     'Query commands'),
    ('migrations','read',    'List torrent migrations'),
    ('users',    'read',     'List users'),
    ('users',    'write',    'Create, edit, disable users and roles'),
    ('enroll',   'write',    'Issue an onboarding join token'),
    ('notify',   'write',    'Send a test notification'),
    ('*',        '*',        'Wildcard: full access (Administrator)')
ON CONFLICT (resource, action) DO NOTHING;

-- Administrator → wildcard (resolves everything in code; explicit seed for clarity).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'Administrator' AND p.resource = '*' AND p.action = '*'
ON CONFLICT DO NOTHING;

-- Publisher: manages content (torrents + trackers + publishing), general read.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'Publisher'
  AND (p.resource, p.action) IN (
    ('torrents','read'), ('torrents','create'), ('torrents','build'),
    ('torrents','write'), ('torrents','delete'), ('torrents','migrate'),
    ('torrents','publish'), ('torrents','download'),
    ('nodes','read'),
    ('trackers','read'), ('trackers','write'), ('trackers','seed'),
    ('publish','read'), ('publish','write'),
    ('settings','read'),
    ('alerts','read'), ('events','read'), ('activities','read'),
    ('metrics','read'), ('system','read'), ('commands','read'), ('migrations','read')
  )
ON CONFLICT DO NOTHING;

-- Downloader: consumes (reads + downloads + adds for own download).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'Downloader'
  AND (p.resource, p.action) IN (
    ('torrents','read'), ('torrents','create'), ('torrents','download'),
    ('nodes','read'),
    ('trackers','read'),
    ('publish','read'),
    ('settings','read'),
    ('alerts','read'), ('events','read'), ('activities','read'),
    ('metrics','read'), ('system','read'), ('commands','read'), ('migrations','read')
  )
ON CONFLICT DO NOTHING;

-- Viewer: read-only.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'Viewer'
  AND (p.resource, p.action) IN (
    ('torrents','read'),
    ('nodes','read'),
    ('trackers','read'),
    ('publish','read'),
    ('settings','read'),
    ('alerts','read'), ('events','read'), ('activities','read'),
    ('metrics','read'), ('system','read'), ('commands','read'), ('migrations','read')
  )
ON CONFLICT DO NOTHING;

-- +goose Down
-- Seeds are idempotent and should not be reverted (removing roles would break
-- existing associations). Down intentionally empty — ADR-035.
