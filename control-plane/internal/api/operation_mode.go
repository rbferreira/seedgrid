package api

import (
	"errors"
	"net/http"

	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

// handleSetClusterMode adjusts the global operation gate (Full auto / Flex) — ADR-024.
// The service emits the audit event (CLUSTER_MODE_UPDATED) and, upon returning to full_auto,
// resets the per-Node modes to SG.
func handleSetClusterMode(d Deps) http.HandlerFunc {
	type request struct {
		Mode string `json:"mode"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if err := d.Nodes.SetGate(r.Context(), req.Mode); err != nil {
			if errors.Is(err, node.ErrInvalidMode) {
				httpx.WriteError(w, http.StatusBadRequest, "invalid_mode", "mode must be full_auto or flex")
				return
			}
			d.Logger.Error("change Cluster gate", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to change the Cluster mode")
			return
		}
		label := "Full auto"
		if req.Mode == node.GateFlex {
			label = "Flex"
		}
		httpx.SetAudit(r.Context(), "cluster_mode_changed", map[string]any{"mode": req.Mode}, "Set Cluster mode to "+label+" (admission)")
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"operation_mode": req.Mode})
	}
}

// handleSetNodeMode adjusts the per-Node mode (SG / Manual) — only allowed with the cluster
// in Flex (in Full auto the toggles are locked — ADR-024).
func handleSetNodeMode(d Deps) http.HandlerFunc {
	type request struct {
		Mode string `json:"mode"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		err := d.Nodes.SetNodeMode(r.Context(), r.PathValue("id"), req.Mode)
		switch {
		case errors.Is(err, node.ErrInvalidMode):
			httpx.WriteError(w, http.StatusBadRequest, "invalid_mode", "mode must be sg or manual")
		case errors.Is(err, node.ErrGateLocked):
			httpx.WriteError(w, http.StatusConflict, "gate_locked", "the Cluster must be in Flex to set the per-Node mode")
		case errors.Is(err, node.ErrNodeNotFound):
			httpx.WriteError(w, http.StatusNotFound, "node_not_found", "node not found")
		case err != nil:
			d.Logger.Error("change node mode", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to change the Node mode")
		default:
			label := "SG (automatic)"
			if req.Mode == node.NodeModeManual {
				label = "Manual"
			}
			httpx.SetAudit(r.Context(), "node_mode_changed", map[string]any{"mode": req.Mode}, "Set a Node to mode "+label)
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"operation_mode": req.Mode})
		}
	}
}
