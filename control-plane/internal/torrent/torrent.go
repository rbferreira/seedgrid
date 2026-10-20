// Package torrent implements the Torrent Service: adds torrents to the Cluster
// delegating Node selection to the Scheduler (the operator never chooses — Vision §5).
package torrent

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/activity"
	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
	"github.com/rbferreira/seedgrid/control-plane/internal/imgbb"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/screenshot"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrentfile"
)

// ErrNotFound indicates a non-existent torrent; ErrNoHash, a torrent without a hash (cannot
// operate on the Engine — e.g., added via URL without a resolved infohash).
var (
	ErrNotFound = errors.New("torrent not found")
	ErrNoHash   = errors.New("torrent without hash")
	// ErrNodeNotEligible: forced (or chosen) Node is non-existent/offline/has no IP.
	ErrNodeNotEligible = errors.New("destination node not found or offline")
	// ErrTicketInvalid: non-existent/expired/already used download ticket (#1).
	ErrTicketInvalid = errors.New("invalid or expired download ticket")
)

// DuplicateError indicates that the info_hash already exists in the cluster (on ≥1 Node). The add
// returns this — instead of silently duplicating — when allowDuplicate=false (#11).
// Existing is the NAME of the already present torrent: it may differ from what was attempted to be added
// (e.g., mislabeled record on the tracker), which makes the collision self-explanatory.
type DuplicateError struct {
	Nodes    []string // hostnames of the Nodes that already have the torrent
	Existing string   // name of the torrent already present with this hash
}

func (e *DuplicateError) Error() string {
	msg := "torrent already exists in cluster"
	if e.Existing != "" {
		msg += " as «" + e.Existing + "»"
	}
	return msg + " (Node(s): " + strings.Join(e.Nodes, ", ") + ")"
}

// Torrent is the representation of a Cluster torrent exposed by the API.
type Torrent struct {
	ID        string     `json:"id"`
	UUID      string     `json:"uuid"`
	NodeID    string     `json:"node_id"`
	Hash      *string    `json:"hash,omitempty"`
	Name      *string    `json:"name,omitempty"`
	Status    string     `json:"status"`
	Progress  *float64   `json:"progress,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	NumSeeds  *int       `json:"num_seeds,omitempty"`
	NumLeechs *int       `json:"num_leechers,omitempty"`
	// Connected: seeds/leechers actually CONNECTED to the Engine now (vs. num_seeds/leechers
	// which are the swarm scrape reported by the tracker — may be 0/stale while
	// the torrent downloads fast from a connected peer). The Portal shows both to avoid seeming
	// contradictory ("0 seeds but downloading"). Populated by the Agent (see adapter.go).
	NumSeedsConnected  *int     `json:"num_seeds_connected,omitempty"`
	NumLeechsConnected *int     `json:"num_leechers_connected,omitempty"`
	DlSpeed            *int64   `json:"dl_speed,omitempty"`
	UpSpeed            *int64   `json:"up_speed,omitempty"`
	SizeBytes          *int64   `json:"size_bytes,omitempty"`
	Ratio              *float64 `json:"ratio,omitempty"`
	ETASecs            *int64   `json:"eta_seconds,omitempty"`
	Downloaded         *int64   `json:"downloaded_bytes,omitempty"`
	Uploaded           *int64   `json:"uploaded_bytes,omitempty"`
	// Cumulative totals (backlog #14): survive the Engine session reset —
	// basis for the aggregated ratio per Node/Cluster. See migration 00022 and SyncStates.
	DownloadedTotal int64 `json:"downloaded_total"`
	UploadedTotal   int64 `json:"uploaded_total"`
	// PrioritizedAt (F3): when not null, the operator manually prioritized this torrent
	// (bypasses the admission queue). The Portal shows it in yellow. See migration 00032.
	PrioritizedAt *time.Time `json:"prioritized_at,omitempty"`
}

// dlTicket is a one-time download ticket (#1): maps an ephemeral token to the torrent,
// so the browser can download without sending the session token in the URL.
type dlTicket struct {
	torrentID string
	expires   time.Time
}

// TrackerProfileProvider reads the image_host section of a tracker profile for the screenshot
// chain (ADR-040); nil = the profile does not have the section. Satisfied by trackerprofile.Service
// — the interface exists so this package does not depend on trackerprofile.
type TrackerProfileProvider interface {
	ImageHostSpec(ctx context.Context, userID, trackerID string) (*imagehost.Spec, error)
}

// ImageHostKeyProvider returns the key and the host recorded with it for the screenshot chain
// (#193). Satisfied by tracker.Service.
type ImageHostKeyProvider interface {
	ImageHostKeyForChain(ctx context.Context, userID, trackerID string) (key, host string, err error)
}

// ScreenshotResult is the return of the screenshot routes with the chain (ADR-040). Failures
// lists the hosts that failed or were skipped before the winner (or all, when none
// won).
type ScreenshotResult struct {
	BBCode   string
	Host     string
	Failures []imagehost.Failure
}

// Service concentrates operations on torrents.
type Service struct {
	pool     *pgxpool.Pool
	sched    *scheduler.Service
	cmds     *command.Service
	events   *events.Emitter
	activity *activity.Recorder // activity trail (ADR-022); nil = off (best-effort)
	pubPort  int                // Agent's receiving port for publishing (ADR-019); same as the migration
	minFree  func() int64       // free disk margin (ADR-032), read per call (hot-reload); nil = 0

	// Providers for the screenshot chain (ADR-040, #193). Injected by main;
	// nil = chain without tracker host (only imgbb, legacy behavior).
	profiles     TrackerProfileProvider
	imageHostKey ImageHostKeyProvider

	dlMu      sync.Mutex
	dlTickets map[string]dlTicket // live download tickets (#1)

	tlMu         sync.Mutex
	trackerLinks map[string]string // "trackerID|torrentID" → download_link from the last browse (#12)

	// Decimation of time series samples (#113): minimum interval between batches
	// per Node, read per call (hot-reload); nil/<=0 = sample on every sync.
	sampleEvery func() time.Duration
	sampleMu    sync.Mutex
	lastSample  map[string]time.Time
}

// hostname fetches the Node's hostname for external notification (ADR-028) — same data
// that the Alert Worker already resolves in its alerts. Best-effort: "" if not found (notify
// falls back to the raw id, harder to read but never breaks the notification).
func (s *Service) hostname(ctx context.Context, nodeID string) string {
	var h string
	_ = s.pool.QueryRow(ctx, `SELECT hostname FROM nodes WHERE id = $1`, nodeID).Scan(&h)
	return h
}

// NewService creates the Torrent Service.
func NewService(pool *pgxpool.Pool, sched *scheduler.Service, cmds *command.Service, emitter *events.Emitter) *Service {
	return &Service{
		pool: pool, sched: sched, cmds: cmds, events: emitter, pubPort: 6900,
		dlTickets: map[string]dlTicket{}, trackerLinks: map[string]string{},
		lastSample: map[string]time.Time{},
	}
}

// SetActivity enables the activity trail (ADR-022) — the service starts recording the
// scheduling (Scheduler decision) and lifecycle transitions observed in
// SyncStates. Optional (injected in main); nil-safe.
func (s *Service) SetActivity(rec *activity.Recorder) { s.activity = rec }

// SetTrackerProfileProvider injects the tracker profiles provider for the screenshot
// chain (ADR-040, #193). Optional; nil = chain without tracker host.
func (s *Service) SetTrackerProfileProvider(p TrackerProfileProvider) { s.profiles = p }

// SetImageHostKeyProvider injects the image host keys provider for the screenshot
// chain (#193). Optional; nil = chain without tracker host.
func (s *Service) SetImageHostKeyProvider(p ImageHostKeyProvider) { s.imageHostKey = p }

// SetDiskMinFree injects the free disk margin provider (ADR-032), read per
// call to respect the panel's hot-reload. nil-safe (diskMinFree returns 0).
func (s *Service) SetDiskMinFree(fn func() int64) { s.minFree = fn }

// SetSampleInterval injects the minimum interval between time series sample
// batches per Node (#113), read per call (hot-reload). nil-safe.
func (s *Service) SetSampleInterval(fn func() time.Duration) { s.sampleEvery = fn }

// diskMinFree reads the current margin (0 if not injected).
func (s *Service) diskMinFree() int64 {
	if s.minFree == nil {
		return 0
	}
	return s.minFree()
}

// RememberTrackerLinks stores the download_link (id→link) from a tracker's browse
// page, so the seed can download without relying on the /api/torrents/{id} endpoint (#12).
func (s *Service) RememberTrackerLinks(trackerID string, links map[string]string) {
	if len(links) == 0 {
		return
	}
	s.tlMu.Lock()
	defer s.tlMu.Unlock()
	if len(s.trackerLinks) > 5000 { // simple anti-growth quota
		s.trackerLinks = map[string]string{}
	}
	for id, l := range links {
		if l != "" {
			s.trackerLinks[trackerID+"|"+id] = l
		}
	}
}

// TrackerLink returns the cached download_link of a catalog item (#12).
func (s *Service) TrackerLink(trackerID, torrentID string) (string, bool) {
	s.tlMu.Lock()
	defer s.tlMu.Unlock()
	l, ok := s.trackerLinks[trackerID+"|"+torrentID]
	return l, ok
}

// EnablePublish sets the Agent's receiving server port (ADR-019),
// used for publishing local content. If not called, uses default 6900.
func (s *Service) EnablePublish(port int) {
	if port > 0 {
		s.pubPort = port
	}
}

// Add adds a torrent from a magnet: the Scheduler chooses the Node, the
// torrent is registered (QUEUED), the decision is audited and an add_torrent command is
// queued to the Agent.
func (s *Service) Add(ctx context.Context, magnet, name string, opts AddOptions) (Torrent, error) {
	return s.add(ctx, magnet, name, nil, 0, opts)
}

// AddFile adds a torrent from a .torrent file. It parses
// (infohash + name + trackers) to register the torrent, but forwards the original
// bytes to the Agent: this way the Engine receives the full metadata (without going
// through metaDL) and honors the torrent's private flag — essential for private
// trackers, where there is no DHT to fetch metadata.
func (s *Service) AddFile(ctx context.Context, data []byte, opts AddOptions) (Torrent, error) {
	meta, err := torrentfile.Parse(data)
	if err != nil {
		return Torrent{}, fmt.Errorf("invalid .torrent file: %w", err)
	}
	// meta.Size (ADR-032) allows the Scheduler to block Nodes with no space for the torrent.
	return s.add(ctx, buildMagnet(meta), meta.Name, data, meta.Size, opts)
}

// Create publishes local content (ADR-019): chooses the Node (Scheduler, or the
// one forced by the operator), streams the content to the Agent (which writes to
// storage, creates the .torrent in the Engine with the announce/private flag and starts
// seeding), registers the torrent in the Cluster State and persists the generated .torrent.
// body is the content packed as a tar (assembled by the handler from the
// browser's multipart); the CP streams it, without storing it.
// trackerID (optional) links the torrent to the chosen tracker: when filled and
// without published_tracker_at, the torrent enters the "pending publication" queue
// (ADR-020, backlog #8) — so the operator can publish later without re-uploading.
func (s *Service) Create(ctx context.Context, name, announce, source, trackerID string, private bool, forceNodeID string, wantMediainfo bool, body io.Reader) (Torrent, string, error) {
	nodeID, nodeIP, score, reason, candidates, err := s.pickPublishNode(ctx, forceNodeID)
	if err != nil {
		return Torrent{}, "", err
	}

	// One-time token: the CP queues prepare_upload (the Agent authorizes reception) and
	// only then POSTs the content.
	token, err := randomToken()
	if err != nil {
		return Torrent{}, "", err
	}
	expiresAt := time.Now().Add(30 * time.Minute)
	pl, _ := json.Marshal(map[string]any{"token": token, "expires_at": expiresAt.UTC().Format(time.RFC3339)})
	cmdID, err := s.cmds.Enqueue(ctx, nodeID, "prepare_upload", pl)
	if err != nil {
		return Torrent{}, "", fmt.Errorf("enqueue prepare_upload: %w", err)
	}
	if err := s.waitCommandDone(ctx, cmdID, 60*time.Second); err != nil {
		return Torrent{}, "", fmt.Errorf("node did not prepare publish: %w", err)
	}

	torrentBytes, mediaInfo, err := s.postContent(ctx, nodeIP, token, name, announce, source, private, wantMediainfo, body)
	if err != nil {
		return Torrent{}, "", err
	}

	meta, err := torrentfile.Parse(torrentBytes)
	if err != nil {
		return Torrent{}, "", fmt.Errorf(".torrent generated invalid: %w", err)
	}
	hash := strings.ToLower(meta.InfoHash)
	tName := name
	if meta.Name != "" {
		tName = meta.Name
	}

	// Torrent row + publishing decision in a SINGLE transaction (#117): failure to
	// log the decision cannot leave the torrent half-registered (the Node is already
	// seeding; the CP must reflect all or nothing and return a clean error).
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Torrent{}, "", fmt.Errorf("open publish transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var t Torrent
	if err := tx.QueryRow(ctx,
		`INSERT INTO torrents (node_id, hash, name, status, media_info, tracker_id)
		 VALUES ($1, $2, $3, 'CHECKING', NULLIF($4, ''), NULLIF($5, '')::uuid)
		 ON CONFLICT (node_id, hash) WHERE deleted_at IS NULL DO UPDATE SET
		    name = EXCLUDED.name,
		    media_info = COALESCE(EXCLUDED.media_info, torrents.media_info),
		    tracker_id = COALESCE(EXCLUDED.tracker_id, torrents.tracker_id),
		    updated_at = now()
		 RETURNING id::text, uuid::text, node_id::text, hash, name, status`,
		nodeID, hash, tName, mediaInfo, trackerID).Scan(&t.ID, &t.UUID, &t.NodeID, &t.Hash, &t.Name, &t.Status); err != nil {
		return Torrent{}, "", fmt.Errorf("register published torrent: %w", err)
	}
	// Audit of the decision (Scheduler or forced bypass — ADR-019 requires logging).
	if _, err := tx.Exec(ctx,
		`INSERT INTO torrent_scheduler (torrent_id, selected_node, decision_score, decision_reason, candidates, algorithm_version)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, nodeID, score, reason, candidates, "publish"); err != nil {
		return Torrent{}, "", fmt.Errorf("register publish decision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Torrent{}, "", fmt.Errorf("commit publish: %w", err)
	}
	// The created .torrent is the source of truth (migration/re-provisioning).
	// Post-commit and best-effort: the backfill (ADR-018) recovers from the Engine if it fails.
	_ = s.StoreTorrentFile(ctx, t.ID, torrentBytes, "created")
	if s.events != nil {
		_ = s.events.Emit(ctx, "TORRENT_CREATED", "INFO", nodeID,
			fmt.Sprintf("Content published (torrent created on Node): %s", label(tName, hash)),
			map[string]any{"hash": hash, "name": tName, "private": private, "hostname": s.hostname(ctx, nodeID), "origin": "created"})
	}
	return t, mediaInfo, nil
}

