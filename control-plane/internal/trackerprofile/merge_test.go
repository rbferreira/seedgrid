package trackerprofile

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// exampleProfile is the taxonomy of a UNIT3D site (labels in Portuguese, seed origin), reduced
// to what the merge tests need.
func exampleProfile() Taxonomy {
	return Taxonomy{
		AxisCategory: {
			{ID: 1, Name: "Filmes", Kind: KindMovie, Origin: OriginSeed},
			{ID: 2, Name: "Séries", Kind: KindTV, Origin: OriginSeed},
			{ID: 5, Name: "Games", Kind: KindGame, Origin: OriginSeed},
		},
		AxisType: {
			{ID: 3, Name: "Encode", Origin: OriginSeed},
			{ID: 4, Name: "WEB-DL", Origin: OriginSeed},
		},
		AxisResolution: {
			{ID: 3, Name: "1080p", Origin: OriginSeed},
			{ID: 10, Name: "Outra", Origin: OriginSeed},
		},
	}
}

func find(t Taxonomy, axis string, id int) (Term, bool) {
	for _, tm := range t[axis] {
		if tm.ID == id {
			return tm, true
		}
	}
	return Term{}, false
}

// What a real UNIT3D site returned (03/10/2026) against this profile: what the profile already
// has, what is missing (PC and Windows types, which the profile didn't have) and the two
// name differences ("Series"/"Séries" and "Other"/"Outra").
func TestPropose_AgainstARealSite(t *testing.T) {
	observed := map[string][]Observed{
		AxisCategory:   {{1, "Filmes", 52}, {2, "Series", 78}, {3, "Animes", 15}, {5, "Games", 3}, {9, "Programas", 2}},
		AxisType:       {{2, "Remux", 8}, {3, "Encode", 9}, {4, "WEB-DL", 128}, {50, "PC", 3}, {71, "Windows", 2}},
		AxisResolution: {{2, "2160p", 11}, {3, "1080p", 128}, {10, "Other", 1}},
	}
	cur := exampleProfile()
	cur[AxisType] = append(cur[AxisType], Term{ID: 6, Name: "HDTV", Origin: OriginSeed}) // the catalog didn't show
	p := Propose(cur, observed)

	status := func(axis string, id int) ProposalItem {
		for _, it := range p.Items[axis] {
			if it.ID == id {
				return it
			}
		}
		t.Fatalf("%s id %d is not in the proposal", axis, id)
		return ProposalItem{}
	}
	if it := status(AxisCategory, 1); it.Status != StatusSame {
		t.Errorf("Filmes: %+v, want same", it)
	}
	if it := status(AxisCategory, 3); it.Status != StatusNew || it.Seen != 15 {
		t.Errorf("Animes: %+v, want new seen 15x", it)
	}
	if it := status(AxisType, 50); it.Status != StatusNew || it.Name != "PC" {
		t.Errorf("type PC: %+v, want new", it)
	}
	it := status(AxisCategory, 2)
	if it.Status != StatusRenamed || it.CurrentName != "Séries" || it.Name != "Series" || it.Protected {
		t.Errorf("Séries→Series: %+v, want renamed, not protected (origin is seed)", it)
	}
	if it := status(AxisResolution, 10); it.Status != StatusRenamed || it.CurrentName != "Outra" {
		t.Errorf("Outra→Other: %+v", it)
	}
	// What the profile has and the catalog DID NOT show is left out of the proposal — absence is not
	// evidence (the catalog only shows what someone posted).
	for _, it := range p.Items[AxisType] {
		if it.ID == 6 {
			t.Errorf("the proposal included HDTV, which the catalog did not show: %+v", it)
		}
	}
	if len(p.Items) != len(Axes) {
		t.Errorf("the proposal has %d axes, want %d (all present)", len(p.Items), len(Axes))
	}
}

