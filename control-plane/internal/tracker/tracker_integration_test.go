package tracker_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
)

// setup connects to the test database (skipping if not configured), applies
// migrations and creates a unique user to satisfy the trackers FK constraint.
func setup(t *testing.T) (*tracker.Service, string) {
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

	var userID string
	username := fmt.Sprintf("tracker-test-%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id::text`,
		username).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return tracker.NewService(pool), userID
}

// TestCreateListForPublishDelete covers the basic cycle and the secret: listing
// exposes name + host but NOT the announce; ForPublish returns the full URL.
func TestCreateListForPublishDelete(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	const announce = "https://tracker.example.org:443/announce?passkey=secret123"
	created, err := svc.Create(ctx, userID, tracker.Input{Name: "My Private", Announce: announce})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Host != "tracker.example.org:443" {
		t.Errorf("host = %q, want tracker.example.org:443", created.Host)
	}

	list, err := svc.List(ctx, userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "My Private" {
		t.Fatalf("List = %+v, want 1 tracker 'My Private'", list)
	}

	sec, err := svc.ForPublish(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("ForPublish: %v", err)
	}
	if sec.Announce != announce {
		t.Errorf("ForPublish announce = %q, want %q", sec.Announce, announce)
	}

	if err := svc.Delete(ctx, userID, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ = svc.List(ctx, userID)
	if len(list) != 0 {
		t.Errorf("after Delete, List = %+v, want empty", list)
	}
	if _, err := svc.ForPublish(ctx, userID, created.ID); !errors.Is(err, tracker.ErrNotFound) {
		t.Errorf("ForPublish after Delete = %v, want ErrNotFound", err)
	}
}

// TestSingleDefault ensures that only one tracker is default per operator.
func TestSingleDefault(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	a, err := svc.Create(ctx, userID, tracker.Input{Name: "A", Announce: "udp://a.example:6969/announce", MakeDefault: true})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	b, err := svc.Create(ctx, userID, tracker.Input{Name: "B", Announce: "https://b.example/announce/key", MakeDefault: true})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	list, _ := svc.List(ctx, userID)
	defaults := map[string]bool{}
	for _, tr := range list {
		defaults[tr.Name] = tr.IsDefault
	}
	if defaults["A"] || !defaults["B"] {
		t.Fatalf("after creating B as default, defaults = %v; want only B", defaults)
	}

	// Revert the default to A.
	if err := svc.SetDefault(ctx, userID, a.ID); err != nil {
		t.Fatalf("SetDefault A: %v", err)
	}
	list, _ = svc.List(ctx, userID)
	for _, tr := range list {
		want := tr.ID == a.ID
		if tr.IsDefault != want {
			t.Errorf("tracker %s default = %v, want %v", tr.Name, tr.IsDefault, want)
		}
	}
	_ = b
}

// TestDuplicateAndInvalid covers duplicate name, malformed announce and invalid api_url.
func TestDuplicateAndInvalid(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, userID, tracker.Input{Name: "Dup", Announce: "https://t.example/announce"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, userID, tracker.Input{Name: "Dup", Announce: "https://t2.example/announce"}); !errors.Is(err, tracker.ErrDuplicateName) {
		t.Errorf("Create duplicate name = %v, want ErrDuplicateName", err)
	}
	if _, err := svc.Create(ctx, userID, tracker.Input{Name: "Bad", Announce: "not-a-url"}); !errors.Is(err, tracker.ErrInvalidAnnounce) {
		t.Errorf("Create invalid announce = %v, want ErrInvalidAnnounce", err)
	}
	if _, err := svc.Create(ctx, userID, tracker.Input{Name: "", Announce: "https://t.example/announce"}); !errors.Is(err, tracker.ErrInvalidAnnounce) {
		t.Errorf("Create empty name = %v, want ErrInvalidAnnounce", err)
	}
	if _, err := svc.Create(ctx, userID, tracker.Input{Name: "NoScheme", Announce: "https://t.example/announce", APIURL: "ftp://x.example"}); !errors.Is(err, tracker.ErrInvalidAPIURL) {
		t.Errorf("Create invalid api_url = %v, want ErrInvalidAPIURL", err)
	}
}

// TestUNIT3DIntegration covers ADR-020: source/api_url exposed, api_token NEVER
// returned (only has_api_token), and ForPublish returns the token for internal use.
func TestUNIT3DIntegration(t *testing.T) {
	svc, userID := setup(t)
	ctx := context.Background()

	const announce = "https://tracker.example/announce/deadbeef"
	const token = "super-secret-api-token"
	created, err := svc.Create(ctx, userID, tracker.Input{
		Name:     "Example",
		Announce: announce,
		Source:   "EXAMPLE",
		APIURL:   "https://tracker.example/", // trailing slash must be normalized
		APIToken: token,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Source != "EXAMPLE" {
		t.Errorf("source = %q, want EXAMPLE", created.Source)
	}
	if created.APIURL != "https://tracker.example" {
		t.Errorf("api_url = %q, want no trailing slash", created.APIURL)
	}
	if !created.HasAPIToken {
		t.Errorf("has_api_token = false, want true")
	}

	// The secret (api_token) never appears in the public view (List).
	list, _ := svc.List(ctx, userID)
	if len(list) != 1 || !list[0].HasAPIToken || list[0].Source != "EXAMPLE" {
		t.Fatalf("List = %+v, want 1 tracker with source+has_api_token", list)
	}

	// ForPublish returns the secrets for internal use (announce + source + api).
	sec, err := svc.ForPublish(ctx, userID, created.ID)
	if err != nil {
		t.Fatalf("ForPublish: %v", err)
	}
	if sec.Announce != announce || sec.Source != "EXAMPLE" || sec.APIURL != "https://tracker.example" || sec.APIToken != token {
		t.Errorf("ForPublish = %+v, want all integration fields", sec)
	}

	// Update without api_token keeps the current token (empty field = keep).
	updated, err := svc.Update(ctx, userID, created.ID, tracker.Input{
		Name:     "Example",
		Announce: announce,
		Source:   "EXAMPLE",
		APIURL:   "https://tracker.example",
		APIToken: "", // keep
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated.HasAPIToken {
		t.Errorf("after Update without token, has_api_token = false, want to keep true")
	}
	sec, _ = svc.ForPublish(ctx, userID, created.ID)
	if sec.APIToken != token {
		t.Errorf("after Update without token, api_token changed = %q, want to keep %q", sec.APIToken, token)
	}

	// Update with new token replaces it.
	if _, err := svc.Update(ctx, userID, created.ID, tracker.Input{
		Name: "Example", Announce: announce, Source: "EXAMPLE",
		APIURL: "https://tracker.example", APIToken: "new-token",
	}); err != nil {
		t.Fatalf("Update with new token: %v", err)
	}
	sec, _ = svc.ForPublish(ctx, userID, created.ID)
	if sec.APIToken != "new-token" {
		t.Errorf("after Update with token, api_token = %q, want new-token", sec.APIToken)
	}

	// Update of non-existent tracker → ErrNotFound.
	if _, err := svc.Update(ctx, userID, "00000000-0000-0000-0000-000000000000", tracker.Input{
		Name: "X", Announce: announce,
	}); !errors.Is(err, tracker.ErrNotFound) {
		t.Errorf("Update nonexistent = %v, want ErrNotFound", err)
	}
}
