package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
)

// call executes a request and returns status + body. Unlike `do`, which only returns
// the code: here we need to read the body (issued token, error code, list of Nodes).
func call(t *testing.T, handler http.Handler, method, path, bearer, body string, headers map[string]string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func errCode(t *testing.T, body []byte) string {
	t.Helper()
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("unreadable error body: %v: %s", err, body)
	}
	return e.Error.Code
}

type listedNode struct {
	ID          string `json:"id"`
	Hostname    string `json:"hostname"`
	DisplayName string `json:"display_name"`
}

func listNodes(t *testing.T, handler http.Handler, token string) []listedNode {
	t.Helper()
	code, body := call(t, handler, http.MethodGet, "/api/v1/nodes", token, "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /nodes: %d %s", code, body)
	}
	var out struct {
		Data []listedNode `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("GET /nodes: %v", err)
	}
	return out.Data
}

func nodeByHostname(t *testing.T, handler http.Handler, token, hostname string) listedNode {
	t.Helper()
	for _, n := range listNodes(t, handler, token) {
		if n.Hostname == hostname {
			return n
		}
	}
	t.Fatalf("node %q not in listing", hostname)
	return listedNode{}
}

// mintToken issues a join token (with optional nickname) and returns the token and the nickname that
// the CP echoed back — already normalized.
func mintToken(t *testing.T, handler http.Handler, adminToken, body string) (token, echoed string) {
	t.Helper()
	code, resp := call(t, handler, http.MethodPost, "/api/v1/enroll-tokens", adminToken, body, nil)
	if code != http.StatusCreated {
		t.Fatalf("POST /enroll-tokens: %d %s", code, resp)
	}
	var out struct {
		Token       string `json:"token"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(resp, &out); err != nil || out.Token == "" {
		t.Fatalf("token response: %v: %s", err, resp)
	}
	return out.Token, out.DisplayName
}

func registerWith(t *testing.T, handler http.Handler, registrationToken, body string) {
	t.Helper()
	code, resp := call(t, handler, http.MethodPost, "/api/v1/agent/register", "", body,
		map[string]string{httpx.AgentRegistrationHeader: registrationToken})
	if code != http.StatusCreated {
		t.Fatalf("POST /agent/register: %d %s", code, resp)
	}
}

