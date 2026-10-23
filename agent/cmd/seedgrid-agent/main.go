// The seedgrid-agent command is the SeedGrid Agent, running on each Seed Node.
//
// On boot it reuses the local identity or registers with the Control Plane; then
// it maintains a heartbeat loop and a command polling loop. All communication
// is initiated by the Agent (ADR-012).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
	"github.com/rbferreira/seedgrid/agent/internal/commands"
	"github.com/rbferreira/seedgrid/agent/internal/config"
	"github.com/rbferreira/seedgrid/agent/internal/engine/qbittorrent"
	"github.com/rbferreira/seedgrid/agent/internal/identity"
	"github.com/rbferreira/seedgrid/agent/internal/inventory"
	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
	"github.com/rbferreira/seedgrid/agent/internal/remotecfg"
	"github.com/rbferreira/seedgrid/agent/internal/resources"
	"github.com/rbferreira/seedgrid/agent/internal/transfer"
)

// incomingPeersSampleInterval is the cadence of the incoming connection scan (#155).
// It does not follow the heartbeat on purpose: see the comment at the usage site.
const incomingPeersSampleInterval = 5 * time.Minute

// trackerVerdictSampleInterval is the cadence of the tracker verdict scan (#160).
// Much slower than the torrent sync (15s) because the data rarely changes — a torrent
// is deleted from the tracker once, not every cycle — and the scan costs one call per
// torrent.
const trackerVerdictSampleInterval = 15 * time.Minute

// version is set at build time via -ldflags.
var version = "dev"

// identityHolder holds the current Node identity in a concurrency-safe way
// (shared between the heartbeat and command loops).
type identityHolder struct {
	mu sync.RWMutex
	id identity.Identity
}

func (h *identityHolder) set(id identity.Identity) {
	h.mu.Lock()
	h.id = id
	h.mu.Unlock()
}

func (h *identityHolder) token() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.id.AgentToken
}

