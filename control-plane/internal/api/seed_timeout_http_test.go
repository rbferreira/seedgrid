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

// REGRESSION #35 — a tracker that hangs during the seed becomes a 504 with the reason, within the
// deadline.
//
// Before, the route inherited the http.Client's 60s (twice, with the second attempt), the server
// could no longer write after 30s and the operator got the proxy's generic 502. Now talking to the
// tracker has its own deadline and the timeout has its own code.
func TestSeedFromTracker_HangingTrackerBecomes504(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	defer api.SetSeedFetchBudget(300 * time.Millisecond)()

	var requests atomic.Int32
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	t.Cleanup(hang.Close)
	id, _ := createTracker(t, h, admin, "Hang", hang.URL)

	start := time.Now()
	code, body := tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/seed", admin, `{"torrent_id":"42"}`)
	if code != http.StatusGatewayTimeout || !strings.Contains(body, "tracker_timeout") {
		t.Fatalf("seed: %d %s, want 504 tracker_timeout", code, body)
	}
	if !strings.Contains(body, `"seconds"`) {
		t.Errorf("error.params lacks the seconds the phrase uses: %s", body)
	}
	if strings.Contains(body, siteToken) || strings.Contains(body, "api_token") {
		t.Errorf("the error response leaks the token: %s", body)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s; the seed deadline was not respected", d)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("expected 1 request to the tracker, got %d", n)
	}
}
