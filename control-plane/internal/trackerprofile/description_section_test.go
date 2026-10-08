package trackerprofile

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/description"
)

// The profile description section (#192): validation, JSONB shape, export and import.

func custom() description.Config {
	return description.Config{
		MediaBlocks:  []string{description.BlockPoster, description.BlockMediaInfo},
		GameBlocks:   []string{description.BlockTitle, description.BlockAbout},
		MediaInfoTag: description.TagSpoiler,
	}
}

func asErr(t *testing.T, err error) *ValidationError {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %#v, want *ValidationError", err)
	}
	return ve
}

// ---- CleanDescription -------------------------------------------------------------------

func TestCleanDescription_ValidatesAndNormalizes(t *testing.T) {
	got, err := CleanDescription(description.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// The section that goes into the profile is always EXPLICIT: never nil, so the file and the API state
	// what it comprises without depending on the reader's default.
	if !reflect.DeepEqual(got, description.DefaultConfig()) {
		t.Errorf("cleaned zero = %+v, want explicit default", got)
	}
	got, err = CleanDescription(description.Config{MediaBlocks: []string{}})
	if err != nil || got.MediaBlocks == nil || len(got.MediaBlocks) != 0 {
		t.Errorf("empty list = %#v (%v), want empty and not nil", got.MediaBlocks, err)
	}
	if got.GameBlocks == nil || len(got.GameBlocks) != 5 {
		t.Errorf("missing list should become the default: %#v", got.GameBlocks)
	}
}

func TestCleanDescription_RejectsWithFieldPath(t *testing.T) {
	cases := []struct {
		path string
		cfg  description.Config
	}{
		{"description.media_blocks[1]", description.Config{MediaBlocks: []string{"poster", "x"}}},
		{"description.media_blocks[1]", description.Config{MediaBlocks: []string{"poster", "poster"}}},
		{"description.game_blocks[0]", description.Config{GameBlocks: []string{"poster"}}},
		{"description.media_info_tag", description.Config{MediaInfoTag: "html"}},
		{"description.media_blocks", description.Config{MediaBlocks: make([]string, 50000)}},
	}
	for _, tc := range cases {
		_, err := CleanDescription(tc.cfg)
		ve := asErr(t, err)
		if ve.Path != tc.path {
			t.Errorf("path = %q, want %q (%s)", ve.Path, tc.path, ve.Msg)
		}
		if ve.ErrorCode() != CodeInvalid {
			t.Errorf("code = %q, want %q (400 invalid_profile)", ve.ErrorCode(), CodeInvalid)
		}
	}
}

// ---- Format in JSONB --------------------------------------------------------------------

// The document of the row that already exists (only the axes, at the root) is read without the section — and rewritten
// identically when no one configured it. This is what dispenses with migration and row rewriting.
func TestDecodeEncode_LegacyLineUnchanged(t *testing.T) {
	const legacy = `{"category":[{"id":1,"name":"Filmes","kind":"movie","origin":"seed"}],"resolution":[],"type":[{"id":3,"name":"Encode","origin":"manual"}]}`
	p, err := decodeProfile(FamilyUNIT3D, 1, []byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if p.Description != nil {
		t.Errorf("legacy line parsed with section: %+v", p.Description)
	}
	if len(p.Taxonomy[AxisCategory]) != 1 || p.Taxonomy[AxisType][0].Origin != OriginManual {
		t.Errorf("read taxonomy = %+v", p.Taxonomy)
	}
	raw, err := encodeProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != legacy {
		t.Errorf("legacy line was rewritten differently:\n got %s\nwant %s", raw, legacy)
	}
	// And it's byte for byte what the previous code wrote (json.Marshal of the Taxonomy).
	old, _ := json.Marshal(p.Taxonomy)
	if string(raw) != string(old) {
		t.Errorf("without section, the document is no longer just the taxonomy:\n got %s\nold %s", raw, old)
	}
}

func TestDecodeEncode_SectionRoundTrip(t *testing.T) {
	cfg := custom()
	in := Profile{Family: FamilyUNIT3D, SchemaVersion: 1, Taxonomy: normalize(Taxonomy{AxisType: {{ID: 3, Name: "Encode", Origin: OriginManual}}}), Description: &cfg}
	raw, err := encodeProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	// The section lives at the root, next to the axes, under the reserved key.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"category", "type", "resolution", "description"} {
		if _, ok := top[key]; !ok {
			t.Errorf("missing key %q in document: %s", key, raw)
		}
	}
	out, err := decodeProfile(FamilyUNIT3D, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("round trip:\n got %+v\nwant %+v", out, in)
	}
	// The section does NOT become an axis of the taxonomy.
	if _, ok := out.Taxonomy["description"]; ok {
		t.Error("description section appeared as taxonomy axis")
	}
}

// The empty list ("no blocks") survives the database: it doesn't come back as the default.
func TestDecodeEncode_EmptyListSurvives(t *testing.T) {
	cfg := description.Config{MediaBlocks: []string{}, GameBlocks: []string{description.BlockTitle}, MediaInfoTag: description.TagCode}
	raw, err := encodeProfile(Profile{Taxonomy: normalize(nil), Description: &cfg})
	if err != nil {
		t.Fatal(err)
	}
	p, err := decodeProfile(FamilyUNIT3D, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Description == nil || p.Description.MediaBlocks == nil || len(p.Description.MediaBlocks) != 0 {
		t.Fatalf("empty list did not survive: %+v", p.Description)
	}
	if got := description.ComposeMedia(description.MediaInput{Poster: "p", MediaInfo: "m"}, *p.Description); got != "" {
		t.Errorf("with empty list description should be empty, got %q", got)
	}
}

// A malformed section saved in the database is an error, not "empty" — otherwise the next edit would
// overwrite it silently.
func TestDecodeProfile_UnreadableDocumentIsError(t *testing.T) {
	cases := map[string]string{
		"description string":        `{"description":"x"}`,
		"description list":          `{"description":[]}`,
		"description with type":     `{"description":{"media_blocks":"poster"}}`,
		"axis with wrong type":      `{"type":"x"}`,
		"axis with unreadable term": `{"type":[{"id":"x"}]}`,
		"not an object":             `[]`,
		"description inside ax":     `{"type":{"description":{}}}`,
	}
	for name, raw := range cases {
		if _, err := decodeProfile(FamilyUNIT3D, 1, []byte(raw)); err == nil {
			t.Errorf("%s: unreadable document accepted", name)
		}
	}
	// null and empty document mean a profile without a section.
	for _, raw := range []string{``, `{}`, `null`, `{"description":null}`} {
		p, err := decodeProfile(FamilyUNIT3D, 1, []byte(raw))
		if err != nil || p.Description != nil {
			t.Errorf("%q: section = %+v, error = %v, want profile without section", raw, p.Description, err)
		}
	}
}

// A hand-written section in the database with missing lists is read as the equivalent explicit config.
func TestDecodeProfile_IncompleteSectionBecomesExplicit(t *testing.T) {
	p, err := decodeProfile(FamilyUNIT3D, 1, []byte(`{"description":{"media_info_tag":"pre"}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := description.DefaultConfig()
	want.MediaInfoTag = description.TagPre
	if p.Description == nil || !reflect.DeepEqual(*p.Description, want) {
		t.Errorf("section = %+v, want %+v", p.Description, want)
	}
}

// ---- O Profile na API -------------------------------------------------------------------

func TestProfile_DescriptionAppearsOnlyWhenFilled(t *testing.T) {
	raw, _ := json.Marshal(Empty())
	if strings.Contains(string(raw), "description") {
		t.Errorf("profile without section serialized the key: %s", raw)
	}
	cfg := custom()
	p := Empty()
	p.Description = &cfg
	raw, _ = json.Marshal(p)
	if !strings.Contains(string(raw), `"description":{"media_blocks":["poster","mediainfo"],"game_blocks":["title","about"],"media_info_tag":"spoiler"}`) {
		t.Errorf("profile with section: %s", raw)
	}
}

// ---- Export -----------------------------------------------------------------------------

func TestExport_WithDescriptionSection(t *testing.T) {
	cfg := custom()
	p := fullProfile()
	p.Description = &cfg
	raw, err := NewDocument("Example", "tracker.example.org:2710", p).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	const want = `  "description": {
    "media_blocks": [
      "poster",
      "mediainfo"
    ],
    "game_blocks": [
      "title",
      "about"
    ],
    "media_info_tag": "spoiler"
  }
}
`
	if !strings.HasSuffix(string(raw), want) {
		t.Errorf("file does not end with the section (which comes after the taxonomy):\n%s", raw)
	}
	if v := mustValidate(t, raw); v.Description == nil || !reflect.DeepEqual(*v.Description, cfg) {
		t.Errorf("section did not round-trip through the file: %+v", v.Description)
	}
}

// Without the section, the file is exactly what it was before #192 (TestExport_StableFormat pins it
// byte for byte) — and does not carry the key.
func TestExport_NoSectionNoKey(t *testing.T) {
	raw, _ := NewDocument("Example", "tracker.example.org", fullProfile()).Marshal()
	if strings.Contains(string(raw), "description") {
		t.Errorf("export without section contains the key:\n%s", raw)
	}
}

func mustValidate(t *testing.T, raw []byte) Document {
	t.Helper()
	d, err := ParseDocument(raw)
	if err != nil {
		t.Fatalf("ParseDocument: %v\n%s", err, raw)
	}
	return d
}

// The export never delivers a file that the import itself would reject: an invalid section
// manually written in the database makes the export fail with the reason.
func TestExport_InvalidSectionNotExported(t *testing.T) {
	bad := description.Config{MediaInfoTag: "html"}
	p := fullProfile()
	p.Description = &bad
	doc := NewDocument("Example", "tracker.example.org", p)
	if _, err := doc.validated(); err == nil {
		t.Fatal("document with invalid tag passed export validation")
	}
}

// The section enters the export without messing with the export: the structural still covers the whole type,
// now with Config inside, and the section is registered in the import.
func TestDescriptionSectionIsHandledOnImport(t *testing.T) {
	if !contains(sectionNames(), "description") {
		t.Fatalf("description section is not a Profile section: %v", sectionNames())
	}
	if !handledSections["description"] {
		t.Error("description is not in handledSections: import would reject it (fail closed)")
	}
	if got := unhandledSection(Profile{Description: &description.Config{}}); got != "" {
		t.Errorf("handled section rejected as %q", got)
	}
}

// ---- Import: the file is hostile content ------------------------------------------------

func withDescription(v any) string {
	return mutate(func(m map[string]any) { m["description"] = v })
}

func TestParseDocument_SectionDescriptionValidAndNormalizes(t *testing.T) {
	d, err := ParseDocument([]byte(withDescription(map[string]any{
		"media_blocks":   []any{"mediainfo", "poster"},
		"game_blocks":    []any{"title"},
		"media_info_tag": "code",
	})))
	if err != nil {
		t.Fatal(err)
	}
	want := description.Config{MediaBlocks: []string{"mediainfo", "poster"}, GameBlocks: []string{"title"}, MediaInfoTag: "code"}
	if d.Description == nil || !reflect.DeepEqual(*d.Description, want) {
		t.Errorf("section = %+v, want %+v", d.Description, want)
	}

	// Missing fields become the default; empty list remains empty.
	d, err = ParseDocument([]byte(withDescription(map[string]any{"media_blocks": []any{}})))
	if err != nil {
		t.Fatal(err)
	}
	if d.Description.MediaBlocks == nil || len(d.Description.MediaBlocks) != 0 {
		t.Errorf("empty list = %#v", d.Description.MediaBlocks)
	}
	if !reflect.DeepEqual(d.Description.GameBlocks, description.DefaultConfig().GameBlocks) || d.Description.MediaInfoTag != "mediainfo" {
		t.Errorf("missing fields did not become the default: %+v", d.Description)
	}

	// Null section and file without section are the same thing.
	d, err = ParseDocument([]byte(withDescription(nil)))
	if err != nil || d.Description != nil {
		t.Errorf("null section: %+v, %v", d.Description, err)
	}
}

func TestParseDocument_SectionDescriptionHostile(t *testing.T) {
	long := strings.Repeat("y", 1<<16)
	cases := []struct {
		name, body, path string
	}{
		{"unknown block", withDescription(map[string]any{"media_blocks": []any{"poster", "x"}}), "description.media_blocks[1]"},
		{"game block in media list", withDescription(map[string]any{"media_blocks": []any{"cover"}}), "description.media_blocks[0]"},
		{"repeated block", withDescription(map[string]any{"game_blocks": []any{"title", "title"}}), "description.game_blocks[1]"},
		{"unknown tag", withDescription(map[string]any{"media_info_tag": "html"}), "description.media_info_tag"},
		{"tag that tries to escape BBCode", withDescription(map[string]any{"media_info_tag": "mediainfo][img=http://x]"}), "description.media_info_tag"},
		{"giant tag", withDescription(map[string]any{"media_info_tag": long}), "description.media_info_tag"},
		{"giant block", withDescription(map[string]any{"media_blocks": []any{long}}), "description.media_blocks[0]"},
		{"list with 20k items", withDescription(map[string]any{"media_blocks": make([]any, 20000)}), "description.media_blocks"},
		{"item that is not text", withDescription(map[string]any{"media_blocks": []any{3}}), ""},
		{"list that is text", withDescription(map[string]any{"media_blocks": "poster"}), ""},
		{"section that is a list", withDescription([]any{"poster"}), ""},
		{"section that is text", withDescription("poster"), ""},
		{"unknown field in section", withDescription(map[string]any{"media_blocks": []any{"poster"}, "html": "<script>"}), ""},
		{"uppercase key in section", withDescription(map[string]any{"Media_Blocks": []any{"poster"}}), "document"},
		{"duplicate key in section", `{"format":"seedgrid.tracker-profile","site":{"name":"S"},"family":"unit3d","schema_version":1,"taxonomy":{},` +
			`"description":{"media_info_tag":"pre","media_info_tag":"code"}}`, "document"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDocument([]byte(tc.body))
			ve := asErr(t, err)
			if tc.path != "" && ve.Path != tc.path {
				t.Errorf("path = %q, want %q (%s)", ve.Path, tc.path, ve.Msg)
			}
			// The received value never comes back intact: it can be megabytes.
			if len(ve.Error()) > 500 {
				t.Errorf("rejection returns %d bytes: %.100s…", len(ve.Error()), ve.Error())
			}
		})
	}
}

// ---- Import plan and application ----------------------------------------------------------

func docWith(cfg *description.Config) Document {
	d := docOf(testSite, Taxonomy{AxisType: {{ID: 3, Name: "Encode"}}})
	d.Description = cfg
	return d
}

func cfgPtr(c description.Config) *description.Config { return &c }

func TestPlanDescription_TheRules(t *testing.T) {
	mine := custom()
	other := description.Config{MediaBlocks: []string{description.BlockSynopsis}, MediaInfoTag: description.TagPre}
	def := description.DefaultConfig()

	cases := []struct {
		name      string
		current   *description.Config
		file      description.Config
		overwrite map[OverwriteKey]bool

		status   string
		changes  bool
		kept     *description.Config // what remains in the profile after applying
		current2 bool                // the plan shows the current config
		warns    bool
	}{
		{name: "profile without section receives the file's", current: nil, file: mine, status: StatusNew, changes: true, kept: &mine},
		{name: "profile with explicit default has nothing to protect", current: &def, file: mine, status: StatusNew, changes: true, kept: &mine},
		{name: "identical section changes nothing", current: &mine, file: mine, status: StatusSame, kept: &mine},
		{name: "file with default over profile without section: same", current: nil, file: def, status: StatusSame, kept: nil},
		{name: "operator config is protected", current: &mine, file: other, status: StatusProtected, kept: &mine, current2: true, warns: true},
		{name: "protected is replaced if operator requests", current: &mine, file: other, overwrite: map[OverwriteKey]bool{DescriptionKey: true}, status: StatusReplaced, changes: true, kept: &other, current2: true},
		{name: "requesting overwrite where there is nothing to protect does nothing special", current: nil, file: mine, overwrite: map[OverwriteKey]bool{DescriptionKey: true}, status: StatusNew, changes: true, kept: &mine},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := tc.file
			cur := profileOf(Taxonomy{})
			cur.Description = tc.current
			plan, err := PlanImport(cur, docWith(&file), testSite, tc.overwrite)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Description == nil || plan.Description.Status != tc.status {
				t.Fatalf("status = %+v, want %q", plan.Description, tc.status)
			}
			if !reflect.DeepEqual(plan.Description.Config, file.Normalized()) {
				t.Errorf("plan should have the normalized file config: %+v", plan.Description.Config)
			}
			if (plan.Description.Current != nil) != tc.current2 {
				t.Errorf("Current = %+v, want present=%v", plan.Description.Current, tc.current2)
			}
			if plan.Description.Status == StatusReplaced && !plan.Description.OverwritesProtected {
				t.Error("replaced without OverwritesProtected")
			}
			if plan.descriptionChanges() != tc.changes {
				t.Errorf("descriptionChanges = %v, want %v", plan.descriptionChanges(), tc.changes)
			}
			// The file brings term 3 from the type axis (new) — so Changes() is always true; what
			// isolates the section is the plan WITHOUT terms, below.
			after := ApplyImportDescription(tc.current, plan)
			switch {
			case tc.kept == nil && after != nil:
				t.Errorf("section should remain absent, got %+v", after)
			case tc.kept != nil && (after == nil || !reflect.DeepEqual(*after, tc.kept.Normalized())):
				t.Errorf("section after apply = %+v, want %+v", after, tc.kept)
			}
			if got := len(plan.Warnings) > 0; tc.warns && !got {
				t.Errorf("plan should warn that it kept the operator config: %v", plan.Warnings)
			}
		})
	}
}

// Only the section: with no terms in the file, Changes() reflects the section — and there is no "no
// terms" warning, which would lie (the file has something to import).
func TestImport_FileWithOnlySection(t *testing.T) {
	mine := custom()
	doc := docOf(testSite, nil)
	doc.Description = &mine

	plan, err := PlanImport(Empty(), doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changes() {
		t.Error("Changes() = false, but the section would be applied")
	}
	for _, w := range plan.Warnings {
		if strings.Contains(w, "no term") {
			t.Errorf("empty file warning in a file that contains the section: %q", w)
		}
	}

	// Protected and without terms: nothing changes.
	cur := Empty()
	cur.Description = cfgPtr(description.Config{MediaBlocks: []string{}})
	plan, err = PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changes() {
		t.Errorf("Changes() = true with protected section and no terms: %+v", plan.Description)
	}
	// And a file without terms and without section continues to warn.
	plan, _ = PlanImport(Empty(), docOf(testSite, nil), testSite, nil)
	if len(plan.Warnings) == 0 || !strings.Contains(plan.Warnings[0], "no terms") {
		t.Errorf("empty file without section should warn: %v", plan.Warnings)
	}
}

// The file that doesn't bring the section doesn't touch the profile's.
func TestImport_FileWithoutSectionDeletesNothing(t *testing.T) {
	mine := custom()
	cur := profileOf(Taxonomy{})
	cur.Description = &mine
	plan, err := PlanImport(cur, docOf(testSite, Taxonomy{AxisType: {{ID: 3, Name: "Encode"}}}), testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Description != nil {
		t.Errorf("plan with section for a file without it: %+v", plan.Description)
	}
	if got := ApplyImportDescription(&mine, plan); got != &mine {
		t.Errorf("profile section changed: %+v", got)
	}
}

func TestImport_IdempotentAndPureSection(t *testing.T) {
	file := custom()
	cur := Empty()
	plan, _ := PlanImport(cur, docWith(&file), testSite, nil)
	first := ApplyImportDescription(cur.Description, plan)

	// Reimporting the same file over the result: everything "same", nothing to save in the section.
	cur.Description = first
	plan2, _ := PlanImport(cur, docWith(&file), testSite, nil)
	if plan2.Description.Status != StatusSame || plan2.descriptionChanges() {
		t.Errorf("reimport should be same: %+v", plan2.Description)
	}
	// Does not mutate the input nor share memory with the plan.
	first.MediaBlocks[0] = "garbage"
	if plan.Description.Config.MediaBlocks[0] == "garbage" {
		t.Error("applied section shares memory with the plan")
	}
}

func TestParseOverwrite_Description(t *testing.T) {
	got, err := ParseOverwrite([]string{"description", "category:2"})
	if err != nil {
		t.Fatal(err)
	}
	if !got[DescriptionKey] || !got[OverwriteKey{Axis: "category", ID: 2}] || len(got) != 2 {
		t.Errorf("requests = %v", got)
	}
	for _, bad := range []string{"Description", "description:1", "descriptions", "description "} {
		if _, err := ParseOverwrite([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// The message states the two accepted formats.
	_, err = ParseOverwrite([]string{"x"})
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Errorf("rejection does not mention description: %v", err)
	}
}

// Asking to overwrite the description when nothing is protected becomes a warning, never takes effect.
func TestImport_OverwriteRequestNoMatchBecomesWarning(t *testing.T) {
	file := custom()
	plan, err := PlanImport(Empty(), docWith(&file), testSite, map[OverwriteKey]bool{DescriptionKey: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range plan.Warnings {
		if strings.Contains(w, "override request") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing warning for unmatched request: %v", plan.Warnings)
	}
	// But when there is something to overwrite, the request matches and does not warn.
	cur := Empty()
	cur.Description = cfgPtr(description.Config{MediaBlocks: []string{}})
	plan, _ = PlanImport(cur, docWith(&file), testSite, map[OverwriteKey]bool{DescriptionKey: true})
	for _, w := range plan.Warnings {
		if strings.Contains(w, "override request") {
			t.Errorf("spurious warning: %q", w)
		}
	}
}
