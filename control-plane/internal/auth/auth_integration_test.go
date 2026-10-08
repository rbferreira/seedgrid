package auth_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
)

func setup(t *testing.T) *auth.Service {
	t.Helper()
	// OWN database for the package: integration tests run in parallel and the
	// DROP SCHEMA here would drop the schema of other packages that share
	// SEEDGRID_TEST_DATABASE_URL. Use a dedicated database (fallback to generic).
	dsn := os.Getenv("SEEDGRID_AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("SEEDGRID_AUTH_TEST_DATABASE_URL/SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	// Clean database per execution (fixed names in tests; DROP SCHEMA ensures
	// repeatability — extensions are restored with CREATE EXTENSION IF NOT EXISTS).
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
	return auth.NewService(pool, "reg-token")
}

func TestChangePassword(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	const user = "chpwd_test"
	if err := s.Bootstrap(ctx, user, "old-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	res, err := s.Login(ctx, user, "old-password", "", "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	u, err := s.Authenticate(ctx, res.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	if err := s.ChangePassword(ctx, u.ID, "wrong", "new-password"); err != auth.ErrInvalidCredentials {
		t.Errorf("wrong current password should fail, got %v", err)
	}
	if err := s.ChangePassword(ctx, u.ID, "old-password", "123"); err != auth.ErrWeakPassword {
		t.Errorf("short password should fail with ErrWeakPassword, got %v", err)
	}
	if err := s.ChangePassword(ctx, u.ID, "old-password", "new-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := s.Login(ctx, user, "old-password", "", ""); err == nil {
		t.Error("old password should fail after change")
	}
	if _, err := s.Login(ctx, user, "new-password", "", ""); err != nil {
		t.Errorf("new password should work: %v", err)
	}
}

func TestBootstrapLoginAuthenticate(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	const user = "admin_test"

	if err := s.Bootstrap(ctx, user, "strong-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// Idempotent.
	if err := s.Bootstrap(ctx, user, "strong-password"); err != nil {
		t.Fatalf("idempotent bootstrap: %v", err)
	}

	res, err := s.Login(ctx, user, "strong-password", "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.Token == "" {
		t.Fatal("empty token")
	}
	if !res.Expires.After(time.Now()) {
		t.Errorf("expiration in the past: %v", res.Expires)
	}
	if res.DefaultCredentials {
		t.Error("strong password should not set DefaultCredentials")
	}

	got, err := s.Authenticate(ctx, res.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.Username != user {
		t.Errorf("username = %q, want %q", got.Username, user)
	}

	if _, err := s.Login(ctx, user, "wrong-password", "", ""); err == nil {
		t.Error("login with wrong password should fail")
	}
	if _, err := s.Authenticate(ctx, "invalid-token"); err == nil {
		t.Error("authenticate with invalid token should fail")
	}
}

// TestAuthenticate_ThrottleLastActivity (#114): the Portal authenticates on EVERY request;
// last_activity should only be updated when stale (60s window) — within the
// window the UPDATE writes nothing (no WAL/bloat per request).
func TestAuthenticate_ThrottleLastActivity(t *testing.T) {
	svc := setup(t)
	dsn := os.Getenv("SEEDGRID_AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	if err := svc.Bootstrap(ctx, "admin-throttle", "throttle-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	res, err := svc.Login(ctx, "admin-throttle", "throttle-password", "", "test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	activity := func() time.Time {
		var ts time.Time
		if err := pool.QueryRow(ctx,
			`SELECT last_activity FROM sessions WHERE session_token = $1`, res.Token).Scan(&ts); err != nil {
			t.Fatalf("read last_activity: %v", err)
		}
		return ts
	}

	// Within the window (login just wrote the default now()): authenticating DOES NOT update.
	before := activity()
	if _, err := svc.Authenticate(ctx, res.Token); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got := activity(); !got.Equal(before) {
		t.Errorf("last_activity changed within the window: %v → %v", before, got)
	}

	// Stale beyond the window: authenticating updates.
	if _, err := pool.Exec(ctx,
		`UPDATE sessions SET last_activity = now() - interval '2 minutes' WHERE session_token = $1`,
		res.Token); err != nil {
		t.Fatal(err)
	}
	stale := activity()
	if _, err := svc.Authenticate(ctx, res.Token); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got := activity(); !got.After(stale) {
		t.Errorf("stale last_activity was not rewritten: %v → %v", stale, got)
	}
}
