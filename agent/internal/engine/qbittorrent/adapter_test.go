package qbittorrent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// captureSetPreferences spins up a fake server that responds 200 and captures the body
// JSON sent to /api/v2/app/setPreferences.
func captureSetPreferences(t *testing.T) (*httptest.Server, *map[string]any) {
	t.Helper()
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/app/setPreferences" {
			_ = r.ParseForm()
			if raw := r.PostFormValue("json"); raw != "" {
				_ = json.Unmarshal([]byte(raw), &got)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	return srv, &got
}

func TestConfigureEngine_QueueingOn(t *testing.T) {
	srv, got := captureSetPreferences(t)
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	ec := EngineConfig{
		BTPort: 6881, SavePath: "/data", QueueingEnabled: true,
		MaxActiveDownloads: 3, MaxActiveUploads: 8, MaxActiveTorrents: 40, MaxActiveCheckingTorrents: 1,
		DontCountSlowTorrents: true, SlowTorrentDlRateThreshold: 1, SlowTorrentUlRateThreshold: 1,
		SlowTorrentInactiveTimer: 300,
	}
	if err := a.ConfigureEngine(context.Background(), ec); err != nil {
		t.Fatalf("ConfigureEngine: %v", err)
	}
	// Numbers arrive as float64 after json.Unmarshal into any.
	want := map[string]any{
		"listen_port":                    float64(6881),
		"save_path":                      "/data",
		"upnp":                           false,
		"queueing_enabled":               true,
		"max_active_downloads":           float64(3),
		"max_active_uploads":             float64(8),
		"max_active_torrents":            float64(40),
		"max_active_checking_torrents":   float64(1),
		"dont_count_slow_torrents":       true,
		"slow_torrent_dl_rate_threshold": float64(1),
		"slow_torrent_inactive_timer":    float64(300),
	}
	for k, v := range want {
		if (*got)[k] != v {
			t.Errorf("pref %q = %v (%T); want %v (%T)", k, (*got)[k], (*got)[k], v, v)
		}
	}
}

func TestConfigureEngine_QueueingOff(t *testing.T) {
	srv, got := captureSetPreferences(t)
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	ec := EngineConfig{BTPort: 6882, QueueingEnabled: false, MaxActiveDownloads: 3}
	if err := a.ConfigureEngine(context.Background(), ec); err != nil {
		t.Fatalf("ConfigureEngine: %v", err)
	}
	if (*got)["queueing_enabled"] != false {
		t.Errorf("queueing_enabled = %v; want false", (*got)["queueing_enabled"])
	}
	// With the queue disabled, the limits must not be sent.
	for _, k := range []string{"max_active_downloads", "max_active_checking_torrents", "dont_count_slow_torrents"} {
		if _, ok := (*got)[k]; ok {
			t.Errorf("pref %q should not be sent with queueing off", k)
		}
	}
}

// ConfigureQueue (ADR-039) hot-reapplies the queue: ONLY the queue. BT port and save_path are from the
// machine — matching the operator's port forward and the mounted disk — and a change in the download
// limit coming from the Control Plane has no reason to resend them.
func TestConfigureQueue_OnlyQueue(t *testing.T) {
	srv, got := captureSetPreferences(t)
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	ec := EngineConfig{
		BTPort: 6884, SavePath: "/data", QueueingEnabled: true,
		MaxActiveDownloads: 3, MaxActiveUploads: 10000, MaxActiveTorrents: 10000, MaxActiveCheckingTorrents: 1,
		DontCountSlowTorrents: false, SlowTorrentDlRateThreshold: 1, SlowTorrentUlRateThreshold: 1, SlowTorrentInactiveTimer: 300,
	}
	if err := a.ConfigureQueue(context.Background(), ec); err != nil {
		t.Fatalf("ConfigureQueue: %v", err)
	}
	if (*got)["max_active_downloads"] != float64(3) || (*got)["queueing_enabled"] != true {
		t.Errorf("prefs = %v, want the queue with downloads=3", *got)
	}
	for _, k := range []string{"listen_port", "save_path", "upnp", "random_port"} {
		if _, ok := (*got)[k]; ok {
			t.Errorf("ConfigureQueue sent %q — it is machine-specific, not from the cluster configuration", k)
		}
	}
}

func TestConfigureQueue_QueueDisabledOnlySends_QueueingEnabled(t *testing.T) {
	srv, got := captureSetPreferences(t)
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	if err := a.ConfigureQueue(context.Background(), EngineConfig{QueueingEnabled: false, MaxActiveDownloads: 3}); err != nil {
		t.Fatalf("ConfigureQueue: %v", err)
	}
	if len(*got) != 1 || (*got)["queueing_enabled"] != false {
		t.Errorf("prefs = %v, want only queueing_enabled=false", *got)
	}
}

// Engine failure returns as an error (the Manager reapplies it on the next heartbeat).
func TestConfigureQueue_EngineError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	a := New(srv.URL, "admin", "pw")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.ConfigureQueue(ctx, EngineConfig{QueueingEnabled: true, MaxActiveDownloads: 3}); err == nil {
		t.Fatal("expected error with Engine returning 500")
	}
}

// TestAddFileTo_FixesSavepathAndLayout ensures that the migration (ADR-018) always
// sends savepath + content_layout=Original + autoTMM=false — otherwise the
// Engine might choose another layout on re-add and not find the data that the
// Pull() already extracted, triggering a redundant download (real bug observed
// during the migration of "The Boy and the Heron" to SG-1, which duplicated 2.9GB).
func TestAddFileTo_FixesSavepathAndLayout(t *testing.T) {
	got := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/add" {
			_ = r.ParseMultipartForm(1 << 20) //nolint:gosec // G120: 1MB is already the explicit limit
			for _, k := range []string{"savepath", "contentLayout", "autoTMM"} {
				got[k] = r.FormValue(k)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	if err := a.AddFileTo(context.Background(), []byte("d4:infod4:name3:fooee"), "/data"); err != nil {
		t.Fatalf("AddFileTo: %v", err)
	}
	want := map[string]string{"savepath": "/data", "contentLayout": "Original", "autoTMM": "false"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestMapState(t *testing.T) {
	cases := map[string]string{
		"uploading":          "SEEDING",
		"stalledUP":          "SEEDING",
		"forcedUP":           "SEEDING",
		"queuedUP":           "SEEDING", // complete, waiting for an upload slot (queueing) — NOT QUEUED
		"queuedDL":           "QUEUED",  // waiting to download
		"downloading":        "DOWNLOADING",
		"metaDL":             "DOWNLOADING",
		"pausedUP":           "PAUSED",
		"stoppedDL":          "PAUSED",
		"error":              "ERROR",
		"missingFiles":       "ERROR",
		"checkingResumeData": "CHECKING",
		"moving":             "MOVING",
		"algonovo":           "UNKNOWN",
	}
	for in, want := range cases {
		if got := MapState(in); got != want {
			t.Errorf("MapState(%q) = %q, want %q", in, got, want)
		}
	}
}
