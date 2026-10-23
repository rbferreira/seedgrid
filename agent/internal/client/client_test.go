package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister(t *testing.T) {
	var gotToken, gotHostname string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/register" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotToken = r.Header.Get("X-Seedgrid-Register-Token")
		var in RegisterRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		gotHostname = in.Hostname
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(RegisterResponse{NodeID: "n1", NodeUUID: "u1", AgentToken: "atok"})
	}))
	defer srv.Close()

	resp, err := New(srv.URL).Register(context.Background(), "reg-tok", RegisterRequest{Hostname: "host-a"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if resp.NodeID != "n1" || resp.AgentToken != "atok" {
		t.Errorf("response = %+v", resp)
	}
	if gotToken != "reg-tok" {
		t.Errorf("header token = %q, want reg-tok", gotToken)
	}
	if gotHostname != "host-a" {
		t.Errorf("hostname = %q, want host-a", gotHostname)
	}
}

func TestHeartbeat(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	resp, err := New(srv.URL).Heartbeat(context.Background(), "atok", HeartbeatRequest{Status: "ONLINE"})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if resp.Status != "ok" || resp.AgentConfig != nil {
		t.Errorf("response = %+v, want status ok and no agent_config (CP prior to ADR-039)", resp)
	}
	if gotAuth != "Bearer atok" {
		t.Errorf("Authorization = %q, want 'Bearer atok'", gotAuth)
	}
}

func TestHeartbeatUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := New(srv.URL).Heartbeat(context.Background(), "bad", HeartbeatRequest{Status: "ONLINE"})
	if err != ErrUnauthorized {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

// The new CP returns the Node's effective values in the heartbeat (ADR-039).
func TestHeartbeatLeAgentConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","agent_config":{"version":"abc123def456","report_local":true,` +
			`"values":{"engine.max_active_downloads":"3","agent.heartbeat_interval":"10s"}}}`))
	}))
	defer srv.Close()

	resp, err := New(srv.URL).Heartbeat(context.Background(), "atok", HeartbeatRequest{Status: "ONLINE"})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	ac := resp.AgentConfig
	if ac == nil || ac.Version != "abc123def456" || !ac.ReportLocal ||
		ac.Values["engine.max_active_downloads"] != "3" || ac.Values["agent.heartbeat_interval"] != "10s" {
		t.Errorf("agent_config = %+v", ac)
	}
}

// Reading the response cannot turn a successful heartbeat into a failure: the Node is alive.
// Before ADR-039 the body wasn't even read; a proxy returning HTML, or a format the Agent
// doesn't understand, still counts as a completed heartbeat — just without configuration.
func TestHeartbeatUnreadableResponseNotFailure(t *testing.T) {
	for name, body := range map[string]string{
		"html":                   "<html>bad gateway?</html>",
		"empty":                  "",
		"json of another format": `["a","b"]`,
		"wrong type":             `{"status":"ok","agent_config":"not-an-object"}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			resp, err := New(srv.URL).Heartbeat(context.Background(), "atok", HeartbeatRequest{Status: "ONLINE"})
			if err != nil {
				t.Fatalf("Heartbeat failed with body %q: %v", body, err)
			}
			if resp.AgentConfig != nil {
				t.Errorf("agent_config = %+v, want nil", resp.AgentConfig)
			}
		})
	}
}

// A non-2xx status remains a failure — including the 400 from an old CP that rejected
// an unknown field (that's why the Agent doesn't send new fields by default).
func TestHeartbeatErrorStatusStillFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"invalid_request"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	if _, err := New(srv.URL).Heartbeat(context.Background(), "atok", HeartbeatRequest{Status: "ONLINE"}); err == nil {
		t.Fatal("400 should be an error")
	}
}

// What goes in the body: without negotiation, NO new fields (an old CP would reject them); with
// the report, the empty map goes too — it's what marks the seeding as done.
func TestHeartbeatNegotiationFieldsOnlyWhenFilled(t *testing.T) {
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = map[string]json.RawMessage{}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()
	cli := New(srv.URL)

	if _, err := cli.Heartbeat(context.Background(), "t", HeartbeatRequest{Status: "ONLINE"}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"config_version", "local_config"} {
		if _, ok := got[k]; ok {
			t.Errorf("heartbeat without negotiation sent %q — a CP prior to ADR-039 would return 400", k)
		}
	}

	empty := map[string]string{}
	if _, err := cli.Heartbeat(context.Background(), "t", HeartbeatRequest{
		Status: "ONLINE", ConfigVersion: "abc123def456", LocalConfig: &empty,
	}); err != nil {
		t.Fatal(err)
	}
	if string(got["config_version"]) != `"abc123def456"` {
		t.Errorf("config_version = %s", got["config_version"])
	}
	if string(got["local_config"]) != `{}` {
		t.Errorf("local_config = %s, want {} (the empty report must be sent)", got["local_config"])
	}

	full := map[string]string{"engine.max_active_downloads": "3"}
	if _, err := cli.Heartbeat(context.Background(), "t", HeartbeatRequest{Status: "ONLINE", LocalConfig: &full}); err != nil {
		t.Fatal(err)
	}
	if string(got["local_config"]) != `{"engine.max_active_downloads":"3"}` {
		t.Errorf("local_config = %s", got["local_config"])
	}
}

// TestRegister_NodeRemoved (ADR-044): 410 is the Control Plane refusing a removed Node's machine
// through the shared token; the Agent tells it from any other failure.
func TestRegister_NodeRemoved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()
	if _, err := New(srv.URL).Register(context.Background(), "reg-tok", RegisterRequest{Hostname: "host-a"}); !errors.Is(err, ErrNodeRemoved) {
		t.Errorf("Register = %v, want ErrNodeRemoved", err)
	}
}