// VerifyReseed compares the infohash of the OFFICIAL .torrent (what the tracker registered upon
// approval) with what the Node is seeding. If they diverge (#23 — the site regenerates the
// .torrent with its own `source`/piece size, changing the infohash; if ours came out without
// this, the tracker responds "InfoHash not found" and we never count it as a seed), it re-seeds
// automatically: updates the row to the official hash (preserving tracker_id/
// published/media_info since it's the SAME row), persists the official .torrent and orchestrates
// the swap on the Node — adds the official one (qBt rechecks against data already in /data →
// SEEDING, no re-download) and removes the old infohash WITHOUT deleting the data.
// Returns reseeded=true when fixed; false when the infohash already matched.
func (s *Service) VerifyReseed(ctx context.Context, torrentID string, officialBytes []byte) (bool, error) {
	meta, err := torrentfile.Parse(officialBytes)
	if err != nil {
		return false, fmt.Errorf("parse official .torrent: %w", err)
	}
	newHash := strings.ToLower(meta.InfoHash)
	nodeID, oldHash, _, err := s.lookup(ctx, torrentID)
	if err != nil {
		return false, err
	}
	if newHash == "" || newHash == strings.ToLower(oldHash) {
		return false, nil // infohash matches the site's: nothing to do
	}

	// Updates the row to the official hash (keeps tracker_id/published_tracker_at/
	// media_info — it's the same row). Uniqueness (node_id, hash) may collide with a
	// live row of the same hash: in this case the error bubbles up and we don't re-seed.
	//
	// New hash + the two swap commands on the Node (add the official, remove the old one
	// WITHOUT deleting data) in a SINGLE transaction (#117): failure at any step → rollback,
	// nothing orphaned. Without this, a failure between add and remove left two torrents in the
	// Engine for the same row — same pattern as the 27k remove_torrent incident.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("open re-seed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`UPDATE torrents SET hash = $2, updated_at = now() WHERE id = $1`, torrentID, newHash); err != nil {
		return false, fmt.Errorf("update hash to official: %w", err)
	}
	addPayload, _ := json.Marshal(map[string]string{"torrent_file": base64.StdEncoding.EncodeToString(officialBytes)})
	if _, err := s.cmds.EnqueueTx(ctx, tx, nodeID, "add_torrent", addPayload); err != nil {
		return false, fmt.Errorf("enqueue add for official .torrent: %w", err)
	}
	rmPayload, _ := json.Marshal(map[string]any{"hash": oldHash, "delete_files": false})
	if _, err := s.cmds.EnqueueTx(ctx, tx, nodeID, "remove_torrent", rmPayload); err != nil {
		return false, fmt.Errorf("enqueue old infohash removal: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit re-seed: %w", err)
	}
	// The official .torrent becomes the source of truth (migration/re-provisioning).
	// Post-commit and best-effort: if it fails, the backfill (ADR-018) recovers from the Engine.
	_ = s.StoreTorrentFile(ctx, torrentID, officialBytes, "tracker_official")
	if s.events != nil {
		_ = s.events.Emit(ctx, "TORRENT_RESEEDED", "WARNING", nodeID,
			fmt.Sprintf("Infohash mismatch from tracker fixed by re-seed: %s", label(meta.Name, newHash)),
			map[string]any{"old_hash": oldHash, "new_hash": newHash, "name": meta.Name, "hostname": s.hostname(ctx, nodeID)})
	}
	return true, nil
}

