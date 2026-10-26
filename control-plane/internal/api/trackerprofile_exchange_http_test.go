package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
)

// Profile export and import (#197), via the full API against the test database.

const (
	exPath      = "/api/v1/trackers/%s/profile/export"
	exPreview   = "/api/v1/trackers/%s/profile/import/preview"
	exImport    = "/api/v1/trackers/%s/profile/import"
	exFullBody  = `{"taxonomy":{"category":[{"id":1,"name":"Filmes","kind":"movie"},{"id":2,"name":"Séries","kind":"tv"},{"id":5,"name":"Games","kind":"game"},{"id":9,"name":"HQs & Mangás"}],"type":[{"id":3,"name":"Encode"},{"id":4,"name":"WEB-DL"}],"resolution":[{"id":3,"name":"1080p"}]}}`
	exSourceTag = "SOURCE-TAG"
)

// exTracker creates a tracker with ALL of layer 3 filled in (announce with passkey, api_url,
// api_token and source) — which is what the export must not leak — and returns the id.
func exTracker(t *testing.T, h http.Handler, token, name, apiURL string) string {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"announce":"https://tracker.example.org:2710/announce/%s","source":%q,"api_url":%q,"api_token":%q}`,
		name, sitePasskey, exSourceTag, apiURL, siteToken)
	code, resp := tpCall(t, h, http.MethodPost, "/api/v1/trackers", token, body)
	if code != http.StatusCreated {
		t.Fatalf("create tracker: %d %s", code, resp)
	}
	var out struct {
		ID string `json:"id"`
	}
	tpDecode(t, resp, &out)
	return out.ID
}

func exExport(t *testing.T, h http.Handler, token, id string) (int, string, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf(exPath, id), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), rec.Header()
}

// jsonKeys gathers every key name that appears at any level of the JSON.
func jsonKeys(v any, into map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			into[k] = true
			jsonKeys(val, into)
		}
	case []any:
		for _, val := range x {
			jsonKeys(val, into)
		}
	}
}

// exFilledTracker fills the tracker profile completely: the three axes, the role of
// categories and terms from different origins (manual and discovered).
func exFilledTracker(t *testing.T, h http.Handler, admin, name string) (id string, fake *site) {
	t.Helper()
	fake = newSite(t)
	id = exTracker(t, h, admin, name, fake.srv.URL)
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/profile", admin, exFullBody); code != http.StatusOK {
		t.Fatalf("PUT profile: %d %s", code, body)
	}
	if code, body := tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/apply-taxonomy", admin,
		`{"items":[{"axis":"type","id":50,"name":"PC"},{"axis":"resolution","id":10,"name":"Outra"}]}`); code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, body)
	}
	return id, fake
}

// The test required by #197: a tracker with api_token, announce with passkey, and the profile
// fully filled is exported — and NO sensitive value appears in the file. Textual search
// for the VALUES, not just the field names: a field name says what the code intended,
// the value says what actually came out.
func TestProfileExport_DoesNotLeakLayerThree(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	id, fake := exFilledTracker(t, h, admin, "My Tracker")

	// The filled image_host section must also be output in the exchange file.
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/profile/image-host", admin,
		`{"name":"img.example.com","upload_url":"https://img.example.com/upload","file_field":"file","auth_header":"X-API-Key","auth_prefix":"Bearer ","url_path":"data.url","thumbnail_path":"data.thumb"}`); code != http.StatusOK {
		t.Fatalf("PUT image-host: %d %s", code, body)
	}
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/image-host-key", admin, `{"key":"EXPORT-SECRET-KEY"}`); code != http.StatusOK {
		t.Fatalf("PUT image-host-key: %d %s", code, body)
	}

	code, body, hdr := exExport(t, h, admin, id)
	if code != http.StatusOK {
		t.Fatalf("export: %d %s", code, body)
	}

	// 1. No layer 3 value — not even a piece of it — is in the file.
	for name, secret := range map[string]string{
		"api_token":                  siteToken,
		"passkey":                    sitePasskey,
		"announce (path)":            "/announce/",
		"announce (full URL)":        "https://tracker.example.org:2710/announce/" + sitePasskey,
		"announce port":              ":2710",
		"api_url":                    fake.srv.URL,
		"api_url (host:port)":        strings.TrimPrefix(fake.srv.URL, "http://"),
		"source":                     exSourceTag,
		"value returned by the site": "MUST-NOT-LEAK",
		"image_host_key (value)":     "EXPORT-SECRET-KEY",
		"api_token field name":       "api_token",
		"announce field name":        "announce",
		"passkey field name":         "passkey",
		"api_url field name":         "api_url",
		"has_api field name":         "has_api_token",
		"source field name":          `"source"`,
		"the tracker id (internal)":  id,
	} {
		if strings.Contains(body, secret) {
			t.Errorf("export contains %s (%q):\n%s", name, secret, body)
		}
	}

	// 2. Dynamic, on the actual file: only keys from the list, at any level.
	var doc map[string]any
	tpDecode(t, body, &doc)
	keys := map[string]bool{}
	jsonKeys(doc, keys)
	allowed := map[string]bool{
		"format": true, "site": true, "name": true, "host": true, "family": true,
		"schema_version": true, "taxonomy": true, "category": true, "type": true, "resolution": true,
		"id": true, "kind": true,
		// description section (#192):
		"description": true, "media_blocks": true, "game_blocks": true, "media_info_tag": true,
		// image_host section (#193):
		"image_host": true, "upload_url": true, "file_field": true,
		"auth_header": true, "auth_prefix": true, "url_path": true, "thumbnail_path": true,
	}
	for k := range keys {
		if !allowed[k] {
			t.Errorf("export has key %q, outside exchange format", k)
		}
	}

	// 3. What belongs to the site is there, completely.
	var out struct {
		Format        string `json:"format"`
		Family        string `json:"family"`
		SchemaVersion int    `json:"schema_version"`
		Site          struct{ Name, Host string }
		Taxonomy      map[string][]struct {
			ID   int
			Name string
			Kind string
		}
	}
	tpDecode(t, body, &out)
	if out.Format != trackerprofile.FormatID || out.Family != "unit3d" || out.SchemaVersion != 1 {
		t.Errorf("envelope = %+v", out)
	}
	if out.Site.Name != "My Tracker" || out.Site.Host != "tracker.example.org" {
		t.Errorf("site = %+v, want name and public host (without announce port)", out.Site)
	}
	if n := len(out.Taxonomy["category"]); n != 4 {
		t.Errorf("categories = %d, want 4", n)
	}
	if n := len(out.Taxonomy["type"]); n != 3 { // 3, 4 (manual) e 50 (discovered)
		t.Errorf("types = %d, want 3", n)
	}
	for _, c := range out.Taxonomy["category"] {
		if c.ID == 2 && c.Kind != "tv" {
			t.Errorf("category role did not appear in file: %+v", c)
		}
	}

	// 4. Headers: file to download, no cache and no sniffing.
	if ct := hdr.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := hdr.Get("Content-Disposition"); cd != `attachment; filename="seedgrid-profile-unit3d-tracker-example-org.json"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if hdr.Get("Cache-Control") != "no-store" || hdr.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("cache/sniffing headers: %v", hdr)
	}

	// 5. The origin is local: it doesn't travel.
	for _, local := range []string{"origin", "manual", "discovered", "imported", "seed"} {
		// "seed" appears in "seedgrid" — search only as a value.
		needle := local
		if local == "seed" {
			needle = `"seed"`
		}
		if strings.Contains(body, needle) {
			t.Errorf("the export carries %q (local state)", needle)
		}
	}
}

