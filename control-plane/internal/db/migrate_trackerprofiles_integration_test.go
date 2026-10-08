package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/rbferreira/seedgrid/control-plane/migrations"
)

// dsnForDatabase returns a DSN pointing to another database on the same server.
func dsnForDatabase(t *testing.T, dsn, database string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + database
	return u.String()
}

// TestMigration00043_TableWithoutSeed applies the migrations in two stages — up to 00041, with
// trackers already registered, and then the rest — on a DISPOSABLE DATABASE, to prove the real
// upgrade path: the profiles table starts empty (no tracker is seeded; a site profile
// comes in via import, ADR-037), with one live profile per tracker, and down leaves no leftovers.
//
// A dedicated database, not a schema, because goose queries the versions table via
// search_path: in a new schema it would see the one from `public` and think it's already at 43.
func TestMigration00043_TableWithoutSeed(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	admin, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()

	database := fmt.Sprintf("mig43_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+database); err != nil {
		t.Skipf("no permission to create the disposable database: %v", err)
	}
	defer func() {
		_, _ = admin.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '"+database+"'")
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+database)
	}()

	sqlDB, err := sql.Open("pgx", dsnForDatabase(t, dsn, database))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}

	// State BEFORE: the operator's cluster at 00041, with a registered UNIT3D tracker.
	if err := goose.UpTo(sqlDB, ".", 41); err != nil {
		t.Fatalf("migrations up to 00041: %v", err)
	}
	var userID, trackerID string
	if err := sqlDB.QueryRow(`INSERT INTO users (username, password_hash) VALUES ('mig43', 'x') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := sqlDB.QueryRow(`
		INSERT INTO trackers (user_id, name, announce, host, source, api_url)
		VALUES ($1, 'Exemplo', 'https://tracker.example/announce/passkey', 'tracker.example', 'EXEMPLO', 'https://tracker.example')
		RETURNING id::text`, userID).Scan(&trackerID); err != nil {
		t.Fatalf("insert tracker: %v", err)
	}

	// Upgrade.
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatalf("migrations from 00042: %v", err)
	}
	var n int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM tracker_profiles`).Scan(&n); err != nil {
		t.Fatalf("count profiles: %v", err)
	}
	if n != 0 {
		t.Errorf("00043 seeded %d profile(s); the table must start empty", n)
	}

	// The partial index: one live profile per tracker.
	if _, err := sqlDB.Exec(`INSERT INTO tracker_profiles (tracker_id) VALUES ($1)`, trackerID); err != nil {
		t.Fatalf("first tracker profile: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO tracker_profiles (tracker_id) VALUES ($1)`, trackerID); err == nil {
		t.Error("database accepted a second live profile for the same tracker")
	}

	// Down removes the table without leaving leftovers (00043 only creates the table).
	if err := goose.DownTo(sqlDB, ".", 42); err != nil {
		t.Fatalf("down da 00043: %v", err)
	}
	var exists bool
	if err := sqlDB.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'tracker_profiles')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("tracker_profiles still exists after down")
	}
}
