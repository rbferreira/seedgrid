package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequireIntegrationToken (#116): with token configured, only those presenting the
// shared secret (dedicated header OR Bearer); without token configured, it passes (compat).
func TestRequireIntegrationToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	call := func(mw Middleware, decorate func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/alertmanager", nil)
		if decorate != nil {
			decorate(req)
		}
		rec := httptest.NewRecorder()
		mw(ok).ServeHTTP(rec, req)
		return rec.Code
	}

	guarded := RequireIntegrationToken("secret-116", nil)
	if got := call(guarded, nil); got != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", got)
	}
	if got := call(guarded, func(r *http.Request) {
		r.Header.Set("X-Seedgrid-Integration-Token", "wrong")
	}); got != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", got)
	}
	if got := call(guarded, func(r *http.Request) {
		r.Header.Set("X-Seedgrid-Integration-Token", "secret-116")
	}); got != http.StatusOK {
		t.Errorf("valid dedicated header: status = %d, want 200", got)
	}
	if got := call(guarded, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer secret-116")
	}); got != http.StatusOK {
		t.Errorf("valid bearer (Alertmanager): status = %d, want 200", got)
	}

	// Without token configured: open (compat with deploys without token; warning in log).
	open := RequireIntegrationToken("", nil)
	if got := call(open, nil); got != http.StatusOK {
		t.Errorf("token not configured: status = %d, want 200 (compat)", got)
	}
}