// From export to import, end-to-end, between two trackers: preview neither saves nor
// audits; import saves, audits (one line) and is incremental; the origin becomes "imported".
func TestProfileImport_DoExportAoImport(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	from, _ := exFilledTracker(t, h, admin, "Origin")
	fake := newSite(t)
	to := exTracker(t, h, admin, "Destination", fake.srv.URL)

	_, file, _ := exExport(t, h, admin, from)

	// Preview: everything new, with a warning (same host: no site warning).
	code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(exPreview, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("preview: %d %s", code, body)
	}
	var prev struct {
		Plan   trackerprofile.ImportPlan `json:"plan"`
		Notice string                    `json:"notice"`
	}
	tpDecode(t, body, &prev)
	if c := prev.Plan.Counts; c.New != 9 || c.Renamed+c.Same+c.Protected+c.LocalOnly != 0 {
		t.Errorf("preview counts = %+v, want 9 new (4 categories + 3 types + 2 resolutions)", c)
	}
	if !strings.Contains(prev.Notice, "came from a file") {
		t.Errorf("preview does not carry the third-party content notice: %q", prev.Notice)
	}
	if len(prev.Plan.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", prev.Plan.Warnings)
	}
	profile := func(id string) profileOut {
		t.Helper()
		_, list := tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
		var l struct {
			Data []struct {
				ID      string     `json:"id"`
				Profile profileOut `json:"profile"`
			} `json:"data"`
		}
		tpDecode(t, list, &l)
		for _, tr := range l.Data {
			if tr.ID == id {
				return tr.Profile
			}
		}
		t.Fatalf("tracker %s is not in the list", id)
		return profileOut{}
	}
	for _, axis := range trackerprofile.Axes {
		if n := len(profile(to).Taxonomy[axis]); n != 0 {
			t.Fatalf("preview SAVED (%s has %d terms)", axis, n)
		}
	}
	if n := len(query(`SELECT action FROM audit_log WHERE path LIKE $1`, "%/trackers/"+to+"/%")); n != 0 {
		t.Errorf("preview entered audit (%d lines)", n)
	}

	// Actually import.
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	var res struct {
		Profile profileOut                `json:"profile"`
		Plan    trackerprofile.ImportPlan `json:"plan"`
	}
	tpDecode(t, body, &res)
	if res.Plan.Counts.New != 9 {
		t.Errorf("counts do import = %+v", res.Plan.Counts)
	}
	got := profile(to)
	if got.Family != "unit3d" || len(got.Taxonomy["category"]) != 4 || len(got.Taxonomy["type"]) != 3 || len(got.Taxonomy["resolution"]) != 2 {
		t.Fatalf("imported profile = %+v", got)
	}
	for axis, terms := range got.Taxonomy {
		for _, tm := range terms {
			if tm.Origin != "imported" {
				t.Errorf("%s %d: origin %q, want imported (file origin does not travel, and import assigns its own)", axis, tm.ID, tm.Origin)
			}
		}
	}
	for _, tm := range got.Taxonomy["category"] {
		if tm.ID == 5 && tm.Kind != "game" {
			t.Errorf("role was not imported: %+v", tm)
		}
	}
	// The ORIGIN profile hasn't changed.
	if n := len(profile(from).Taxonomy["category"]); n != 4 {
		t.Errorf("the source tracker changed: %d categories", n)
	}

	// Incremental: importing again changes nothing.
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("2º import: %d %s", code, body)
	}
	var again struct {
		Plan trackerprofile.ImportPlan `json:"plan"`
	}
	tpDecode(t, body, &again)
	if again.Plan.Changes() || again.Plan.Counts.Same != 9 {
		t.Errorf("2nd import = %+v, want 9 'same' and nothing to change", again.Plan.Counts)
	}

	// Audit: one record per applied import (preview doesn't count), with the tracker name.
	actions := query(`SELECT action FROM audit_log WHERE path LIKE $1 ORDER BY created_at`, "%/trackers/"+to+"/%")
	want := []string{
		"Imported a profile from a file into tracker «Destination»: 9 new, 0 renamed, 0 role(s) filled; 0 kept because they belong to the operator or were already confirmed by its site",
		"Imported a profile from a file into tracker «Destination»: 0 new, 0 renamed, 0 role(s) filled; 0 kept because they belong to the operator or were already confirmed by its site",
	}
	if strings.Join(actions, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit:\n got %q\nwant %q", actions, want)
	}
	if n := len(query(`SELECT action FROM audit_log WHERE path LIKE $1`, "%/trackers/"+from+"/profile/export")); n != 0 {
		t.Errorf("export entered audit (%d lines) — it is a read", n)
	}
}

