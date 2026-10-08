package trackerprofile

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// docOf builds an already validated document from terms by axis (no origin — the exchange
// file does not carry it).
func docOf(site Site, tax Taxonomy) Document {
	return Document{
		Format:  FormatID,
		Site:    site,
		Profile: Profile{Family: FamilyUNIT3D, SchemaVersion: SchemaVersion, Taxonomy: normalize(tax)},
	}
}

var testSite = Site{Name: "Example", Host: "tracker.example.org"}

func profileOf(tax Taxonomy) Profile {
	return Profile{Family: FamilyUNIT3D, SchemaVersion: SchemaVersion, Taxonomy: normalize(tax)}
}

func itemByID(p ImportPlan, axis string, id int) ImportItem {
	for _, it := range p.Items[axis] {
		if it.ID == id {
			return it
		}
	}
	return ImportItem{}
}

func termByID(t Taxonomy, axis string, id int) (Term, bool) {
	for _, x := range t[axis] {
		if x.ID == id {
			return x, true
		}
	}
	return Term{}, false
}

// The case that justifies the feature: a newly registered tracker, with an empty profile, imports the
// profile that another user contributed — and everything comes in as new, with origin "imported".
func TestImport_EmptyProfileReceivesEverythingAsNew(t *testing.T) {
	doc := docOf(testSite, Taxonomy{
		AxisCategory:   {{ID: 1, Name: "Filmes", Kind: KindMovie}, {ID: 2, Name: "Series", Kind: KindTV}},
		AxisType:       {{ID: 3, Name: "Encode"}},
		AxisResolution: {{ID: 3, Name: "1080p"}},
	})
	plan, err := PlanImport(Empty(), doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := plan.Counts; c.New != 4 || c.Renamed+c.Same+c.Protected+c.KindSet+c.LocalOnly != 0 {
		t.Errorf("counts = %+v, want 4 new and nothing else", c)
	}
	if !plan.Changes() {
		t.Error("Changes() = false, but there are 4 new terms")
	}
	got := ApplyImport(Empty().Taxonomy, plan)
	want := Taxonomy{
		AxisCategory: {
			{ID: 1, Name: "Filmes", Kind: KindMovie, Origin: OriginImported},
			{ID: 2, Name: "Series", Kind: KindTV, Origin: OriginImported},
		},
		AxisType:       {{ID: 3, Name: "Encode", Origin: OriginImported}},
		AxisResolution: {{ID: 3, Name: "1080p", Origin: OriginImported}},
	}
	if !reflect.DeepEqual(got, normalize(want)) {
		t.Errorf("taxonomy:\n got %+v\nwant %+v", got, normalize(want))
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", plan.Warnings)
	}
}

// The coexistence rule from #219, extended to import: what the operator adjusted by hand is not
// overwritten, what THEIR SITE confirmed (discovered) is not either — discovery treats rename
// as opt-in, and import cannot be more permissive than that —, what came from the migration can be
// fixed, and what the file does not bring stays as is.
func TestImport_ManualCoexistenceRule(t *testing.T) {
	cur := profileOf(Taxonomy{AxisCategory: {
		{ID: 1, Name: "Filmes", Kind: KindMovie, Origin: OriginSeed},
		{ID: 2, Name: "Seriados", Kind: KindTV, Origin: OriginManual}, // the operator renamed
		{ID: 3, Name: "Anime", Origin: OriginSeed},
		{ID: 4, Name: "Cursos", Origin: OriginManual},  // the file agrees
		{ID: 6, Name: "Doc", Origin: OriginDiscovered}, // the operator's site confirmed this name
		{ID: 9, Name: "Local only", Origin: OriginManual},
	}})
	doc := docOf(testSite, Taxonomy{AxisCategory: {
		{ID: 1, Name: "Filmes", Kind: KindMovie},
		{ID: 2, Name: "Series", Kind: KindTV},
		{ID: 3, Name: "Animes", Kind: KindTV},
		{ID: 4, Name: "Cursos"},
		{ID: 5, Name: "Games", Kind: KindGame},
		{ID: 6, Name: "Documentarios"},
	}})
	plan, err := PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]string{1: StatusSame, 2: StatusProtected, 3: StatusRenamed, 4: StatusSame, 5: StatusNew, 6: StatusProtected} {
		if got := itemByID(plan, AxisCategory, id).Status; got != want {
			t.Errorf("id %d: status %q, want %q", id, got, want)
		}
	}
	if it := itemByID(plan, AxisCategory, 2); it.CurrentName != "Seriados" || it.CurrentOrigin != OriginManual {
		t.Errorf("the protected item must say what the operator called it: %+v", it)
	}
	if c := plan.Counts; c.New != 1 || c.Renamed != 1 || c.Same != 2 || c.Protected != 2 || c.LocalOnly != 1 || c.KindSet != 1 {
		t.Errorf("counts = %+v", c)
	}
	if it := itemByID(plan, AxisCategory, 6); it.CurrentName != "Doc" || it.CurrentOrigin != OriginDiscovered {
		t.Errorf("the protected item must say where the name it keeps comes from: %+v", it)
	}

	got := ApplyImport(cur.Taxonomy, plan)
	if x, _ := termByID(got, AxisCategory, 2); x.Name != "Seriados" || x.Origin != OriginManual {
		t.Errorf("manual adjustment was overwritten: %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 3); x.Name != "Animes" || x.Origin != OriginImported || x.Kind != KindTV {
		t.Errorf("the rename of a seed term was not applied (or did not gain the role): %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 6); x.Name != "Doc" || x.Origin != OriginDiscovered {
		t.Errorf("the name confirmed by the operator's site was overwritten without request: %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 9); x.Name != "Local only" || x.Origin != OriginManual {
		t.Errorf("the term absent from the file must remain untouched (the import never removes): %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 5); x.Origin != OriginImported || x.Kind != KindGame {
		t.Errorf("new term: %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 1); x.Origin != OriginSeed {
		t.Errorf("a term that the file only confirms cannot change its origin: %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 4); x.Origin != OriginManual {
		t.Errorf("manual with the same name remains manual: %+v", x)
	}
}

// "Without asking": the operator can overwrite ONE manually adjusted term, requesting it by id.
func TestImport_OverwriteManualOnlyWhenRequested(t *testing.T) {
	cur := profileOf(Taxonomy{AxisCategory: {
		{ID: 2, Name: "Seriados", Origin: OriginManual},
		{ID: 3, Name: "Desenhos", Origin: OriginManual},
	}})
	doc := docOf(testSite, Taxonomy{AxisCategory: {{ID: 2, Name: "Series", Kind: KindTV}, {ID: 3, Name: "Animes"}}})

	plan, err := PlanImport(cur, doc, testSite, map[OverwriteKey]bool{{Axis: AxisCategory, ID: 2}: true})
	if err != nil {
		t.Fatal(err)
	}
	if it := itemByID(plan, AxisCategory, 2); it.Status != StatusRenamed || !it.OverwritesProtected || !it.SetsKind {
		t.Errorf("id 2 (requested): %+v", it)
	}
	if it := itemByID(plan, AxisCategory, 3); it.Status != StatusProtected {
		t.Errorf("id 3 (not requested) must remain protected: %+v", it)
	}
	got := ApplyImport(cur.Taxonomy, plan)
	if x, _ := termByID(got, AxisCategory, 2); x.Name != "Series" || x.Origin != OriginImported || x.Kind != KindTV {
		t.Errorf("id 2 after the request: %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 3); x.Name != "Desenhos" || x.Origin != OriginManual {
		t.Errorf("id 3 was changed without request: %+v", x)
	}

	// A request that does not correspond to any manual term in the file becomes a warning, never an effect.
	plan, err = PlanImport(cur, doc, testSite, map[OverwriteKey]bool{{Axis: AxisType, ID: 99}: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "override request") {
		t.Errorf("warnings = %v, want the one for the ineffective request", plan.Warnings)
	}
	if plan.Counts.Renamed != 0 {
		t.Errorf("an unmatched request renamed something: %+v", plan.Counts)
	}
}

// The file only FILLS an empty role; it never changes what already exists. Discovery does not create
// kind, so filling is the most common gain of importing over a discovered profile.
func TestImport_KindOnlyFillsWhatIsEmpty(t *testing.T) {
	cur := profileOf(Taxonomy{AxisCategory: {
		{ID: 1, Name: "Filmes", Origin: OriginDiscovered},                  // no role: fills
		{ID: 2, Name: "Series", Kind: KindMovie, Origin: OriginDiscovered}, // different role: keeps the local one
		{ID: 3, Name: "Animes", Kind: KindTV, Origin: OriginDiscovered},    // file without role: keeps it
		{ID: 4, Name: "Jogos", Origin: OriginManual},                       // same name, manual: fills the role
		{ID: 5, Name: "Cursos local", Origin: OriginManual},                // protected: not even the role
	}})
	doc := docOf(testSite, Taxonomy{AxisCategory: {
		{ID: 1, Name: "Filmes", Kind: KindMovie},
		{ID: 2, Name: "Series", Kind: KindTV},
		{ID: 3, Name: "Animes"},
		{ID: 4, Name: "Jogos", Kind: KindGame},
		{ID: 5, Name: "Cursos", Kind: KindGame},
	}})
	plan, err := PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Counts.KindSet != 2 {
		t.Errorf("KindSet = %d, want 2 (ids 1 and 4)", plan.Counts.KindSet)
	}
	// The divergent role of id 2 is warned; and filling the role of id 1 makes it the FIRST category
	// "Movie" (before it was id 2): the panel choice would change without the operator having decided anything.
	if !hasWarning(plan.Warnings, "different role") || !hasWarning(plan.Warnings, "collide") {
		t.Errorf("warnings = %v, want the divergent role warning (id 2) and the collision warning", plan.Warnings)
	}
	if w := itemByID(plan, AxisCategory, 1).Warnings; len(w) != 1 || !strings.Contains(w[0], "first category with the role «Movie»") || !strings.Contains(w[0], "id 2") {
		t.Errorf("warning for id 1 = %v", w)
	}
	got := ApplyImport(cur.Taxonomy, plan)
	for id, want := range map[int]string{1: KindMovie, 2: KindMovie, 3: KindTV, 4: KindGame, 5: ""} {
		if x, _ := termByID(got, AxisCategory, id); x.Kind != want {
			t.Errorf("id %d: kind %q, want %q", id, x.Kind, want)
		}
	}
	// Filling only the role does not change the origin (the same rule as manual editing).
	if x, _ := termByID(got, AxisCategory, 1); x.Origin != OriginDiscovered {
		t.Errorf("filling the role changed the origin: %+v", x)
	}
	if x, _ := termByID(got, AxisCategory, 4); x.Origin != OriginManual {
		t.Errorf("filling the role of a manual term changed the origin: %+v", x)
	}
}

// Importing the same file twice is incremental: the second time changes nothing.
func TestImport_Idempotent(t *testing.T) {
	doc := docOf(testSite, Taxonomy{
		AxisCategory: {{ID: 1, Name: "Filmes", Kind: KindMovie}},
		AxisType:     {{ID: 3, Name: "Encode"}, {ID: 4, Name: "WEB-DL"}},
	})
	plan1, _ := PlanImport(Empty(), doc, testSite, nil)
	after1 := ApplyImport(Empty().Taxonomy, plan1)

	plan2, err := PlanImport(profileOf(after1), doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.Changes() {
		t.Errorf("the 2nd import of the same file still changes things: %+v", plan2.Counts)
	}
	if plan2.Counts.Same != 3 {
		t.Errorf("Same = %d, want 3", plan2.Counts.Same)
	}
	if after2 := ApplyImport(after1, plan2); !reflect.DeepEqual(after1, after2) {
		t.Errorf("applying a plan with no changes altered the taxonomy:\n%+v\n%+v", after1, after2)
	}
}

// Applying cannot touch the received taxonomy (it comes from the read state of the database).
func TestApplyImport_DoesNotMutateInput(t *testing.T) {
	cur := Taxonomy{AxisCategory: {{ID: 1, Name: "Filmes", Origin: OriginDiscovered}}}
	snapshot := normalize(cur)
	plan, _ := PlanImport(profileOf(cur), docOf(testSite, Taxonomy{AxisCategory: {
		{ID: 1, Name: "Cinema", Kind: KindMovie}, {ID: 2, Name: "Series"},
	}}), testSite, nil)
	_ = ApplyImport(cur, plan)
	if !reflect.DeepEqual(normalize(cur), snapshot) {
		t.Errorf("ApplyImport changed entry: %+v", cur)
	}
}

func TestImport_TermsPerAxisLimit(t *testing.T) {
	full := make([]Term, MaxTermsPerAxis)
	for i := range full {
		full[i] = Term{ID: i + 1, Name: "x", Origin: OriginManual}
	}
	cur := profileOf(Taxonomy{AxisType: full})

	// An id that already exists fits; a new id would overflow the axis.
	if _, err := PlanImport(cur, docOf(testSite, Taxonomy{AxisType: {{ID: 1, Name: "x"}}}), testSite, nil); err != nil {
		t.Errorf("existing term in full axis: %v", err)
	}
	_, err := PlanImport(cur, docOf(testSite, Taxonomy{AxisType: {{ID: MaxTermsPerAxis + 1, Name: "new"}}}), testSite, nil)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Path != "taxonomy.type" {
		t.Errorf("error = %v, want *ValidationError in taxonomy.type", err)
	}
}

func hasWarning(list []string, part string) bool {
	for _, w := range list {
		if strings.Contains(w, part) {
			return true
		}
	}
	return false
}

// A file from another site warns — the ids belong to the site, and importing another's publishes wrong. The warning
// also appears when the comparison is not possible: a file without a host does not disable the check.
func TestImport_WarnsWhenFileIsFromOtherSite(t *testing.T) {
	tax := Taxonomy{AxisType: {{ID: 12, Name: "Encode"}}}
	doc := docOf(Site{Name: "Blutopia", Host: "blutopia.example"}, tax)

	plan, _ := PlanImport(Empty(), doc, Site{Name: "My", Host: "tracker.example.org"}, nil)
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "Blutopia") || !strings.Contains(plan.Warnings[0], "blutopia.example") {
		t.Errorf("warnings = %v, want the different-site one naming both sides", plan.Warnings)
	}

	// Same host: there is nothing to warn about.
	if p, _ := PlanImport(Empty(), doc, Site{Name: "x", Host: "blutopia.example"}, nil); len(p.Warnings) != 0 {
		t.Errorf("same host: warnings %v, want none", p.Warnings)
	}

	// File WITHOUT host: cannot check — warns, instead of staying silent.
	noHost := docOf(Site{Name: "Anonymous"}, tax)
	p, _ := PlanImport(Empty(), noHost, Site{Name: "x", Host: "tracker.example.org"}, nil)
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "does not carry the site host") || !strings.Contains(p.Warnings[0], "Anonymous") {
		t.Errorf("file without host: warnings %v", p.Warnings)
	}
	// ...and the warning applies even when the tracker also has no host.
	if p, _ := PlanImport(Empty(), noHost, Site{Name: "x"}, nil); !hasWarning(p.Warnings, "does not carry the site host") {
		t.Errorf("file without host and tracker without host: warnings %v", p.Warnings)
	}

	// Tracker without a valid host, file with host: also cannot compare.
	p, _ = PlanImport(Empty(), doc, Site{Name: "x"}, nil)
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "does not have a valid host") || !strings.Contains(p.Warnings[0], "blutopia.example") {
		t.Errorf("tracker without host: warnings %v", p.Warnings)
	}
}

func TestImport_FileWithoutTermsWarns(t *testing.T) {
	plan, err := PlanImport(Empty(), docOf(testSite, nil), testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changes() || len(plan.Warnings) != 1 {
		t.Errorf("plan = %+v", plan)
	}
}

// The axes always come out as a list (never null): the Portal iterates without checking.
func TestPlanImport_AllAxesPresent(t *testing.T) {
	plan, _ := PlanImport(Empty(), docOf(testSite, Taxonomy{AxisType: {{ID: 1, Name: "a"}}}), testSite, nil)
	for _, axis := range Axes {
		if plan.Items[axis] == nil {
			t.Errorf("plan axis %s is nil", axis)
		}
	}
	if plan.Warnings == nil {
		t.Error("Warnings nil: JSON would be null")
	}
}

func TestParseOverwrite(t *testing.T) {
	got, err := ParseOverwrite([]string{"category:2", "type:50"})
	if err != nil || len(got) != 2 || !got[OverwriteKey{AxisCategory, 2}] || !got[OverwriteKey{AxisType, 50}] {
		t.Errorf("got %v, %v", got, err)
	}
	if got, err := ParseOverwrite(nil); err != nil || len(got) != 0 {
		t.Errorf("no requests: %v, %v", got, err)
	}
	for _, bad := range []string{
		"", "category", "category:", ":2", "category:x", "category:-1", "category:0",
		"category:99999999", "audio:1", "Category:1", "category:1:2", "category:2; DROP TABLE trackers",
		strings.Repeat("a", 5000),
	} {
		_, err := ParseOverwrite([]string{bad})
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%.40q: error = %v, want *ValidationError", bad, err)
		} else if len(ve.Error()) > 200 {
			t.Errorf("%.40q: message of %d bytes", bad, len(ve.Error()))
		}
	}
	many := make([]string, MaxApplyItems+1)
	for i := range many {
		many[i] = "type:1"
	}
	if _, err := ParseOverwrite(many); err == nil {
		t.Error("accepted too many requests")
	}
}
