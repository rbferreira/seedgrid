// Torrent lifecycle handlers: add, list, actions on the Engine
// (pause/resume/recheck), admission queue priority (F3), removal, torrent
// series, download via Portal (#1) and migration between Nodes (ADR-018/031).

package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/capacity"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// handleAddTorrent accepts a magnet (JSON) OR a .torrent file upload
// (multipart, "torrent" field). The Scheduler chooses the Node.
func handleAddTorrent(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			t   torrent.Torrent
			err error
		)
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			file, _, ferr := r.FormFile("torrent")
			if ferr != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "the .torrent file is required (field 'torrent')")
				return
			}
			defer func() { _ = file.Close() }()
			data, rerr := io.ReadAll(io.LimitReader(file, 5<<20)) // .torrent < 5 MB
			if rerr != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "failed to read the file")
				return
			}
			// allow_duplicate: the operator confirmed a 2nd copy (#11).
			allowDup := r.FormValue("allow_duplicate") == "true" || r.FormValue("allow_duplicate") == "1"
			t, err = d.Torrents.AddFile(r.Context(), data, allowDup)
		} else {
			var req struct {
				Magnet         string `json:"magnet"`
				Name           string `json:"name"`
				AllowDuplicate bool   `json:"allow_duplicate"`
			}
			if derr := httpx.DecodeJSON(r, &req); derr != nil {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
				return
			}
			if req.Magnet == "" {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "magnet or .torrent file is required")
				return
			}
			t, err = d.Torrents.Add(r.Context(), req.Magnet, req.Name, req.AllowDuplicate)
		}
		var dup *torrent.DuplicateError
		switch {
		case errors.As(err, &dup):
			// 409 with the Nodes that already have the torrent — the Portal confirms and resends.
			httpx.WriteJSON(w, http.StatusConflict, map[string]any{
				"error": map[string]any{
					"code":     "duplicate",
					"message":  dup.Error(),
					"nodes":    dup.Nodes,
					"existing": dup.Existing,
				},
			})
			return
		case errors.Is(err, scheduler.ErrNoEligibleNode):
			httpx.WriteErrorParams(w, http.StatusConflict, "no_eligible_node", noEligibleNodeMessage(err), noEligibleNodeParams(err))
		case errors.Is(err, scheduler.ErrNoNodeWithSpace):
			httpx.WriteErrorParams(w, http.StatusConflict, "no_node_with_space", noNodeWithSpaceMessage(err), noNodeWithSpaceParams(err))
			return
		case err != nil:
			d.Logger.Error("add torrent", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", fmt.Sprintf("failed to add torrent: %s", err))
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, t)
	}
}

// handleListTorrents lists torrents with conditional ETag and optional pagination (#115).
//
// ETag: the Portal retries this call every tick (8s) and most of the time NOTHING
// changed. The handler summarizes live state into (count, max(updated_at)); if the browser
// sends the same value in If-None-Match, it responds 304 without building the list — the
// Portal's fetch() gets the private cache body transparently, without any
// client changes. Any upsert/sync/removal changes the summary and invalidates the ETag.
//
// Pagination: optional ?limit= and ?offset= (absent = full list, previous
// contract). The response includes "total" for the client to paginate.
func handleListTorrents(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				offset = n
			}
		}
		total, maxUpdated, err := d.Torrents.ListMeta(r.Context())
		if err != nil {
			d.Logger.Error("torrent summary", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list torrents")
			return
		}
		etag := fmt.Sprintf(`W/"t-%d-%d-%d-%d"`, total, maxUpdated.UnixNano(), limit, offset)
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		torrents, err := d.Torrents.ListPage(r.Context(), limit, offset)
		if err != nil {
			d.Logger.Error("list torrents", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list torrents")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": torrents, "total": total})
	}
}

