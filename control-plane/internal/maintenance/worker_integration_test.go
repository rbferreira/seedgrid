package maintenance_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/maintenance"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// TestSweep (#112): the sweep only removes what has passed retention — TERMINAL commands
// older than retention, old events/activities/audit, and sessions expired for more than 7 days.
// PENDING/DISPATCHED (any age), recent rows, and live sessions survive.
func TestSweep(t *testing.T) {
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	store := settings.New(pool)
	if err := store.Load(ctx); err != nil {
		t.Fatalf("settings: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := maintenance.NewWorker(pool, logger, store)

	nodeSvc := node.NewService(pool, events.NewEmitter(pool))
	if err := nodeSvc.EnsureDefaultCluster(ctx); err != nil {
		t.Fatalf("cluster: %v", err)
	}
	reg, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "maint-test-node"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("seed (%s): %v", q[:40], err)
		}
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	// The test database ACCUMULATES state between runs (see testing.md): clean the seeds
	// for this test before re-inserting, otherwise counts and token uniqueness collide.
	// Migrations are pruned before the commands, which they reference.
	exec(`DELETE FROM torrent_migrations WHERE torrent_id IN (SELECT id FROM torrents WHERE name LIKE 'maint-%')`)
	exec(`DELETE FROM torrents WHERE name LIKE 'maint-%'`)
	exec(`DELETE FROM commands WHERE type LIKE 'maint_%'`)
	exec(`DELETE FROM events WHERE event_type_id IN (SELECT id FROM event_types WHERE code = 'MAINT_TEST')`)
	exec(`DELETE FROM activities WHERE message LIKE 'maint-%'`)
	exec(`DELETE FROM audit_log WHERE actor = 'maint-teste'`)
	exec(`DELETE FROM sessions WHERE session_token LIKE 'maint-tok-%'`)

	// Commands: old DONE (pruned), old PENDING (kept), recent DONE (kept).
	exec(`INSERT INTO commands (node_id, type, status, created_at, finished_at)
	      VALUES ($1, 'maint_old_done', 'DONE', now() - interval '2 years', now() - interval '2 years')`, reg.NodeID)
	exec(`INSERT INTO commands (node_id, type, status, created_at)
	      VALUES ($1, 'maint_old_pending', 'PENDING', now() - interval '2 years')`, reg.NodeID)
	exec(`INSERT INTO commands (node_id, type, status, created_at, finished_at)
	      VALUES ($1, 'maint_new_done', 'DONE', now(), now())`, reg.NodeID)

	// Migration commands (#213): old DONE, referenced by a completed migration.
	// The FKs don't have ON DELETE, and ONE of them inside the batch would fail the entire DELETE — the
	// maint_old_done above survived along with it. In production, 64 commands like this blocked the pruning
	// of another 53.732, and the table had never been pruned.
	dest, err := nodeSvc.Register(ctx, node.RegisterInput{Hostname: "maint-test-node-dest"})
	if err != nil {
		t.Fatalf("register destination: %v", err)
	}
	var serveID, pullID, migTorrent, queuedTorrent string
	for _, c := range []struct {
		dst  *string
		kind string
	}{{&serveID, "maint_old_mig_serve"}, {&pullID, "maint_old_mig_pull"}} {
		if err := pool.QueryRow(ctx,
			`INSERT INTO commands (node_id, type, status, created_at, finished_at)
			 VALUES ($1, $2, 'DONE', now() - interval '2 years', now() - interval '2 years')
			 RETURNING id::text`, reg.NodeID, c.kind).Scan(c.dst); err != nil {
			t.Fatalf("seed migration command: %v", err)
		}
	}
	for _, tr := range []struct {
		dst        *string
		name, hash string
	}{
		{&migTorrent, "maint-migrado", "aa00bb11cc22dd33ee44ff5566778899aabbcc01"},
		{&queuedTorrent, "maint-na-fila", "aa00bb11cc22dd33ee44ff5566778899aabbcc02"},
	} {
		if err := pool.QueryRow(ctx,
			`INSERT INTO torrents (node_id, hash, name, status) VALUES ($1, $2, $3, 'SEEDING')
			 RETURNING id::text`, reg.NodeID, tr.hash, tr.name).Scan(tr.dst); err != nil {
			t.Fatalf("seed torrent: %v", err)
		}
	}
	exec(`INSERT INTO torrent_migrations (torrent_id, source_node_id, dest_node_id, status,
	                                      serve_command_id, pull_command_id, created_at)
	      VALUES ($1, $2, $3, 'COMPLETED', $4, $5, now() - interval '2 years')`,
		migTorrent, reg.NodeID, dest.NodeID, serveID, pullID)
	// Queued migration: both NULL columns. It's the one that would catch a NOT IN instead of NOT
	// EXISTS — a NULL in the list makes NOT IN prune NOTHING, with no error at all.
	exec(`INSERT INTO torrent_migrations (torrent_id, source_node_id, dest_node_id, status)
	      VALUES ($1, $2, $3, 'QUEUED')`, queuedTorrent, reg.NodeID, dest.NodeID)

	// Event: test's own type + one old and one recent.
	exec(`INSERT INTO event_types (code, name) VALUES ('MAINT_TEST', 'teste de retenção')
	      ON CONFLICT (code) DO NOTHING`)
	exec(`INSERT INTO events (event_type_id, message, created_at)
	      SELECT id, 'velho', now() - interval '2 years' FROM event_types WHERE code = 'MAINT_TEST'`)
	exec(`INSERT INTO events (event_type_id, message, created_at)
	      SELECT id, 'recente', now() FROM event_types WHERE code = 'MAINT_TEST'`)

	// Activity and audit: one old and one recent of each.
	exec(`INSERT INTO activities (kind, message, created_at) VALUES ('scheduled', 'maint-velho', now() - interval '2 years')`)
	exec(`INSERT INTO activities (kind, message, created_at) VALUES ('scheduled', 'maint-recente', now())`)
	exec(`INSERT INTO audit_log (actor, action, created_at) VALUES ('maint-teste', 'velho', now() - interval '3 years')`)
	exec(`INSERT INTO audit_log (actor, action, created_at) VALUES ('maint-teste', 'recente', now())`)

	// Sessions: own user + expired for 30d (pruned) and live (kept).
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, display_name, password_hash, status)
		 VALUES ('maint-user', 'maint', 'x', 'ACTIVE')
		 ON CONFLICT (username) DO UPDATE SET updated_at = now()
		 RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	exec(`INSERT INTO sessions (user_id, session_token, expires_at)
	      VALUES ($1, 'maint-tok-expirada', now() - interval '30 days')`, userID)
	exec(`INSERT INTO sessions (user_id, session_token, expires_at)
	      VALUES ($1, 'maint-tok-viva', now() + interval '1 hour')`, userID)

	w.Sweep(ctx)

	if n := count(`SELECT count(*) FROM commands WHERE type = 'maint_old_done'`); n != 0 {
		t.Errorf("old DONE command survived (%d)", n)
	}
	if n := count(`SELECT count(*) FROM commands WHERE type = 'maint_old_pending'`); n != 1 {
		t.Errorf("old PENDING command was pruned (%d, want 1) — PENDING is never pruned", n)
	}
	if n := count(`SELECT count(*) FROM commands WHERE type = 'maint_new_done'`); n != 1 {
		t.Errorf("recent DONE command was pruned (%d, want 1)", n)
	}
	if n := count(`SELECT count(*) FROM commands WHERE type IN ('maint_old_mig_serve', 'maint_old_mig_pull')`); n != 2 {
		t.Errorf("command referenced by migration was pruned (%d, want 2) — "+
			"serve/pull_command_id IS NOT NULL decides migration cleanup", n)
	}
	if n := count(`SELECT count(*) FROM events WHERE message = 'velho'
	               AND event_type_id = (SELECT id FROM event_types WHERE code = 'MAINT_TEST')`); n != 0 {
		t.Errorf("old event survived (%d)", n)
	}
	if n := count(`SELECT count(*) FROM events WHERE message = 'recente'
	               AND event_type_id = (SELECT id FROM event_types WHERE code = 'MAINT_TEST')`); n != 1 {
		t.Errorf("recent event was pruned (%d, want 1)", n)
	}
	if n := count(`SELECT count(*) FROM activities WHERE message = 'maint-velho'`); n != 0 {
		t.Errorf("old activity survived (%d)", n)
	}
	if n := count(`SELECT count(*) FROM activities WHERE message = 'maint-recente'`); n != 1 {
		t.Errorf("recent activity was pruned (%d, want 1)", n)
	}
	if n := count(`SELECT count(*) FROM audit_log WHERE actor = 'maint-teste' AND action = 'velho'`); n != 0 {
		t.Errorf("old audit survived (%d)", n)
	}
	if n := count(`SELECT count(*) FROM audit_log WHERE actor = 'maint-teste' AND action = 'recente'`); n != 1 {
		t.Errorf("recent audit was pruned (%d, want 1)", n)
	}
	if n := count(`SELECT count(*) FROM sessions WHERE session_token = 'maint-tok-expirada'`); n != 0 {
		t.Errorf("session expired 30d ago survived (%d)", n)
	}
	if n := count(`SELECT count(*) FROM sessions WHERE session_token = 'maint-tok-viva'`); n != 1 {
		t.Errorf("live session was pruned (%d, want 1)", n)
	}

	// Retention 0 disables: a new old one survives the sweep with retention.events = 0.
	if err := store.Set(ctx, "retention.events", "0", "maint-teste"); err != nil {
		t.Fatalf("disable retention.events: %v", err)
	}
	t.Cleanup(func() { _ = store.Reset(context.Background(), "retention.events", "maint-teste") })
	exec(`INSERT INTO events (event_type_id, message, created_at)
	      SELECT id, 'velho-desligado', now() - interval '2 years' FROM event_types WHERE code = 'MAINT_TEST'`)
	w.Sweep(ctx)
	if n := count(`SELECT count(*) FROM events WHERE message = 'velho-desligado'`); n != 1 {
		t.Errorf("retention 0 should disable pruning of events (%d, want 1)", n)
	}
}
