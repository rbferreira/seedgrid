// Content PUBLISHING handlers: create the .torrent from local files
// (ADR-019), register on the UNIT3D tracker (ADR-020), screenshots for the description
// (ADR-026/030) and metadata inference that pre-fills the form.

package api

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/description"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/igdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
	"github.com/rbferreira/seedgrid/control-plane/internal/release"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/tmdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
	"github.com/rbferreira/seedgrid/control-plane/internal/unit3d"
)

// handleCreateTorrent publishes local content (ADR-019): receives files via
// multipart and STREAMS them to the chosen Node (Scheduler or forced),
// which saves, creates the .torrent with the chosen announce and starts seeding. The CP does not
// store the bytes. The body must include, BEFORE the files, the text fields:
// name, tracker_id ("public" or the id of a cluster tracker), force_node_id and
// manifest (JSON [{name,size}] in file order — tar needs the size).
func handleCreateTorrent(d Deps) http.HandlerFunc {
	type fileSpec struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Large uploads (GBs): remove the read/write deadlines for this request.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})

		mr, err := r.MultipartReader()
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "send the content as multipart/form-data")
			return
		}

		var name, trackerID, forceNode string
		var manifest []fileSpec
		var firstFile *multipart.Part
		// Reads the text fields up to the first file part (they must come first).
		for {
			part, perr := mr.NextPart()
			if perr == io.EOF {
				break
			}
			if perr != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid multipart")
				return
			}
			if part.FileName() != "" {
				firstFile = part
				break
			}
			val, _ := io.ReadAll(io.LimitReader(part, 1<<20))
			switch part.FormName() {
			case "name":
				name = strings.TrimSpace(string(val))
			case "tracker_id":
				trackerID = strings.TrimSpace(string(val))
			case "force_node_id":
				forceNode = strings.TrimSpace(string(val))
			case "manifest":
				_ = json.Unmarshal(val, &manifest)
			}
			_ = part.Close()
		}
		if firstFile == nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "no file uploaded")
			return
		}
		if len(manifest) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "manifest (file names+sizes) is required")
			return
		}
		if name == "" {
			name = filepath.Base(firstFile.FileName())
		}

		// Resolves the announce: "public"/empty → no tracker (DHT); otherwise, an operator's
		// tracker. For UNIT3D tracker (ADR-020), source is embedded in the .torrent.
		announce, source, private, trackerRef := "", "", false, ""
		if trackerID != "" && trackerID != "public" {
			sec, terr := d.Trackers.ForPublish(r.Context(), trackerID)
			if terr != nil {
				trackerError(w, d, terr)
				return
			}
			announce, source, private, trackerRef = sec.Announce, sec.Source, true, trackerID
		}

		// Builds the tar in streaming (io.Pipe): entries use name+size from the manifest.
		pr, pw := io.Pipe()
		go func() {
			tw := tar.NewWriter(pw)
			werr := streamFilesToTar(tw, mr, firstFile, func(i int) (string, int64, bool) {
				if i >= len(manifest) {
					return "", 0, false
				}
				return filepath.Base(manifest[i].Name), manifest[i].Size, true
			})
			if cerr := tw.Close(); werr == nil {
				werr = cerr
			}
			pw.CloseWithError(werr)
		}()

		// Extracts mediainfo on the Node only when publishing to a UNIT3D tracker (source != "").
		wantMediainfo := source != ""
		t, mediaInfo, err := d.Torrents.Create(r.Context(), name, announce, source, trackerRef, private, forceNode, wantMediainfo, pr)
		switch {
		case errors.Is(err, scheduler.ErrNoEligibleNode):
			httpx.WriteErrorParams(w, http.StatusConflict, "no_eligible_node", noEligibleNodeMessage(err), noEligibleNodeParams(err))
		case errors.Is(err, scheduler.ErrNoNodeWithSpace):
			httpx.WriteErrorParams(w, http.StatusConflict, "no_node_with_space", noNodeWithSpaceMessage(err), noNodeWithSpaceParams(err))
		case errors.Is(err, torrent.ErrNodeNotEligible):
			httpx.WriteError(w, http.StatusConflict, "node_not_eligible", "destination Node not found or offline")
		case err != nil:
			d.Logger.Error("publish content", "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "publish_failed", fmt.Sprintf("failed to publish content: %s", err))
		default:
			// Flattens the torrent + media_info fields (the Portal uses both in the Publish tab).
			httpx.WriteJSON(w, http.StatusCreated, struct {
				torrent.Torrent
				MediaInfo string `json:"media_info,omitempty"`
			}{Torrent: t, MediaInfo: mediaInfo})
		}
	}
}

