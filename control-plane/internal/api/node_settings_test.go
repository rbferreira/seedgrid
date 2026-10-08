package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
)

// HTTP tests for the Agent/Engine behavior configuration per Node (ADR-039, #144):
// the heartbeat delivers the effective values, the per-Node override only applies to that Node, and the
// RBAC and Audit follow the standard of the rest of the API.

type agentNode struct {
	ID    string
	Token string // Agent credential (Bearer)
	Host  string
}

// registerAgent registers a Node via the legacy path (shared token) and returns the
// Agent credential — this is what is used to call the heartbeat.
func registerAgent(t *testing.T, handler http.Handler, hostname string) agentNode {
	t.Helper()
	code, body := call(t, handler, http.MethodPost, "/api/v1/agent/register", "",
		`{"hostname":"`+hostname+`","engine":"qbittorrent"}`,
		map[string]string{httpx.AgentRegistrationHeader: "reg-token"})
	if code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", hostname, code, body)
	}
	var out struct {
		NodeID     string `json:"node_id"`
		AgentToken string `json:"agent_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AgentToken == "" {
		t.Fatalf("register response: %v: %s", err, body)
	}
	return agentNode{ID: out.NodeID, Token: out.AgentToken, Host: hostname}
}

type agentConfigBlock struct {
	Version     string            `json:"version"`
	Values      map[string]string `json:"values"`
	ReportLocal bool              `json:"report_local"`
}

// heartbeat calls POST /agent/heartbeat and returns the status, the agent_config block (nil if the
// response didn't include it) and the raw body.
func heartbeat(t *testing.T, handler http.Handler, a agentNode, body string) (int, *agentConfigBlock, []byte) {
	t.Helper()
	code, resp := call(t, handler, http.MethodPost, "/api/v1/agent/heartbeat", a.Token, body, nil)
	var out struct {
		Status      string            `json:"status"`
		AgentConfig *agentConfigBlock `json:"agent_config"`
	}
	if code == http.StatusOK {
		if err := json.Unmarshal(resp, &out); err != nil {
			t.Fatalf("heartbeat response: %v: %s", err, resp)
		}
	}
	return code, out.AgentConfig, resp
}

type nodeSettingsView struct {
	NodeID           string  `json:"node_id"`
	Status           string  `json:"status"`
	EffectiveVersion string  `json:"effective_version"`
	AppliedVersion   *string `json:"applied_version"`
	Items            []struct {
		Key           string  `json:"key"`
		Type          string  `json:"type"`
		Label         string  `json:"label"`
		Value         string  `json:"value"`
		Source        string  `json:"source"`
		ClusterValue  string  `json:"cluster_value"`
		ClusterSource string  `json:"cluster_source"`
		Override      *string `json:"override"`
	} `json:"items"`
}

func getNodeSettings(t *testing.T, handler http.Handler, token, nodeID string) nodeSettingsView {
	t.Helper()
	code, body := call(t, handler, http.MethodGet, "/api/v1/nodes/"+nodeID+"/settings", token, "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /nodes/%s/settings: %d %s", nodeID, code, body)
	}
	var v nodeSettingsView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("GET /nodes/{id}/settings: %v: %s", err, body)
	}
	return v
}

func (v nodeSettingsView) item(t *testing.T, key string) (value, source string, override *string) {
	t.Helper()
	for _, it := range v.Items {
		if it.Key == key {
			return it.Value, it.Source, it.Override
		}
	}
	t.Fatalf("key %s missing from /nodes/{id}/settings", key)
	return "", "", nil
}

// The entire cycle through the API: the heartbeat delivers the effective state, the operator creates an override
// just for node-04, the other Node follows the Cluster, and the Agent confirms what it applied.
func TestNodeSettingsHTTP(t *testing.T) {
	handler, login, admin := setupRouter(t)
	tokens := tokensByRole(t, handler, login, admin)
	adminTok := tokens["Administrator"]
	const dl = "engine.max_active_downloads"

	n1 := registerAgent(t, handler, "api-cfg-01")
	n4 := registerAgent(t, handler, "api-cfg-04") // o "Atom"

	// 1. The first heartbeat: the CP delivers ALL keys with the Cluster value and ASKS for the
	// env report (hasn't seeded yet). The new Agent doesn't apply anything before reporting.
	code, ac, raw := heartbeat(t, handler, n4, `{"status":"ONLINE"}`)
	if code != http.StatusOK || ac == nil {
		t.Fatalf("heartbeat: %d %s, want 200 with agent_config", code, raw)
	}
	if !ac.ReportLocal {
		t.Error("first heartbeat: the CP should request the env report (report_local)")
	}
	if len(ac.Values) != 12 || ac.Values[dl] != "10" || ac.Values["agent.heartbeat_interval"] != "10s" {
		t.Errorf("first heartbeat values = %v, want the 12 keys with the cluster value (downloads=10)", ac.Values)
	}
	if ac.Version == "" {
		t.Error("agent_config without version")
	}

	// 2. The env report from node-04 (3 downloads) becomes an override and the heartbeat already returns the
	// protected value. The field is accepted: the heartbeat body has new fields, and the Agent only
	// sends them after seeing agent_config in the previous response.
	code, ac, raw = heartbeat(t, handler, n4,
		`{"status":"ONLINE","local_config":{"`+dl+`":"3","agent.heartbeat_interval":"10s"}}`)
	if code != http.StatusOK || ac == nil {
		t.Fatalf("heartbeat with report: %d %s", code, raw)
	}
	if ac.ReportLocal || ac.Values[dl] != "3" {
		t.Errorf("after the report: report_local=%v downloads=%q, want false and 3 (seeded from env)", ac.ReportLocal, ac.Values[dl])
	}
	// The other Node, which reports the same value as the Cluster, doesn't get an override.
	code, ac1, _ := heartbeat(t, handler, n1, `{"status":"ONLINE","local_config":{"`+dl+`":"10"}}`)
	if code != http.StatusOK || ac1 == nil || ac1.Values[dl] != "10" {
		t.Fatalf("node-01: %d %+v, want 200 with downloads=10", code, ac1)
	}
	if _, src, ov := getNodeSettings(t, handler, adminTok, n1.ID).item(t, dl); src != "cluster" || ov != nil {
		t.Errorf("node-01: source %q override %v, want cluster without override", src, ov)
	}

	// 3. The Portal sees what takes effect, where it comes from, and what the Node would have without the override.
	v4 := getNodeSettings(t, handler, adminTok, n4.ID)
	val, src, ov := v4.item(t, dl)
	if val != "3" || src != "node" || ov == nil || *ov != "3" {
		t.Errorf("node-04: %q/%q/%v, want 3 coming from Node", val, src, ov)
	}
	for _, it := range v4.Items {
		if it.Key == dl && (it.ClusterValue != "10" || it.ClusterSource != "default") {
			t.Errorf("node-04: cluster %q/%q, want 10/default", it.ClusterValue, it.ClusterSource)
		}
	}
	if v4.Status != "unsupported" {
		t.Errorf("before the Agent reports the version: status %q, want unsupported", v4.Status)
	}

	// 4. The Agent confirms the version it applied → up to date. Cluster value changed → the Node
	// WITHOUT override follows (waiting), the one WITH override doesn't move.
	if code, _, raw := heartbeat(t, handler, n4, `{"status":"ONLINE","config_version":"`+ac.Version+`"}`); code != http.StatusOK {
		t.Fatalf("heartbeat with config_version: %d %s", code, raw)
	}
	if st := getNodeSettings(t, handler, adminTok, n4.ID).Status; st != "in_sync" {
		t.Errorf("Agent applied the effective version: status %q, want in_sync", st)
	}
	code, body := call(t, handler, http.MethodPut, "/api/v1/settings/"+dl, adminTok, `{"value":"6"}`, nil)
	if code != http.StatusOK {
		t.Fatalf("PUT /settings/%s: %d %s", dl, code, body)
	}
	if !auditHas(t, handler, adminTok,
		"Changed setting «Simultaneous downloads per Node» to 6 (cluster value — applies to Nodes without an override)",
		http.StatusOK) {
		t.Error("cluster value audit should say it applies to Nodes without override")
	}
	_, ac1, _ = heartbeat(t, handler, n1, `{"status":"ONLINE"}`)
	if ac1 == nil || ac1.Values[dl] != "6" {
		t.Errorf("node-01 after changing the cluster: %+v, want downloads=6", ac1)
	}
	_, ac4, _ := heartbeat(t, handler, n4, `{"status":"ONLINE"}`)
	if ac4 == nil || ac4.Values[dl] != "3" {
		t.Errorf("node-04 (override) after changing the cluster: %+v, want downloads=3", ac4)
	}
	// The Cluster value changed, but node-04's didn't (it has an override for this key): its
	// effective version is the same and it remains up to date.
	if st := getNodeSettings(t, handler, adminTok, n4.ID).Status; st != "in_sync" {
		t.Errorf("node-04 after changing the cluster: status %q, want in_sync (none of its effective values changed)", st)
	}

	// 5. Override by the operator (nodes:write) — and the Node sees it on the next heartbeat.
	code, body = call(t, handler, http.MethodPut, "/api/v1/nodes/"+n1.ID+"/settings/engine.max_active_checking", adminTok, `{"value":"2"}`, nil)
	if code != http.StatusOK {
		t.Fatalf("PUT override: %d %s", code, body)
	}
	var item struct {
		Value    string  `json:"value"`
		Source   string  `json:"source"`
		Override *string `json:"override"`
	}
	if err := json.Unmarshal(body, &item); err != nil || item.Value != "2" || item.Source != "node" {
		t.Errorf("PUT override response = %s (%v), want value=2 source=node", body, err)
	}
	_, ac1, _ = heartbeat(t, handler, n1, `{"status":"ONLINE"}`)
	if ac1 == nil || ac1.Values["engine.max_active_checking"] != "2" {
		t.Errorf("node-01 after the override: %+v, want checking=2", ac1)
	}
	want := "Set «Simultaneous checks per Node» on Node «api-cfg-01» to 2 (it followed the cluster before)"
	if !auditHas(t, handler, adminTok, want, http.StatusOK) {
		t.Errorf("the audit trail should record %q", want)
	}

	// 6. Removing reverts to the Cluster, with the previous state in Audit.
	code, body = call(t, handler, http.MethodDelete, "/api/v1/nodes/"+n1.ID+"/settings/engine.max_active_checking", adminTok, "", nil)
	if code != http.StatusOK {
		t.Fatalf("DELETE override: %d %s", code, body)
	}
	_, ac1, _ = heartbeat(t, handler, n1, `{"status":"ONLINE"}`)
	if ac1 == nil || ac1.Values["engine.max_active_checking"] != "1" {
		t.Errorf("node-01 after removing the override: %+v, want checking=1 (cluster)", ac1)
	}
	want = "Removed the Node-specific value of «Simultaneous checks per Node» from Node «api-cfg-01» (was 2; back to the cluster value: 1)"
	if !auditHas(t, handler, adminTok, want, http.StatusOK) {
		t.Errorf("the audit trail should record %q", want)
	}

	// 7. Any Node with a custom value appears in the Settings screen list.
	code, body = call(t, handler, http.MethodGet, "/api/v1/settings/overrides", adminTok, "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /settings/overrides: %d %s", code, body)
	}
	var ovs struct {
		Data []struct {
			NodeLabel    string `json:"node_label"`
			Key          string `json:"key"`
			Label        string `json:"label"`
			Value        string `json:"value"`
			ClusterValue string `json:"cluster_value"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ovs); err != nil {
		t.Fatalf("overrides: %v", err)
	}
	foundOv := false
	for _, o := range ovs.Data {
		if o.NodeLabel == "api-cfg-04" && o.Key == dl {
			foundOv = true
			if o.Value != "3" || o.ClusterValue != "6" || o.Label == "" {
				t.Errorf("listed override = %+v, want 3 against cluster 6", o)
			}
		}
		if o.NodeLabel == "api-cfg-01" {
			t.Errorf("node-01 no longer has override and appears: %+v", o)
		}
	}
	if !foundOv {
		t.Error("node-04 override does not appear in /settings/overrides")
	}
}

