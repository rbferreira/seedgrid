package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAgentEnrollReturnsNodeIndex covers the enroll contract that the bootstrap uses to
// build the Node's deterministic IPv6 suffix (ADR-038): besides `bt_port`, the response
// returns `node_index` — the Node's position in the port sequence. The router DB is
// cleared before each test, so the sequence starts from zero: 6881 → 1, 6882 → 2.
func TestAgentEnrollReturnsNodeIndex(t *testing.T) {
	handler, login, admin := setupRouter(t)
	adminToken := login(admin)

	mint := func() string {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll-tokens", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("issue join token: %d %s", rec.Code, rec.Body.String())
		}
		var out struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
			t.Fatalf("join token missing from response: %s (%v)", rec.Body.String(), err)
		}
		return out.Token
	}

	type enrollResp struct {
		BTPort    int  `json:"bt_port"`
		NodeIndex *int `json:"node_index"`
	}
	enroll := func(token string) (int, enrollResp) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/enroll",
			strings.NewReader(fmt.Sprintf(`{"token":%q}`, token)))
		handler.ServeHTTP(rec, req)
		var out enrollResp
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode enroll: %v (%s)", err, rec.Body.String())
			}
		}
		return rec.Code, out
	}

	tokA, tokB := mint(), mint()

	code, a := enroll(tokA)
	if code != http.StatusOK {
		t.Fatalf("enroll A: status %d", code)
	}
	if a.BTPort != 6881 || a.NodeIndex == nil || *a.NodeIndex != 1 {
		t.Errorf("enroll A = %+v, want bt_port=6881 and node_index=1", a)
	}

	code, b := enroll(tokB)
	if code != http.StatusOK {
		t.Fatalf("enroll B: status %d", code)
	}
	if b.BTPort != 6882 || b.NodeIndex == nil || *b.NodeIndex != 2 {
		t.Errorf("enroll B = %+v, want bt_port=6882 and node_index=2", b)
	}

	// Repeating enroll with the same token (the bootstrap can be run again) returns the
	// same port and same index: the Node's address does not change because of this.
	code, again := enroll(tokA)
	if code != http.StatusOK || again.BTPort != 6881 || again.NodeIndex == nil || *again.NodeIndex != 1 {
		t.Errorf("repeated enroll = (%d, %+v), want the same port/index as before", code, again)
	}

	// Invalid token does not reveal any index.
	if code, _ := enroll("token-that-does-not-exist"); code != http.StatusUnauthorized {
		t.Errorf("enroll with invalid token: status %d, want 401", code)
	}
}
