// Package transfer implements the direct destination←source transfer channel
// used in torrent migration (ADR-018). The source Agent spins up an HTTP server
// that serves (tar stream) the content of ONE torrent, and only under a valid
// one-time token issued by the Control Plane (scoped by hash, short TTL). The
// destination Agent pulls this tar and extracts it to its storage. The CP never
// proxies the bytes; it only authorizes (ADR-012 preserved: pull initiated by dest).
package transfer

import (
	"archive/tar"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/mediafile"
)

// ErrNothingToTransfer: the source has no data on disk for that torrent (hasn't
// downloaded anything yet). PERMANENT condition — caller must not retry (ADR-031).
var ErrNothingToTransfer = errors.New("source has no data to transfer")

// grant authorizes serving the content of a torrent until it expires.
type grant struct {
	hash    string
	expires time.Time
}

// Server serves, under a token, the content of torrents during migrations. Runs on
// the source. contentPath resolves the data path of a torrent (on disk) from
// the hash; storagePath and the extra roots confine what can be served (anti-traversal).
type Server struct {
	addr        string
	storagePath string
	// roots are the directories data may be served from: storagePath, plus the host side of an
	// adopted Engine's bind mounts (ADR-043), whose torrents may live outside the Node's storage.
	roots       []string
	contentPath func(ctx context.Context, hash string) (string, error)
	logger      *slog.Logger

	// createTorrent creates the .torrent (and starts seeding) from a path in the
	// storage, with trackers, private flag and source (embedded tag, ADR-020);
	// returns the .torrent bytes. Injected by the Agent (Engine adapter). nil
	// disables publishing on this Node.
	createTorrent func(ctx context.Context, sourcePath string, trackers []string, private bool, source string) ([]byte, error)

	mu      sync.Mutex
	grants  map[string]grant     // token → grant (migration, ADR-018)
	uploads map[string]time.Time // token → expiration (publishing, ADR-019)
}

// NewServer creates the transfer server. port is the listening port;
// storagePath is the allowed root; contentPath resolves the path of a torrent.
func NewServer(port int, storagePath string, contentPath func(ctx context.Context, hash string) (string, error), logger *slog.Logger) *Server {
	return &Server{
		addr:        ":" + strconv.Itoa(port),
		storagePath: filepath.Clean(storagePath),
		roots:       []string{filepath.Clean(storagePath)},
		contentPath: contentPath,
		logger:      logger,
		grants:      make(map[string]grant),
		uploads:     make(map[string]time.Time),
	}
}

// WithCreator enables receiving content + creating .torrent (publishing,
// ADR-019). fn creates the .torrent from a path in the storage. Returns the
// Server itself to chain into NewServer.
func (s *Server) WithCreator(fn func(ctx context.Context, sourcePath string, trackers []string, private bool, source string) ([]byte, error)) *Server {
	s.createTorrent = fn
	return s
}

// WithRoots adds directories, besides the storage, that torrent data may be served from — the
// host side of an adopted Engine's bind mounts (ADR-043). Returns the Server itself to chain
// into NewServer.
func (s *Server) WithRoots(roots []string) *Server {
	for _, r := range roots {
		s.roots = append(s.roots, filepath.Clean(r))
	}
	return s
}

// AuthorizeUpload allows receiving ONE upload under the token until it expires (the CP
// queues prepare_upload before doing the POST). Single use: the handler consumes
// the token upon completion.
func (s *Server) AuthorizeUpload(token string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads[token] = time.Now().Add(ttl)
	for k, exp := range s.uploads {
		if time.Now().After(exp) {
			delete(s.uploads, k)
		}
	}
}

// consumeUpload validates and removes the upload token (single use, constant-time).
func (s *Server) consumeUpload(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tok, exp := range s.uploads {
		if time.Now().After(exp) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(token)) == 1 {
			delete(s.uploads, tok)
			return true
		}
	}
	return false
}

// Authorize registers a token that allows serving the content of a torrent until ttl.
func (s *Server) Authorize(token, hash string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[token] = grant{hash: hash, expires: time.Now().Add(ttl)}
	// Opportunistic cleanup of expired grants.
	for k, g := range s.grants {
		if time.Now().After(g.expires) {
			delete(s.grants, k)
		}
	}
}