// pickPublishNode resolves the publishing Node and its IP. With forceNodeID, it validates
// that the Node exists, is ONLINE and has an IP; otherwise it delegates to the Scheduler.
func (s *Service) pickPublishNode(ctx context.Context, forceNodeID string) (nodeID, nodeIP string, score int, reason string, candidates int, err error) {
	if forceNodeID != "" {
		var ip *string
		var online bool
		qerr := s.pool.QueryRow(ctx,
			`SELECT last_ip, status = 'ONLINE' FROM nodes WHERE id = $1 AND deleted_at IS NULL`,
			forceNodeID).Scan(&ip, &online)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return "", "", 0, "", 0, ErrNodeNotEligible
		}
		if qerr != nil {
			return "", "", 0, "", 0, qerr
		}
		if !online || ip == nil || *ip == "" {
			return "", "", 0, "", 0, ErrNodeNotEligible
		}
		return forceNodeID, *ip, 0, "Node forced by operator (Scheduler bypass)", 0, nil
	}
	// Publishing (ADR-019): content size is only known when packing, after
	// this choice → needBytes=0 (no disk filter). The Node with the most free space wins the
	// tiebreaker; a full Node still triggers the NODE_DISK_FULL alert network (ADR-032).
	dec, derr := s.sched.Select(ctx, 0, s.diskMinFree())
	if derr != nil {
		return "", "", 0, "", 0, derr // includes scheduler.ErrNoEligibleNode
	}
	var ip *string
	if qerr := s.pool.QueryRow(ctx,
		`SELECT last_ip FROM nodes WHERE id = $1 AND deleted_at IS NULL`, dec.NodeID).Scan(&ip); qerr != nil {
		return "", "", 0, "", 0, qerr
	}
	if ip == nil || *ip == "" {
		return "", "", 0, "", 0, ErrNodeNotEligible
	}
	return dec.NodeID, *ip, dec.Score, dec.Reason, dec.Candidates, nil
}

// waitCommandDone waits for a command to reach DONE (or fail/timeout) — used to
// wait for the Agent to authorize reception before the CP POSTs the content.
// Delegates the polling wait to command.Service (ADR-026), which generalized what
// was previously implemented right here (ADR-019).
func (s *Service) waitCommandDone(ctx context.Context, cmdID string, timeout time.Duration) error {
	st, err := s.cmds.WaitDone(ctx, cmdID, timeout)
	if err != nil {
		return err
	}
	if st.Status == "FAILED" {
		if st.Error != nil {
			return errors.New(*st.Error)
		}
		return errors.New("command failed on the Node")
	}
	return nil
}

