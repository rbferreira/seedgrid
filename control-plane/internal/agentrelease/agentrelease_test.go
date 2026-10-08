package agentrelease

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

const manifestJSON = `{
  "version": "v1.2.0",
  "binaries": {
    "amd64": { "file": "seedgrid-agent-amd64", "sha256": "aaa111" },
    "arm64": { "file": "seedgrid-agent-arm64", "sha256": "bbb222" }
  }
}`

// serveManifest starts a server that returns the manifest and counts accesses (to prove
// the cache).
func serveManifest(t *testing.T, body string) (string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/node/agents.json" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/node", &hits
}

func svc(url string) *Service {
	return New(settings.NewForTest(map[string]string{"agent.artifacts_url": url}))
}

// Empty = feature disabled (deliberate in ADR-027: the CP does not invent its own public
// URL). It must report this with a specific error, not a network failure.
func TestNotConfigured(t *testing.T) {
	s := svc("")
	if _, err := s.Available(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("empty URL should return ErrNotConfigured; got %v", err)
	}
	if _, err := s.TargetFor(context.Background(), "amd64"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("TargetFor with empty URL should return ErrNotConfigured; got %v", err)
	}
}

func TestAvailable(t *testing.T) {
	url, _ := serveManifest(t, manifestJSON)
	rel, err := svc(url).Available(context.Background())
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	if rel.Version != "v1.2.0" {
		t.Errorf("version = %q, want v1.2.0", rel.Version)
	}
	if len(rel.Archs) != 2 {
		t.Errorf("archs = %v, want 2 (amd64, arm64)", rel.Archs)
	}
}

// ADR-027 critical point: each Node receives the binary for ITS architecture. Sending the
// wrong one crashes the Node — which almost happened with the single /node/seedgrid-agent ARM64.
func TestTargetPerArchitecture(t *testing.T) {
	url, _ := serveManifest(t, manifestJSON)
	s := svc(url)

	amd, err := s.TargetFor(context.Background(), "amd64")
	if err != nil {
		t.Fatalf("TargetFor(amd64): %v", err)
	}
	if amd.URL != url+"/seedgrid-agent-amd64" || amd.SHA256 != "aaa111" {
		t.Errorf("amd64 resolved incorrectly: %+v", amd)
	}
	arm, err := s.TargetFor(context.Background(), "arm64")
	if err != nil {
		t.Fatalf("TargetFor(arm64): %v", err)
	}
	if arm.URL != url+"/seedgrid-agent-arm64" || arm.SHA256 != "bbb222" {
		t.Errorf("arm64 resolved incorrectly: %+v", arm)
	}
	if amd.SHA256 == arm.SHA256 {
		t.Error("different architectures must not share checksum")
	}
}

// Architecture without published binary: explicit error instead of sending just anything.
func TestUnknownArchitecture(t *testing.T) {
	url, _ := serveManifest(t, manifestJSON)
	if _, err := svc(url).TargetFor(context.Background(), "riscv64"); err == nil {
		t.Error("architecture without published binary should fail, not choose another")
	}
}

func TestInvalidManifest(t *testing.T) {
	url, _ := serveManifest(t, `{"version":"","binaries":{}}`)
	if _, err := svc(url).Available(context.Background()); err == nil {
		t.Error("manifest without version/binaries should fail")
	}
}

// The Portal queries on every refresh of the Nodes screen; without cache it would be an HTTP
// request per refresh, per operator.
func TestCache(t *testing.T) {
	url, hits := serveManifest(t, manifestJSON)
	s := svc(url)
	for i := 0; i < 3; i++ {
		if _, err := s.Available(context.Background()); err != nil {
			t.Fatalf("Available: %v", err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("fetched manifest %d times; cache should keep it at 1", got)
	}
}

// Tunable has hot-reload (ADR-023): changing the URL in the panel must invalidate the cache,
// otherwise the CP would keep serving the manifest from the old address.
func TestCacheInvalidatesWhenChangingURL(t *testing.T) {
	url1, hits1 := serveManifest(t, manifestJSON)
	url2, hits2 := serveManifest(t, `{"version":"v9.9.9","binaries":{"amd64":{"file":"f","sha256":"c"}}}`)

	store := settings.NewForTest(map[string]string{"agent.artifacts_url": url1})
	s := New(store)
	if _, err := s.Available(context.Background()); err != nil {
		t.Fatalf("Available: %v", err)
	}

	// Simulates changing the tunable in the panel.
	s.cfg = settings.NewForTest(map[string]string{"agent.artifacts_url": url2})
	rel, err := s.Available(context.Background())
	if err != nil {
		t.Fatalf("Available after changing the URL: %v", err)
	}
	if rel.Version != "v9.9.9" {
		t.Errorf("version = %q; should come from the NEW manifest (v9.9.9), not from cache", rel.Version)
	}
	if hits1.Load() != 1 || hits2.Load() != 1 {
		t.Errorf("unexpected hits: url1=%d url2=%d", hits1.Load(), hits2.Load())
	}
}
