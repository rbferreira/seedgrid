package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Hardening of the profile export/import (#197), findings from the security review: the passkey
// pasted in the tracker name, the ambiguous profile, the filename in the audit trail and the name
// that the operator's site confirmed.

const exHexKey = "0123456789abcdef0123456789abcdef01234567" //nolint:gosec // G101: fake test passkey

// The tracker name is free text from the operator. Pasting the announce (or just the passkey) cannot
// take the credential to a file that will be shared — not a single piece of it leaks.
func TestProfileExport_TrackerNameWithCredentialDoesNotLeak(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)

	for _, name := range []string{
		"https://tracker.example.org:2710/announce/" + exHexKey,
		exHexKey,
		"House " + exHexKey,
		sitePasskey + "?x=" + exHexKey,
	} {
		id := exTracker(t, h, admin, name, fake.srv.URL)
		code, body, _ := exExport(t, h, admin, id)
		if code != http.StatusOK {
			t.Fatalf("export (name %.30q): %d %s", name, code, body)
		}
		for _, secret := range []string{exHexKey, sitePasskey} {
			for i := 0; i+8 <= len(secret); i++ {
				if strings.Contains(body, secret[i:i+8]) {
					t.Errorf("name %.30q: file contains a piece of the passkey (%q):\n%s", name, secret[i:i+8], body)
				}
			}
		}
		if strings.Contains(body, "/announce") || strings.Contains(body, siteToken) {
			t.Errorf("name %.30q: file contains the announce or the token:\n%s", name, body)
		}
		if !strings.Contains(body, `"name": "tracker.example.org"`) {
			t.Errorf("name %.30q: site.name should have defaulted to host:\n%s", name, body)
		}
	}
}

// A profile with two terms that the publish panel wouldn't be able to distinguish (same name, ignoring
// case and accents) isn't exported as is: the export never delivers a file that the import itself would
// reject. The operator receives the reason.
func TestProfileExport_ProfileWithDuplicateNamesNotExported(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id := exTracker(t, h, admin, "Ambiguous", fake.srv.URL)
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/profile", admin,
		`{"taxonomy":{"category":[{"id":1,"name":"Séries","kind":"tv"},{"id":2,"name":"Series","kind":"tv"}]}}`); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	code, body, _ := exExport(t, h, admin, id)
	if code != http.StatusConflict || !strings.Contains(body, "profile_not_exportable") || !strings.Contains(body, "duplicate name") {
		t.Errorf("ambiguous profile export: %d %s, want 409 profile_not_exportable with reason", code, body)
	}
	if strings.Contains(body, siteToken) {
		t.Errorf("the refusal leaked the token: %s", body)
	}

	// Fixed the profile, exports.
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/profile", admin,
		`{"taxonomy":{"category":[{"id":1,"name":"Séries","kind":"tv"},{"id":2,"name":"Animes","kind":"tv"}]}}`); code != http.StatusOK {
		t.Fatalf("fixed PUT: %d %s", code, body)
	}
	if code, body, _ := exExport(t, h, admin, id); code != http.StatusOK {
		t.Errorf("fixed profile export: %d %s", code, body)
	}
}

// A file with two names differing only in case or accent is rejected: the panel matches by name and
// couldn't choose — a new term would obscure the right one.
func TestProfileImport_FileWithDuplicateNamesRejected(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id := exTracker(t, h, admin, "Destination", fake.srv.URL)
	file := `{"format":"seedgrid.tracker-profile","site":{"name":"x","host":"tracker.example.org"},"family":"unit3d","schema_version":1,` +
		`"taxonomy":{"category":[{"id":1,"name":"Séries","kind":"tv"},{"id":2,"name":"SERIES","kind":"tv"}]}}`
	for _, path := range []string{exPreview, exImport} {
		code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(path, id), admin, file)
		if code != http.StatusBadRequest || !strings.Contains(body, "duplicate name in axis") || !strings.Contains(body, "taxonomy.category[1].name") {
			t.Errorf("%s: %d %s, want 400 pointing to taxonomy.category[1].name", path, code, body)
		}
	}
}

// The name the OPERATOR'S SITE confirmed (discovered) only changes if requested term by term — the
// discovery treats rename as opt-in and the import is no more permissive than it.
func TestProfileImport_DiscoveredIsProtectedWithoutRequest(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	to := exTracker(t, h, admin, "Destination", fake.srv.URL)
	if code, body := tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+to+"/apply-taxonomy", admin,
		`{"items":[{"axis":"type","id":50,"name":"PC"}]}`); code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, body)
	}
	file := `{"format":"seedgrid.tracker-profile","site":{"name":"Other","host":"tracker.example.org"},"family":"unit3d","schema_version":1,` +
		`"taxonomy":{"type":[{"id":50,"name":"Programas de PC"}]}}`

	typeTerm := func(raw string) (string, string) {
		var r struct {
			Profile profileOut `json:"profile"`
		}
		tpDecode(t, raw, &r)
		for _, tm := range r.Profile.Taxonomy["type"] {
			if tm.ID == 50 {
				return tm.Name, tm.Origin
			}
		}
		return "", ""
	}

	code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if name, origin := typeTerm(body); name != "PC" || origin != "discovered" {
		t.Errorf("name confirmed by site was changed without request: %q (%s)", name, origin)
	}
	if !strings.Contains(body, `"status":"protected"`) || !strings.Contains(body, `"current_origin":"discovered"`) {
		t.Errorf("plan must state that it protected a discovered name: %s", body)
	}

	q := "?" + url.Values{"overwrite": {"type:50"}}.Encode()
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to)+q, admin, file)
	if code != http.StatusOK {
		t.Fatalf("import with request: %d %s", code, body)
	}
	if name, origin := typeTerm(body); name != "Programas de PC" || origin != "imported" {
		t.Errorf("after request: %q (%s), want Programas de PC (imported)", name, origin)
	}
}

// The audit trail doesn't take anything from the file (its site name is third-party text and could
// forge the rest of the line with « quotes»), and the tracker name loses the quotes that delimit it.
func TestProfileImport_AuditDoesNotIncludeFileText(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	to := exTracker(t, h, admin, "False» and then «Admin", fake.srv.URL)
	file := `{"format":"seedgrid.tracker-profile","site":{"name":"X» FORGED «Y","host":"tracker.example.org"},"family":"unit3d","schema_version":1,` +
		`"taxonomy":{"type":[{"id":3,"name":"Encode"}]}}`
	if code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file); code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	actions := query(`SELECT action FROM audit_log WHERE path LIKE $1 AND status_code = 200`, "%/trackers/"+to+"/profile/import")
	want := "Imported a profile from a file into tracker «False and then Admin»: 1 new, 0 renamed, 0 role(s) filled; " +
		"0 kept because they belong to the operator or were already confirmed by its site"
	if len(actions) != 1 || actions[0] != want {
		t.Errorf("audit:\n got %q\nwant %q", actions, want)
	}
	for _, a := range actions {
		if strings.Contains(a, "FORGED") {
			t.Errorf("file text in trail: %q", a)
		}
	}
}
