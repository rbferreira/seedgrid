package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestScreenshotRender_TrackerFromOtherOperator ensures that the /screenshots/render route responds
// 404 when the tracker_id belongs to another operator (same pattern as profile routes: a tracker
// from another operator does not exist for the requester).
func TestScreenshotRender_TrackerFromOtherOperator(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	pub := tokens["Publisher"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Tracker do Admin", fake.srv.URL)

	// Publisher (another operator) tries to render with the admin's tracker_id. The command
	// is fake, but tracker validation comes before anything else in the chain.
	body := fmt.Sprintf(`{"command_id":"00000000-0000-0000-0000-000000000000","tracker_id":%q}`, id)
	code, resp := tpCall(t, h, http.MethodPost, "/api/v1/screenshots/render", pub, body)
	if code != http.StatusNotFound || !strings.Contains(resp, "tracker not found") {
		t.Errorf("render with tracker from another operator: %d %s, want 404 from tracker", code, resp)
	}
}

// TestScreenshotGenerate_TrackerFromOtherOperator ensures that the /torrents/{id}/screenshots route
// returns 404 when the tracker_id belongs to another operator.
func TestScreenshotGenerate_TrackerFromOtherOperator(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	pub := tokens["Publisher"]
	fake := newSite(t)
	trackerID, _ := createTracker(t, h, admin, "Tracker do Admin Gen", fake.srv.URL)

	// The Publisher points to the admin's tracker. The torrent doesn't even exist: the 404 must be
	// from the TRACKER, proving that ownership is checked before any work on the torrent.
	body := fmt.Sprintf(`{"tracker_id":%q}`, trackerID)
	code, resp := tpCall(t, h, http.MethodPost,
		"/api/v1/torrents/00000000-0000-0000-0000-000000000000/screenshots", pub, body)
	if code != http.StatusNotFound || !strings.Contains(resp, "tracker not found") {
		t.Errorf("generate with tracker from another operator: %d %s, want 404 from tracker", code, resp)
	}
}

// TestScreenshotRender_BodyWithoutTrackerID ensures that a body with {command_id} but no tracker_id
// is still accepted (legacy behavior — just imgbb). The command doesn't exist, so the route fails
// with 502 screenshots_failed, NOT with a 400 validation error.
func TestScreenshotRender_BodyWithoutTrackerID(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]

	code, _ := tpCall(t, h, http.MethodPost, "/api/v1/screenshots/render", admin,
		`{"command_id":"00000000-0000-0000-0000-000000000000"}`)
	if code == http.StatusBadRequest {
		t.Errorf("render without tracker_id should not be 400: %d", code)
	}
}

// TestScreenshotGenerate_EmptyBody ensures that the route accepts an empty body (legacy
// behavior). The torrent doesn't exist → 404 torrent not_found, NOT a 400 validation error.
func TestScreenshotGenerate_EmptyBody(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]

	code, _ := tpCall(t, h, http.MethodPost,
		"/api/v1/torrents/00000000-0000-0000-0000-000000000000/screenshots", admin, "")
	if code != http.StatusNotFound {
		t.Errorf("generate with empty body and non-existent torrent: status %d, want 404 not_found", code)
	}
}

// TestScreenshotRoutes_MalformedTrackerID: a tracker_id that isn't a UUID returns 404, not 500 —
// and it happens before touching the command or the torrent.
func TestScreenshotRoutes_MalformedTrackerID(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]

	code, resp := tpCall(t, h, http.MethodPost, "/api/v1/screenshots/render", admin,
		`{"command_id":"00000000-0000-0000-0000-000000000000","tracker_id":"not-a-uuid"}`)
	if code != http.StatusNotFound || !strings.Contains(resp, "tracker not found") {
		t.Errorf("render: %d %s, want tracker 404", code, resp)
	}
	code, resp = tpCall(t, h, http.MethodPost,
		"/api/v1/torrents/00000000-0000-0000-0000-000000000000/screenshots", admin, `{"tracker_id":"not-a-uuid"}`)
	if code != http.StatusNotFound || !strings.Contains(resp, "tracker not found") {
		t.Errorf("generate: %d %s, want tracker 404", code, resp)
	}
}

// TestScreenshotGenerate_InvalidBody: empty body is valid (legacy behavior), but an unexpected
// body returns 400 — it is not silently ignored.
func TestScreenshotGenerate_InvalidBody(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]

	for _, body := range []string{`{"tracker":"x"}`, `not-json`} {
		code, _ := tpCall(t, h, http.MethodPost,
			"/api/v1/torrents/00000000-0000-0000-0000-000000000000/screenshots", admin, body)
		if code != http.StatusBadRequest {
			t.Errorf("generate with body %q: status %d, want 400", body, code)
		}
	}
}
