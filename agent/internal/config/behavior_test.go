package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func baseline(t *testing.T) Behavior {
	t.Helper()
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	for _, k := range Keys() {
		t.Setenv(k.Env, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c.Behavior()
}

// The baseline is what the Agent uses without the Control Plane: env + code defaults
// (which DO NOT change with ADR-039 — the "old CP" fallback remains the usual behavior).
func TestBehaviorBaselineIsTheCodeDefaults(t *testing.T) {
	b := baseline(t)
	want := Behavior{
		HeartbeatInterval:   30 * time.Second,
		CommandPollInterval: 5 * time.Second,
		LogLevel:            "info",
		Queue: QueueSettings{
			Enabled: true, MaxActiveDownloads: 2, MaxActiveUploads: 10000, MaxActiveTorrents: 10000,
			MaxActiveChecking: 1, DontCountSlow: false, SlowDlThreshold: 1, SlowUlThreshold: 1, SlowInactiveTimer: 300,
		},
	}
	if b != want {
		t.Errorf("baseline = %+v, want %+v", b, want)
	}
}

func TestBehaviorBaselineRespectsEnv(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	t.Setenv("SEEDGRID_AGENT_HEARTBEAT_INTERVAL", "10s")
	t.Setenv("SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS", "3")
	t.Setenv("SEEDGRID_LOG_LEVEL", "debug")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	b := c.Behavior()
	if b.HeartbeatInterval != 10*time.Second || b.Queue.MaxActiveDownloads != 3 || b.LogLevel != "debug" {
		t.Errorf("baseline = %+v, want the Node env", b)
	}
}

// The CP sends the effective values and they override the env — this is the precedence of ADR-039.
func TestOverlayControlPlaneOverridesEnv(t *testing.T) {
	t.Setenv("SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS", "3")
	b := baseline(t)
	b.Queue.MaxActiveDownloads = 3 // the Node env

	got, rejected := b.Overlay(map[string]string{
		"engine.max_active_downloads": "10",
		"agent.heartbeat_interval":    "10s",
		"agent.command_poll_interval": "5s",
		"agent.log_level":             "warn",
		"engine.queueing_enabled":     "false",
		"engine.dont_count_slow":      "true",
		"engine.max_active_uploads":   "50",
		"engine.max_active_torrents":  "60",
		"engine.max_active_checking":  "2",
		"engine.slow_dl_threshold":    "4",
		"engine.slow_ul_threshold":    "5",
		"engine.slow_inactive_timer":  "120",
	})
	if len(rejected) != 0 {
		t.Fatalf("rejected valid values: %+v", rejected)
	}
	want := Behavior{
		HeartbeatInterval: 10 * time.Second, CommandPollInterval: 5 * time.Second, LogLevel: "warn",
		Queue: QueueSettings{
			Enabled: false, MaxActiveDownloads: 10, MaxActiveUploads: 50, MaxActiveTorrents: 60,
			MaxActiveChecking: 2, DontCountSlow: true, SlowDlThreshold: 4, SlowUlThreshold: 5, SlowInactiveTimer: 120,
		},
	}
	if got != want {
		t.Errorf("Overlay = %+v, want %+v", got, want)
	}
}

// Missing key from the map stays as it was; unknown key (newer CP) is silently
// ignored; nil map changes nothing.
func TestOverlayAbsentAndUnknown(t *testing.T) {
	b := baseline(t)
	got, rejected := b.Overlay(map[string]string{
		"engine.max_active_downloads": "7",
		"engine.future_key":           "anything",
	})
	if len(rejected) != 0 {
		t.Errorf("unknown key must not be rejected: %+v", rejected)
	}
	if got.Queue.MaxActiveDownloads != 7 || got.HeartbeatInterval != b.HeartbeatInterval || got.LogLevel != b.LogLevel {
		t.Errorf("Overlay = %+v, want only the changed downloads", got)
	}
	if same, _ := b.Overlay(nil); same != b {
		t.Errorf("Overlay(nil) changed Behavior: %+v", same)
	}
}

// The Agent defends itself: a value that would crash it is refused and the key KEEPS its
// previous value. ticker.Reset(0) panics; download ceiling ≤ 0 is not "unlimited" in qBittorrent.
func TestOverlayRefusesDangerousValue(t *testing.T) {
	b := baseline(t)
	got, rejected := b.Overlay(map[string]string{
		"agent.heartbeat_interval":    "0s",
		"agent.command_poll_interval": "-5s",
		"engine.max_active_downloads": "-1",
		"engine.max_active_checking":  "0",
		"engine.max_active_uploads":   "many",
		"engine.queueing_enabled":     "maybe",
		"agent.log_level":             "verbose",
		"engine.slow_inactive_timer":  "0",
		"engine.max_active_torrents":  "9", // this one is valid
	})
	if got.HeartbeatInterval != b.HeartbeatInterval || got.CommandPollInterval != b.CommandPollInterval ||
		got.LogLevel != b.LogLevel {
		t.Errorf("dangerous value was applied: %+v", got)
	}
	q := b.Queue
	q.MaxActiveTorrents = 9
	if got.Queue != q {
		t.Errorf("queue = %+v, want only max_active_torrents=9 (invalid ones keep the previous value)", got.Queue)
	}
	if len(rejected) != 8 {
		t.Errorf("rejections = %d, want 8: %+v", len(rejected), rejected)
	}
	for _, r := range rejected {
		if r.Key == "" || r.Reason == "" {
			t.Errorf("rejection without key/reason: %+v", r)
		}
	}
}

// Duration limits: the Agent's sanity is wider than the CP Spec (the operator's ruler),
// but it still blocks what would break a ticker.
func TestOverlayIntervalBounds(t *testing.T) {
	b := baseline(t)
	for _, c := range []struct {
		v  string
		ok bool
	}{
		{"1s", true}, {"10s", true}, {"60s", true}, {"10m", true},
		{"999ms", false}, {"0s", false}, {"11m", false}, {"abc", false}, {"", false},
	} {
		got, rejected := b.Overlay(map[string]string{"agent.heartbeat_interval": c.v})
		if (len(rejected) == 0) != c.ok {
			t.Errorf("heartbeat_interval=%q: rejected=%v, want ok=%v", c.v, len(rejected) > 0, c.ok)
		}
		if !c.ok && got.HeartbeatInterval != b.HeartbeatInterval {
			t.Errorf("heartbeat_interval=%q rejected but the value changed", c.v)
		}
	}
}

func TestValuesECanonicoEDiff(t *testing.T) {
	b := baseline(t)
	v := b.Values()
	if len(v) != len(Keys()) {
		t.Fatalf("Values has %d keys, table has %d", len(v), len(Keys()))
	}
	if v["agent.heartbeat_interval"] != "30s" || v["engine.max_active_downloads"] != "2" || v["engine.queueing_enabled"] != "true" {
		t.Errorf("Values = %v", v)
	}
	// What the Agent reads back with Overlay is the same: round trip without loss.
	if round, rej := (Behavior{}).Overlay(v); round != b || len(rej) != 0 {
		t.Errorf("Overlay(Values) = %+v (rej %v), want the same Behavior", round, rej)
	}
	next, _ := b.Overlay(map[string]string{"engine.max_active_downloads": "10"})
	d := Diff(b, next)
	if len(d) != 1 || d["engine.max_active_downloads"] != "2 → 10" {
		t.Errorf("Diff = %v, want only downloads 2 → 10", d)
	}
	if len(Diff(b, b)) != 0 {
		t.Error("Diff of equal values should be empty")
	}
}

// ExplicitEnv is the seeding base: only what the operator DEFINED, in the canonical
// form the catalog validates — "5000ms" and "5s" are the same duration and "1" is "true".
func TestExplicitEnvOnlyWhatWasDefined(t *testing.T) {
	env := map[string]string{
		"SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS": "3",
		"SEEDGRID_AGENT_HEARTBEAT_INTERVAL":    "10s",
		"SEEDGRID_AGENT_COMMAND_POLL_INTERVAL": "5000ms",
		"SEEDGRID_ENGINE_DONT_COUNT_SLOW":      "1",
		"SEEDGRID_LOG_LEVEL":                   "debug",
		// invalid: Load already ignores them (or fails), and they cannot become overrides
		"SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS":  "many",
		"SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING": "0",
		// empty and spaces only = not defined
		"SEEDGRID_ENGINE_SLOW_DL_THRESHOLD": "  ",
		// identity/machine: never included, even if defined
		"SEEDGRID_ENGINE_BT_PORT":       "6884",
		"SEEDGRID_AGENT_STORAGE_PATH":   "/data",
		"SEEDGRID_CONTROL_PLANE_URL":    "http://cp",
		"SEEDGRID_ENGINE_QBT_PASSWORD":  "secret",
		"SEEDGRID_AGENT_MIGRATION_PORT": "6900",
	}
	got := explicitFrom(func(k string) string { return env[k] })
	want := map[string]string{
		"engine.max_active_downloads": "3",
		"agent.heartbeat_interval":    "10s",
		"agent.command_poll_interval": "5s",
		"engine.dont_count_slow":      "true",
		"agent.log_level":             "debug",
	}
	if len(got) != len(want) {
		t.Errorf("ExplicitEnv = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ExplicitEnv[%s] = %q, want %q", k, got[k], v)
		}
	}
}

// With no behavior variables — the newly created Node from bootstrap — the report is EMPTY,
// not the defaults: if it were, every Node would be born stuck in them (the #144 bug).
func TestExplicitEnvEmptyForNewNode(t *testing.T) {
	if got := explicitFrom(func(string) string { return "" }); len(got) != 0 {
		t.Errorf("ExplicitEnv without env = %v, want empty", got)
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError,
		"": slog.LevelInfo, "INFO": slog.LevelInfo, "garbage": slog.LevelInfo,
	} {
		if got := ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestKeysMatchSharedContract locks the CP↔Agent contract from ADR-039: the Control
// Plane catalog delivers the keys from shared/agent-config.json, and the Agent needs to know how
// to apply each one — with the same type and the same env. Forgetting a key here wouldn't break
// anything at runtime: it simply would never reach the Engine.
func TestKeysMatchSharedContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "shared", "agent-config.json") // agent/internal/config → repository root
	raw, err := os.ReadFile(path)                                          //nolint:gosec // G304: fixed path of the repository itself
	if err != nil {
		t.Skipf("contract missing (module outside the repository?): %v", err)
	}
	// KeyInfo has no JSON tags (it's from the Agent, not the file format): read with a local struct.
	var file struct {
		Keys []struct {
			Key  string `json:"key"`
			Kind string `json:"kind"`
			Env  string `json:"env"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("invalid shared/agent-config.json: %v", err)
	}

	have := map[string]KeyInfo{}
	for _, k := range Keys() {
		have[k.Key] = k
	}
	if len(have) != len(Keys()) {
		t.Error("duplicate key in the Agent table")
	}
	want := map[string]KeyInfo{}
	for _, f := range file.Keys {
		k := KeyInfo{Key: f.Key, Env: f.Env, Kind: f.Kind}
		want[k.Key] = k
		got, ok := have[k.Key]
		if !ok {
			t.Errorf("the contract lists %s and the Agent does not know how to apply it — the CP would deliver it and it would be ignored", k.Key)
			continue
		}
		if got != k {
			t.Errorf("%s: Agent %+v, contract %+v", k.Key, got, k)
		}
	}
	for key := range have {
		if _, ok := want[key]; !ok {
			t.Errorf("the Agent knows how to apply %s, which is not in the contract (the CP would never deliver it)", key)
		}
	}
}

// Every behavior variable in the table must be read by Load — otherwise the baseline
// "Node env" and the seeding report would diverge (one would say 3, the other nothing).
func TestLoadAndTableReadAgree(t *testing.T) {
	t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
	for _, k := range Keys() {
		t.Setenv(k.Env, "")
	}
	t.Setenv("SEEDGRID_AGENT_HEARTBEAT_INTERVAL", "12s")
	t.Setenv("SEEDGRID_AGENT_COMMAND_POLL_INTERVAL", "7s")
	t.Setenv("SEEDGRID_LOG_LEVEL", "warn")
	t.Setenv("SEEDGRID_ENGINE_QUEUEING_ENABLED", "false")
	t.Setenv("SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS", "4")
	t.Setenv("SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS", "41")
	t.Setenv("SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS", "42")
	t.Setenv("SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING", "3")
	t.Setenv("SEEDGRID_ENGINE_DONT_COUNT_SLOW", "true")
	t.Setenv("SEEDGRID_ENGINE_SLOW_DL_THRESHOLD", "8")
	t.Setenv("SEEDGRID_ENGINE_SLOW_UL_THRESHOLD", "9")
	t.Setenv("SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER", "77")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fromLoad := c.Behavior().Values()
	fromEnv := explicitFrom(os.Getenv)
	if len(fromEnv) != len(Keys()) {
		t.Fatalf("explicitFrom read %d keys, table has %d", len(fromEnv), len(Keys()))
	}
	for k, v := range fromEnv {
		if fromLoad[k] != v {
			t.Errorf("%s: Load returned %q, table read %q", k, fromLoad[k], v)
		}
	}
}

// The example env is the reference for the Agent variables and the starting point for
// whoever sets up /etc/seedgrid-agent.env by hand. If it included an ACTIVE behavior
// variable, every Node configured from it would be born with that number — the Control
// Plane would read it as the operator's decision and seed it as an override, which is
// exactly the "new Node is born different" that #144 fixes. The behavior variables are
// COMMENTED in the example; this test prevents a future edit from activating them.
func TestEnvExampleDoesNotEnableBehavior(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "agent", "seedgrid-agent.env.example")
	raw, err := os.ReadFile(path) //nolint:gosec // G304: fixed path of the repository itself
	if err != nil {
		t.Skipf("sample env missing (module outside the repository?): %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	for _, k := range Keys() {
		mentioned := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, k.Env+"=") {
				t.Errorf("%s is ACTIVE in the sample env: a Node installed from it would start with this value instead of the cluster value (ADR-039)", k.Env)
			}
			if strings.HasPrefix(trimmed, "#"+k.Env+"=") {
				mentioned = true
			}
		}
		if !mentioned {
			t.Errorf("%s does not appear (commented out) in the sample env: anyone reading the file would not know it exists", k.Env)
		}
	}
}
