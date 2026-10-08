package trackerprofile

import (
	"testing"
)

// Discovery treats rename as opt-in; the import cannot be more permissive. The name that the
// OPERATOR's site confirmed (discovered) only changes if requested term by term — as per the manual.
func TestImport_DiscoveredOnlyChangesWhenRequested(t *testing.T) {
	cur := profileOf(Taxonomy{AxisType: {
		{ID: 3, Name: "Encode", Origin: OriginDiscovered},
		{ID: 4, Name: "WEBDL", Origin: OriginSeed},      // da migration: cede
		{ID: 5, Name: "WEBRip", Origin: OriginImported}, // from a previous import: yields
	}})
	doc := docOf(testSite, Taxonomy{AxisType: {{ID: 3, Name: "Re-encode"}, {ID: 4, Name: "WEB-DL"}, {ID: 5, Name: "WEB Rip"}}})

	plan, err := PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]string{3: StatusProtected, 4: StatusRenamed, 5: StatusRenamed} {
		if got := itemByID(plan, AxisType, id).Status; got != want {
			t.Errorf("id %d: status %q, want %q", id, got, want)
		}
	}
	if x, _ := termByID(ApplyImport(cur.Taxonomy, plan), AxisType, 3); x.Name != "Encode" || x.Origin != OriginDiscovered {
		t.Errorf("site-confirmed name was changed without request: %+v", x)
	}

	plan, err = PlanImport(cur, doc, testSite, map[OverwriteKey]bool{{Axis: AxisType, ID: 3}: true})
	if err != nil {
		t.Fatal(err)
	}
	if it := itemByID(plan, AxisType, 3); it.Status != StatusRenamed || !it.OverwritesProtected {
		t.Errorf("id 3 requested: %+v", it)
	}
	if len(plan.Warnings) != 0 {
		t.Errorf("request matched a protected one, there should be no warning: %v", plan.Warnings)
	}
	if x, _ := termByID(ApplyImport(cur.Taxonomy, plan), AxisType, 3); x.Name != "Re-encode" || x.Origin != OriginImported {
		t.Errorf("id 3 after request: %+v", x)
	}
}

// Closed failure: a source this code doesn't recognize — written by another version or by hand in
// the database — is protected in import and discovery. The error must default to not modifying it.
func TestUnknownSourceIsProtected(t *testing.T) {
	cur := profileOf(Taxonomy{AxisType: {{ID: 3, Name: "Encode", Origin: "curation"}, {ID: 4, Name: "Remux", Origin: ""}}})

	doc := docOf(testSite, Taxonomy{AxisType: {{ID: 3, Name: "Re-encode"}, {ID: 4, Name: "Remux HD"}}})
	plan, err := PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{3, 4} {
		if got := itemByID(plan, AxisType, id).Status; got != StatusProtected {
			t.Errorf("import, id %d with unknown origin: %q, want protected", id, got)
		}
	}

	// Discovery (merge.go): the proposal marks it as protected and Merge does not rename.
	p := Propose(cur.Taxonomy, map[string][]Observed{AxisType: {{ID: 3, Name: "Re-encode", Seen: 2}}})
	if it := p.Items[AxisType][0]; it.Status != StatusRenamed || !it.Protected {
		t.Errorf("Propose with unknown origin = %+v, want renamed + protected", it)
	}
	next, applied, skipped, err := Merge(cur.Taxonomy, []ApplyItem{{Axis: AxisType, ID: 3, Name: "Re-encode", Origin: OriginDiscovered}})
	if err != nil || len(applied) != 0 || len(skipped) != 1 || skipped[0].Reason != ReasonProtected {
		t.Errorf("Merge with unknown origin: applied=%v skipped=%v err=%v", applied, skipped, err)
	}
	if x, _ := termByID(next, AxisType, 3); x.Name != "Encode" {
		t.Errorf("the Merge renamed a term with unknown origin: %+v", x)
	}
}

func hasItemWarning(plan ImportPlan, axis string, id int, part string) bool {
	return hasWarning(itemByID(plan, axis, id).Warnings, part)
}

// A new term repeating the name (ignoring case and accent) of one the operator has would OBSCURE the
// correct one in the dashboard, which matches by name — the warning appears on the new term's line.
func TestImport_WarnsNameCollidingWithProfile(t *testing.T) {
	cur := profileOf(Taxonomy{AxisCategory: {{ID: 2, Name: "Series", Kind: KindTV, Origin: OriginManual}}})
	doc := docOf(testSite, Taxonomy{AxisCategory: {{ID: 1, Name: "Séries", Kind: KindTV}, {ID: 7, Name: "Games", Kind: KindGame}}})

	plan, err := PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasItemWarning(plan, AxisCategory, 1, "as that of id 2 «Series»") {
		t.Errorf("missing duplicate name warning on id 1: %+v", itemByID(plan, AxisCategory, 1))
	}
	if w := itemByID(plan, AxisCategory, 7).Warnings; len(w) != 0 {
		t.Errorf("id 7 collides with nothing, but has warnings: %v", w)
	}
	// id 1 would also become the first "Series" category (previously: id 2) — the second way to obscure.
	if !hasItemWarning(plan, AxisCategory, 1, "first category with the role «Series»") {
		t.Errorf("missing role warning on id 1: %+v", itemByID(plan, AxisCategory, 1))
	}
	if !hasWarning(plan.Warnings, "1 term(s) from the file collide") {
		t.Errorf("missing summary in the plan: %v", plan.Warnings)
	}
}

