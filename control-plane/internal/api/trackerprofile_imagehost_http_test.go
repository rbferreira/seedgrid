package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The image_host section (#193), via the full API against the test database:
// PUT and DELETE /trackers/{id}/profile/image-host (trackers:write).

const (
	ihSection = "/api/v1/trackers/%s/profile/image-host"

	// A valid Spec from the ADR-040 example.
	ihValid = `{
		"name": "Host do site",
		"upload_url": "https://img.tracker.exemplo.org/api/v1/sharex/upload",
		"file_field": "file",
		"auth_header": "Authorization",
		"auth_prefix": "Bearer ",
		"url_path": "url",
		"thumbnail_path": "thumbnail_url"
	}`

	// Another valid Spec, different from the first.
	ihValid2 = `{
		"name": "Outro Host",
		"upload_url": "https://img2.tracker.exemplo.org/upload",
		"file_field": "source",
		"auth_header": "X-API-Key",
		"auth_prefix": "",
		"url_path": "image.url"
	}`
)

func TestTrackerImageHost_PutDeleteEListagem(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, createResp := createTracker(t, h, admin, "Edita ImageHost", fake.srv.URL)

	// Creation returns the profile without the image_host section.
	var created struct {
		Profile map[string]interface{} `json:"profile"`
	}
	tpDecode(t, createResp, &created)
	if _, has := created.Profile["image_host"]; has {
		t.Errorf("profile without section serialized image_host key: %s", createResp)
	}

	// PUT saves and returns the profile.
	code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihSection, id), admin, ihValid)
	if code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	var put struct {
		Family    string `json:"family"`
		ImageHost *struct {
			Name      string `json:"name"`
			UploadURL string `json:"upload_url"`
		} `json:"image_host"`
	}
	tpDecode(t, body, &put)
	if put.Family != "unit3d" || put.ImageHost == nil || put.ImageHost.Name != "Host do site" {
		t.Errorf("PUT response: %s", body)
	}

	// Listing returns the section.
	code, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"image_host"`) {
		t.Errorf("listing does not include section: %d %s", code, body)
	}

	// Editing the taxonomy (the profile PUT) does NOT delete the section.
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/profile", admin, exFullBody); code != http.StatusOK {
		t.Fatalf("PUT profile: %d %s", code, body)
	}
	code, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"image_host"`) {
		t.Errorf("editing taxonomy deleted section: %d %s", code, body)
	}

	// DELETE removes: the section disappears.
	code, body = tpCall(t, h, http.MethodDelete, fmt.Sprintf(ihSection, id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("DELETE: %d %s", code, body)
	}
	code, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if code != http.StatusOK {
		t.Fatalf("listing after DELETE: %d %s", code, body)
	}
	// The section must not appear (or appear as null).
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			Profile struct {
				ImageHost interface{} `json:"image_host"`
			} `json:"profile"`
		} `json:"data"`
	}
	tpDecode(t, body, &list)
	for _, tr := range list.Data {
		if tr.ID == id && tr.Profile.ImageHost != nil {
			t.Errorf("section survived DELETE: %+v", tr.Profile.ImageHost)
		}
	}
	if code, _ := tpCall(t, h, http.MethodDelete, fmt.Sprintf(ihSection, id), admin, ""); code != http.StatusOK {
		t.Errorf("repeated DELETE: %d, want 200 (idempotent)", code)
	}

	// Audit: each write, with the tracker name.
	edits := query(`SELECT action FROM audit_log WHERE path LIKE $1 ORDER BY created_at`, "%/trackers/"+id+"/profile/image-host")
	want := []string{
		"Edited the image host of tracker «Edita ImageHost»",
		"Removed the image host of tracker «Edita ImageHost»",
		"Removed the image host of tracker «Edita ImageHost»",
	}
	if strings.Join(edits, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit:\n got %q\nwant %q", edits, want)
	}

	// No layer 3 credentials in any response.
	for _, secret := range []string{siteToken, sitePasskey, "announce"} {
		if strings.Contains(createResp+body, secret) {
			t.Errorf("a response leaks %q", secret)
		}
	}
}

