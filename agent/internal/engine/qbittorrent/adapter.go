// Package qbittorrent adapts the qBittorrent WebUI API to SeedGrid's operations
// (Engine.md §6-8). All Engine-specific dependencies are isolated here.
package qbittorrent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
)

// slotLimitRe extracts the DL slot limit per account from the UNIT3D tracker message
// (error 164): "Your slot limit is reached! You may at most download N torrents at
// the same time". The N is the very L that the CP's admission controller needs.
var slotLimitRe = regexp.MustCompile(`(?i)at most download\s+(\d+)\s+torrents`)

// Adapter is the qBittorrent WebUI API client.
type Adapter struct {
	baseURL  string
	username string
	password string
	http     *http.Client
	// paths translates between the Engine's paths and the host's (ADR-043). The zero value is
	// the identity, which is what a Node SeedGrid installs needs (`/data` is `/data` on both sides).
	paths pathmap.Map
}

// New creates the adapter. baseURL e.g.: http://127.0.0.1:8080
func New(baseURL, username, password string) *Adapter {
	jar, _ := cookiejar.New(nil)
	return &Adapter{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 20 * time.Second, Jar: jar},
	}
}

// WithPaths sets the translation between the Engine's paths and the host's (ADR-043): every
// path the adapter returns is the host's, and every path it receives is translated before it
// reaches the Engine. Returns the Adapter itself to chain into New.
func (a *Adapter) WithPaths(m pathmap.Map) *Adapter {
	a.paths = m
	return a
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
	State              string
	Progress           float64
	NumSeeds           int // swarm (scrape do tracker)
	NumLeechs          int // swarm (scrape do tracker)
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

// Login authenticates with the WebUI (required before any other operations).
// qBittorrent 5.x responds 204 (empty body) on success; older versions 200 "Ok.".
// Invalid credentials return 200 "Fails." or 403.
func (a *Adapter) Login(ctx context.Context) error {
	form := url.Values{"username": {a.username}, "password": {a.password}}
	body, status, err := a.post(ctx, "/api/v2/auth/login", form, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(body)) == "Fails." {
		return fmt.Errorf("login rejected by qBittorrent (invalid credentials)")
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("login rejected by qBittorrent (status %d)", status)
	}
	return nil
}

// AddURL adds a torrent from a magnet or .torrent URL.
func (a *Adapter) AddURL(ctx context.Context, magnetOrURL string) error {
	form := url.Values{"urls": {magnetOrURL}}
	return a.opWithRetry(ctx, "/api/v2/torrents/add", form)
}

// AddFile adds a torrent from the contents of a .torrent file
// (multipart, field "torrents"). It preserves the metadata — the Engine doesn't need
// to fetch it from the network (avoids the metaDL state) — and respects the torrent's
// private flag (no DHT/PeX/LSD), essential for private trackers.
func (a *Adapter) AddFile(ctx context.Context, data []byte) error {
	return a.uploadWithRetry(ctx, "/api/v2/torrents/add", "torrents", "upload.torrent", data, nil)
}

// AddFileTo adds a torrent pinning savepath (a host path, translated to the Engine's) and content_layout=Original
// explicitly (autoTMM=false so the savepath is not ignored). Used
// exclusively in migration (ADR-018/031): Pull() already extracted the data into
// savePath respecting the original .torrent structure (transfer.go
// extractTar); without pinning these parameters here, the Engine decides the layout by
// its own default (which may diverge from what was extracted) and the recheck doesn't
// find the already-copied data, triggering a redundant full download
// into a different folder — real observed case: migrating "The Boy and the
// Heron" to SG-1 duplicated 2.9GB on disk.
func (a *Adapter) AddFileTo(ctx context.Context, data []byte, savePath string) error {
	extra := url.Values{
		"savepath":      {a.paths.ToEngine(savePath)},
		"contentLayout": {"Original"},
		"autoTMM":       {"false"},
	}
	return a.uploadWithRetry(ctx, "/api/v2/torrents/add", "torrents", "upload.torrent", data, extra)
}

// EngineConfig gathers the operational preferences applied to the Torrent Engine
// at Agent startup (ConfigureEngine).
type EngineConfig struct {
	// BTPort is the BitTorrent listen port (matches the operator's port forward).
	BTPort int
	// SavePath is the directory where data is written (the Node's storage, e.g.: /data), as the
	// host sees it; ConfigureEngine translates it for an Engine in a container (ADR-043).
	SavePath string
	// QueueingEnabled enables qBittorrent's queueing system. When enabled (default), it limits
	// how many torrents are active at the same time — essential to avoid saturating I/O
	// when many torrents are added at once.
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

// ConfigureEngine applies the Engine's operational preferences:
//   - listen_port fixed (matches the operator's port forward) and UPnP off (manual opening);
//   - save_path: where data is written. Pinned to the Node's storage (e.g.: /data,
//     the mounted disk) — otherwise qBittorrent uses the default /downloads, which on a Node
//     with an external HD lands in the container layer (SD card), not on the disk.
//   - queueing: with QueueingEnabled=true, limits the number of concurrent active
//     torrents (downloads/uploads/checking). This is the protection against I/O saturation:
//     without queueing, adding dozens of torrents triggers dozens of downloads at the same
//     time → I/O at 100%, load through the roof, checkingResumeData stuck and (on USB HD) even
//     disk dropout from power starvation. The old risk — a stuck torrent holding
//     a slot — is handled by auto-recovery, not by disabling the queue.
func (a *Adapter) ConfigureEngine(ctx context.Context, ec EngineConfig) error {
	prefs := map[string]any{
		"listen_port": ec.BTPort,
		"upnp":        false,
		"random_port": false,
	}
	for k, v := range queuePrefs(ec) {
		prefs[k] = v
	}
	if ec.SavePath != "" {
		prefs["save_path"] = a.paths.ToEngine(ec.SavePath) // a host path, as every path the adapter receives
	}
	return a.setPreferences(ctx, prefs)
}

// ConfigureQueue reapplies ONLY the Engine's queueing (queueing, downloads/uploads/checking caps and
// the anti-stall rotation), without touching the BT port or save_path. This is what the Agent uses to
// hot-apply a configuration change coming from the Control Plane (ADR-039): BT port and
// data directory belong to the MACHINE (they match the port forward and the mounted disk) and don't
// change with cluster configuration — resending them on every cap change would be risk without gain.
func (a *Adapter) ConfigureQueue(ctx context.Context, ec EngineConfig) error {
	return a.setPreferences(ctx, queuePrefs(ec))
}

// queuePrefs are the Engine's queueing preferences. With queueing disabled only
// queueing_enabled=false is sent: the caps have no effect without it and resending them would only confuse.
func queuePrefs(ec EngineConfig) map[string]any {
	prefs := map[string]any{"queueing_enabled": ec.QueueingEnabled}
	if ec.QueueingEnabled {
		prefs["max_active_downloads"] = ec.MaxActiveDownloads
		prefs["max_active_uploads"] = ec.MaxActiveUploads
		prefs["max_active_torrents"] = ec.MaxActiveTorrents
		prefs["max_active_checking_torrents"] = ec.MaxActiveCheckingTorrents
		prefs["dont_count_slow_torrents"] = ec.DontCountSlowTorrents
		prefs["slow_torrent_dl_rate_threshold"] = ec.SlowTorrentDlRateThreshold
		prefs["slow_torrent_ul_rate_threshold"] = ec.SlowTorrentUlRateThreshold
		prefs["slow_torrent_inactive_timer"] = ec.SlowTorrentInactiveTimer
	}
	return prefs
}

func (a *Adapter) setPreferences(ctx context.Context, prefs map[string]any) error {
	b, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	form := url.Values{"json": {string(b)}}
	return a.opWithRetry(ctx, "/api/v2/app/setPreferences", form)
}

// Stop pauses (stop) a torrent by hash (qBittorrent 5.x).
func (a *Adapter) Stop(ctx context.Context, hash string) error {
	return a.opWithRetry(ctx, "/api/v2/torrents/stop", url.Values{"hashes": {hash}})
}

// Start resumes (start) a torrent by hash (qBittorrent 5.x).
func (a *Adapter) Start(ctx context.Context, hash string) error {
	return a.opWithRetry(ctx, "/api/v2/torrents/start", url.Values{"hashes": {hash}})
}

// Recheck forces verification of a torrent by hash.
func (a *Adapter) Recheck(ctx context.Context, hash string) error {
	return a.opWithRetry(ctx, "/api/v2/torrents/recheck", url.Values{"hashes": {hash}})
}

// Delete removes a torrent by hash. deleteFiles also removes the downloaded data.
func (a *Adapter) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	form := url.Values{"hashes": {hash}, "deleteFiles": {fmt.Sprintf("%t", deleteFiles)}}
	return a.opWithRetry(ctx, "/api/v2/torrents/delete", form)
}

// Export downloads the .torrent bytes of a torrent already known to the Engine (from
// qBittorrent's BT_backup). Used to send the CP the .torrent of
// magnet-added torrents that the CP doesn't have yet (backfill, ADR-018).
func (a *Adapter) Export(ctx context.Context, hash string) ([]byte, error) {
	path := "/api/v2/torrents/export?hash=" + url.QueryEscape(hash)
	do := func() ([]byte, int, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Referer", a.baseURL)
		resp, err := a.http.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // .torrent can be a few MB
		return body, resp.StatusCode, nil
	}
	body, status, err := do()
	if err != nil {
		return nil, err
	}
	if status == http.StatusForbidden {
		if err := a.Login(ctx); err != nil {
			return nil, err
		}
		body, status, err = do()
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("export %s: status %d", hash, status)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("export %s: empty response", hash)
	}
	return body, nil
}

// CreateTorrent creates a .torrent in the Engine from sourcePath (a file or
// folder already on disk, as a host path), with the given trackers, private flag and source (embedded
// tag), and immediately starts seeding (torrentcreator + startSeeding). Waits
// for the task to complete and returns the bytes of the generated .torrent (the CP extracts the infohash and
// persists it). Used in local content publishing (ADR-019). The source (ADR-020)
// matches the infohash with the UNIT3D site's (e.g.: the site's tag) — empty = no source.
func (a *Adapter) CreateTorrent(ctx context.Context, sourcePath string, trackers []string, private bool, source string) ([]byte, error) {
	form := url.Values{
		"sourcePath":        {a.paths.ToEngine(sourcePath)},
		"format":            {"v1"}, // maximum compatibility with clients/trackers
		"private":           {fmt.Sprintf("%t", private)},
		"startSeeding":      {"true"},
		"optimizeAlignment": {"true"},
	}
	if len(trackers) > 0 {
		form.Set("trackers", strings.Join(trackers, "\n"))
	}
	if source != "" {
		form.Set("source", source)
	}
	// addTask (with login-on-403).
	body, status, err := a.post(ctx, "/api/v2/torrentcreator/addTask", form, true)
	if err != nil {
		return nil, err
	}
	if status == http.StatusForbidden {
		if err := a.Login(ctx); err != nil {
			return nil, err
		}
		body, status, err = a.post(ctx, "/api/v2/torrentcreator/addTask", form, true)
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("torrentcreator/addTask: status %d: %s", status, strings.TrimSpace(string(body)))
	}
	var addResp struct {
		TaskID string `json:"taskID"`
	}
	if err := json.Unmarshal(body, &addResp); err != nil || addResp.TaskID == "" {
		return nil, fmt.Errorf("torrentcreator/addTask: unexpected response: %s", strings.TrimSpace(string(body)))
	}
	taskID := addResp.TaskID
	// Clears the task at the end (best-effort) — frees resources on the Engine.
	defer func() {
		_, _, _ = a.post(context.WithoutCancel(ctx), "/api/v2/torrentcreator/deleteTask", url.Values{"taskID": {taskID}}, true)
	}()

	// Waits for creation to complete.
	if err := a.waitCreatorFinished(ctx, taskID); err != nil {
		return nil, err
	}

	// Downloads the generated .torrent.
	data, status, err := a.get(ctx, "/api/v2/torrentcreator/torrentFile?taskID="+url.QueryEscape(taskID))
	if err != nil {
		return nil, err
	}
	if status == http.StatusForbidden {
		if err := a.Login(ctx); err != nil {
			return nil, err
		}
		data, status, err = a.get(ctx, "/api/v2/torrentcreator/torrentFile?taskID="+url.QueryEscape(taskID))
		if err != nil {
			return nil, err
		}
	}
	if status < 200 || status >= 300 || len(data) == 0 {
		return nil, fmt.Errorf("torrentcreator/torrentFile: status %d, %d bytes", status, len(data))
	}
	return data, nil
}

// waitCreatorFinished polls the creation task status until Finished (or
// Failed/timeout). Creation reads all content bytes to calculate the piece
// hashes — it can take from seconds to minutes for large files.
//
// Robustness for nodes with SLOW disks (e.g.: node-03, USB HD): while qBt hashes
// a large file it may stop responding to the WebUI for longer than the 20s
// HTTP client timeout — a poll that errors/times out does NOT abort creation: it just
// retries on the next tick until the deadline (30 min). Only a canceled ctx (the CP
// gave up), the deadline, or an explicit "Failed" status terminate with an error.
func (a *Adapter) waitCreatorFinished(ctx context.Context, taskID string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	deadline := time.Now().Add(30 * time.Minute)
	for {
		body, status, err := a.get(ctx, "/api/v2/torrentcreator/status?taskID="+url.QueryEscape(taskID))
		if err == nil && status == http.StatusForbidden {
			if lerr := a.Login(ctx); lerr != nil {
				return lerr
			}
			body, status, err = a.get(ctx, "/api/v2/torrentcreator/status?taskID="+url.QueryEscape(taskID))
		}
		// Poll ok: decide based on the task status. Poll with error (transient timeout from
		// busy qBt): ignore and retry on the next tick (does not abort).
		if err == nil && status >= 200 && status < 300 {
			// The status comes as an array of tasks (filtered by taskID).
			var tasks []struct {
				Status       string `json:"status"`
				ErrorMessage string `json:"errorMessage"`
			}
			if uerr := json.Unmarshal(body, &tasks); uerr == nil && len(tasks) > 0 {
				switch tasks[0].Status {
				case "Finished":
					return nil
				case "Failed":
					return fmt.Errorf("torrent creation failed: %s", tasks[0].ErrorMessage)
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("torrent creation timed out")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Shutdown asks qBittorrent to terminate. With a restart policy on the container
// (unless-stopped), the Engine starts right back up — used by restart_engine
// (recovery/maintenance, e.g.: after the disk reconnects). Tolerant: the connection
// may drop when the process terminates, which is the expected outcome.
func (a *Adapter) Shutdown(ctx context.Context) error {
	_, status, err := a.post(ctx, "/api/v2/app/shutdown", url.Values{}, true)
	if err != nil {
		return nil // connection dropped = terminated
	}
	if status == http.StatusForbidden {
		if lerr := a.Login(ctx); lerr != nil {
			return lerr
		}
		_, _, _ = a.post(ctx, "/api/v2/app/shutdown", url.Values{}, true)
	}
	return nil
}

// ReadQueue reads the Engine's current queue preferences — what queuePrefs writes. A Node that
// joined with the qBittorrent it already ran (ADR-043) reports them on its first contact, so the
// Control Plane seeds them as the Node's overrides and the first application changes nothing.
// Values come as qBittorrent has them: -1 is its "no limit", which the Control Plane maps.
func (a *Adapter) ReadQueue(ctx context.Context) (EngineConfig, error) {
	body, status, err := a.get(ctx, "/api/v2/app/preferences")
	if err != nil {
		return EngineConfig{}, err
	}
	if status == http.StatusForbidden {
		if lerr := a.Login(ctx); lerr != nil {
			return EngineConfig{}, lerr
		}
		if body, status, err = a.get(ctx, "/api/v2/app/preferences"); err != nil {
			return EngineConfig{}, err
		}
	}
	if status != http.StatusOK {
		return EngineConfig{}, fmt.Errorf("app/preferences: status %d", status)
	}
	var p struct {
		QueueingEnabled            bool `json:"queueing_enabled"`
		MaxActiveDownloads         int  `json:"max_active_downloads"`
		MaxActiveUploads           int  `json:"max_active_uploads"`
		MaxActiveTorrents          int  `json:"max_active_torrents"`
		MaxActiveCheckingTorrents  int  `json:"max_active_checking_torrents"`
		DontCountSlowTorrents      bool `json:"dont_count_slow_torrents"`
		SlowTorrentDlRateThreshold int  `json:"slow_torrent_dl_rate_threshold"`
		SlowTorrentUlRateThreshold int  `json:"slow_torrent_ul_rate_threshold"`
		SlowTorrentInactiveTimer   int  `json:"slow_torrent_inactive_timer"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return EngineConfig{}, fmt.Errorf("app/preferences: %w", err)
	}
	return EngineConfig{
		QueueingEnabled:            p.QueueingEnabled,
		MaxActiveDownloads:         p.MaxActiveDownloads,
		MaxActiveUploads:           p.MaxActiveUploads,
		MaxActiveTorrents:          p.MaxActiveTorrents,
		MaxActiveCheckingTorrents:  p.MaxActiveCheckingTorrents,
		DontCountSlowTorrents:      p.DontCountSlowTorrents,
		SlowTorrentDlRateThreshold: p.SlowTorrentDlRateThreshold,
		SlowTorrentUlRateThreshold: p.SlowTorrentUlRateThreshold,
		SlowTorrentInactiveTimer:   p.SlowTorrentInactiveTimer,
	}, nil
}

// Version queries the Engine version (also serves as a reachability probe:
// error/status != 200 → engine unavailable). Logs in on 403.
func (a *Adapter) Version(ctx context.Context) (string, error) {
	body, status, err := a.get(ctx, "/api/v2/app/version")
	if err != nil {
		return "", err
	}
	if status == http.StatusForbidden {
		if lerr := a.Login(ctx); lerr != nil {
			return "", lerr
		}
		body, status, err = a.get(ctx, "/api/v2/app/version")
		if err != nil {
			return "", err
		}
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("app/version: status %d", status)
	}
	return strings.TrimSpace(string(body)), nil
}

// ConnectionStatus returns the Engine's verdict on its own INBOUND
// connectability: "connected" when libtorrent has accepted at least one connection from
// outside since startup, "firewalled" when it never accepted (the BT port is not
// reachable), "disconnected" when the session is stopped.
//
// It is the only connectability signal the Node produces alone, without depending on an external
// service — and it is what distinguishes a "healthy Node" from a "healthy Node that no one reaches"
// (ADR-034). The field is `has_incoming_connections` from libtorrent exposed via the WebUI.
//
// Note on the reading: "firewalled" with a freshly started Node is not yet a verdict — it just
// means no one has knocked on the door YET. The CP decides what is sustained.
func (a *Adapter) ConnectionStatus(ctx context.Context) (string, error) {
	body, status, err := a.get(ctx, "/api/v2/transfer/info")
	if err != nil {
		return "", err
	}
	if status == http.StatusForbidden {
		if lerr := a.Login(ctx); lerr != nil {
			return "", lerr
		}
		body, status, err = a.get(ctx, "/api/v2/transfer/info")
		if err != nil {
			return "", err
		}
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("transfer/info: status %d", status)
	}
	var out struct {
		ConnectionStatus string `json:"connection_status"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("transfer/info: %w", err)
	}
	return out.ConnectionStatus, nil
}

// TrackerVerdict is what the tracker responded on a torrent's announce.
type TrackerVerdict struct {
	Status  int    // libtorrent status: 2=working, 4=not working, 5=refused…
	Message string // raw tracker message (e.g.: "Torrent has been deleted.")
}

// TrackerVerdicts returns, by hash, the REAL tracker verdict for each torrent.
//
// It is the signal missing from the product: a torrent deleted on the tracker remains SEEDING,
// green in the Portal, announcing to those who no longer know it. Measured on 06/09/2026 in the
// cluster: out of 983 torrents, 980 announced normally and 3 responded
// "Torrent has been deleted." — and the only way to find this out was to SSH into the four
// Nodes and query torrent by torrent.
//
// Ignores DHT/PeX/LSD: they are pseudo-trackers that qBittorrent lists together and never
// carry a private tracker verdict. It keeps the WORST status found (the highest), which is
// what matters — a torrent with one tracker refusing and another ok has a problem.
//
// Best-effort per torrent: a hash that disappeared between Info and the query is skipped, it does not
// break the scan of the others.
func (a *Adapter) TrackerVerdicts(ctx context.Context) (map[string]TrackerVerdict, error) {
	torrents, err := a.Info(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]TrackerVerdict, len(torrents))
	for _, t := range torrents {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		body, status, err := a.get(ctx, "/api/v2/torrents/trackers?hash="+url.QueryEscape(t.Hash))
		if err != nil {
			continue
		}
		if status == http.StatusForbidden {
			if lerr := a.Login(ctx); lerr != nil {
				return out, lerr
			}
			body, status, err = a.get(ctx, "/api/v2/torrents/trackers?hash="+url.QueryEscape(t.Hash))
			if err != nil {
				continue
			}
		}
		if status != http.StatusOK {
			continue
		}
		var trs []struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
			Msg    string `json:"msg"`
		}
		if err := json.Unmarshal(body, &trs); err != nil {
			continue
		}
		v := TrackerVerdict{}
		for _, tr := range trs {
			if strings.HasPrefix(tr.URL, "**") { // ** [DHT] **, ** [PeX] **, ** [LSD] **
				continue
			}
			if tr.Status >= v.Status {
				v.Status = tr.Status
				if m := strings.TrimSpace(tr.Msg); m != "" {
					v.Message = m
				}
			}
		}
		out[strings.ToLower(t.Hash)] = v
	}
	return out, nil
}

// IncomingPeers counts the INBOUND connections open right now, summed over all
// torrents. A peer is inbound when the "I" flag appears in `flags` — it is libtorrent
// itself saying the other side dialed in, meaning someone from the outside
// managed to reach this Node's BT port.
//
// Why it exists, given there is already ConnectionStatus: the `has_incoming_connections` that
// method reads is STICKY — it turns true on the first connection of the session and never goes back
// as long as the Engine doesn't restart (#155). It answers "this session has received someone
// at some point", not "this Node is reachable right now". Measured on 06/09/2026: node-01 and
// node-03 reported "connected" with the Engine up for 1 month and 5 days, going through
// the entire period where they were proven unreachable. This count, on the
// other hand, DECAYS: it vanishes when the connections disappear.
//
// ⚠️ The instantaneous value is NOT a verdict and should not be read as such. Measured on the four
// Nodes on 07/09/2026, all healthy: 0, 1, 1, and 0 inbound connections. In an idle
// seeding catalog, an inbound connection is a RARE EVENT, not a continuous state — alerting on
// "0 right now" would yield false positives on healthy Nodes. Who accumulates this over time (the timestamp of the
// last positive) and decides what is sustained is the Control Plane.
//
// Cost: one call per torrent. Measured on 07/09/2026, ~1 ms each with the Engine on
// localhost — 417 torrents in 0.3 s. It grows with the number of PEERS in the response, not
// with the number of torrents, so a busy swarm costs more than this number suggests.
func (a *Adapter) IncomingPeers(ctx context.Context) (int, error) {
	torrents, err := a.Info(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, t := range torrents {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		body, status, err := a.get(ctx, "/api/v2/sync/torrentPeers?hash="+url.QueryEscape(t.Hash))
		if err != nil {
			// Best-effort per torrent: a hash that disappeared in the middle of the scan (removed
			// between Info and this call) cannot zero out the count of the others.
			continue
		}
		if status == http.StatusForbidden {
			if lerr := a.Login(ctx); lerr != nil {
				return 0, lerr
			}
			body, status, err = a.get(ctx, "/api/v2/sync/torrentPeers?hash="+url.QueryEscape(t.Hash))
			if err != nil {
				continue
			}
		}
		if status != http.StatusOK {
			continue
		}
		var out struct {
			Peers map[string]struct {
				Flags string `json:"flags"`
			} `json:"peers"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			continue
		}
		for _, p := range out.Peers {
			if strings.Contains(p.Flags, "I") {
				total++
			}
		}
	}
	return total, nil
}

// Info lists the torrents known to the Engine.
func (a *Adapter) Info(ctx context.Context) ([]Torrent, error) {
	body, status, err := a.get(ctx, "/api/v2/torrents/info")
	if err != nil {
		return nil, err
	}
	if status == http.StatusForbidden {
		if err := a.Login(ctx); err != nil {
			return nil, err
		}
		body, _, err = a.get(ctx, "/api/v2/torrents/info")
		if err != nil {
			return nil, err
		}
	}
	var raw []struct {
		Hash          string  `json:"hash"`
		Name          string  `json:"name"`
		State         string  `json:"state"`
		Progress      float64 `json:"progress"`
		NumSeeds      int     `json:"num_seeds"`
		NumLeechs     int     `json:"num_leechs"`
		DlSpeed       int64   `json:"dlspeed"`
		UpSpeed       int64   `json:"upspeed"`
		Size          int64   `json:"size"`
		Ratio         float64 `json:"ratio"`
		ETA           int64   `json:"eta"`
		Downloaded    int64   `json:"downloaded"`
		Uploaded      int64   `json:"uploaded"`
		ContentPath   string  `json:"content_path"`
		NumComplete   int     `json:"num_complete"`   // seeds no swarm (scrape do tracker)
		NumIncomplete int     `json:"num_incomplete"` // leechers no swarm
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode torrents: %w", err)
	}
	// In SWARM fields we prefer the scrape totals (num_complete/num_incomplete) —
	// the number the user understands as "seeds/peers of the torrent". They fall back to connected
	// when the tracker hasn't reported yet (-1). The CONNECTED ones (num_seeds/num_leechs) go
	// into their own fields, without collapsing: they are the only evidence that the torrent can
	// actually download from someone (the scrape might announce ghost seeders).
	pick := func(swarm, connected int) int {
		if swarm >= 0 {
			return swarm
		}
		return connected
	}
	out := make([]Torrent, 0, len(raw))
	for _, t := range raw {
		out = append(out, Torrent{
			Hash: t.Hash, Name: t.Name, State: t.State, Progress: t.Progress,
			NumSeeds: pick(t.NumComplete, t.NumSeeds), NumLeechs: pick(t.NumIncomplete, t.NumLeechs),
			NumSeedsConnected: t.NumSeeds, NumLeechsConnected: t.NumLeechs,
			DlSpeed: t.DlSpeed, UpSpeed: t.UpSpeed, Size: t.Size, Ratio: t.Ratio, ETA: t.ETA,
			Downloaded: t.Downloaded, Uploaded: t.Uploaded, ContentPath: a.paths.ToHost(t.ContentPath),
		})
	}
	return out, nil
}

// State returns the progress (0..1) and the SeedGrid state of a torrent by hash.
// Used in migration for the destination to confirm SEEDING after the recheck (ADR-018).
// Returns ("", 0) if the hash is not in the Engine.
func (a *Adapter) State(ctx context.Context, hash string) (float64, string, error) {
	torrents, err := a.Info(ctx)
	if err != nil {
		return 0, "", err
	}
	for _, t := range torrents {
		if strings.EqualFold(t.Hash, hash) {
			return t.Progress, MapState(t.State), nil
		}
	}
	return 0, "", nil
}

// ContentPathOf returns the data path of a torrent by hash (for the
// source to serve the content during migration). "" if the hash is not in the Engine.
func (a *Adapter) ContentPathOf(ctx context.Context, hash string) (string, error) {
	torrents, err := a.Info(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range torrents {
		if strings.EqualFold(t.Hash, hash) {
			return t.ContentPath, nil
		}
	}
	return "", nil
}

// SlotLimit probes the tracker message for up to len(hashes) torrents and returns the
// DL slot limit per account announced by the tracker (UNIT3D error 164), when some
// torrent is hitting the limit ("You may at most download N torrents"). Stops at the
// first N found. (0,false) if no msg matches or on any error — it is
// best-effort: the CP falls back to the config fallback when there is no observation.
func (a *Adapter) SlotLimit(ctx context.Context, hashes []string) (int, bool) {
	for _, h := range hashes {
		if h == "" {
			continue
		}
		body, status, err := a.get(ctx, "/api/v2/torrents/trackers?hash="+url.QueryEscape(h))
		if err != nil || status != http.StatusOK {
			continue
		}
		var trackers []struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal(body, &trackers); err != nil {
			continue
		}
		for _, t := range trackers {
			if m := slotLimitRe.FindStringSubmatch(t.Msg); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
					return n, true
				}
			}
		}
	}
	return 0, false
}

// MapState translates qBittorrent state to the official SeedGrid state
// (Database.md §36).
func MapState(qbtState string) string {
	switch qbtState {
	case "error", "missingFiles":
		return "ERROR"
	case "uploading", "stalledUP", "forcedUP", "queuedUP":
		// queuedUP = COMPLETE torrent waiting for upload slot (queueing enabled) —
		// already seeding/ready; it is SEEDING, not QUEUED. Only queuedDL (queue of
		// DOWNLOAD) is QUEUED.
		return "SEEDING"
	case "downloading", "metaDL", "stalledDL", "forcedDL", "allocating":
		return "DOWNLOADING"
	case "pausedUP", "pausedDL", "stoppedUP", "stoppedDL":
		return "PAUSED"
	case "queuedDL":
		return "QUEUED"
	case "checkingUP", "checkingDL", "checkingResumeData":
		return "CHECKING"
	case "moving":
		return "MOVING"
	default:
		return "UNKNOWN"
	}
}

// opWithRetry executes a POST operation, retrying login once on 403.
func (a *Adapter) opWithRetry(ctx context.Context, path string, form url.Values) error {
	body, status, err := a.post(ctx, path, form, true)
	if err != nil {
		return err
	}
	if status == http.StatusForbidden {
		if err := a.Login(ctx); err != nil {
			return err
		}
		body, status, err = a.post(ctx, path, form, true)
		if err != nil {
			return err
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%s: status %d: %s", path, status, strings.TrimSpace(string(body)))
	}
	return nil
}

// uploadWithRetry sends a multipart POST with a file (plus extra fields,
// if any), retrying login once on 403.
func (a *Adapter) uploadWithRetry(ctx context.Context, path, field, filename string, data []byte, extra url.Values) error {
	body, status, err := a.postMultipart(ctx, path, field, filename, data, extra)
	if err != nil {
		return err
	}
	if status == http.StatusForbidden {
		if err := a.Login(ctx); err != nil {
			return err
		}
		body, status, err = a.postMultipart(ctx, path, field, filename, data, extra)
		if err != nil {
			return err
		}
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%s: status %d: %s", path, status, strings.TrimSpace(string(body)))
	}
	return nil
}

func (a *Adapter) postMultipart(ctx context.Context, path, field, filename string, data []byte, extra url.Values) ([]byte, int, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, vals := range extra {
		for _, v := range vals {
			if err := w.WriteField(k, v); err != nil {
				return nil, 0, err
			}
		}
	}
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		return nil, 0, err
	}
	if _, err := fw.Write(data); err != nil {
		return nil, 0, err
	}
	if err := w.Close(); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, &buf)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", a.baseURL)
	req.Header.Set("Origin", a.baseURL)
	return a.send(req)
}

func (a *Adapter) post(ctx context.Context, path string, form url.Values, referer bool) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// qBittorrent applies CSRF protection based on Referer/Origin.
	req.Header.Set("Referer", a.baseURL)
	req.Header.Set("Origin", a.baseURL)
	return a.send(req)
}

func (a *Adapter) get(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Referer", a.baseURL)
	return a.send(req)
}

func (a *Adapter) send(req *http.Request) ([]byte, int, error) {
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, resp.StatusCode, nil
}