func main() {
	// `--version` prints the version and exits 0, WITHOUT touching config/network. It is the smoke test for
	// ADR-027: before swapping the binary, the Agent runs the candidate with this flag and requires
	// exit 0 — proving that executable runs on that host (catches wrong architecture,
	// corrupted download, incompatible libc) before replacing the binary in production.
	// That is why it cannot depend on env: a valid candidate must exit 0 in any
	// context. It accepts -v/-version/--version to not depend on the caller's style.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-version", "-v":
			fmt.Println(version)
			return
		}
	}
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// The log level changes hot via Control Plane configuration (ADR-039): the handler
	// queries this LevelVar on every log entry.
	logLevel := new(slog.LevelVar)
	logLevel.Set(config.ParseLevel(cfg.LogLevel))
	logger := newLogger(logLevel)
	logger.Info("starting seedgrid-agent", "version", version, "control_plane", cfg.ControlPlaneURL)

	cli := client.New(cfg.ControlPlaneURL)
	inv := inventory.Collect()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	initial, err := ensureRegistered(ctx, logger, cfg, cli, inv)
	if err != nil {
		return err
	}
	holder := &identityHolder{}
	holder.set(initial)
	logger.Info("node registered", "node_id", initial.NodeID, "hostname", inv.Hostname)

	// Command loop (polling) → Torrent Engine. The path map (ADR-043) is empty on the Nodes
	// SeedGrid installs; config.Load already refused a malformed one.
	paths, err := pathmap.Parse(cfg.EnginePathMap)
	if err != nil {
		return err
	}
	engine := qbittorrent.New(cfg.EngineURL, cfg.EngineUsername, cfg.EnginePassword).WithPaths(paths)
	if cfg.EngineAdopted {
		logger.Info("Node joined with the qBittorrent it already ran: its first settings are read from it (ADR-043)",
			"storage", cfg.StoragePath, "path_map", cfg.EnginePathMap)
	}

	// The BT port, save_path, and Engine queue are NO longer set here on boot: the queue
	// comes from the Control Plane (ADR-039) and the right value is only known after the first heartbeat.
	// Applying env before that would make the Engine flash to the old value and, with it, unleash a
	// burst of downloads that the cluster ceiling would undo a second later. The first
	// application (remotecfg.Manager → applyEngine, below) sets everything, including the port and
	// save_path; if the CP does not respond, the Manager applies env after a few retries.

	// Transfer server (ADR-018): serves the content of a torrent under a one-time
	// token (source) and pulls from the source (destination). It also receives publication
	// uploads and creates the .torrent in the Engine (ADR-019, via WithCreator).
	xfer := transfer.NewServer(cfg.MigrationPort, cfg.StoragePath, engine.ContentPathOf, logger).
		WithCreator(engine.CreateTorrent).
		WithRoots(paths.HostRoots())
	go xfer.Run(ctx)

	executor := commands.NewExecutor(engine).EnableMigration(xfer, cfg.StoragePath)
	// ADR-027: after an update_agent swaps the binary (and the CP has already received the ack), exiting
	// with 0 is enough — systemd (Restart=always) brings the Agent back up on the new binary.
	poller := commands.NewPoller(cli, holder.token, executor, cfg.CommandPollInterval, logger).
		OnSelfUpdate(func() { os.Exit(0) })
	go poller.Run(ctx)

	// Behavior configuration coming from the Control Plane (ADR-039). Baseline = env + code
	// defaults; the CP sends the effective values for this Node in the heartbeat response and the Manager
	// hot-applies what changed. hbInterval is read by the heartbeat loop (below).
	var hbInterval atomic.Int64
	hbInterval.Store(int64(cfg.HeartbeatInterval))
	applyEngine := func(actx context.Context, q config.QueueSettings, first bool) error {
		lctx, lcancel := context.WithTimeout(actx, 15*time.Second)
		defer lcancel()
		ec := qbittorrent.EngineConfig{
			BTPort:                     cfg.BTPort,
			SavePath:                   cfg.StoragePath,
			QueueingEnabled:            q.Enabled,
			MaxActiveDownloads:         q.MaxActiveDownloads,
			MaxActiveUploads:           q.MaxActiveUploads,
			MaxActiveTorrents:          q.MaxActiveTorrents,
			MaxActiveCheckingTorrents:  q.MaxActiveChecking,
			DontCountSlowTorrents:      q.DontCountSlow,
			SlowTorrentDlRateThreshold: q.SlowDlThreshold,
			SlowTorrentUlRateThreshold: q.SlowUlThreshold,
			SlowTorrentInactiveTimer:   q.SlowInactiveTimer,
		}
		if first {
			// First application of the process: also sets what belongs to the MACHINE — the listening
			// BitTorrent port (matches the operator's port forward) and the save_path.
			return engine.ConfigureEngine(lctx, ec)
		}
		return engine.ConfigureQueue(lctx, ec)
	}
	hooks := remotecfg.Hooks{
		SetLogLevel:          func(level string) { logLevel.Set(config.ParseLevel(level)) },
		SetHeartbeatInterval: func(d time.Duration) { hbInterval.Store(int64(d)) },
		SetPollInterval:      poller.SetInterval,
		ApplyEngine:          applyEngine,
	}
	if cfg.EngineAdopted {
		// A qBittorrent the machine already ran (ADR-043): its queue is what the Node starts from,
		// and the first application must change nothing in it.
		hooks.EngineValues = func(actx context.Context) (map[string]string, error) {
			rctx, rcancel := context.WithTimeout(actx, 15*time.Second)
			defer rcancel()
			ec, err := engine.ReadQueue(rctx)
			if err != nil {
				return nil, err
			}
			return config.EngineValues(config.QueueSettings{
				Enabled:            ec.QueueingEnabled,
				MaxActiveDownloads: ec.MaxActiveDownloads,
				MaxActiveUploads:   ec.MaxActiveUploads,
				MaxActiveTorrents:  ec.MaxActiveTorrents,
				MaxActiveChecking:  ec.MaxActiveCheckingTorrents,
				DontCountSlow:      ec.DontCountSlowTorrents,
				SlowDlThreshold:    ec.SlowTorrentDlRateThreshold,
				SlowUlThreshold:    ec.SlowTorrentUlRateThreshold,
				SlowInactiveTimer:  ec.SlowTorrentInactiveTimer,
			}), nil
		}
	}
	cfgMgr := remotecfg.New(logger, cfg.Behavior(), config.ExplicitEnv(), hooks)

	// Torrent state synchronization loop (Engine → Control Plane).
	go syncTorrents(ctx, logger, cli, engine, holder.token, 15*time.Second)

	// .torrent backfill loop: exports from the Engine and sends to the CP the .torrents
	// it does not have yet (torrents coming from magnets), making them migratable
	// and the Cluster resilient to re-provisioning (ADR-018).
	go backfillTorrentFiles(ctx, logger, cli, engine, holder.token, time.Minute)

	// INCOMING connection sample (#155), at its OWN cadence and deliberately
	// much slower than the heartbeat. What becomes an alert is the AGE of the last positive,
	// measured in hours — scanning along with the heartbeat (10s on Nodes today, versus the 30s
	// default of this binary) would be 6 scans per minute per Node to feed
	// a 12h rule. The value stays here and the heartbeat only reads the latest sample.
	//
	// -1 = not sampled yet: the heartbeat sends nil and the CP reads it as UNKNOWN, which
	// never alerts. Distinguishing this from 0 is the point — 0 is a legitimate reading.
	var incomingPeers atomic.Int64
	incomingPeers.Store(-1)
	go func() {
		sample := func() {
			sCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			n, err := engine.IncomingPeers(sCtx)
			if err != nil {
				// Best-effort: Engine down should not zero the signal, otherwise the Engine
				// going down would become a false alert of unreachable Node.
				logger.Debug("sample inbound connections", "error", err)
				return
			}
			incomingPeers.Store(int64(n))
		}
		sample() // first immediate sample
		t := time.NewTicker(incomingPeersSampleInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sample()
			}
		}
	}()

	// Loop de heartbeat.
	start := time.Now()
	ioSampler := resources.NewIOSampler(cfg.StoragePath) // disk util% between heartbeats
	cpuSampler := resources.NewCPUSampler()              // CPU util% between heartbeats (#151)
	sendHeartbeat := func() {
		hbCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		res := resources.Collect(cfg.StoragePath)
		diskUtil := ioSampler.Util()
		cpuUtil := cpuSampler.Util()
		storageOK, storageDetail := resources.StorageHealth(cfg.StoragePath, cfg.StorageRequireMount)
		hn, _ := os.Hostname() // re-read on every heartbeat: renaming the Node reflects in the CP without re-registering
		// Engine probe: version doubles as a reachability signal (troubleshooting
		// without SSH). Container start is best-effort (docker inspect) — "" if unavailable.
		engineVer, verr := engine.Version(hbCtx)
		// Connectability (ADR-034): best-effort on both. IPv6 is read from the interfaces;
		// the incoming verdict comes from the Engine ("" if it didn't respond — the CP treats
		// unknown as "don't know", never as firewalled).
		connectable, _ := engine.ConnectionStatus(hbCtx)
		// Latest incoming connection sample; nil while never sampled (#155).
		var incoming *int
		if v := incomingPeers.Load(); v >= 0 {
			c := int(v)
			incoming = &c
		}
		hbReq := client.HeartbeatRequest{
			Status:            "ONLINE",
			Hostname:          hn,
			AgentVersion:      version,
			EngineVersion:     engineVer,
			UptimeSeconds:     int64(time.Since(start).Seconds()),
			DiskTotalBytes:    res.DiskTotalBytes,
			DiskFreeBytes:     res.DiskFreeBytes,
			MemTotalBytes:     res.MemTotalBytes,
			MemAvailBytes:     res.MemAvailBytes,
			BTPort:            cfg.BTPort,
			Load1:             res.Load1,
			CPUCores:          res.CPUCores,
			DiskIOUtilPct:     diskUtil,
			CPUUtilPct:        cpuUtil,
			HostUptimeSeconds: res.HostUptimeSecs,
			EngineReachable:   verr == nil,
			EngineStartedAt:   containerStartedAt(hbCtx, cfg.EngineContainer),
			StorageOK:         storageOK,
			StorageDetail:     storageDetail,
			PublicIPv6:        resources.GlobalIPv6(),
			EngineConnectable: connectable,
			IncomingPeers:     incoming,
		}
		// Configuration negotiation fields (ADR-039): they only go in if the CP has already proven it
		// understands them — otherwise the body goes out exactly as it always has.
		cfgMgr.Prepare(&hbReq)
		hbResp, err := cli.Heartbeat(hbCtx, holder.token(), hbReq)
		cfgMgr.Handle(ctx, hbResp, err)
		switch err {
		case nil:
			logger.Debug("heartbeat sent")
		case client.ErrUnauthorized:
			logger.Warn("credential rejected; re-registering")
			newID, rerr := register(ctx, cfg, cli, inv)
			if rerr != nil {
				logger.Error("failed to re-register", "error", rerr)
				return
			}
			if serr := identity.Save(cfg.DataDir, newID); serr != nil {
				logger.Error("save identity", "error", serr)
			}
			holder.set(newID)
			logger.Info("re-registered", "node_id", newID.NodeID)
		default:
			logger.Error("heartbeat failed", "error", err)
		}
	}

	// heartbeatCycle sends a heartbeat and, if the CP requested the env report (Node not yet
	// seeded, ADR-039 §7), sends the next one NOW — without waiting for the ticker: the Agent holds the
	// application until it reports, and a new Node would wait a full interval with the Engine without the
	// BT port set. Bounded: a CP that asks for the report forever won't stall the loop.
	heartbeatCycle := func() {
		sendHeartbeat()
		for i := 0; i < 2 && cfgMgr.NeedsFollowUp(); i++ {
			sendHeartbeat()
		}
	}

	heartbeatCycle()
	curInterval := time.Duration(hbInterval.Load())
	ticker := time.NewTicker(curInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("stopping agent")
			return nil
		case <-ticker.C:
			heartbeatCycle()
			// The interval may have changed due to Control Plane configuration (ADR-039); the
			// Reset runs in the same goroutine as the select, so it doesn't race with the tick.
			if d := time.Duration(hbInterval.Load()); d != curInterval {
				curInterval = d
				ticker.Reset(d)
			}
		}
	}
}