// The name a RENAME creates also collides: the rename cannot push to a name that another id already uses.
func TestImport_WarnsNameMadeDuplicateByRename(t *testing.T) {
	cur := profileOf(Taxonomy{AxisType: {
		{ID: 3, Name: "Encode", Origin: OriginSeed},
		{ID: 9, Name: "WEB-DL", Origin: OriginManual},
	}})
	doc := docOf(testSite, Taxonomy{AxisType: {{ID: 3, Name: "web-dl"}}})
	plan, err := PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if it := itemByID(plan, AxisType, 3); it.Status != StatusRenamed || !hasItemWarning(plan, AxisType, 3, "as that of id 9 «WEB-DL»") {
		t.Errorf("id 3 = %+v", it)
	}
}

// Multiple categories with the same role are normal (series and anime are "tv"). It only warns when the
// term becomes the FIRST — the one the dashboard pre-fills — for a role that already had an owner.
func TestImport_RoleOnlyNotifiesWhenDefaultOwnerChanges(t *testing.T) {
	cur := profileOf(Taxonomy{AxisCategory: {
		{ID: 2, Name: "Series", Kind: KindTV, Origin: OriginSeed},
		{ID: 3, Name: "Animes", Kind: KindTV, Origin: OriginSeed},
	}})

	// id 20: another "tv" after the existing ones — the owner (id 2) remains the same.
	late, _ := PlanImport(cur, docOf(testSite, Taxonomy{AxisCategory: {{ID: 20, Name: "Novelas", Kind: KindTV}}}), testSite, nil)
	if w := itemByID(late, AxisCategory, 20).Warnings; len(w) != 0 || len(late.Warnings) != 0 {
		t.Errorf("an extra tv after the owner does not change the pattern, but it warned: item=%v plan=%v", w, late.Warnings)
	}

	// id 1: gets ahead of the owner — the dashboard would start pre-filling "tv" with it.
	early, _ := PlanImport(cur, docOf(testSite, Taxonomy{AxisCategory: {{ID: 1, Name: "Docs de TV", Kind: KindTV}}}), testSite, nil)
	if !hasItemWarning(early, AxisCategory, 1, "it used to be id 2 «Series»") {
		t.Errorf("missing warning for id 1: %+v", itemByID(early, AxisCategory, 1))
	}

	// profile without any "tv" category: whoever enters sets the default, there is nothing to obscure.
	fresh, _ := PlanImport(Empty(), docOf(testSite, Taxonomy{AxisCategory: {{ID: 1, Name: "Series", Kind: KindTV}, {ID: 2, Name: "Animes", Kind: KindTV}}}), testSite, nil)
	if len(fresh.Warnings) != 0 {
		t.Errorf("empty profile has no pattern to obfuscate: %v", fresh.Warnings)
	}
}

// The calculation is about the state the axis would END UP in: what the plan leaves as is counts, and what
// it changes no longer counts under the old name.
func TestImport_CollisionWithProtectedTermConsidersFinalState(t *testing.T) {
	cur := profileOf(Taxonomy{AxisType: {{ID: 3, Name: "Encode", Origin: OriginManual}}})
	file := docOf(testSite, Taxonomy{AxisType: {{ID: 3, Name: "Recode"}, {ID: 4, Name: "encode"}}})

	// id 3 stays "Encode" (protected); the new id 4 is named "encode" — it collides with what STAYS.
	plan, _ := PlanImport(cur, file, testSite, nil)
	if !hasItemWarning(plan, AxisType, 4, "as that of id 3 «Encode»") {
		t.Errorf("id 4 = %+v", itemByID(plan, AxisType, 4))
	}
	// If the operator REQUESTS to overwrite id 3, it stops being "Encode" and the collision disappears.
	plan, _ = PlanImport(cur, file, testSite, map[OverwriteKey]bool{{Axis: AxisType, ID: 3}: true})
	if w := itemByID(plan, AxisType, 4).Warnings; len(w) != 0 {
		t.Errorf("with id 3 renamed there is no collision, but it warned: %v", w)
	}
}

// Parity with the Portal: NormalizeLabel is the normalizeLabel from lib/taxonomy.ts (the same examples
// are in taxonomy.test.ts). Both sides must agree, or the server rejects/warns about what the
// dashboard would treat as different — or lets pass what the dashboard wouldn't distinguish.
func TestNormalizeLabel(t *testing.T) {
	cases := map[string]string{
		"  Séries   HD ":                    "series hd",
		"HQs e Mangás":                      "hqs e mangas",
		"  AÇÃO   &  Aventura ":             "acao & aventura",
		"Músicas":                           "musicas",
		"WEB-DL":                            "web-dl",
		"1080p":                             "1080p",
		"":                                  "",
		"   ":                               "",
		"Series":                            "series",
		"Se" + string(rune(0x301)) + "ries": "series", // "e" + combining accent: the same as the composed one
	}
	for in, want := range cases {
		if got := NormalizeLabel(in); got != want {
			t.Errorf("NormalizeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
