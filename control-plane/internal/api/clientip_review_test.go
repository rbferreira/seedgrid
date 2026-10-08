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

// TestLoginToleratesXRealIPValues reproduces the regression pointed out in the review of
// #85: X-Real-IP becomes the session's ip_address (INET column) — a value that does not
// parse as IP would break login with "invalid credentials" (correct password).
// With the validation, all values below should result in 200.
func TestLoginToleratesXRealIPValues(t *testing.T) {
	handler, _, admin := setupRouter(t)

	values := []struct {
		name string
		val  string
	}{
		{"clean IPv4", "192.168.0.50"},
		{"IPv6", "2001:db8::1"},
		{"with spaces", "  192.168.0.50  "},
		{"empty (fallback)", ""},
		{"IP with port", "192.168.0.50:9999"},
		{"unknown", "unknown"},
	}
	for _, tt := range values {
		t.Run(tt.name, func(t *testing.T) {
			// VALID credentials of the setup admin: before the fix, an invalid
			// X-Real-IP caused the session INSERT to blow up (INET column) and the
			// handler returned 401 "invalid credentials" — correct password.
			body := fmt.Sprintf(`{"username":%q,"password":"strong-password"}`, admin)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
			if tt.val != "" {
				req.Header.Set("X-Real-IP", tt.val)
			}
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("X-Real-IP %q: login with correct password → %d (was 401 in the bug): %s",
					tt.val, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestHeartbeatInvalidHeaderFallsBackToSocket reproduces point 2 of review #85:
// X-Real-IP that is not an IP must not pollute last_ip (TEXT column that feeds
// Prometheus file_sd and migration) — it must fall back to the socket IP.
func TestHeartbeatInvalidHeaderFallsBackToSocket(t *testing.T) {
	handler, login, admin := setupRouter(t)
	adminToken := login(admin)

	hostname := fmt.Sprintf("node-%d", time.Now().UnixNano())

	// Registers a real node via API.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/register",
		strings.NewReader(fmt.Sprintf(`{"hostname":%q,"os_name":"Linux"}`, hostname)))
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

	// Heartbeat with an invalid X-Real-IP ("not-an-ip-at-all").
	hbBody := fmt.Sprintf(`{"status":"ONLINE","hostname":%q,"bt_port":6881}`, hostname)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", strings.NewReader(hbBody))
	req.Header.Set("Authorization", "Bearer "+reg.AgentToken)
	req.Header.Set("X-Real-IP", "not-an-ip-at-all")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat: %d %s", rec.Code, rec.Body.String())
	}

	// last_ip must be the socket IP (httptest: 192.0.2.1), NEVER the raw string.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	handler.ServeHTTP(rec, req)
	var out struct {
		Data []struct {
			Hostname string  `json:"hostname"`
			LastIP   *string `json:"last_ip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode nodes: %v", err)
	}
	var lastIP *string
	for _, n := range out.Data {
		if n.Hostname == hostname {
			lastIP = n.LastIP
		}
	}
	if lastIP == nil {
		t.Fatal("node without last_ip")
	}
	if *lastIP == "not-an-ip-at-all" {
		t.Fatalf("last_ip = %q — invalid header polluted the column (review #85)", *lastIP)
	}
	if *lastIP != "192.0.2.1" {
		t.Errorf("last_ip = %q, want 192.0.2.1 (socket IP, fallback)", *lastIP)
	}
}
