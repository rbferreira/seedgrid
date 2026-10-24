package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_AGENT_REGISTRATION_TOKEN", "tok")
	t.Setenv("SEEDGRID_AGENT_DATA_DIR", "")
	t.Setenv("SEEDGRID_AGENT_ENGINE", "")
	t.Setenv("SEEDGRID_AGENT_HEARTBEAT_INTERVAL", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.DataDir != "/var/lib/seedgrid-agent" {
		t.Errorf("DataDir default = %q", c.DataDir)
	}
	if c.Engine != "qbittorrent" {
		t.Errorf("Engine default = %q", c.Engine)
	}
	if c.HeartbeatInterval != 30*time.Second {
		t.Errorf("HeartbeatInterval default = %v", c.HeartbeatInterval)
	}
	// Queue enabled by default, with sensible limits and anti-stall rotation.
	if !c.QueueingEnabled {
		t.Error("QueueingEnabled default should be true")
	}
	// Modest per-Node cap: the CP admission controller (#21) is what guarantees
	// ≤ L downloads in the cluster; here we just prevent a single Node from maxing out.
	if c.MaxActiveDownloads != 2 {
		t.Errorf("MaxActiveDownloads default = %d, want 2", c.MaxActiveDownloads)
	}
	if c.MaxActiveCheckingTorrents != 1 {
		t.Errorf("MaxActiveCheckingTorrents default = %d, want 1", c.MaxActiveCheckingTorrents)
	}
	// false on purpose: with true a paused download doesn't count towards the cap and qBt
	// activates all → thundering herd on the tracker → general stall (#21).
	if c.DontCountSlowTorrents {
		t.Error("DontCountSlowTorrents default should be false")
	}
	if c.SlowTorrentInactiveTimer != 300 {
		t.Errorf("SlowTorrentInactiveTimer default = %d, want 300", c.SlowTorrentInactiveTimer)
	}
}

func TestLoad_QueueingOverrides(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_ENGINE_QUEUEING_ENABLED", "false")
	t.Setenv("SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS", "5")
	t.Setenv("SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER", "120")
	c, err := Load()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if c.QueueingEnabled {
		t.Error("QueueingEnabled should respect override false")
	}
	if c.MaxActiveDownloads != 5 {
		t.Errorf("MaxActiveDownloads = %d, want 5", c.MaxActiveDownloads)
	}
	if c.SlowTorrentInactiveTimer != 120 {
		t.Errorf("SlowTorrentInactiveTimer = %d, want 120", c.SlowTorrentInactiveTimer)
	}
}

func TestLoad_MissingControlPlaneURL(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error without SEEDGRID_CONTROL_PLANE_URL")
	}
}

func TestLoad_HeartbeatIntervalParse(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_AGENT_HEARTBEAT_INTERVAL", "10s")
	c, err := Load()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if c.HeartbeatInterval != 10*time.Second {
		t.Errorf("interval = %v, want 10s", c.HeartbeatInterval)
	}

	t.Setenv("SEEDGRID_AGENT_HEARTBEAT_INTERVAL", "xyz")
	if _, err := Load(); err == nil {
		t.Error("expected error with invalid interval")
	}
}

// Interval ≤ 0 would crash the ticker (time.NewTicker/Reset panics): startup refuses it,
// with the reason, instead of the Agent crashing halfway through.
func TestLoad_NonPositiveIntervalsAreRejected(t *testing.T) {
	for _, env := range []string{"SEEDGRID_AGENT_HEARTBEAT_INTERVAL", "SEEDGRID_AGENT_COMMAND_POLL_INTERVAL"} {
		for _, v := range []string{"0s", "-5s"} {
			t.Run(env+"="+v, func(t *testing.T) {
				t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
				t.Setenv("SEEDGRID_AGENT_HEARTBEAT_INTERVAL", "")
				t.Setenv("SEEDGRID_AGENT_COMMAND_POLL_INTERVAL", "")
				t.Setenv(env, v)
				if _, err := Load(); err == nil {
					t.Errorf("%s=%s should be rejected", env, v)
				}
			})
		}
	}
}

// TestLoad_UnknownEngine (ADR-045): an Engine the Agent has no adapter for stops it at start,
// with the variable named, instead of running against an API it does not speak.
func TestLoad_UnknownEngine(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_AGENT_ENGINE", "transmission")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SEEDGRID_AGENT_ENGINE") {
		t.Fatalf("Load() error = %v, want one naming SEEDGRID_AGENT_ENGINE", err)
	}
}

