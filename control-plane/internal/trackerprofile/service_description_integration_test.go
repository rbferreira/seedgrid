package trackerprofile_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/description"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
)

// The description section (#192) with a real database: saved in the SAME JSONB document, without
// migration, without a new column, and preserved by any write path that doesn't touch it.

func customDesc() description.Config {
	return description.Config{
		MediaBlocks:  []string{description.BlockPoster, description.BlockMediaInfo},
		GameBlocks:   []string{description.BlockTitle, description.BlockAbout},
		MediaInfoTag: description.TagSpoiler,
	}
}

func otherDesc() description.Config {
	return description.Config{
		MediaBlocks:  []string{description.BlockSynopsis, description.BlockTags},
		GameBlocks:   []string{},
		MediaInfoTag: description.TagPre,
	}
}

// rawDoc returns the root keys of the profile's JSONB document, as they are in the database.
func (e env) rawDoc(t *testing.T, trackerID string) map[string]json.RawMessage {
	t.Helper()
	var raw []byte
	if err := e.pool.QueryRow(context.Background(),
		`SELECT taxonomy FROM tracker_profiles WHERE tracker_id = $1 AND deleted_at IS NULL`, trackerID).Scan(&raw); err != nil {
		t.Fatalf("read the profile document: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unreadable document: %v: %s", err, raw)
	}
	return doc
}