// The section is untrusted content at EVERY write point: each hostile payload is rejected
// before the database — 400 with the failed field, never 500 — and nothing is saved.
func TestTrackerImageHost_HostilePayloads(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Hostil", fake.srv.URL)
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihSection, id), admin, ihValid); code != http.StatusOK {
		t.Fatalf("initial PUT: %d %s", code, body)
	}

	cases := []struct {
		name, body, path, code string
	}{
		{"upload_url without https", `{"name":"H","upload_url":"http://img.tracker.org/upload","file_field":"file","auth_header":"Authorization","url_path":"url"}`, "image_host.upload_url", "invalid_profile"},
		{"malformed upload_url does not echo value", `{"name":"H","upload_url":"https://a/%zz","file_field":"file","auth_header":"Authorization","url_path":"url"}`, "image_host.upload_url", "invalid_profile"},
		{"upload_url with IP", `{"name":"H","upload_url":"https://192.168.1.1/upload","file_field":"file","auth_header":"Authorization","url_path":"url"}`, "image_host.upload_url", "invalid_profile"},
		{"upload_url with localhost", `{"name":"H","upload_url":"https://localhost/upload","file_field":"file","auth_header":"Authorization","url_path":"url"}`, "image_host.upload_url", "invalid_profile"},
		{"upload_url with port", `{"name":"H","upload_url":"https://img.tracker.org:8080/upload","file_field":"file","auth_header":"Authorization","url_path":"url"}`, "image_host.upload_url", "invalid_profile"},
		{"empty file_field", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"","auth_header":"Authorization","url_path":"url"}`, "image_host.file_field", "invalid_profile"},
		{"file_field with invalid character", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file name","auth_header":"Authorization","url_path":"url"}`, "image_host.file_field", "invalid_profile"},
		{"auth_header proibido (Host)", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Host","url_path":"url"}`, "image_host.auth_header", "invalid_profile"},
		{"auth_header proibido (Content-Type)", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Content-Type","url_path":"url"}`, "image_host.auth_header", "invalid_profile"},
		{"auth_header with prefix Proxy-", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Proxy-Authorization","url_path":"url"}`, "image_host.auth_header", "invalid_profile"},
		{"empty url_path", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Authorization","url_path":""}`, "image_host.url_path", "invalid_profile"},
		{"url_path with invalid segment", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Authorization","url_path":"data.link-url"}`, "image_host.url_path", "invalid_profile"},
		{"thumbnail_path with invalid segment", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Authorization","url_path":"url","thumbnail_path":"thumb-nail"}`, "image_host.thumbnail_path", "invalid_profile"},
		{"wrong type: upload_url is a number", `{"name":"H","upload_url":123,"file_field":"file","auth_header":"Authorization","url_path":"url"}`, "", "invalid_body"},
		{"campo desconhecido", `{"name":"H","upload_url":"https://img.tracker.org/upload","file_field":"file","auth_header":"Authorization","url_path":"url","html":"<script>"}`, "", "invalid_body"},
		{"empty body", ``, "", "invalid_body"},
		{"not JSON", `{name`, "", "invalid_body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihSection, id), admin, tc.body)
			if code != http.StatusBadRequest || !strings.Contains(body, `"`+tc.code+`"`) {
				t.Fatalf("%d %.300s, want 400 %s", code, body, tc.code)
			}
			if tc.path != "" && !strings.Contains(body, tc.path) {
				t.Errorf("rejection does not mention field %q: %.300s", tc.path, body)
			}
			if len(body) > 1000 {
				t.Errorf("rejection returns %d bytes: received value must not be returned", len(body))
			}
			if tc.name == "malformed upload_url does not echo value" {
				if strings.Contains(body, "https://a/%zz") {
					t.Errorf("rejection echoed received URL: %s", body)
				}
				if len(body) > 500 {
					t.Errorf("rejection has %d bytes, above expected limit", len(body))
				}
			}
		})
	}
	// Nothing rejected was saved: the section is the same as before.
	code, body := tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if code != http.StatusOK {
		t.Fatalf("listing: %d %s", code, body)
	}
	if !strings.Contains(body, `"upload_url":"https://img.tracker.exemplo.org/api/v1/sharex/upload"`) {
		t.Errorf("rejected write changed section: %s", body)
	}
}

// RBAC follows the tracker endpoints pattern: writing the section is trackers:write (the Viewer does
// not pass, the owning Publisher passes) and a tracker from another operator does not exist for the requester.
func TestTrackerImageHost_RBAC(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	fake := newSite(t)
	id, _ := createTracker(t, h, tokens["Publisher"], "RBAC secao", fake.srv.URL)
	other, _ := createTracker(t, h, tokens["Administrator"], "Do admin", fake.srv.URL)

	routes := []struct{ method, body string }{{http.MethodPut, ihValid}, {http.MethodDelete, ""}}
	for _, r := range routes {
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(ihSection, id), tokens["Viewer"], r.body); code != http.StatusForbidden {
			t.Errorf("Viewer on %s: %d, want 403", r.method, code)
		}
		if code, body := tpCall(t, h, r.method, fmt.Sprintf(ihSection, id), tokens["Publisher"], r.body); code != http.StatusOK {
			t.Errorf("Publisher owner on %s: %d %.200s, want 200", r.method, code, body)
		}
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(ihSection, other), tokens["Publisher"], r.body); code != http.StatusNotFound {
			t.Errorf("Publisher on admin's tracker, %s: %d, want 404", r.method, code)
		}
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(ihSection, id), "", r.body); code != http.StatusUnauthorized {
			t.Errorf("no token on %s: %d, want 401", r.method, code)
		}
	}
}

