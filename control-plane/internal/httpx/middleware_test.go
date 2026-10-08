package httpx

import (
	"net/http"
	"testing"
)

// TestFriendlyAction covers the friendly PT-BR audit labels (ADR-025), focusing on
// cases that were wrong (/seed came as "Managed trackers") and on the fallback.
func TestFriendlyAction(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{"POST", "/api/v1/trackers/abc/seed", "Seeded a torrent from the tracker"},
		{"POST", "/api/v1/trackers/abc/default", "Set the default tracker"},
		{"POST", "/api/v1/trackers", "Registered a tracker"},
		{"PUT", "/api/v1/trackers/abc", "Edited a tracker"},
		{"DELETE", "/api/v1/trackers/abc", "Removed a tracker"},
		// Site profile (#196): the profile PUT MUST NOT fall into "Edited a tracker" — they are
		// different actions, and the trail exists to distinguish a credential edit from
		// a taxonomy edit.
		{"PUT", "/api/v1/trackers/abc/profile", "Edited a tracker's profile"},
		{"POST", "/api/v1/trackers/abc/apply-taxonomy", "Applied the discovered taxonomy to a tracker"},
		{"POST", "/api/v1/trackers/abc/profile/import", "Imported a tracker profile"},
		// Description composition (#192) has own labels: without them DELETE would fall into
		// "Removed a tracker" and PUT into "Edited a tracker" — in a denied attempt (403), which is
		// what the trail logs by the route label, this would say the wrong thing.
		{"PUT", "/api/v1/trackers/abc/profile/description", "Edited a tracker's description composition"},
		{"DELETE", "/api/v1/trackers/abc/profile/description", "Reset a tracker's description composition to default"},
		{"PUT", "/api/v1/trackers/abc/profile/image-host", "Edited a tracker's image host"},
		{"DELETE", "/api/v1/trackers/abc/profile/image-host", "Removed a tracker's image host"},
		{"PUT", "/api/v1/trackers/abc/image-host-key", "Registered a tracker's image host key"},
		{"DELETE", "/api/v1/trackers/abc/image-host-key", "Removed a tracker's image host key"},
		{"POST", "/api/v1/torrents/abc/pause", "Paused a torrent"},
		{"DELETE", "/api/v1/torrents/abc", "Removed a torrent"},
		{"POST", "/api/v1/torrents", "Added a torrent"},
		{"DELETE", "/api/v1/nodes/abc", "Removed a Node"},
		{"PUT", "/api/v1/settings/admission.interval", "Changed a setting"},
		{"POST", "/api/v1/enroll-tokens", "Generated a join token"},
		// The nickname PUT (#145) has its own label: a denied attempt (403) never reaches
		// the handler, so this is the label the trail logs — and not "PUT nodes/abc/display-name".
		{"PUT", "/api/v1/nodes/abc/display-name", "Renamed a Node"},
		// Fallback: route without dedicated label falls to method + trimmed path.
		{"POST", "/api/v1/something/new", "POST something/new"},
	}
	for _, c := range cases {
		if got := friendlyAction(c.method, c.path); got != c.want {
			t.Errorf("friendlyAction(%s %s) = %q, want %q", c.method, c.path, got, c.want)
		}
	}
}

// TestSkipAuditPath: queries (publish/infer) are not audited; real mutations are.
func TestSkipAuditPath(t *testing.T) {
	if !skipAuditPath("/api/v1/publish/infer") {
		t.Error("publish/infer should be skipped (it's a query, not a mutation)")
	}
	// Composing description (#192) only builds text from the body: it's a query, like infer.
	if !skipAuditPath("/api/v1/publish/description") {
		t.Error("publish/description should be skipped (only composes text, does not mutate state)")
	}
	if skipAuditPath("/api/v1/trackers/abc/profile/description") {
		t.Error("trackers/{id}/profile/description should NOT be skipped (mutates the profile)")
	}
	if skipAuditPath("/api/v1/trackers/abc/seed") {
		t.Error("/seed should NOT be skipped (mutates state — adds torrent)")
	}
	// Discovering taxonomy only PROPOSES (#196); applying and editing profile are actions.
	if !skipAuditPath("/api/v1/trackers/abc/discover-taxonomy") {
		t.Error("discover-taxonomy should be skipped (reads the catalog and writes nothing)")
	}
	// Previewing profile import (#197) also only reads; the import itself changes profile.
	if !skipAuditPath("/api/v1/trackers/abc/profile/import/preview") {
		t.Error("profile/import/preview should be skipped (validates the file and writes nothing)")
	}
	for _, p := range []string{"/api/v1/trackers/abc/apply-taxonomy", "/api/v1/trackers/abc/profile", "/api/v1/trackers/abc/profile/import"} {
		if skipAuditPath(p) {
			t.Errorf("%s should NOT be skipped (mutates the profile)", p)
		}
	}
}

// TestIsMutatingMethod: only methods that change state are audited.
func TestIsMutatingMethod(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !isMutatingMethod(m) {
			t.Errorf("%s should be mutating", m)
		}
	}
	if isMutatingMethod(http.MethodGet) {
		t.Error("GET should not be mutating")
	}
}