func handleTorrentSamples(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rng := 6 * time.Hour
		if v := r.URL.Query().Get("range"); v != "" {
			if d2, err := time.ParseDuration(v); err == nil && d2 > 0 {
				rng = d2
			}
		}
		samples, err := d.Torrents.Samples(r.Context(), r.PathValue("id"), rng)
		if err != nil {
			torrentActionError(w, d, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": samples})
	}
}

// handleDownloadTicket issues a one-time ticket for the browser to download the content
// of a torrent (#1). The ticket goes in the download GET URL (Bearer is not sent in a
// browser navigation). Protected route — only the authenticated operator issues tickets.
func handleDownloadTicket(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := httpx.OperatorFromContext(r.Context()); !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		ticket, err := d.Torrents.CreateDownloadTicket(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, torrent.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "torrent_not_found", "torrent not found")
		case errors.Is(err, torrent.ErrNoHash):
			httpx.WriteError(w, http.StatusConflict, "no_hash", "torrent has no hash yet; wait for synchronization")
		case err != nil:
			d.Logger.Error("create download ticket", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to prepare download")
		default:
			httpx.WriteJSON(w, http.StatusOK, map[string]any{
				"ticket": ticket,
				"url":    "/api/v1/torrents/download?ticket=" + url.QueryEscape(ticket),
			})
		}
	}
}

// handleDownload proxies/streams the torrent content to the browser (#1): it validates
// the ticket, authorizes the Node to serve and passes the stream (raw file for single-file,
// tar for folder). The Node is never exposed to the browser.
func handleDownload(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ticket := r.URL.Query().Get("ticket")
		if ticket == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "missing ticket")
			return
		}
		// Large streams (GBs): remove the read/write deadlines for this request.
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})

		body, ctype, disp, err := d.Torrents.OpenDownload(r.Context(), ticket)
		switch {
		case errors.Is(err, torrent.ErrTicketInvalid):
			httpx.WriteError(w, http.StatusForbidden, "invalid_ticket", "invalid or expired ticket")
			return
		case errors.Is(err, torrent.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "torrent_not_found", "torrent not found")
			return
		case errors.Is(err, torrent.ErrNoHash):
			httpx.WriteError(w, http.StatusConflict, "no_hash", "torrent has no hash yet")
			return
		case errors.Is(err, torrent.ErrNodeNotEligible):
			httpx.WriteError(w, http.StatusConflict, "node_not_eligible", "the torrent's Node is offline")
			return
		case err != nil:
			d.Logger.Error("open download", "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "download_failed", "failed to open content")
			return
		}
		defer func() { _ = body.Close() }()
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ctype)
		if disp != "" {
			w.Header().Set("Content-Disposition", disp)
		}
		_, _ = io.Copy(w, body)
	}
}

func torrentActionError(w http.ResponseWriter, d Deps, err error) {
	switch {
	case errors.Is(err, torrent.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "torrent_not_found", "torrent not found")
	case errors.Is(err, torrent.ErrNoHash):
		httpx.WriteError(w, http.StatusConflict, "no_hash", "torrent has no hash yet; wait for synchronization")
	case errors.Is(err, torrent.ErrMigrationInFlight):
		// The message says you can CANCEL the migration (and which one): without this the operator would
		// be stuck until it finishes, and a migration in recheck can take hours (#87).
		msg := "the torrent is migrating; wait for completion or cancel the migration"
		var inFlight *torrent.MigrationInFlightError
		if errors.As(err, &inFlight) {
			msg = inFlight.Error()
		}
		httpx.WriteError(w, http.StatusConflict, "migration_in_flight", msg)
	default:
		d.Logger.Error("torrent action", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to execute action")
	}
}

// torrentAuditDesc builds the friendly description of the action on a torrent (ADR-025),
// including the name when known: "Paused the torrent «X»" (or "…a torrent" without a name).
func torrentAuditDesc(verb, name string) string {
	if name == "" {
		return verb + " a torrent"
	}
	return verb + " torrent «" + name + "»"
}

func handleTorrentAction(d Deps, action string) http.HandlerFunc {
	verb := map[string]string{"pause": "Paused", "resume": "Resumed", "recheck": "Rechecked"}[action]
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cmdID, err := d.Torrents.Command(r.Context(), id, action)
		if err != nil {
			torrentActionError(w, d, err)
			return
		}
		if verb != "" {
			name := d.Torrents.Label(r.Context(), id)
			params, text := map[string]any{"name": name}, torrentAuditDesc(verb, name)
			switch action {
			case "pause":
				httpx.SetAudit(r.Context(), "torrent_paused", params, text)
			case "resume":
				httpx.SetAudit(r.Context(), "torrent_resumed", params, text)
			case "recheck":
				httpx.SetAudit(r.Context(), "torrent_rechecked", params, text)
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "command_id": cmdID})
	}
}

