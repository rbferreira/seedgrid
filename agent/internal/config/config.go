// Package config loads the Agent configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
)

// DefaultEngineContainer is the name of the Torrent Engine container the Agent looks for
// by default (SEEDGRID_ENGINE_CONTAINER). It MUST match the `container_name` of the
// Engine service in deploy/agent/docker-compose.yml — the compose file is what the
// onboarding delivers to the Node (/node/docker-compose.yml). The name is purpose-neutral
// (it doesn't mention the engine): the Engine is a product concept, qBittorrent is
// today's implementation. A test (TestEngineContainerMatchesDeployArtifacts) fails if
// the deploy artifacts and this default drift apart — this divergence is what left
// `engine_started_at` NULL on a new Node (#146).
const DefaultEngineContainer = "seedgrid-engine"

// Config gathers the Agent's execution parameters.
type Config struct {
	ControlPlaneURL     string
	RegistrationToken   string
	DataDir             string
	HeartbeatInterval   time.Duration
	CommandPollInterval time.Duration
	Engine              string
	EngineURL           string
	EngineUsername      string
	EnginePassword      string
	StoragePath         string
	// StorageRequireMount: the Node declared dedicated disk at StoragePath, so storage is only
	// healthy if it's a mountpoint (SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT). Set by the
	// bootstrap on install; absent = true, which is the safe side and what existing Nodes have
	// always had. Never inferred at runtime (see resources.StorageHealth).
	StorageRequireMount bool
	BTPort              int
	MigrationPort       int
	// Engine queue (protection against I/O saturation). See qbittorrent.EngineConfig.
	QueueingEnabled            bool
	MaxActiveDownloads         int
	MaxActiveUploads           int
	MaxActiveTorrents          int
	MaxActiveCheckingTorrents  int
	DontCountSlowTorrents      bool
	SlowTorrentDlRateThreshold int
	SlowTorrentUlRateThreshold int
	SlowTorrentInactiveTimer   int
	// EngineContainer is the name of the Torrent Engine container, used only to report
	// the container uptime in the heartbeat (best-effort via docker inspect). "" disables it.
	// Default: DefaultEngineContainer.
	EngineContainer string
	LogLevel        string
	// EngineAdopted: the Node joined with the qBittorrent the machine already ran (ADR-043,
	// SEEDGRID_ENGINE_ADOPTED). It registers so, and its first settings are read from that Engine
	// (remotecfg.Hooks.EngineValues); from then on it is managed like any Node.
	EngineAdopted bool
	// EnginePathMap: the Engine's bind mounts, `engine path=host path;...`
	// (SEEDGRID_ENGINE_PATH_MAP, ADR-043). Empty when the Engine sees the host's paths. Parsed by
	// pathmap.Parse, which Load runs to refuse a malformed value at startup.
	EnginePathMap string
}