func (e env) desc(t *testing.T, trackerID string) *description.Config {
	t.Helper()
	p, err := e.profiles.Get(context.Background(), e.userID, trackerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return p.Description
}

func TestSetDescription_WritesToSameDocumentAndPreservesTaxonomy(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "With Description")
	e.fill(t, id)

	cfg := customDesc()
	res, err := e.profiles.SetDescription(ctx, e.userID, id, &cfg)
	if err != nil {
		t.Fatalf("SetDescription: %v", err)
	}
	if res.TrackerName != "With Description" || res.Profile.Description == nil || !reflect.DeepEqual(*res.Profile.Description, cfg) {
		t.Errorf("result = %+v", res)
	}
	if got := e.desc(t, id); got == nil || !reflect.DeepEqual(*got, cfg) {
		t.Fatalf("Get returned %+v, want %+v", got, cfg)
	}

	// It lives in the SAME document: the reserved key next to the three axes. No new column.
	doc := e.rawDoc(t, id)
	for _, key := range []string{"category", "type", "resolution", "description"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("the document does not have the key %q: %v", key, doc)
		}
	}
	var cols []string
	rows, err := e.pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_name = 'tracker_profiles' AND table_schema = current_schema() ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		cols = append(cols, c)
	}
	rows.Close()
	if want := []string{"id", "tracker_id", "family", "schema_version", "taxonomy", "created_at", "updated_at", "deleted_at"}; !reflect.DeepEqual(cols, want) {
		t.Errorf("tracker_profiles columns = %v, want %v (#192 does not create a column)", cols, want)
	}

	// The taxonomy stayed as it was.
	p, _ := e.profiles.Get(ctx, e.userID, id)
	if len(p.Taxonomy[trackerprofile.AxisCategory]) != 2 || len(p.Taxonomy[trackerprofile.AxisType]) != 1 {
		t.Errorf("the taxonomy changed when saving the section: %+v", p.Taxonomy)
	}

	// Editing the taxonomy manually does NOT delete the section.
	if _, err := e.profiles.Edit(ctx, e.userID, id, map[string][]trackerprofile.TermInput{
		trackerprofile.AxisType: {{ID: 3, Name: "Encode"}, {ID: 4, Name: "WEB-DL"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.desc(t, id); got == nil || !reflect.DeepEqual(*got, cfg) {
		t.Errorf("Edit deleted or changed the section: %+v", got)
	}

	// Applying a discovery doesn't either.
	if _, err := e.profiles.Apply(ctx, e.userID, id, []trackerprofile.ApplyItem{{Axis: trackerprofile.AxisType, ID: 50, Name: "PC", Origin: trackerprofile.OriginDiscovered}}); err != nil {
		t.Fatal(err)
	}
	if got := e.desc(t, id); got == nil || !reflect.DeepEqual(*got, cfg) {
		t.Errorf("Apply deleted or changed the section: %+v", got)
	}
	p, _ = e.profiles.Get(ctx, e.userID, id)
	if len(p.Taxonomy[trackerprofile.AxisType]) != 3 {
		t.Errorf("Apply did not save the term: %+v", p.Taxonomy[trackerprofile.AxisType])
	}

	// Importing a file WITHOUT the section doesn't either.
	src := e.newTracker(t, "Origin without section")
	e.fill(t, src)
	doc2, err := e.profiles.Export(ctx, e.userID, src)
	if err != nil {
		t.Fatal(err)
	}
	if doc2.Description != nil {
		t.Fatalf("the export of a profile without a section included it: %+v", doc2.Description)
	}
	if _, err := e.profiles.Import(ctx, e.userID, id, doc2, nil); err != nil {
		t.Fatal(err)
	}
	if got := e.desc(t, id); got == nil || !reflect.DeepEqual(*got, cfg) {
		t.Errorf("Import without a section deleted or changed your section: %+v", got)
	}
}

func TestSetDescription_NilRestoresDefaultAndRemovesKey(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Restore")
	e.fill(t, id)
	cfg := customDesc()
	if _, err := e.profiles.SetDescription(ctx, e.userID, id, &cfg); err != nil {
		t.Fatal(err)
	}

	res, err := e.profiles.SetDescription(ctx, e.userID, id, nil)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Profile.Description != nil || e.desc(t, id) != nil {
		t.Errorf("the section is still there after restoring: %+v", res.Profile.Description)
	}
	doc := e.rawDoc(t, id)
	if _, ok := doc["description"]; ok {
		t.Errorf("the key is still in the document: %v", doc)
	}
	if _, ok := doc["category"]; !ok {
		t.Error("restoring the description deleted the taxonomy")
	}
	// Restoring what is already in the default is idempotent.
	if _, err := e.profiles.SetDescription(ctx, e.userID, id, nil); err != nil {
		t.Errorf("restore again: %v", err)
	}
}

func TestSetDescription_InvalidWritesNothing(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Invalid")
	cfg := customDesc()
	if _, err := e.profiles.SetDescription(ctx, e.userID, id, &cfg); err != nil {
		t.Fatal(err)
	}
	before := e.rawDoc(t, id)

	for name, bad := range map[string]description.Config{
		"unknown block":  {MediaBlocks: []string{"poster", "x"}},
		"repeated block": {GameBlocks: []string{"title", "title"}},
		"invalid tag":    {MediaInfoTag: "mediainfo][img"},
		"huge list":      {MediaBlocks: make([]string, 100000)},
	} {
		_, err := e.profiles.SetDescription(ctx, e.userID, id, &bad)
		var ve *trackerprofile.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: error = %v, want *ValidationError", name, err)
		}
	}
	if after := e.rawDoc(t, id); !reflect.DeepEqual(before, after) {
		t.Errorf("a rejected write changed the database:\nbefore  %v\nafter %v", before, after)
	}
}

func TestSetDescription_OwnershipAndLineOnDemand(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	cfg := customDesc()

	for name, args := range map[string][2]string{
		"other operator": {"00000000-0000-0000-0000-000000000001", e.newTracker(t, "Other")},
		"nonexistent id": {e.userID, "00000000-0000-0000-0000-000000000002"},
		"malformed id":   {e.userID, "'; DROP TABLE trackers;--"},
	} {
		if _, err := e.profiles.SetDescription(ctx, args[0], args[1], &cfg); !errors.Is(err, trackerprofile.ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}

	// Tracker without a profile row: the first write creates it — with empty taxonomy and the section.
	id := e.newTracker(t, "No row")
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM tracker_profiles WHERE tracker_id = $1`, id).Scan(&n)
	if n != 0 {
		t.Fatalf("the new tracker already has a profile row")
	}
	if _, err := e.profiles.SetDescription(ctx, e.userID, id, &cfg); err != nil {
		t.Fatal(err)
	}
	p, err := e.profiles.Get(ctx, e.userID, id)
	if err != nil || p.Description == nil {
		t.Fatalf("Get = %+v, %v", p, err)
	}
	for _, axis := range trackerprofile.Axes {
		if p.Taxonomy[axis] == nil || len(p.Taxonomy[axis]) != 0 {
			t.Errorf("axis %s = %v, want empty list", axis, p.Taxonomy[axis])
		}
	}
}

// The row that migration 00043 already wrote (just the axes, at the root) is read and accepts the section without
// any prior rewrite.
func TestSetDescription_LineInOldFormat(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Old format")
	e.fill(t, id)
	if _, err := e.pool.Exec(ctx, `UPDATE tracker_profiles SET taxonomy = '{"type":[{"id":3,"name":"Encode","origin":"seed"}]}'::jsonb WHERE tracker_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	p, err := e.profiles.Get(ctx, e.userID, id)
	if err != nil || p.Description != nil || len(p.Taxonomy[trackerprofile.AxisType]) != 1 {
		t.Fatalf("old row read as %+v (%v)", p, err)
	}
	cfg := customDesc()
	if _, err := e.profiles.SetDescription(ctx, e.userID, id, &cfg); err != nil {
		t.Fatal(err)
	}
	p, _ = e.profiles.Get(ctx, e.userID, id)
	if p.Description == nil || p.Taxonomy[trackerprofile.AxisType][0].Origin != trackerprofile.OriginSeed {
		t.Errorf("after saving the section: %+v", p)
	}
}

// An unreadable document in the database is an error (500 in the API), never "empty profile": the next edit
// would overwrite it.
func TestGet_UnreadableSectionInDBIsError(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newTracker(t, "Corrupted")
	e.fill(t, id)
	if _, err := e.pool.Exec(ctx, `UPDATE tracker_profiles SET taxonomy = '{"description":"x"}'::jsonb WHERE tracker_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.profiles.Get(ctx, e.userID, id); err == nil || errors.Is(err, trackerprofile.ErrNotFound) {
		t.Errorf("Get for unreadable document = %v, want an error other than ErrNotFound", err)
	}
	if _, err := e.profiles.SetDescription(ctx, e.userID, id, nil); err == nil {
		t.Error("SetDescription on an unreadable document succeeded (would overwrite it)")
	}
}

// ---- Section export and import, end-to-end in the service ---------------------------------

func TestExportImport_SectionTravelsInFile(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	src := e.newTracker(t, "Origin")
	e.fill(t, src)
	cfg := customDesc()
	if _, err := e.profiles.SetDescription(ctx, e.userID, src, &cfg); err != nil {
		t.Fatal(err)
	}

	doc, err := e.profiles.Export(ctx, e.userID, src)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	raw, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	// The file goes through the ONLY entry point — as if it came from a third party.
	parsed, err := trackerprofile.ParseDocument(raw)
	if err != nil {
		t.Fatalf("export does not pass its own import: %v\n%s", err, raw)
	}
	if parsed.Description == nil || !reflect.DeepEqual(*parsed.Description, cfg) {
		t.Fatalf("section did not round-trip: %+v", parsed.Description)
	}

	// Destination without a section: receives the one from the file, along with the taxonomy, in the same write.
	dst := e.newTracker(t, "Destination")
	res, err := e.profiles.Import(ctx, e.userID, dst, parsed, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Plan.Description == nil || res.Plan.Description.Status != trackerprofile.StatusNew {
		t.Errorf("section plan = %+v, want new", res.Plan.Description)
	}
	if res.Profile.Description == nil || !reflect.DeepEqual(*res.Profile.Description, cfg) {
		t.Errorf("returned profile = %+v", res.Profile.Description)
	}
	if got := e.desc(t, dst); got == nil || !reflect.DeepEqual(*got, cfg) {
		t.Errorf("section was not saved: %+v", got)
	}
	p, _ := e.profiles.Get(ctx, e.userID, dst)
	if len(p.Taxonomy[trackerprofile.AxisCategory]) != 2 {
		t.Errorf("file taxonomy was not included: %+v", p.Taxonomy)
	}

	// Re-importing the same file: everything is "same", nothing to write.
	var before, after string
	_ = e.pool.QueryRow(ctx, `SELECT updated_at::text FROM tracker_profiles WHERE tracker_id = $1`, dst).Scan(&before)
	res, err = e.profiles.Import(ctx, e.userID, dst, parsed, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = e.pool.QueryRow(ctx, `SELECT updated_at::text FROM tracker_profiles WHERE tracker_id = $1`, dst).Scan(&after)
	if res.Plan.Description.Status != trackerprofile.StatusSame || res.Plan.Changes() || before != after {
		t.Errorf("reimport should be a no-op: plan %+v, updated_at %s → %s", res.Plan.Description, before, after)
	}
}

// The manual rule, applied to the section: what the operator configured is only changed if they request it.
func TestImport_OperatorSectionIsProtected(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	file := customDesc()
	src := e.newTracker(t, "Origin")
	e.fill(t, src)
	if _, err := e.profiles.SetDescription(ctx, e.userID, src, &file); err != nil {
		t.Fatal(err)
	}
	exported, err := e.profiles.Export(ctx, e.userID, src)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := exported.Marshal()
	// The file enters through the ONLY entry point, as if it came from a third party.
	doc, err := trackerprofile.ParseDocument(raw)
	if err != nil {
		t.Fatal(err)
	}

	dst := e.newTracker(t, "Destination")
	mine := otherDesc()
	if _, err := e.profiles.SetDescription(ctx, e.userID, dst, &mine); err != nil {
		t.Fatal(err)
	}

	// Preview shows what would be kept — and does not write.
	plan, err := e.profiles.PreviewImport(ctx, e.userID, dst, doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Description == nil || plan.Description.Status != trackerprofile.StatusProtected || plan.Description.Current == nil {
		t.Fatalf("plan = %+v, want protected with current config", plan.Description)
	}

	// Applying without requesting: the taxonomy comes in, the operator's section stays.
	res, err := e.profiles.Import(ctx, e.userID, dst, doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Plan.Description.Status != trackerprofile.StatusProtected {
		t.Errorf("status = %q", res.Plan.Description.Status)
	}
	if got := e.desc(t, dst); got == nil || !reflect.DeepEqual(*got, mine) {
		t.Errorf("operator config was overwritten without request: %+v", got)
	}

	// If requested, it is replaced.
	res, err = e.profiles.Import(ctx, e.userID, dst, doc, map[trackerprofile.OverwriteKey]bool{trackerprofile.DescriptionKey: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Plan.Description.Status != trackerprofile.StatusReplaced {
		t.Errorf("status = %q, want replaced", res.Plan.Description.Status)
	}
	if got := e.desc(t, dst); got == nil || !reflect.DeepEqual(*got, file) {
		t.Errorf("section was not replaced on request: %+v", got)
	}
}

// Preview never writes nor creates the row — even with the section.
func TestPreviewImport_WithSectionDoesNotSave(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	file := customDesc()
	d := trackerprofile.Document{Format: trackerprofile.FormatID, Site: trackerprofile.Site{Name: "S"}, Profile: trackerprofile.Profile{
		Family: trackerprofile.FamilyUNIT3D, SchemaVersion: trackerprofile.SchemaVersion, Description: &file,
	}}
	id := e.newTracker(t, "Preview only")
	plan, err := e.profiles.PreviewImport(ctx, e.userID, id, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Description == nil || plan.Description.Status != trackerprofile.StatusNew {
		t.Errorf("plan = %+v", plan.Description)
	}
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM tracker_profiles WHERE tracker_id = $1`, id).Scan(&n)
	if n != 0 {
		t.Error("preview created the profile row")
	}
}
