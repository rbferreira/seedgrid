// Handlers for the Agent API (ADR-012: communication initiated by the Agent) and
// Node commands: register/enroll (ADR-017), heartbeat, command queue
// (ADR-014), torrent inventory, .torrent backfill (ADR-018) and the
// Agent self-update (ADR-027).

package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/agentconfig"
	"github.com/rbferreira/seedgrid/control-plane/internal/agentrelease"
	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// enrollTokenTTL is the validity of a one-time join token (ADR-017).
const enrollTokenTTL = time.Hour

func handleAgentRegister(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Auth: shared token OR one-time join token (ADR-017).
		tok := r.Header.Get(httpx.AgentRegistrationHeader)
		shared := d.Auth.AgentRegistrationTokenValid(tok)
		var enroll node.EnrollTokenInfo
		if !shared {
			info, ok := d.Nodes.EnrollToken(r.Context(), tok)
			if !ok {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid registration token")
				return
			}
			enroll = info
		}

		var in node.RegisterInput
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if in.Hostname == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "hostname is required")
			return
		}
		in.BTPort = enroll.BTPort // port assigned by the CP via join token (nil in the shared path)
		if !shared {
			// The alias is the OPERATOR's decision, made when issuing the join token (#145) — what the
			// Agent sends in the body doesn't count. In the legacy path (shared token) there is no
			// token to query, and the body value remains as it was (Register validates it).
			in.DisplayName = enroll.DisplayName
		}

		res, err := d.Nodes.Register(r.Context(), in)
		if err != nil {
			d.Logger.Error("registrar agent", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to register node")
			return
		}
		if !shared {
			d.Nodes.MarkEnrollTokenUsed(r.Context(), tok) // consumes the join token after registration
		}
		httpx.WriteJSON(w, http.StatusCreated, res)
	}
}