func TestNodeSettingsErrors(t *testing.T) {
	handler, login, admin := setupRouter(t)
	tokens := tokensByRole(t, handler, login, admin)
	adminTok := tokens["Administrator"]
	n := registerAgent(t, handler, "api-cfg-err")
	path := "/api/v1/nodes/" + n.ID + "/settings/"

	for _, tc := range []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"value out of range (same bounds as cluster)", http.MethodPut, path + "engine.max_active_downloads", `{"value":"0"}`, http.StatusBadRequest, "invalid_value"},
		{"qBittorrent unlimited would crash an Atom", http.MethodPut, path + "engine.max_active_downloads", `{"value":"-1"}`, http.StatusBadRequest, "invalid_value"},
		{"heartbeat de 0s derrubaria o ticker", http.MethodPut, path + "agent.heartbeat_interval", `{"value":"0s"}`, http.StatusBadRequest, "invalid_value"},
		{"tipo errado", http.MethodPut, path + "engine.queueing_enabled", `{"value":"talvez"}`, http.StatusBadRequest, "invalid_value"},
		{"Control Plane tunable has no notion of Node", http.MethodPut, path + "admission.interval", `{"value":"30s"}`, http.StatusBadRequest, "not_per_node"},
		{"Control Plane tunable: remove as well", http.MethodDelete, path + "admission.interval", ``, http.StatusBadRequest, "not_per_node"},
		{"nonexistent tunable", http.MethodPut, path + "engine.nao_existe", `{"value":"1"}`, http.StatusNotFound, "setting_not_found"},
		{"Node inexistente", http.MethodPut, "/api/v1/nodes/00000000-0000-0000-0000-000000000000/settings/engine.max_active_downloads", `{"value":"3"}`, http.StatusNotFound, "node_not_found"},
		{"id that is not even a uuid", http.MethodGet, "/api/v1/nodes/xyz/settings", ``, http.StatusNotFound, "node_not_found"},
		{"campo desconhecido no corpo", http.MethodPut, path + "engine.max_active_downloads", `{"valor":"3"}`, http.StatusBadRequest, "invalid_body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := call(t, handler, tc.method, tc.path, adminTok, tc.body, nil)
			if code != tc.status || errCode(t, body) != tc.code {
				t.Errorf("%s %s: %d %s, want %d %s", tc.method, tc.path, code, body, tc.status, tc.code)
			}
		})
	}
	// None of this was logged.
	for _, it := range getNodeSettings(t, handler, adminTok, n.ID).Items {
		if it.Override != nil {
			t.Errorf("%s ended up with override %q after invalid requests", it.Key, *it.Override)
		}
	}
}

