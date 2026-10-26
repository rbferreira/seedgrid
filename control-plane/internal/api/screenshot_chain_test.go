package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestScreenshotRoutes_ClusterTracker: trackers belong to the cluster (ADR-047), so the
// Publisher renders and generates with the tracker the Administrator registered — the tracker
// check passes and the request fails further on, on the fake command or torrent. A tracker id
// that does not exist is a 404 from the TRACKER, before any work on the command or torrent.
func TestScreenshotRoutes_ClusterTracker(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	pub := tokens["Publisher"]
	fake := newSite(t)
	id, _ := createTracker(t, h, tokens["Administrator"], "Tracker do Admin", fake.srv.URL)
	const unknown = "00000000-0000-0000-0000-0000000000aa"

	render := func(trackerID string) (int, string) {
		body := fmt.Sprintf(`{"command_id":"00000000-0000-0000-0000-000000000000","tracker_id":%q}`, trackerID)
		return tpCall(t, h, http.MethodPost, "/api/v1/screenshots/render", pub, body)
	}
	generate := func(trackerID string) (int, string) {
		return tpCall(t, h, http.MethodPost, "/api/v1/torrents/00000000-0000-0000-0000-000000000000/screenshots",
			pub, fmt.Sprintf(`{"tracker_id":%q}`, trackerID))
	}

	if code, resp := render(id); code == http.StatusForbidden || strings.Contains(resp, "tracker not found") {
		t.Errorf("Publisher rendering with the cluster's tracker: %d %s, want past the tracker check", code, resp)
	}
	if code, resp := generate(id); code != http.StatusNotFound || !strings.Contains(resp, "torrent not found") {
		t.Errorf("Publisher generating with the cluster's tracker: %d %s, want 404 from the torrent", code, resp)
	}
	if code, resp := render(unknown); code != http.StatusNotFound || !strings.Contains(resp, "tracker not found") {
		t.Errorf("render with an unknown tracker: %d %s, want 404 from tracker", code, resp)
	}
	if code, resp := generate(unknown); code != http.StatusNotFound || !strings.Contains(resp, "tracker not found") {
		t.Errorf("generate with an unknown tracker: %d %s, want 404 from tracker", code, resp)
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