// postContent streams the POST of the content (tar) to the Agent and returns the
// bytes of the .torrent that the Agent generated. No timeout on the client (GB uploads); the
// ctx controls the cancellation.
func (s *Service) postContent(ctx context.Context, nodeIP, token, name, announce, source string, private, wantMediainfo bool, body io.Reader) ([]byte, string, error) {
	u := fmt.Sprintf("http://%s/content?token=%s",
		net.JoinHostPort(nodeIP, strconv.Itoa(s.pubPort)), url.QueryEscape(token))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("X-Seedgrid-Name", base64.StdEncoding.EncodeToString([]byte(name)))
	req.Header.Set("X-Seedgrid-Private", fmt.Sprintf("%t", private))
	if announce != "" {
		req.Header.Set("X-Seedgrid-Announce", base64.StdEncoding.EncodeToString([]byte(announce)))
	}
	// source (ADR-020): tag embedded in the .torrent to match the infohash with the
	// UNIT3D site's (e.g., EXAMPLE). Empty = no source (regular tracker).
	if source != "" {
		req.Header.Set("X-Seedgrid-Source", base64.StdEncoding.EncodeToString([]byte(source)))
	}
	if wantMediainfo {
		req.Header.Set("X-Seedgrid-Mediainfo", "true")
	}
	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("send content to Node: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("node rejected publish (status %d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		TorrentFile string `json:"torrent_file"`
		MediaInfo   string `json:"media_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, "", fmt.Errorf("invalid Node response: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(out.TorrentFile)
	if err != nil || len(data) == 0 {
		return nil, "", fmt.Errorf(".torrent returned by Node invalid")
	}
	return data, out.MediaInfo, nil
}

// randomToken generates a random token (32-byte hex) for the reception channel.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// add is the common addition flow. When fileData != nil, the .torrent bytes
// are sent in the payload (base64) and the Agent prefers them over the magnet.
func (s *Service) add(ctx context.Context, magnet, name string, fileData []byte, sizeBytes int64, opts AddOptions) (Torrent, error) {
	hash, dn := parseMagnet(magnet)
	if name == "" {
		name = dn
	}

	// #11: without AllowDuplicate, refuses (409) if the hash already exists on any Node —
	// prevents duplicate content in the cluster without the operator knowing. Since #37 the Portal
	// no longer offers a 2nd copy: an identical file is "already in the cluster", and that's it.
	if !opts.AllowDuplicate && hash != "" {
		nodes, existing, nerr := s.existingNodesForHash(ctx, hash)
		if nerr != nil {
			return Torrent{}, nerr
		}
		if len(nodes) > 0 {
			return Torrent{}, &DuplicateError{Nodes: nodes, Existing: existing}
		}
	}
	// #37: another version of the same work (another resolution, codec or release) isn't an error,
	// it's the operator's decision — but an informed one: without AllowOtherVersion, it returns the
	// versions the cluster already has, for the Portal to show and ask.
	if !opts.AllowOtherVersion && name != "" {
		lib, lerr := s.Library(ctx)
		if lerr != nil {
			return Torrent{}, lerr
		}
		if vs := lib.OtherVersions(name, hash, 0); len(vs) > 0 {
			adding := newLibraryEntry(hash, name, sizeBytes, "").v
			return Torrent{}, &OtherVersionsError{Versions: vs, Adding: adding}
		}
	}

	// ADR-032: with known size (add via .torrent), the Scheduler excludes Nodes without
	// space; if none fits, it returns ErrNoNodeWithSpace. Magnet (sizeBytes=0) doesn't
	// filter by disk — only the Node with the most free space wins the tiebreaker.
	dec, err := s.sched.Select(ctx, sizeBytes, s.diskMinFree())
	if err != nil {
		// Exclusion due to failing storage is no longer silent (#180). Logs it HERE, and
		// not when the Node fails: this is the moment when exclusion costs something — a
		// torrent that didn't get in. An I/O hiccup with no one adding anything generates
		// no record at all, which is what prevents the false positive from reappearing through another path.
		var blocked *scheduler.NoEligibleNodeError
		if errors.As(err, &blocked) && s.activity != nil {
			_ = s.activity.Record(ctx, activity.Entry{
				Kind:        activity.KindNodeBlocked,
				TorrentName: name,
				Message:     blocked.Error(),
				Detail:      map[string]any{"nodes": blocked.BlockedByStorage, "reason": "faulty storage", "reason_code": "storage_failed"},
			})
		}
		return Torrent{}, err // includes ErrNoEligibleNode / ErrNoNodeWithSpace
	}

	// Torrent row + Scheduler decision + add_torrent command in a SINGLE
	// transaction (#117): either everything commits, or nothing exists. Without this, a failure after the
	// INSERT left a phantom QUEUED torrent — it never reaches the Engine and reconcile
	// (#19) doesn't remove it (null last_synced_at protects it on purpose).
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Torrent{}, fmt.Errorf("open add transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var t Torrent
	if err := tx.QueryRow(ctx,
		`INSERT INTO torrents (node_id, hash, name, status, magnet, size_bytes)
		 VALUES ($1, NULLIF($2,''), NULLIF($3,''), 'QUEUED', $4, NULLIF($5::bigint, 0))
		 ON CONFLICT (node_id, hash) WHERE deleted_at IS NULL DO UPDATE SET updated_at = now()
		 RETURNING id::text, uuid::text, node_id::text, hash, name, status`,
		dec.NodeID, hash, name, magnet, sizeBytes).Scan(&t.ID, &t.UUID, &t.NodeID, &t.Hash, &t.Name, &t.Status); err != nil {
		return Torrent{}, fmt.Errorf("registrar torrent: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO torrent_scheduler (torrent_id, selected_node, decision_score, decision_reason, candidates, algorithm_version)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, dec.NodeID, dec.Score, dec.Reason, dec.Candidates, dec.Algorithm); err != nil {
		return Torrent{}, fmt.Errorf("register scheduler decision: %w", err)
	}

	pl := map[string]string{"magnet": magnet}
	if len(fileData) > 0 {
		pl["torrent_file"] = base64.StdEncoding.EncodeToString(fileData)
	}
	payload, _ := json.Marshal(pl)
	if _, err := s.cmds.EnqueueTx(ctx, tx, dec.NodeID, "add_torrent", payload); err != nil {
		return Torrent{}, fmt.Errorf("enqueue command: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Torrent{}, fmt.Errorf("commit add: %w", err)
	}

	if len(fileData) > 0 {
		// Persists the .torrent in the CP (source of truth for migration/re-provisioning,
		// ADR-018). Post-commit and best-effort: doesn't fail the add if persistence fails
		// (recoverable later via engine export).
		_ = s.StoreTorrentFile(ctx, t.ID, fileData, "upload")
	}

	if s.events != nil {
		_ = s.events.Emit(ctx, "TORRENT_CREATED", "INFO", dec.NodeID,
			fmt.Sprintf("Torrent added to Node (score %d): %s", dec.Score, label(name, hash)),
			map[string]any{"hash": hash, "name": name, "hostname": s.hostname(ctx, dec.NodeID), "origin": "scheduled", "score": dec.Score})
	}
	if s.activity != nil {
		_ = s.activity.Record(ctx, activity.Entry{
			Kind: activity.KindScheduled, TorrentID: t.ID, TorrentName: label(name, hash), NodeID: dec.NodeID,
			Message: "Scheduled — Scheduler chose the Node",
			Detail:  map[string]any{"reason": dec.Reason, "reason_code": dec.ReasonCode, "score": dec.Score, "candidates": dec.Candidates},
		})
	}
	return t, nil
}

// existingNodesForHash returns the hostnames of the Nodes that already have the torrent (not
// removed) with that hash + the NAME of one of them — basis for the duplicate warning (#11).
func (s *Service) existingNodesForHash(ctx context.Context, hash string) (nodes []string, name string, err error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT COALESCE(NULLIF(n.hostname, ''), n.id::text), COALESCE(t.name, '')
		   FROM torrents t JOIN nodes n ON n.id = t.node_id
		  WHERE t.hash = $1 AND t.deleted_at IS NULL`,
		strings.ToLower(hash))
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	nodes = make([]string, 0)
	for rows.Next() {
		var h, nm string
		if err := rows.Scan(&h, &nm); err != nil {
			return nil, "", err
		}
		nodes = append(nodes, h)
		if name == "" && nm != "" {
			name = nm
		}
	}
	return nodes, name, rows.Err()
}

var reNameNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// NormalizeName reduces a name to lowercase alphanumerics — basis for the heuristic cross-check
// "I already have this torrent" with the tracker's catalog (#12; UNIT3D doesn't expose info_hash,
// so the match is by name — the hard guarantee is the duplicate guard on add, #11).
func NormalizeName(s string) string {
	return reNameNonAlnum.ReplaceAllString(strings.ToLower(s), "")
}

// NameInCluster reports if `name` (raw) matches any torrent in the cluster: exact match
// of the normalized name OR one being a prefix of the other (covers EXTENSION/suffix differences —
// e.g.: catalog "SamUploadAssistantSetup" vs cluster "SamUploadAssistantSetup.exe"). The
// length guard (≥10) and short difference guard (≤8) prevents false positives between distinct
// names that only share a prefix. Heuristic (UNIT3D doesn't expose info_hash).
func NameInCluster(name string, clusterNorm []string) bool {
	q := NormalizeName(name)
	if q == "" {
		return false
	}
	for _, c := range clusterNorm {
		if c == q {
			return true
		}
		short, long := q, c
		if len(long) < len(short) {
			short, long = long, short
		}
		if len(short) >= 10 && len(long)-len(short) <= 8 && strings.HasPrefix(long, short) {
			return true
		}
	}
	return false
}

// StoreTorrentFile persists (upserts) the .torrent bytes — canonical metadata for
// migration/re-provisioning (ADR-018). source: "upload" or "engine_export".
func (s *Service) StoreTorrentFile(ctx context.Context, torrentID string, data []byte, source string) error {
	if len(data) == 0 {
		return nil
	}
	if source == "" {
		source = "upload"
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO torrent_files (torrent_id, data, byte_size, source, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (torrent_id) DO UPDATE SET
		    data = EXCLUDED.data, byte_size = EXCLUDED.byte_size, source = EXCLUDED.source, updated_at = now()`,
		torrentID, data, len(data), source)
	return err
}

// HashesNeedingFile returns the hashes of a Node's torrents that already have metadata
// (known size) but whose .torrent is not yet persisted in the CP. The Agent uses
// this to export only what is missing from the engine (ADR-018).
func (s *Service) HashesNeedingFile(ctx context.Context, nodeID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.hash FROM torrents t
		   LEFT JOIN torrent_files tf ON tf.torrent_id = t.id
		  WHERE t.node_id = $1 AND t.deleted_at IS NULL
		    AND t.hash IS NOT NULL AND t.size_bytes > 0 AND tf.torrent_id IS NULL`,
		nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// StoreTorrentFileByHash persists the .torrent exported by the Agent, resolving the
// torrent by (node, hash).
func (s *Service) StoreTorrentFileByHash(ctx context.Context, nodeID, hash string, data []byte) error {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM torrents WHERE node_id = $1 AND hash = $2 AND deleted_at IS NULL`,
		nodeID, strings.ToLower(hash)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.StoreTorrentFile(ctx, id, data, "engine_export")
}

// TorrentMeta returns the persisted name and media_info of a torrent (used in the
// external registration on a UNIT3D tracker, ADR-020). ErrNotFound if the torrent does not exist.
func (s *Service) TorrentMeta(ctx context.Context, id string) (name, mediaInfo string, err error) {
	var n, mi *string
	err = s.pool.QueryRow(ctx,
		`SELECT name, media_info FROM torrents WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&n, &mi)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if n != nil {
		name = *n
	}
	if mi != nil {
		mediaInfo = *mi
	}
	return name, mediaInfo, nil
}

// PendingPublish is a torrent created for a tracker but not yet published on it
// (ADR-020, backlog #8) — the queue the Portal exposes in the Publish tab.
type PendingPublish struct {
	ID            string     `json:"id"`
	Name          *string    `json:"name,omitempty"`
	Status        string     `json:"status"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
	HasFile       bool       `json:"has_file"` // .torrent persisted (prerequisite to publish)
	TrackerID     string     `json:"tracker_id"`
	TrackerName   string     `json:"tracker_name"`
	TrackerSource string     `json:"tracker_source,omitempty"`
}

// PendingTracker lists torrents with a linked tracker that are not yet published —
// the "pending publication" ones (backlog #8). Only torrents from the operator owning the tracker.
func (s *Service) PendingTracker(ctx context.Context, userID string) ([]PendingPublish, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id::text, t.name, t.status, t.created_at,
		        (tf.torrent_id IS NOT NULL) AS has_file,
		        tr.id::text, tr.name, COALESCE(tr.source, '')
		   FROM torrents t
		   JOIN trackers tr ON tr.id = t.tracker_id AND tr.deleted_at IS NULL AND tr.user_id = $1
		   LEFT JOIN torrent_files tf ON tf.torrent_id = t.id
		  WHERE t.deleted_at IS NULL AND t.tracker_id IS NOT NULL AND t.published_tracker_at IS NULL
		  ORDER BY t.created_at DESC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PendingPublish, 0)
	for rows.Next() {
		var p PendingPublish
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.CreatedAt, &p.HasFile,
			&p.TrackerID, &p.TrackerName, &p.TrackerSource); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// MarkPublished marks a torrent as published on the tracker (backlog #8): records the
// timestamp and the download URL returned by the tracker's API. Best-effort at the caller.
func (s *Service) MarkPublished(ctx context.Context, torrentID, downloadURL string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE torrents SET published_tracker_at = now(), tracker_torrent_url = NULLIF($2, ''), updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL`,
		torrentID, downloadURL)
	return err
}

// CreateDownloadTicket validates the torrent and issues a one-time ticket (short TTL) for the
// browser to download the content (#1). The ticket goes in the download GET URL instead of the
// session token (which wouldn't be sent in a browser navigation/anchor).
func (s *Service) CreateDownloadTicket(ctx context.Context, id string) (string, error) {
	if _, _, _, err := s.lookup(ctx, id); err != nil {
		return "", err // ErrNotFound / ErrNoHash
	}
	tok, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.dlMu.Lock()
	for k, v := range s.dlTickets { // lazy cleanup of expired ones
		if now.After(v.expires) {
			delete(s.dlTickets, k)
		}
	}
	s.dlTickets[tok] = dlTicket{torrentID: id, expires: now.Add(5 * time.Minute)}
	s.dlMu.Unlock()
	return tok, nil
}

// OpenDownload consumes a ticket and opens the stream of the torrent content, pulling from the
// Node's transfer.Server (the CP does PROXY -> the Node is never exposed to the browser, #1).
// Authorizes the Node to serve that hash (reuses the migrate_serve command) and requests raw=1
// (raw file for single-file, tar for folder). Returns the body (the caller CLOSES) and the
// Content-Type/Content-Disposition that the Node defined.
func (s *Service) OpenDownload(ctx context.Context, ticket string) (body io.ReadCloser, contentType, disposition string, err error) {
	s.dlMu.Lock()
	t, ok := s.dlTickets[ticket]
	if ok {
		delete(s.dlTickets, ticket) // single use
	}
	s.dlMu.Unlock()
	if !ok || time.Now().After(t.expires) {
		return nil, "", "", ErrTicketInvalid
	}

	nodeID, hash, _, err := s.lookup(ctx, t.torrentID)
	if err != nil {
		return nil, "", "", err
	}
	var ip *string
	if qerr := s.pool.QueryRow(ctx,
		`SELECT last_ip FROM nodes WHERE id = $1 AND deleted_at IS NULL`, nodeID).Scan(&ip); qerr != nil {
		return nil, "", "", qerr
	}
	if ip == nil || *ip == "" {
		return nil, "", "", ErrNodeNotEligible
	}

	// Authorizes the Node to serve the hash under an ephemeral token (same command as migration).
	tok, err := randomToken()
	if err != nil {
		return nil, "", "", err
	}
	expiresAt := time.Now().Add(30 * time.Minute)
	pl, _ := json.Marshal(map[string]any{"hash": hash, "token": tok, "expires_at": expiresAt.UTC().Format(time.RFC3339)})
	cmdID, err := s.cmds.Enqueue(ctx, nodeID, "migrate_serve", pl)
	if err != nil {
		return nil, "", "", fmt.Errorf("authorize download on Node: %w", err)
	}
	if err := s.waitCommandDone(ctx, cmdID, 60*time.Second); err != nil {
		return nil, "", "", fmt.Errorf("node did not authorize download: %w", err)
	}

	u := fmt.Sprintf("http://%s/transfer?token=%s&raw=1",
		net.JoinHostPort(*ip, strconv.Itoa(s.pubPort)), url.QueryEscape(tok))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", "", err
	}
	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("open Node stream: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, "", "", fmt.Errorf("the Node rejected the download (status %d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp.Body, resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"), nil
}

// GetTorrentFile returns the bytes of the persisted .torrent (ErrNotFound if none).
func (s *Service) GetTorrentFile(ctx context.Context, torrentID string) ([]byte, error) {
	var data []byte
	err := s.pool.QueryRow(ctx,
		`SELECT data FROM torrent_files WHERE torrent_id = $1`, torrentID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return data, err
}

// HasTorrentFile reports whether the torrent's .torrent is persisted in the CP.
func (s *Service) HasTorrentFile(ctx context.Context, torrentID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM torrent_files WHERE torrent_id = $1)`, torrentID).Scan(&ok)
	return ok, err
}

// buildMagnet builds a magnet from the metadata of a .torrent.
func buildMagnet(m torrentfile.Meta) string {
	q := "magnet:?xt=urn:btih:" + m.InfoHash
	if m.Name != "" {
		q += "&dn=" + url.QueryEscape(m.Name)
	}
	for _, tr := range m.Trackers {
		q += "&tr=" + url.QueryEscape(tr)
	}
	return q
}

// Report is the state of a torrent reported by the Agent (already mapped to the
// official SeedGrid state by the Engine adapter).
//
// NumSeeds/NumLeechs are the SWARM (tracker scrape); NumSeedsConnected/NumLeechsConnected
// are the peers actually CONNECTED (refinement from ADR-021). Connected ones are nil when the
// Agent predates the field — the columns become NULL and consumers fall back to the swarm.
type Report struct {
	Hash               string   `json:"hash"`
	Name               string   `json:"name"`
	Status             string   `json:"status"`
	Progress           *float64 `json:"progress,omitempty"`
	NumSeeds           *int     `json:"num_seeds,omitempty"`
	NumLeechs          *int     `json:"num_leechs,omitempty"`
	NumSeedsConnected  *int     `json:"num_seeds_connected,omitempty"`
	NumLeechsConnected *int     `json:"num_leechs_connected,omitempty"`
	DlSpeed            *int64   `json:"dl_speed,omitempty"`
	UpSpeed            *int64   `json:"up_speed,omitempty"`
	Size               *int64   `json:"size,omitempty"`
	Ratio              *float64 `json:"ratio,omitempty"`
	ETA                *int64   `json:"eta,omitempty"`
	Downloaded         *int64   `json:"downloaded,omitempty"`
	Uploaded           *int64   `json:"uploaded,omitempty"`
	// Tracker verdict (#160): announce status and the raw message. nil = Agent that
	// does not report — unknown, never read as "dead".
	TrackerStatus  *int    `json:"tracker_status,omitempty"`
	TrackerMessage *string `json:"tracker_message,omitempty"`
}

// SyncStates reconciles the state of a Node's torrents from what the Agent
// reported from the Engine. It's an UPSERT: known torrents are updated and torrents
// that exist in the Engine but the CP doesn't know yet are **adopted** (inserted as
// Node's torrents) — the CP reflects the Engine's reality, covering torrents
// added outside the CP flow or reprovisioned from data on disk.
//
// Processes the inventory IN BATCH (#113): there were up to 4 round-trips per torrent every
// 15s sync (name inheritance, previous status, upsert, sample) — with hundreds of
// torrents, thousands of statements/min. Now there are at most 4 queries per sync:
// previous status (batch), name inheritance (batch), upsert via unnest and samples via
// unnest — the latter DECIMATED by the minimum interval (SetSampleInterval).
func (s *Service) SyncStates(ctx context.Context, nodeID string, reports []Report) error {
	// Defensive deduplication by hash (the same hash twice in an INSERT ... ON CONFLICT
	// would fail with "cannot affect row a second time"); the Engine doesn't repeat hashes.
	idx := make(map[string]int, len(reports))
	rs := make([]Report, 0, len(reports))
	hashes := make([]string, 0, len(reports))
	for _, r := range reports {
		if r.Hash == "" {
			continue
		}
		h := strings.ToLower(r.Hash)
		r.Hash = h
		if k, ok := idx[h]; ok {
			rs[k] = r
			continue
		}
		idx[h] = len(rs)
		rs = append(rs, r)
		hashes = append(hashes, h)
	}
	if len(rs) == 0 {
		return nil // without inventory there's no upsert nor reconcile (empty report isn't "everything disappeared")
	}

	// Previous status by hash (one query for the batch) — only for the ADR-022 trail. Missing
	// hash = torrent still unknown in this Node -> does not record transition (avoids
	// "started seeding" spam on adoption/1st sight after restart).
	oldStatus := map[string]string{}
	if s.activity != nil {
		rows, err := s.pool.Query(ctx,
			`SELECT hash, status FROM torrents
			  WHERE node_id = $1 AND hash = ANY($2) AND deleted_at IS NULL`,
			nodeID, hashes)
		if err != nil {
			return fmt.Errorf("previous batch status: %w", err)
		}
		for rows.Next() {
			var h, st string
			if err := rows.Scan(&h, &st); err != nil {
				rows.Close()
				return err
			}
			oldStatus[h] = st
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}

	// Name inheritance (one query for the batch): report without a name inherits from another row of
	// the same hash (e.g.: adoption of a known torrent in another Node) before falling back to the
	// hash as a placeholder — avoids hash-names in the Portal.
	needName := make([]string, 0)
	for _, r := range rs {
		if r.Name == "" {
			needName = append(needName, r.Hash)
		}
	}
	inherited := map[string]string{}
	if len(needName) > 0 {
		rows, err := s.pool.Query(ctx,
			`SELECT DISTINCT ON (hash) hash, name FROM torrents
			  WHERE hash = ANY($1) AND deleted_at IS NULL AND name <> hash
			  ORDER BY hash, created_at`,
			needName)
		if err != nil {
			return fmt.Errorf("inherit batch names: %w", err)
		}
		for rows.Next() {
			var h, nm string
			if err := rows.Scan(&h, &nm); err != nil {
				rows.Close()
				return err
			}
			inherited[h] = nm
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}

	// Batch columns (parallel arrays for unnest; nil pointer becomes NULL).
	n := len(rs)
	names := make([]string, n)
	statuses := make([]string, n)
	progress := make([]*float64, n)
	numSeeds := make([]*int32, n)
	numLeechs := make([]*int32, n)
	seedsConn := make([]*int32, n)
	leechsConn := make([]*int32, n)
	dlSpeed := make([]*int64, n)
	upSpeed := make([]*int64, n)
	sizes := make([]*int64, n)
	ratios := make([]*float64, n)
	etas := make([]*int64, n)
	downloaded := make([]*int64, n)
	uploaded := make([]*int64, n)
	trkStatus := make([]*int32, n)
	trkMsg := make([]*string, n)
	i32 := func(v *int) *int32 {
		if v == nil {
			return nil
		}
		x := int32(*v) //nolint:gosec // G115: peer count, never near MaxInt32
		return &x
	}
	for i, r := range rs {
		name := r.Name
		if name == "" {
			if inh := inherited[r.Hash]; inh != "" {
				name = inh
			} else {
				name = r.Hash
			}
		}
		names[i] = name
		statuses[i] = r.Status
		progress[i] = r.Progress
		numSeeds[i] = i32(r.NumSeeds)
		numLeechs[i] = i32(r.NumLeechs)
		seedsConn[i] = i32(r.NumSeedsConnected)
		leechsConn[i] = i32(r.NumLeechsConnected)
		dlSpeed[i] = r.DlSpeed
		upSpeed[i] = r.UpSpeed
		sizes[i] = r.Size
		ratios[i] = r.Ratio
		etas[i] = r.ETA
		downloaded[i] = r.Downloaded
		uploaded[i] = r.Uploaded
		trkStatus[i] = i32(r.TrackerStatus)
		trkMsg[i] = r.TrackerMessage
	}

	// Batch upsert. Cumulative totals (#14): accumulate the positive delta of the session
	// between syncs; reset session (new < old, e.g.: re-add) adds the new integer —
	// the total never regresses. torrents.* in the SET clause is the OLD value.
	type row struct {
		id        string
		createdAt time.Time
	}
	byHash := make(map[string]row, n)
	rows, err := s.pool.Query(ctx,
		`INSERT INTO torrents (node_id, hash, name, status, progress,
		        num_seeds, num_leechers, dl_speed, up_speed, size_bytes, ratio, eta_seconds,
		        downloaded_bytes, uploaded_bytes, num_seeds_connected, num_leechers_connected,
		        downloaded_total, uploaded_total, tracker_status, tracker_message, last_synced_at)
		 SELECT $1, u.hash, u.name, u.status, u.progress,
		        u.num_seeds, u.num_leechers, u.dl_speed, u.up_speed, u.size_bytes, u.ratio, u.eta_seconds,
		        u.downloaded, u.uploaded, u.seeds_conn, u.leechs_conn,
		        COALESCE(u.downloaded, 0), COALESCE(u.uploaded, 0), u.trk_status, u.trk_msg, now()
		   FROM unnest($2::text[], $3::text[], $4::text[], $5::float8[],
		               $6::int[], $7::int[], $8::bigint[], $9::bigint[], $10::bigint[],
		               $11::float8[], $12::bigint[], $13::bigint[], $14::bigint[],
		               $15::int[], $16::int[], $17::int[], $18::text[])
		        AS u(hash, name, status, progress,
		             num_seeds, num_leechers, dl_speed, up_speed, size_bytes,
		             ratio, eta_seconds, downloaded, uploaded, seeds_conn, leechs_conn,
		             trk_status, trk_msg)
		 ON CONFLICT (node_id, hash) WHERE deleted_at IS NULL DO UPDATE SET
		    status = EXCLUDED.status, progress = EXCLUDED.progress,
		    num_seeds = EXCLUDED.num_seeds, num_leechers = EXCLUDED.num_leechers,
		    num_seeds_connected = EXCLUDED.num_seeds_connected,
		    num_leechers_connected = EXCLUDED.num_leechers_connected,
		    dl_speed = EXCLUDED.dl_speed, up_speed = EXCLUDED.up_speed,
		    size_bytes = EXCLUDED.size_bytes, ratio = EXCLUDED.ratio, eta_seconds = EXCLUDED.eta_seconds,
		    downloaded_total = torrents.downloaded_total + CASE
		        WHEN COALESCE(EXCLUDED.downloaded_bytes, 0) >= COALESCE(torrents.downloaded_bytes, 0)
		        THEN COALESCE(EXCLUDED.downloaded_bytes, 0) - COALESCE(torrents.downloaded_bytes, 0)
		        ELSE COALESCE(EXCLUDED.downloaded_bytes, 0) END,
		    uploaded_total = torrents.uploaded_total + CASE
		        WHEN COALESCE(EXCLUDED.uploaded_bytes, 0) >= COALESCE(torrents.uploaded_bytes, 0)
		        THEN COALESCE(EXCLUDED.uploaded_bytes, 0) - COALESCE(torrents.uploaded_bytes, 0)
		        ELSE COALESCE(EXCLUDED.uploaded_bytes, 0) END,
		    downloaded_bytes = EXCLUDED.downloaded_bytes, uploaded_bytes = EXCLUDED.uploaded_bytes,
		    name = CASE WHEN torrents.name = torrents.hash THEN EXCLUDED.name
		                ELSE COALESCE(torrents.name, EXCLUDED.name) END,
		    -- Tracker verdict (#160): Agent NULL = unknown, and unknown
		    -- does NOT erase the last known verdict. Without COALESCE, an old Agent (or
		    -- the window before the first scan) would zero out the info on every sync.
		    tracker_status = COALESCE(EXCLUDED.tracker_status, torrents.tracker_status),
		    tracker_message = COALESCE(EXCLUDED.tracker_message, torrents.tracker_message),
		    last_synced_at = now(),
		    updated_at = now()
		 RETURNING hash, id::text, created_at`,
		nodeID, hashes, names, statuses, progress,
		numSeeds, numLeechs, dlSpeed, upSpeed, sizes, ratios, etas,
		downloaded, uploaded, seedsConn, leechsConn, trkStatus, trkMsg)
	if err != nil {
		return fmt.Errorf("reconcile batch of %d torrents: %w", n, err)
	}
	for rows.Next() {
		var h string
		var r row
		if err := rows.Scan(&h, &r.id, &r.createdAt); err != nil {
			rows.Close()
			return err
		}
		byHash[h] = r
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Time series samples, DECIMATED (#113): writes the entire batch at most every
	// sampleEvery per Node — the Portal's series (hour window) doesn't need
	// 15s resolution, and this INSERT was the biggest source of database growth.
	if s.shouldSample(nodeID) {
		ids := make([]string, n)
		for i, r := range rs {
			ids[i] = byHash[r.Hash].id
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO torrent_stat_samples (torrent_id, downloaded_bytes, uploaded_bytes, dl_speed, up_speed)
			 SELECT * FROM unnest($1::uuid[], $2::bigint[], $3::bigint[], $4::bigint[], $5::bigint[])`,
			ids, downloaded, uploaded, dlSpeed, upSpeed); err != nil {
			return fmt.Errorf("write batch samples: %w", err)
		}
	}

	// Observed transitions -> activity trail (ADR-022): only when the torrent was already
	// known (oldStatus populated) and there was a status change.
	if s.activity != nil {
		hostname := ""
		for i, r := range rs {
			old := oldStatus[r.Hash]
			if old == "" || old == r.Status {
				continue
			}
			if hostname == "" {
				hostname = s.hostname(ctx, nodeID)
			}
			meta := byHash[r.Hash]
			switch {
			case r.Status == "SEEDING":
				var sizeBytes int64
				if r.Size != nil {
					sizeBytes = *r.Size
				}
				_ = s.activity.Record(ctx, activity.Entry{
					Kind: activity.KindSeeding, TorrentID: meta.id, TorrentName: label(names[i], r.Hash),
					NodeID: nodeID, Message: "Completed download and started seeding",
					Detail: map[string]any{"size_bytes": sizeBytes, "added_at": meta.createdAt, "hostname": hostname},
				})
			case r.Status == "DOWNLOADING" && (old == "QUEUED" || old == "CHECKING"):
				_ = s.activity.Record(ctx, activity.Entry{
					Kind: activity.KindDownloading, TorrentID: meta.id, TorrentName: label(names[i], r.Hash),
					NodeID: nodeID, Message: "Started downloading",
					Detail: map[string]any{"hostname": hostname},
				})
			}
		}
	}

	// Deletion reconciliation (#19): torrent missing from the Node's inventory ceased
	// to exist in the Engine -> soft-delete. Only ALREADY seen rows (non-null last_synced_at
	// protects newly added ones still missing from the Engine).
	if err := s.reconcileMissing(ctx, nodeID, hashes); err != nil {
		return fmt.Errorf("reconcile Node %s deletions: %w", nodeID, err)
	}
	return nil
}

// shouldSample decides if THIS sync records time series samples (#113): at most
// one batch per Node every sampleEvery. Without injection (or interval <= 0), it samples
// always — previous behavior, and that of the tests.
func (s *Service) shouldSample(nodeID string) bool {
	if s.sampleEvery == nil {
		return true
	}
	every := s.sampleEvery()
	if every <= 0 {
		return true
	}
	now := time.Now()
	s.sampleMu.Lock()
	defer s.sampleMu.Unlock()
	if last, ok := s.lastSample[nodeID]; ok && now.Sub(last) < every {
		return false
	}
	s.lastSample[nodeID] = now
	return true
}

// reconcileMissing soft-deletes the Node's alive rows whose hash is not in seen (the
// current Engine inventory). Ignores torrents with non-terminal migration (the migration
// moves the origin->destination row and takes care of the origin itself). Emits TORRENT_REMOVED
// per reconciled torrent for the Portal's audit.
func (s *Service) reconcileMissing(ctx context.Context, nodeID string, seen []string) error {
	rows, err := s.pool.Query(ctx,
		`UPDATE torrents t SET deleted_at = now(), updated_at = now()
		 WHERE t.node_id = $1
		   AND t.deleted_at IS NULL
		   AND t.last_synced_at IS NOT NULL
		   AND t.hash <> ALL($2::text[])
		   AND NOT EXISTS (
		       SELECT 1 FROM torrent_migrations m
		       WHERE m.torrent_id = t.id AND m.status NOT IN ('COMPLETED', 'FAILED')
		   )
		 RETURNING COALESCE(hash, ''), COALESCE(name, '')`,
		nodeID, seen)
	if err != nil {
		return err
	}
	type gone struct{ hash, name string }
	var removed []gone
	for rows.Next() {
		var g gone
		if err := rows.Scan(&g.hash, &g.name); err != nil {
			rows.Close()
			return err
		}
		removed = append(removed, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if s.events != nil {
		hostname := s.hostname(ctx, nodeID)
		for _, g := range removed {
			_ = s.events.Emit(ctx, "TORRENT_REMOVED", "INFO", nodeID,
				fmt.Sprintf("Torrent removed from Node (missing from inventory): %s", label(g.name, g.hash)),
				map[string]any{"hash": g.hash, "name": g.name, "reason": "inventory_reconcile", "actor": "SeedGrid", "hostname": hostname})
		}
	}
	return nil
}

// StatSample is a time series sample of a torrent (t in Unix seconds).
type StatSample struct {
	T          float64 `json:"t"`
	Downloaded int64   `json:"downloaded_bytes"`
	Uploaded   int64   `json:"uploaded_bytes"`
	DlSpeed    int64   `json:"dl_speed"`
	UpSpeed    int64   `json:"up_speed"`
}

// Samples returns the samples of a torrent in the [now-rng, now] window.
// Returns ErrNotFound if the torrent does not exist (or is removed).
func (s *Service) Samples(ctx context.Context, id string, rng time.Duration) ([]StatSample, error) {
	if rng <= 0 {
		rng = 6 * time.Hour
	}
	var ok bool
	if err := s.pool.QueryRow(ctx,
		`SELECT true FROM torrents WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&ok); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT extract(epoch from sampled_at),
		        COALESCE(downloaded_bytes, 0), COALESCE(uploaded_bytes, 0),
		        COALESCE(dl_speed, 0), COALESCE(up_speed, 0)
		   FROM torrent_stat_samples
		  WHERE torrent_id = $1 AND sampled_at >= now() - make_interval(secs => $2)
		  ORDER BY sampled_at`,
		id, rng.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]StatSample, 0)
	for rows.Next() {
		var smp StatSample
		if err := rows.Scan(&smp.T, &smp.Downloaded, &smp.Uploaded, &smp.DlSpeed, &smp.UpSpeed); err != nil {
			return nil, err
		}
		out = append(out, smp)
	}
	return out, rows.Err()
}

// ListMeta summarizes the live state of torrents — (total, max(updated_at)) — basis of the
// conditional ETag for GET /torrents (#115): any upsert/sync/removal changes the pair.
func (s *Service) ListMeta(ctx context.Context) (int, time.Time, error) {
	var total int
	var maxUpdated time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT count(*), COALESCE(max(updated_at), 'epoch'::timestamptz)
		   FROM torrents WHERE deleted_at IS NULL`).Scan(&total, &maxUpdated)
	return total, maxUpdated, err
}

// List returns the non-removed torrents.
func (s *Service) List(ctx context.Context) ([]Torrent, error) {
	return s.ListPage(ctx, 0, 0)
}

// ListPage is List with optional pagination (#115): limit <= 0 returns everything (previous
// contract); offset only applies with limit. Stable sorting (created_at DESC, id).
func (s *Service) ListPage(ctx context.Context, limit, offset int) ([]Torrent, error) {
	q := `SELECT id::text, uuid::text, node_id::text, hash, name, status, progress, created_at,
	        num_seeds, num_leechers, num_seeds_connected, num_leechers_connected,
	        dl_speed, up_speed, size_bytes, ratio, eta_seconds,
	        downloaded_bytes, uploaded_bytes, downloaded_total, uploaded_total, prioritized_at
	   FROM torrents WHERE deleted_at IS NULL ORDER BY created_at DESC, id`
	args := []any{}
	if limit > 0 {
		q += ` LIMIT $1 OFFSET $2`
		args = append(args, limit, offset)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Torrent, 0)
	for rows.Next() {
		var t Torrent
		if err := rows.Scan(&t.ID, &t.UUID, &t.NodeID, &t.Hash, &t.Name, &t.Status, &t.Progress, &t.CreatedAt,
			&t.NumSeeds, &t.NumLeechs, &t.NumSeedsConnected, &t.NumLeechsConnected,
			&t.DlSpeed, &t.UpSpeed, &t.SizeBytes, &t.Ratio, &t.ETASecs,
			&t.Downloaded, &t.Uploaded, &t.DownloadedTotal, &t.UploadedTotal, &t.PrioritizedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// lookup resolves the Node, hash and name of a torrent (to operate on the Engine and
// register Events).
func (s *Service) lookup(ctx context.Context, id string) (nodeID, hash, name string, err error) {
	var h, n *string
	err = s.pool.QueryRow(ctx,
		`SELECT node_id::text, hash, name FROM torrents WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&nodeID, &h, &n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", err
	}
	if h == nil || *h == "" {
		return "", "", "", ErrNoHash
	}
	if n != nil {
		name = *n
	}
	return nodeID, *h, name, nil
}

// GenerateScreenshots (ADR-026/ADR-040) extracts frames from the torrent media file via
// the Agent (command queue) and returns the BBCode ready to paste in the upload description
// on the tracker, using the host chain (tracker → imgbb) when trackerID is provided.
// Without trackerID, only imgbb (old behavior). imgbbAPIKey comes from the catalog (ADR-029/030);
// count comes from the tunables catalog (publish.screenshot_count).
func (s *Service) GenerateScreenshots(ctx context.Context, id, userID, trackerID, imgbbAPIKey string, count int) (ScreenshotResult, error) {
	nodeID, hash, name, err := s.lookup(ctx, id)
	if err != nil {
		return ScreenshotResult{}, err
	}
	frames, err := screenshot.RequestFrames(ctx, s.cmds, nodeID, hash, count)
	if err != nil {
		return ScreenshotResult{}, fmt.Errorf("extract screenshots on Node: %w", err)
	}
	return s.uploadChain(ctx, userID, trackerID, imgbbAPIKey, id, name, frames)
}

// RequestScreenshots queues screenshot extraction on the torrent Node and returns the
// command id, WITHOUT blocking (F1: the card with the step bar follows the stage
// "Extracting on Node" through the command cycle). The render (chain + BBCode) is a 2nd call.
func (s *Service) RequestScreenshots(ctx context.Context, id string, count int) (string, error) {
	nodeID, hash, _, err := s.lookup(ctx, id)
	if err != nil {
		return "", err
	}
	return screenshot.EnqueueExtract(ctx, s.cmds, nodeID, hash, count)
}

// RenderScreenshots reads the frames from a completed extract_screenshots command, uploads to
// the host chain (ADR-040) and returns the BBCode. Called by the Portal after seeing the
// command in DONE. Without trackerID, only imgbb (old behavior). Tracker ownership
// is checked by the handler before reaching here.
func (s *Service) RenderScreenshots(ctx context.Context, cmdID, userID, trackerID, imgbbAPIKey string) (ScreenshotResult, error) {
	frames, err := screenshot.FramesFromCommand(ctx, s.cmds, cmdID)
	if err != nil {
		return ScreenshotResult{}, fmt.Errorf("read extracted frames: %w", err)
	}
	torrentID, torrentName := s.torrentForCommand(ctx, cmdID)
	return s.uploadChain(ctx, userID, trackerID, imgbbAPIKey, torrentID, torrentName, frames)
}

// torrentForCommand finds the torrent for an extract_screenshots command, so the Feed
// entry includes the torrent too in the step-based flow. The command only stores the Node and hash
// (payload `{hash, screenshot_count}`), so the torrent comes from the pair (node_id, hash).
// Best-effort: any failure returns empty — the entry goes without torrent, the upload proceeds.
func (s *Service) torrentForCommand(ctx context.Context, cmdID string) (id, name string) {
	var n *string
	err := s.pool.QueryRow(ctx, `
		SELECT t.id::text, t.name
		  FROM commands c
		  JOIN torrents t ON t.node_id = c.node_id AND t.hash = c.payload->>'hash' AND t.deleted_at IS NULL
		 WHERE c.id = $1 AND c.type = 'extract_screenshots'
		 LIMIT 1`, cmdID).Scan(&id, &n)
	if err != nil {
		return "", ""
	}
	if n != nil {
		name = *n
	}
	return id, name
}

// uploadChain implements the screenshot upload chain (ADR-040, #193): host of the
// tracker → imgbb, with a 25s budget. Each failure is recorded in the Feed with the sanitized
// reason. It returns the BBCode, the winning host and the failure list; when all fail,
// the error is *imagehost.ChainError (with the failures also in ScreenshotResult).
func (s *Service) uploadChain(ctx context.Context, userID, trackerID, imgbbAPIKey, torrentID, torrentName string, frames [][]byte) (ScreenshotResult, error) {
	candidates, err := s.screenshotCandidates(ctx, userID, trackerID, imgbbAPIKey)
	if err != nil {
		return ScreenshotResult{}, err
	}
	// No host to try: the same error as before #193 (without a tracker with its own host,
	// the chain is only imgbb, and imgbb without a key always returned this).
	if len(candidates) == 0 {
		return ScreenshotResult{}, errors.New("upload screenshots: imgbb api_key not configured")
	}

	// 25s budget (ADR-040 §4): below the 30s WriteTimeout and the 60s proxy.
	imgs, host, failures, err := imagehost.Chain(ctx, 25*time.Second, candidates, frames)

	if s.activity != nil {
		for _, e := range screenshotFailureEntries(candidates, failures, trackerID, torrentID, torrentName) {
			_ = s.activity.Record(ctx, e)
		}
	}

	if err != nil {
		return ScreenshotResult{Failures: failures}, err
	}
	shots := make([]screenshot.Shot, len(imgs))
	for i, im := range imgs {
		shots[i] = screenshot.Shot{Full: im.Full, Thumb: im.Thumb}
	}
	return ScreenshotResult{BBCode: screenshot.BuildBBCode(shots), Host: host, Failures: failures}, nil
}

// screenshotCandidates builds the chain in ADR-040 order: the tracker host (when the
// trackerID comes, the profile has the section and there is a key) and, last, imgbb (when there is a
// global key). Empty list = nothing to try.
func (s *Service) screenshotCandidates(ctx context.Context, userID, trackerID, imgbbAPIKey string) ([]imagehost.Candidate, error) {
	var candidates []imagehost.Candidate
	if trackerID != "" {
		c, err := s.trackerCandidate(ctx, userID, trackerID)
		if err != nil {
			return nil, err
		}
		if c != nil {
			candidates = append(candidates, *c)
		}
	}
	if strings.TrimSpace(imgbbAPIKey) != "" {
		candidates = append(candidates, imagehost.Candidate{
			Host: "imgbb",
			Upload: func(ctx context.Context, frames [][]byte) ([]imagehost.Image, error) {
				imgs, err := imgbb.Upload(ctx, imgbbAPIKey, frames)
				if err != nil {
					return nil, imgbbChainError(err)
				}
				out := make([]imagehost.Image, len(imgs))
				for i, im := range imgs {
					out[i] = imagehost.Image{Full: im.Full, Thumb: im.Thumb}
				}
				return out, nil
			},
		})
	}
	return candidates, nil
}

// imgbbChainError translates the imgbb client error into the chain vocabulary, so the
// reason in the Feed comes out as "HTTP <status>" or "response without URL" instead of "connection failure".
// The original error remains wrapped (its text never becomes the reason).
func imgbbChainError(err error) error {
	var se *imgbb.StatusError
	switch {
	case errors.As(err, &se):
		return fmt.Errorf("%w: %w", &imagehost.StatusError{Status: se.Status}, err)
	case errors.Is(err, imgbb.ErrNoURL):
		return fmt.Errorf("%w: %w", imagehost.ErrNoURL, err)
	}
	return err
}

// trackerCandidate builds the tracker host candidate. nil = the tracker is not included (without
// providers, without the image_host section or without a key). The error from providers (including the "not
// found" from each one) comes back as it came — the handler already checked ownership beforehand.
func (s *Service) trackerCandidate(ctx context.Context, userID, trackerID string) (*imagehost.Candidate, error) {
	if s.profiles == nil || s.imageHostKey == nil {
		return nil, nil
	}
	spec, err := s.profiles.ImageHostSpec(ctx, userID, trackerID)
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, nil
	}
	key, registeredHost, err := s.imageHostKey.ImageHostKeyForChain(ctx, userID, trackerID)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, nil
	}
	host := spec.Host()
	// Strict binding (ADR-040): the key only goes to the host recorded with it. A key without
	// a recorded host does not go either — there is no way to know where the operator saw it going.
	if host == "" || registeredHost != host {
		return &imagehost.Candidate{Host: host, Skip: imagehost.ReasonKeyMismatch}, nil
	}
	sp := *spec
	return &imagehost.Candidate{
		Host: host,
		Upload: func(ctx context.Context, frames [][]byte) ([]imagehost.Image, error) {
			return imagehost.Upload(ctx, sp, key, frames)
		},
	}, nil
}

