package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
)

// TestAgentHeartbeatLastIPFromProxyHeader reproduces #84: the Agent reaches the CP
// through nginx, so the RemoteAddr is the proxy IP (172.18.0.4). The Node's
// last_ip must come from the X-Real-IP header (set by nginx with the Agent's
// real IP), not from the socket. Without the header, it falls back to the RemoteAddr.
func TestAgentHeartbeatLastIPFromProxyHeader(t *testing.T) {
	handler, login, admin := setupRouter(t)
	adminToken := login(admin)

	hostname := fmt.Sprintf("node-%d", time.Now().UnixNano())

	// Registers a node via API with the shared registration token.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/register",
		strings.NewReader(fmt.Sprintf(`{"hostname":%q,"os_name":"Linux","architecture":"amd64","agent_version":"test"}`, hostname)))
	req.Header.Set(httpx.AgentRegistrationHeader, "reg-token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("registrar node: %d %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		AgentToken string `json:"agent_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reg); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	if reg.AgentToken == "" {
		t.Fatal("empty agent_token")
	}

	hbBody := fmt.Sprintf(`{"status":"ONLINE","hostname":%q,"agent_version":"test","engine_reachable":true,"bt_port":6881}`, hostname)

	// Heartbeat WITHOUT proxy header → last_ip = socket RemoteAddr (httptest: 192.0.2.1).
	doHeartbeat := func(headers map[string]string) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", strings.NewReader(hbBody))
		req.Header.Set("Authorization", "Bearer "+reg.AgentToken)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("heartbeat: %d %s", rec.Code, rec.Body.String())
		}
	}

	doHeartbeat(nil) // writes 192.0.2.1 (httptest RemoteAddr)
	// Heartbeat WITH X-Real-IP (what nginx does): last_ip = 192.168.0.114.
	doHeartbeat(map[string]string{"X-Real-IP": "192.168.0.114"})

	// Reads the node's last_ip via API (GET /nodes, admin).
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list nodes: %d", rec.Code)
	}
	var out struct {
		Data []struct {
			Hostname string  `json:"hostname"`
			LastIP   *string `json:"last_ip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode nodes: %v", err)
	}
	t.Logf("GET /nodes body: %s", rec.Body.String())
	var found *string
	for _, n := range out.Data {
		if n.Hostname == hostname {
			found = n.LastIP
		}
	}
	if found == nil {
		t.Fatalf("node %s not found in the listing", hostname)
	}
	// The last heartbeat was with X-Real-IP — the last_ip MUST be the one from the header,
	// not the proxy's 172.18.0.4 (which doesn't even appear here — httptest uses 192.0.2.1).
	if *found != "192.168.0.114" {
		t.Errorf("last_ip = %q, want 192.168.0.114 (nginx X-Real-IP) — bug #84: recorded socket IP", *found)
	}
}