// The #219 merge rule applies to import: what the operator adjusted manually is not overwritten
// without them asking — and asking is by term, in the query.
func TestProfileImport_ProtectsManualAndAllowsOverwriteWhenRequested(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	from, _ := exFilledTracker(t, h, admin, "Origin")
	_, file, _ := exExport(t, h, admin, from)

	fake := newSite(t)
	to := exTracker(t, h, admin, "Destination", fake.srv.URL)
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+to+"/profile", admin,
		`{"taxonomy":{"category":[{"id":2,"name":"Seriados"},{"id":1,"name":"Filmes"}]}}`); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}

	catName := func(raw string, id int) (string, string) {
		var r struct {
			Profile profileOut `json:"profile"`
		}
		tpDecode(t, raw, &r)
		for _, tm := range r.Profile.Taxonomy["category"] {
			if tm.ID == id {
				return tm.Name, tm.Origin
			}
		}
		return "", ""
	}

	// Without asking: category 2 remains "Seriados" (manual), and the plan says it kept it.
	code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if name, origin := catName(body, 2); name != "Seriados" || origin != "manual" {
		t.Errorf("manual adjustment was overwritten without request: %q (%s)", name, origin)
	}
	var res struct {
		Plan trackerprofile.ImportPlan `json:"plan"`
	}
	tpDecode(t, body, &res)
	// Movies (id 1) gets the "movie" role; "Seriados" is protected and doesn't even get the role.
	if res.Plan.Counts.Protected != 1 || res.Plan.Counts.KindSet != 1 {
		t.Errorf("counts = %+v, want 1 protected and 1 role filled", res.Plan.Counts)
	}

	// Asking (only for category 2): it overwrites, and the term becomes "imported".
	q := "?" + url.Values{"overwrite": {"category:2"}}.Encode()
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to)+q, admin, file)
	if code != http.StatusOK {
		t.Fatalf("import with request: %d %s", code, body)
	}
	if name, origin := catName(body, 2); name != "Séries" || origin != "imported" {
		t.Errorf("after request: %q (%s), want Séries (imported)", name, origin)
	}
	tpDecode(t, body, &res)
	if res.Plan.Counts.Renamed != 1 {
		t.Errorf("counts = %+v, want 1 renamed", res.Plan.Counts)
	}

	// Malformed request is 400, never silently ignored nor 500.
	for _, bad := range []string{"category", "category:abc", "audio:1", "category:0", "category:1;DROP"} {
		q := "?" + url.Values{"overwrite": {bad}}.Encode()
		if code, resp := tpCall(t, h, http.MethodPost, fmt.Sprintf(exPreview, to)+q, admin, file); code != http.StatusBadRequest {
			t.Errorf("overwrite=%q: %d %.150s, want 400", bad, code, resp)
		}
	}
}

