package remotecfg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
	"github.com/rbferreira/seedgrid/agent/internal/config"
)

// These tests wire the Manager to the REAL HTTP client against fake Control Planes, to
// prove bidirectional compatibility (ADR-039 §6) on the wire, not just in logic.

// negotiationFields are the heartbeat body fields that ADR-039 added.
var negotiationFields = map[string]bool{"config_version": true, "local_config": true}

// oldCPFields are the fields that a Control Plane PRIOR to ADR-039 accepts in the
// heartbeat body: everything the Agent sends today, minus the negotiation. Derived via reflection from the client's
// own struct, so a legitimate new field needs no maintenance here — only the
// negotiation is left out, which is exactly what an old CP is unaware of.
func oldCPFields() map[string]bool {
	allowed := map[string]bool{}
	t := reflect.TypeOf(client.HeartbeatRequest{})
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && !negotiationFields[name] {
			allowed[name] = true
		}
	}
	return allowed
}

// fakeCP is a fake Control Plane.
type fakeCP struct {
	mu       sync.Mutex
	old      bool // true = prior to ADR-039: rejects unknown fields (DisallowUnknownFields) and doesn't send agent_config
	seeded   bool
	override map[string]string // Node override, seeded from the report
	cluster  map[string]string
	bodies   []map[string]json.RawMessage
	rejected int // heartbeats answered with 400
}

func (f *fakeCP) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body := map[string]json.RawMessage{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.bodies = append(f.bodies, body)

		if f.old {
			allowed := oldCPFields()
			for k := range body {
				if !allowed[k] {
					f.rejected++
					http.Error(w, `{"error":{"code":"invalid_request","message":"invalid body"}}`, http.StatusBadRequest)
					return
				}
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}

		if raw, ok := body["local_config"]; ok {
			var local map[string]string
			_ = json.Unmarshal(raw, &local)
			if !f.seeded {
				f.seeded = true
				for k, v := range local {
					if f.cluster[k] != v { // only what differs from the cluster becomes an override
						f.override[k] = v
					}
				}
			}
		}
		values := map[string]string{}
		for k, v := range f.cluster {
			values[k] = v
		}
		for k, v := range f.override {
			values[k] = v
		}
		out := map[string]any{"status": "ok", "agent_config": map[string]any{
			"version": "ver-" + values["engine.max_active_downloads"], "values": values, "report_local": !f.seeded,
		}}
		_ = json.NewEncoder(w).Encode(out)
	})
}

// cycle repeats what the Agent's heartbeat loop does (main.go): Prepare → Heartbeat → Handle,
// with the immediate follow-up when the CP requests the report.
func cycle(t *testing.T, m *Manager, cli *client.Client) {
	t.Helper()
	send := func() {
		req := client.HeartbeatRequest{Status: "ONLINE", Hostname: "n"}
		m.Prepare(&req)
		resp, err := cli.Heartbeat(context.Background(), "tok", req)
		m.Handle(context.Background(), resp, err)
	}
	send()
	for i := 0; i < 2 && m.NeedsFollowUp(); i++ {
		send()
	}
}

func newClusterValues() map[string]string { return clusterValues("10") }

// NEW Agent + OLD CP: the Agent never sends a field the CP doesn't know (the CP would reject it with
// 400 and the Node would go OFFLINE), and sticks to the env. It's the "I upgraded the Agent before the CP" scenario.
func TestWireNewAgentOldCP(t *testing.T) {
	cp := &fakeCP{old: true}
	srv := httptest.NewServer(cp.handler())
	defer srv.Close()

	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{"engine.max_active_downloads": "3"}, rec.hooks())
	cli := client.New(srv.URL)
	for i := 0; i < 6; i++ {
		cycle(t, m, cli)
	}

	if cp.rejected != 0 {
		t.Fatalf("old CP rejected %d heartbeats (400): Agent sent unknown field", cp.rejected)
	}
	for i, b := range cp.bodies {
		for f := range negotiationFields {
			if _, ok := b[f]; ok {
				t.Errorf("heartbeat %d carries %q against an old CP", i, f)
			}
		}
	}
	if len(rec.engine) != 1 || rec.engine[0].q.MaxActiveDownloads != 3 || !rec.engine[0].first {
		t.Errorf("Engine = %+v, want the env (downloads=3) applied once", rec.engine)
	}
}

