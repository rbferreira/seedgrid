package audit_test

import (
	"context"
	"os"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/audit"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
)

func setup(t *testing.T) *audit.Recorder {
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
	return audit.NewRecorder(pool, nil)
}

// TestRecordAndList records two actions (an HTTP mutation and a login) and confirms that
// they return in the listing (most recent first), with actor filter.
func TestRecordAndList(t *testing.T) {
	rec := setup(t)
	ctx := context.Background()

	rec.RecordAction(ctx, "alice", "POST torrents/abc/pause", "", nil, "POST", "/api/v1/torrents/abc/pause", 200, "10.0.0.9")
	rec.RecordAction(ctx, "bob", "login", "", nil, "POST", "/api/v1/auth/login", 200, "10.0.0.10")

	all, err := rec.List(ctx, "", 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected >= 2 entries, got %d", len(all))
	}

	// Actor filter returns only those by alice.
	mine, err := rec.List(ctx, "alice", 50)
	if err != nil {
		t.Fatalf("list(alice): %v", err)
	}
	if len(mine) == 0 {
		t.Fatal("actor filter returned nothing for alice")
	}
	for _, e := range mine {
		if e.Actor != "alice" {
			t.Errorf("actor filter leaked %q", e.Actor)
		}
	}
	// Structured fields preserved.
	got := mine[0]
	if got.Method == nil || *got.Method != "POST" {
		t.Errorf("method = %v, want POST", got.Method)
	}
	if got.StatusCode == nil || *got.StatusCode != 200 {
		t.Errorf("status = %v, want 200", got.StatusCode)
	}
	if got.SourceIP == nil || *got.SourceIP != "10.0.0.9" {
		t.Errorf("source_ip = %v, want 10.0.0.9", got.SourceIP)
	}
}

// TestRecordActionDetachedContext ensures that recording happens even when the
// caller's context has already been canceled (the audit uses a detached context — ADR-025).
func TestRecordActionDetachedContext(t *testing.T) {
	rec := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled, like a client that disconnected

	rec.RecordAction(ctx, "carol", "DELETE nodes/x", "", nil, "DELETE", "/api/v1/nodes/x", 200, "10.0.0.11")

	entries, err := rec.List(context.Background(), "carol", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) == 0 {
		t.Error("write should have occurred despite canceled context (detached context)")
	}
}