// lookup validates the token (constant-time against each live grant) and returns the hash.
func (s *Server) lookup(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tok, g := range s.grants {
		if time.Now().After(g.expires) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(token)) == 1 {
			return g.hash, true
		}
	}
	return "", false
}

// Run starts the HTTP server until the context is canceled.
func (s *Server) Run(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /transfer", s.handleTransfer)
	mux.HandleFunc("POST /content", s.handleContent)
	// No ReadTimeout/WriteTimeout: content uploads (GBs) can take minutes.
	srv := &http.Server{Addr: s.addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { //nolint:gosec // G118: ctx is already Done() here, using it as parent would cancel shutdown immediately
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	s.logger.Info("migration transfer server listening", "addr", s.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		s.logger.Error("transfer server exited", "error", err)
	}
}

// handleTransfer serves the tar of the torrent content authorized by the token.
func (s *Server) handleTransfer(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	hash, ok := s.lookup(token)
	if !ok {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}
	root, err := s.contentPath(r.Context(), hash)
	if err != nil || root == "" {
		http.Error(w, "torrent content not found", http.StatusNotFound)
		return
	}
	// Anti-traversal: the content must be under the storage or a mapped Engine directory.
	root = filepath.Clean(root)
	if !underAnyRoot(s.roots, root) {
		s.logger.Warn("refusing to serve path outside storage", "root", root, "roots", s.roots)
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	// ⚠️ Content must exist and have bytes BEFORE we write any headers
	// (ADR-031). Previously, a torrent that hadn't downloaded anything passed here, the 200 was
	// already sent and the `os.Stat` of tarPath failed too late: the `defer
	// tw.Close()` emitted the footer and the destination received A VALID AND EMPTY TAR with status
	// 200 — extractTar via io.EOF on the 1st entry and returned nil, so the pull "worked
	// out" without copying a byte. The destination then added the .torrent and downloaded everything
	// from the swarm (burns ratio — the opposite of what ADR-018 exists to prevent).
	// A pull that copies nothing is an ERROR, never a silent success.
	empty, err := isEmptyContent(root)
	if errors.Is(err, os.ErrNotExist) {
		// The path doesn't even exist: the Engine knows the torrent, but the download never
		// wrote anything. It's the SAME case as "nothing to migrate" (permanent), not a transient
		// 404 — returning 404 here would make the destination insist for 90s before
		// showing the operator an error that was already definitive.
		empty, err = true, nil
	}
	if err != nil {
		s.logger.Warn("torrent content unavailable for migration", "hash", hash, "root", root, "error", err)
		http.Error(w, "torrent content not found on disk", http.StatusNotFound)
		return
	}
	if empty {
		http.Error(w, "torrent has not yet downloaded data — nothing to migrate", http.StatusConflict)
		return
	}
	// Download via Portal (backlog #1): raw=1 serves the raw FILE when the torrent is
	// single-file (real name + Range/resume via ServeContent); folder becomes .tar.
	// Migration (ADR-018) calls WITHOUT raw → always tar (intact behavior).
	if r.URL.Query().Get("raw") == "1" {
		if fi, err := os.Stat(root); err == nil && fi.Mode().IsRegular() {
			f, err := os.Open(root)
			if err != nil {
				http.Error(w, "failed to open file", http.StatusInternalServerError)
				return
			}
			defer func() { _ = f.Close() }()
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(root)))
			http.ServeContent(w, r, filepath.Base(root), fi.ModTime(), f) // supports Range (resume)
			return
		}
		// Folder: delivers as tar, but with a user-friendly filename for the download.
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(root)+".tar"))
	}
	w.Header().Set("Content-Type", "application/x-tar")
	if err := tarPath(w, root); err != nil {
		s.logger.Error("error packaging content", "hash", hash, "error", err)
		// Header already sent; nothing to do but log.
	}
}

