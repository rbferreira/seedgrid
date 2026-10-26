package trackerprofile_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
)

// Export and import with a real database (#197): the service only reads tracker name and host, writes
// under the profile row lock and never creates the row when previewing.

func (e env) fill(t *testing.T, trackerID string) {
	t.Helper()
	if _, err := e.profiles.Edit(context.Background(), trackerID, map[string][]trackerprofile.TermInput{
		trackerprofile.AxisCategory:   {{ID: 1, Name: "Filmes", Kind: trackerprofile.KindMovie}, {ID: 2, Name: "Séries", Kind: trackerprofile.KindTV}},
		trackerprofile.AxisType:       {{ID: 3, Name: "Encode"}},
		trackerprofile.AxisResolution: {{ID: 3, Name: "1080p"}},
	}); err != nil {
		t.Fatalf("fill profile: %v", err)
	}
}

// Layer 3 out on export, end-to-end in the service: a tracker with announce+passkey,
// api_url, api_token and source has its profile exported, and none of these values leak.
func TestExport_DoesNotLoadCredential(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "With Secrets")
	e.fill(t, id)

	doc, err := e.profiles.Export(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"PASSKEY-SECRET-123", "API-TOKEN-SECRET-456", "announce", "/announce/",
		"https://tracker.example.org", "TEST", "api_url", "api_token", id,
	} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("export leaks %q:\n%s", secret, raw)
		}
	}
	if doc.Site.Name != e.name("With Secrets") || doc.Site.Host != "tracker.example.org" {
		t.Errorf("site = %+v", doc.Site)
	}
	if _, err := trackerprofile.ParseDocument(raw); err != nil {
		t.Errorf("export does not pass its own import: %v", err)
	}
}

