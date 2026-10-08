package trackerprofile

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// The name of a term is a label, but it comes from outside (a site's catalog, editing, import). Each
// line in the "reject" part is a way a hostile label can cause damage: reversing text on the
// screen, making two "equal" terms distinct, breaking the log/terminal, or overflowing the screen.
func TestCleanName(t *testing.T) {
	ok := []struct{ in, want string }{
		{"Filmes", "Filmes"},
		{"  Comics   and  Mangas ", "Comics and Mangas"}, // collapsed spaces
		{"Movies\nSeries", "Movies Series"},              // newline becomes space, doesn't pass
		{"WEB-DL", "WEB-DL"},
		{"4K 🎬", "4K 🎬"}, // emoji is symbol, not control
		{strings.Repeat("a", MaxNameRunes), strings.Repeat("a", MaxNameRunes)},
	}
	for _, tc := range ok {
		got, err := CleanName(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("CleanName(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}

	bad := map[string]string{
		"empty":               "",
		"only spaces":         "  \t \n ",
		"NUL":                 "Mov\x00ies",
		"escape de terminal":  "\x1b[31mMovies",
		"direction override":  "Movies\u202eSeires", // U+202E inverts the text on the screen
		"zero-width":          "Fil\u200bmes",       // two "Movies" that are not equal
		"BOM in the middle":   "Fil\ufeffmes",
		"private use":         "Mov\ue000ies",
		"invalid UTF-8":       "Fil\xffmes",
		"too long":            strings.Repeat("a", MaxNameRunes+1),
		"C1 control":          "Fil\u0085\u009bmes", // NEL is space (collapses), CSI is control
		"paragraph separator": " \u0000",
	}
	for name, in := range bad {
		if got, err := CleanName(in); err == nil {
			t.Errorf("%s: CleanName(%q) accepted (%q); should reject", name, in, got)
		}
	}
}

func TestValidateInput(t *testing.T) {
	good := map[string][]TermInput{
		AxisCategory: {{ID: 1, Name: "Filmes", Kind: KindMovie}, {ID: 5, Name: "Games", Kind: KindGame}},
		AxisType:     {{ID: 4, Name: "WEB-DL"}},
	}
	out, err := ValidateInput(good)
	if err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}
	if _, has := out[AxisResolution]; has {
		t.Error("axis missing from input appeared in output — the edit must not touch what was not sent")
	}

	tooMany := make([]TermInput, MaxTermsPerAxis+1)
	for i := range tooMany {
		tooMany[i] = TermInput{ID: i + 1, Name: "x"}
	}

	cases := []struct {
		name     string
		in       map[string][]TermInput
		wantPath string
	}{
		{"unknown axis", map[string][]TermInput{"audio": {{ID: 1, Name: "x"}}}, "taxonomy"},
		{"hostile axis", map[string][]TermInput{"../../etc": {{ID: 1, Name: "x"}}}, "taxonomy"},
		{"id zero", map[string][]TermInput{AxisType: {{ID: 0, Name: "x"}}}, "taxonomy.type[0].id"},
		{"negative id", map[string][]TermInput{AxisType: {{ID: -4, Name: "x"}}}, "taxonomy.type[0].id"},
		{"huge id", map[string][]TermInput{AxisType: {{ID: MaxTermID + 1, Name: "x"}}}, "taxonomy.type[0].id"},
		{"id overflows int32", map[string][]TermInput{AxisType: {{ID: 1 << 40, Name: "x"}}}, "taxonomy.type[0].id"},
		{"duplicate id", map[string][]TermInput{AxisType: {{ID: 3, Name: "a"}, {ID: 3, Name: "b"}}}, "taxonomy.type[1].id"},
		{"empty name", map[string][]TermInput{AxisType: {{ID: 1, Name: "  "}}}, "taxonomy.type[0].name"},
		{"hostile name", map[string][]TermInput{AxisType: {{ID: 1, Name: "a\u202eb"}}}, "taxonomy.type[0].name"},
		{"kind outside category", map[string][]TermInput{AxisType: {{ID: 1, Name: "x", Kind: KindMovie}}}, "taxonomy.type[0].kind"},
		{"unknown kind", map[string][]TermInput{AxisCategory: {{ID: 1, Name: "x", Kind: "<script>"}}}, "taxonomy.category[0].kind"},
		{"too many terms", map[string][]TermInput{AxisType: tooMany}, "taxonomy.type"},
	}
	for _, tc := range cases {
		_, err := ValidateInput(tc.in)
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: error = %v, want *ValidationError", tc.name, err)
			continue
		}
		if ve.Path != tc.wantPath {
			t.Errorf("%s: path = %q, want %q (%s)", tc.name, ve.Path, tc.wantPath, ve.Msg)
		}
	}
}

// The hostile value that arrives in an error cannot return whole: the "unknown" axis can be
// megabytes of garbage, and the message goes to the Portal and to the log.
func TestValidateInput_NoFullGarbageInMessage(t *testing.T) {
	_, err := ValidateInput(map[string][]TermInput{strings.Repeat("A", 10_000): {}})
	if err == nil || len(err.Error()) > 200 {
		t.Fatalf("error = %d bytes, want a short message", len(err.Error()))
	}
}

func TestApplyEdit_DecidesOriginByWhatChanged(t *testing.T) {
	cur := Taxonomy{
		AxisCategory: {
			{ID: 1, Name: "Filmes", Kind: KindMovie, Origin: OriginSeed},
			{ID: 2, Name: "Series", Origin: OriginDiscovered},
			{ID: 3, Name: "Animes", Origin: OriginSeed},
		},
		AxisType: {{ID: 4, Name: "WEB-DL", Origin: OriginDiscovered}},
	}
	in := map[string][]TermInput{AxisCategory: {
		{ID: 1, Name: "Filmes", Kind: KindMovie}, // equal: keeps seed
		{ID: 2, Name: "Series", Kind: KindTV},    // only the kind changed: keeps discovered
		{ID: 3, Name: "Desenhos"},                // renamed manually: manual
		{ID: 9, Name: "New"},                     // created manually: manual
	}}
	got := ApplyEdit(cur, in)

	want := []Term{
		{1, "Filmes", KindMovie, OriginSeed},
		{2, "Series", KindTV, OriginDiscovered},
		{3, "Desenhos", "", OriginManual},
		{9, "New", "", OriginManual},
	}
	if !reflect.DeepEqual(got[AxisCategory], want) {
		t.Errorf("category:\n got %+v\nwant %+v", got[AxisCategory], want)
	}
	if !reflect.DeepEqual(got[AxisType], cur[AxisType]) {
		t.Error("the type axis was not sent and must remain exactly as it was")
	}
	if got[AxisResolution] == nil {
		t.Error("resolution axis missing: must exist as an empty list, never nil")
	}
}

// Present and empty axis REPLACES (deletes); absent axis doesn't touch it. This is what prevents a
// client from deleting the entire taxonomy by forgetting a field.
func TestApplyEdit_EmptyErasesAndAbsentDoesNotTouch(t *testing.T) {
	cur := Taxonomy{
		AxisCategory: {{ID: 1, Name: "Filmes", Origin: OriginSeed}},
		AxisType:     {{ID: 4, Name: "WEB-DL", Origin: OriginSeed}},
	}
	got := ApplyEdit(cur, map[string][]TermInput{AxisCategory: {}})
	if len(got[AxisCategory]) != 0 {
		t.Errorf("category = %+v, want empty (was sent empty)", got[AxisCategory])
	}
	if len(got[AxisType]) != 1 {
		t.Errorf("type = %+v, want intact (not sent)", got[AxisType])
	}
}

// The list comes out sorted by id, as the Portal has always shown — the order of the <option> is part
// of the parity contract with what was hardcoded in the frontend.
func TestApplyEdit_SortsByID(t *testing.T) {
	got := ApplyEdit(nil, map[string][]TermInput{AxisType: {{ID: 6, Name: "f"}, {ID: 2, Name: "b"}, {ID: 4, Name: "d"}}})
	var ids []int
	for _, tm := range got[AxisType] {
		ids = append(ids, tm.ID)
	}
	if !reflect.DeepEqual(ids, []int{2, 4, 6}) {
		t.Errorf("ids = %v, want [2 4 6]", ids)
	}
}

func TestEmpty_AllAxesAsList(t *testing.T) {
	raw, err := json.Marshal(Empty())
	if err != nil {
		t.Fatal(err)
	}
	for _, axis := range Axes {
		if !strings.Contains(string(raw), `"`+axis+`":[]`) {
			t.Errorf("empty profile = %s; axis %s must be [] (null breaks iteration)", raw, axis)
		}
	}
	if !strings.Contains(string(raw), `"family":"unit3d"`) || !strings.Contains(string(raw), `"schema_version":1`) {
		t.Errorf("empty profile without family/schema_version: %s", raw)
	}
}

// ADR-037, layer 3 out BY DESIGN: no type that makes up the profile has a field that
// can carry operator credentials or config. The test traverses the TYPE TREE (not
// a filled example): whoever adds an `api_token` field to the profile — which the export from
// #197 would serialize — breaks here, before an export exists to leak it.
func TestProfileHasNoCredentialOrOperatorConfigField(t *testing.T) {
	prohibited := []string{
		"token", "secret", "passkey", "password", "announce", "api_url", "apiurl",
		"anon", "modq", "credential", "key",
	}
	var visit func(rt reflect.Type, path string)
	seen := map[reflect.Type]bool{}
	visit = func(rt reflect.Type, path string) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map || rt.Kind() == reflect.Array {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			name := strings.ToLower(f.Name + " " + f.Tag.Get("json"))
			for _, p := range prohibited {
				if strings.Contains(name, p) {
					t.Errorf("%s.%s (%q) looks like operator credential/config — belongs in layer 3, not the profile",
						path, f.Name, f.Tag.Get("json"))
				}
			}
			visit(f.Type, path+"."+f.Name)
		}
	}
	for _, rt := range []reflect.Type{
		reflect.TypeOf(Profile{}), reflect.TypeOf(Term{}), reflect.TypeOf(TermInput{}),
		reflect.TypeOf(ProposalItem{}), reflect.TypeOf(Proposal{}), reflect.TypeOf(ApplyItem{}),
	} {
		visit(rt, rt.Name())
	}
}