// NEW Agent + NEW CP, existing Node with explicit env (node-04): the Agent reports the env,
// the CP seeds the override and the Engine NEVER sees the cluster value — a single application, with the
// protected value. Then the Agent reports the applied version.
func TestWireSeedingProtectsNode04(t *testing.T) {
	cp := &fakeCP{cluster: newClusterValues(), override: map[string]string{}}
	srv := httptest.NewServer(cp.handler())
	defer srv.Close()

	rec := &recorder{}
	explicit := map[string]string{"engine.max_active_downloads": "3", "agent.heartbeat_interval": "10s"}
	m := New(quiet(), envBaseline(), explicit, rec.hooks())
	cli := client.New(srv.URL)

	cycle(t, m, cli) // 1st heartbeat + follow-up with the report, all in the same cycle

	if len(cp.bodies) != 2 {
		t.Fatalf("heartbeats in the boot cycle = %d, want 2 (the report request and the report, without waiting for the ticker)", len(cp.bodies))
	}
	if _, ok := cp.bodies[0]["local_config"]; ok {
		t.Error("the 1st heartbeat already carried local_config: the CP had not yet proven that it understands it")
	}
	if _, ok := cp.bodies[1]["local_config"]; !ok {
		t.Fatal("2nd heartbeat should carry env report")
	}
	if len(rec.engine) != 1 {
		t.Fatalf("applications to the Engine = %+v, want ONE", rec.engine)
	}
	if got := rec.engine[0].q.MaxActiveDownloads; got != 3 {
		t.Errorf("Engine received downloads=%d, want 3: the cluster value (10) could never reach the Node", got)
	}
	if cp.override["engine.max_active_downloads"] != "3" || len(cp.override) != 1 {
		t.Errorf("seeded override = %v, want only downloads=3 (the 10s heartbeat is equal to the cluster)", cp.override)
	}

	// Subsequent cycles: no report, with the applied version.
	cycle(t, m, cli)
	last := cp.bodies[len(cp.bodies)-1]
	if _, ok := last["local_config"]; ok {
		t.Error("the report continues to be sent after seeding")
	}
	if string(last["config_version"]) != `"ver-3"` {
		t.Errorf("config_version = %s, want \"ver-3\" (the version the Agent applied)", last["config_version"])
	}
	if len(rec.engine) != 1 {
		t.Errorf("the Engine was reapplied without change: %+v", rec.engine)
	}
}

// New Node (only identity in .env): the report goes EMPTY, nothing becomes an override, and the Node is born
// identical to the cluster.
func TestWireNewNodeBornSameAsCluster(t *testing.T) {
	cp := &fakeCP{cluster: newClusterValues(), override: map[string]string{}}
	srv := httptest.NewServer(cp.handler())
	defer srv.Close()

	rec := &recorder{}
	m := New(quiet(), config.Behavior{HeartbeatInterval: 30 * time.Second, CommandPollInterval: 5 * time.Second, LogLevel: "info",
		Queue: config.QueueSettings{Enabled: true, MaxActiveDownloads: 2}}, map[string]string{}, rec.hooks())
	cli := client.New(srv.URL)
	cycle(t, m, cli)

	if string(cp.bodies[1]["local_config"]) != `{}` {
		t.Errorf("report = %s, want {}", cp.bodies[1]["local_config"])
	}
	if len(cp.override) != 0 {
		t.Errorf("new Node got override: %v", cp.override)
	}
	if len(rec.engine) != 1 || rec.engine[0].q.MaxActiveDownloads != 10 {
		t.Errorf("Engine = %+v, want the cluster value (10), not the code default (2)", rec.engine)
	}
}

// CP rollback with the Agent already up to date: the Agent only finds out the CP regressed when a
// heartbeat fails — it costs ONE heartbeat (and not the Node: the offline limit is 90s) — and on the
// next attempt it no longer sends the new field.
func TestWireRollbackDoCPCustaUmHeartbeat(t *testing.T) {
	cp := &fakeCP{cluster: newClusterValues(), override: map[string]string{}}
	srv := httptest.NewServer(cp.handler())
	defer srv.Close()

	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	cli := client.New(srv.URL)
	cycle(t, m, cli)
	cycle(t, m, cli) // already reporting the version

	cp.mu.Lock()
	cp.old = true // the CP was rolled back to a version prior to ADR-039
	cp.mu.Unlock()

	cycle(t, m, cli) // this heartbeat still carries config_version → 400
	if cp.rejected != 1 {
		t.Fatalf("rejected heartbeats = %d, want 1 (the only one the Agent could not avoid)", cp.rejected)
	}
	for i := 0; i < 4; i++ {
		cycle(t, m, cli)
	}
	if cp.rejected != 1 {
		t.Errorf("the Agent kept sending a new field to an old CP: %d rejections", cp.rejected)
	}
	if b, v := m.Applied(); b.Queue.MaxActiveDownloads != 10 || v == "" {
		t.Errorf("CP rollback undid applied: %+v / %q", b, v)
	}
}