// TestLoad_RTorrent (ADR-045): rTorrent is chosen by SEEDGRID_AGENT_ENGINE and needs its XML-RPC URL.
func TestLoad_RTorrent(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_AGENT_ENGINE", "rtorrent")
	t.Setenv("SEEDGRID_ENGINE_RTORRENT_URL", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SEEDGRID_ENGINE_RTORRENT_URL") {
		t.Fatalf("Load() without the URL = %v, want an error naming SEEDGRID_ENGINE_RTORRENT_URL", err)
	}
	t.Setenv("SEEDGRID_ENGINE_RTORRENT_URL", "scgi:///run/rtorrent/rpc.socket")
	c, err := Load()
	if err != nil || c.Engine != EngineRTorrent || c.EngineRTorrentURL != "scgi:///run/rtorrent/rpc.socket" {
		t.Fatalf("Load() = %q %q %v", c.Engine, c.EngineRTorrentURL, err)
	}
}

func TestLoad_EngineContainerDefault(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_ENGINE_CONTAINER", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if c.EngineContainer != DefaultEngineContainer {
		t.Errorf("EngineContainer default = %q, want %q", c.EngineContainer, DefaultEngineContainer)
	}

	// The operator override remains valid — it's what existing Nodes use
	// while their container still has the old name (#146 rollout).
	t.Setenv("SEEDGRID_ENGINE_CONTAINER", "my-engine")
	c, err = Load()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if c.EngineContainer != "my-engine" {
		t.Errorf("EngineContainer = %q, want the environment override", c.EngineContainer)
	}
}

// TestEngineContainerMatchesDeployArtifacts pins down the cause of #146: the Agent was looking for a
// `qbittorrent` container while compose was creating `seedgrid-qbittorrent`, and the new Node
// was left with engine_started_at NULL forever, without any error. The name lives in four
// artifacts (Agent default, compose served at /node/, bootstrap, storage reattachment
// script); this test keeps them identical.
func TestEngineContainerMatchesDeployArtifacts(t *testing.T) {
	root := filepath.Join("..", "..", "..") // agent/internal/config → repository root
	compose := filepath.Join(root, "deploy", "agent", "docker-compose.yml")
	if _, err := os.Stat(compose); err != nil {
		t.Skipf("deploy artifacts missing (module outside the repository?): %v", err)
	}

	const old = "seedgrid-qbittorrent"
	for _, tc := range []struct {
		file string // relative to the root
		want string // snippet that must exist
	}{
		{"deploy/agent/docker-compose.yml", "container_name: " + DefaultEngineContainer},
		{"scripts/bootstrap-node.sh", "ENGINE_CONTAINER=" + DefaultEngineContainer},
		{"deploy/agent/seedgrid-storage-reattach.sh", DefaultEngineContainer},
	} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tc.file))) //nolint:gosec // G304: fixed list of artifacts from the repository itself
		if err != nil {
			t.Fatalf("read %s: %v", tc.file, err)
		}
		body := string(raw)
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s does not contain %q — the Engine container name diverged from the Agent default", tc.file, tc.want)
		}
		if strings.Contains(body, old) {
			t.Errorf("%s still references %q (old container name)", tc.file, old)
		}
	}
}

// TestLoad_AdoptedEngine (ADR-043): an Agent running against the operator's own qBittorrent is
// off by default, and a malformed path map is refused at startup, not on the first migration.
func TestLoad_AdoptedEngine(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_ENGINE_ADOPTED", "")
	t.Setenv("SEEDGRID_ENGINE_PATH_MAP", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.EngineAdopted || c.EnginePathMap != "" {
		t.Errorf("defaults: adopted=%v map=%q, want false and empty", c.EngineAdopted, c.EnginePathMap)
	}

	t.Setenv("SEEDGRID_ENGINE_ADOPTED", "true")
	t.Setenv("SEEDGRID_ENGINE_PATH_MAP", "/downloads=/mnt/user/torrents")
	if c, err = Load(); err != nil || !c.EngineAdopted || c.EnginePathMap != "/downloads=/mnt/user/torrents" {
		t.Errorf("adopted: (%+v, %v)", c, err)
	}

	t.Setenv("SEEDGRID_ENGINE_PATH_MAP", "downloads=/mnt")
	if _, err := Load(); err == nil {
		t.Error("a relative path in SEEDGRID_ENGINE_PATH_MAP was accepted")
	}
}