func TestExport_EmptyProfileAndNotFound(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Empty")

	doc, err := e.profiles.Export(ctx, id)
	if err != nil {
		t.Fatalf("tracker export without profile row: %v", err)
	}
	for _, axis := range trackerprofile.Axes {
		if doc.Taxonomy[axis] == nil || len(doc.Taxonomy[axis]) != 0 {
			t.Errorf("axis %s of empty export = %v, want empty list", axis, doc.Taxonomy[axis])
		}
	}
	// Exporting does not create the profile row (it is a read).
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM tracker_profiles WHERE tracker_id = $1`, id).Scan(&n); err != nil || n != 0 {
		t.Errorf("export created profile row (n=%d, err=%v)", n, err)
	}

	// A tracker that does not exist, and the format of the id.
	for _, bad := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid", "' OR 1=1 --"} {
		if _, err := e.profiles.Export(ctx, bad); !errors.Is(err, trackerprofile.ErrNotFound) {
			t.Errorf("export of %q: %v, want ErrNotFound", bad, err)
		}
	}
}

func parseExport(t *testing.T, e env, trackerID string) trackerprofile.Document {
	t.Helper()
	doc, err := e.profiles.Export(context.Background(), trackerID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := doc.Marshal()
	parsed, err := trackerprofile.ParseDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// The whole cycle in the service: exports from one, previews (without writing or creating the row),
// imports into the other and checks what was saved — including the source.
func TestImport_PreviewDoesNotSaveAndImportSaves(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	from := e.newTracker(t, "Origin")
	e.fill(t, from)
	to := e.newTracker(t, "Destination")
	doc := parseExport(t, e, from)

	plan, err := e.profiles.PreviewImport(ctx, to, doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Counts.New != 4 {
		t.Errorf("preview: %+v, want 4 new", plan.Counts)
	}
	var rows int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM tracker_profiles WHERE tracker_id = $1`, to).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("preview created the profile row (rows=%d, err=%v)", rows, err)
	}
	if p, _ := e.profiles.Get(ctx, to); len(p.Taxonomy[trackerprofile.AxisCategory]) != 0 {
		t.Error("preview saved terms")
	}

	res, err := e.profiles.Import(ctx, to, doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.TrackerName != e.name("Destination") || res.Plan.Counts.New != 4 {
		t.Errorf("res = %s %+v", res.TrackerName, res.Plan.Counts)
	}
	got, _ := e.profiles.Get(ctx, to)
	for axis, terms := range got.Taxonomy {
		for _, tm := range terms {
			if tm.Origin != trackerprofile.OriginImported {
				t.Errorf("%s %d: origin %q, want imported", axis, tm.ID, tm.Origin)
			}
		}
	}
	if len(got.Taxonomy[trackerprofile.AxisCategory]) != 2 || got.Taxonomy[trackerprofile.AxisCategory][1].Kind != trackerprofile.KindTV {
		t.Errorf("imported categories: %+v", got.Taxonomy[trackerprofile.AxisCategory])
	}

	// A 2nd import changes nothing and does not overwrite (updated_at stays).
	var before, after string
	_ = e.pool.QueryRow(ctx, `SELECT updated_at::text FROM tracker_profiles WHERE tracker_id = $1`, to).Scan(&before)
	res2, err := e.profiles.Import(ctx, to, doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = e.pool.QueryRow(ctx, `SELECT updated_at::text FROM tracker_profiles WHERE tracker_id = $1`, to).Scan(&after)
	if res2.Plan.Changes() || before != after {
		t.Errorf("2nd import: changes=%v, updated_at %s -> %s", res2.Plan.Changes(), before, after)
	}
}

// The manual is not overwritten without a request — and the rule runs on the locked state: an edit
// that happens BETWEEN the preview and the import is respected.
func TestImport_RespectsManualOnLockedState(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	to := e.newTracker(t, "Destination")
	doc := parseExport(t, e, func() string {
		from := e.newTracker(t, "Origin")
		e.fill(t, from)
		return from
	}())

	// Preview: category 2 would be a new addition.
	plan, err := e.profiles.PreviewImport(ctx, to, doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if it := plan.Items[trackerprofile.AxisCategory][1]; it.Status != trackerprofile.StatusNew {
		t.Fatalf("category 2 in preview = %+v, want new", it)
	}
	// Before importing, the operator manually creates category 2 with another name.
	if _, err := e.profiles.Edit(ctx, to, map[string][]trackerprofile.TermInput{
		trackerprofile.AxisCategory: {{ID: 2, Name: "Seriados"}},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := e.profiles.Import(ctx, to, doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if it := res.Plan.Items[trackerprofile.AxisCategory][1]; it.Status != trackerprofile.StatusProtected {
		t.Errorf("category 2 in import = %+v, want protected (state changed since preview)", it)
	}
	got, _ := e.profiles.Get(ctx, to)
	for _, tm := range got.Taxonomy[trackerprofile.AxisCategory] {
		if tm.ID == 2 && (tm.Name != "Seriados" || tm.Origin != trackerprofile.OriginManual) {
			t.Errorf("manual adjustment was overwritten: %+v", tm)
		}
	}

	// When requesting, overwrites.
	res, err = e.profiles.Import(ctx, to, doc, map[trackerprofile.OverwriteKey]bool{{Axis: trackerprofile.AxisCategory, ID: 2}: true})
	if err != nil {
		t.Fatal(err)
	}
	got = res.Profile
	for _, tm := range got.Taxonomy[trackerprofile.AxisCategory] {
		if tm.ID == 2 && (tm.Name != "Séries" || tm.Origin != trackerprofile.OriginImported) {
			t.Errorf("after the request: %+v", tm)
		}
	}
}

// Concurrent imports of different files do not lose writes: the plan is calculated
// over the locked row (FOR UPDATE), so each one starts from the result of the previous one.
func TestImport_ConcurrentDoesNotLoseWrites(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Concurrent")

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		raw := fmt.Sprintf(`{"format":%q,"site":{"name":"File %d"},"family":"unit3d","schema_version":1,`+
			`"taxonomy":{"type":[{"id":%d,"name":"Type %d"}]}}`, trackerprofile.FormatID, i, i+1, i+1)
		doc, err := trackerprofile.ParseDocument([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.profiles.Import(ctx, id, doc, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent import: %v", err)
		}
	}
	p, _ := e.profiles.Get(ctx, id)
	if got := len(p.Taxonomy[trackerprofile.AxisType]); got != n {
		t.Errorf("types after %d concurrent imports = %d (%v), want %d — some write was lost", n, got, ids(p.Taxonomy[trackerprofile.AxisType]), n)
	}
}

// A file of a different family than the destination tracker is rejected (409 in HTTP).
func TestImport_FamilyIncompatibleWithTracker(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Other family")
	e.fill(t, id)
	// The database only requires a non-empty family; Go is what restricts it (new family is new code).
	if _, err := e.pool.Exec(ctx, `UPDATE tracker_profiles SET family = 'gazelle' WHERE tracker_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"format": trackerprofile.FormatID, "site": map[string]any{"name": "x"}, "family": "unit3d", "schema_version": 1,
		"taxonomy": map[string]any{"type": []any{map[string]any{"id": 9, "name": "New"}}},
	})
	doc, err := trackerprofile.ParseDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	var ve *trackerprofile.ValidationError
	if _, err := e.profiles.PreviewImport(ctx, id, doc, nil); !errors.As(err, &ve) || ve.ErrorCode() != trackerprofile.CodeIncompatibleFamily {
		t.Errorf("preview: %v, want incompatible_family", err)
	}
	if _, err := e.profiles.Import(ctx, id, doc, nil); !errors.As(err, &ve) || ve.ErrorCode() != trackerprofile.CodeIncompatibleFamily {
		t.Errorf("import: %v, want incompatible_family", err)
	}
	p, _ := e.profiles.Get(ctx, id)
	for _, tm := range p.Taxonomy[trackerprofile.AxisType] {
		if tm.ID == 9 {
			t.Error("incompatible family import saved anyway")
		}
	}
}

// Import on a tracker that does not exist, or with a malformed id, is 404, not a cast error.
func TestImport_NotFoundAndMalformedId(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	raw, _ := json.Marshal(map[string]any{
		"format": trackerprofile.FormatID, "site": map[string]any{"name": "x"}, "family": "unit3d", "schema_version": 1,
		"taxonomy": map[string]any{"type": []any{map[string]any{"id": 9, "name": "New"}}},
	})
	doc, _ := trackerprofile.ParseDocument(raw)
	for _, bad := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		if _, err := e.profiles.Import(ctx, bad, doc, nil); !errors.Is(err, trackerprofile.ErrNotFound) {
			t.Errorf("import on %q: %v", bad, err)
		}
		if _, err := e.profiles.PreviewImport(ctx, bad, doc, nil); !errors.Is(err, trackerprofile.ErrNotFound) {
			t.Errorf("preview on %q: %v", bad, err)
		}
	}
}