// handleContent receives the content (tar stream) of a publish under a one-time
// token, extracts it to storage in a folder with the given name, creates the .torrent (with
// the trackers and the private flag) and returns it to the CP (ADR-019). The CP does the POST in
// streaming; this Agent is never exposed to the browser.
func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	if s.createTorrent == nil {
		http.Error(w, "publishing not enabled on this Node", http.StatusNotImplemented)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if !s.consumeUpload(token) {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}
	name := sanitizeName(decodeHeader(r.Header.Get("X-Seedgrid-Name")))
	if name == "" {
		http.Error(w, "invalid content name", http.StatusBadRequest)
		return
	}
	announce := decodeHeader(r.Header.Get("X-Seedgrid-Announce"))
	private := r.Header.Get("X-Seedgrid-Private") == "true"
	source := decodeHeader(r.Header.Get("X-Seedgrid-Source"))

	destDir := filepath.Clean(filepath.Join(s.storagePath, name))
	if !underRoot(s.storagePath, destDir) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil { //nolint:gosec // G301: content read by the Engine (qBittorrent), needs to be accessible
		s.logger.Error("create publish directory", "dir", destDir, "error", err)
		http.Error(w, "failed to prepare storage", http.StatusInternalServerError)
		return
	}
	// The folder takes the storage's owner, and extractTar gives its content the same one.
	if uid, gid, ok := ownerOf(s.storagePath); ok {
		chownBestEffort(destDir, uid, gid)
	}
	if err := extractTar(r.Body, destDir); err != nil {
		s.logger.Error("extract published content", "error", err)
		http.Error(w, "failed to receive content", http.StatusInternalServerError)
		return
	}
	var trackers []string
	if announce != "" {
		trackers = []string{announce}
	}
	data, err := s.createTorrent(r.Context(), destDir, trackers, private, source)
	if err != nil {
		s.logger.Error("create .torrent for published content", "dir", destDir, "error", err)
		http.Error(w, "failed to create torrent", http.StatusInternalServerError)
		return
	}
	// mediainfo (ADR-020): only when the CP requests it (publishing to UNIT3D tracker). Best-effort:
	// failure/missing binary does not invalidate the publish — returns empty and the operator completes it.
	mediaInfo := ""
	if r.Header.Get("X-Seedgrid-Mediainfo") == "true" {
		mediaInfo = runMediainfo(r.Context(), destDir, s.logger)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"torrent_file": base64.StdEncoding.EncodeToString(data),
		"media_info":   mediaInfo,
	})
}

