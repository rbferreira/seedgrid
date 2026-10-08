package trackerprofile_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
)

// env groups what is needed for database tests: pool, both services, and a dedicated
// operator (the trackers FK requires a user).
type env struct {
	pool     *pgxpool.Pool
	profiles *trackerprofile.Service
	trackers *tracker.Service
	userID   string
}

func setup(t *testing.T) env {
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
	username := fmt.Sprintf("profile-test-%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id::text`,
		username).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return env{pool: pool, profiles: trackerprofile.NewService(pool), trackers: tracker.NewService(pool), userID: userID}
}

func (e env) newTracker(t *testing.T, name string) string {
	t.Helper()
	tr, err := e.trackers.Create(context.Background(), e.userID, tracker.Input{
		Name:     name,
		Announce: "https://tracker.example.org/announce/PASSKEY-SECRET-123",
		Source:   "TEST",
		APIURL:   "https://tracker.example.org",
		APIToken: "API-TOKEN-SECRET-456",
	})
	if err != nil {
		t.Fatalf("create tracker: %v", err)
	}
	return tr.ID
}

func ids(terms []trackerprofile.Term) []int {
	out := make([]int, 0, len(terms))
	for _, t := range terms {
		out = append(out, t.ID)
	}
	return out
}

// A tracker without a profile row is no error: it reads as an empty profile, with all axes.
// And ownership is verified — an external tracker's profile does not exist for the requester.
func TestGet_NoRowMeansEmptyProfileAndOwnershipChecked(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "No Profile")

	p, err := e.profiles.Get(ctx, e.userID, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Family != trackerprofile.FamilyUNIT3D || p.SchemaVersion != trackerprofile.SchemaVersion {
		t.Errorf("empty profile = %+v, want family unit3d and schema_version %d", p, trackerprofile.SchemaVersion)
	}
	for _, axis := range trackerprofile.Axes {
		if p.Taxonomy[axis] == nil || len(p.Taxonomy[axis]) != 0 {
			t.Errorf("axis %s = %#v, want non-nil empty list", axis, p.Taxonomy[axis])
		}
	}

	for name, args := range map[string][2]string{
		"other operator": {"00000000-0000-0000-0000-000000000001", id},
		"nonexistent id": {e.userID, "00000000-0000-0000-0000-000000000002"},
		"malformed id":   {e.userID, "'; DROP TABLE trackers;--"},
		"empty id":       {e.userID, ""},
	} {
		if _, err := e.profiles.Get(ctx, args[0], args[1]); !errors.Is(err, trackerprofile.ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestEdit_CreatesLineAndSavesManualOrigin(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Edited")

	res, err := e.profiles.Edit(ctx, e.userID, id, map[string][]trackerprofile.TermInput{
		trackerprofile.AxisCategory: {{ID: 2, Name: "Series", Kind: trackerprofile.KindTV}, {ID: 1, Name: "Filmes", Kind: trackerprofile.KindMovie}},
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if res.TrackerName != "Edited" {
		t.Errorf("TrackerName = %q, want Edited (the audit needs to say which tracker)", res.TrackerName)
	}

	p, err := e.profiles.Get(ctx, e.userID, id)
	if err != nil {
		t.Fatal(err)
	}
	cat := p.Taxonomy[trackerprofile.AxisCategory]
	if got := ids(cat); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("ids = %v, want [1 2] (sorted by id)", got)
	}
	for _, tm := range cat {
		if tm.Origin != trackerprofile.OriginManual {
			t.Errorf("%+v: operator-entered term should be manual", tm)
		}
	}
	if len(p.Taxonomy[trackerprofile.AxisType]) != 0 {
		t.Error("type axis was not sent and should remain empty")
	}

	all, err := e.profiles.ForTrackers(ctx, e.userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := all[id]; !ok {
		t.Error("ForTrackers did not return the newly created profile")
	}
}

// The issue cycle against the database: applying fills what's missing, manual tweaks survive
// a second application and running it again changes nothing.
func TestApply_CompletesAndDoesNotOverwriteManual(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Incremental")

	if _, err := e.profiles.Edit(ctx, e.userID, id, map[string][]trackerprofile.TermInput{
		trackerprofile.AxisCategory: {{ID: 2, Name: "Seriados", Kind: trackerprofile.KindTV}},
	}); err != nil {
		t.Fatal(err)
	}

	items := []trackerprofile.ApplyItem{
		{Axis: trackerprofile.AxisCategory, ID: 1, Name: "Filmes", Origin: trackerprofile.OriginDiscovered},
		{Axis: trackerprofile.AxisCategory, ID: 2, Name: "Series", Origin: trackerprofile.OriginDiscovered},
		{Axis: trackerprofile.AxisType, ID: 50, Name: "PC", Origin: trackerprofile.OriginDiscovered},
	}
	res, err := e.profiles.Apply(ctx, e.userID, id, items)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Applied) != 2 || len(res.Skipped) != 1 || res.Skipped[0].Reason != trackerprofile.ReasonProtected {
		t.Errorf("applied=%+v skipped=%+v, want 2 applied and id 2 protected", res.Applied, res.Skipped)
	}

	p, _ := e.profiles.Get(ctx, e.userID, id)
	byID := map[int]trackerprofile.Term{}
	for _, tm := range p.Taxonomy[trackerprofile.AxisCategory] {
		byID[tm.ID] = tm
	}
	if byID[2].Name != "Seriados" || byID[2].Origin != trackerprofile.OriginManual {
		t.Errorf("manual adjustment did not survive: %+v", byID[2])
	}
	if byID[1].Origin != trackerprofile.OriginDiscovered {
		t.Errorf("Filmes should be discovered: %+v", byID[1])
	}

	// Running it again: nothing applied, and the row is not even rewritten.
	var before time.Time
	if err := e.pool.QueryRow(ctx, `SELECT updated_at FROM tracker_profiles WHERE tracker_id = $1`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	again, err := e.profiles.Apply(ctx, e.userID, id, items)
	if err != nil || len(again.Applied) != 0 {
		t.Fatalf("2nd application: applied=%+v err=%v, want none applied", again.Applied, err)
	}
	var after time.Time
	if err := e.pool.QueryRow(ctx, `SELECT updated_at FROM tracker_profiles WHERE tracker_id = $1`, id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Error("application with no change rewrote the row (updated_at changed)")
	}
}

// Read-modify-write without a lock would lose concurrent applications (each reads the
// old state and overwrites the other). The FOR UPDATE lock serializes them: all get in.
func TestApply_ConcurrentDoesNotLoseWrites(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Concurrent")

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := e.profiles.Apply(ctx, e.userID, id, []trackerprofile.ApplyItem{
				{Axis: trackerprofile.AxisType, ID: i, Name: fmt.Sprintf("Type %d", i), Origin: trackerprofile.OriginDiscovered},
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Apply: %v", err)
		}
	}
	p, _ := e.profiles.Get(ctx, e.userID, id)
	if got := ids(p.Taxonomy[trackerprofile.AxisType]); len(got) != n {
		t.Errorf("ids = %v, want %d terms (lost concurrent writes)", got, n)
	}
}

// The profile follows the tracker, and a removed tracker does not leak the profile either via bulk
// reads or individual ones.
func TestTrackerRemovedAndProfileForgotten(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Ephemeral")
	if _, err := e.profiles.Edit(ctx, e.userID, id, map[string][]trackerprofile.TermInput{
		trackerprofile.AxisType: {{ID: 1, Name: "x"}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.trackers.Delete(ctx, e.userID, id); err != nil {
		t.Fatal(err)
	}
	// Even WITHOUT Forget, the profile of a removed tracker is inert.
	if _, err := e.profiles.Get(ctx, e.userID, id); !errors.Is(err, trackerprofile.ErrNotFound) {
		t.Errorf("Get of removed tracker: %v, want ErrNotFound", err)
	}
	all, _ := e.profiles.ForTrackers(ctx, e.userID)
	if _, ok := all[id]; ok {
		t.Error("ForTrackers returned the profile of a removed tracker")
	}
	if _, err := e.profiles.Edit(ctx, e.userID, id, map[string][]trackerprofile.TermInput{trackerprofile.AxisType: {}}); !errors.Is(err, trackerprofile.ErrNotFound) {
		t.Errorf("Edit of removed tracker: %v, want ErrNotFound", err)
	}

	if err := e.profiles.Forget(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := e.profiles.Forget(ctx, id); err != nil {
		t.Errorf("Forget is not idempotent: %v", err)
	}
	var live int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM tracker_profiles WHERE tracker_id = $1 AND deleted_at IS NULL`, id).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Errorf("%d live row(s) for removed tracker profile", live)
	}
}

// Layer 3 out BY DESIGN (ADR-037, and what #197 will require from the export): the profile
// table has no column capable of storing credentials — and what the service returns, end to
// end, does not contain the announce, the passkey, or the token of a tracker that has them.
func TestProfileDoesNotLoadCredential(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "With Secrets")
	if _, err := e.profiles.Edit(ctx, e.userID, id, map[string][]trackerprofile.TermInput{
		trackerprofile.AxisCategory: {{ID: 1, Name: "Filmes", Kind: trackerprofile.KindMovie}},
		trackerprofile.AxisType:     {{ID: 3, Name: "Encode"}},
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := e.pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_name = 'tracker_profiles' AND table_schema = current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
		for _, banned := range []string{"token", "announce", "passkey", "secret", "api_url", "source", "anon", "modq"} {
			if strings.Contains(c, banned) {
				t.Errorf("tracker_profiles has column %q — operator credential/config (layer 3) cannot reside in the profile", c)
			}
		}
	}
	if len(cols) == 0 {
		t.Fatal("tracker_profiles without columns? catalog query did not find the table")
	}

	p, err := e.profiles.Get(ctx, e.userID, id)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := e.profiles.ForTrackers(ctx, e.userID)
	for name, v := range map[string]any{"Get": p, "ForTrackers": all} {
		raw, _ := json.Marshal(v)
		for _, secret := range []string{"PASSKEY-SECRET-123", "API-TOKEN-SECRET-456", "announce", "tracker.example.org"} {
			if strings.Contains(string(raw), secret) {
				t.Errorf("%s leaks %q: %s", name, secret, raw)
			}
		}
	}
}

// The guarantees that remain in the database, in case someone writes outside the service.
func TestDatabaseConstraints(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Restrictions")
	if _, err := e.profiles.Edit(ctx, e.userID, id, map[string][]trackerprofile.TermInput{trackerprofile.AxisType: {}}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.pool.Exec(ctx, `UPDATE tracker_profiles SET taxonomy = '[]'::jsonb WHERE tracker_id = $1`, id); err == nil {
		t.Error("non-object taxonomy was accepted by the database")
	}
	if _, err := e.pool.Exec(ctx, `UPDATE tracker_profiles SET schema_version = 0 WHERE tracker_id = $1`, id); err == nil {
		t.Error("schema_version 0 was accepted by the database")
	}
	if _, err := e.pool.Exec(ctx, `UPDATE tracker_profiles SET family = '' WHERE tracker_id = $1`, id); err == nil {
		t.Error("empty family was accepted by the database")
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO tracker_profiles (tracker_id) VALUES ($1)`, id); err == nil {
		t.Error("a second live profile for the same tracker was accepted")
	}
}
