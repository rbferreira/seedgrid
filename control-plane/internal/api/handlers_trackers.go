// Operator tracker handlers (ADR-019/020): CRUD of announce/API credentials,
// default tracker, catalog browsing (#12) and seeding an item from there.

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/enrich"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/release"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/unit3d"
)

// trackerError maps tracker service errors to HTTP responses.
func trackerError(w http.ResponseWriter, d Deps, err error) {
	switch {
	case errors.Is(err, tracker.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "tracker_not_found", "tracker not found")
	case errors.Is(err, tracker.ErrDuplicateName):
		httpx.WriteError(w, http.StatusConflict, "duplicate_name", "a tracker with this name already exists")
	case errors.Is(err, tracker.ErrInvalidAnnounce):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_announce", "name and announce URL (http/https/udp) are required")
	case errors.Is(err, tracker.ErrInvalidAPIURL):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_api_url", "tracker API URL (api_url) must be http(s)")
	default:
		d.Logger.Error("tracker", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to process tracker")
	}
}

func handleListTrackers(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		ts, err := d.Trackers.List(r.Context(), user.ID)
		if err != nil {
			d.Logger.Error("list trackers", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list trackers")
			return
		}
		// Each tracker carries the site profile (taxonomy, ADR-037) — that's where the
		// publishing panel reads options from, without asking again. No credential: the public
		// tracker view (layer 3 without secrets) plus the profile (layer 2).
		profiles := profilesOf(r.Context(), d, user.ID)
		views := make([]trackerView, 0, len(ts))
		for _, t := range ts {
			views = append(views, viewOf(t, profiles))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
	}
}

// trackerRequest is the Create/Update body. Source/APIURL/APIToken are optional
// (UNIT3D integration, ADR-020). On Update, empty APIToken keeps the current token.
type trackerRequest struct {
	Name      string `json:"name"`
	Announce  string `json:"announce"`
	Source    string `json:"source"`
	APIURL    string `json:"api_url"`
	APIToken  string `json:"api_token"`
	IsDefault bool   `json:"is_default"`
}

func (req trackerRequest) input() tracker.Input {
	return tracker.Input{
		Name:        req.Name,
		Announce:    req.Announce,
		Source:      req.Source,
		APIURL:      req.APIURL,
		APIToken:    req.APIToken,
		MakeDefault: req.IsDefault,
	}
}

func handleCreateTracker(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var req trackerRequest
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		t, err := d.Trackers.Create(r.Context(), user.ID, req.input())
		if err != nil {
			trackerError(w, d, err)
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, viewOf(t, profilesOf(r.Context(), d, user.ID)))
	}
}

func handleUpdateTracker(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var req trackerRequest
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		t, err := d.Trackers.Update(r.Context(), user.ID, r.PathValue("id"), req.input())
		if err != nil {
			trackerError(w, d, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, viewOf(t, profilesOf(r.Context(), d, user.ID)))
	}
}

func handleDeleteTracker(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		if err := d.Trackers.Delete(r.Context(), user.ID, r.PathValue("id")); err != nil {
			trackerError(w, d, err)
			return
		}
		// The profile follows the tracker. Best-effort: an orphan profile is inert (every read
		// goes through tracker ownership and existence), so it doesn't break the removal.
		if d.Profiles != nil {
			if err := d.Profiles.Forget(r.Context(), r.PathValue("id")); err != nil {
				d.Logger.Warn("remove tracker profile", "error", err)
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleSetDefaultTracker(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		if err := d.Trackers.SetDefault(r.Context(), user.ID, r.PathValue("id")); err != nil {
			trackerError(w, d, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleBrowseTracker lists a UNIT3D tracker catalog (ADR-020, backlog #12) and
// marks which items are already in the cluster (heuristic name cross-referencing — UNIT3D doesn't
// expose info_hash). Cursor-based pagination (?cursor=).
func handleBrowseTracker(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		sec, err := d.Trackers.ForPublish(r.Context(), user.ID, r.PathValue("id"))
		if err != nil {
			trackerError(w, d, err)
			return
		}
		if strings.TrimSpace(sec.APIURL) == "" || strings.TrimSpace(sec.APIToken) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "tracker_without_api", "this tracker has no API integration (api_url + token)")
			return
		}
		res, err := unit3d.Browse(r.Context(), sec.APIURL, sec.APIToken, r.URL.Query().Get("cursor"), r.URL.Query().Get("q"))
		if err != nil {
			d.Logger.Warn("list tracker catalog", "tracker", sec.Source, "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "browse_failed", err.Error())
			return
		}
		// Caches the download_link (server-side) for the seed to use without re-querying the tracker.
		d.Torrents.RememberTrackerLinks(r.PathValue("id"), res.Links)
		names, err := d.Torrents.ClusterNames(r.Context())
		if err != nil {
			d.Logger.Error("cluster names for cross-referencing", "error", err)
			names = nil // proceeds without cross-matching
		}

		// Parallel enrichment (#12): TMDB for movie/show, IGDB for game, Open Library
		// for book/audiobook. Silent failure per item — the card falls back to neutral mode
		// (no poster/PT title) instead of disappearing from the list.
		// The catalog is the site's: its metadata comes in the site's content language (#74).
		lang := contentLanguage(r.Context(), d, user.ID, r.PathValue("id"))
		enrichIn := make([]enrich.Item, len(res.Items))
		for i, it := range res.Items {
			enrichIn[i] = enrich.Item{
				Category: it.Category, Type: it.Type, Name: it.Name,
				// IGDB was returning empty here, which left the ID path dead in
				// enrich and always forced name search. The site almost always
				// sends `igdb_id: null`, but when it does the match is exact (#106).
				Year: it.Year, TMDB: it.TMDB, IGDB: it.IGDB,
				Lang: lang,
			}
		}
		results := d.Enrich.EnrichBatch(r.Context(), enrichIn)

		items := make([]map[string]any, 0, len(res.Items))
		for i, it := range res.Items {
			m := results[i]
			// Resolution: prefers the UNIT3D field (when populated); falls back to name parsing.
			resolution := strings.TrimSpace(it.Resolution)
			if resolution == "" {
				resolution = release.ParseResolution(it.Name)
			}
			items = append(items, map[string]any{
				"id": it.ID, "name": it.Name, "category": it.Category, "type": it.Type,
				"size": it.Size, "seeders": it.Seeders, "leechers": it.Leechers,
				"times_completed": it.TimesCompleted, "tmdb_id": it.TMDB, "imdb_id": it.IMDB,
				"year": it.Year, "details_link": it.DetailsLink,
				"in_cluster": torrent.NameInCluster(it.Name, names),
				// Enriched fields (all optional — the UI handles absence with a fallback).
				"media_type":        string(m.MediaType),
				"title_pt":          m.Title,
				"title_original":    m.OriginalName,
				"poster_url":        m.PosterURL,
				"overview":          m.Overview,
				"genres":            m.Genres,
				"original_language": m.OrigLanguage,
				"resolution":        resolution,
				"audio_langs":       release.ParseAudioLangs(it.Name),
				"subtitle_langs":    release.ParseSubtitleLangs(it.Name),
			})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": res.NextCursor})
	}
}

// seedFetchBudget is the deadline for talking to the tracker in the seed — the ADR-040 §4 budget,
// below the 30s WriteTimeout and the proxy's 60s on /api/. Without it, a tracker that hangs used
// up the http.Client's 60s and the operator got the proxy's 502 instead of the reason (#35). A variable
// only so the test can shorten it.
var seedFetchBudget = 25 * time.Second

// handleSeedFromTracker downloads the .torrent of an item from the tracker catalog and adds it
// to the cluster for seeding (ADR-020, backlog #12). Reuses the duplicate guard (#11): 409 if
// it already exists (the Portal confirms and resends with allow_duplicate).
func handleSeedFromTracker(d Deps) http.HandlerFunc {
	type request struct {
		TorrentID      string `json:"torrent_id"`
		AllowDuplicate bool   `json:"allow_duplicate"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if strings.TrimSpace(req.TorrentID) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "torrent_id is required")
			return
		}
		sec, err := d.Trackers.ForPublish(r.Context(), user.ID, r.PathValue("id"))
		if err != nil {
			trackerError(w, d, err)
			return
		}
		if strings.TrimSpace(sec.APIURL) == "" || strings.TrimSpace(sec.APIToken) == "" {
			httpx.WriteError(w, http.StatusBadRequest, "tracker_without_api", "this tracker has no API integration (api_url + token)")
			return
		}
		// Prefer the cached download_link from browse (the /api/torrents/{id} endpoint doesn't
		// always return the download_link); falls back to single-fetch if not cached.
		fetchCtx, cancel := context.WithTimeout(r.Context(), seedFetchBudget)
		var data []byte
		if link, ok := d.Torrents.TrackerLink(r.PathValue("id"), req.TorrentID); ok {
			data, err = unit3d.DownloadFromLink(fetchCtx, link, sec.APIToken)
		} else {
			data, err = unit3d.FetchTorrent(fetchCtx, sec.APIURL, sec.APIToken, req.TorrentID)
		}
		cancel()
		if err != nil {
			// torrent_id in the log: without it the line doesn't say WHICH torrent failed, and the
			// diagnosis starts without the most basic data (#207).
			d.Logger.Warn("download .torrent from tracker", "tracker", sec.Source,
				"torrent_id", req.TorrentID, "error", err)
			if errors.Is(err, unit3d.ErrTimeout) {
				httpx.WriteErrorParams(w, http.StatusGatewayTimeout, "tracker_timeout", fmt.Sprintf(
					"the tracker did not answer within %s — it may be down or overloaded; try again in a moment",
					seedFetchBudget), map[string]any{"seconds": int(seedFetchBudget.Seconds())})
				return
			}
			httpx.WriteError(w, http.StatusBadGateway, "fetch_failed", err.Error())
			return
		}
		t, err := d.Torrents.AddFile(r.Context(), data, req.AllowDuplicate)
		var dup *torrent.DuplicateError
		switch {
		case errors.As(err, &dup):
			httpx.WriteJSON(w, http.StatusConflict, map[string]any{
				"error": map[string]any{"code": "duplicate", "message": dup.Error(), "nodes": dup.Nodes, "existing": dup.Existing},
			})
		case errors.Is(err, scheduler.ErrNoEligibleNode):
			httpx.WriteErrorParams(w, http.StatusConflict, "no_eligible_node", noEligibleNodeMessage(err), noEligibleNodeParams(err))
		case errors.Is(err, scheduler.ErrNoNodeWithSpace):
			httpx.WriteErrorParams(w, http.StatusConflict, "no_node_with_space", noNodeWithSpaceMessage(err), noNodeWithSpaceParams(err))
		case err != nil:
			d.Logger.Error("seed from tracker", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("failed to add torrent: %s", err))
		default:
			// The audit says which torrent and which tracker: the route alone only carries the tracker
			// UUID. The name goes through AuditLabel, which never lets a passkey-like run through.
			var trackerName string
			_ = d.Pool.QueryRow(r.Context(), `SELECT name FROM trackers WHERE id = $1`, r.PathValue("id")).Scan(&trackerName)
			name := ""
			if t.Name != nil {
				name = *t.Name
			}
			params, text := seedAudit(name, trackerName)
			httpx.SetAudit(r.Context(), "torrent_seeded_from_tracker", params, text)
			httpx.WriteJSON(w, http.StatusCreated, t)
		}
	}
}
