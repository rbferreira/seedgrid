package activity_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/activity"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
)

func setup(t *testing.T) *activity.Recorder {
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
	return activity.NewRecorder(pool)
}

// TestRecordAndList records an activity (without torrent/node — null FKs) and confirms
// that it returns in the listing with the kind, message and detail (JSONB) preserved.
func TestRecordAndList(t *testing.T) {
	rec := setup(t)
	ctx := context.Background()

	if err := rec.Record(ctx, activity.Entry{
		Kind:    activity.KindAdmissionHold,
		Message: "Download on hold — admission cap reached",
		Detail:  map[string]any{"reason": "surplus"},
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	acts, err := rec.List(ctx, "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(acts) == 0 {
		t.Fatal("list empty after record")
	}
	got := acts[0] // most recent first
	if got.Kind != activity.KindAdmissionHold {
		t.Errorf("kind = %q, want %q", got.Kind, activity.KindAdmissionHold)
	}
	if got.Message == "" {
		t.Error("empty message")
	}
	var d map[string]any
	if err := json.Unmarshal(got.Detail, &d); err != nil {
		t.Fatalf("detail is not valid JSON: %v", err)
	}
	if d["reason"] != "surplus" {
		t.Errorf("detail.reason = %v, want surplus", d["reason"])
	}
}