// screenshotFailureEntries builds one Feed entry per chain failure. The Chain returns
// exactly one failure per candidate tried or skipped, in order, until the winner — so
// failure i is from candidate i, and `next` is the following candidate (whether it failed or won),
// empty when there are no more.
func screenshotFailureEntries(candidates []imagehost.Candidate, failures []imagehost.Failure, trackerID, torrentID, torrentName string) []activity.Entry {
	out := make([]activity.Entry, 0, len(failures))
	for i, f := range failures {
		var next string
		if i+1 < len(candidates) {
			next = candidates[i+1].Host
		}
		out = append(out, activity.Entry{
			Kind:        activity.KindScreenshotHostFailed,
			TorrentID:   torrentID,
			TorrentName: torrentName,
			Message:     fmt.Sprintf("Image host failed: %s (%s)", f.Host, f.Reason),
			Detail: map[string]any{
				"tracker_id":  trackerID,
				"host":        f.Host,
				"reason":      f.Reason,
				"reason_code": imagehost.ReasonCode(f.Reason),
				"next":        next,
			},
		})
	}
	return out
}

// Command queues a simple action by hash on the torrent Node (pause/resume/recheck),
// registers the corresponding Event and returns the command id for tracking.
func (s *Service) Command(ctx context.Context, id, action string) (string, error) {
	spec, ok := map[string]struct{ cmd, code, msg string }{
		"pause":   {"pause_torrent", "TORRENT_PAUSED", "Torrent paused"},
		"resume":  {"resume_torrent", "TORRENT_RESUMED", "Torrent resumed"},
		"recheck": {"recheck_torrent", "TORRENT_RECHECK", "Torrent recheck requested"},
	}[action]
	if !ok {
		return "", fmt.Errorf("invalid action: %q", action)
	}
	nodeID, hash, name, err := s.lookup(ctx, id)
	if err != nil {
		return "", err
	}
	if migID, err := s.inMigration(ctx, id, nodeID, hash); err != nil {
		return "", err
	} else if migID != "" {
		return "", &MigrationInFlightError{MigrationID: migID}
	}
	payload, _ := json.Marshal(map[string]string{"hash": hash})
	cmdID, err := s.cmds.Enqueue(ctx, nodeID, spec.cmd, payload)
	if err != nil {
		return "", err
	}
	// ADR-024 (manual resume wins): when resuming a torrent that was in admission-hold,
	// the operator takes control of that torrent — clears admission_paused_at so the
	// admission controller does not re-pause it (fixing the asymmetry with pause, which
	// was already respected). The torrent starts counting in L and the controller rebalances the
	// rest. Best-effort: does not fail the resume if the UPDATE fails.
	if action == "resume" {
		_, _ = s.pool.Exec(ctx,
			`UPDATE torrents SET admission_paused_at = NULL, updated_at = now()
			  WHERE id = $1 AND admission_paused_at IS NOT NULL`, id)
	}
	if s.events != nil {
		_ = s.events.Emit(ctx, spec.code, "INFO", nodeID,
			fmt.Sprintf("%s: %s", spec.msg, label(name, hash)),
			map[string]any{"hash": hash, "name": name, "hostname": s.hostname(ctx, nodeID)})
	}
	return cmdID, nil
}