// A file from another site warns on preview: the ids are from the site.
func TestProfileImport_WarnsOnFileFromOtherSite(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	to := exTracker(t, h, admin, "Destination", fake.srv.URL)
	file := `{"format":"seedgrid.tracker-profile","site":{"name":"Blutopia","host":"blutopia.example"},"family":"unit3d","schema_version":1,` +
		`"taxonomy":{"type":[{"id":12,"name":"Encode"}]}}`
	code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(exPreview, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("preview: %d %s", code, body)
	}
	if !strings.Contains(body, "Blutopia") || !strings.Contains(body, "tracker.example.org") || !strings.Contains(body, "wrong id publishes to the wrong category") {
		t.Errorf("missing notice from another site: %s", body)
	}
}

// Imported profile is hostile content: each payload is rejected BEFORE the database, with the
// correct status and a short message — never 500 — and nothing reaches the profile. Applies to preview
// and to import.
func TestProfileImport_HostilePayloads(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id := exTracker(t, h, admin, "Hostile", fake.srv.URL)

	valid := func(mut func(m map[string]any)) string {
		m := map[string]any{
			"format": "seedgrid.tracker-profile", "family": "unit3d", "schema_version": 1,
			"site":     map[string]any{"name": "Example", "host": "tracker.example"},
			"taxonomy": map[string]any{"type": []any{map[string]any{"id": 3, "name": "Encode"}}},
		}
		mut(m)
		raw, _ := json.Marshal(m)
		return string(raw)
	}
	typeTerms := func(terms ...any) string {
		return valid(func(m map[string]any) { m["taxonomy"] = map[string]any{"type": terms} })
	}
	bigName := strings.Repeat("x", 1<<20)

	cases := []struct {
		name string
		body string
		want int
		code string
	}{
		{"Malformed JSON", `{"format":`, 400, "invalid_profile"},
		{"empty body", ``, 400, "invalid_profile"},
		{"not an object", `[1,2,3]`, 400, "invalid_profile"},
		{"data after document", valid(func(m map[string]any) {}) + `{"x":1}`, 400, "invalid_profile"},
		{"unknown field", valid(func(m map[string]any) { m["api_token"] = "x" }), 400, "invalid_profile"},
		{"unknown field in term", typeTerms(map[string]any{"id": 1, "name": "a", "url": "http://internal/"}), 400, "invalid_profile"},
		{"origin in file", typeTerms(map[string]any{"id": 1, "name": "a", "origin": "manual"}), 400, "invalid_profile"},
		{"wrong format", valid(func(m map[string]any) { m["format"] = "other" }), 400, "invalid_profile"},
		{"future schema_version", valid(func(m map[string]any) { m["schema_version"] = 2 }), 400, "unsupported_schema_version"},
		{"schema_version zero", valid(func(m map[string]any) { m["schema_version"] = 0 }), 400, "invalid_profile"},
		{"schema_version string", valid(func(m map[string]any) { m["schema_version"] = "1" }), 400, "invalid_profile"},
		{"wrong family", valid(func(m map[string]any) { m["family"] = "gazelle" }), 400, "invalid_profile"},
		{"negative id", typeTerms(map[string]any{"id": -1, "name": "a"}), 400, "invalid_profile"},
		{"id out of range", typeTerms(map[string]any{"id": trackerprofile.MaxTermID + 1, "name": "a"}), 400, "invalid_profile"},
		{"huge id", `{"format":"seedgrid.tracker-profile","family":"unit3d","schema_version":1,"site":{"name":"x"},"taxonomy":{"type":[{"id":99999999999999999999,"name":"a"}]}}`, 400, "invalid_profile"},
		{"id as string", typeTerms(map[string]any{"id": "1", "name": "a"}), 400, "invalid_profile"},
		{"duplicate id", typeTerms(map[string]any{"id": 1, "name": "a"}, map[string]any{"id": 1, "name": "b"}), 400, "invalid_profile"},
		{"name with 65 characters", typeTerms(map[string]any{"id": 1, "name": strings.Repeat("x", 65)}), 400, "invalid_profile"},
		{"name with direction override", typeTerms(map[string]any{"id": 1, "name": "a\xe2\x80\xaeb"}), 400, "invalid_profile"},
		{"name with NUL", typeTerms(map[string]any{"id": 1, "name": "a\x00b"}), 400, "invalid_profile"},
		{"huge name (above body limit)", typeTerms(map[string]any{"id": 1, "name": bigName}), 413, "payload_too_large"},
		{"body above limit", `{"format":"` + strings.Repeat("x", 2<<20) + `"}`, 413, "payload_too_large"},
		{"unknown axis", valid(func(m map[string]any) { m["taxonomy"] = map[string]any{"audio": []any{}} }), 400, "invalid_profile"},
		{"kind in wrong axis", typeTerms(map[string]any{"id": 1, "name": "a", "kind": "movie"}), 400, "invalid_profile"},
		{"site.host with credential", valid(func(m map[string]any) { m["site"] = map[string]any{"name": "x", "host": "u:p@x.org"} }), 400, "invalid_profile"},
		{"duplicate key", `{"format":"seedgrid.tracker-profile","family":"unit3d","schema_version":1,"schema_version":1,"site":{"name":"x"}}`, 400, "invalid_profile"},
		{"absurd nesting", strings.Repeat("[", 200000), 400, "invalid_profile"},
	}
	for _, path := range []string{exPreview, exImport} {
		for _, tc := range cases {
			code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(path, id), admin, tc.body)
			if code != tc.want {
				t.Errorf("%s [%s]: status %d, want %d (%.200s)", tc.name, path[strings.LastIndex(path, "/")+1:], code, tc.want, body)
				continue
			}
			var env struct {
				Error struct{ Code, Message string } `json:"error"`
			}
			tpDecode(t, body, &env)
			if env.Error.Code != tc.code {
				t.Errorf("%s: code %q, want %q", tc.name, env.Error.Code, tc.code)
			}
			if len(body) > 600 {
				t.Errorf("%s: error response has %d bytes — hostile value must not be returned", tc.name, len(body))
			}
		}
	}

	// Nothing hostile reached the database: the profile remains empty, and no attempt was applied.
	_, list := tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if strings.Contains(list, `"name":"a`) || strings.Contains(list, `"name":"Encode"`) {
		t.Errorf("hostile payload was persisted: %.300s", list)
	}

	// Non-existent or malformed tracker: 404 (with a valid file), never 500.
	good := valid(func(m map[string]any) {})
	for _, bad := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid", "1%27%20OR%201=1"} {
		for _, path := range []string{exPreview, exImport} {
			if code, resp := tpCall(t, h, http.MethodPost, fmt.Sprintf(path, bad), admin, good); code != http.StatusNotFound {
				t.Errorf("%s: status %d, want 404 (%.150s)", fmt.Sprintf(path, bad), code, resp)
			}
		}
		if code, resp, _ := exExport(t, h, admin, bad); code != http.StatusNotFound {
			t.Errorf("export of %s: status %d, want 404 (%.150s)", bad, code, resp)
		}
	}
	// Import rejections are logged in the audit with the status (import is mutating),
	// but none of them became "Imported a profile from file..." — only what was applied gets this description.
	for _, a := range query(`SELECT action FROM audit_log WHERE path LIKE $1 AND status_code = 200`, "%/trackers/"+id+"/profile/import") {
		t.Errorf("hostile import was audited as success: %q", a)
	}
}

