// Package commands executes the commands received from the Control Plane (via polling)
// translating them into Torrent Engine operations, and confirms the result.
package commands

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
	"github.com/rbferreira/seedgrid/agent/internal/mediafile"
	"github.com/rbferreira/seedgrid/agent/internal/screenshot"
	"github.com/rbferreira/seedgrid/agent/internal/transfer"
)

// Engine is the minimal Torrent Engine interface used by the commands.
type Engine interface {
	AddURL(ctx context.Context, magnetOrURL string) error
	AddFile(ctx context.Context, data []byte) error
	// AddFileTo adds a torrent fixing savepath and content_layout=Original
	// explicitly — used in migration (ADR-018), whose data has already been
	// extracted by Pull() respecting this same structure. Without fixing these
	// parameters, the Engine might apply a different layout on re-add and fail
	// to find the already copied data on recheck, triggering a full redundant
	// download in a different folder (see adapter.go).
	AddFileTo(ctx context.Context, data []byte, savePath string) error
	Stop(ctx context.Context, hash string) error
	Start(ctx context.Context, hash string) error
	Recheck(ctx context.Context, hash string) error
	Delete(ctx context.Context, hash string, deleteFiles bool) error
	// State returns the progress (0..1) and SeedGrid state of a torrent (migration).
	State(ctx context.Context, hash string) (float64, string, error)
	// Shutdown terminates the Engine (the container restarts via restart policy).
	Shutdown(ctx context.Context) error
	// ContentPathOf returns the data path of a torrent by hash (ADR-026,
	// screenshots) — same method already used in migration (see adapter.go).
	ContentPathOf(ctx context.Context, hash string) (string, error)
}

// Migrator executes the migration transfer operations (ADR-018): the source
// authorizes serving; the destination pulls. It also authorizes receiving publish
// uploads (ADR-019). Implemented by transfer.Server.
type Migrator interface {
	Authorize(token, hash string, ttl time.Duration)
	Pull(ctx context.Context, host string, port int, token, destPath string) error
	// AuthorizeUpload allows receiving a publish upload under the token (ADR-019).
	AuthorizeUpload(token string, ttl time.Duration)
}

// Executor translates a command into an Engine operation.
type Executor struct {
	engine       Engine
	mig          Migrator      // nil disables migration on this Agent
	storagePath  string        // pull extraction destination
	pullRetryFor time.Duration // pull retry window (absorbs the source/destination race)

	// engineAdopted: the Engine is the operator's own qBittorrent (ADR-043). restart_engine is
	// refused — it relies on a restart policy SeedGrid set, which that Engine may not have.
	engineAdopted bool

	// pendingExit: an update_agent (ADR-027) swapped the binary and the process must die
	// for systemd to respawn it on the new one — but only AFTER the ack, otherwise the command stays
	// DISPATCHED forever. The Poller consumes it, via TakePendingExit(). The Executor
	// runs in a single goroutine (the Poller tick) → no lock.
	pendingExit bool

	// selfPath forces the update_agent (ADR-027) target. Empty (production) = os.Executable().
	// It exists so tests can operate on a disposable copy instead of the runner's
	// binary — without this, testing the swap would mean overwriting go test itself.
	selfPath string

	// pauseSettle is how long we wait for the Engine to confirm the source pause in a
	// migration (issue #8). Field, not a constant, so the timeout test doesn't
	// cost 10s of wall-clock time.
	pauseSettle time.Duration

	// Limits of the destination recheck watchdog (#79) — fields, not constants,
	// for the same reason as pauseSettle: testing stagnation with production values
	// would cost 10 minutes of wall-clock time per case.
	recheckStall time.Duration
	recheckMax   time.Duration
	recheckPoll  time.Duration
}

// NewExecutor creates the executor over an Engine.
func NewExecutor(engine Engine) *Executor {
	return &Executor{
		engine:       engine,
		pauseSettle:  pauseSettleTimeout,
		recheckStall: recheckStallWindow,
		recheckMax:   recheckMaxWait,
		recheckPoll:  recheckPollEvery,
	}
}

