package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The image host key (#193 / ADR-040), via the full API against the test database:
// PUT and DELETE /trackers/{id}/image-host-key (trackers:write).

const ihkPath = "/api/v1/trackers/%s/image-host-key"

// ihkKey is the value of the key used in tests — searched textually in the responses and
// in the audit to ensure it never leaks.
const ihkKey = "CHAVE-SECRETA-DO-OPERADOR" //nolint:gosec // G101: fake test credential

func TestTrackerImageHostKey_PutDeleteEListagem(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, createResp := createTracker(t, h, admin, "ImageHost Key", fake.srv.URL)

	// Creation returns the tracker without has_image_host_key.
	var created struct {
		HasImageHostKey bool `json:"has_image_host_key"`
	}
	tpDecode(t, createResp, &created)
	if created.HasImageHostKey {
		t.Errorf("newly created tracker has has_image_host_key=true: %s", createResp)
	}

	// Needs to have the image_host section first (ihValid is in trackerprofile_imagehost_http_test.go).
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf("/api/v1/trackers/%s/profile/image-host", id), admin, ihValid); code != http.StatusOK {
		t.Fatalf("PUT image-host: %d %s", code, body)
	}

	// PUT saves the key.
	code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, id), admin, fmt.Sprintf(`{"key":%q}`, ihkKey))
	if code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}

	// Listing returns has_image_host_key=true and image_host_key_host.
	code, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if code != http.StatusOK {
		t.Fatalf("listing: %d %s", code, body)
	}
	if !strings.Contains(body, `"has_image_host_key":true`) {
		t.Errorf("listing does not include has_image_host_key=true: %s", body)
	}
	if !strings.Contains(body, `"image_host_key_host":"img.tracker.exemplo.org"`) {
		t.Errorf("listing does not include image_host_key_host: %s", body)
	}
	// The key NEVER appears in the response.
	if strings.Contains(body, ihkKey) {
		t.Errorf("key leaked in listing: %s", body)
	}

	// DELETE removes the key.
	code, body = tpCall(t, h, http.MethodDelete, fmt.Sprintf(ihkPath, id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("DELETE: %d %s", code, body)
	}

	// Listing after DELETE: has_image_host_key=false and image_host_key_host="".
	code, body = tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if code != http.StatusOK {
		t.Fatalf("listing after DELETE: %d %s", code, body)
	}
	if !strings.Contains(body, `"has_image_host_key":false`) {
		t.Errorf("key was not removed: %s", body)
	}
	if !strings.Contains(body, `"image_host_key_host":""`) {
		t.Errorf("key host was not cleared: %s", body)
	}

	// DELETE is idempotent.
	if code, _ := tpCall(t, h, http.MethodDelete, fmt.Sprintf(ihkPath, id), admin, ""); code != http.StatusOK {
		t.Errorf("repeated DELETE: %d, want 200 (idempotent)", code)
	}

	// Audit: each write, with the host but NEVER the key.
	edits := query(`SELECT action FROM audit_log WHERE path LIKE $1 ORDER BY created_at`, "%/trackers/"+id+"/image-host-key")
	want := []string{
		"Saved the key for image host «img.tracker.exemplo.org» on tracker «ImageHost Key»",
		"Removed the image host key of tracker «ImageHost Key»",
		"Removed the image host key of tracker «ImageHost Key»",
	}
	if strings.Join(edits, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit:\n got %q\nwant %q", edits, want)
	}
	// The key never appeared in the audit.
	for _, a := range edits {
		if strings.Contains(a, ihkKey) {
			t.Errorf("key leaked in audit: %q", a)
		}
	}
}