// handleMintEnrollToken (operator): issues a one-time join token for onboarding. The body
// is optional: {"display_name": "..."} defines the alias the Node will have in SeedGrid (#145);
// it travels in the token and is applied when the Node registers.
func handleMintEnrollToken(d Deps) http.HandlerFunc {
	type request struct {
		DisplayName string `json:"display_name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		// Empty body is the original usage (no alias), so EOF is not an error.
		if err := httpx.DecodeJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		name, err := node.NormalizeDisplayName(req.DisplayName)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_display_name", err.Error())
			return
		}
		token, expiresAt, err := d.Nodes.MintEnrollToken(r.Context(), enrollTokenTTL, name)
		switch {
		case errors.Is(err, node.ErrInvalidDisplayName):
			httpx.WriteError(w, http.StatusBadRequest, "invalid_display_name", err.Error())
			return
		case errors.Is(err, node.ErrDisplayNameTaken):
			httpx.WriteError(w, http.StatusConflict, "display_name_taken", err.Error())
			return
		case err != nil:
			d.Logger.Error("issue join token", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to issue join token")
			return
		}
		if name != "" {
			httpx.SetAudit(r.Context(), "enroll_token_created", map[string]any{"name": name}, "Generated a join token for Node «"+name+"»")
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"token":        token,
			"expires_at":   expiresAt,
			"display_name": name,
		})
	}
}

// handleAgentEnroll (public, the join token in the body authenticates): validates the token and
// returns the BT port assigned by the CP (ADR-017) and the Node index in the port
// sequence (`node_index`, the origin of the deterministic IPv6 suffix — ADR-038). Called by the
// Node bootstrap before starting the Torrent Engine.
func handleAgentEnroll(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if err := httpx.DecodeJSON(r, &body); err != nil || body.Token == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "token is required")
			return
		}
		port, err := d.Nodes.AssignPort(r.Context(), body.Token)
		if errors.Is(err, node.ErrInvalidEnrollToken) {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid, expired or already used join token")
			return
		}
		if err != nil {
			d.Logger.Error("node enroll", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "enroll failed")
			return
		}
		resp := map[string]any{"bt_port": port}
		if idx, ok := node.IndexForPort(port); ok {
			resp["node_index"] = idx
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

func handleAgentHeartbeat(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := httpx.NodeIDFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated Agent")
			return
		}
		var in node.HeartbeatInput
		if err := httpx.DecodeJSON(r, &in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		in.SourceIP = httpx.ClientIP(r) // Node IP as seen by the CP (the operator uses it for port forwarding)
		if err := d.Nodes.Heartbeat(r.Context(), nodeID, in); err != nil {
			d.Logger.Error("heartbeat", "error", err, "node_id", nodeID)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to process heartbeat")
			return
		}
		resp := map[string]any{"status": "ok"}
		// Agent/Engine behavior configuration (ADR-039): the heartbeat response
		// carries the EFFECTIVE values for this Node. NEVER drops the heartbeat — the Node was already
		// registered as alive above; if resolution fails, the response goes out without the block and the
		// Agent keeps what it already applied (or the env, on boot).
		if d.AgentConfig != nil {
			ac, err := d.AgentConfig.Exchange(r.Context(), nodeID, agentconfig.Report{
				Version: in.ConfigVersion,
				Local:   in.LocalConfig,
			})
			if err != nil {
				d.Logger.Warn("resolve Agent configuration", "error", err, "node_id", nodeID)
			} else {
				resp["agent_config"] = ac
			}
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	}
}

func handleAgentCommands(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := httpx.NodeIDFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated Agent")
			return
		}
		cmds, err := d.Commands.Claim(r.Context(), nodeID, 50)
		if err != nil {
			d.Logger.Error("claim commands", "error", err, "node_id", nodeID)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get commands")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": cmds})
	}
}

func handleAgentCommandResult(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := httpx.NodeIDFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated Agent")
			return
		}
		var res command.Result
		if err := httpx.DecodeJSON(r, &res); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		err := d.Commands.SubmitResult(r.Context(), nodeID, r.PathValue("id"), res)
		if err == command.ErrNotFound {
			httpx.WriteError(w, http.StatusNotFound, "command_not_found", "command not found")
			return
		}
		if err != nil {
			d.Logger.Error("submit result", "error", err, "node_id", nodeID)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to save result")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleEnqueueCommand(d Deps) http.HandlerFunc {
	type request struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if req.Type == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "type is required")
			return
		}
		id, err := d.Commands.Enqueue(r.Context(), r.PathValue("id"), req.Type, req.Payload)
		if err != nil {
			d.Logger.Error("enqueue command", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to enqueue command")
			return
		}
		httpx.WriteJSON(w, http.StatusCreated, map[string]string{"command_id": id})
	}
}

func handleAgentTorrents(d Deps) http.HandlerFunc {
	type request struct {
		Torrents []torrent.Report `json:"torrents"`
		// DLSlotLimit is the limit of DL slots per account that the Agent extracted from the tracker
		// message (#21 — UNIT3D error 164). Optional; absent when not observed.
		DLSlotLimit *int `json:"dl_slot_limit,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := httpx.NodeIDFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated Agent")
			return
		}
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if req.DLSlotLimit != nil && *req.DLSlotLimit > 0 {
			// Best-effort: registering the observed L should not fail the state sync.
			if err := d.Trackers.ObserveDLSlotLimit(r.Context(), *req.DLSlotLimit); err != nil {
				d.Logger.Warn("record observed DL slot limit", "error", err, "node_id", nodeID)
			}
		}
		if err := d.Torrents.SyncStates(r.Context(), nodeID, req.Torrents); err != nil {
			d.Logger.Error("sync torrents", "error", err, "node_id", nodeID)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to sync torrents")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleAgentPendingTorrentFiles returns the hashes of torrents from this Node whose
// .torrent is not yet persisted in the CP — the Agent exports from the engine and sends
// it back (backfill, ADR-018), making the torrent migratable.
func handleAgentPendingTorrentFiles(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := httpx.NodeIDFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated Agent")
			return
		}
		hashes, err := d.Torrents.HashesNeedingFile(r.Context(), nodeID)
		if err != nil {
			d.Logger.Error("pending .torrent hashes", "error", err, "node_id", nodeID)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get pending items")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": hashes})
	}
}

// handleAgentUploadTorrentFile persists the .torrent exported by the Agent (base64),
// resolved by (node, hash) (backfill, ADR-018).
func handleAgentUploadTorrentFile(d Deps) http.HandlerFunc {
	type request struct {
		Hash string `json:"hash"`
		Data string `json:"data"` // .torrent em base64
	}
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := httpx.NodeIDFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated Agent")
			return
		}
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if req.Hash == "" || req.Data == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "hash and data are required")
			return
		}
		data, err := base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid base64 data")
			return
		}
		err = d.Torrents.StoreTorrentFileByHash(r.Context(), nodeID, req.Hash, data)
		if errors.Is(err, torrent.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "torrent_not_on_node", "torrent not found on this Node")
			return
		}
		if err != nil {
			d.Logger.Error("persist exported .torrent", "error", err, "node_id", nodeID)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to persist .torrent")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleAgentAvailable returns the published Agent version and the covered architectures
// (ADR-027) — the Portal compares it with the agent_version of each Node for the "update
// available" badge. 503 when `agent.artifacts_url` is not configured: the feature is
// disabled, not a server error.
func handleAgentAvailable(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AgentRelease == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "agent_update_unavailable", "no Agent update configured")
			return
		}
		rel, err := d.AgentRelease.Available(r.Context())
		if errors.Is(err, agentrelease.ErrNotConfigured) {
			httpx.WriteError(w, http.StatusServiceUnavailable, "agent_artifacts_url_missing",
				"set agent.artifacts_url in Settings to enable Agent updates")
			return
		}
		if err != nil {
			d.Logger.Error("read Agent manifest", "error", err)
			httpx.WriteError(w, http.StatusBadGateway, "manifest_error", "failed to read artifacts manifest")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, rel)
	}
}