// containerStartedAt returns the RFC3339 of the Engine container start (best-effort
// via `docker inspect`). Returns "" if docker is not available, the name is
// empty or the container does not exist — the container uptime is a troubleshooting extra,
// it must never take down the heartbeat.
func containerStartedAt(ctx context.Context, name string) string {
	if name == "" {
		return ""
	}
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.StartedAt}}", name).Output() //nolint:gosec // G204: name comes from cfg.EngineContainer, operator config
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// slotProbeEvery limits the probing of the DL slots limit in the tracker msg (#21): the
// L rarely changes and the msg persists while there is a refusal, so it's not worth probing every
// sync (avoids extra requests to the Engine every few seconds).
const slotProbeEvery = 10 * time.Minute

// syncTorrents periodically reports the Engine's torrent state to the Control Plane.
func syncTorrents(ctx context.Context, logger *slog.Logger, cli *client.Client, engine *qbittorrent.Adapter, token func() string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lastSlotProbe time.Time

	// Tracker verdict sample (#160) on its OWN cadence, in a map that the sync
	// loop only READS. Stays here, not next to the heartbeat, because it's torrent data — and
	// separated from the sync ticker because the sweep costs one call per torrent and
	// the data rarely changes: a torrent is deleted on the tracker once, not every 15s.
	var verdicts sync.Map // map[string]qbittorrent.TrackerVerdict
	go func() {
		sample := func() {
			sCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			m, err := engine.TrackerVerdicts(sCtx)
			if err != nil && len(m) == 0 {
				// An offline Engine cannot clear the verdict: without this, a qBittorrent
				// crash would turn into "all torrents dead on the tracker".
				logger.Debug("sample tracker verdict", "error", err)
				return
			}
			for h, v := range m {
				verdicts.Store(h, v)
			}
		}
		sample() // first immediate sample
		t := time.NewTicker(trackerVerdictSampleInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sample()
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			opCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			torrents, err := engine.Info(opCtx)
			cancel()
			if err != nil {
				logger.Debug("list engine torrents", "error", err)
				continue
			}
			reports := make([]client.TorrentReport, 0, len(torrents))
			incomplete := make([]string, 0)
			for _, t := range torrents {
				status := qbittorrent.MapState(t.State)
				reports = append(reports, client.TorrentReport{
					Hash:               t.Hash,
					Name:               t.Name,
					Status:             status,
					Progress:           t.Progress,
					NumSeeds:           t.NumSeeds,
					NumLeechs:          t.NumLeechs,
					NumSeedsConnected:  t.NumSeedsConnected,
					NumLeechsConnected: t.NumLeechsConnected,
					DlSpeed:            t.DlSpeed,
					UpSpeed:            t.UpSpeed,
					Size:               t.Size,
					Ratio:              t.Ratio,
					ETA:                t.ETA,
					Downloaded:         t.Downloaded,
					Uploaded:           t.Uploaded,
				})
				// Attaches the last known tracker verdict; absent until the first
				// sweep has finished (the CP treats it as unknown).
				if v, ok := verdicts.Load(strings.ToLower(t.Hash)); ok {
					tv := v.(qbittorrent.TrackerVerdict)
					reports[len(reports)-1].TrackerStatus = tv.Status
					reports[len(reports)-1].TrackerMessage = tv.Message
				}
				// Candidates to hit the DL slots limit: incompletes that announce or
				// wait for a slot (DOWNLOADING/QUEUED). One msg is enough to find the L.
				if t.Progress < 1 && (status == "DOWNLOADING" || status == "QUEUED") {
					incomplete = append(incomplete, t.Hash)
				}
			}
			// Probes the L (tracker msg), at most every slotProbeEvery and only if there
			// are incompletes. Best-effort: nil when nothing observed → the CP uses the fallback.
			var dlSlotLimit *int
			if len(incomplete) > 0 && time.Since(lastSlotProbe) >= slotProbeEvery {
				lastSlotProbe = time.Now()
				if len(incomplete) > 10 {
					incomplete = incomplete[:10]
				}
				probeCtx, cancelP := context.WithTimeout(ctx, 15*time.Second)
				if n, ok := engine.SlotLimit(probeCtx, incomplete); ok {
					dlSlotLimit = &n
					logger.Debug("DL slot limit observed in tracker msg", "limit", n)
				}
				cancelP()
			}
			repCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := cli.ReportTorrents(repCtx, token(), reports, dlSlotLimit); err != nil {
				logger.Debug("report torrents", "error", err)
			}
			cancel()
		}
	}
}