// Remove queues the removal in the Engine (with the data) and marks the torrent as
// removed in the Cluster State (soft delete). It returns the command id. actor identifies
// who requested the removal (operator username) — it goes to the external notification
// (ADR-028), distinguishing manual removal from automatic reconciliation.
func (s *Service) Remove(ctx context.Context, id, actor string) (string, error) {
	nodeID, hash, name, err := s.lookup(ctx, id)
	if err != nil {
		return "", err
	}
	if migID, err := s.inMigration(ctx, id, nodeID, hash); err != nil {
		return "", err
	} else if migID != "" {
		return "", &MigrationInFlightError{MigrationID: migID}
	}
	// Soft-delete + remove_torrent command in a SINGLE transaction (#117), with the command
	// queued LAST (same pattern as complete() from the post-incident 27k migration):
	// failure at any step → rollback, without an orphan command or a live row with removal
	// already queued in the Engine.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("open removal transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`UPDATE torrents SET deleted_at = now(), updated_at = now() WHERE id = $1`, id); err != nil {
		return "", fmt.Errorf("mark torrent as removed: %w", err)
	}
	payload, _ := json.Marshal(map[string]any{"hash": hash, "delete_files": true})
	cmdID, err := s.cmds.EnqueueTx(ctx, tx, nodeID, "remove_torrent", payload)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit removal: %w", err)
	}
	if s.events != nil {
		_ = s.events.Emit(ctx, "TORRENT_REMOVED", "INFO", nodeID,
			fmt.Sprintf("Torrent removed from Node: %s", label(name, hash)),
			map[string]any{"hash": hash, "name": name, "actor": actor, "hostname": s.hostname(ctx, nodeID), "reason": "operator"})
	}
	return cmdID, nil
}

