package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// BEHAVIOR configuration for the Agent and Engine (ADR-039, #144).
//
// What THIS file knows: which keys the Control Plane can send, how to validate them, and how to
// put them into a Behavior. What the rest of the Config stores — CP URL, token, ports,
// credentials, paths — is machine identity and stays only in the .env.
//
// Precedence: Node override > cluster value (both resolved by the Control Plane and
// delivered in the heartbeat response) > Node env > code default. This package covers
// the last two steps (Load) and knows how to override what the CP sends (Overlay).

// QueueSettings is the Engine queue (protection against I/O saturation) — the subset of
// behavior that goes to qBittorrent. Comparable in purpose: the Agent only reapplies it to the
// Engine when it changes.
type QueueSettings struct {
	Enabled            bool
	MaxActiveDownloads int
	MaxActiveUploads   int
	MaxActiveTorrents  int
	MaxActiveChecking  int
	DontCountSlow      bool
	SlowDlThreshold    int // KB/s
	SlowUlThreshold    int // KB/s
	SlowInactiveTimer  int // seconds
}

// Behavior gathers the behavior parameters that the Control Plane can change on the fly.
type Behavior struct {
	HeartbeatInterval   time.Duration
	CommandPollInterval time.Duration
	LogLevel            string
	Queue               QueueSettings
}

// Behavior extracts from a Config (env + code defaults) the behavior baseline: what the
// Agent uses when the Control Plane sends nothing.
func (c Config) Behavior() Behavior {
	return Behavior{
		HeartbeatInterval:   c.HeartbeatInterval,
		CommandPollInterval: c.CommandPollInterval,
		LogLevel:            c.LogLevel,
		Queue: QueueSettings{
			Enabled:            c.QueueingEnabled,
			MaxActiveDownloads: c.MaxActiveDownloads,
			MaxActiveUploads:   c.MaxActiveUploads,
			MaxActiveTorrents:  c.MaxActiveTorrents,
			MaxActiveChecking:  c.MaxActiveCheckingTorrents,
			DontCountSlow:      c.DontCountSlowTorrents,
			SlowDlThreshold:    c.SlowTorrentDlRateThreshold,
			SlowUlThreshold:    c.SlowTorrentUlRateThreshold,
			SlowInactiveTimer:  c.SlowTorrentInactiveTimer,
		},
	}
}

// Sanity ranges for what the Agent accepts. They are NOT the operator's standard — that is the
// catalog Spec in the Control Plane (which rejects the value at the source). They are the Agent's own
// defense against a value that would crash it: a ticker with duration ≤ 0 panics, and a download
// limit ≤ 0 is not "no limit" in qBittorrent.
const (
	minInterval = time.Second
	maxInterval = 10 * time.Minute
)

// keyDef describes a behavior key: the name in the Control Plane catalog, the equivalent
// env in the Node, and how to validate, apply, and serialize the value.
type keyDef struct {
	Key  string
	Env  string
	Kind string // duration | int | bool | string — mirrors shared/agent-config.json
	// set validates raw and, if good, writes to b. The error is readable (goes to the Agent log).
	set func(b *Behavior, raw string) error
	// get serializes the current value of b in the canonical form used by the catalog.
	get func(b Behavior) string
}

func durationKey(key, env string, field func(*Behavior) *time.Duration) keyDef {
	return keyDef{
		Key: key, Env: env, Kind: "duration",
		set: func(b *Behavior, raw string) error {
			d, err := time.ParseDuration(strings.TrimSpace(raw))
			if err != nil {
				return fmt.Errorf("invalid duration: %w", err)
			}
			if d < minInterval || d > maxInterval {
				return fmt.Errorf("duration %s out of [%s, %s]", d, minInterval, maxInterval)
			}
			*field(b) = d
			return nil
		},
		get: func(b Behavior) string { return (*field(&b)).String() },
	}
}

func intKey(key, env string, min int, field func(*Behavior) *int) keyDef {
	return keyDef{
		Key: key, Env: env, Kind: "int",
		set: func(b *Behavior, raw string) error {
			n, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				return fmt.Errorf("invalid integer: %w", err)
			}
			if n < min {
				return fmt.Errorf("value %d below minimum (%d)", n, min)
			}
			*field(b) = n
			return nil
		},
		get: func(b Behavior) string { return strconv.Itoa(*field(&b)) },
	}
}

func boolKey(key, env string, field func(*Behavior) *bool) keyDef {
	return keyDef{
		Key: key, Env: env, Kind: "bool",
		set: func(b *Behavior, raw string) error {
			v, err := strconv.ParseBool(strings.TrimSpace(raw))
			if err != nil {
				return fmt.Errorf("invalid boolean: %w", err)
			}
			*field(b) = v
			return nil
		},
		get: func(b Behavior) string { return strconv.FormatBool(*field(&b)) },
	}
}

// logLevels are the values of agent.log_level (the catalog Spec offers the same ones).
var logLevels = []string{"debug", "info", "warn", "error"}

