package db

import (
	"context"
	"os"
	"testing"
)

// testDSN returns the test DSN or skips the test when not configured.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	return dsn
}

func TestMigrate_Idempotent(t *testing.T) {
	dsn := testDSN(t)

	if err := Migrate(dsn); err != nil {
		t.Fatalf("first application of migrations: %v", err)
	}
	// Reapplying must not fail (nothing pending).
	if err := Migrate(dsn); err != nil {
		t.Fatalf("reapplication of migrations: %v", err)
	}

	pool, err := Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// Central tables must exist.
	for _, table := range []string{"clusters", "nodes", "users", "sessions", "settings"} {
		var exists bool
		err := pool.QueryRow(context.Background(),
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %q does not exist after migrations", table)
		}
	}
}
