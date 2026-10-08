package settings_test

import (
	"context"
	"os"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// setup opens the ISOLATED test Postgres (SEEDGRID_TEST_DATABASE_URL — never prod;
// in-memory isolated-db-for-worker-tests) and clears the settings table to isolate cases.
func setup(t *testing.T) *settings.Store {
	t.Helper()
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
	if _, err := pool.Exec(context.Background(), `TRUNCATE settings`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return settings.New(pool)
}

// TestSeedFromEnv: with no row in the DB and env set, Load seeds the value.
func TestSeedFromEnv(t *testing.T) {
	s := setup(t)
	t.Setenv("SEEDGRID_ADMISSION_DEAD_GRACE", "9m")
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := s.Duration("admission.dead_grace"); got.String() != "9m0s" {
		t.Errorf("dead_grace after seed = %v, want 9m", got)
	}
}

// TestInvalidEnvIgnored: out-of-range env is ignored in seed (keeps the default).
func TestInvalidEnvIgnored(t *testing.T) {
	s := setup(t)
	t.Setenv("SEEDGRID_ADMISSION_DEAD_GRACE", "999h") // above the maximum
	if err := s.Load(context.Background()); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := s.Duration("admission.dead_grace"); got.String() != "5m0s" {
		t.Errorf("dead_grace = %v, want default 5m (invalid env ignored)", got)
	}
}

// TestDatabaseWinsEnv: value saved in the DB is NOT overwritten by the env seed
// (precedence ADR-023: the DB rules; env only seeds when absent).
func TestDatabaseWinsEnv(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	if err := s.Set(ctx, "admission.dead_grace", "6m", "operator"); err != nil {
		t.Fatalf("set: %v", err)
	}
	t.Setenv("SEEDGRID_ADMISSION_DEAD_GRACE", "9m")
	if err := s.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := s.Duration("admission.dead_grace"); got.String() != "6m0s" {
		t.Errorf("dead_grace = %v, want 6m (db beats env)", got)
	}
}

// TestSetHotReload: Set reflects immediately in the getter (no reload/redeploy).
func TestSetHotReload(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	if err := s.Load(ctx); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Set(ctx, "admission.interval", "45s", "operator"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := s.Duration("admission.interval"); got.String() != "45s" {
		t.Errorf("interval = %v, want 45s after Set", got)
	}
}

// TestSetValidation: out-of-range value is rejected and nothing is saved.
func TestSetValidation(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	if err := s.Set(ctx, "alerting.disk_saturated_pct", "150", "operator"); err == nil {
		t.Fatal("expected validation error for 150%")
	}
	if got := s.Float("alerting.disk_saturated_pct"); got != 90 {
		t.Errorf("disk_pct = %v, want default 90 (invalid Set does not persist)", got)
	}
}

// TestReset: after Set, Reset reverts to catalog default.
func TestReset(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	if err := s.Set(ctx, "admission.slow_grace", "15m", "operator"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.Reset(ctx, "admission.slow_grace", "operator"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got := s.Duration("admission.slow_grace"); got.String() != "10m0s" {
		t.Errorf("slow_grace = %v, want default 10m after reset", got)
	}
	// Confirms "default" origin in Effective.
	for _, e := range s.Effective() {
		if e.Key == "admission.slow_grace" && e.Source != "default" {
			t.Errorf("source = %q, want default after reset", e.Source)
		}
	}
}
