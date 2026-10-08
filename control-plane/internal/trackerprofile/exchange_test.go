package trackerprofile

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// ---- What the export cannot load -------------------------------------------------

// Words that indicate operator credential or config (layer 3, ADR-037).
var forbiddenFieldWords = []string{
	"token", "secret", "passkey", "password", "announce", "api_url", "apiurl",
	"anon", "modq", "credential", "key",
}

var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

// exportViolations traverses the root TYPE TREE and returns everything an export must not
// have: a field with a credential name, an opaque field (interface, bytes, RawMessage) — which
// would load anything without a name — and a type with its own MarshalJSON, which would emit what
// it wanted without reflection seeing it. It traverses types, not a filled example: what
// fails is the EXISTENCE of the field.
func exportViolations(root reflect.Type) []string {
	var out []string
	seen := map[reflect.Type]bool{}
	var visit func(rt reflect.Type, path string)
	visit = func(rt reflect.Type, path string) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map || rt.Kind() == reflect.Array {
			if rt.Kind() == reflect.Slice && rt.Elem().Kind() == reflect.Uint8 {
				out = append(out, path+": opaque bytes field (would load anything unnamed)")
				return
			}
			rt = rt.Elem()
		}
		switch rt.Kind() {
		case reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
			out = append(out, fmt.Sprintf("%s: campo opaco do tipo %s", path, rt.Kind()))
			return
		case reflect.Struct:
		default:
			return
		}
		if seen[rt] {
			return
		}
		seen[rt] = true
		if rt.Implements(jsonMarshalerType) || reflect.PointerTo(rt).Implements(jsonMarshalerType) {
			out = append(out, path+": custom MarshalJSON — reflection doesn't see what it emits")
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			name := strings.ToLower(f.Name + " " + f.Tag.Get("json"))
			for _, w := range forbiddenFieldWords {
				if strings.Contains(name, w) {
					out = append(out, fmt.Sprintf("%s.%s (%q) parece credencial/config do operador (camada 3)", path, f.Name, f.Tag.Get("json")))
				}
			}
			visit(f.Type, path+"."+f.Name)
		}
	}
	visit(root, root.Name())
	return out
}

// Structural test for #197: the type the export serializes has — and cannot gain without
// breaking this test — no layer 3 fields. It traverses the entire Document, including the
// embedded Profile and whatever it gains going forward (name format, rules,
// description, image host).
func TestExportedDocumentHasNoCredentialField(t *testing.T) {
	for _, v := range exportViolations(reflect.TypeOf(Document{})) {
		t.Error(v)
	}
}

// The test of the test: a verifier that never fails proves nothing. Each type below is a
// way for a secret to enter the export, and the verifier must catch them all.
func TestStructuralVerifierCatchesTheLeaks(t *testing.T) {
	type comToken struct {
		Name     string
		APIToken string `json:"api_token"`
	}
	type withNestedPasskey struct {
		Site struct {
			Announce string `json:"announce"`
		}
	}
	type withOpaque struct{ Extra any }
	type comBytes struct{ Blob []byte }
	type comRaw struct{ Raw json.RawMessage }
	type withOpaqueMap struct{ Config map[string]any }
	type comAPIURL struct {
		URL string `json:"api_url"`
	}
	type withGenericKey struct {
		SigningKey string
	}
	bad := map[string]any{
		"api_token":       comToken{},
		"nested announce": withNestedPasskey{},
		"interface":       withOpaque{},
		"bytes":           comBytes{},
		"RawMessage":      comRaw{},
		"map[string]any":  withOpaqueMap{},
		"api_url":         comAPIURL{},
		"key":             withGenericKey{},
		"MarshalJSON":     comMarshaler{},
	}
	for name, v := range bad {
		if len(exportViolations(reflect.TypeOf(v))) == 0 {
			t.Errorf("%s: checker did not reject a leaking type", name)
		}
	}
	type clean struct {
		Name string `json:"name"`
		Host string `json:"host,omitempty"`
		IDs  []int  `json:"ids"`
	}
	if v := exportViolations(reflect.TypeOf(clean{})); len(v) != 0 {
		t.Errorf("clean type failed: %v", v)
	}
}

type comMarshaler struct{ N int }

func (comMarshaler) MarshalJSON() ([]byte, error) { return []byte(`{"segredo":"x"}`), nil }

