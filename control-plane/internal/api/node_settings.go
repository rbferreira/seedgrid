package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/agentconfig"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// Agent/Engine behavior configuration per Node (ADR-039, #144): the value of
// CLUSTER is from the catalog (/settings routes, ADR-023, settings:write permission); here live
// the per-Node override (nodes:write) and the "what applies to this Node, and where it comes from" view.

// nodeSettingItem is a PerNode key resolved for a Node: catalog metadata + effective
// value + origin + what the Node would have without an override.
type nodeSettingItem struct {
	Key           string   `json:"key"`
	Type          string   `json:"type"`
	Label         string   `json:"label"`
	Description   string   `json:"description"`
	Unit          string   `json:"unit"`
	Min           *float64 `json:"min"`
	Max           *float64 `json:"max"`
	Options       []string `json:"options,omitempty"`
	Value         string   `json:"value"`          // effective
	Source        string   `json:"source"`         // "node" | "cluster"
	ClusterValue  string   `json:"cluster_value"`  // what the Node would have without an override
	ClusterSource string   `json:"cluster_source"` // "db" (the operator defined it) | "default" (catalog)
	Override      *string  `json:"override"`       // the Node's override; null = none exists
}

func toNodeSettingItem(it agentconfig.Item) nodeSettingItem {
	clusterSource := "default"
	if it.ClusterExplicit {
		clusterSource = "db"
	}
	return nodeSettingItem{
		Key: it.Spec.Key, Type: string(it.Spec.Kind), Label: it.Spec.Label, Description: it.Spec.Description,
		Unit: it.Spec.Unit, Min: it.Spec.Min, Max: it.Spec.Max, Options: it.Spec.Options,
		Value: it.Value, Source: it.Source, ClusterValue: it.ClusterValue, ClusterSource: clusterSource,
		Override: it.Override,
	}
}

// itemByKey locates a key in a Node's state.
func itemByKey(st agentconfig.NodeState, key string) (nodeSettingItem, bool) {
	for _, it := range st.Items {
		if it.Spec.Key == key {
			return toNodeSettingItem(it), true
		}
	}
	return nodeSettingItem{}, false
}

// writeNodeSettingError translates errors from the agentconfig package. Returns true if handled.
func writeNodeSettingError(d Deps, w http.ResponseWriter, err error, what string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, agentconfig.ErrNodeNotFound):
		httpx.WriteError(w, http.StatusNotFound, "node_not_found", "node not found")
	case errors.Is(err, agentconfig.ErrUnknownKey):
		httpx.WriteError(w, http.StatusNotFound, "setting_not_found", "unknown tunable")
	case errors.Is(err, agentconfig.ErrNotPerNode):
		httpx.WriteError(w, http.StatusBadRequest, "not_per_node", "this tunable does not accept per-Node values")
	default:
		d.Logger.Error(what, "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to "+what)
	}
	return true
}

// handleGetNodeSettings (GET /nodes/{id}/settings): the Node's effective configuration, where
// each value comes from, and if the Agent is up to date. It's read-only data: applies to whoever reads Nodes.
func handleGetNodeSettings(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AgentConfig == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "node_config_unavailable", "per-Node configuration unavailable")
			return
		}
		st, err := d.AgentConfig.Node(r.Context(), r.PathValue("id"))
		if writeNodeSettingError(d, w, err, "read the Node configuration") {
			return
		}
		items := make([]nodeSettingItem, 0, len(st.Items))
		for _, it := range st.Items {
			items = append(items, toNodeSettingItem(it))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"node_id":           st.NodeID,
			"status":            st.Status(),
			"effective_version": st.EffectiveVersion,
			"applied_version":   st.AppliedVersion,
			"applied_at":        st.AppliedAt,
			"seeded_at":         st.SeededAt,
			"items":             items,
		})
	}
}