// Load reads the configuration from the environment and validates required fields.
func Load() (Config, error) {
	c := Config{
		ControlPlaneURL:     os.Getenv("SEEDGRID_CONTROL_PLANE_URL"),
		RegistrationToken:   os.Getenv("SEEDGRID_AGENT_REGISTRATION_TOKEN"),
		DataDir:             getenv("SEEDGRID_AGENT_DATA_DIR", "/var/lib/seedgrid-agent"),
		Engine:              getenv("SEEDGRID_AGENT_ENGINE", "qbittorrent"),
		EngineURL:           getenv("SEEDGRID_ENGINE_QBT_URL", "http://127.0.0.1:8080"),
		EngineUsername:      getenv("SEEDGRID_ENGINE_QBT_USERNAME", "admin"),
		EnginePassword:      os.Getenv("SEEDGRID_ENGINE_QBT_PASSWORD"),
		StoragePath:         getenv("SEEDGRID_AGENT_STORAGE_PATH", "/data"),
		StorageRequireMount: getenvBool("SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT", true),
		BTPort:              getenvInt("SEEDGRID_ENGINE_BT_PORT", 6881),
		MigrationPort:       getenvInt("SEEDGRID_AGENT_MIGRATION_PORT", 6900),
		EngineContainer:     getenv("SEEDGRID_ENGINE_CONTAINER", DefaultEngineContainer),
		EngineAdopted:       getenvBool("SEEDGRID_ENGINE_ADOPTED", false),
		EnginePathMap:       os.Getenv("SEEDGRID_ENGINE_PATH_MAP"),
		LogLevel:            getenv("SEEDGRID_LOG_LEVEL", "info"),
		HeartbeatInterval:   30 * time.Second,
		CommandPollInterval: 5 * time.Second,
		// Queue enabled by default: without it, adding many torrents saturates I/O
		// (high load, checkingResumeData stuck, USB HDD dropping due to underpowering).
		// HOWEVER the queue must only limit DOWNLOADS/CHECKING (what weighs on I/O). SEEDERS
		// must never be queued: a seeder in queuedUP sits idle and sends
		// event=stopped to the tracker → stops accumulating seedtime → Hit and Run. That's why
		// uploads/total are practically unlimited (only downloads/checking are queued).
		QueueingEnabled: getenvBool("SEEDGRID_ENGINE_QUEUEING_ENABLED", true),
		// Modest active downloads limit per Node (belt and suspenders): the
		// Control Plane admission controller (#21) is what ensures ≤ L downloads
		// across the ENTIRE cluster respecting the tracker's DL slot limit per account;
		// here we just prevent a single Node from maxing out on its own.
		MaxActiveDownloads:        getenvInt("SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS", 2),
		MaxActiveUploads:          getenvInt("SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS", 10000),
		MaxActiveTorrents:         getenvInt("SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS", 10000),
		MaxActiveCheckingTorrents: getenvInt("SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING", 1),
		// dont_count_slow_torrents=FALSE on purpose: with true, a stalled download
		// (0 KB/s, waiting for a peer slot on the tracker) doesn't count towards max_active_downloads,
		// so qBt ENABLES ALL downloads → all announce at once → the tracker
		// refuses slots (thundering herd) → general stall. With false, the cap actually holds
		// and the excess stay in queuedDL (they don't announce as leechers, don't consume a slot).
		DontCountSlowTorrents:      getenvBool("SEEDGRID_ENGINE_DONT_COUNT_SLOW", false),
		SlowTorrentDlRateThreshold: getenvInt("SEEDGRID_ENGINE_SLOW_DL_THRESHOLD", 1),
		SlowTorrentUlRateThreshold: getenvInt("SEEDGRID_ENGINE_SLOW_UL_THRESHOLD", 1),
		SlowTorrentInactiveTimer:   getenvInt("SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER", 300),
	}

	if v := os.Getenv("SEEDGRID_AGENT_HEARTBEAT_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("invalid SEEDGRID_AGENT_HEARTBEAT_INTERVAL: %w", err)
		}
		// A ticker with duration ≤ 0 panics: better to reject at startup, with the reason.
		if d <= 0 {
			return c, fmt.Errorf("invalid SEEDGRID_AGENT_HEARTBEAT_INTERVAL: %s is not positive", d)
		}
		c.HeartbeatInterval = d
	}
	if v := os.Getenv("SEEDGRID_AGENT_COMMAND_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("invalid SEEDGRID_AGENT_COMMAND_POLL_INTERVAL: %w", err)
		}
		if d <= 0 {
			return c, fmt.Errorf("invalid SEEDGRID_AGENT_COMMAND_POLL_INTERVAL: %s is not positive", d)
		}
		c.CommandPollInterval = d
	}

	if c.ControlPlaneURL == "" {
		return c, fmt.Errorf("SEEDGRID_CONTROL_PLANE_URL is required")
	}
	if _, err := pathmap.Parse(c.EnginePathMap); err != nil {
		return c, fmt.Errorf("invalid SEEDGRID_ENGINE_PATH_MAP: %w", err)
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