// A new Profile section is exported automatically (the Document embeds the Profile), but is only
// IMPORTABLE when it has validation and a merge rule. This test reminds whoever adds
// a section: without registering it in handledSections — and thus without validating it — the import rejects it
// at runtime and the suite fails here.
func TestEveryProfileSectionIsHandledOnImport(t *testing.T) {
	for _, name := range sectionNames() {
		if !handledSections[name] {
			t.Errorf("section %q of Profile is exported automatically, but import does not know how to validate or apply it. "+
				"Write the validation in Document.validated and the rule in PlanImport/ApplyImport, and only then register it in "+
				"handledSections. Until then import rejects it (fail closed).", name)
		}
	}
	for name := range handledSections {
		if !contains(sectionNames(), name) {
			t.Errorf("handledSections lists %q, which is not a Profile section", name)
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Fail closed at runtime: a section that the import doesn't know and that comes FILLED is rejected.
func TestSectionWithoutHandlingRejected(t *testing.T) {
	type future struct {
		Family        string            `json:"family"`
		SchemaVersion int               `json:"schema_version"`
		Taxonomy      Taxonomy          `json:"taxonomy"`
		Description   map[string]string `json:"description"`
	}
	handled := map[string]bool{"taxonomy": true}
	if got := unhandledSectionOf(future{Taxonomy: Taxonomy{"type": nil}}, handled); got != "" {
		t.Errorf("empty section rejected: %q", got)
	}
	if got := unhandledSectionOf(future{Description: map[string]string{"mediainfo": "[pre]"}}, handled); got != "description" {
		t.Errorf("populated and unhandled section = %q, want description", got)
	}
}

// ---- Export ---------------------------------------------------------------------------

func fullProfile() Profile {
	return Profile{Family: FamilyUNIT3D, SchemaVersion: SchemaVersion, Taxonomy: Taxonomy{
		AxisCategory: {
			{ID: 2, Name: "Séries", Kind: KindTV, Origin: OriginSeed},
			{ID: 1, Name: "Filmes", Kind: KindMovie, Origin: OriginManual},
		},
		AxisType:       {{ID: 3, Name: "Encode", Origin: OriginDiscovered}, {ID: 9, Name: "HQs & Mangás", Origin: OriginImported}},
		AxisResolution: {{ID: 3, Name: "1080p", Origin: OriginSeed}},
	}}
}

func TestExport_DoesNotCarryOriginAndReadsInOrder(t *testing.T) {
	doc := NewDocument("Exemplo", "tracker.exemplo.org:2710", fullProfile())
	raw, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, local := range []string{"origin", "manual", "discovered", "imported"} {
		if strings.Contains(out, local) {
			t.Errorf("the export carries %q (local state):\n%s", local, out)
		}
	}
	if strings.Contains(out, `\u0026`) {
		t.Errorf("export escaped & (unreadable in PR review):\n%s", out)
	}
	if !strings.HasSuffix(out, "}\n") {
		t.Errorf("file does not end with newline: %q", out[len(out)-5:])
	}
	// envelope first, then the profile: format, site, family, schema_version, taxonomy.
	pos := -1
	for _, k := range []string{`"format"`, `"site"`, `"family"`, `"schema_version"`, `"taxonomy"`} {
		i := strings.Index(out, k)
		if i <= pos {
			t.Errorf("key %s out of order in:\n%s", k, out)
		}
		pos = i
	}
	if doc.Site.Host != "tracker.exemplo.org" {
		t.Errorf("host = %q, want hostname without port", doc.Site.Host)
	}
	// The source profile is unchanged: the export copies, it does not zero the original.
	p := fullProfile()
	_ = NewDocument("x", "", p)
	if p.Taxonomy[AxisCategory][1].Origin != OriginManual {
		t.Error("NewDocument zeroed the ORIGIN profile origin — must copy")
	}
	// Sorted by id.
	if doc.Taxonomy[AxisCategory][0].ID != 1 {
		t.Errorf("categories out of order: %+v", doc.Taxonomy[AxisCategory])
	}
}

// The export always generates a file that the import itself accepts — even with name and host outside
// the rules (the name is the free label the operator gave to the tracker).
func TestExport_RoundTripWithDirtyNameAndHost(t *testing.T) {
	cases := []struct{ name, host string }{
		{"Exemplo", "tracker.exemplo.org"},
		{strings.Repeat("N", 200), "tracker.exemplo.org"},
		{"Com\xe2\x80\xaeBidi\x00NUL\xe2\x80\x8bZW", "Tracker.Exemplo.ORG."},
		{"   ", "tracker.exemplo.org/announce/PASSKEY-SECRETA"},
		{"", "user:PASSKEY-SECRETA@tracker.exemplo.org"},
		{"x", "[2001:db8::1]:8080"},
		{"x", "https://tracker.exemplo.org"},
	}
	for _, c := range cases {
		raw, err := NewDocument(c.name, c.host, fullProfile()).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "PASSKEY") {
			t.Errorf("host %q leaked to file:\n%s", c.host, raw)
		}
		if _, err := ParseDocument(raw); err != nil {
			t.Errorf("name=%q host=%q: export fails its own import: %v\n%s", c.name, c.host, err, raw)
		}
	}
}

// The file stamps the format version the code emits — not the database row's, which
// might be older: the document is rendered now, with the sections the code knows.
func TestExport_StampsCodeFormatVersion(t *testing.T) {
	p := fullProfile()
	p.SchemaVersion = 0
	if doc := NewDocument("x", "", p); doc.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %d, want %d", doc.SchemaVersion, SchemaVersion)
	}
}

// Pins the BYTES of the exchange file. It's the same example in ADR-037 and docs/13 - API.md:
// if this test breaks, the format changed — and an already shared file stops matching what
// the code generates. A shape change is a SchemaVersion change (and, if the old import wouldn't
// understand it, a MinSchemaVersion/upgrade one): decide this BEFORE updating the fixture.
func TestExport_StableFormat(t *testing.T) {
	p := Profile{Family: FamilyUNIT3D, SchemaVersion: SchemaVersion, Taxonomy: Taxonomy{
		AxisCategory: {
			{ID: 1, Name: "Filmes", Kind: KindMovie, Origin: OriginSeed},
			{ID: 2, Name: "Séries", Kind: KindTV, Origin: OriginManual},
			{ID: 5, Name: "Games", Kind: KindGame, Origin: OriginDiscovered},
		},
		AxisType:       {{ID: 3, Name: "Encode", Origin: OriginDiscovered}, {ID: 4, Name: "WEB-DL", Origin: OriginImported}},
		AxisResolution: {{ID: 3, Name: "1080p", Origin: OriginSeed}, {ID: 2, Name: "2160p", Origin: OriginSeed}},
	}}
	raw, err := NewDocument("Exemplo", "tracker.exemplo.org:2710", p).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	const golden = `{
  "format": "seedgrid.tracker-profile",
  "site": {
    "name": "Exemplo",
    "host": "tracker.exemplo.org"
  },
  "family": "unit3d",
  "schema_version": 1,
  "taxonomy": {
    "category": [
      {
        "id": 1,
        "name": "Filmes",
        "kind": "movie"
      },
      {
        "id": 2,
        "name": "Séries",
        "kind": "tv"
      },
      {
        "id": 5,
        "name": "Games",
        "kind": "game"
      }
    ],
    "resolution": [
      {
        "id": 2,
        "name": "2160p"
      },
      {
        "id": 3,
        "name": "1080p"
      }
    ],
    "type": [
      {
        "id": 3,
        "name": "Encode"
      },
      {
        "id": 4,
        "name": "WEB-DL"
      }
    ]
  }
}
`
	if string(raw) != golden {
		t.Errorf("exported file changed shape:\n%s\nwant:\n%s", raw, golden)
	}
}

func TestExport_RoundTripPreservesContent(t *testing.T) {
	raw, _ := NewDocument("Exemplo", "tracker.exemplo.org", fullProfile()).Marshal()
	doc, err := ParseDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := withoutLocalState(fullProfile().Taxonomy)
	if !reflect.DeepEqual(doc.Taxonomy, want) {
		t.Errorf("taxonomy after round-trip:\n got %+v\nwant %+v", doc.Taxonomy, want)
	}
	if doc.Format != FormatID || doc.Family != FamilyUNIT3D || doc.SchemaVersion != SchemaVersion {
		t.Errorf("envelope = %+v", doc)
	}
}

func TestFilename(t *testing.T) {
	cases := map[string]string{
		"tracker.exemplo.org": "seedgrid-profile-unit3d-tracker-exemplo-org.json",
		"":                    "seedgrid-profile-unit3d-tracker.json",
	}
	for host, want := range cases {
		d := NewDocument("", host, Empty())
		d.Site.Name = ""
		if got := d.Filename(); got != want {
			t.Errorf("host %q: %q, want %q", host, got, want)
		}
	}
	// Without host, the name gets in — and no character that the header treats as structure.
	d := Document{Site: Site{Name: "a\"b\r\nContent-Type: x/y; ../é"}, Profile: Profile{Family: FamilyUNIT3D}}
	got := d.Filename()
	for _, r := range strings.TrimSuffix(strings.TrimPrefix(got, "seedgrid-profile-unit3d-"), ".json") {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			t.Errorf("filename %q with character %q", got, r)
		}
	}
	if long := (Document{Site: Site{Name: strings.Repeat("a", 500)}, Profile: Profile{Family: FamilyUNIT3D}}).Filename(); len(long) > 80 {
		t.Errorf("filename of %d characters", len(long))
	}
}