// handleSetNodeSetting (PUT /nodes/{id}/settings/{key}) writes the Node's override. The value is
// validated by the SAME catalog Spec (type, range, options). It takes effect on the next Agent
// heartbeat, without a restart.
func handleSetNodeSetting(d Deps) http.HandlerFunc {
	type request struct {
		Value string `json:"value"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AgentConfig == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "node_config_unavailable", "per-Node configuration unavailable")
			return
		}
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		user, _ := httpx.OperatorFromContext(r.Context())
		actor := "operator"
		if user != nil {
			actor = user.Username
		}
		nodeID, key := r.PathValue("id"), r.PathValue("key")
		change, err := d.AgentConfig.SetOverride(r.Context(), nodeID, key, req.Value, actor)
		var invalid *agentconfig.InvalidValueError
		if errors.As(err, &invalid) {
			// The catalog Spec rejected the value (type, range, option): the same cluster rule.
			httpx.WriteError(w, http.StatusBadRequest, "invalid_value", invalid.Error())
			return
		}
		if writeNodeSettingError(d, w, err, "write the Node configuration") {
			return
		}
		httpx.SetAudit(r.Context(), "node_override_set", overrideAuditParams(change), setOverrideAuditText(change))
		st, err := d.AgentConfig.Node(r.Context(), nodeID)
		if writeNodeSettingError(d, w, err, "read the Node configuration") {
			return
		}
		it, _ := itemByKey(st, key)
		httpx.WriteJSON(w, http.StatusOK, it)
	}
}

// handleResetNodeSetting (DELETE /nodes/{id}/settings/{key}) removes the override: the Node reverts
// to the cluster value.
func handleResetNodeSetting(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AgentConfig == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "node_config_unavailable", "per-Node configuration unavailable")
			return
		}
		nodeID, key := r.PathValue("id"), r.PathValue("key")
		change, err := d.AgentConfig.ClearOverride(r.Context(), nodeID, key)
		if writeNodeSettingError(d, w, err, "remove the Node configuration") {
			return
		}
		httpx.SetAudit(r.Context(), "node_override_cleared", overrideAuditParams(change), clearOverrideAuditText(change))
		st, err := d.AgentConfig.Node(r.Context(), nodeID)
		if writeNodeSettingError(d, w, err, "read the Node configuration") {
			return
		}
		it, _ := itemByKey(st, key)
		httpx.WriteJSON(w, http.StatusOK, it)
	}
}

// withUnit appends the catalog unit to the value ("3" → "3 KB/s"), as the cluster value
// Audit already does.
func withUnit(spec settings.Spec, v string) string {
	if spec.Unit != "" {
		return v + " " + spec.Unit
	}
	return v
}

// setOverrideAuditText describes the change with before and after — that's all the trail
// keeps, the old value no longer exists anywhere after the upsert.
// overrideAuditParams: the data for the custom value sentence of a Node in the trail (#57) — the Portal
// builds the sentence in the reader's language; the variant comes from had_before (had custom value before).
func overrideAuditParams(c agentconfig.Change) map[string]any {
	p := map[string]any{"key": c.Spec.Key, "label": c.Spec.Label, "node": c.NodeLabel, "had_before": c.Before != nil,
		"effective": withUnit(c.Spec, c.Effective)}
	if c.After != nil {
		p["value"] = withUnit(c.Spec, *c.After)
	}
	if c.Before != nil {
		p["before"] = withUnit(c.Spec, *c.Before)
	}
	return p
}

func setOverrideAuditText(c agentconfig.Change) string {
	base := "Set «" + c.Spec.Label + "» on Node «" + c.NodeLabel + "» to " + withUnit(c.Spec, *c.After)
	if c.Before != nil {
		return base + " (was " + withUnit(c.Spec, *c.Before) + ")"
	}
	return base + " (it followed the cluster before)"
}

func clearOverrideAuditText(c agentconfig.Change) string {
	base := "Removed the Node-specific value of «" + c.Spec.Label + "» from Node «" + c.NodeLabel + "»"
	if c.Before == nil {
		return base + " (it already followed the cluster)"
	}
	return base + " (was " + withUnit(c.Spec, *c.Before) + "; back to the cluster value: " + withUnit(c.Spec, c.Effective) + ")"
}

// handleListSettingOverrides (GET /settings/overrides) lists the Nodes with a custom value. On the
// Settings screen it answers "if I change the cluster value, who is NOT affected?".
func handleListSettingOverrides(d Deps) http.HandlerFunc {
	type row struct {
		NodeID       string    `json:"node_id"`
		NodeLabel    string    `json:"node_label"`
		Key          string    `json:"key"`
		Label        string    `json:"label"`
		Unit         string    `json:"unit"`
		Value        string    `json:"value"`
		ClusterValue string    `json:"cluster_value"`
		UpdatedAt    time.Time `json:"updated_at"`
		UpdatedBy    *string   `json:"updated_by,omitempty"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if d.AgentConfig == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "node_config_unavailable", "per-Node configuration unavailable")
			return
		}
		rows, err := d.AgentConfig.Overrides(r.Context())
		if err != nil {
			d.Logger.Error("list overrides", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list per-Node values")
			return
		}
		out := make([]row, 0, len(rows))
		for _, o := range rows {
			spec, _ := settings.Lookup(o.Key)
			out = append(out, row{
				NodeID: o.NodeID, NodeLabel: o.Label(), Key: o.Key, Label: spec.Label, Unit: spec.Unit,
				Value: o.Value, ClusterValue: o.ClusterValue, UpdatedAt: o.UpdatedAt, UpdatedBy: o.UpdatedBy,
			})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
	}
}
