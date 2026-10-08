package api

import (
	"net/http"

	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// PT-BR labels for the tunable groups, in the display order on the panel (ADR-023).
//
// ⚠️ New categories in the catalog MUST be added here: the panel builds the groups from
// this list, so a missing category silently disappears from the screen — the tunable would
// exist in the API and be uneditable by the operator. (TestHandleListSettings enforces this
// invariant by comparing the displayed total with len(settings.Specs()).)
var settingsCategoryOrder = []struct {
	cat   settings.Category
	label string
}{
	{settings.CatAdmission, "Admission"},
	{settings.CatAgent, "Agent / Engine"},
	{settings.CatAlerting, "Alerts"},
	{settings.CatNotify, "Notifications"},
	{settings.CatIntegrations, "Integrations"},
	{settings.CatWorkers, "Workers"},
	{settings.CatPublish, "Publishing"},
}

// settingItem is the projection of a tunable to the panel (metadata + effective value).
type settingItem struct {
	Key         string            `json:"key"`
	Type        string            `json:"type"`
	Label       string            `json:"label"`
	Description string            `json:"description"`
	Unit        string            `json:"unit"`
	Value       string            `json:"value"`
	Default     string            `json:"default"`
	Source      string            `json:"source"` // "db" (override) ou "default"
	Min         *float64          `json:"min"`
	Max         *float64          `json:"max"`
	Options     []string          `json:"options,omitempty"`   // enum (KindString restricted) → select in the panel
	Multi       bool              `json:"multi,omitempty"`     // multi-selection (CSV of codes) → checkboxes
	Choices     []settings.Choice `json:"choices,omitempty"`   // multi: code→friendly label
	Secret      bool              `json:"secret,omitempty"`    // user credential (ADR-028)
	HasValue    bool              `json:"has_value,omitempty"` // secret: whether it is filled (the value is not returned)
	PerNode     bool              `json:"per_node,omitempty"`  // CLUSTER value of an Agent/Engine tunable: each Node can have an override (ADR-039)
}

// toItem projects a tunable to the panel.
//
// ⚠️ Secret field (ADR-028) NEVER carries the value: the read API returns only
// `has_value`, as is already done with trackers.api_token (ADR-020). A token returned in a
// GET response leaks in proxy logs, browser history, and devtools — and the panel doesn't
// need it: to change it, the operator types the new one; to disable it, restore the default.
func toItem(e settings.Effective) settingItem {
	it := settingItem{
		Key: e.Key, Type: string(e.Kind), Label: e.Label, Description: e.Description,
		Unit: e.Unit, Value: e.Value, Default: e.Default, Source: e.Source,
		Min: e.Min, Max: e.Max, Options: e.Options, Multi: e.Multi, Choices: e.Choices,
		Secret: e.Secret, PerNode: e.PerNode,
	}
	if e.Secret {
		it.HasValue = e.Value != ""
		it.Value = ""
	}
	return it
}

// handleListSettings returns the tunables grouped by category, with effective value,
// default and origin — input for the settings panel (ADR-023).
func handleListSettings(d Deps) http.HandlerFunc {
	type group struct {
		Category string        `json:"category"`
		Label    string        `json:"label"`
		Items    []settingItem `json:"items"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		eff := d.Settings.Effective()
		byCat := make(map[settings.Category][]settingItem, len(settingsCategoryOrder))
		for _, e := range eff {
			byCat[e.Category] = append(byCat[e.Category], toItem(e))
		}
		groups := make([]group, 0, len(settingsCategoryOrder))
		for _, g := range settingsCategoryOrder {
			groups = append(groups, group{Category: string(g.cat), Label: g.label, Items: byCat[g.cat]})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"groups": groups})
	}
}

// effectiveByKey locates the current effective value of a key (small N).
func effectiveByKey(d Deps, key string) (settings.Effective, bool) {
	for _, e := range d.Settings.Effective() {
		if e.Key == key {
			return e, true
		}
	}
	return settings.Effective{}, false
}

// handleUpdateSetting validates and saves an override (PUT /settings/{key}) — applies at
// runtime (hot-reload) and logs an audit event.
func handleUpdateSetting(d Deps) http.HandlerFunc {
	type request struct {
		Value string `json:"value"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		spec, ok := settings.Lookup(key)
		if !ok {
			httpx.WriteError(w, http.StatusNotFound, "setting_not_found", "unknown tunable")
			return
		}
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		// Secret field (ADR-028): empty = KEEP. The panel never receives the value back,
		// so the form arrives empty — without this, saving any field would erase the token.
		// To actually clear it (= disable the channel), the operator uses "Restore default" (DELETE).
		if spec.Secret && req.Value == "" {
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "unchanged", "key": key})
			return
		}
		user, _ := httpx.OperatorFromContext(r.Context())
		actor := "operator"
		if user != nil {
			actor = user.Username
		}
		if err := d.Settings.Set(r.Context(), key, req.Value, actor); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_value", err.Error())
			return
		}
		// Tunable change is an operator action → goes only to Audit (ADR-025),
		// not to Events (which are domain facts). Friendly description: catalog
		// label + value.
		//
		// ⚠️ Secret field DOES NOT have the audited value (ADR-028): Audit is a query
		// screen like any other, and logging the bot token here would expose it in plain
		// text forever. The FACT of the change is logged, which is what audit needs to prove.
		//
		// value_kind chooses the phrase for the reader (#57): plain, secret (without the value) or cluster.
		value := req.Value
		if spec.Unit != "" {
			value += " " + spec.Unit
		}
		kind, text := "plain", "Changed setting «"+spec.Label+"» to "+value
		if spec.Secret {
			kind, value, text = "secret", "", "Changed setting «"+spec.Label+"» (value hidden)"
		} else if spec.PerNode {
			// CLUSTER value of an Agent/Engine tunable (ADR-039): applies to every Node
			// WITHOUT override, on the next heartbeat — the trail needs to say this, otherwise "Changed
			// X to 10" looks like a local change.
			kind = "cluster"
			text += " (cluster value — applies to Nodes without an override)"
		}
		httpx.SetAudit(r.Context(), "setting_changed", map[string]any{"key": key, "label": spec.Label, "value": value, "value_kind": kind}, text)
		e, _ := effectiveByKey(d, key)
		httpx.WriteJSON(w, http.StatusOK, toItem(e))
	}
}

// handleResetSetting removes the override, reverting to the catalog default
// (DELETE /settings/{key}) — hot-reload + audit.
func handleResetSetting(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if _, ok := settings.Lookup(key); !ok {
			httpx.WriteError(w, http.StatusNotFound, "setting_not_found", "unknown tunable")
			return
		}
		user, _ := httpx.OperatorFromContext(r.Context())
		actor := "operator"
		if user != nil {
			actor = user.Username
		}
		if err := d.Settings.Reset(r.Context(), key, actor); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "reset_failed", err.Error())
			return
		}
		// Operator action → only Audit (ADR-025), not Events.
		if spec, ok := settings.Lookup(key); ok {
			httpx.SetAudit(r.Context(), "setting_reset", map[string]any{"key": key, "label": spec.Label}, "Reset setting «"+spec.Label+"» to its default")
		}
		e, _ := effectiveByKey(d, key)
		httpx.WriteJSON(w, http.StatusOK, toItem(e))
	}
}