// handleTorrentPriority sets (on=true) or removes (on=false) the manual prioritization of a
// torrent in the admission queue (F3). Only changes state in the Control Plane; the admission
// controller applies the effect (skip the queue / protect in persistent mode) on the next pass.
func handleTorrentPriority(d Deps, on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		name := d.Torrents.Label(r.Context(), id) // before the UPDATE, for the audit description
		if err := d.Torrents.SetPriority(r.Context(), id, on); err != nil {
			torrentActionError(w, d, err)
			return
		}
		if on {
			httpx.SetAudit(r.Context(), "torrent_prioritized", map[string]any{"name": name}, torrentAuditDesc("Prioritized", name))
		} else {
			httpx.SetAudit(r.Context(), "torrent_unprioritized", map[string]any{"name": name}, torrentAuditDesc("Removed the priority of", name))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleTorrentRemove(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		name := d.Torrents.Label(r.Context(), id) // before Remove (soft-delete erases the record)
		actor := "admin"
		if user, ok := httpx.OperatorFromContext(r.Context()); ok && user != nil {
			actor = user.Username
		}
		cmdID, err := d.Torrents.Remove(r.Context(), id, actor)
		if err != nil {
			torrentActionError(w, d, err)
			return
		}
		httpx.SetAudit(r.Context(), "torrent_removed", map[string]any{"name": name}, torrentAuditDesc("Removed", name))
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "command_id": cmdID})
	}
}

