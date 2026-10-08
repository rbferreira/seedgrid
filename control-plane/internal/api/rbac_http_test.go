package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/agentconfig"
	"github.com/rbferreira/seedgrid/control-plane/internal/api"
	"github.com/rbferreira/seedgrid/control-plane/internal/audit"
	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
	"github.com/rbferreira/seedgrid/control-plane/internal/users"
)

// setupRouter spins up the full router against a clean database (DEDICATED to the package —
// integration packages run in parallel and DROP SCHEMA on a shared
// database would break the others). Creates 1 user per system role.
// Returns the handler, the login helper and the admin username.
func setupRouter(t *testing.T) (http.Handler, func(string) string, string) {
	t.Helper()
	dsn := os.Getenv("SEEDGRID_API_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("SEEDGRID_API_TEST_DATABASE_URL/SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	poolRaw, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := poolRaw.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		poolRaw.Close()
		t.Fatalf("clean schema: %v", err)
	}
	poolRaw.Close()

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	authSvc := auth.NewService(pool, "reg-token")
	admin := fmt.Sprintf("admin_%d", time.Now().UnixNano())
	if err := authSvc.Bootstrap(context.Background(), admin, "strong-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	usersSvc := users.NewService(pool)
	auditRec := audit.NewRecorder(pool, logger)
	settingsStore := settings.New(pool)
	_ = settingsStore.Load(context.Background())

	// Minimum domain services for the routes exercised in the test (torrents).
	commandSvc := command.NewService(pool)
	schedulerSvc := scheduler.NewService(pool)
	emitter := events.NewEmitter(pool)
	torrentSvc := torrent.NewService(pool, schedulerSvc, commandSvc, emitter)
	nodeSvc := node.NewService(pool, emitter)
	if err := nodeSvc.EnsureDefaultCluster(context.Background()); err != nil {
		t.Fatalf("default cluster: %v", err)
	}

	// Agent behavior configuration per Node (ADR-039): the heartbeat delivers, the
	// override routes respond.
	agentConfigSvc := agentconfig.New(pool, settingsStore)
	agentConfigSvc.SetEvents(emitter)

	handler := api.NewRouter(api.Deps{
		Logger:      logger,
		Pool:        pool,
		Auth:        authSvc,
		Users:       usersSvc,
		Audit:       auditRec,
		Settings:    settingsStore,
		Commands:    commandSvc,
		Torrents:    torrentSvc,
		Nodes:       nodeSvc,
		AgentConfig: agentConfigSvc,
		// Cancel migration (#87) — see migration_cancel_http_test.go.
		Migrations: migration.NewService(pool, commandSvc, emitter, 6900),
	})

	login := func(username string) string {
		t.Helper()
		body := fmt.Sprintf(`{"username":%q,"password":"strong-password"}`, username)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s: status %d: %s", username, rec.Code, rec.Body.String())
		}
		var out struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("login %s: decode: %v", username, err)
		}
		return out.Token
	}

	// Creates one user per system role (the admin is already the Administrator).
	for _, role := range []string{"Publisher", "Downloader", "Viewer"} {
		uname := fmt.Sprintf("%s_%d", strings.ToLower(role), time.Now().UnixNano())
		if _, err := usersSvc.Create(context.Background(), users.CreateInput{
			Username: uname,
			Password: "strong-password",
			Roles:    []string{role},
		}); err != nil {
			t.Fatalf("create %s: %v", role, err)
		}
	}

	return handler, login, admin
}