// SetPriority sets (on=true) or unsets (on=false) manual prioritization of a torrent
// in the admission queue (F3): prioritized_at=now() makes it jump the promotion queue and, in persistent
// mode (admission.priority_persistent), protects it from dead/slow rotation until
// downloaded. It does NOT queue a command for the Node — it is only Control Plane state that the admission
// controller consumes on the next pass. ErrNotFound if the torrent does not exist or was removed.
func (s *Service) SetPriority(ctx context.Context, id string, on bool) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE torrents SET prioritized_at = CASE WHEN $2 THEN now() ELSE NULL END, updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL`, id, on)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Label returns a human-readable label for the torrent (name, or short hash) for
// Audit/UI descriptions. "" when the torrent is not found.
func (s *Service) Label(ctx context.Context, id string) string {
	_, hash, name, err := s.lookup(ctx, id)
	if err != nil {
		return ""
	}
	return label(name, hash)
}

// label returns a human-readable label for the torrent for Event messages (backlog
// #7 — say WHICH torrent): the name, or the short hash when there is no name.
func label(name, hash string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	if len(hash) >= 8 {
		return hash[:8]
	}
	return hash
}

// parseMagnet extracts the infohash (btih) and name (dn) from a magnet link.
func parseMagnet(magnet string) (hash, name string) {
	u, err := url.Parse(magnet)
	if err != nil {
		return "", ""
	}
	q := u.Query()
	for _, xt := range q["xt"] {
		if strings.HasPrefix(xt, "urn:btih:") {
			hash = strings.ToLower(strings.TrimPrefix(xt, "urn:btih:"))
			break
		}
	}
	return hash, q.Get("dn")
}