// EnableMigration enables the migration commands (migrate_serve/migrate_pull).
func (e *Executor) EnableMigration(mig Migrator, storagePath string) *Executor {
	e.mig = mig
	e.storagePath = storagePath
	e.pullRetryFor = 90 * time.Second
	return e
}

// AdoptedEngine marks the Engine as the operator's own (ADR-043): commands that would act on the
// Engine itself, rather than on its torrents, are refused.
func (e *Executor) AdoptedEngine() *Executor {
	e.engineAdopted = true
	return e
}

type payload struct {
	Magnet      string `json:"magnet"`
	URL         string `json:"url"`
	TorrentFile string `json:"torrent_file"` // .torrent content in base64 (preferred over magnet)
	Hash        string `json:"hash"`
	DeleteFiles bool   `json:"delete_files"`
	// Migration (ADR-018).
	Token      string `json:"token"`
	ExpiresAt  string `json:"expires_at"`  // RFC3339 (migrate_serve)
	SourceHost string `json:"source_host"` // migrate_pull: where to pull from
	SourcePort int    `json:"source_port"`
	// ServeOnly (#61): the migrate_serve of a download from the Portal. A download only reads the
	// files, so the torrent keeps running; only a migration stops the source (#8).
	ServeOnly bool `json:"serve_only"`
	// MinProgress (ADR-031): progress the destination must reach after recheck for
	// the migration to be considered good — the progress the source had, minus tolerance.
	// COMPLETE torrent migration sends ~1.0 (equivalent to "requires SEEDING" in ADR-018);
	// partial sends the corresponding fraction. Missing/0 (old CP) = requires complete,
	// preserving the previous behavior.
	MinProgress float64 `json:"min_progress"`
	// AsyncRecheck (#87): the CP warns that IT watches the destination recheck and concludes the
	// migration by the torrent state. With this, migrate_pull responds DONE as soon as the
	// copy finishes and the recheck is triggered, instead of holding the Poller — and thus
	// all Node commands — until the recheck ends. Missing (CP prior to #87)
	// keeps the wait on the Agent: responding DONE before the destination is verified would make
	// this CP remove the source without proof.
	AsyncRecheck bool `json:"async_recheck"`
	// Screenshots (ADR-026).
	ScreenshotCount int `json:"screenshot_count"` // extract_screenshots: how many frames to extract
	// Self-update (ADR-027): `url` (above) is where to download the binary for this
	// Node's architecture; sha256 is mandatory (integrity) and version is for logging/tracing.
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// TakePendingExit returns (and clears) the exit request left by an update_agent
// (ADR-027). The Poller calls it AFTER confirming the command to the CP: then the process can
// die and systemd respawns it on the new binary.
func (e *Executor) TakePendingExit() bool {
	pending := e.pendingExit
	e.pendingExit = false
	return pending
}

// Execute runs a command and returns the outcome (DONE/FAILED).
func (e *Executor) Execute(ctx context.Context, cmd client.Command) client.CommandResult {
	var p payload
	if len(cmd.Payload) > 0 {
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return failed(fmt.Sprintf("invalid payload: %v", err))
		}
	}

	var err error
	switch cmd.Type {
	case "add_torrent":
		// We prefer the .torrent bytes (complete metadata, respects the
		// private flag); we only fall back to magnet/URL when there is no file.
		if p.TorrentFile != "" {
			data, decErr := base64.StdEncoding.DecodeString(p.TorrentFile)
			if decErr != nil {
				return failed(fmt.Sprintf("invalid torrent_file base64: %v", decErr))
			}
			err = e.engine.AddFile(ctx, data)
			break
		}
		src := p.Magnet
		if src == "" {
			src = p.URL
		}
		if src == "" {
			return failed("add_torrent requires torrent_file, magnet or url")
		}
		err = e.engine.AddURL(ctx, src)
	case "pause_torrent":
		err = requireHash(p.Hash, e.engine.Stop, ctx)
	case "resume_torrent":
		err = requireHash(p.Hash, e.engine.Start, ctx)
	case "recheck_torrent":
		err = requireHash(p.Hash, e.engine.Recheck, ctx)
	case "remove_torrent":
		if p.Hash == "" {
			return failed("remove_torrent requires hash")
		}
		err = e.engine.Delete(ctx, p.Hash, p.DeleteFiles)
	case "migrate_serve":
		return e.migrateServe(ctx, p)
	case "migrate_pull":
		return e.migratePull(ctx, p)
	case "prepare_upload":
		return e.prepareUpload(p)
	case "restart_engine":
		if e.engineAdopted {
			return failed("the Engine is the operator's own qBittorrent (ADR-043): SeedGrid does not restart it")
		}
		err = e.engine.Shutdown(ctx)
	case "extract_screenshots":
		return e.extractScreenshots(ctx, p)
	case "update_agent":
		return e.updateAgent(ctx, p)
	default:
		return failed(fmt.Sprintf("unknown command type: %q", cmd.Type))
	}
	if err != nil {
		return failed(err.Error())
	}
	return client.CommandResult{Status: "DONE"}
}

