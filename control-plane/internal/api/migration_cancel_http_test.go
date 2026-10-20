package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// Cancel migration (#87), end-to-end over HTTP: RBAC (torrents:migrate), clear 409s, and the
// audit trail (ADR-025).

// post performs the authenticated POST (reuses the package `call`) and returns the status and
// decoded body; errCode and auditHas also come from node_display_name_test.go.
func post(t *testing.T, handler http.Handler, path, token string) (int, map[string]any, []byte) {
	t.Helper()
	code, raw := call(t, handler, http.MethodPost, path, token, "", nil)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return code, body, raw
}

func errMessage(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	msg, _ := e["message"].(string)
	return msg
}

// Whoever can migrate can cancel — and only those who can migrate: canceling discards the copy at the
// destination, it is the opposite of migration and shouldn't be behind a weaker permission.
func TestMigrationCancelRBAC(t *testing.T) {
	handler, login, admin := setupRouter(t)
	tokens := tokensByRole(t, handler, login, admin)
	const path = "/api/v1/migrations/00000000-0000-0000-0000-000000000000/cancel"

	for _, role := range []string{"Administrator", "Publisher"} {
		code, _, raw := post(t, handler, path, tokens[role])
		if code != http.StatusNotFound || errCode(t, raw) != "migration_not_found" {
			t.Errorf("%s: %d %s, want 404 not_found (reached handler and migration does not exist)", role, code, raw)
		}
	}
	for _, role := range []string{"Downloader", "Viewer"} {
		if code, _, _ := post(t, handler, path, tokens[role]); code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403 — without torrents:migrate a migration cannot be canceled", role, code)
		}
	}
	// Malformed ID is also 404, not 500.
	if code, _, _ := post(t, handler, "/api/v1/migrations/not-a-uuid/cancel", tokens["Administrator"]); code != http.StatusNotFound {
		t.Errorf("malformed id: %d, want 404", code)
	}
}

func TestMigrationCancelHTTP(t *testing.T) {
	handler, login, admin := setupRouter(t)
	tokens := tokensByRole(t, handler, login, admin)

	// Migration data, straight into the DB the router uses (setupRouter recreated it).
	dsn := os.Getenv("SEEDGRID_API_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	emitter := events.NewEmitter(pool)
	cmds := command.NewService(pool)
	nodes := node.NewService(pool, emitter)
	if err := nodes.EnsureDefaultCluster(ctx); err != nil {
		t.Fatalf("default cluster: %v", err)
	}
	var nodeIDs []string
	for _, host := range []string{"cancel-http-a", "cancel-http-b"} {
		n, err := nodes.Register(ctx, node.RegisterInput{Hostname: host, Engine: "qbittorrent"})
		if err != nil {
			t.Fatalf("register %s: %v", host, err)
		}
		nodeIDs = append(nodeIDs, n.NodeID)
	}
	if _, err := pool.Exec(ctx, `UPDATE nodes SET last_ip = '10.0.0.9' WHERE id = ANY($1::uuid[])`, nodeIDs); err != nil {
		t.Fatalf("last_ip: %v", err)
	}
	torrents := torrent.NewService(pool, scheduler.NewService(pool), cmds, emitter)
	migs := migration.NewService(pool, cmds, emitter, 6900)

	startMigration := func(hash, name string) (torrent.Torrent, migration.Migration) {
		t.Helper()
		tr, err := torrents.Add(ctx, "magnet:?xt=urn:btih:"+hash+"&dn="+name, "", torrent.AddOptions{AllowOtherVersion: true})
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		if err := torrents.StoreTorrentFile(ctx, tr.ID, []byte("d4:infod6:lengthi1e4:name4:demo12:piece lengthi16384eee"), "upload"); err != nil {
			t.Fatalf("StoreTorrentFile: %v", err)
		}
		dest := nodeIDs[0]
		if tr.NodeID == dest {
			dest = nodeIDs[1]
		}
		m, err := migs.Migrate(ctx, tr.ID, dest)
		if err != nil {
			t.Fatalf("Migrate: %v", err)
		}
		if err := migs.ReconcileAndPromote(ctx, 10); err != nil {
			t.Fatalf("promote: %v", err)
		}
		return tr, m
	}

	tr, m := startMigration("c4a0000000000000000000000000000000000001", "CancelHTTP")
	adminTok, pubTok := tokens["Administrator"], tokens["Publisher"]

	// During migration the action is rejected — and the rejection says it can be canceled, and which one.
	code, body, raw := post(t, handler, "/api/v1/torrents/"+tr.ID+"/pause", adminTok)
	if code != http.StatusConflict || errCode(t, raw) != "migration_in_flight" {
		t.Fatalf("pause during migration: %d %s, want 409 migration_in_flight", code, raw)
	}
	if msg := errMessage(body); !strings.Contains(msg, "cancel the migration") || !strings.Contains(msg, "/api/v1/migrations/"+m.ID+"/cancel") {
		t.Errorf("message for 409 = %q, want it to say cancellation is possible and point to migration %s", msg, m.ID)
	}

	// Cancel (as Publisher, which has torrents:migrate).
	code, body, raw = post(t, handler, "/api/v1/migrations/"+m.ID+"/cancel", pubTok)
	if code != http.StatusOK {
		t.Fatalf("cancel: %d %s, want 200", code, raw)
	}
	if body["status"] != "FAILED" || body["cancelled"] != true || body["error"] != migration.CancelReason {
		t.Errorf("body = %v, want status FAILED, cancelled true and reason %q", body, migration.CancelReason)
	}

	// Repeat: clear 409, without re-executing anything.
	code, _, raw = post(t, handler, "/api/v1/migrations/"+m.ID+"/cancel", pubTok)
	if code != http.StatusConflict || errCode(t, raw) != "migration_already_cancelled" {
		t.Errorf("cancel again: %d %s, want 409 migration_already_cancelled", code, raw)
	}

	// The torrent is free again.
	if code, _, raw := post(t, handler, "/api/v1/torrents/"+tr.ID+"/pause", adminTok); code != http.StatusOK {
		t.Errorf("pause after cancellation: %d %s, want 200", code, raw)
	}

	// Migration that finished via another path: 409 migration_finished.
	_, done := startMigration("c4a0000000000000000000000000000000000002", "CancelHTTPDone")
	if _, err := pool.Exec(ctx, `UPDATE torrent_migrations SET status = 'COMPLETED' WHERE id = $1`, done.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	code, _, raw = post(t, handler, "/api/v1/migrations/"+done.ID+"/cancel", adminTok)
	if code != http.StatusConflict || errCode(t, raw) != "migration_finished" {
		t.Errorf("canceling finished migration: %d %s, want 409 migration_finished", code, raw)
	}

	// Audit (ADR-025): the cancellation appears with friendly text, torrent and Nodes. The
	// rejected attempts are also recorded (route label), with the status they received.
	host := map[string]string{nodeIDs[0]: "cancel-http-a", nodeIDs[1]: "cancel-http-b"}
	want := "Cancelled the migration of torrent «CancelHTTP» from Node «" + host[m.SourceNodeID] +
		"» to Node «" + host[m.DestNodeID] + "»"
	if !auditHas(t, handler, adminTok, want, http.StatusOK) {
		t.Errorf("the audit trail should have %q (200)", want)
	}
	if !auditHas(t, handler, adminTok, "Canceled a migration", http.StatusConflict) {
		t.Error("rejected attempts (409) should appear in the audit trail with the route label")
	}
}