func TestPropose_ProtectsHandAdjustedTerms(t *testing.T) {
	cur := exampleProfile()
	cur[AxisCategory][1] = Term{ID: 2, Name: "Seriados", Kind: KindTV, Origin: OriginManual}
	p := Propose(cur, map[string][]Observed{AxisCategory: {{2, "Series", 10}}})

	it := p.Items[AxisCategory][0]
	if it.Status != StatusRenamed || !it.Protected || it.CurrentOrigin != OriginManual {
		t.Errorf("item = %+v, want renamed + protected (manual origin)", it)
	}
}

// The catalog is third-party. Each hostile entry is discarded AND counted — the operator sees that
// there was a rejection instead of a proposal that silently seems complete.
func TestPropose_DiscardsAndCountsValidationFailures(t *testing.T) {
	p := Propose(Taxonomy{}, map[string][]Observed{
		AxisCategory: {
			{1, "Filmes", 5},
			{2, "Fil\u202eSeries", 5},        // direction override
			{3, "", 5},                       // empty
			{0, "no id", 5},                  // id out of range
			{MaxTermID + 1, "Giant", 5},      // id out of range
			{4, strings.Repeat("x", 500), 5}, // too long
			{5, "Ok", -1},                    // negative count
		},
	})
	if got := p.Items[AxisCategory]; len(got) != 1 || got[0].Name != "Filmes" {
		t.Errorf("items = %+v, want only Filmes", got)
	}
	if p.Rejected != 6 {
		t.Errorf("Rejected = %d, want 6", p.Rejected)
	}
}

// The same id with two names (the admin renamed it in the middle of the scan) keeps the most
// frequent one, adds the counts and warns — and the choice is deterministic on a tie.
func TestPropose_SameIDWithTwoNames(t *testing.T) {
	p := Propose(Taxonomy{}, map[string][]Observed{
		AxisType: {{3, "FHD", 2}, {3, "1080p", 7}, {4, "Beta", 1}, {4, "Alfa", 1}},
	})
	got := p.Items[AxisType]
	if len(got) != 2 || got[0].Name != "1080p" || got[0].Seen != 9 {
		t.Errorf("id 3 = %+v, want 1080p seen 9x (2+7)", got)
	}
	if got[1].Name != "Alfa" {
		t.Errorf("id 4 = %+v, want Alfa (tie resolved in alphabetical order)", got[1])
	}
	if len(p.Warnings) != 2 {
		t.Errorf("warnings = %v, want 2 (one per id with divergent names)", p.Warnings)
	}
}

func TestValidateApplyItems(t *testing.T) {
	ok, err := ValidateApplyItems([]ApplyItem{
		{Axis: AxisType, ID: 50, Name: "  PC  "},
		{Axis: AxisType, ID: 71, Name: "Windows", Origin: OriginManual},
	})
	if err != nil {
		t.Fatalf("valid items rejected: %v", err)
	}
	if ok[0].Name != "PC" || ok[0].Origin != OriginDiscovered || ok[1].Origin != OriginManual {
		t.Errorf("items = %+v; name must be clean and empty origin must be discovered", ok)
	}

	many := make([]ApplyItem, MaxApplyItems+1)
	for i := range many {
		many[i] = ApplyItem{Axis: AxisType, ID: i + 1, Name: "x"}
	}
	cases := map[string][]ApplyItem{
		"no item":        {},
		"too many items": many,
		"hostile axis":   {{Axis: "'; DROP TABLE trackers;--", ID: 1, Name: "x"}},
		"missing axis":   {{ID: 1, Name: "x"}},
		"id zero":        {{Axis: AxisType, ID: 0, Name: "x"}},
		"huge id":        {{Axis: AxisType, ID: MaxTermID + 1, Name: "x"}},
		"hostile name":   {{Axis: AxisType, ID: 1, Name: "a\x00b"}},
		"empty name":     {{Axis: AxisType, ID: 1, Name: ""}},
		"seed origin":    {{Axis: AxisType, ID: 1, Name: "x", Origin: OriginSeed}}, // only the migration seeds
		"junk origin":    {{Axis: AxisType, ID: 1, Name: "x", Origin: "admin"}},
		"repeated item":  {{Axis: AxisType, ID: 1, Name: "a"}, {Axis: AxisType, ID: 1, Name: "b"}},
	}
	for name, items := range cases {
		_, err := ValidateApplyItems(items)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: error = %v, want *ValidationError", name, err)
		}
	}
	// The same id on different axes is NOT a duplicate.
	if _, err := ValidateApplyItems([]ApplyItem{{Axis: AxisType, ID: 3, Name: "a"}, {Axis: AxisResolution, ID: 3, Name: "b"}}); err != nil {
		t.Errorf("id 3 on different axes was rejected: %v", err)
	}
}