// Reading is for those who read Nodes; writing the override is nodes:write; the Cluster value remains
// settings:write (ADR-023). The denied attempt is also logged in the trail.
func TestNodeSettingsRBAC(t *testing.T) {
	handler, login, admin := setupRouter(t)
	tokens := tokensByRole(t, handler, login, admin)
	adminTok := tokens["Administrator"]
	n := registerAgent(t, handler, "api-cfg-rbac")
	put := "/api/v1/nodes/" + n.ID + "/settings/engine.max_active_downloads"

	// All four roles can read.
	for role, tok := range tokens {
		if code, _ := call(t, handler, http.MethodGet, "/api/v1/nodes/"+n.ID+"/settings", tok, "", nil); code != http.StatusOK {
			t.Errorf("%s reading Node configuration: %d, want 200", role, code)
		}
		if code, _ := call(t, handler, http.MethodGet, "/api/v1/settings/overrides", tok, "", nil); code != http.StatusOK {
			t.Errorf("%s reading override list: %d, want 200", role, code)
		}
	}
	// Only the Administrator can write (nodes:write).
	for _, role := range []string{"Publisher", "Downloader", "Viewer"} {
		if code, _ := call(t, handler, http.MethodPut, put, tokens[role], `{"value":"2"}`, nil); code != http.StatusForbidden {
			t.Errorf("%s setting override: %d, want 403", role, code)
		}
		if code, _ := call(t, handler, http.MethodDelete, put, tokens[role], "", nil); code != http.StatusForbidden {
			t.Errorf("%s removing override: %d, want 403", role, code)
		}
		// The Cluster value remains under settings:write: no one but the Administrator.
		if code, _ := call(t, handler, http.MethodPut, "/api/v1/settings/engine.max_active_downloads", tokens[role], `{"value":"2"}`, nil); code != http.StatusForbidden {
			t.Errorf("%s changing cluster value: %d, want 403", role, code)
		}
	}
	for _, it := range getNodeSettings(t, handler, adminTok, n.ID).Items {
		if it.Override != nil {
			t.Errorf("denied attempt wrote override to %s", it.Key)
		}
	}
	if !auditHasDenied(t, handler, adminTok, "/nodes/"+n.ID+"/settings/engine.max_active_downloads") {
		t.Error("the denied attempt (403) to set the override should appear in the audit trail")
	}
	// The Administrator writes.
	if code, body := call(t, handler, http.MethodPut, put, adminTok, `{"value":"2"}`, nil); code != http.StatusOK {
		t.Errorf("Administrator setting override: %d %s", code, body)
	}
}

