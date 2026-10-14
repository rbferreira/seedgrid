package settings

import (
	"regexp"
	"testing"
	"time"
)

// TestCatalogDefaultsValid confirms that every catalog default is parseable and
// respects its own limits (init panics otherwise; this test
// documents and fails legibly if someone introduces an invalid default).
func TestCatalogDefaultsValid(t *testing.T) {
	for _, s := range Specs() {
		if err := s.Validate(s.Default); err != nil {
			t.Errorf("%s: invalid default %q: %v", s.Key, s.Default, err)
		}
		if s.Label == "" || s.Description == "" {
			t.Errorf("%s: label/description required in panel", s.Key)
		}
	}
}

// internalRef matches design-record and issue references ("ADR-032", "#112"). They mean
// something to whoever reads the code, not to the operator reading the panel — and an issue
// number may point to a tracker the reader has no access to.
var internalRef = regexp.MustCompile(`ADR-\d+|#\d+`)

// TestCatalogTextHasNoInternalRefs keeps references out of what the panel shows. Comments in the
// code keep theirs.
func TestCatalogTextHasNoInternalRefs(t *testing.T) {
	for _, s := range Specs() {
		texts := []string{s.Label, s.Description}
		for _, c := range s.Choices {
			texts = append(texts, c.Label)
		}
		for _, text := range texts {
			if ref := internalRef.FindString(text); ref != "" {
				t.Errorf("%s: user-facing text cites %q: %q", s.Key, ref, text)
			}
		}
	}
}

func TestLookup(t *testing.T) {
	if _, ok := Lookup("admission.dead_grace"); !ok {
		t.Fatal("expected to find admission.dead_grace")
	}
	if _, ok := Lookup("nonexistent.key"); ok {
		t.Fatal("should not find nonexistent key")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		key, value string
		ok         bool
	}{
		{"admission.dead_grace", "9m", true},
		{"admission.dead_grace", "0s", true},      // 0 disables (AllowZero exception to min)
		{"admission.dead_grace", "3m", false},     // below minimum (300s) and != 0 — false positive
		{"admission.dead_grace", "banana", false}, // not a duration
		{"admission.dead_grace", "999h", false},   // above maximum (86400s)
		{"admission.min_dl_rate", "51200", true},  // int64
		{"admission.min_dl_rate", "0", true},      // 0 disables slow rotation
		{"admission.min_dl_rate", "-1", false},    // below minimum (0)
		{"admission.min_dl_rate", "12.5", false},  // not an integer
		{"admission.slow_grace", "12m", true},
		{"admission.slow_grace", "8m", false},         // below minimum (600s) — no exception (disabling is via min_dl_rate=0)
		{"alerting.disk_saturated_pct", "90", true},   // float
		{"alerting.disk_saturated_pct", "101", false}, // above 100
		{"alerting.disk_saturated_for", "90s", true},  // default
		{"alerting.disk_saturated_for", "0s", true},   // 0 = opens on first read (old behavior)
		{"alerting.disk_saturated_for", "2h", false},  // above maximum (3600s)
		{"alerting.disk_saturated_for", "-1s", false}, // below minimum (0)
		{"admission.max_cluster_downloads", "5", true},
		// NODE_INBOUND_SILENT (#206) weighted demand: float >= 0; 0 is the strictest reading
		// (only swarm with no seed), not "disabled".
		{"alerting.incoming_demand_seeds_per_leecher", "1", true},
		{"alerting.incoming_demand_seeds_per_leecher", "2.5", true},
		{"alerting.incoming_demand_seeds_per_leecher", "0", true},
		{"alerting.incoming_demand_seeds_per_leecher", "100000", true},
		{"alerting.incoming_demand_seeds_per_leecher", "-1", false},
		{"alerting.incoming_demand_seeds_per_leecher", "many", false},
		// Free KindString (ADR-027): any text, including empty (= disabled).
		{"agent.artifacts_url", "http://controller.example/node", true},
		{"agent.artifacts_url", "", true},
		// KindString with Options = validated enum (ADR-028).
		{"notify.alerts_min_severity", "CRITICAL", true},
		{"notify.alerts_min_severity", "OFF", true},
		{"notify.alerts_min_severity", "critical", false}, // case-sensitive: not one of the options
		{"notify.alerts_min_severity", "URGENT", false},   // outside the enum
		// KindBool (ADR-028).
		{"notify.alerts_on_resolve", "true", true},
		{"notify.alerts_on_resolve", "false", true},
		{"notify.alerts_on_resolve", "yes", false},
		{"notify.alerts_on_resolve", "1", false}, // only true/false, no ambiguity
		// Secret is a regular KindString in validation (the secrecy is of the API, not the value).
		{"notify.telegram_bot_token", "123456:ABC-DEF", true},
	}
	for _, c := range cases {
		spec, ok := Lookup(c.key)
		if !ok {
			t.Fatalf("key %s missing from catalog", c.key)
		}
		err := spec.Validate(c.value)
		if (err == nil) != c.ok {
			t.Errorf("Validate(%s=%q) err=%v, want ok=%v", c.key, c.value, err, c.ok)
		}
	}
}

// TestForTestGetters exercises the typed getters without a database (in-memory Store).
func TestForTestGetters(t *testing.T) {
	s := NewForTest(map[string]string{
		"admission.dead_grace":  "7m",
		"admission.min_dl_rate": "102400",
	})
	if got := s.Duration("admission.dead_grace"); got.String() != "7m0s" {
		t.Errorf("dead_grace = %v, want 7m", got)
	}
	if got := s.Int64("admission.min_dl_rate"); got != 102400 {
		t.Errorf("min_dl_rate = %d, want 102400", got)
	}
	// No override → catalog default.
	if got := s.Duration("admission.slow_grace"); got.String() != "10m0s" {
		t.Errorf("slow_grace (default) = %v, want 10m", got)
	}
	if got := s.Float("alerting.disk_saturated_pct"); got != 90 {
		t.Errorf("disk_pct (default) = %v, want 90", got)
	}
}

// TestGetterCorruptFallsBackToDefault ensures that a corrupt override does not
// crash the worker: the getter falls back to the catalog default.
func TestGetterCorruptFallsBackToDefault(t *testing.T) {
	s := NewForTest(map[string]string{"admission.interval": "garbage"})
	if got := s.Duration("admission.interval"); got.String() != "20s" {
		t.Errorf("corrupted interval = %v, want fallback 20s", got)
	}
}

func TestUnknownKeyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for unknown key")
		}
	}()
	NewForTest(nil).Duration("does.not.exist")
}

// The NODE_INBOUND_SILENT (#206) defaults were calibrated on production data; this
// test exists so that changing them is a decision, not a side effect.
func TestIncomingSilenceDefaults(t *testing.T) {
	st := NewForTest(nil)
	if got := st.Duration("alerting.incoming_silence"); got != 240*time.Hour {
		t.Errorf("alerting.incoming_silence = %v, expected 240h", got)
	}
	if got := st.Float("alerting.incoming_demand_seeds_per_leecher"); got != 1 {
		t.Errorf("alerting.incoming_demand_seeds_per_leecher = %v, expected 1", got)
	}
}
