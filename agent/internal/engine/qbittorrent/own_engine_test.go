package qbittorrent

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
)

// TestPaths_EngineInAContainer (ADR-043): with the Engine in a container that mounts
// /mnt/user/torrents as /downloads, the paths it reports reach the Agent as host paths, and the
// paths the Agent gives it reach it as container paths.
func TestPaths_EngineInAContainer(t *testing.T) {
	var savepath, sourcePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			_, _ = io.WriteString(w, `[{"hash":"ABC","name":"Movie","state":"uploading","content_path":"/downloads/Movie"}]`)
		case "/api/v2/torrents/add":
			_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				if part.FormName() == "savepath" {
					b, _ := io.ReadAll(part)
					savepath = string(b)
				}
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v2/torrentcreator/addTask":
			_ = r.ParseForm()
			sourcePath = r.PostFormValue("sourcePath")
			w.WriteHeader(http.StatusBadRequest) // stops CreateTorrent here: only the path matters
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	m, err := pathmap.Parse("/downloads=/mnt/user/torrents")
	if err != nil {
		t.Fatal(err)
	}
	a := New(srv.URL, "admin", "pw").WithPaths(m)
	ctx := context.Background()

	got, err := a.ContentPathOf(ctx, "abc")
	if err != nil || got != "/mnt/user/torrents/Movie" {
		t.Errorf("ContentPathOf = (%q, %v), want the host path", got, err)
	}
	if err := a.AddFileTo(ctx, []byte("d8:announce0:e"), "/mnt/user/torrents"); err != nil {
		t.Fatalf("AddFileTo: %v", err)
	}
	if savepath != "/downloads" {
		t.Errorf("savepath sent to the Engine = %q, want /downloads", savepath)
	}
	_, _ = a.CreateTorrent(ctx, "/mnt/user/torrents/New", nil, false, "")
	if sourcePath != "/downloads/New" {
		t.Errorf("sourcePath sent to the Engine = %q, want /downloads/New", sourcePath)
	}
}

// TestReadQueue (ADR-043): the queue a Node that joined with its own qBittorrent reports on its
// first contact is what the Engine has, as qBittorrent has it (-1 = no limit).
func TestReadQueue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/app/preferences" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"listen_port":51413,"queueing_enabled":true,"max_active_downloads":3,
			"max_active_uploads":-1,"max_active_torrents":5,"max_active_checking_torrents":1,
			"dont_count_slow_torrents":true,"slow_torrent_dl_rate_threshold":2,
			"slow_torrent_ul_rate_threshold":2,"slow_torrent_inactive_timer":60}`)
	}))
	defer srv.Close()

	got, err := New(srv.URL, "admin", "pw").ReadQueue(context.Background())
	if err != nil {
		t.Fatalf("ReadQueue: %v", err)
	}
	want := EngineConfig{
		QueueingEnabled: true, MaxActiveDownloads: 3, MaxActiveUploads: -1, MaxActiveTorrents: 5,
		MaxActiveCheckingTorrents: 1, DontCountSlowTorrents: true, SlowTorrentDlRateThreshold: 2,
		SlowTorrentUlRateThreshold: 2, SlowTorrentInactiveTimer: 60,
	}
	if got != want {
		t.Errorf("ReadQueue = %+v, want %+v", got, want)
	}
}

// TestConfigureEngine_SavePathInAContainer (ADR-043): the Agent pins save_path to its storage,
// a host folder; an Engine in a container gets the container's path for it.
func TestConfigureEngine_SavePathInAContainer(t *testing.T) {
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/app/setPreferences" {
			_ = r.ParseForm()
			sent = r.PostFormValue("json")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m, err := pathmap.Parse("/downloads=/mnt/user/torrents")
	if err != nil {
		t.Fatal(err)
	}
	a := New(srv.URL, "admin", "pw").WithPaths(m)
	if err := a.ConfigureEngine(context.Background(), EngineConfig{BTPort: 51413, SavePath: "/mnt/user/torrents"}); err != nil {
		t.Fatalf("ConfigureEngine: %v", err)
	}
	if !strings.Contains(sent, `"save_path":"/downloads"`) || !strings.Contains(sent, `"listen_port":51413`) {
		t.Errorf("preferences sent = %s, want save_path /downloads and the port", sent)
	}
}