// tokensByRole logs in with each role (lists users via API with the admin token and
// logs in as each one). Returns map[role]token.
func tokensByRole(t *testing.T, handler http.Handler, login func(string) string, admin string) map[string]string {
	t.Helper()
	adminToken := login(admin)
	tokens := map[string]string{"Administrator": adminToken}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list users: status %d", rec.Code)
	}
	var out struct {
		Data []struct {
			Username string   `json:"username"`
			Roles    []string `json:"roles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	for _, u := range out.Data {
		for _, r := range u.Roles {
			if _, ok := tokens[r]; !ok {
				tokens[r] = login(u.Username)
			}
		}
	}
	return tokens
}

// do executes an authenticated request and returns the code.
func do(t *testing.T, handler http.Handler, method, path, token, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(rec, req)
	return rec.Code
}

// TestRequirePermissionHTTP validates end-to-end RBAC via HTTP: each role against
// representative routes (read ok, mutation blocked, admin passes everything).
// Closes the risk admitted by ADR-035 — route without permission or role with access
// beyond what is declared — without relying on middleware unit tests.
func TestRequirePermissionHTTP(t *testing.T) {
	handler, login, admin := setupRouter(t)
	tokens := tokensByRole(t, handler, login, admin)

	type tc struct {
		name      string
		method    string
		path      string
		body      string
		allowed   []string // roles that MUST have status != 403 (business or success)
		forbidden []string // roles that MUST get 403
	}
	tests := []tc{
		{
			name:      "viewer reads torrents",
			method:    http.MethodGet,
			path:      "/api/v1/torrents",
			allowed:   []string{"Administrator", "Publisher", "Downloader", "Viewer"},
			forbidden: []string{},
		},
		{
			name:      "add torrent: downloader and publisher can",
			method:    http.MethodPost,
			path:      "/api/v1/torrents",
			body:      `{"magnet":""}`, // reaches the handler (400 business, not 403)
			allowed:   []string{"Administrator", "Publisher", "Downloader"},
			forbidden: []string{"Viewer"},
		},
		{
			name:      "creating .torrent requires build (publisher, not downloader)",
			method:    http.MethodPost,
			path:      "/api/v1/torrents/create",
			body:      `x`, // blocks at the permission middleware before the handler
			allowed:   []string{"Administrator", "Publisher"},
			forbidden: []string{"Downloader", "Viewer"},
		},
		{
			name:      "composing publication description requires publish:write (publisher, not downloader)",
			method:    http.MethodPost,
			path:      "/api/v1/publish/description",
			body:      `{"kind":"media"}`, // without tracker_id: reaches the handler (400 business, not 403)
			allowed:   []string{"Administrator", "Publisher"},
			forbidden: []string{"Downloader", "Viewer"},
		},
		{
			name:      "user management: admin only",
			method:    http.MethodGet,
			path:      "/api/v1/users",
			allowed:   []string{"Administrator"},
			forbidden: []string{"Publisher", "Downloader", "Viewer"},
		},
		{
			name:      "audit: admin only",
			method:    http.MethodGet,
			path:      "/api/v1/audit",
			allowed:   []string{"Administrator"},
			forbidden: []string{"Publisher", "Downloader", "Viewer"},
		},
		{
			name:      "remover node: admin only",
			method:    http.MethodDelete,
			path:      "/api/v1/nodes/00000000-0000-0000-0000-000000000000",
			allowed:   []string{"Administrator"},
			forbidden: []string{"Publisher", "Downloader", "Viewer"},
		},
		{
			name:      "rename node (alias): requires nodes:write, admin only",
			method:    http.MethodPut,
			path:      "/api/v1/nodes/00000000-0000-0000-0000-000000000000/display-name",
			body:      `{"display_name":"x"}`,
			allowed:   []string{"Administrator"},
			forbidden: []string{"Publisher", "Downloader", "Viewer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, role := range tt.allowed {
				code := do(t, handler, tt.method, tt.path, tokens[role], tt.body)
				if code == http.StatusForbidden {
					t.Errorf("%s: %s %s → 403, should be allowed", role, tt.method, tt.path)
				}
			}
			for _, role := range tt.forbidden {
				code := do(t, handler, tt.method, tt.path, tokens[role], tt.body)
				if code != http.StatusForbidden {
					t.Errorf("%s: %s %s → %d, should be 403 (forbidden)", role, tt.method, tt.path, code)
				}
			}
		})
	}
}

// TestDisabledUserRevokedHTTP reproduces review bug #1: a live token from a
// user who became DISABLED by status must stop working immediately.
func TestDisabledUserRevokedHTTP(t *testing.T) {
	handler, login, admin := setupRouter(t)
	adminToken := login(admin)

	// Creates a viewer (via API, as the Portal would).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users",
		strings.NewReader(`{"username":"vera","password":"strong-password","roles":["Viewer"]}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create vera: %d %s", rec.Code, rec.Body.String())
	}
	var vera struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &vera); err != nil {
		t.Fatalf("decode vera: %v", err)
	}

	veraToken := login("vera")

	// Live token works before disabling.
	if code := do(t, handler, http.MethodGet, "/api/v1/torrents", veraToken, ""); code == http.StatusUnauthorized {
		t.Fatal("vera token should not be 401 before disabling")
	}

	// Admin disables via status (PUT, like the Portal inline editor).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/users/"+vera.ID,
		strings.NewReader(`{"status":"DISABLED"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable vera: %d %s", rec.Code, rec.Body.String())
	}

	// OLD token must be rejected immediately (bug #1: Authenticate filters
	// status != ACTIVE AND Update invalidates the sessions).
	if code := do(t, handler, http.MethodGet, "/api/v1/torrents", veraToken, ""); code != http.StatusUnauthorized {
		t.Errorf("old vera token after DISABLED: %d, should be 401", code)
	}
	if code := do(t, handler, http.MethodGet, "/api/v1/auth/me", veraToken, ""); code != http.StatusUnauthorized {
		t.Errorf("auth/me with old token after DISABLED: %d, should be 401", code)
	}
}

// TestLastAdminCannotDisableSelfHTTP reproduces review bug #3: the last admin
// cannot disable themselves by status nor lose the role (409).
func TestLastAdminCannotDisableSelfHTTP(t *testing.T) {
	handler, login, admin := setupRouter(t)
	adminToken := login(admin)

	// Discovers the id of the admin itself (GET /users).
	var adminID string
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		handler.ServeHTTP(rec, req)
		var out struct {
			Data []struct {
				ID       string   `json:"id"`
				Username string   `json:"username"`
				Roles    []string `json:"roles"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, u := range out.Data {
			if u.Username == admin {
				adminID = u.ID
			}
		}
	}
	if adminID == "" {
		t.Fatal("admin not found in list")
	}

	// Single admin: disable by status → 409 last_admin.
	if code := do(t, handler, http.MethodPut, "/api/v1/users/"+adminID, adminToken, `{"status":"DISABLED"}`); code != http.StatusConflict {
		t.Errorf("last admin disabling self: %d, should be 409", code)
	}

	// Remove own Administrator role → 409.
	if code := do(t, handler, http.MethodPut, "/api/v1/users/"+adminID, adminToken, `{"roles":["Viewer"]}`); code != http.StatusConflict {
		t.Errorf("last admin losing role: %d, should be 409", code)
	}
}
