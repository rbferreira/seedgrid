// Package engine is what the Agent needs from a Torrent Engine (ADR-045). The Agent's code depends
// on Engine, not on an adapter: each adapter (engine/qbittorrent today) translates its Engine's API
// into these operations and types, with the SeedGrid state already mapped (Database.md §36).
package engine

import "context"

// Engine is the Torrent Engine as the Agent uses it. Paths it receives and returns are the host's;
// an adapter whose Engine sees other paths (a container, ADR-043) translates them.
type Engine interface {
	// Torrents lists the torrents the Engine holds.
	Torrents(ctx context.Context) ([]Torrent, error)
	// State returns a torrent's progress (0..1) and SeedGrid state; ("", 0) when the Engine
	// does not hold it.
	State(ctx context.Context, hash string) (float64, string, error)
	// ContentPathOf returns where a torrent's data is ("" when the Engine does not hold it).
	ContentPathOf(ctx context.Context, hash string) (string, error)

	AddURL(ctx context.Context, magnetOrURL string) error
	AddFile(ctx context.Context, data []byte) error
	// AddFileTo adds a torrent whose data is already at savePath, laid out as the .torrent says
	// (migration, ADR-018).
	AddFileTo(ctx context.Context, data []byte, savePath string) error
	Stop(ctx context.Context, hash string) error
	Start(ctx context.Context, hash string) error
	Recheck(ctx context.Context, hash string) error
	Delete(ctx context.Context, hash string, deleteFiles bool) error

	// Export returns the .torrent of a torrent the Engine holds (backfill, ADR-018).
	Export(ctx context.Context, hash string) ([]byte, error)
	// CreateTorrent creates a .torrent from data on disk, starts seeding it and returns its
	// bytes (publishing, ADR-019).
	CreateTorrent(ctx context.Context, sourcePath string, trackers []string, private bool, source string) ([]byte, error)

	// Configure applies the machine's settings (BT port, data folder) and the queue: the first
	// application of the process.
	Configure(ctx context.Context, c Config) error
	// ConfigureQueue applies only the queue: a change from the Control Plane (ADR-039).
	ConfigureQueue(ctx context.Context, c Config) error
	// ReadQueue reads the queue the Engine has now (a Node that joined with its own Engine,
	// ADR-043).
	ReadQueue(ctx context.Context) (Config, error)

	// Version is the Engine's version; an error means it is unreachable.
	Version(ctx context.Context) (string, error)
	// ConnectionStatus is the Engine's verdict on its inbound connectability ("connected",
	// "firewalled", "disconnected"), or "" when it has none (ADR-034).
	ConnectionStatus(ctx context.Context) (string, error)
	// IncomingPeers counts the inbound connections open now, over all torrents (#155).
	IncomingPeers(ctx context.Context) (int, error)
	// TrackerVerdicts returns, by lowercase hash, the worst answer of each torrent's trackers (#160).
	TrackerVerdicts(ctx context.Context) (map[string]TrackerVerdict, error)
	// SlotLimit looks for the tracker's per-account download limit in the messages of these
	// torrents (#21); false when none says it.
	SlotLimit(ctx context.Context, hashes []string) (int, bool)

	// Shutdown stops the Engine; whatever supervises it brings it back (restart_engine).
	Shutdown(ctx context.Context) error
}

// Torrent is a summary view of a torrent in the Engine.
//
// Peers come in TWO flavors and the distinction matters (refinement of ADR-021): NumSeeds/NumLeechs
// are the SWARM totals (tracker scrape) — the number the operator understands as "torrent
// seeds" and that the Portal displays; NumSeedsConnected/NumLeechsConnected are the peers
// actually CONNECTED. A tracker may announce a ghost seeder (in the scrape, but that
// never responds): then the swarm says 1 and the connected says 0 — only the latter reveals
// that the torrent has no one to download from. See rotateDead in the admission controller.
type Torrent struct {
	Hash               string
	Name               string
	Status             string // SeedGrid state (Database.md §36)
	Progress           float64
	NumSeeds           int // swarm (tracker scrape)
	NumLeechs          int // swarm (tracker scrape)
	NumSeedsConnected  int // connected to the Engine now
	NumLeechsConnected int // connected to the Engine now
	DlSpeed            int64
	UpSpeed            int64
	Size               int64
	Ratio              float64
	ETA                int64
	Downloaded         int64
	Uploaded           int64
	ContentPath        string // path to data on disk, as the host sees it (file for single, folder for multi)
}

// Config gathers the Engine settings the Agent applies (Configure, ConfigureQueue) and reads
// (ReadQueue). The queue fields are the engine.* settings of ADR-039.
type Config struct {
	// BTPort is the BitTorrent listen port (matches the operator's port forward).
	BTPort int
	// SavePath is the directory where data is written (the Node's storage, e.g.: /data), as the
	// host sees it.
	SavePath string
	// QueueingEnabled turns the queue on. When on (default), it limits how many torrents are
	// active at the same time — essential to avoid saturating I/O when many torrents are added
	// at once.
	QueueingEnabled bool
	// MaxActive* only take effect with QueueingEnabled=true.
	MaxActiveDownloads        int
	MaxActiveUploads          int
	MaxActiveTorrents         int
	MaxActiveCheckingTorrents int
	// DontCountSlowTorrents: anti-stall rotation. When true, a "slow" torrent
	// (below SlowTorrentDlRateThreshold KB/s for more than SlowTorrentInactiveTimer
	// seconds) stops counting toward MaxActive*, freeing the slot for the next in queue.
	// Fixes the case of a peerless download occupying an active slot indefinitely.
	DontCountSlowTorrents      bool
	SlowTorrentDlRateThreshold int // KB/s below which the download is "slow"
	SlowTorrentUlRateThreshold int // KB/s below which the upload is "slow"
	SlowTorrentInactiveTimer   int // seconds "slow" before freeing the slot
}

// TrackerVerdict is what the tracker responded on a torrent's announce.
type TrackerVerdict struct {
	Status  int    // libtorrent status: 2=working, 4=not working, 5=refused…
	Message string // raw tracker message (e.g.: "Torrent has been deleted.")
}
