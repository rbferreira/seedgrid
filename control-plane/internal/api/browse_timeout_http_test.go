package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/api"
)

// REGRESSION #41 — a tracker that hangs in the catalog browse becomes a 504 with the reason.
//
// The same defect as #35, in Explore: the browse inherited the http.Client's 30s, the same value
// as the WriteTimeout, and the operator got the proxy's generic 502.
func TestBrowseTracker_HangingTrackerBecomes504(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	defer api.SetBrowseBudget(300 * time.Millisecond)()

	var requests atomic.Int32
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	t.Cleanup(hang.Close)
	id, _ := createTracker(t, h, admin, "Hang", hang.URL)

	start := time.Now()
	code, body := tpCall(t, h, http.MethodGet, "/api/v1/trackers/"+id+"/browse", admin, "")
	if code != http.StatusGatewayTimeout || !strings.Contains(body, "tracker_timeout") {
		t.Fatalf("browse: %d %s, want 504 tracker_timeout", code, body)
	}
	if !strings.Contains(body, `"seconds"`) {
		t.Errorf("error.params lacks the seconds the phrase uses: %s", body)
	}
	if strings.Contains(body, siteToken) || strings.Contains(body, "api_token") {
		t.Errorf("the error response leaks the token: %s", body)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s; the browse deadline was not respected", d)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("expected 1 request to the tracker, got %d", n)
	}
}

// The duplicate search lists the same catalog and promises never to fail upward: with the
// tracker hanging, it answers 200 with the note, within the deadline.
func TestTrackerDupes_HangingTrackerAnswersWithNote(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	defer api.SetBrowseBudget(300 * time.Millisecond)()

	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(hang.Close)
	id, _ := createTracker(t, h, admin, "Hang", hang.URL)

	start := time.Now()
	code, body := tpCall(t, h, http.MethodGet, "/api/v1/trackers/"+id+"/dupes?name=Inception.2010.1080p", admin, "")
	if code != http.StatusOK || !strings.Contains(body, "note") {
		t.Fatalf("dupes: %d %s, want 200 with note", code, body)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s; the duplicate search deadline was not respected", d)
	}
}
