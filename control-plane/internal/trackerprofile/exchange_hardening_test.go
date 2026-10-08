package trackerprofile

import (
	"errors"
	"strings"
	"testing"
)

// Runes that are not space, control, or formatting — yet still render nothing. A name made
// only of them looks empty on screen; inside a name, it makes two "equal" terms distinct.
func TestCleanName_RejectsBlankRunes(t *testing.T) {
	for _, r := range []rune{0x3164, 0x115F, 0x1160, 0xFFA0, 0x2800, 0x17B4, 0x17B5, 0x034F} {
		for name, in := range map[string]string{
			"alone":            string(r),
			"in the middle":    "Fil" + string(r) + "mes",
			"at the beginning": string(r) + "Filmes",
			"no final":         "Filmes" + string(r),
			"in sequence":      "Filmes " + string(r) + string(r) + " HD",
		} {
			if got, err := CleanName(in); err == nil {
				t.Errorf("U+%04X %s: CleanName accepted (%q)", r, name, got)
			}
		}
	}
	// What is actual text still applies: real hangul, non-empty braille, combining accents.
	for _, ok := range []string{"한국어", "Séries", "Cafe" + string(rune(0x301)), "⠁⠃"} {
		if _, err := CleanName(ok); err != nil {
			t.Errorf("CleanName(%q) rejected legitimate text: %v", ok, err)
		}
	}
}

// siteLabel is the site name in the file. The tracker name is free text from the operator: whoever pastes
// the announce (or a part of it) takes the passkey into a file that will be shared.
func TestSiteLabel_NeverCarriesCredentialOrTruncatesURL(t *testing.T) {
	const passkey = "0123456789abcdef0123456789abcdef01234567"
	const host = "tracker.example.org"
	announce := "https://" + host + "/announce/" + passkey

	cases := map[string]string{
		"the entire announce":        announce,
		"announce without scheme":    host + "/announce/" + passkey,
		"just the passkey":           passkey,
		"passkey with prefix":        "my " + passkey,
		"passkey in the middle":      "Tracker " + passkey + " Private",
		"query string":               "Tracker?passkey=abc",
		"assignment":                 "passkey=abc",
		"backslash":                  `C:\tracker\chave`,
		"uuid without hyphen":        strings.ReplaceAll("123e4567-e89b-12d3-a456-426614174000", "-", ""),
		"uuid with hyphen":           "House 123e4567-e89b-12d3-a456-426614174000",
		"twenty consecutive letters": strings.Repeat("a", credentialRun),
		"90-character announce":      "https://t.example/announce/" + strings.Repeat("Ab3", 20),
		"name that fails CleanName":  strings.Repeat("x", MaxNameRunes+1),
		"name with bidi":             "a" + string(rune(0x202E)) + "b",
		"empty name":                 "   ",
	}
	for name, in := range cases {
		got := siteLabel(in, host)
		if got != host {
			t.Errorf("%s: siteLabel = %q, want host %q", name, got, host)
		}
		// No part of the passkey (6 consecutive characters) remains in the output.
		for i := 0; i+6 <= len(passkey); i++ {
			if strings.Contains(got, passkey[i:i+6]) {
				t.Errorf("%s: exported name contains a piece of the passkey (%q in %q)", name, passkey[i:i+6], got)
			}
		}
		// Without a host, it falls to "tracker" — never what the operator typed.
		if bare := siteLabel(in, ""); bare != "tracker" {
			t.Errorf("%s: no host, siteLabel = %q, want \"tracker\"", name, bare)
		}
	}
	// A host that doesn't fit in a name doesn't become the name either.
	long := strings.Repeat("a", 60) + ".example.org"
	if got := siteLabel(announce, long); got != "tracker" {
		t.Errorf("host of %d characters became name: %q", len(long), got)
	}
	// Normal names pass through as-is (including hyphens, dots, and accents).
	for _, ok := range []string{"Example", "My Private Tracker", "BLU", "Séries & Filmes", "Tracker-BR 2", "t.example.org"} {
		if got := siteLabel(ok, host); got != ok {
			t.Errorf("siteLabel(%q) = %q, want the name as is", ok, got)
		}
	}
}