// keyDefs is the behavior key table. shared/agent-config.json is the contract with the
// Control Plane catalog (control-plane/internal/settings/catalog.go, PerNode=true): both
// sides compare THEIR table against it, so a new key forgotten on one side breaks the CI.
var keyDefs = []keyDef{
	durationKey("agent.heartbeat_interval", "SEEDGRID_AGENT_HEARTBEAT_INTERVAL",
		func(b *Behavior) *time.Duration { return &b.HeartbeatInterval }),
	durationKey("agent.command_poll_interval", "SEEDGRID_AGENT_COMMAND_POLL_INTERVAL",
		func(b *Behavior) *time.Duration { return &b.CommandPollInterval }),
	{
		Key: "agent.log_level", Env: "SEEDGRID_LOG_LEVEL", Kind: "string",
		set: func(b *Behavior, raw string) error {
			v := strings.TrimSpace(raw)
			for _, l := range logLevels {
				if v == l {
					b.LogLevel = v
					return nil
				}
			}
			return fmt.Errorf("unknown log level %q (accepted: %s)", v, strings.Join(logLevels, ", "))
		},
		get: func(b Behavior) string { return b.LogLevel },
	},
	boolKey("engine.queueing_enabled", "SEEDGRID_ENGINE_QUEUEING_ENABLED",
		func(b *Behavior) *bool { return &b.Queue.Enabled }),
	intKey("engine.max_active_downloads", "SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS", 1,
		func(b *Behavior) *int { return &b.Queue.MaxActiveDownloads }),
	intKey("engine.max_active_uploads", "SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS", 1,
		func(b *Behavior) *int { return &b.Queue.MaxActiveUploads }),
	intKey("engine.max_active_torrents", "SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS", 1,
		func(b *Behavior) *int { return &b.Queue.MaxActiveTorrents }),
	intKey("engine.max_active_checking", "SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING", 1,
		func(b *Behavior) *int { return &b.Queue.MaxActiveChecking }),
	boolKey("engine.dont_count_slow", "SEEDGRID_ENGINE_DONT_COUNT_SLOW",
		func(b *Behavior) *bool { return &b.Queue.DontCountSlow }),
	intKey("engine.slow_dl_threshold", "SEEDGRID_ENGINE_SLOW_DL_THRESHOLD", 0,
		func(b *Behavior) *int { return &b.Queue.SlowDlThreshold }),
	intKey("engine.slow_ul_threshold", "SEEDGRID_ENGINE_SLOW_UL_THRESHOLD", 0,
		func(b *Behavior) *int { return &b.Queue.SlowUlThreshold }),
	intKey("engine.slow_inactive_timer", "SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER", 1,
		func(b *Behavior) *int { return &b.Queue.SlowInactiveTimer }),
}

// Rejection is a Control Plane value that the Agent rejected, along with the reason.
type Rejection struct {
	Key    string
	Value  string
	Reason string
}

// Overlay overrides b with the values sent by the Control Plane (catalog key → value). A
// value that the Agent does not accept is REJECTED — the key keeps the value it already had in b — and goes
// into rejected; a key that the Agent does not know (newer Control Plane) is silently
// ignored: this is forward compatibility.
func (b Behavior) Overlay(remote map[string]string) (out Behavior, rejected []Rejection) {
	out = b
	for _, kd := range keyDefs {
		raw, ok := remote[kd.Key]
		if !ok {
			continue
		}
		next := out
		if err := kd.set(&next, raw); err != nil {
			rejected = append(rejected, Rejection{Key: kd.Key, Value: raw, Reason: err.Error()})
			continue
		}
		out = next
	}
	return out, rejected
}

// Values serializes b the same way the Control Plane does (for logging and comparison).
func (b Behavior) Values() map[string]string {
	m := make(map[string]string, len(keyDefs))
	for _, kd := range keyDefs {
		m[kd.Key] = kd.get(b)
	}
	return m
}

// Diff returns, per key, "before → after" for the values that differ between a and b — what the
// Agent logs when applying a change.
func Diff(a, b Behavior) map[string]string {
	out := map[string]string{}
	for _, kd := range keyDefs {
		if x, y := kd.get(a), kd.get(b); x != y {
			out[kd.Key] = x + " → " + y
		}
	}
	return out
}

// ExplicitEnv returns the behavior values that the OPERATOR defined in this Node's env
// (catalog key → canonical value). Only variables that are actually present and
// valid are included: a code default is no one's decision, and reporting it as such would pin
// every new Node to the defaults (the bug that #144 fixes). This is what the Agent reports to the Control
// Plane when asked, for the seeding of overrides (ADR-039 §7).
func ExplicitEnv() map[string]string {
	return explicitFrom(os.Getenv)
}

func explicitFrom(getenv func(string) string) map[string]string {
	out := map[string]string{}
	var scratch Behavior
	for _, kd := range keyDefs {
		raw := getenv(kd.Env)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		b := scratch
		if err := kd.set(&b, raw); err != nil {
			continue // invalid env: the Agent already ignores it on Load; it doesn't become an override
		}
		out[kd.Key] = kd.get(b)
	}
	return out
}

// ParseLevel converts the log level (debug|info|warn|error) to slog.Level. Any other
// text becomes info, as the Agent has always done.
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// KeyInfo describes a behavior key that the Agent knows how to apply.
type KeyInfo struct{ Key, Env, Kind string }

// Keys returns the behavior keys that the Agent knows how to apply, in table order (used
// by the contract tests with shared/agent-config.json).
func Keys() []KeyInfo {
	out := make([]KeyInfo, 0, len(keyDefs))
	for _, kd := range keyDefs {
		out = append(out, KeyInfo{Key: kd.Key, Env: kd.Env, Kind: kd.Kind})
	}
	return out
}