// migrationError maps the migration service errors to HTTP responses.
func migrationError(w http.ResponseWriter, d Deps, err error) {
	switch {
	case errors.Is(err, migration.ErrTorrentNotFound):
		httpx.WriteError(w, http.StatusNotFound, "torrent_not_found", "torrent not found")
	case errors.Is(err, migration.ErrNoTorrentFile):
		httpx.WriteError(w, http.StatusConflict, "no_torrent_file", "torrent without .torrent persisted in the CP (wait for backfill)")
	case errors.Is(err, migration.ErrSameNode):
		httpx.WriteError(w, http.StatusBadRequest, "same_node", "destination Node is the same as the source")
	case errors.Is(err, migration.ErrDestNotEligible):
		httpx.WriteError(w, http.StatusConflict, "dest_not_eligible", "destination Node not found or offline")
	case errors.Is(err, migration.ErrSourceNoEndpoint):
		httpx.WriteError(w, http.StatusConflict, "source_no_endpoint", "source Node has no known endpoint for transfer")
	case errors.Is(err, migration.ErrAlreadyMigrating):
		httpx.WriteError(w, http.StatusConflict, "already_migrating", "a migration is already in progress for this torrent")
	case errors.Is(err, migration.ErrDestNoSpace):
		httpx.WriteError(w, http.StatusConflict, "dest_no_space", "destination Node does not have disk space for this torrent")
	default:
		d.Logger.Error("migration", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to start migration")
	}
}

// migrateAuditDesc builds the friendly description of a migration (ADR-025): torrent +
// source Node + target Node, with "a torrent"/"a Node" when the name is
// unknown (torrent removed between the query and the event, for example).
func migrateAuditDesc(torrentName, sourceNode, destNode string) string {
	torrent := "a torrent"
	if torrentName != "" {
		torrent = "torrent «" + torrentName + "»"
	}
	source := "from a Node"
	if sourceNode != "" {
		source = "from Node «" + sourceNode + "»"
	}
	dest := "a Node"
	if destNode != "" {
		dest = "Node «" + destNode + "»"
	}
	return "Migrated " + torrent + " " + source + " to " + dest
}

// handleTorrentMigrate migrates a torrent to the target Node (ADR-018).
func handleTorrentMigrate(d Deps) http.HandlerFunc {
	type request struct {
		DestNodeID string `json:"dest_node_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil || req.DestNodeID == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "dest_node_id is required")
			return
		}
		m, err := d.Migrations.Migrate(r.Context(), r.PathValue("id"), req.DestNodeID)
		if err != nil {
			migrationError(w, d, err)
			return
		}
		tName, src, dst := d.Torrents.Label(r.Context(), m.TorrentID), d.Nodes.Label(r.Context(), m.SourceNodeID), d.Nodes.Label(r.Context(), m.DestNodeID)
		httpx.SetAudit(r.Context(), "torrent_migrated", map[string]any{"name": tName, "source": src, "dest": dst},
			migrateAuditDesc(tName, src, dst))
		httpx.WriteJSON(w, http.StatusCreated, m)
	}
}

// handleNodeDrain migrates all torrents from a Node to the target (drain — a clean
// step before removing a Node). Torrents without a .torrent are still skipped.
func handleNodeDrain(d Deps) http.HandlerFunc {
	type request struct {
		DestNodeID string `json:"dest_node_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil || req.DestNodeID == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "dest_node_id is required")
			return
		}
		sourceNodeID := r.PathValue("id")
		migs, err := d.Migrations.DrainNode(r.Context(), sourceNodeID, req.DestNodeID)
		if err != nil {
			migrationError(w, d, err)
			return
		}
		src, dst := d.Nodes.Label(r.Context(), sourceNodeID), d.Nodes.Label(r.Context(), req.DestNodeID)
		httpx.SetAudit(r.Context(), "node_drained", map[string]any{"source": src, "count": len(migs), "dest": dst},
			fmt.Sprintf("Drained Node «%s» (%s) to Node «%s»",
				labelOrFallback(src, "unknown"), pluralTorrents(len(migs)), labelOrFallback(dst, "unknown")))
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"data": migs, "count": len(migs)})
	}
}

// handleMigrationCancel cancels an in-flight migration (#87): the same safe shutdown as the
// watchdog — source resumed, target copy discarded. The operator's way out for a
// torrent that actions reject (409 migration_in_flight) while the migration lasts.
func handleMigrationCancel(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m, err := d.Migrations.Cancel(r.Context(), r.PathValue("id"))
		switch {
		case errors.Is(err, migration.ErrMigrationNotFound):
			httpx.WriteError(w, http.StatusNotFound, "migration_not_found", "migration not found")
			return
		case errors.Is(err, migration.ErrMigrationAlreadyCancelled):
			httpx.WriteError(w, http.StatusConflict, "migration_already_cancelled", "migration has already been cancelled")
			return
		case errors.Is(err, migration.ErrMigrationFinished):
			httpx.WriteError(w, http.StatusConflict, "migration_finished",
				"migration has already finished ("+migrationOutcome(m)+"); nothing to cancel")
			return
		case err != nil:
			d.Logger.Error("cancel migration", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to cancel migration")
			return
		}
		tName := d.Torrents.Label(r.Context(), m.TorrentID)
		src, dst := d.Nodes.Label(r.Context(), m.SourceNodeID), d.Nodes.Label(r.Context(), m.DestNodeID)
		httpx.SetAudit(r.Context(), "migration_cancelled", map[string]any{"name": tName, "source": src, "dest": dst},
			"Cancelled the migration of torrent «"+labelOrFallback(tName, "unknown")+"» from Node «"+
				labelOrFallback(src, "unknown")+"» to Node «"+labelOrFallback(dst, "unknown")+"»")
		httpx.WriteJSON(w, http.StatusOK, m)
	}
}