// ---- Import: the file is hostile content ------------------------------------------------

func validDoc() map[string]any {
	return map[string]any{
		"format":         FormatID,
		"site":           map[string]any{"name": "Exemplo", "host": "tracker.exemplo.org"},
		"family":         "unit3d",
		"schema_version": 1,
		"taxonomy": map[string]any{
			"category":   []any{map[string]any{"id": 1, "name": "Filmes", "kind": "movie"}},
			"type":       []any{map[string]any{"id": 3, "name": "Encode"}},
			"resolution": []any{map[string]any{"id": 3, "name": "1080p"}},
		},
	}
}

func mutate(f func(m map[string]any)) string {
	m := validDoc()
	f(m)
	raw, _ := json.Marshal(m)
	return string(raw)
}

func termsWith(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = map[string]any{"id": i + 1, "name": "x"}
	}
	return out
}

func TestParseDocument_Valid(t *testing.T) {
	raw, _ := json.Marshal(validDoc())
	doc, err := ParseDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Site.Name != "Exemplo" || doc.Taxonomy[AxisCategory][0].Kind != KindMovie {
		t.Errorf("doc = %+v", doc)
	}
	// Windows editor BOM is accepted; the content is the same.
	if _, err := ParseDocument(append([]byte("\xEF\xBB\xBF"), raw...)); err != nil {
		t.Errorf("with BOM: %v", err)
	}
	// Uppercase host is normalized; missing axis is not an error.
	d2, err := ParseDocument([]byte(mutate(func(m map[string]any) {
		m["site"] = map[string]any{"name": "  Sama   ritano ", "host": "TRACKER.Exemplo.org"}
		delete(m["taxonomy"].(map[string]any), "resolution")
	})))
	if err != nil {
		t.Fatal(err)
	}
	if d2.Site.Name != "Sama ritano" || d2.Site.Host != "tracker.exemplo.org" {
		t.Errorf("site not normalized: %+v", d2.Site)
	}
	if d2.Taxonomy[AxisResolution] == nil {
		t.Error("missing axis must exist as empty list in clean document")
	}
}