// streamFilesToTar writes each multipart file part to the tar (starting from
// first), using the name/size provided by spec(i) — tar requires the size
// upfront, which comes from the manifest (browser's File.size). Ignores non-file parts.
func streamFilesToTar(tw *tar.Writer, mr *multipart.Reader, first *multipart.Part, spec func(i int) (string, int64, bool)) error {
	idx := 0
	part := first
	for {
		if part.FileName() != "" {
			hdrName, size, ok := spec(idx)
			if !ok || hdrName == "" {
				return errors.New("file has no matching entry in the manifest")
			}
			idx++
			if err := tw.WriteHeader(&tar.Header{Name: hdrName, Mode: 0o644, Size: size, Typeflag: tar.TypeReg}); err != nil {
				return err
			}
			if _, err := io.Copy(tw, part); err != nil {
				return err
			}
		}
		_ = part.Close()
		next, err := mr.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		part = next
	}
}

// publishRequest is the form reviewed by the operator. Shared between preview and
// submit (#188) — the preview only makes sense if both read the same body.
type publishRequest struct {
	TrackerID    string `json:"tracker_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	CategoryID   int    `json:"category_id"`
	TypeID       int    `json:"type_id"`
	ResolutionID int    `json:"resolution_id"`
	TMDB         int    `json:"tmdb"`
	IMDB         string `json:"imdb"`
	TVDB         int    `json:"tvdb"`
	MAL          int    `json:"mal"`
	IGDB         int    `json:"igdb"` // games (#17)
	Season       int    `json:"season_number"`
	Episode      int    `json:"episode_number"`
	Keywords     string `json:"keywords"`
	MediaInfo    string `json:"mediainfo"`
	Anonymous    bool   `json:"anonymous"`
	// PreviewToken: the fingerprint returned by the preview. Optional to avoid breaking
	// existing clients, but VERIFIED when provided — the Portal always sends it.
	PreviewToken string `json:"preview_token"`
}

// publishParams translates the form into the tracker payload. A single function, used by
// both preview and submit: two translations could diverge, and the preview exists
// precisely because the UNIT3D API lacks a dry-run to catch a divergence.
func publishParams(req publishRequest, mediaInfo string) unit3d.Params {
	return unit3d.Params{
		Name:         req.Name,
		Description:  req.Description,
		CategoryID:   req.CategoryID,
		TypeID:       req.TypeID,
		ResolutionID: req.ResolutionID,
		TMDB:         req.TMDB,
		IMDB:         req.IMDB,
		TVDB:         req.TVDB,
		MAL:          req.MAL,
		IGDB:         req.IGDB,
		Season:       req.Season,
		Episode:      req.Episode,
		Keywords:     req.Keywords,
		MediaInfo:    mediaInfo,
		Anonymous:    req.Anonymous,
	}
}

// handlePublishToTracker registers an already created torrent in a tracker's
// UNIT3D catalog (ADR-020): resolves the tracker's api_url/api_token, retrieves the persisted
// .torrent and POSTs with the metadata REVIEWED by the operator. It's the final step,
// triggered explicitly by the Portal (anti-pollution gate). Nothing is posted without this call.
func handlePublishToTracker(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := r.PathValue("id")
		var req publishRequest
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if strings.TrimSpace(req.TrackerID) == "" || req.TrackerID == "public" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "tracker_id is required")
			return
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Description) == "" || req.CategoryID == 0 || req.TypeID == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "name, description, category_id and type_id are required")
			return
		}

		// Tracker API credentials (never exposed; only used here).
		sec, err := d.Trackers.ForPublish(r.Context(), req.TrackerID)
		if err != nil {
			trackerError(w, d, err)
			return
		}
		if strings.TrimSpace(sec.APIURL) == "" || strings.TrimSpace(sec.APIToken) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "tracker_without_api", "this tracker has no API integration configured (api_url + token)")
			return
		}

		// Persisted .torrent (source of truth) + torrent name/mediainfo.
		data, err := d.Torrents.GetTorrentFile(r.Context(), torrentID)
		if errors.Is(err, torrent.ErrNotFound) {
			httpx.WriteError(w, http.StatusConflict, "torrent_file_missing", "the .torrent for this torrent is not yet available")
			return
		}
		if err != nil {
			d.Logger.Error("fetch .torrent for external registration", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve .torrent")
			return
		}
		tName, storedMedia, _ := d.Torrents.TorrentMeta(r.Context(), torrentID)
		mediaInfo := req.MediaInfo
		if strings.TrimSpace(mediaInfo) == "" {
			mediaInfo = storedMedia // fallback to what was extracted on the Node if the operator didn't edit it
		}

		params := publishParams(req, mediaInfo)

		// Frozen payload (#188): if the operator reviewed a preview and changed the form
		// afterwards, what they approved isn't what would be posted. Reject instead of
		// publishing the unreviewed — UNIT3D has no dry-run or undo.
		if tok := strings.TrimSpace(req.PreviewToken); tok != "" {
			if now := unit3d.Fingerprint(params, tName, data); now != tok {
				httpx.WriteError(w, http.StatusConflict, "preview_stale",
					"the form changed after preview; check again before publishing")
				return
			}
		}

		res, err := unit3d.Upload(r.Context(), sec.APIURL, sec.APIToken, params, tName, data)
		if err != nil {
			d.Logger.Warn("external tracker registration", "tracker", sec.Source, "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "tracker_upload_failed", err.Error())
			return
		}
		// Exits the pending queue (backlog #8): records the timestamp and the returned URL.
		if err := d.Torrents.MarkPublished(r.Context(), torrentID, res.DownloadURL); err != nil {
			d.Logger.Warn("mark torrent as published", "torrent", torrentID, "error", err)
		}
		// Infohash verification (#23): downloads the approved OFFICIAL .torrent and, if the
		// infohash differs from what we seed, automatically re-seeds (the site might have
		// regenerated it with its own source/pieces). Best-effort: failure here doesn't invalidate the
		// already completed publication.
		reseeded := false
		if strings.TrimSpace(res.DownloadURL) != "" {
			if official, derr := unit3d.DownloadFromLink(r.Context(), res.DownloadURL, sec.APIToken); derr != nil {
				d.Logger.Warn("download official .torrent for infohash verification", "torrent", torrentID, "error", derr)
			} else if rs, verr := d.Torrents.VerifyReseed(r.Context(), torrentID, official); verr != nil {
				d.Logger.Warn("verify infohash post-publication", "torrent", torrentID, "error", verr)
			} else if rs {
				reseeded = true
				d.Logger.Info("divergent infohash fixed by re-seed", "torrent", torrentID)
			}
		}
		if d.Events != nil {
			label := req.Name
			if strings.TrimSpace(label) == "" {
				label = tName
			}
			_ = d.Events.Emit(r.Context(), "TORRENT_PUBLISHED_EXTERNAL", "INFO", "",
				fmt.Sprintf("Torrent published to tracker %s: %s", sec.Source, label),
				map[string]any{"tracker": sec.Source, "name": req.Name, "download_url": res.DownloadURL})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":       "ok",
			"download_url": res.DownloadURL,
			"message":      res.Message,
			"reseeded":     reseeded, // #23: infohash differed from the site and was corrected
		})
	}
}

// handleExtractScreenshots (F1) queues the extraction on the Node and returns the command_id — the
// Portal tracks the "Extracting on Node" step via GET /commands/{id} and then calls
// /screenshots/render. Only extracts frames; doesn't touch imgbb.
func handleExtractScreenshots(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := httpx.OperatorFromContext(r.Context()); !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		torrentID := r.PathValue("id")
		count := d.Settings.Int("publish.screenshot_count")
		cmdID, err := d.Torrents.RequestScreenshots(r.Context(), torrentID, count)
		switch {
		case errors.Is(err, torrent.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "torrent_not_found", "torrent not found")
		case errors.Is(err, torrent.ErrNoHash):
			httpx.WriteError(w, http.StatusConflict, "no_hash", "torrent has no hash yet; wait for synchronization")
		case err != nil:
			d.Logger.Warn("queue screenshot extraction", "torrent", torrentID, "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "screenshots_failed", err.Error())
		default:
			httpx.WriteJSON(w, http.StatusAccepted, struct {
				CommandID string `json:"command_id"`
			}{CommandID: cmdID})
		}
	}
}

// screenshotResponse is the 200 response for both screenshot routes (#193): the BBCode, the host
// that hosted it, and the failures of previous hosts in the chain — `failures` is always a list,
// empty when the first host succeeded.
type screenshotResponse struct {
	BBCode   string              `json:"bbcode"`
	Host     string              `json:"host"`
	Failures []imagehost.Failure `json:"failures"`
}

func newScreenshotResponse(res torrent.ScreenshotResult) screenshotResponse {
	failures := res.Failures
	if failures == nil {
		failures = []imagehost.Failure{}
	}
	return screenshotResponse{BBCode: res.BBCode, Host: res.Host, Failures: failures}
}

// trackerExists verifies, when the request includes tracker_id, that the tracker exists —
// BEFORE extracting frames or uploading anything (removed tracker or malformed id returns 404,
// like in profile routes). Writes the error and returns false when it fails.
func trackerExists(w http.ResponseWriter, r *http.Request, d Deps, trackerID string) bool {
	if trackerID == "" {
		return true
	}
	if _, err := d.Profiles.Get(r.Context(), trackerID); err != nil {
		profileError(w, d, err)
		return false
	}
	return true
}

// screenshotError responds to failures in the screenshot routes. The text of a *ChainError lists
// only the reasons from the closed set (ADR-040), never the raw error from a host.
func screenshotError(w http.ResponseWriter, d Deps, err error, logArgs ...any) {
	switch {
	case errors.Is(err, torrent.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "torrent_not_found", "torrent not found")
	case errors.Is(err, torrent.ErrNoHash):
		httpx.WriteError(w, http.StatusConflict, "no_hash", "torrent has no hash yet; wait for synchronization")
	case errors.Is(err, trackerprofile.ErrNotFound), errors.Is(err, tracker.ErrNotFound):
		// The tracker disappeared between the ownership check and the chain setup.
		httpx.WriteError(w, http.StatusNotFound, "tracker_not_found", "tracker not found")
	default:
		d.Logger.Warn("screenshots", append(logArgs, "error", err)...)
		httpx.WriteError(w, http.StatusBadGateway, "screenshots_failed", err.Error())
	}
}

// handleRenderScreenshots (F1) reads the frames from a completed extract_screenshots command,
// uploads them to the host chain (ADR-040, #193) and returns the BBCode. Called by the Portal after
// the command reaches DONE. tracker_id is optional: if present, the tracker's host joins the chain.
func handleRenderScreenshots(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CommandID string `json:"command_id"`
			TrackerID string `json:"tracker_id"`
		}
		if err := httpx.DecodeJSON(r, &req); err != nil || strings.TrimSpace(req.CommandID) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "command_id is required")
			return
		}
		if !trackerExists(w, r, d, req.TrackerID) {
			return
		}
		imgbbKey := d.Settings.String("integrations.imgbb_api_key")
		res, err := d.Torrents.RenderScreenshots(r.Context(), req.CommandID, req.TrackerID, imgbbKey)
		if err != nil {
			screenshotError(w, d, err, "command", req.CommandID)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, newScreenshotResponse(res))
	}
}

// handleGenerateScreenshots extracts screenshots from the torrent (via Agent) and returns the
// ready BBCode for the operator to paste/review in the description before publishing
// (ADR-026) — same human review gate as ADR-020, nothing is sent to the
// tracker via this route. Optional body `{"tracker_id"}` (#193): if present, the tracker's
// host joins the upload chain; empty remains valid (only imgbb).
func handleGenerateScreenshots(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := r.PathValue("id")
		var req struct {
			TrackerID string `json:"tracker_id"`
		}
		// Empty body (io.EOF) is the legacy behavior: only imgbb.
		if err := httpx.DecodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", `invalid body: expected {"tracker_id"} or empty`)
			return
		}
		if !trackerExists(w, r, d, req.TrackerID) {
			return
		}
		count := d.Settings.Int("publish.screenshot_count")
		// The imgbb key comes from the catalog (ADR-029/030), editable in the Portal — empty disables it.
		imgbbKey := d.Settings.String("integrations.imgbb_api_key")
		res, err := d.Torrents.GenerateScreenshots(r.Context(), torrentID, req.TrackerID, imgbbKey, count)
		if err != nil {
			screenshotError(w, d, err, "torrent", torrentID)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, newScreenshotResponse(res))
	}
}

// handlePendingTracker lists torrents that were created with a tracker but not yet
// published to it (ADR-020, backlog #8) — the queue the Portal shows in the
// Publish tab to resume publishing without re-uploading.
func handlePendingTracker(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := d.Torrents.PendingTracker(r.Context())
		if err != nil {
			d.Logger.Error("list pending releases", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list pending releases")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"pending": list})
	}
}

// handleInferPublish infers release metadata (ADR-020) from the name:
// "scene" parse (title/year/season/resolution/type/category) + TMDB query
// (tmdb/imdb/tvdb) when configured. It's just a guess — the Portal pre-fills the
// form and the operator reviews before posting. Never fails due to TMDB.
func handleInferPublish(d Deps) http.HandlerFunc {
	type request struct {
		Name      string `json:"name"`
		TV        *bool  `json:"tv"`        // optional override for the series/movie heuristic
		MediaInfo string `json:"mediainfo"` // mediainfo text to enrich the name (ADR-020)
		// TrackerID (optional, #74): the synopsis comes in the content language of that
		// tracker's profile. Without it, English.
		TrackerID string `json:"tracker_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		parsed := release.Parse(name)
		// Enriches the name with the file's technical data (resolution/codec/audio) that
		// the name might lack — resolves "The Witch 2015" → "…1080p x264 DDP5.1" (ADR-020).
		release.EnrichFromMediaInfo(&parsed, req.MediaInfo)

		isTV := parsed.IsTV
		if req.TV != nil {
			isTV = *req.TV
		}

		resp := map[string]any{"parsed": parsed}
		if d.TMDB != nil && d.TMDB.Enabled() {
			lang := description.LangEN
			if strings.TrimSpace(req.TrackerID) != "" {
				lang = contentLanguage(r.Context(), d, req.TrackerID)
			}
			res, err := d.TMDB.Lookup(r.Context(), parsed.Title, parsed.Year, isTV, lang)
			switch {
			case err == nil:
				resp["tmdb"] = res
			case errors.Is(err, tmdb.ErrNotFound):
				resp["tmdb"] = nil
				resp["tmdb_note"] = "no TMDB results for the inferred title/year"
			default:
				d.Logger.Warn("TMDB query", "error", err)
				resp["tmdb"] = nil
				resp["tmdb_note"] = "TMDB query failed"
			}
		} else {
			resp["tmdb"] = nil
			resp["tmdb_note"] = "TMDB not configured in Control Plane"
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

// maxDescriptionBody limits the POST /publish/description body. MediaInfo for a long release
// exceeds 100 KiB and the screenshot block comes along; 4 MiB gives headroom and blocks hostile bodies.
const maxDescriptionBody = 4 << 20

// handleComposeDescription composes the BBCode description of an upload (#192). It is what the Portal
// used to build in TypeScript (bbcode.ts), now in the Control Plane and per TRACKER: the composition follows the
// description section of the tracker profile (blocks, order, MediaInfo tag) or, if missing, the
// default — which is byte-for-byte the text the Portal always produced. The result is just a starting
// point: the operator keeps editing the field before publishing (human review, ADR-020).
//
// It only composes text from what comes in the body: it doesn't read the torrent, doesn't call TMDB or IGDB, and doesn't
// change state — which is why it stays out of the audit trail (skipAuditPath), like the infer.
func handleComposeDescription(d Deps) http.HandlerFunc {
	type request struct {
		TrackerID string                 `json:"tracker_id"`
		Kind      string                 `json:"kind"` // "media" (movie/series) or "game"
		Media     description.MediaInput `json:"media"`
		Game      description.GameInput  `json:"game"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		r.Body = http.MaxBytesReader(w, r.Body, maxDescriptionBody)
		if err := httpx.DecodeJSON(r, &req); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				httpx.WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "body too large")
				return
			}
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if req.Kind != "media" && req.Kind != "game" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", `kind must be "media" or "game"`)
			return
		}
		if strings.TrimSpace(req.TrackerID) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "tracker_id is required")
			return
		}
		// Get checks existence: a removed tracker or one with an id
		// that is malformed is a 404, as in every tracker route.
		prof, err := d.Profiles.Get(r.Context(), req.TrackerID)
		if err != nil {
			profileError(w, d, err)
			return
		}
		cfg := description.Config{} // zero = the default
		if prof.Description != nil {
			cfg = *prof.Description
		}
		var text string
		if req.Kind == "game" {
			text = description.ComposeGame(req.Game, cfg)
		} else {
			text = description.ComposeMedia(req.Media, cfg)
		}
		httpx.WriteJSON(w, http.StatusOK, struct {
			Description string `json:"description"`
		}{Description: text})
	}
}

// handleSearchGames searches for games on IGDB by title (#17) and returns the candidates for the
// operator to choose from on the publishing panel (IGDB search sometimes prioritizes mods/hacks).
func handleSearchGames(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "provide the q parameter")
			return
		}
		if d.IGDB == nil || !d.IGDB.Enabled() {
			httpx.WriteError(w, http.StatusServiceUnavailable, "igdb_disabled",
				"IGDB not configured in Control Plane (set SEEDGRID_IGDB_CLIENT_ID/SECRET)")
			return
		}
		games, err := d.IGDB.Search(r.Context(), q, 12)
		if errors.Is(err, igdb.ErrNotFound) {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"games": []any{}})
			return
		}
		if err != nil {
			d.Logger.Warn("IGDB search", "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "igdb_failed", "failed to search IGDB")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"games": games})
	}
}

// ── Duplicate evidence on the tracker (#186) ─────────────────────────────────
//
// Before publishing, the operator needs to know if that release already exists in the tracker's
// catalog — publishing a duplicate on a private tracker yields a warning. The result is EVIDENCE
// for human review, never a block: tracker down, network error, or a search
// with no results returns 200 with an empty list and a notice. Search failure cannot prevent
// publishing; the operator decides, just as they already decide on cluster duplicates (#11).

// dupeCandidate is an item from the tracker catalog + what helps the operator to judge.
type dupeCandidate struct {
	unit3d.BrowseItem
	SameYear bool `json:"same_year"`
}

// maxDupeCandidates limits what goes to the panel — the search is by title and might return
// the entire franchise; past a certain point the list stops informing and starts getting in the way.
const maxDupeCandidates = 25

// dupeSearch is the signature of unit3d.Browse, isolated so the test can inject a failure
// without spinning up a fake tracker.
type dupeSearch func(ctx context.Context, apiURL, apiToken, cursor, search string) (unit3d.BrowseResult, error)

// dupeQuery derives the search term from the release name. It uses the inferred TITLE, not the full
// standard name: the full name is too specific and the UNIT3D name filter wouldn't
// match anything. Without an inferable title, it falls back to the original name.
func dupeQuery(name string) string {
	if title := strings.TrimSpace(release.Parse(name).Title); title != "" {
		return title
	}
	return strings.TrimSpace(name)
}

// dupeCandidates flags and limits the items returned by the tracker.
//
// The year only FLAGS (same_year), it doesn't filter: the catalog sometimes brings the wrong year, and
// hiding a candidate is worse than showing too many candidates — the cost of a false
// positive is the operator having to look; the cost of a false negative is publishing a duplicate.
func dupeCandidates(items []unit3d.BrowseItem, year int) []dupeCandidate {
	out := make([]dupeCandidate, 0, len(items))
	for _, it := range items {
		if len(out) >= maxDupeCandidates {
			break
		}
		out = append(out, dupeCandidate{
			BrowseItem: it,
			SameYear:   year > 0 && it.Year == year,
		})
	}
	return out
}

// dupeEvidence builds the response. A search error does NOT become an HTTP error: it degrades to an empty
// list with a notice, so the panel can enable the button instead of blocking the publishing.
func dupeEvidence(ctx context.Context, search dupeSearch, apiURL, apiToken, name string) (map[string]any, error) {
	query := dupeQuery(name)
	res, err := search(ctx, apiURL, apiToken, "", query)
	if err != nil {
		return map[string]any{
			"query":      query,
			"candidates": []dupeCandidate{},
			"note":       "could not query the tracker catalog; publish manually",
		}, err
	}
	return map[string]any{
		"query":      query,
		"candidates": dupeCandidates(res.Items, release.Parse(name).Year),
	}, nil
}

// handleTrackerDupes: GET /trackers/{id}/dupes?name=…
func handleTrackerDupes(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		sec, err := d.Trackers.ForPublish(r.Context(), r.PathValue("id"))
		if err != nil {
			trackerError(w, d, err)
			return
		}
		if strings.TrimSpace(sec.APIURL) == "" || strings.TrimSpace(sec.APIToken) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "tracker_without_api", "this tracker has no API integration (api_url + token)")
			return
		}
		// Its own deadline, below the WriteTimeout: without it a slow tracker became the proxy's
		// 502, and the route promises never to fail upward (#41).
		ctx, cancel := context.WithTimeout(r.Context(), browseBudget)
		defer cancel()
		resp, err := dupeEvidence(ctx, unit3d.Browse, sec.APIURL, sec.APIToken, name)
		if err != nil {
			d.Logger.Warn("search for duplicate on tracker", "tracker", sec.Source, "error", err)
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

// handlePublishPreview: POST /torrents/{id}/publish-tracker/preview
//
// Shows the EXACT payload that submit would send, without touching the network. It exists because the UNIT3D
// API has no dry-run — without this, the only way to find out what will be posted
// is to post it, and a wrong field becomes a wrong record in a private tracker's catalog.
//
// It also returns a `token`: the payload's fingerprint. The Portal returns it on submit, which
// rejects if the form has changed in the meantime (409 preview_stale). It's what makes "frozen"
// mean something — without the token, the preview would just be a pretty screen over a
// payload that could still change.
//
// The `.torrent` doesn't go in the body (it's MBs and the operator doesn't check bytes): name,
// size, and the fact that the fingerprint covers them do.
func handlePublishPreview(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		torrentID := r.PathValue("id")
		var req publishRequest
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if strings.TrimSpace(req.TrackerID) == "" || req.TrackerID == "public" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "tracker_id is required")
			return
		}

		// Checks that the tracker exists and has an API — the preview cannot say
		// "all good" for a publish that submit would reject due to credentials.
		sec, err := d.Trackers.ForPublish(r.Context(), req.TrackerID)
		if err != nil {
			trackerError(w, d, err)
			return
		}
		if strings.TrimSpace(sec.APIURL) == "" || strings.TrimSpace(sec.APIToken) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "tracker_without_api", "this tracker has no API integration configured (api_url + token)")
			return
		}

		data, err := d.Torrents.GetTorrentFile(r.Context(), torrentID)
		if errors.Is(err, torrent.ErrNotFound) {
			httpx.WriteError(w, http.StatusConflict, "torrent_file_missing", "the .torrent for this torrent is not yet available")
			return
		}
		if err != nil {
			d.Logger.Error("get .torrent for preview", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve .torrent")
			return
		}
		tName, storedMedia, _ := d.Torrents.TorrentMeta(r.Context(), torrentID)
		mediaInfo := req.MediaInfo
		if strings.TrimSpace(mediaInfo) == "" {
			mediaInfo = storedMedia
		}

		params := publishParams(req, mediaInfo)
		// What's missing to fill out, said BEFORE the POST. The submit rejects the same fields;
		// here it becomes a warning, so the operator can fix it on the screen instead of getting a 400.
		var missing []string
		if strings.TrimSpace(req.Name) == "" {
			missing = append(missing, "name")
		}
		if strings.TrimSpace(req.Description) == "" {
			missing = append(missing, "description")
		}
		if req.CategoryID == 0 {
			missing = append(missing, "category_id")
		}
		if req.TypeID == 0 {
			missing = append(missing, "type_id")
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"endpoint":     strings.TrimRight(sec.APIURL, "/") + "/api/torrents/upload",
			"tracker":      sec.Source,
			"fields":       unit3d.Fields(params),
			"torrent_name": tName,
			"torrent_size": len(data),
			"missing":      missing,
			"token":        unit3d.Fingerprint(params, tName, data),
		})
	}
}

// contentLanguage is the content language of a tracker's profile (#74): the language of the
// upload description and of the metadata TMDB returns for that site. A tracker without a
// profile, without the description section, or that does not exist is English — the
// lookup never fails a request because of it.
func contentLanguage(ctx context.Context, d Deps, trackerID string) string {
	if d.Profiles == nil {
		return description.LangEN
	}
	prof, err := d.Profiles.Get(ctx, trackerID)
	if err != nil || prof.Description == nil {
		return description.LangEN
	}
	return prof.Description.ContentLanguage()
}
