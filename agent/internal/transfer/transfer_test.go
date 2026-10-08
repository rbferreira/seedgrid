package transfer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeTree creates a file tree and returns the root path.
func writeTree(t *testing.T, base string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil { //nolint:gosec // G306: test fixture
			t.Fatal(err)
		}
	}
	return base
}

func TestServe_And_Pull_RoundTrip(t *testing.T) {
	storage := t.TempDir()
	// Multi-file torrent: storage/mytorrent/{a.txt, sub/b.txt}
	content := writeTree(t, filepath.Join(storage, "mytorrent"), map[string]string{
		"a.txt":     "content A",
		"sub/b.txt": "content B",
	})

	srv := NewServer(0, storage, func(_ context.Context, hash string) (string, error) {
		return content, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.Authorize("tok-123", "hashX", 10*time.Minute)

	ts := httptest.NewServer(http.HandlerFunc(srv.handleTransfer))
	defer ts.Close()
	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))
	port, _ := strconv.Atoi(portStr)

	// No token → 401.
	resp, err := http.Get(ts.URL + "/transfer")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: status = %d, want 401", resp.StatusCode)
	}

	// With valid token → pulls and extracts to the destination.
	dest := t.TempDir()
	if err := Pull(context.Background(), host, port, "tok-123", dest); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for rel, want := range map[string]string{
		"mytorrent/a.txt":     "content A",
		"mytorrent/sub/b.txt": "content B",
	} {
		got, err := os.ReadFile(filepath.Join(dest, rel)) //nolint:gosec // G304: test fixture (t.TempDir())
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

// TestServe_RejectsEmptyContent pins the root cause of the 2026-07-18 incident (ADR-031).
// A torrent that had not downloaded anything would pass through the handler, the 200 had already
// been written, and the defer tw.Close() emitted the footer: the destination received A VALID AND
// EMPTY TAR and Pull returned nil — "success" without copying a single byte. The destination then
// added the .torrent and re-downloaded everything from the swarm, burning ratio. Now it must be 409,
// and Pull must return ErrNothingToTransfer (permanent, non-retryable error).
func TestServe_RejectsEmptyContent(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(t *testing.T, storage string) string
	}{
		{
			// REAL incident case: qBittorrent never even created the path.
			// Validated live on 2026-07-18 with "The.Great.Gatsby" at 0%.
			name: "nonexistent path (download never wrote anything)",
			prepare: func(_ *testing.T, storage string) string {
				return filepath.Join(storage, "never-existed")
			},
		},
		{
			name: "directory without any files (download did not start)",
			prepare: func(t *testing.T, storage string) string {
				dir := filepath.Join(storage, "empty")
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatal(err)
				}
				return dir
			},
		},
		{
			name: "directory with only zero-sized files",
			prepare: func(t *testing.T, storage string) string {
				return writeTree(t, filepath.Join(storage, "zeroed"), map[string]string{
					"a.bin":     "",
					"sub/b.bin": "",
				})
			},
		},
		{
			name: "single zero-sized file",
			prepare: func(t *testing.T, storage string) string {
				p := filepath.Join(storage, "single.bin")
				if err := os.WriteFile(p, nil, 0o644); err != nil { //nolint:gosec // G306: test fixture
					t.Fatal(err)
				}
				return p
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			storage := t.TempDir()
			content := c.prepare(t, storage)
			srv := NewServer(0, storage, func(_ context.Context, _ string) (string, error) {
				return content, nil
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			srv.Authorize("tok", "hashX", 10*time.Minute)

			ts := httptest.NewServer(http.HandlerFunc(srv.handleTransfer))
			defer ts.Close()
			host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))
			port, _ := strconv.Atoi(portStr)

			resp, err := http.Get(ts.URL + "/transfer?token=tok")
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusConflict {
				t.Errorf("status = %d, want 409 (nothing to migrate)", resp.StatusCode)
			}

			// And Pull must recognize this as permanent, without extracting anything.
			dest := t.TempDir()
			err = Pull(context.Background(), host, port, "tok", dest)
			if !errors.Is(err, ErrNothingToTransfer) {
				t.Errorf("Pull = %v, want ErrNothingToTransfer", err)
			}
			entries, readErr := os.ReadDir(dest)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Errorf("destination has %d entries, want 0", len(entries))
			}
		})
	}
}

// TestServe_PartialContentIsServed: the opposite of the test above — a PARTIAL torrent has
// bytes and must be served normally. This is the use case that ADR-031 unlocks; if this
// test turns into a 409, the "empty" check got too aggressive and broke the drain.
func TestServe_PartialContentIsServed(t *testing.T) {
	storage := t.TempDir()
	// Partial: one file with data, another still zeroed (piece not downloaded).
	content := writeTree(t, filepath.Join(storage, "partial"), map[string]string{
		"downloaded.bin": "real data",
		"missing.bin":    "",
	})
	srv := NewServer(0, storage, func(_ context.Context, _ string) (string, error) {
		return content, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.Authorize("tok", "hashX", 10*time.Minute)

	ts := httptest.NewServer(http.HandlerFunc(srv.handleTransfer))
	defer ts.Close()
	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))
	port, _ := strconv.Atoi(portStr)

	dest := t.TempDir()
	if err := Pull(context.Background(), host, port, "tok", dest); err != nil {
		t.Fatalf("partial torrent Pull should work: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "partial", "downloaded.bin")) //nolint:gosec // G304: test fixture (t.TempDir())
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != "real data" {
		t.Errorf("content = %q, want %q", got, "real data")
	}
}

func TestServe_RejectsPathOutsideStorage(t *testing.T) {
	storage := t.TempDir()
	outside := t.TempDir() // outside allowed storage
	srv := NewServer(0, storage, func(_ context.Context, hash string) (string, error) {
		return outside, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.Authorize("tok", "h", time.Minute)

	ts := httptest.NewServer(http.HandlerFunc(srv.handleTransfer))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/transfer?token=tok")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("path outside storage: status = %d, want 403", resp.StatusCode)
	}
}

func TestServe_ExpiredToken(t *testing.T) {
	storage := t.TempDir()
	srv := NewServer(0, storage, func(_ context.Context, hash string) (string, error) {
		return storage, nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.Authorize("tok", "h", -time.Second) // already expired

	ts := httptest.NewServer(http.HandlerFunc(srv.handleTransfer))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/transfer?token=tok")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token: status = %d, want 401", resp.StatusCode)
	}
}