func TestParseDocument_HostilePayloads(t *testing.T) {
	bigName := strings.Repeat("x", 1<<20)
	cases := []struct {
		name string
		body string
		code string // "" = invalid_profile
	}{
		{"not JSON", `{taxonomy`, ""},
		{"empty", ``, ""},
		{"only space", "  \n ", ""},
		{"array no topo", `[]`, ""},
		{"null", `null`, ""},
		{"number", `42`, ""},
		{"data after document", string(mustJSON(validDoc())) + ` {}`, ""},
		{"garbage after document", string(mustJSON(validDoc())) + ` garbage`, ""},
		{"JSON truncado", `{"format":"seedgrid.tracker-profile","site":{"name":"x"`, ""},
		{"invalid UTF-8", "{\"format\":\"seedgrid.tracker-profile\",\"site\":{\"name\":\"a\xffb\"}}", ""},

		{"campo desconhecido no topo", mutate(func(m map[string]any) { m["api_token"] = "x" }), ""},
		{"campo desconhecido no site", mutate(func(m map[string]any) { m["site"].(map[string]any)["announce"] = "http://x" }), ""},
		{"campo desconhecido no termo", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a", "url": "http://interno/"}}
		}), ""},
		{"origin in term (is local)", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a", "origin": "manual"}}
		}), ""},
		{"eixo desconhecido", mutate(func(m map[string]any) { m["taxonomy"].(map[string]any)["audio"] = termsWith(1) }), ""},
		{"axis with uppercase", mutate(func(m map[string]any) { m["taxonomy"].(map[string]any)["Category"] = termsWith(1) }), ""},
		{"eixo hostil", mutate(func(m map[string]any) { m["taxonomy"].(map[string]any)["../../etc"] = termsWith(1) }), ""},

		{"format errado", mutate(func(m map[string]any) { m["format"] = "prowlarr.definition" }), ""},
		{"format ausente", mutate(func(m map[string]any) { delete(m, "format") }), ""},
		{"schema_version futura", mutate(func(m map[string]any) { m["schema_version"] = 2 }), CodeUnsupportedVersion},
		{"schema_version enorme", mutate(func(m map[string]any) { m["schema_version"] = 999999 }), CodeUnsupportedVersion},
		{"schema_version zero", mutate(func(m map[string]any) { m["schema_version"] = 0 }), ""},
		{"schema_version negativa", mutate(func(m map[string]any) { m["schema_version"] = -1 }), ""},
		{"schema_version string", mutate(func(m map[string]any) { m["schema_version"] = "1" }), ""},
		{"fractional schema_version", mutate(func(m map[string]any) { m["schema_version"] = 1.5 }), ""},
		{"schema_version ausente", mutate(func(m map[string]any) { delete(m, "schema_version") }), ""},
		{"family errada", mutate(func(m map[string]any) { m["family"] = "gazelle" }), ""},
		{"family ausente", mutate(func(m map[string]any) { delete(m, "family") }), ""},
		{"family hostil", mutate(func(m map[string]any) { m["family"] = strings.Repeat("A", 5000) }), ""},

		{"site ausente", mutate(func(m map[string]any) { delete(m, "site") }), ""},
		{"empty site.name", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": " "} }), ""},
		{"site.name with bidi", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": "a\xe2\x80\xaeb"} }), ""},
		{"site.name gigante", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": bigName} }), CodeTooLarge},
		{"site.host is URL", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": "x", "host": "https://x.org/a"} }), ""},
		{"site.host with credential", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": "x", "host": "u:p@x.org"} }), ""},
		{"site.host with port", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": "x", "host": "x.org:80"} }), ""},
		{"site.host with space", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": "x", "host": "x .org"} }), ""},
		{"site.host longo", mutate(func(m map[string]any) {
			m["site"] = map[string]any{"name": "x", "host": strings.Repeat("a", 300) + ".org"}
		}), ""},

		{"id negativo", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": -1, "name": "a"}}
		}), ""},
		{"id zero", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 0, "name": "a"}}
		}), ""},
		{"id acima da faixa", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": MaxTermID + 1, "name": "a"}}
		}), ""},
		{"id estoura int64", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d","schema_version":1,` +
			`"taxonomy":{"type":[{"id":99999999999999999999,"name":"a"}]}}`, ""},
		{"fractional id", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d","schema_version":1,` +
			`"taxonomy":{"type":[{"id":1.5,"name":"a"}]}}`, ""},
		{"huge scientific notation id", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d","schema_version":1,` +
			`"taxonomy":{"type":[{"id":1e999,"name":"a"}]}}`, ""},
		{"id string", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": "1", "name": "a"}}
		}), ""},
		{"id repetido", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a"}, map[string]any{"id": 1, "name": "b"}}
		}), ""},
		{"empty name", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": ""}}
		}), ""},
		{"name with 65 characters", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": strings.Repeat("x", 65)}}
		}), ""},
		{"huge name (1 MiB)", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": bigName}}
		}), CodeTooLarge},
		{"name with direction override", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a\xe2\x80\xaeb"}}
		}), ""},
		{"name with zero-width", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a\xe2\x80\x8bb"}}
		}), ""},
		{"name with NUL", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a\u0000b"}}
		}), ""},
		{"name with only Hangul fillers (looks empty)", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": string(rune(0x3164))}}
		}), ""},
		{"name with blank braille", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a" + string(rune(0x2800)) + "b"}}
		}), ""},
		{"nomes repetidos no eixo (caixa e acento)", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["category"] = []any{map[string]any{"id": 1, "name": "Séries"}, map[string]any{"id": 2, "name": "SERIES"}}
		}), ""},
		{"name is not text", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": 7}}
		}), ""},
		{"kind em eixo errado", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["type"] = []any{map[string]any{"id": 1, "name": "a", "kind": "movie"}}
		}), ""},
		{"kind desconhecido", mutate(func(m map[string]any) {
			m["taxonomy"].(map[string]any)["category"] = []any{map[string]any{"id": 1, "name": "a", "kind": "<script>"}}
		}), ""},
		{"termos demais no eixo", mutate(func(m map[string]any) { m["taxonomy"].(map[string]any)["type"] = termsWith(MaxTermsPerAxis + 1) }), ""},
		{"taxonomy is not object", mutate(func(m map[string]any) { m["taxonomy"] = []any{} }), ""},
		{"axis is not list", mutate(func(m map[string]any) { m["taxonomy"].(map[string]any)["type"] = "string" }), ""},

		{"duplicate key (the last one would win silently)", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d",` +
			`"schema_version":1,"taxonomy":{"type":[{"id":1,"name":"a"}]},"taxonomy":{"type":[{"id":1,"name":"b"}]}}`, ""},
		{"duplicate key only in case", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d",` +
			`"schema_version":1,"taxonomy":{},"Taxonomy":{"type":[{"id":1,"name":"b"}]}}`, ""},
		{"key with uppercase (decoder would match it)", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"FAMILY":"unit3d",` +
			`"schema_version":1,"taxonomy":{}}`, ""},
		{"key with Kelvin (Unicode folds to k)", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d",` +
			`"schema_version":1,"taxonomy":{"category":[{"id":1,"name":"a","\u212Aind":"tv"}]}}`, ""},

		{"aninhamento absurdo", strings.Repeat("[", 100000), ""},
		{"aninhamento moderado no meio do documento", `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d",` +
			`"schema_version":1,"taxonomy":{"type":[[[[[[[[[[[[1]]]]]]]]]]]]}}`, ""},
		{"acima do limite de bytes", `{"format":"` + strings.Repeat("x", MaxDocumentBytes) + `"}`, CodeTooLarge},
	}
	for _, tc := range cases {
		doc, err := ParseDocument([]byte(tc.body))
		if err == nil {
			t.Errorf("%s: accepted hostile payload (%+v)", tc.name, doc.Site)
			continue
		}
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: error %T (%v), want *ValidationError — anything else would become 500", tc.name, err, err)
			continue
		}
		want := tc.code
		if want == "" {
			want = CodeInvalid
		}
		if ve.ErrorCode() != want {
			t.Errorf("%s: code %q, want %q (%s)", tc.name, ve.ErrorCode(), want, ve.Error())
		}
		if len(ve.Error()) > 300 {
			t.Errorf("%s: error message has %d bytes — hostile value must not be returned intact", tc.name, len(ve.Error()))
		}
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// A field error says WHICH field failed (the Portal shows it as it came).
func TestParseDocument_FieldPathInError(t *testing.T) {
	withType := func(terms ...any) string {
		return mutate(func(m map[string]any) { m["taxonomy"].(map[string]any)["type"] = terms })
	}
	cases := []struct{ path, body string }{
		{"taxonomy.type[1].name", withType(map[string]any{"id": 1, "name": "a"}, map[string]any{"id": 2, "name": "a\xe2\x80\xaeb"})},
		{"taxonomy.type[0].id", withType(map[string]any{"id": -5, "name": "a"})},
		{"taxonomy.type[0].origin", withType(map[string]any{"id": 1, "name": "a", "origin": "seed"})},
		{"schema_version", mutate(func(m map[string]any) { m["schema_version"] = 7 })},
		{"site.host", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": "x", "host": "a/b"} })},
		{"site.name", mutate(func(m map[string]any) { m["site"] = map[string]any{"name": ""} })},
		{"format", mutate(func(m map[string]any) { m["format"] = "outro" })},
		{"family", mutate(func(m map[string]any) { m["family"] = "gazelle" })},
		// Wrong type: the decoder reports the path without index and without the map key (the axis).
		{"taxonomy.id", withType(map[string]any{"id": "7", "name": "a"})},
	}
	for _, tc := range cases {
		_, err := ParseDocument([]byte(tc.body))
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: error = %v", tc.path, err)
			continue
		}
		if ve.Path != tc.path {
			t.Errorf("path = %q, want %q (%s)", ve.Path, tc.path, ve.Msg)
		}
	}
}

func TestCheckTarget_FamilyMustMatch(t *testing.T) {
	doc := Document{Profile: Profile{Family: FamilyUNIT3D}}
	if err := doc.CheckTarget(Profile{Family: FamilyUNIT3D}); err != nil {
		t.Errorf("same family rejected: %v", err)
	}
	err := doc.CheckTarget(Profile{Family: "gazelle"})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.ErrorCode() != CodeIncompatibleFamily {
		t.Errorf("different family = %v, want incompatible_family", err)
	}
}
