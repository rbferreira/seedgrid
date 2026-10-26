package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/rbferreira/seedgrid/control-plane/migrations"
)

// TestMigration00050_ClusterTrackers upgrades a cluster at 00049 where two operators
// registered trackers with the same name, each with their own default (ADR-047). After
// 00050: names are unique across the cluster, one default is left, the owner column is a
// record (created_by) and the Publisher no longer holds trackers:write. Down restores the
// per-operator schema and the permission. Disposable database, as in the 00043 test.
func TestMigration00050_ClusterTrackers(t *testing.T) {
	dsn := testDSN(t)
	ctx := context.Background()
	admin, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer admin.Close()

	database := fmt.Sprintf("mig50_%d", time.Now().UnixNano())
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

	// State BEFORE: two operators, both with a "Site" tracker set as their default; the
	// second also has a removed "Site", which does not count as a duplicate.
	if err := goose.UpTo(sqlDB, ".", 49); err != nil {
		t.Fatalf("migrations up to 00049: %v", err)
	}
	user := func(name string) string {
		t.Helper()
		var id string
		if err := sqlDB.QueryRow(`INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id::text`, name).Scan(&id); err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
		return id
	}
	tracker := func(userID, name string, isDefault bool, created string, deleted bool) string {
		t.Helper()
		var id string
		if err := sqlDB.QueryRow(`
			INSERT INTO trackers (user_id, name, announce, host, is_default, created_at, deleted_at)
			VALUES ($1, $2, 'https://site.example/announce/k', 'site.example', $3, $4::timestamptz,
			        CASE WHEN $5 THEN now() END)
			RETURNING id::text`, userID, name, isDefault, created, deleted).Scan(&id); err != nil {
			t.Fatalf("insert tracker %s: %v", name, err)
		}
		return id
	}
	ana, bia := user("ana"), user("bia")
	anaSite := tracker(ana, "Site", true, "2026-01-01", false)
	biaSite := tracker(bia, "Site", true, "2026-02-01", false)
	biaOther := tracker(bia, "Other", false, "2026-02-02", false)
	tracker(bia, "Site", false, "2026-01-15", true)

	if err := goose.UpTo(sqlDB, ".", 50); err != nil {
		t.Fatalf("migration 00050: %v", err)
	}

	type row struct {
		name      string
		isDefault bool
		createdBy string
	}
	read := func(id string) row {
		t.Helper()
		var r row
		if err := sqlDB.QueryRow(`SELECT name, is_default, created_by::text FROM trackers WHERE id = $1`, id).
			Scan(&r.name, &r.isDefault, &r.createdBy); err != nil {
			t.Fatalf("read tracker: %v", err)
		}
		return r
	}
	if got := read(anaSite); got != (row{"Site", true, ana}) {
		t.Errorf("oldest Site = %+v, want its name and the default kept", got)
	}
	if got := read(biaSite); got != (row{"Site (bia)", false, bia}) {
		t.Errorf("newer Site = %+v, want renamed with the owner and not default", got)
	}
	if got := read(biaOther); got != (row{"Other", false, bia}) {
		t.Errorf("Other = %+v, want untouched", got)
	}

	// The cluster-wide indexes hold: no second "Site", no second default.
	if _, err := sqlDB.Exec(`INSERT INTO trackers (created_by, name, announce, host) VALUES ($1, 'Site', 'udp://x.example/a', 'x.example')`, ana); err == nil {
		t.Error("database accepted a second active tracker named Site")
	}
	if _, err := sqlDB.Exec(`UPDATE trackers SET is_default = true WHERE id = $1`, biaOther); err == nil {
		t.Error("database accepted a second default tracker")
	}

	publisherHas := func(action string) bool {
		t.Helper()
		var ok bool
		if err := sqlDB.QueryRow(`
			SELECT EXISTS (SELECT 1 FROM role_permissions rp
			                 JOIN roles r ON r.id = rp.role_id
			                 JOIN permissions p ON p.id = rp.permission_id
			                WHERE r.name = 'Publisher' AND p.resource = 'trackers' AND p.action = $1)`,
			action).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if publisherHas("write") {
		t.Error("Publisher still holds trackers:write after 00050")
	}
	if !publisherHas("read") || !publisherHas("seed") {
		t.Error("Publisher lost trackers:read or trackers:seed")
	}

	// Down: the per-operator schema and the permission come back.
	if err := goose.DownTo(sqlDB, ".", 49); err != nil {
		t.Fatalf("down of 00050: %v", err)
	}
	if !publisherHas("write") {
		t.Error("down did not give trackers:write back to the Publisher")
	}
	var owner string
	if err := sqlDB.QueryRow(`SELECT user_id::text FROM trackers WHERE id = $1`, biaSite).Scan(&owner); err != nil || owner != bia {
		t.Errorf("after down, user_id = %q (%v), want the owner back", owner, err)
	}
}
