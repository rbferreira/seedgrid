package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/activity"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// choiceValues extracts the Value from the options of a multi-field in the catalog.
func choiceValues(key string) []string {
	spec, _ := settings.Lookup(key)
	out := make([]string, 0, len(spec.Choices))
	for _, c := range spec.Choices {
		out = append(out, c.Value)
	}
	sort.Strings(out)
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Guards against drift: the options offered by the panel must match the vocabulary
// canonical emitted by the domain. If someone adds a new event/kind and forgets the
// catalog (or vice versa), this test breaks — instead of the operator never being able to choose
// the new type, or choosing one that no longer exists.
func TestNotifyChoicesDoNotLag(t *testing.T) {
	wantEv := append([]string(nil), events.NotifiableCodes()...)
	sort.Strings(wantEv)
	if got := choiceValues("notify.events"); !eq(got, wantEv) {
		t.Errorf("notify.events choices diverge from events.NotifiableCodes():\n got=%v\nwant=%v", got, wantEv)
	}
	wantAct := append([]string(nil), activity.AllKinds()...)
	sort.Strings(wantAct)
	if got := choiceValues("notify.activities"); !eq(got, wantAct) {
		t.Errorf("notify.activities choices diverge from activity.AllKinds():\n got=%v\nwant=%v", got, wantAct)
	}
	// Every label must exist (otherwise the panel shows raw code — which this job
	// came to eliminate).
	for _, key := range []string{"notify.events", "notify.activities"} {
		spec, _ := settings.Lookup(key)
		for _, c := range spec.Choices {
			if strings.TrimSpace(c.Label) == "" {
				t.Errorf("%s: option %q without friendly label", key, c.Value)
			}
		}
	}
}

// handleListSettings and the PUT error paths do not touch the database — they can be tested
// with an in-memory Store (settings.NewForTest), without Postgres.

func TestHandleListSettings(t *testing.T) {
	d := Deps{Settings: settings.NewForTest(map[string]string{"admission.dead_grace": "9m"})}
	rec := httptest.NewRecorder()
	handleListSettings(d)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Groups []struct {
			Category string `json:"category"`
			Label    string `json:"label"`
			Items    []struct {
				Key    string `json:"key"`
				Value  string `json:"value"`
				Source string `json:"source"`
			} `json:"items"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(body.Groups) != len(settingsCategoryOrder) {
		t.Fatalf("groups = %d, want %d (one per settingsCategoryOrder category)",
			len(body.Groups), len(settingsCategoryOrder))
	}
	total := 0
	var deadGraceSource, deadGraceValue string
	for _, g := range body.Groups {
		total += len(g.Items)
		for _, it := range g.Items {
			if it.Key == "admission.dead_grace" {
				deadGraceSource, deadGraceValue = it.Source, it.Value
			}
		}
	}
	if total != len(settings.Specs()) {
		t.Errorf("items = %d, want %d (entire catalog)", total, len(settings.Specs()))
	}
	if deadGraceValue != "9m" || deadGraceSource != "db" {
		t.Errorf("dead_grace = %q/%q, want 9m/db (override)", deadGraceValue, deadGraceSource)
	}
}

// The central guarantee of ADR-028: the value of a secret field NEVER leaves the read API
// — only `has_value`. A token that appears in a GET leaks in proxy logs, history, and
// devtools. This test fails if someone removes the masking from toItem.
func TestHandleListSettingsDoesNotLeakSecret(t *testing.T) {
	const token = "123456:SECRET-THAT-CANNOT-LEAK" //nolint:gosec // G101: test fixture, not a real credential
	d := Deps{Settings: settings.NewForTest(map[string]string{
		"notify.telegram_bot_token": token,
		"notify.telegram_chat_id":   "-1001234567890", // non-secret: can appear
	})}
	rec := httptest.NewRecorder()
	handleListSettings(d)(rec, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))

	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("the secret token LEAKED in the GET /settings body")
	}
	var body struct {
		Groups []struct {
			Items []struct {
				Key      string `json:"key"`
				Value    string `json:"value"`
				Secret   bool   `json:"secret"`
				HasValue bool   `json:"has_value"`
			} `json:"items"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	var found bool
	for _, g := range body.Groups {
		for _, it := range g.Items {
			if it.Key != "notify.telegram_bot_token" {
				continue
			}
			found = true
			if it.Value != "" {
				t.Errorf("secret field value = %q, want empty", it.Value)
			}
			if !it.Secret || !it.HasValue {
				t.Errorf("secret=%v has_value=%v; want true/true (populated, not exposed)", it.Secret, it.HasValue)
			}
		}
	}
	if !found {
		t.Error("notify.telegram_bot_token did not appear in panel (unregistered category?)")
	}
}

// Secret + empty PUT = KEEP. The panel does not receive the value back, so the form
// arrives empty; without this, saving any field would delete the operator's token.
func TestHandleUpdateSettingEmptySecretKept(t *testing.T) {
	d := Deps{Settings: settings.NewForTest(map[string]string{"notify.telegram_bot_token": "current-tok"})}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/notify.telegram_bot_token",
		strings.NewReader(`{"value":""}`))
	req.SetPathValue("key", "notify.telegram_bot_token")
	handleUpdateSetting(d)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := d.Settings.String("notify.telegram_bot_token"); got != "current-tok" {
		t.Errorf("token = %q; empty PUT on a secret field must KEEP current", got)
	}
}

func TestHandleUpdateSettingUnknownKey(t *testing.T) {
	d := Deps{Settings: settings.NewForTest(nil)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/does.not.exist", strings.NewReader(`{"value":"1s"}`))
	req.SetPathValue("key", "does.not.exist")
	handleUpdateSetting(d)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown key", rec.Code)
	}
}

func TestHandleUpdateSettingInvalidValue(t *testing.T) {
	d := Deps{Settings: settings.NewForTest(nil)}
	rec := httptest.NewRecorder()
	// 150% violates the maximum (100) → validation rejects it before touching the database.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/alerting.disk_saturated_pct", strings.NewReader(`{"value":"150"}`))
	req.SetPathValue("key", "alerting.disk_saturated_pct")
	handleUpdateSetting(d)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid value", rec.Code)
	}
}
