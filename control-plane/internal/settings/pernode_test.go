package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// contractKey is an entry from shared/agent-config.json.
type contractKey struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	Env  string `json:"env"`
}

// TestPerNodeSpecsMatchSharedContract locks the CP↔Agent contract from ADR-039. The two Go
// modules don't import each other, so what the Control Plane delivers and what the Agent knows to apply only
// stay in sync if someone compares them: each side compares its table with
// shared/agent-config.json. Forgetting the Agent on a new key wouldn't break anything at runtime —
// the key would just silently never reach the Engine —, and that's what this test prevents.
func TestPerNodeSpecsMatchSharedContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "shared", "agent-config.json") // internal/settings → repository root
	raw, err := os.ReadFile(path)                                          //nolint:gosec // G304: fixed path to the repository itself
	if err != nil {
		t.Skipf("contract missing (module outside repository?): %v", err)
	}
	var contract struct {
		Keys []contractKey `json:"keys"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("shared/agent-config.json invalid: %v", err)
	}

	want := map[string]contractKey{}
	for _, k := range contract.Keys {
		want[k.Key] = k
	}
	got := map[string]Spec{}
	for _, s := range NodeSpecs() {
		got[s.Key] = s
	}

	var missing, extra []string
	for k := range want {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("in contract but no PerNode Spec in catalog: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("PerNode Spec in catalog but not in contract (Agent does not know it): %v", extra)
	}
	for k, spec := range got {
		c, ok := want[k]
		if !ok {
			continue
		}
		if string(spec.Kind) != c.Kind {
			t.Errorf("%s: type %q in catalog, %q in contract", k, spec.Kind, c.Kind)
		}
		if spec.AgentEnv != c.Env {
			t.Errorf("%s: env %q in catalog, %q in contract", k, spec.AgentEnv, c.Env)
		}
	}
}

// TestPerNodeSpecsInvariants: what every key delivered to the Agent must respect.
func TestPerNodeSpecsInvariants(t *testing.T) {
	specs := NodeSpecs()
	if len(specs) == 0 {
		t.Fatal("no PerNode tunables in catalog")
	}
	for _, s := range specs {
		if s.EnvVar != "" {
			// The EnvVar seeds the database from the CONTROL PLANE env; the env that matters
			// here is the Node's, and the Agent reports it (AgentEnv). Seeding from the CP env
			// would create an "explicit" cluster value that no one chose — and the seeding
			// of overrides (which only acts on NON-explicit cluster values) would halt.
			t.Errorf("%s: PerNode cannot have EnvVar (%q) — the seeded env would be the Control Plane's", s.Key, s.EnvVar)
		}
		if s.AgentEnv == "" {
			t.Errorf("%s: PerNode needs AgentEnv (the equivalent env on Node)", s.Key)
		}
		if s.Secret || s.Multi {
			t.Errorf("%s: PerNode does not allow Secret/Multi (value is transmitted in plaintext and compared as scalar)", s.Key)
		}
		if s.Category != CatAgent {
			t.Errorf("%s: category %q, want %q (Settings screen groups by category)", s.Key, s.Category, CatAgent)
		}
	}
	// A key that is NOT PerNode cannot carry a loose AgentEnv (it would be orphaned metadata).
	for _, s := range Specs() {
		if !s.PerNode && s.AgentEnv != "" {
			t.Errorf("%s: AgentEnv without PerNode", s.Key)
		}
	}
}

// Engine key limits exist so a bad cluster value doesn't propagate: a
// simultaneous download "0" or "-1" (unlimited in qBittorrent) is exactly what would bring down
// an Atom, and a 0s heartbeat would bring down the Agent's ticker.
func TestPerNodeLimitsRejectDangerousValues(t *testing.T) {
	cases := []struct {
		key, value string
		ok         bool
	}{
		{"engine.max_active_downloads", "3", true},
		{"engine.max_active_downloads", "10", true},
		{"engine.max_active_downloads", "0", false},    // 0 = no downloads
		{"engine.max_active_downloads", "-1", false},   // -1 = unlimited in qBittorrent
		{"engine.max_active_downloads", "1000", false}, // above the ceiling
		{"engine.max_active_checking", "2", true},
		{"engine.max_active_checking", "11", false},
		{"agent.heartbeat_interval", "10s", true},
		{"agent.heartbeat_interval", "0s", false}, // ticker.Reset(0) panics
		{"agent.heartbeat_interval", "1s", false}, // below the minimum
		{"agent.heartbeat_interval", "5m", false}, // the Node would appear offline (offline_after = 90s)
		{"agent.command_poll_interval", "5s", true},
		{"agent.command_poll_interval", "0s", false},
		{"agent.log_level", "debug", true},
		{"agent.log_level", "verbose", false},
		{"engine.queueing_enabled", "false", true},
		{"engine.queueing_enabled", "maybe", false},
		{"engine.slow_inactive_timer", "300", true},
		{"engine.slow_inactive_timer", "0", false},
	}
	for _, c := range cases {
		spec, ok := Lookup(c.key)
		if !ok {
			t.Fatalf("key %s missing from catalog", c.key)
		}
		if err := spec.Validate(c.value); (err == nil) != c.ok {
			t.Errorf("Validate(%s=%q) err=%v, want ok=%v", c.key, c.value, err, c.ok)
		}
	}
}

// Cluster defaults are what most of the fleet used in .env before ADR-039
// (inventory from 03/10/2026). Changing one means changing the behavior of every Node without an override.
func TestPerNodeDefaultsMatchFleetInventory(t *testing.T) {
	want := map[string]string{
		"agent.heartbeat_interval":    "10s", // the 4 Nodes used 10s (the code default is 30s)
		"agent.command_poll_interval": "5s",
		"agent.log_level":             "info",
		"engine.max_active_downloads": "10", // 3 of the 4 Nodes; node-04 (Atom) uses 3 via override
		"engine.queueing_enabled":     "true",
		"engine.max_active_uploads":   "10000",
		"engine.max_active_torrents":  "10000",
		"engine.max_active_checking":  "1",
		"engine.dont_count_slow":      "false",
		"engine.slow_dl_threshold":    "1",
		"engine.slow_ul_threshold":    "1",
		"engine.slow_inactive_timer":  "300",
	}
	for k, v := range want {
		spec, ok := Lookup(k)
		if !ok {
			t.Errorf("%s missing from catalog", k)
			continue
		}
		if spec.Default != v {
			t.Errorf("%s: default %q, want %q (fleet inventory)", k, spec.Default, v)
		}
	}
	if len(want) != len(NodeSpecs()) {
		t.Errorf("inventory covers %d keys, PerNode catalog has %d", len(want), len(NodeSpecs()))
	}
}

func TestSame(t *testing.T) {
	cases := []struct {
		key, a, b string
		same      bool
	}{
		{"agent.heartbeat_interval", "10s", "10s", true},
		{"agent.heartbeat_interval", "10s", "10000ms", true}, // same duration, different spelling
		{"agent.heartbeat_interval", "10s", "30s", false},
		{"agent.heartbeat_interval", "10s", "garbage", false}, // doesn't parse: it's never equal
		{"engine.max_active_downloads", "10", "10", true},
		{"engine.max_active_downloads", "3", "10", false},
		{"engine.max_active_downloads", "3", "x", false},
		{"engine.queueing_enabled", "true", "true", true},
		{"engine.queueing_enabled", "true", "false", false},
		{"engine.queueing_enabled", "true", "1", true}, // ParseBool accepts it; Validate is the one that validates the format
		{"agent.log_level", "info", "info", true},
		{"agent.log_level", "info", "debug", false},
	}
	for _, c := range cases {
		spec, _ := Lookup(c.key)
		if got := spec.Same(c.a, c.b); got != c.same {
			t.Errorf("%s.Same(%q, %q) = %v, want %v", c.key, c.a, c.b, got, c.same)
		}
	}
}

func TestClusterDistinguishesExplicitFromDefault(t *testing.T) {
	s := NewForTest(map[string]string{"engine.max_active_downloads": "6"})
	if v, explicit := s.Cluster("engine.max_active_downloads"); v != "6" || !explicit {
		t.Errorf("Cluster(explicit) = %q/%v, want 6/true", v, explicit)
	}
	if v, explicit := s.Cluster("agent.heartbeat_interval"); v != "10s" || explicit {
		t.Errorf("Cluster(default) = %q/%v, want 10s/false", v, explicit)
	}
	// Corrupted override in the database (manual edit): falls back to default and DOES NOT count as explicit.
	bad := NewForTest(map[string]string{"engine.max_active_downloads": "garbage"})
	if v, explicit := bad.Cluster("engine.max_active_downloads"); v != "10" || explicit {
		t.Errorf("Cluster(corrupted) = %q/%v, want 10/false", v, explicit)
	}
}