// migrateServe authorizes the source to serve the torrent content under the token, with
// TTL until expires_at. The service itself is passive (the destination pulls whenever it wants).
//
// PAUSE THE TORRENT BEFORE AUTHORIZING (issue #8): copying the content of a torrent that
// the Engine is still writing reads blocks halfway — the tar comes out inconsistent and
// the destination only finds out on recheck (or doesn't find out at all, if the bad piece was already
// hashed). The pause comes BEFORE Authorize on purpose: as long as the token doesn't
// exist, the destination gets 401 and there is no way to pull data in flight.
//
// If the migration fails later, the Control Plane resumes it (queues
// resume_torrent on the source) — the Agent does not keep state between commands.
func (e *Executor) migrateServe(ctx context.Context, p payload) client.CommandResult {
	if e.mig == nil {
		return failed("migration not enabled on this Agent")
	}
	if p.Hash == "" || p.Token == "" {
		return failed("migrate_serve requires hash and token")
	}
	ttl := time.Hour
	if p.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
			ttl = time.Until(t)
		}
	}
	if ttl <= 0 {
		return failed("transfer token already expired")
	}
	paused := false
	var err error
	if !p.ServeOnly {
		paused, err = e.pauseForMigration(ctx, p.Hash)
	}
	if err != nil {
		// Without a confirmed pause we don't serve: an aborted migration is recoverable
		// (the torrent stays on the source, ADR-018), corrupted data is not.
		return failed(err.Error())
	}
	e.mig.Authorize(p.Token, p.Hash, ttl)
	// The result says if the pause was ours — a trace to debug a migration that
	// failed and left the torrent stopped on the source until the CP resumes it.
	res, _ := json.Marshal(map[string]any{"paused": paused})
	return client.CommandResult{Status: "DONE", Result: res}
}

// pauseSettleTimeout limits the wait for the Engine to confirm the pause. qBittorrent
// releases the resume data and closes the files when stopping; seconds are enough on LAN.
const pauseSettleTimeout = 10 * time.Second