// handleUpdateAgent queues the Agent update for a Node (ADR-027): the CP resolves the
// binary for that Node's ARCHITECTURE and sends url+sha256; the Agent downloads, validates, proves it
// runs and swaps. Rollout is manual and per Node — nothing moves on its own.
func handleUpdateAgent(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AgentRelease == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "agent_update_unavailable", "no Agent update configured")
			return
		}
		nodeID := r.PathValue("id")
		var arch, status, current string
		if err := d.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(architecture,''), COALESCE(status,''), COALESCE(agent_version,'')
			   FROM nodes WHERE id = $1 AND deleted_at IS NULL`, nodeID).Scan(&arch, &status, &current); err != nil {
			httpx.WriteError(w, http.StatusNotFound, "node_not_found", "node not found")
			return
		}
		// Without a known architecture there is no right binary to send — and sending the wrong one
		// brings down the Node (the case ADR-027 exists to prevent).
		if arch == "" {
			httpx.WriteError(w, http.StatusConflict, "unknown_arch",
				"unknown Node architecture (wait for the next Agent inventory)")
			return
		}
		target, err := d.AgentRelease.TargetFor(r.Context(), arch)
		if errors.Is(err, agentrelease.ErrNotConfigured) {
			httpx.WriteError(w, http.StatusServiceUnavailable, "agent_artifacts_url_missing",
				"set agent.artifacts_url in Settings to enable Agent updates")
			return
		}
		if err != nil {
			d.Logger.Error("resolve Agent binary", "error", err, "arch", arch)
			httpx.WriteError(w, http.StatusConflict, "no_binary", err.Error())
			return
		}
		if current != "" && current == target.Version {
			httpx.WriteError(w, http.StatusConflict, "already_current",
				"the Node is already on the published version ("+target.Version+")")
			return
		}
		payload, _ := json.Marshal(map[string]string{
			"version": target.Version, "url": target.URL, "sha256": target.SHA256,
		})
		cmdID, err := d.Commands.Enqueue(r.Context(), nodeID, "update_agent", payload)
		if err != nil {
			d.Logger.Error("enqueue update_agent", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to request update")
			return
		}
		// Node OFFLINE is not an error: the queue is durable (ADR-014) and it executes when it comes back online.
		if d.Events != nil {
			_ = d.Events.Emit(r.Context(), "AGENT_UPDATE", "INFO", nodeID,
				"Agent update requested ("+current+" → "+target.Version+")",
				map[string]any{"hostname": d.Nodes.Label(r.Context(), nodeID), "from": current, "to": target.Version})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "command_id": cmdID, "version": target.Version,
		})
	}
}

// handleRestartEngine queues the restart of the Node's Torrent Engine (the Agent
// requests shutdown from qBittorrent; the container's restart policy brings it back).
func handleRestartEngine(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID := r.PathValue("id")
		var exists bool
		if err := d.Pool.QueryRow(r.Context(),
			`SELECT true FROM nodes WHERE id = $1 AND deleted_at IS NULL`, nodeID).Scan(&exists); err != nil {
			httpx.WriteError(w, http.StatusNotFound, "node_not_found", "node not found")
			return
		}
		cmdID, err := d.Commands.Enqueue(r.Context(), nodeID, "restart_engine", nil)
		if err != nil {
			d.Logger.Error("enqueue restart_engine", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to request restart")
			return
		}
		if d.Events != nil {
			_ = d.Events.Emit(r.Context(), "ENGINE_RESTART", "WARNING", nodeID, "Torrent Engine restart requested",
				map[string]any{"hostname": d.Nodes.Label(r.Context(), nodeID)})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "command_id": cmdID})
	}
}

// handleGetCommand returns the state of a command (for the Portal to display progress).
func handleGetCommand(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := d.Commands.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, command.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "command_not_found", "command not found")
			return
		}
		if err != nil {
			d.Logger.Error("query command", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to query command")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, st)
	}
}
