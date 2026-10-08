// Package agentrelease knows which version of the Agent is published and where to download it, per
// architecture (ADR-027).
//
// The source is the `agents.json` manifest published by the Portal build alongside the binaries
// (ADR-017 already served Node artifacts at /node/*). The CP reads this manifest via HTTP and
// compares it to the `agent_version`/`architecture` each Node reports in inventory — this is what
// allows showing "update available" and building the `update_agent` command with the right URL and
// checksum, without anyone typing hashes by hand.
//
// The base URL is tunable (ADR-023) and must be reachable BY THE NODES: the Agent downloads the
// binary from it. Empty = feature disabled — deliberate, so the CP never invents a
// public address and tells the Node to fetch from the wrong place.
package agentrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// ErrNotConfigured indicates that `agent.artifacts_url` is empty (feature disabled).
var ErrNotConfigured = errors.New("agent.artifacts_url not configured")

// Binary is the artifact for an architecture.
type Binary struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

// Manifest is the content of agents.json.
type Manifest struct {
	Version  string            `json:"version"`
	Binaries map[string]Binary `json:"binaries"`
}

// Release is the public view: available version + covered architectures.
type Release struct {
	Version string   `json:"version"`
	Archs   []string `json:"archs"`
}

// Service reads and caches the manifest.
type Service struct {
	cfg  *settings.Store
	http *http.Client

	// Short cache: the Portal queries it on every Nodes screen refresh, and the manifest only
	// changes on deploy. Small TTL so a new build shows up without restarting the CP.
	mu        sync.Mutex
	cached    *Manifest
	cachedAt  time.Time
	cachedURL string // invalidates the cache when the tunable changes (hot-reload from ADR-023)
}

const cacheTTL = 60 * time.Second

// New creates the service over the tunables catalog.
func New(cfg *settings.Store) *Service {
	return &Service{cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}}
}

// baseURL returns the base URL of the artifacts, without a trailing slash.
func (s *Service) baseURL() string {
	return strings.TrimRight(strings.TrimSpace(s.cfg.String("agent.artifacts_url")), "/")
}

// Manifest returns the published manifest (cached for cacheTTL).
func (s *Service) Manifest(ctx context.Context) (*Manifest, error) {
	base := s.baseURL()
	if base == "" {
		return nil, ErrNotConfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && s.cachedURL == base && time.Since(s.cachedAt) < cacheTTL {
		return s.cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/agents.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch agents.json: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agents.json: status %d", resp.StatusCode)
	}
	var m Manifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode agents.json: %w", err)
	}
	if m.Version == "" || len(m.Binaries) == 0 {
		return nil, errors.New("invalid agents.json (no version/binaries)")
	}
	s.cached, s.cachedAt, s.cachedURL = &m, time.Now(), base
	return &m, nil
}

// Available returns the published version and the covered architectures (for the Portal).
func (s *Service) Available(ctx context.Context) (Release, error) {
	m, err := s.Manifest(ctx)
	if err != nil {
		return Release{}, err
	}
	archs := make([]string, 0, len(m.Binaries))
	for a := range m.Binaries {
		archs = append(archs, a)
	}
	return Release{Version: m.Version, Archs: archs}, nil
}

// Target describes what to send in the update_agent command for an architecture.
type Target struct {
	Version string
	URL     string
	SHA256  string
}

// TargetFor resolves the published binary for the given architecture.
func (s *Service) TargetFor(ctx context.Context, arch string) (Target, error) {
	m, err := s.Manifest(ctx)
	if err != nil {
		return Target{}, err
	}
	bin, ok := m.Binaries[arch]
	if !ok {
		return Target{}, fmt.Errorf("no binary published for architecture %q", arch)
	}
	return Target{
		Version: m.Version,
		URL:     s.baseURL() + "/" + bin.File,
		SHA256:  bin.SHA256,
	}, nil
}