// migrationOutcome describes in PT-BR how a finished migration ended.
func migrationOutcome(m migration.Migration) string {
	if m.Status == "COMPLETED" {
		return "completed"
	}
	return "failed"
}

// labelOrFallback returns the label, or fallback when the label was not found
// (e.g., Node removed between the action and the audit description).
func labelOrFallback(label, fallback string) string {
	if label == "" {
		return fallback
	}
	return label
}

// pluralTorrents returns "1 torrent" or "N torrents" for the drain audit description.
func pluralTorrents(n int) string {
	if n == 1 {
		return "1 torrent"
	}
	return fmt.Sprintf("%d torrents", n)
}

// handleListMigrations lists recent migrations.
func handleListMigrations(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := queryLimit(r, 100, 500)
		migs, err := d.Migrations.List(r.Context(), limit)
		if err != nil {
			d.Logger.Error("list migrations", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list migrations")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": migs})
	}
}

// noEligibleNodeMessage explains WHY there was no eligible Node.
//
// "no eligible Seed Node at the moment" with all Nodes green in the Portal is a dead
// end: the operator has no way to connect one thing to another. When the cause is failed
// storage, the Scheduler now says which Nodes went down (#180) — and it's the only time the
// exclusion costs anything, which is why it appears here.
func noEligibleNodeMessage(err error) string {
	var noNode *scheduler.NoEligibleNodeError
	if errors.As(err, &noNode) && len(noNode.BlockedByStorage) > 0 {
		return noNode.Error() + " — check the Node's disk before trying again"
	}
	return "no eligible Seed Node at the moment"
}

// noNodeWithSpaceParams: the space refusal numbers, for the Portal to build the sentence in
// the reader's language (error.params). nil when the error does not include the numbers.
func noNodeWithSpaceParams(err error) map[string]any {
	var noSpace *scheduler.NoNodeWithSpaceError
	if !errors.As(err, &noSpace) {
		return nil
	}
	return map[string]any{"need_bytes": noSpace.NeedBytes, "min_free_bytes": noSpace.MinFreeBytes, "best_free_bytes": noSpace.BestFreeBytes}
}

// noEligibleNodeParams: the Nodes out due to failed storage (error.params); nil when there are none.
func noEligibleNodeParams(err error) map[string]any {
	var noNode *scheduler.NoEligibleNodeError
	if !errors.As(err, &noNode) || len(noNode.BlockedByStorage) == 0 {
		return nil
	}
	return map[string]any{"nodes": noNode.BlockedByStorage}
}

// noNodeWithSpaceMessage explains the space refusal with the numbers and the way out.
//
// What blocks is rarely the torrent not fitting: it's the free disk margin (ADR-032), which
// the operator doesn't see on the add screen. With the three numbers they decide alone — free
// up space, migrate, or, on a small disk Node, lower the margin (#22).
func noNodeWithSpaceMessage(err error) string {
	var noSpace *scheduler.NoNodeWithSpaceError
	if !errors.As(err, &noSpace) {
		return "no Seed Node with disk space for this torrent — free up space or migrate torrents between Nodes"
	}
	return fmt.Sprintf("no Seed Node can accommodate this torrent: it is %s and the free disk margin is %s, but the Node with the most free space has %s free. "+
		"Free up space, migrate torrents between Nodes, or adjust \"Free disk margin\" in Settings",
		capacity.HumanBytes(noSpace.NeedBytes), capacity.HumanBytes(noSpace.MinFreeBytes), capacity.HumanBytes(noSpace.BestFreeBytes))
}
