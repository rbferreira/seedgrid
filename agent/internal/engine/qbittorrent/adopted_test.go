package qbittorrent

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
)

// TestStartStop_QBittorrent4 (ADR-043): an adopted Engine may be a qBittorrent 4.x, which has
// no torrents/stop or torrents/start (404) and calls them pause and resume. The adapter falls
// back to the 4.x name, and after the first 404 goes there directly.
func TestStartStop_QBittorrent4(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/torrents/pause", "/api/v2/torrents/resume":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	ctx := context.Background()
	if err := a.Stop(ctx, "abc"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := a.Start(ctx, "abc"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	want := []string{"/api/v2/torrents/stop", "/api/v2/torrents/pause", "/api/v2/torrents/resume"}
	if strings.Join(calls, " ") != strings.Join(want, " ") {
		t.Errorf("calls = %v, want %v", calls, want)
	}
}

// TestStartStop_QBittorrent5 keeps the 5.x names on a 5.x Engine: no extra request.
func TestStartStop_QBittorrent5(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	if err := a.Stop(context.Background(), "abc"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(calls) != 1 || calls[0] != "/api/v2/torrents/stop" {
		t.Errorf("calls = %v, want only torrents/stop", calls)
	}
}

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

func TestListenPort(t *testing.T) {
	body := `{"listen_port":51413,"save_path":"/downloads/"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/app/preferences" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	a := New(srv.URL, "admin", "pw")
	if port, err := a.ListenPort(context.Background()); err != nil || port != 51413 {
		t.Errorf("ListenPort = (%d, %v), want 51413", port, err)
	}
	body = `{"listen_port":0}`
	if _, err := a.ListenPort(context.Background()); err == nil {
		t.Error("listen_port 0 was accepted")
	}
}
