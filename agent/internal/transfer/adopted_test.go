package transfer

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestServe_FromAMappedRoot (ADR-043): on an adopted Engine, a torrent may live in another of
// the Engine's directories, outside the Node's storage. A directory added with WithRoots can be
// served; one that is not stays refused.
func TestServe_FromAMappedRoot(t *testing.T) {
	storage := t.TempDir()
	mapped := t.TempDir()
	content := writeTree(t, filepath.Join(mapped, "other"), map[string]string{"x.txt": "data"})

	newServer := func(roots ...string) *httptest.Server {
		srv := NewServer(0, storage, func(_ context.Context, _ string) (string, error) {
			return content, nil
		}, slog.New(slog.NewTextHandler(io.Discard, nil))).WithRoots(roots)
		srv.Authorize("tok", "h", time.Minute)
		return httptest.NewServer(http.HandlerFunc(srv.handleTransfer))
	}
	status := func(ts *httptest.Server) int {
		resp, err := http.Get(ts.URL + "/transfer?token=tok")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	without := newServer()
	defer without.Close()
	if got := status(without); got != http.StatusForbidden {
		t.Errorf("unmapped directory: status %d, want 403", got)
	}
	with := newServer(mapped)
	defer with.Close()
	if got := status(with); got != http.StatusOK {
		t.Errorf("mapped directory: status %d, want 200", got)
	}
}

// TestPull_GivesTheDestinationOwner (ADR-043): files the Agent (root) extracts get the owner of
// the folder they land in, so an Engine running as another user can keep writing to them.
// Only meaningful as root, which is how the Agent runs and how the Node tests run.
func TestPull_GivesTheDestinationOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to give files another owner")
	}
	storage := t.TempDir()
	content := writeTree(t, filepath.Join(storage, "movie"), map[string]string{"a.mkv": "A", "sub/b.srt": "B"})
	srv := NewServer(0, storage, func(_ context.Context, _ string) (string, error) {
		return content, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.Authorize("tok", "h", time.Minute)
	ts := httptest.NewServer(http.HandlerFunc(srv.handleTransfer))
	defer ts.Close()
	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))
	port, _ := strconv.Atoi(portStr)

	dest := t.TempDir()
	const uid, gid = 1234, 2345
	if err := os.Chown(dest, uid, gid); err != nil {
		t.Fatal(err)
	}
	if err := Pull(context.Background(), host, port, "tok", dest); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for _, rel := range []string{"movie", "movie/a.mkv", "movie/sub", "movie/sub/b.srt"} {
		fi, err := os.Lstat(filepath.Join(dest, rel))
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		st := fi.Sys().(*syscall.Stat_t)
		if st.Uid != uid || st.Gid != gid {
			t.Errorf("%s owned by %d:%d, want %d:%d", rel, st.Uid, st.Gid, uid, gid)
		}
	}
}