// The heartbeat MUST NOT break due to configuration: a body with a field the CP doesn't
// know about still returns 400 (DisallowUnknownFields) — that's exactly why the Agent only
// sends new fields after seeing `agent_config` in the response.
func TestHeartbeatUnknownFieldRemains400(t *testing.T) {
	handler, _, _ := setupRouter(t)
	n := registerAgent(t, handler, "api-cfg-hb")
	if code, _, raw := heartbeat(t, handler, n, `{"status":"ONLINE","campo_que_o_cp_nao_conhece":1}`); code != http.StatusBadRequest {
		t.Errorf("heartbeat with unknown field: %d %s, want 400", code, raw)
	}
	// Legacy body (Agent prior to ADR-039), without any new fields, still gets 200 — and receives the
	// configuration block in the response, which the legacy Agent simply ignores.
	code, ac, raw := heartbeat(t, handler, n, `{"status":"ONLINE","hostname":"api-cfg-hb","uptime_seconds":5,"engine_reachable":true}`)
	if code != http.StatusOK {
		t.Fatalf("old heartbeat: %d %s", code, raw)
	}
	if ac == nil {
		t.Error("the response should include agent_config (the old Agent ignores it)")
	}
	if !strings.Contains(string(raw), `"status":"ok"`) {
		t.Errorf("response lost the status field: %s", raw)
	}
}