// The passkey pasted in the tracker name doesn't leak into the exported file, end-to-end in the type.
func TestExport_NameWithPasskeyDoesNotLeak(t *testing.T) {
	const passkey = "0123456789abcdef0123456789abcdef01234567"
	for _, name := range []string{"https://tracker.example.org/announce/" + passkey, passkey, "House " + passkey} {
		raw, err := NewDocument(name, "tracker.example.org:2710", fullProfile()).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i+8 <= len(passkey); i++ {
			if strings.Contains(string(raw), passkey[i:i+8]) {
				t.Errorf("name %q: file contains a piece of the passkey (%q):\n%s", name, passkey[i:i+8], raw)
			}
		}
		if strings.Contains(string(raw), "/announce") {
			t.Errorf("name %q: file contains the announce path:\n%s", name, raw)
		}
		doc, err := ParseDocument(raw)
		if err != nil {
			t.Fatalf("export does not pass its own import: %v", err)
		}
		if doc.Site.Name != "tracker.example.org" {
			t.Errorf("site.name = %q, want the host", doc.Site.Name)
		}
	}
}

// The audit trail uses the tracker's safe label, without the quotes that delimit it.
func TestAuditLabel(t *testing.T) {
	cases := map[string]string{
		"Example":                "Example",
		"False» and then «Admin": "False and then Admin",
		"«»":                     "tracker",
		"https://x.org/announce/" + strings.Repeat("a", 32): "tracker",
	}
	for in, want := range cases {
		got := AuditLabel(in)
		if got != want {
			t.Errorf("AuditLabel(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(got, "«»") {
			t.Errorf("AuditLabel(%q) = %q still has the trail quotes", in, got)
		}
	}
}

// Two names that only differ in case or accent are the SAME label for the publishing panel: the
// file is rejected, pointing out the repeated term and the id it collides with.
func TestParseDocument_RefusesDuplicateAxisName(t *testing.T) {
	dup := func(axis string, a, b string) string {
		return mutate(func(m map[string]any) {
			m["taxonomy"] = map[string]any{axis: []any{map[string]any{"id": 1, "name": a}, map[string]any{"id": 2, "name": b}}}
		})
	}
	for _, c := range []struct{ axis, a, b string }{
		{"type", "WEB-DL", "web-dl"},
		{"category", "Séries", "Series"},
		{"category", "Series", "SÉRIES"},
		{"resolution", "1080p", " 1080P "},
		{"type", "Web  DL", "web dl"},
		{"category", "Se" + string(rune(0x301)) + "ries", "Séries"}, // composed and decomposed
	} {
		_, err := ParseDocument([]byte(dup(c.axis, c.a, c.b)))
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s %q/%q: error = %v, want *ValidationError", c.axis, c.a, c.b, err)
			continue
		}
		if ve.Path != "taxonomy."+c.axis+"[1].name" || !strings.Contains(ve.Msg, "same as id 1") {
			t.Errorf("%s %q/%q: %s: %s", c.axis, c.a, c.b, ve.Path, ve.Msg)
		}
	}
	// The same name on DIFFERENT axes doesn't collide (the panel matches by axis), and distinct names pass.
	ok := mutate(func(m map[string]any) {
		m["taxonomy"] = map[string]any{
			"category": []any{map[string]any{"id": 1, "name": "Encode"}},
			"type":     []any{map[string]any{"id": 1, "name": "Encode"}, map[string]any{"id": 2, "name": "Encode HD"}},
		}
	})
	if _, err := ParseDocument([]byte(ok)); err != nil {
		t.Errorf("same name in different axes was rejected: %v", err)
	}
}

// The export passes the same validation as the import before leaving: it never yields a file that the
// import itself would reject. (The service path is tested with a database; here, the type.)
func TestExport_ProfileWithRepeatedNamesFailsValidation(t *testing.T) {
	p := fullProfile()
	p.Taxonomy[AxisType] = append(p.Taxonomy[AxisType], Term{ID: 77, Name: "encode", Origin: OriginManual})
	_, err := NewDocument("My", "tracker.example.org", p).validated()
	var ve *ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, "duplicate name") {
		t.Errorf("validated = %v, want rejection for duplicate name", err)
	}
}