// pauseForMigration stops the torrent on the source and only returns when the Engine CONFIRMS the
// stopped state — sending stop and moving on immediately doesn't guarantee writing has ceased.
// Returns whether it actually paused (false = it was already stopped, or the hash isn't even in the Engine
// and no writing is possible).
func (e *Executor) pauseForMigration(ctx context.Context, hash string) (bool, error) {
	_, state, err := e.engine.State(ctx, hash)
	if err != nil {
		return false, fmt.Errorf("query torrent state on source: %w", err)
	}
	switch state {
	case "":
		// Hash unknown to the Engine: there is nothing to stop. What decides if there is
		// still data to serve anyway is handleTransfer (ErrNothingToTransfer, ADR-031).
		return false, nil
	case "PAUSED":
		return false, nil
	}
	if err := e.engine.Stop(ctx, hash); err != nil {
		return false, fmt.Errorf("stop torrent at source before migrating: %w", err)
	}
	settle := e.pauseSettle
	if settle <= 0 {
		settle = pauseSettleTimeout
	}
	deadline := time.Now().Add(settle)
	for {
		_, state, err := e.engine.State(ctx, hash)
		if err != nil {
			return false, fmt.Errorf("confirm pause on source: %w", err)
		}
		if state == "PAUSED" || state == "" {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, fmt.Errorf("torrent did not stop at source at %s (state %q) — migration aborted to avoid copying in-flight data", settle, state)
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// prepareUpload authorizes receiving a publish upload under the token, with
// TTL until expires_at (ADR-019). The CP POSTs the content right after; the creation
// of the .torrent happens in the receiving handler (transfer.Server), not here.
func (e *Executor) prepareUpload(p payload) client.CommandResult {
	if e.mig == nil {
		return failed("publishing not enabled on this Agent")
	}
	if p.Token == "" {
		return failed("prepare_upload requires token")
	}
	ttl := 30 * time.Minute
	if p.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
			ttl = time.Until(t)
		}
	}
	if ttl <= 0 {
		return failed("upload token already expired")
	}
	e.mig.AuthorizeUpload(p.Token, ttl)
	return client.CommandResult{Status: "DONE"}
}

// extractScreenshots (ADR-026) locates the torrent's main media file
// (via the Engine's ContentPath) and extracts JPEG frames via ffmpeg, returning them in
// base64 in the command result — used when publishing to UNIT3D trackers that require
// screenshots in the description.
func (e *Executor) extractScreenshots(ctx context.Context, p payload) client.CommandResult {
	if p.Hash == "" {
		return failed("extract_screenshots requires hash")
	}
	count := p.ScreenshotCount
	if count <= 0 {
		count = 4
	}
	contentPath, err := e.engine.ContentPathOf(ctx, p.Hash)
	if err != nil {
		return failed(fmt.Sprintf("locate torrent content: %v", err))
	}
	if contentPath == "" {
		return failed("torrent not found in Engine")
	}
	file := mediafile.LargestFile(contentPath)
	if file == "" {
		return failed("no media file found in torrent content")
	}
	frames, err := screenshot.Extract(ctx, file, count)
	if err != nil {
		return failed(fmt.Sprintf("extract screenshots: %v", err))
	}
	encoded := make([]string, len(frames))
	for i, f := range frames {
		encoded[i] = base64.StdEncoding.EncodeToString(f)
	}
	result, err := json.Marshal(struct {
		Frames []string `json:"frames"`
	}{Frames: encoded})
	if err != nil {
		return failed(fmt.Sprintf("build result: %v", err))
	}
	return client.CommandResult{Status: "DONE", Result: result}
}

// migratePull pulls data from the source, re-adds the .torrent, triggers the recheck, and
// starts the torrent. What happens next depends on who watches the recheck (#87):
//
//   - CP declaring async_recheck: responds DONE here, right away. DONE means "the
//     copy finished and the recheck was triggered", NOT "the destination is verified" — the one
//     who decides this, and only then removes the source, is the CP, looking at the destination torrent
//     (migration/verify.go). The Agent doesn't wait for the recheck: it is the only long process
//     of the Engine, and waiting for it here left the Node deaf to all other commands.
//   - Pre-#87 CP (missing field): waits for the destination to prove it has AT LEAST what the
//     source had (p.MinProgress) and only then responds DONE — otherwise FAILED. It's the original
//     ADR-018 contract, kept because that CP treats DONE as "verified" and
//     would remove the source without proof.
//
// ADR-031: the criterion was "is SEEDING", which failed every torrent migration
// that was incomplete even with a perfect copy. Now the floor is the source's progress: a
// 60% torrent migrates at 60% and KEEPS downloading what's missing on the destination (this is the use case
// for draining a Node out of space). A complete torrent sends MinProgress ~1.0 and behaves
// as before.
func (e *Executor) migratePull(ctx context.Context, p payload) client.CommandResult {
	if e.mig == nil {
		return failed("migration not enabled on this Agent")
	}
	if p.Hash == "" || p.Token == "" || p.SourceHost == "" || p.SourcePort == 0 {
		return failed("migrate_pull requires hash, token, source_host and source_port")
	}
	if p.TorrentFile == "" {
		return failed("migrate_pull requires torrent_file")
	}
	data, err := base64.StdEncoding.DecodeString(p.TorrentFile)
	if err != nil {
		return failed(fmt.Sprintf("invalid torrent_file base64: %v", err))
	}
	// 1) Copies data from the source to local storage. Since source and destination do
	// independent polling, the destination might run the pull before the source registers
	// the token (migrate_serve) — the source responds 401 until then. We retry for a
	// short window to absorb this race (the source authorizes in ~1 cycle).
	if err := e.pullWithRetry(ctx, p, e.pullRetryFor); err != nil {
		return failed(fmt.Sprintf("pull from source: %v", err))
	}
	// 2) Adds the .torrent pointing to the copied data (savepath and
	// content_layout fixed at e.storagePath/Original — the same place and the
	// same structure that the Pull just wrote) and forces the recheck.
	if err := e.engine.AddFileTo(ctx, data, e.storagePath); err != nil {
		return failed(fmt.Sprintf("add .torrent to destination: %v", err))
	}
	if err := e.engine.Recheck(ctx, p.Hash); err != nil {
		return failed(fmt.Sprintf("recheck on destination: %v", err))
	}
	// 2.1) Starts the torrent (#79). qBittorrent accepts a Recheck on a stopped torrent and
	// shows it in `checkingDL`, but DOES NOT verify any piece until it is
	// started: the progress stays stuck at 0 with the disk idle, and the migration died
	// on timeout even with all data intact on the destination. An explicit start unblocks the
	// verification. It comes AFTER the Recheck on purpose — starting before would make the Engine
	// treat the data as missing and start downloading from the swarm what is already on disk.
	if err := e.engine.Start(ctx, p.Hash); err != nil {
		return failed(fmt.Sprintf("start torrent on destination: %v", err))
	}
	// 3) #87: the CP watches the recheck — we don't block the Poller waiting for it.
	if p.AsyncRecheck {
		res, _ := json.Marshal(map[string]any{"recheck": "dispatched"})
		return client.CommandResult{Status: "DONE", Result: res}
	}
	// 3') Legacy path (CP without async_recheck): waits for the destination to prove, via the
	// recheck, that it has at least what the source had.
	if err := e.waitProgress(ctx, p.Hash, minProgressOf(p)); err != nil {
		return failed(err.Error())
	}
	return client.CommandResult{Status: "DONE"}
}

// pieceTolerance is the slack deducted from the required progress on the destination (ADR-031). The
// recheck invalidates the BOUNDARY piece when the download stopped in the middle of it — losing
// a piece on the edge is normal and does not invalidate the migration.
const pieceTolerance = 0.005 // 0,5%

// minProgressOf returns the progress floor that the destination needs to reach. An old CP
// (without the field) or a value outside (0,1] falls back to the ADR-018 behavior: requires complete.
func minProgressOf(p payload) float64 {
	if p.MinProgress <= 0 || p.MinProgress > 1 {
		return 0.999
	}
	if floor := p.MinProgress - pieceTolerance; floor > 0 {
		return floor
	}
	return 0
}

// pullWithRetry retries the pull until success or timeout exhaustion (absorbs the source/destination
// polling race). The extraction is idempotent (overwrites), so
// retrying is safe.
func (e *Executor) pullWithRetry(ctx context.Context, p payload, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = e.mig.Pull(ctx, p.SourceHost, p.SourcePort, p.Token, e.storagePath)
		if lastErr == nil {
			return nil
		}
		// "Nothing to transfer" is permanent (the source didn't download data): retrying for 90s
		// would just delay the same error, and the operator deserves the message right away (ADR-031).
		if errors.Is(lastErr, transfer.ErrNothingToTransfer) {
			return lastErr
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// Recheck watchdog limits (#79). The previous fixed deadline (10 min for any
// size) failed large migrations on slow disks: a 10 GB recheck on node-03's USB HDD
// takes ~15 min RUNNING, and failing in this case discards a perfect copy.
//
// What matters isn't how much time has passed, but whether the recheck ADVANCES: as long as
// progress goes up, the wait continues; if it stops going up for stallWindow, it's a real
// stall and it fails fast. maxWait is just the safety ceiling against an Engine reporting
// oscillating progress forever.
//
// ⚠️ maxWait is deliberately moderate: the Poller executes Node commands IN SERIES
// (see Poller.tick), so this command holds the Node's queue while it waits. Too high a ceiling
// would leave a Node deaf to pause/resume/remove for hours.
//
// #87: this now only applies to the LEGACY path (CP without async_recheck). With the current CP the
// wait left the Agent — the watchdog (stall + ceiling, both tunable in the catalog) lives
// in the CP's reconciler, which watches the torrent on the destination —, and that's why the ceiling there
// could go up. These constants remain for the new Agent + old CP pair, and will be removed when the
// old contract no longer needs to be supported.
const (
	recheckStallWindow = 10 * time.Minute
	recheckMaxWait     = 2 * time.Hour
	recheckPollEvery   = 3 * time.Second
	// progressEpsilon prevents Engine rounding noise from counting as progress.
	progressEpsilon = 0.0001
)

// waitProgress waits for the torrent to reach min (0..1) on the destination. With min ~1.0 it equals
// the old waitSeeding (requires the whole torrent and seeding); with a lower min, it accepts the
// migration of a partial as soon as the recheck confirms the copied data — the destination
// keeps downloading the rest from the swarm (ADR-031).
//
// The wait is governed by stall, not by total deadline (#79) — see constants above.
func (e *Executor) waitProgress(ctx context.Context, hash string, min float64) error {
	stall, max, poll := e.recheckStall, e.recheckMax, e.recheckPoll
	if stall <= 0 {
		stall = recheckStallWindow
	}
	if max <= 0 {
		max = recheckMaxWait
	}
	if poll <= 0 {
		poll = recheckPollEvery
	}
	start := time.Now()
	lastAdvance := start
	best := -1.0
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		progress, state, err := e.engine.State(ctx, hash)
		if err == nil {
			if progress >= min && (min < 0.999 || state == "SEEDING") {
				return nil
			}
			if progress > best+progressEpsilon {
				best = progress
				lastAdvance = time.Now()
			}
		}
		if stalled := time.Since(lastAdvance); stalled >= stall {
			return fmt.Errorf("recheck at destination stalled: no progress for %s (state=%q progress=%.3f, required=%.3f)",
				stalled.Round(time.Second), state, progress, min)
		}
		if time.Since(start) >= max {
			return fmt.Errorf("destination did not reach source progress in %s (state=%q progress=%.3f, required=%.3f)",
				max, state, progress, min)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func requireHash(hash string, op func(context.Context, string) error, ctx context.Context) error {
	if hash == "" {
		return fmt.Errorf("operation requires hash")
	}
	return op(ctx, hash)
}

func failed(msg string) client.CommandResult {
	return client.CommandResult{Status: "FAILED", Error: msg}
}

// Poller periodically queries the command queue and executes them.
//
// The batch is executed IN SERIES, and the next is only fetched when the current one finishes. That's why
// no command should wait for a long Engine process (#87): while a command
// waits, the entire Node becomes deaf to pause/resume/remove of ALL other torrents. The
// migrate_pull was the case — it waited for the recheck — and now it returns it to the CP. What remains
// long in the Poller is the Agent's own work (the data copy in the pull, the ffmpeg for the
// screenshots, the update download), proportional to the size of what is being moved.
type Poller struct {
	cli   *client.Client
	token func() string
	exec  *Executor
	// interval is the current polling interval, in ns. Atomic because it changes on the fly (ADR-039):
	// the Control Plane can send another value at any heartbeat and Run adopts it on the next tick.
	interval atomic.Int64
	// intervalChanged wakes up Run to adopt the new interval WITHOUT waiting for the next tick
	// (with a 30s interval, waiting would be 30s of stale behavior). Buffer 1 and
	// non-blocking send: multiple consecutive changes collapse into one, which reads the newest value.
	intervalChanged chan struct{}
	logger          *slog.Logger

	// exitFn terminates the process after an update_agent (ADR-027). Injectable because the default
	// (os.Exit) would kill the runner itself in tests. nil = does not exit (the binary has already been
	// swapped; the Agent stays on the old one until the next restart).
	exitFn func()
}

// NewPoller creates the command poller. token returns the current Agent credential.
func NewPoller(cli *client.Client, token func() string, exec *Executor, interval time.Duration, logger *slog.Logger) *Poller {
	p := &Poller{cli: cli, token: token, exec: exec, logger: logger, intervalChanged: make(chan struct{}, 1)}
	p.interval.Store(int64(interval))
	return p
}

// SetInterval changes the polling interval on the fly (ADR-039). Takes effect immediately if the Poller is
// idle, or at the end of the executing command; a value ≤ 0 is ignored — a ticker with a duration
// that is not positive will panic.
func (p *Poller) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	p.interval.Store(int64(d))
	select {
	case p.intervalChanged <- struct{}{}:
	default: // there is already a pending notification: Run will read the newest value anyway
	}
}

// Interval returns the current polling interval.
func (p *Poller) Interval() time.Duration { return time.Duration(p.interval.Load()) }

// OnSelfUpdate defines how the Agent terminates after swapping its own binary (ADR-027).
// In production it is os.Exit(0) — systemd (Restart=always) brings it back up on the new binary.
func (p *Poller) OnSelfUpdate(fn func()) *Poller {
	p.exitFn = fn
	return p
}

// Run executes the polling loop until the context is canceled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.Interval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.intervalChanged:
			// Control Plane configuration change (ADR-039). Reset in the same goroutine as the
			// select: does not race with the tick.
			ticker.Reset(p.Interval())
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	cmds, err := p.cli.Commands(ctx, p.token())
	if err != nil {
		p.logger.Error("get commands", "error", err)
		return
	}
	for _, cmd := range cmds {
		res := p.exec.Execute(ctx, cmd)
		if err := p.cli.SubmitResult(ctx, p.token(), cmd.ID, res); err != nil {
			p.logger.Error("acknowledge command", "error", err, "command_id", cmd.ID)
			// The ack failed. If it was an update_agent, the binary has ALREADY been swapped — not exiting here
			// is intentional: the CP still sees the command in flight and will re-deliver it; exiting now
			// would bring it back up on the new binary with the command pending, and the next update is
			// idempotent (same checksum, same swap). Better one less restart than a
			// command given up as lost.
			continue
		}
		p.logger.Info("command executed", "command_id", cmd.ID, "type", cmd.Type, "status", res.Status)
		// ADR-027: only now, with the outcome already recorded in the CP, can the process die —
		// systemd (Restart=always) will bring it back up on the new binary.
		if p.exec.TakePendingExit() {
			p.logger.Info("binary updated; exiting for systemd to restart with the new one",
				"command_id", cmd.ID)
			if p.exitFn != nil {
				p.exitFn()
			}
			return
		}
	}
}