// backfillTorrentFiles asks the CP which torrents on this Node do not yet have the
// .torrent persisted, exports each one from the Engine and sends it back (ADR-018).
func backfillTorrentFiles(ctx context.Context, logger *slog.Logger, cli *client.Client, engine *qbittorrent.Adapter, token func() string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			hashes, err := cli.PendingTorrentFiles(listCtx, token())
			cancel()
			if err != nil {
				logger.Debug("get pending .torrents", "error", err)
				continue
			}
			var sent int
			for _, h := range hashes {
				opCtx, opCancel := context.WithTimeout(ctx, 20*time.Second)
				data, err := engine.Export(opCtx, h)
				if err != nil {
					logger.Debug("export .torrent from engine", "hash", h, "error", err)
					opCancel()
					continue
				}
				if err := cli.UploadTorrentFile(opCtx, token(), h, data); err != nil {
					logger.Debug("upload .torrent to CP", "hash", h, "error", err)
					opCancel()
					continue
				}
				opCancel()
				sent++
			}
			if sent > 0 {
				logger.Info("backfill of .torrent completed", "sent", sent, "pending", len(hashes))
			}
		}
	}
}

func ensureRegistered(ctx context.Context, logger *slog.Logger, cfg config.Config, cli *client.Client, inv inventory.Info) (identity.Identity, error) {
	id, exists, err := identity.Load(cfg.DataDir)
	if err != nil {
		return identity.Identity{}, err
	}
	if exists {
		logger.Info("local identity found", "node_id", id.NodeID)
		return id, nil
	}
	newID, err := register(ctx, cfg, cli, inv)
	if err != nil {
		return identity.Identity{}, err
	}
	if err := identity.Save(cfg.DataDir, newID); err != nil {
		return identity.Identity{}, err
	}
	return newID, nil
}

func register(ctx context.Context, cfg config.Config, cli *client.Client, inv inventory.Info) (identity.Identity, error) {
	regCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := cli.Register(regCtx, cfg.RegistrationToken, client.RegisterRequest{
		Hostname:      inv.Hostname,
		OSName:        inv.OSName,
		OSVersion:     inv.OSVersion,
		KernelVersion: inv.KernelVersion,
		Architecture:  inv.Architecture,
		AgentVersion:  version,
		Engine:        cfg.Engine,
		EngineAdopted: cfg.EngineAdopted,
	})
	if err != nil {
		return identity.Identity{}, err
	}
	return identity.Identity{NodeID: resp.NodeID, AgentToken: resp.AgentToken}, nil
}

func newLogger(level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
