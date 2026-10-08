package api

import (
	"errors"
	"net/http"

	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

// handleRenameNode sets (or removes) a Node's nickname — #145. The nickname lives only in
// SeedGrid: nothing is sent to the Node, the OS hostname doesn't change and the Agent doesn't restart.
//
// The field is required: "" removes the nickname (the Portal falls back to showing the hostname), but a
// body without `display_name` is an error — deleting by omission would be too easy to do by accident.
func handleRenameNode(d Deps) http.HandlerFunc {
	type request struct {
		DisplayName *string `json:"display_name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil || req.DisplayName == nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "display_name is required (\"\" removes the nickname)")
			return
		}
		res, err := d.Nodes.SetDisplayName(r.Context(), r.PathValue("id"), *req.DisplayName)
		switch {
		case errors.Is(err, node.ErrInvalidDisplayName):
			httpx.WriteError(w, http.StatusBadRequest, "invalid_display_name", err.Error())
		case errors.Is(err, node.ErrDisplayNameTaken):
			httpx.WriteError(w, http.StatusConflict, "display_name_taken", err.Error())
		case errors.Is(err, node.ErrNodeNotFound):
			httpx.WriteError(w, http.StatusNotFound, "node_not_found", "node not found")
		case err != nil:
			d.Logger.Error("rename Node", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to rename the Node")
		default:
			// Operator action → Audit (ADR-025), not Events: same criterion as the per-Node
			// mode. The description carries the before and after, because that's all the trail
			// stores — the old value no longer exists anywhere after the UPDATE.
			httpx.SetAudit(r.Context(), "node_renamed", renameAuditParams(res), renameAuditText(res))
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"display_name": res.After})
		}
	}
}

// renameAuditText describes the nickname change for the audit trail.
// renameAuditParams: the data for the nickname phrase in the trail (#57); change picks the variant.
func renameAuditParams(res node.RenameResult) map[string]any {
	change := "renamed"
	switch {
	case res.After == "" && res.Before == "":
		change = "none"
	case res.After == "":
		change = "removed"
	case res.Before == "":
		change = "set"
	}
	return map[string]any{"change": change, "hostname": res.Hostname, "before": res.Before, "after": res.After}
}

func renameAuditText(res node.RenameResult) string {
	switch {
	case res.After == "" && res.Before == "":
		return "Removed the nickname of Node «" + res.Hostname + "» (it had none)"
	case res.After == "":
		return "Removed the nickname «" + res.Before + "» from Node «" + res.Hostname + "»"
	case res.Before == "":
		return "Gave Node «" + res.Hostname + "» the nickname «" + res.After + "»"
	default:
		return "Renamed Node «" + res.Before + "» to «" + res.After + "»"
	}
}
