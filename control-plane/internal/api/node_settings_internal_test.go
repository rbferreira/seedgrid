package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/agentconfig"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

func strp(s string) *string { return &s }

// Without the config service (Deps without AgentConfig) routes respond 503 instead of
// crashing the process with a null pointer — and the heartbeat (which uses the same service) keeps
// working without the agent_config block, which is the contract with the Agent.
func TestNodeSettingsRoutesWithoutServiceReturn503(t *testing.T) {
	d := Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for name, h := range map[string]http.HandlerFunc{
		"GET /nodes/{id}/settings":          handleGetNodeSettings(d),
		"PUT /nodes/{id}/settings/{key}":    handleSetNodeSetting(d),
		"DELETE /nodes/{id}/settings/{key}": handleResetNodeSetting(d),
		"GET /settings/overrides":           handleListSettingOverrides(d),
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{"value":"3"}`)))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s without AgentConfig: %d, want 503", name, rec.Code)
		}
	}
}

// The Audit text carries the before and after: that's all the trail keeps, the old
// value no longer exists anywhere after the upsert (ADR-025).
func TestOverrideAuditText(t *testing.T) {
	downloads, _ := settings.Lookup("engine.max_active_downloads")
	slow, _ := settings.Lookup("engine.slow_dl_threshold") // has unit (KB/s)

	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			"set for the first time",
			setOverrideAuditText(agentconfig.Change{NodeLabel: "node-04", Spec: downloads, After: strp("3")}),
			"Set «Simultaneous downloads per Node» on Node «node-04» to 3 (it followed the cluster before)",
		},
		{
			"changed an override value",
			setOverrideAuditText(agentconfig.Change{NodeLabel: "node-04", Spec: downloads, Before: strp("3"), After: strp("4")}),
			"Set «Simultaneous downloads per Node» on Node «node-04» to 4 (was 3)",
		},
		{
			"catalog unit follows the value",
			setOverrideAuditText(agentconfig.Change{NodeLabel: "n", Spec: slow, Before: strp("1"), After: strp("5")}),
			"Set «Slow download threshold» on Node «n» to 5 KB/s (was 1 KB/s)",
		},
		{
			"removed the override value",
			clearOverrideAuditText(agentconfig.Change{NodeLabel: "node-04", Spec: downloads, Before: strp("3"), Effective: "10"}),
			"Removed the Node-specific value of «Simultaneous downloads per Node» from Node «node-04» (was 3; back to the cluster value: 10)",
		},
		{
			"remove what no longer existed",
			clearOverrideAuditText(agentconfig.Change{NodeLabel: "node-04", Spec: downloads, Effective: "10"}),
			"Removed the Node-specific value of «Simultaneous downloads per Node» from Node «node-04» (it already followed the cluster)",
		},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, c.got, c.want)
		}
	}
}