// The entire Merge rule, one line per case.
func TestMerge_TheRules(t *testing.T) {
	cur := exampleProfile()
	cur[AxisType] = append(cur[AxisType], Term{ID: 9, Name: "MyType", Origin: OriginManual})
	cur[AxisType] = append(cur[AxisType], Term{ID: 6, Name: "HDTV", Origin: OriginDiscovered})

	got, applied, skipped, err := Merge(cur, []ApplyItem{
		{Axis: AxisType, ID: 50, Name: "PC", Origin: OriginDiscovered},        // new: add
		{Axis: AxisType, ID: 4, Name: "WEB-DL", Origin: OriginDiscovered},     // same name: nothing
		{Axis: AxisType, ID: 9, Name: "Other name", Origin: OriginDiscovered}, // manual: protected
		{Axis: AxisType, ID: 6, Name: "HDTV 1080", Origin: OriginDiscovered},  // discovered: rename
		{Axis: AxisCategory, ID: 2, Name: "Series", Origin: OriginDiscovered}, // seed: renames, keeps kind
		{Axis: AxisCategory, ID: 7, Name: "Doc", Origin: OriginManual},        // new, edited by the operator
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	wantApplied := []Applied{
		{AxisType, 50, "PC", ActionAdded},
		{AxisType, 6, "HDTV 1080", ActionRenamed},
		{AxisCategory, 2, "Series", ActionRenamed},
		{AxisCategory, 7, "Doc", ActionAdded},
	}
	wantSkipped := []Skipped{
		{AxisType, 4, "WEB-DL", ReasonSame},
		{AxisType, 9, "Other name", ReasonProtected},
	}
	if !reflect.DeepEqual(applied, wantApplied) {
		t.Errorf("applied:\n got %+v\nwant %+v", applied, wantApplied)
	}
	if !reflect.DeepEqual(skipped, wantSkipped) {
		t.Errorf("skipped:\n got %+v\nwant %+v", skipped, wantSkipped)
	}

	if tm, _ := find(got, AxisType, 9); tm.Name != "MyType" || tm.Origin != OriginManual {
		t.Errorf("the manual term was changed: %+v", tm)
	}
	if tm, _ := find(got, AxisType, 50); tm.Origin != OriginDiscovered {
		t.Errorf("new term should start as discovered: %+v", tm)
	}
	if tm, _ := find(got, AxisCategory, 2); tm.Name != "Series" || tm.Kind != KindTV || tm.Origin != OriginDiscovered {
		t.Errorf("Séries→Series: %+v; should change the name, keep the kind and become discovered", tm)
	}
	if tm, _ := find(got, AxisCategory, 7); tm.Origin != OriginManual {
		t.Errorf("term accepted with operator edit should be manual: %+v", tm)
	}
	// What the item didn't mention was not touched.
	if tm, _ := find(got, AxisCategory, 5); tm.Name != "Games" || tm.Kind != KindGame {
		t.Errorf("unreferenced term was touched: %+v", tm)
	}
}

// "Running again is incremental": applying the same set a second time changes nothing.
func TestMerge_RunningAgainChangesNothing(t *testing.T) {
	items := []ApplyItem{
		{Axis: AxisType, ID: 50, Name: "PC", Origin: OriginDiscovered},
		{Axis: AxisCategory, ID: 9, Name: "Programas", Origin: OriginDiscovered},
	}
	once, applied1, _, err := Merge(exampleProfile(), items)
	if err != nil || len(applied1) != 2 {
		t.Fatalf("1st application: applied=%v err=%v", applied1, err)
	}
	twice, applied2, skipped2, err := Merge(once, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied2) != 0 || len(skipped2) != 2 {
		t.Errorf("2nd application: applied=%v skipped=%v, want 0 applied and 2 skipped", applied2, skipped2)
	}
	if !reflect.DeepEqual(once, twice) {
		t.Error("the 2nd application changed the taxonomy")
	}
}

// Merge cannot change the taxonomy it received: the caller uses it to compare and, in the
// service, it is the state read under lock — mutating it underneath would be a hard-to-spot bug.
func TestMerge_DoesNotMutateInput(t *testing.T) {
	cur := exampleProfile()
	before := fmt.Sprint(cur)
	if _, _, _, err := Merge(cur, []ApplyItem{
		{Axis: AxisCategory, ID: 2, Name: "Series", Origin: OriginDiscovered},
		{Axis: AxisCategory, ID: 99, Name: "New", Origin: OriginDiscovered},
	}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(cur) != before {
		t.Errorf("Merge altered the entry:\n before  %s\n after %v", before, cur)
	}
}

func TestMerge_RefusesToExceedAxisLimit(t *testing.T) {
	var terms []Term
	for i := 1; i <= MaxTermsPerAxis; i++ {
		terms = append(terms, Term{ID: i, Name: "x", Origin: OriginManual})
	}
	_, _, _, err := Merge(Taxonomy{AxisType: terms}, []ApplyItem{{Axis: AxisType, ID: MaxTermsPerAxis + 1, Name: "y", Origin: OriginDiscovered}})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("error = %v, want *ValidationError (full axis)", err)
	}
}

// The scenario the issue describes, from start to finish: the operator adjusts it manually, the discovery
// runs AGAIN, and the adjustment survives.
func TestScenario_RepeatedDiscoveryNeverOverwritesManual(t *testing.T) {
	// 1. Seeded profile; the operator renames "Séries" to "Seriados" and creates id 99.
	tax := ApplyEdit(exampleProfile(), map[string][]TermInput{
		AxisCategory: {
			{ID: 1, Name: "Filmes", Kind: KindMovie},
			{ID: 2, Name: "Seriados", Kind: KindTV}, // renamed
			{ID: 5, Name: "Games", Kind: KindGame},
			{ID: 99, Name: "My Course"}, // created
		},
	})

	// 2. The discovery shows what the site has — including "Series" (id 2) and a new id.
	obs := map[string][]Observed{AxisCategory: {{1, "Filmes", 5}, {2, "Series", 5}, {3, "Animes", 5}}}
	prop := Propose(tax, obs)

	// 3. The operator accepts EVERYTHING the proposal listed.
	var items []ApplyItem
	for _, it := range prop.Items[AxisCategory] {
		items = append(items, ApplyItem{Axis: AxisCategory, ID: it.ID, Name: it.Name})
	}
	clean, err := ValidateApplyItems(items)
	if err != nil {
		t.Fatal(err)
	}
	next, applied, skipped, err := Merge(tax, clean)
	if err != nil {
		t.Fatal(err)
	}

	// 4. What belonged to the operator is intact; only what was missing came in.
	if tm, _ := find(next, AxisCategory, 2); tm.Name != "Seriados" || tm.Origin != OriginManual {
		t.Errorf("the manual rename was overwritten: %+v", tm)
	}
	if tm, _ := find(next, AxisCategory, 99); tm.Name != "My Course" || tm.Origin != OriginManual {
		t.Errorf("the manually created term disappeared or changed: %+v", tm)
	}
	if tm, ok := find(next, AxisCategory, 3); !ok || tm.Name != "Animes" || tm.Origin != OriginDiscovered {
		t.Errorf("Animes should have been added as discovered: %+v", tm)
	}
	if len(applied) != 1 || applied[0].ID != 3 {
		t.Errorf("applied = %+v, want only id 3", applied)
	}
	foundProtected := false
	for _, s := range skipped {
		if s.ID == 2 && s.Reason == ReasonProtected {
			foundProtected = true
		}
	}
	if !foundProtected {
		t.Errorf("skipped = %+v, want id 2 as protected", skipped)
	}
}