// RBAC follows the tracker endpoints standard: export is read (trackers:read); preview
// and import are write (trackers:write), which only the Administrator holds (ADR-047). Trackers
// belong to the cluster: anyone who reads trackers exports any of them.
func TestProfileExchange_RBAC(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	fake := newSite(t)
	id := exTracker(t, h, tokens["Administrator"], "Do admin", fake.srv.URL)
	file := `{"format":"seedgrid.tracker-profile","site":{"name":"x"},"family":"unit3d","schema_version":1,"taxonomy":{"type":[{"id":3,"name":"Encode"}]}}`
	missing := "00000000-0000-0000-0000-000000000000"

	// No session: 401 on all three.
	for _, r := range []struct{ method, path, body string }{
		{"GET", fmt.Sprintf(exPath, id), ``},
		{"POST", fmt.Sprintf(exPreview, id), file},
		{"POST", fmt.Sprintf(exImport, id), file},
	} {
		if code, _ := tpCall(t, h, r.method, r.path, "", r.body); code != http.StatusUnauthorized {
			t.Errorf("no session on %s %s: %d, want 401", r.method, r.path, code)
		}
	}

	// Viewer and Publisher (trackers:read): export the cluster's tracker, 403 on the write ones.
	for _, role := range []string{"Viewer", "Publisher"} {
		if code, body, _ := exExport(t, h, tokens[role], id); code != http.StatusOK {
			t.Errorf("%s exporting: %d %.150s, want 200", role, code, body)
		}
		for _, path := range []string{exPreview, exImport} {
			if code, _ := tpCall(t, h, http.MethodPost, fmt.Sprintf(path, id), tokens[role], file); code != http.StatusForbidden {
				t.Errorf("%s on %s: %d, want 403", role, path, code)
			}
		}
	}

	// Administrator: all three work; a tracker that does not exist is 404.
	for _, path := range []string{exPreview, exImport} {
		if code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(path, id), tokens["Administrator"], file); code != http.StatusOK {
			t.Errorf("Administrator on %s: %d %.150s", path, code, body)
		}
		if code, _ := tpCall(t, h, http.MethodPost, fmt.Sprintf(path, missing), tokens["Administrator"], file); code != http.StatusNotFound {
			t.Errorf("Administrator on %s of a missing tracker: %d, want 404", path, code)
		}
	}
	if code, _, _ := exExport(t, h, tokens["Viewer"], missing); code != http.StatusNotFound {
		t.Errorf("exporting a missing tracker: %d, want 404", code)
	}
}