// TestNodeDisplayNameHTTP goes through the entire nickname cycle (#145) via the API: name in the
// join token → registration → listing → rename → remove, with the errors and the audit.
func TestNodeDisplayNameHTTP(t *testing.T) {
	handler, login, admin := setupRouter(t)
	tokens := tokensByRole(t, handler, login, admin)
	adminTok := tokens["Administrator"]

	// 1. The name chosen in the modal travels in the join token; it comes back normalized.
	joinTok, echoed := mintToken(t, handler, adminTok, `{"display_name":"  Living   Room "}`)
	if echoed != "Living Room" {
		t.Errorf("echoed display_name = %q, want %q", echoed, "Living Room")
	}

	// 2. The registration applies the name from the TOKEN. What the Agent sends in the body doesn't count: the
	// nickname is the operator's decision, made when issuing the token.
	registerWith(t, handler, joinTok, `{"hostname":"api-dn-01","display_name":"Set by Agent","engine":"qbittorrent"}`)
	n1 := nodeByHostname(t, handler, adminTok, "api-dn-01")
	if n1.DisplayName != "Living Room" {
		t.Errorf("Node nickname = %q, want the token's (%q)", n1.DisplayName, "Living Room")
	}

	// 3. Token without body keeps working (the original use) and doesn't give a nickname.
	joinTok2, echoed2 := mintToken(t, handler, adminTok, "")
	if echoed2 != "" {
		t.Errorf("token without body echoed display_name %q", echoed2)
	}
	registerWith(t, handler, joinTok2, `{"hostname":"api-dn-02","engine":"qbittorrent"}`)
	n2 := nodeByHostname(t, handler, adminTok, "api-dn-02")
	if n2.DisplayName != "" {
		t.Errorf("unnamed Node in token got display name %q", n2.DisplayName)
	}

	// 4. Errors when issuing the token: name already in use (nickname, hostname — without differentiating
	// case) and invalid name.
	for _, taken := range []string{"living room", "API-DN-02"} {
		code, body := call(t, handler, http.MethodPost, "/api/v1/enroll-tokens", adminTok, `{"display_name":"`+taken+`"}`, nil)
		if code != http.StatusConflict || errCode(t, body) != "display_name_taken" {
			t.Errorf("token with name in use %q: %d %s, want 409 display_name_taken", taken, code, body)
		}
	}
	code, body := call(t, handler, http.MethodPost, "/api/v1/enroll-tokens", adminTok,
		`{"display_name":"`+strings.Repeat("x", 65)+`"}`, nil)
	if code != http.StatusBadRequest || errCode(t, body) != "invalid_display_name" {
		t.Errorf("token with long name: %d %s, want 400 invalid_display_name", code, body)
	}
	if code, _ := call(t, handler, http.MethodPost, "/api/v1/enroll-tokens", adminTok, `{"name":"x"}`, nil); code != http.StatusBadRequest {
		t.Errorf("token with unknown field: %d, want 400", code)
	}

	// 5. Renaming via the Portal: without touching the Node, with before/after in the audit.
	path := "/api/v1/nodes/" + n1.ID + "/display-name"
	code, body = call(t, handler, http.MethodPut, path, adminTok, `{"display_name":" Bedroom "}`, nil)
	if code != http.StatusOK {
		t.Fatalf("rename: %d %s", code, body)
	}
	if got := nodeByHostname(t, handler, adminTok, "api-dn-01"); got.DisplayName != "Bedroom" || got.Hostname != "api-dn-01" {
		t.Errorf("after rename: %+v, want display name Bedroom and hostname intact", got)
	}
	if !auditHas(t, handler, adminTok, "Renamed Node «Living Room» to «Bedroom»", http.StatusOK) {
		t.Error("audit should record before and after the change")
	}

	// 6. Rename errors.
	if code, body := call(t, handler, http.MethodPut, path, adminTok, `{}`, nil); code != http.StatusBadRequest || errCode(t, body) != "invalid_request" {
		t.Errorf("body without display_name: %d %s, want 400 invalid_request (removal is explicit \"\", not omission)", code, body)
	}
	if code, body := call(t, handler, http.MethodPut, path, adminTok, `{"display_name":"`+strings.Repeat("y", 65)+`"}`, nil); code != http.StatusBadRequest || errCode(t, body) != "invalid_display_name" {
		t.Errorf("long nickname: %d %s, want 400 invalid_display_name", code, body)
	}
	pathN2 := "/api/v1/nodes/" + n2.ID + "/display-name"
	if code, body := call(t, handler, http.MethodPut, pathN2, adminTok, `{"display_name":"BEDROOM"}`, nil); code != http.StatusConflict || errCode(t, body) != "display_name_taken" {
		t.Errorf("display name of another Node: %d %s, want 409 display_name_taken", code, body)
	}
	if code, _ := call(t, handler, http.MethodPut, "/api/v1/nodes/00000000-0000-0000-0000-000000000000/display-name", adminTok, `{"display_name":"Ghost"}`, nil); code != http.StatusNotFound {
		t.Errorf("nonexistent Node: %d, want 404", code)
	}

	// 7. Those without nodes:write get a 403 — and the DENIED attempt also enters the audit trail,
	// with the friendly label (the handler doesn't even run, so it's the route's fallback).
	for _, role := range []string{"Publisher", "Downloader", "Viewer"} {
		if code, _ := call(t, handler, http.MethodPut, path, tokens[role], `{"display_name":"Invader"}`, nil); code != http.StatusForbidden {
			t.Errorf("%s renaming: %d, want 403", role, code)
		}
	}
	if got := nodeByHostname(t, handler, adminTok, "api-dn-01"); got.DisplayName != "Bedroom" {
		t.Errorf("denied attempt changed the display name: %q", got.DisplayName)
	}
	if !auditHas(t, handler, adminTok, "Renamed a Node", http.StatusForbidden) {
		t.Error("the denied attempt (403) should appear in the audit trail")
	}

	// 8. Remove: explicit "" returns the Node to the hostname.
	code, body = call(t, handler, http.MethodPut, path, adminTok, `{"display_name":""}`, nil)
	if code != http.StatusOK {
		t.Fatalf("remove display name: %d %s", code, body)
	}
	if got := nodeByHostname(t, handler, adminTok, "api-dn-01"); got.DisplayName != "" {
		t.Errorf("display name after removal = %q, want empty", got.DisplayName)
	}
	if !auditHas(t, handler, adminTok, "Removed the nickname «Bedroom» from Node «api-dn-01»", http.StatusOK) {
		t.Error("display name removal should appear in the audit")
	}
}

// auditHas says whether the audit trail has an entry with exactly this action and this status.
func auditHas(t *testing.T, handler http.Handler, adminToken, action string, status int) bool {
	t.Helper()
	code, body := call(t, handler, http.MethodGet, "/api/v1/audit?limit=100", adminToken, "", nil)
	if code != http.StatusOK {
		t.Fatalf("GET /audit: %d %s", code, body)
	}
	var out struct {
		Data []struct {
			Action     string `json:"action"`
			StatusCode int    `json:"status_code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("GET /audit: %v", err)
	}
	for _, e := range out.Data {
		if e.Action == action && e.StatusCode == status {
			return true
		}
	}
	return false
}