// runMediainfo runs the `mediainfo` binary on the main media file under dir
// (the largest regular file) and returns the text output. "" on any failure (missing
// binary, no files, execution error) — it's best-effort.
func runMediainfo(ctx context.Context, dir string, logger *slog.Logger) string {
	file := mediafile.LargestFile(dir)
	if file == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mediainfo", file).Output() //nolint:gosec // G204: file comes from mediafile.LargestFile() inside the already validated destDir
	if err != nil {
		logger.Warn("mediainfo unavailable or failed", "file", file, "error", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// decodeHeader decodes a base64 header (used for name and announce, which may
// have characters outside header ASCII). Empty or invalid → "".
func decodeHeader(v string) string {
	if v == "" {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// sanitizeName reduces the content name to its base component, rejecting
// traversal and empty/special names — the destination is always a direct folder under
// storage.
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(filepath.Clean(name))
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, os.PathSeparator) {
		return ""
	}
	return name
}

// isEmptyContent reports if root has no bytes at all to transfer — a 0-sized
// file, or a directory without any regular file with bytes (ADR-031). A stat
// error (non-existent path) bubbles up for the caller to respond 404.
//
// Note: a PARTIAL torrent is not empty — with pre-allocation off qBittorrent
// writes sparse files, whose Size() is the total and whose content already has the pieces
// downloaded. Migrating partial is exactly the case that ADR-031 unlocks.
func isEmptyContent(root string) (bool, error) {
	fi, err := os.Stat(root)
	if err != nil {
		return false, err
	}
	if !fi.IsDir() {
		return fi.Size() == 0, nil
	}
	empty := true
	err = filepath.Walk(root, func(_ string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !f.IsDir() && f.Size() > 0 {
			empty = false
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return empty, nil
}

// tarPath writes a tar of root (file or directory) to w, preserving the base
// name as the root of the entries (single-file → 1 entry; multi → the tree).
func tarPath(w io.Writer, root string) error {
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()
	base := filepath.Base(root)
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return writeTarFile(tw, root, base, info)
	}
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := base
		if rel != "." {
			name = filepath.Join(base, rel)
		}
		if fi.IsDir() {
			return tw.WriteHeader(&tar.Header{Name: name + "/", Mode: 0o755, Typeflag: tar.TypeDir})
		}
		return writeTarFile(tw, path, name, fi)
	})
}

func writeTarFile(tw *tar.Writer, path, name string, fi os.FileInfo) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: fi.Size(), Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	f, err := os.Open(path) //nolint:gosec // G304: path comes from the internal filepath.Walk of the storage tree, not from external input
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(tw, f)
	return err
}

// Pull on Server delegates to the package function (the same Agent serves and pulls) — this way the
// Server satisfies the Migrator interface of the commands.
func (s *Server) Pull(ctx context.Context, host string, port int, token, destPath string) error {
	return Pull(ctx, host, port, token, destPath)
}

// Pull pulls the tar of the content of a torrent from source:port under a token and extracts it
// to destPath (the destination Agent calls this). Returns an error on any failure.
func Pull(ctx context.Context, sourceHost string, sourcePort int, token, destPath string) error {
	u := fmt.Sprintf("http://%s/transfer?token=%s", net.JoinHostPort(sourceHost, strconv.Itoa(sourcePort)), token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 0} // large transfers; ctx controls cancellation
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err := fmt.Errorf("source rejected (status %d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
		// 409 = the source has no data to serve (ADR-031). It's a PERMANENT condition:
		// retrying for 90s only delays the error. 401 remains retryable — it's the normal race
		// between the destination's pull and the source's migrate_serve.
		if resp.StatusCode == http.StatusConflict {
			return fmt.Errorf("%w: %s", ErrNothingToTransfer, strings.TrimSpace(string(b)))
		}
		return err
	}
	return extractTar(resp.Body, filepath.Clean(destPath))
}

// extractTar extracts a tar to destPath, rejecting entries with traversal. What it creates gets
// destPath's owner (ADR-043): the Agent runs as root, and an Engine in a container usually does
// not — it must be able to keep writing to a partially migrated torrent.
func extractTar(r io.Reader, destPath string) error {
	uid, gid, own := ownerOf(destPath)
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destPath, hdr.Name) //nolint:gosec // G305: path validated via underRoot() below
		if !underRoot(destPath, filepath.Clean(target)) {
			return fmt.Errorf("tar entry with invalid path: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil { //nolint:gosec // G301: content read by the Engine (qBittorrent), needs to be accessible
				return err
			}
			if own {
				chownBestEffort(target, uid, gid)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // G301: idem
				return err
			}
			if own && filepath.Dir(target) != destPath {
				chownBestEffort(filepath.Dir(target), uid, gid)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644) //nolint:gosec // G304/G306: target validated by underRoot() above; needs to be readable by the Engine
			if err != nil {
				return err
			}
			// io.CopyN limited to hdr.Size (not io.Copy): a tar entry cannot
			// write beyond what the header itself declared (defense against tar bomb).
			// written == hdr.Size iff err == nil (io.CopyN doc) — no need
			// to handle io.EOF separately; if it occurs, it's a truly truncated entry.
			if _, err := io.CopyN(f, tr, hdr.Size); err != nil {
				_ = f.Close()
				return err
			}
			_ = f.Close()
			if own {
				chownBestEffort(target, uid, gid)
			}
		}
	}
}

// underAnyRoot reports whether path is contained in one of roots (all already cleaned).
func underAnyRoot(roots []string, path string) bool {
	for _, r := range roots {
		if underRoot(r, path) {
			return true
		}
	}
	return false
}

// underRoot reports whether path is contained in root (both already cleaned).
func underRoot(root, path string) bool {
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(os.PathSeparator))
}