// The Settings listing marks what is a CLUSTER value for an Agent/Engine tunable and
// renames the category.
func TestSettingsListMarksPerNode(t *testing.T) {
	handler, login, admin := setupRouter(t)
	adminTok := login(admin)
	code, body := call(t, handler, http.MethodGet, "/api/v1/settings", adminTok, "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /settings: %d %s", code, body)
	}
	var out struct {
		Groups []struct {
			Category string `json:"category"`
			Label    string `json:"label"`
			Items    []struct {
				Key     string `json:"key"`
				PerNode bool   `json:"per_node"`
			} `json:"items"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	perNode, agentLabel := 0, ""
	for _, g := range out.Groups {
		if g.Category == "agent" {
			agentLabel = g.Label
		}
		for _, it := range g.Items {
			if it.PerNode {
				perNode++
				if g.Category != "agent" {
					t.Errorf("%s is per_node outside agent category (%s)", it.Key, g.Category)
				}
			}
		}
	}
	if perNode != 12 {
		t.Errorf("per_node items = %d, want 12", perNode)
	}
	if agentLabel != "Agent / Engine" {
		t.Errorf("agent category label = %q, want %q", agentLabel, "Agent / Engine")
	}
}

// auditHasDenied tells if the trail has a DENIED (403) attempt on this path. Its label
// comes from the route fallback, not the handler (which doesn't even run), so the identity is the path.
func auditHasDenied(t *testing.T, handler http.Handler, adminToken, pathPart string) bool {
	t.Helper()
	code, body := call(t, handler, http.MethodGet, "/api/v1/audit?limit=100", adminToken, "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /audit: %d %s", code, body)
	}
	var out struct {
		Data []struct {
			Path       string `json:"path"`
			StatusCode int    `json:"status_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("GET /audit: %v", err)
	}
	for _, e := range out.Data {
		if e.StatusCode == http.StatusForbidden && strings.Contains(e.Path, pathPart) {
			return true
		}
	}
	return false
}