// ---- The section travels in export and import ---------------------------------------------------

func TestTrackerImageHost_ProfileExportAndImport(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	from, _ := exFilledTracker(t, h, admin, "Origem ImageHost")
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihSection, from), admin, ihValid); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}

	// The file includes the section.
	code, file, _ := exExport(t, h, admin, from)
	if code != http.StatusOK {
		t.Fatalf("export: %d %s", code, file)
	}
	var doc struct {
		ImageHost *struct {
			Name      string `json:"name"`
			UploadURL string `json:"upload_url"`
		} `json:"image_host"`
	}
	tpDecode(t, file, &doc)
	if doc.ImageHost == nil || doc.ImageHost.Name != "Host do site" {
		t.Fatalf("exported file: %s", file)
	}

	// Destination without section: the preview says what it would apply and does NOT save; the import applies.
	fake := newSite(t)
	to := exTracker(t, h, admin, "Destino ImageHost", fake.srv.URL)
	code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(exPreview, to), admin, file)
	if code != http.StatusOK || !strings.Contains(body, `"image_host":{"status":"new"`) {
		t.Fatalf("preview: %d %s", code, body)
	}
	// Checks that the `to` tracker did not gain the section after the preview (the listing includes
	// `from`, which has the section, so it needs to filter by id).
	_, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if ih := profileImageHostOf(t, body, to); ih != nil {
		t.Errorf("preview wrote section to destination: %+v", ih)
	}
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	_, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	ih := profileImageHostOf(t, body, to)
	if ih == nil || ih.UploadURL != "https://img.tracker.exemplo.org/api/v1/sharex/upload" {
		t.Errorf("import did not apply the section: %+v", ih)
	}

	// The operator's section is protected: separate config, and the import does not swap it without a request.
	if code, b := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihSection, to), admin, ihValid2); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, b)
	}
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK || !strings.Contains(body, `"image_host":{"status":"protected"`) {
		t.Fatalf("import over own config: %d %s", code, body)
	}
	_, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if ih := profileImageHostOf(t, body, to); ih == nil || ih.UploadURL != "https://img2.tracker.exemplo.org/upload" {
		t.Errorf("operator config was overwritten without request: %+v", ih)
	}
	// When requested (?overwrite=image_host), it is swapped.
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to)+"?overwrite=image_host", admin, file)
	if code != http.StatusOK || !strings.Contains(body, `"image_host":{"status":"replaced"`) {
		t.Fatalf("import with overwrite=image_host: %d %s", code, body)
	}
	_, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if ih := profileImageHostOf(t, body, to); ih == nil || ih.UploadURL != "https://img.tracker.exemplo.org/api/v1/sharex/upload" {
		t.Errorf("section was not replaced on request: %+v", ih)
	}

	// Audit: the apply says what it did with the section; the preview is not logged.
	var imports []string
	for _, a := range query(`SELECT action FROM audit_log WHERE path LIKE $1 ORDER BY created_at`, "%/trackers/"+to+"/profile/import") {
		if strings.HasPrefix(a, "Imported a profile from a file into tracker") {
			imports = append(imports, a[strings.LastIndex(a, "; image host"):])
		}
	}
	want := []string{"; image host: applied", "; image host: kept the operator's", "; image host: replaced on request"}
	if strings.Join(imports, "|") != strings.Join(want, "|") {
		t.Errorf("imports audit:\n got %q\nwant %q", imports, want)
	}
}

// profileImageHostOf extracts the image_host of the tracker by id from the listing. nil if the tracker
// does not have the section (or is not in the listing). Used to check the effect of the operations
// without confusing the tested tracker with the source one (which also has the section).
func profileImageHostOf(t *testing.T, body, id string) *struct {
	Name      string `json:"name"`
	UploadURL string `json:"upload_url"`
} {
	t.Helper()
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			Profile struct {
				ImageHost *struct {
					Name      string `json:"name"`
					UploadURL string `json:"upload_url"`
				} `json:"image_host"`
			} `json:"profile"`
		} `json:"data"`
	}
	tpDecode(t, body, &list)
	for _, tr := range list.Data {
		if tr.ID == id {
			return tr.Profile.ImageHost
		}
	}
	return nil
}