// Empty key is 400; profile without image_host is 409; nonexistent tracker is 404.
func TestTrackerImageHostKey_ErrorsAndEdges(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Key Validation", fake.srv.URL)

	// Tracker without image_host section: 409.
	code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, id), admin, `{"key": "CHAVE"}`)
	if code != http.StatusConflict || !strings.Contains(body, "profile_without_image_host") {
		t.Fatalf("PUT without section: %d %s, want 409 profile_without_image_host", code, body)
	}

	// Sets up the section.
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf("/api/v1/trackers/%s/profile/image-host", id), admin, ihValid); code != http.StatusOK {
		t.Fatalf("PUT image-host: %d %s", code, body)
	}

	// Empty key: 400.
	code, body = tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, id), admin, `{"key": ""}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "invalid_request") {
		t.Errorf("PUT empty key: %d %s, want 400 invalid_request", code, body)
	}

	// Key with only spaces: 400.
	code, body = tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, id), admin, `{"key": "   "}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "invalid_request") {
		t.Errorf("PUT blank key: %d %s, want 400 invalid_request", code, body)
	}

	// Empty body: 400.
	code, body = tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, id), admin, ``)
	if code != http.StatusBadRequest || !strings.Contains(body, "invalid_body") {
		t.Errorf("PUT empty body: %d %s, want 400 invalid_body", code, body)
	}

	// Non-existent tracker: 404.
	code, _ = tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, "00000000-0000-0000-0000-000000000000"), admin, `{"key": "CHAVE"}`)
	if code != http.StatusNotFound {
		t.Errorf("PUT nonexistent tracker: %d, want 404", code)
	}
	code, _ = tpCall(t, h, http.MethodDelete, fmt.Sprintf(ihkPath, "00000000-0000-0000-0000-000000000000"), admin, "")
	if code != http.StatusNotFound {
		t.Errorf("DELETE nonexistent tracker: %d, want 404", code)
	}

	// Malformed ID: 404.
	code, _ = tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, "not-a-uuid"), admin, `{"key": "CHAVE"}`)
	if code != http.StatusNotFound {
		t.Errorf("PUT with malformed ID: %d, want 404", code)
	}
	code, _ = tpCall(t, h, http.MethodDelete, fmt.Sprintf(ihkPath, "not-a-uuid"), admin, "")
	if code != http.StatusNotFound {
		t.Errorf("DELETE with malformed ID: %d, want 404", code)
	}

	// Key with spaces/\n at the ends is saved trimmed.
	code, body = tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, id), admin, `{"key": "  \n TRIMMED-KEY \n  "}`)
	if code != http.StatusOK {
		t.Errorf("PUT with key containing spaces: %d %s, want 200", code, body)
	}

	// In the database, the key is without the spaces and line breaks at the ends.
	if got := query(`SELECT image_host_key FROM trackers WHERE id = $1`, id); len(got) != 1 || got[0] != "TRIMMED-KEY" {
		t.Errorf("stored key = %q, want [TRIMMED-KEY]", got)
	}
}

// PUT /api/v1/trackers/{id} (tracker edit) returns has_image_host_key=true when there is a key.
func TestTrackerImageHostKey_EditReturnsField(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Key Edit", fake.srv.URL)

	// Sets up the section and saves the key.
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf("/api/v1/trackers/%s/profile/image-host", id), admin, ihValid); code != http.StatusOK {
		t.Fatalf("PUT image-host: %d %s", code, body)
	}
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(ihkPath, id), admin, `{"key": "CHAVE"}`); code != http.StatusOK {
		t.Fatalf("PUT key: %d %s", code, body)
	}

	// Editing the tracker (PUT) returns has_image_host_key=true.
	code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id, admin, `{"name": "Edição Chave Renomeado", "announce": "https://announce.exemplo.org/passkey"}`)
	if code != http.StatusOK {
		t.Fatalf("PUT tracker: %d %s", code, body)
	}
	if !strings.Contains(body, `"has_image_host_key":true`) {
		t.Errorf("edit does not return has_image_host_key=true: %s", body)
	}
	// The key itself never appears.
	if strings.Contains(body, "CHAVE") {
		t.Errorf("key leaked in edit: %s", body)
	}
}

// RBAC as per tracker endpoints: writing the key is trackers:write (the Viewer does
// not pass, the owner Publisher passes) and a tracker from another operator does not exist for the requester.
func TestTrackerImageHostKey_RBAC(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	fake := newSite(t)
	id, _ := createTracker(t, h, tokens["Publisher"], "RBAC key", fake.srv.URL)
	other, _ := createTracker(t, h, tokens["Administrator"], "Do admin", fake.srv.URL)

	// Sets up the section for both.
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf("/api/v1/trackers/%s/profile/image-host", id), tokens["Publisher"], ihValid); code != http.StatusOK {
		t.Fatalf("PUT image-host: %d %s", code, body)
	}
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf("/api/v1/trackers/%s/profile/image-host", other), tokens["Administrator"], ihValid); code != http.StatusOK {
		t.Fatalf("PUT image-host: %d %s", code, body)
	}

	routes := []struct{ method, body string }{{http.MethodPut, `{"key": "CHAVE"}`}, {http.MethodDelete, ""}}
	for _, r := range routes {
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(ihkPath, id), tokens["Viewer"], r.body); code != http.StatusForbidden {
			t.Errorf("Viewer on %s: %d, want 403", r.method, code)
		}
		if code, body := tpCall(t, h, r.method, fmt.Sprintf(ihkPath, id), tokens["Publisher"], r.body); code != http.StatusOK {
			t.Errorf("Publisher owner on %s: %d %.200s, want 200", r.method, code, body)
		}
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(ihkPath, other), tokens["Publisher"], r.body); code != http.StatusNotFound {
			t.Errorf("Publisher on admin's tracker, %s: %d, want 404", r.method, code)
		}
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(ihkPath, id), "", r.body); code != http.StatusUnauthorized {
			t.Errorf("no token on %s: %d, want 401", r.method, code)
		}
	}
}
