// Package remotecfg applies the behavior configuration to the Agent that the Control Plane
// delivers in the heartbeat response (ADR-039, #144).
//
// Precedence: Node override > cluster value (the CP already resolves them and sends the
// EFFECTIVE value) > Node env > code default. The env and default are the Agent's BASELINE
// (config.Config.Behavior); this package overrides whatever the CP sends and hot-applies what
// changed — log level, intervals, and the Engine queue — without restarting.
//
// Three rules guide the design:
//
//  1. Never bring down the Node. The heartbeat body is decoded by the CP with
//     DisallowUnknownFields, so a new field sent to an old CP would result in 400 (Node
//     OFFLINE). Therefore `config_version` and `local_config` only enter the body AFTER the CP
//     proves, in a previous response, that it understands `agent_config` — and any failure resets this
//     proof (a CP rollback prevents the Agent from sending a field the CP doesn't know).
//  2. The CP sends the entire DESIRED STATE on every heartbeat. Losing one costs nothing: the
//     next one fixes it. And applying is idempotent: only what changed is touched.
//  3. A bad value is not applied. The Agent validates what it receives (config.Behavior.Overlay) and,
//     if it rejected something, it DOES NOT declare the version as applied — the Portal shows "waiting".
package remotecfg

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
	"github.com/rbferreira/seedgrid/agent/internal/config"
)

// bootGrace is how many UNRESOLVED heartbeats (CP unreachable, or still requesting the env
// report) the Agent waits, on boot, before applying the Node env. Without the CP the Agent needs
// to work — but in a normal boot the CP responds on the first heartbeat, and applying the env before
// that would make the Engine flash to the old value (and with it, trigger a burst of downloads
// that the cluster ceiling would undo a second later).
const bootGrace = 3

// Hooks are the effects triggered by the Manager. Each one is called only when the corresponding
// value has changed (or on the first application).
type Hooks struct {
	// SetLogLevel changes the log level (slog.LevelVar).
	SetLogLevel func(level string)
	// SetHeartbeatInterval changes the heartbeat interval (the loop recreates the ticker).
	SetHeartbeatInterval func(d time.Duration)
	// SetPollInterval changes the command polling interval.
	SetPollInterval func(d time.Duration)
	// ApplyEngine applies the queue to the Engine. first is true on the first application of the process — the
	// caller uses it to also pin down machine-specific settings (BT port, save_path), which the
	// cluster configuration never changes.
	ApplyEngine func(ctx context.Context, q config.QueueSettings, first bool) error
	// EngineAdopted: the Engine is the operator's own qBittorrent (ADR-043) and ApplyEngine does
	// not touch it. The engine.* values then stay out of what the log reports as applied.
	EngineAdopted bool
}

// Manager holds the state of the negotiation and what has been applied. The methods are safe for concurrent
// use, but normal usage is from a single goroutine (the heartbeat one).
type Manager struct {
	logger   *slog.Logger
	env      config.Behavior   // baseline: Node env + code defaults
	explicit map[string]string // explicit behavior env (seeding report)
	hooks    Hooks

	mu sync.Mutex

	// Proof that the CP understands the negotiation: the last successful response included
	// agent_config. Resets on any failure.
	speaks bool
	// report: the CP requested the env report (report_local) in the last response.
	report bool

	// State of what is applied.
	resolved     bool // the first application has already occurred (boot completed)
	misses       int  // unresolved heartbeats since boot
	cur          config.Behavior
	logApplied   bool
	ivalsApplied bool
	engineOK     bool
	engineBooted bool
	target       config.Behavior // last chosen target (to reapply the Engine if it failed)
	// version is the CP version successfully APPLIED ("" = none: the Agent runs on env).
	version      string
	lastEngineEr string
}

// New creates the Manager. baseline is the env+defaults behavior (cfg.Behavior()); explicit is
// the explicit env (config.ExplicitEnv()).
func New(logger *slog.Logger, baseline config.Behavior, explicit map[string]string, hooks Hooks) *Manager {
	return &Manager{logger: logger, env: baseline, explicit: explicit, hooks: hooks, cur: baseline, target: baseline}
}

// Prepare fills the negotiation fields in the heartbeat to be sent — and ONLY them if the
// CP has already proven it understands them. Against an old CP (or after a failure) the body goes out
// exactly as it always has.
func (m *Manager) Prepare(req *client.HeartbeatRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.speaks {
		return
	}
	if m.version != "" {
		req.ConfigVersion = m.version
	}
	if m.report {
		local := make(map[string]string, len(m.explicit))
		for k, v := range m.explicit {
			local[k] = v
		}
		// Pointer to a possibly EMPTY map: the empty report needs to go — it's what
		// marks the seeding as done on the CP.
		req.LocalConfig = &local
	}
}

// NeedsFollowUp says whether it's worth sending another heartbeat right now, without waiting for the ticker: the CP requested the env
// report and the Agent is holding the application until it reports. Without this, a new Node would wait a
// full interval (30s in the code default) with the Engine still missing the pinned BT port.
func (m *Manager) NeedsFollowUp() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.report
}

// Handle processes the outcome of a heartbeat: err != nil is a failure (including 401). On success,
// resp may or may not include agent_config.
func (m *Manager) Handle(ctx context.Context, resp client.HeartbeatResponse, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case err != nil:
		// The proof that the CP understands the negotiation expires: if the CP comes back from a rollback, the
		// Agent doesn't send a new field on the first attempt. What is already applied remains.
		m.speaks, m.report = false, false
		m.unresolved(ctx)

	case resp.AgentConfig == nil:
		// CP prior to ADR-039 (or that failed to resolve): stays on env. What's already applied
		// is not undone — the Agent keeps the last value until the CP sends another one, or until
		// it restarts (then it starts from env). Only the boot needs to wrap up: apply the env, once.
		m.speaks, m.report = false, false
		if !m.resolved {
			m.apply(ctx, m.env, "", "env (Control Plane did not send configuration)")
		} else {
			m.retryEngine(ctx)
		}

	case resp.AgentConfig.ReportLocal:
		// Seeding hasn't happened yet: the Agent DOES NOT apply anything — the cluster value that just came
		// in might be exactly what would overload this Node, and the seeding (which runs on the
		// CP when the report arrives) exists to protect the env value. Reports on the next
		// heartbeat and applies what comes back.
		m.speaks, m.report = true, true
		m.unresolved(ctx)

	default:
		m.speaks, m.report = true, false
		target, rejected := m.env.Overlay(resp.AgentConfig.Values)
		for _, r := range rejected {
			m.logger.Warn("Control Plane configuration value rejected; keeping previous",
				"key", r.Key, "value", r.Value, "reason", r.Reason)
		}
		version := resp.AgentConfig.Version
		if len(rejected) > 0 {
			// Does not declare the version as applied: the Portal should show "waiting".
			version = ""
		}
		m.apply(ctx, target, version, "control-plane")
	}
}

// unresolved counts an unresolved heartbeat on boot and, after the grace period, applies the env: without
// the CP the Agent needs to work.
func (m *Manager) unresolved(ctx context.Context) {
	if m.resolved {
		m.retryEngine(ctx)
		return
	}
	m.misses++
	if m.misses >= bootGrace {
		m.apply(ctx, m.env, "", "env (Control Plane did not respond in time)")
	}
}

// apply brings the Agent to the target, hot-applying only what changed. version is what will be reported
// as applied when EVERYTHING went right ("" = nothing to report).
func (m *Manager) apply(ctx context.Context, target config.Behavior, version, source string) {
	prev := m.cur
	first := !m.resolved
	ok := true

	if !m.logApplied || target.LogLevel != m.cur.LogLevel {
		if m.hooks.SetLogLevel != nil {
			m.hooks.SetLogLevel(target.LogLevel)
		}
		m.cur.LogLevel, m.logApplied = target.LogLevel, true
	}
	// Each interval only triggers its hook when it has CHANGED: reconnecting the Poller due to a change in the
	// heartbeat would restart the polling phase for no reason.
	if !m.ivalsApplied || target.HeartbeatInterval != m.cur.HeartbeatInterval {
		if m.hooks.SetHeartbeatInterval != nil {
			m.hooks.SetHeartbeatInterval(target.HeartbeatInterval)
		}
		m.cur.HeartbeatInterval = target.HeartbeatInterval
	}
	if !m.ivalsApplied || target.CommandPollInterval != m.cur.CommandPollInterval {
		if m.hooks.SetPollInterval != nil {
			m.hooks.SetPollInterval(target.CommandPollInterval)
		}
		m.cur.CommandPollInterval = target.CommandPollInterval
	}
	m.ivalsApplied = true
	m.target = target
	if !m.engineOK || target.Queue != m.cur.Queue {
		m.engineOK = false
		ok = m.applyEngine(ctx, target.Queue) && ok
	}

	m.resolved = true
	if ok {
		m.version = version
	}
	if first {
		if m.hooks.EngineAdopted {
			m.logger.Info("Agent configuration applied", "source", source, "version", version,
				"values", m.applied(m.cur.Values()), "engine", "not applied: the operator's own qBittorrent (ADR-043)")
		} else {
			m.logger.Info("Agent configuration applied", "source", source, "version", version, "values", m.cur.Values())
		}
	} else if changed := m.applied(config.Diff(prev, m.cur)); len(changed) > 0 {
		m.logger.Info("Agent configuration hot-reloaded", "source", source, "version", version, "changes", sortedPairs(changed))
	}
}

// applied drops the engine.* keys from values when the Engine is adopted (ADR-043): they reach
// the Agent, but it never writes them to that Engine, and the log must not say otherwise.
func (m *Manager) applied(values map[string]string) map[string]string {
	if !m.hooks.EngineAdopted {
		return values
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if !strings.HasPrefix(k, "engine.") {
			out[k] = v
		}
	}
	return out
}

// applyEngine applies the queue to the Engine; failure is recorded for the next heartbeat to retry.
func (m *Manager) applyEngine(ctx context.Context, q config.QueueSettings) bool {
	if m.hooks.ApplyEngine == nil {
		m.cur.Queue, m.engineOK = q, true
		return true
	}
	if err := m.hooks.ApplyEngine(ctx, q, !m.engineBooted); err != nil {
		// Warn the first time and when the error changes; the rest is Debug — with the Engine down
		// for hours, a Warn every 10s would just be noise.
		if msg := err.Error(); msg != m.lastEngineEr {
			m.logger.Warn("failed to apply queue to Engine; retrying on next heartbeat", "error", err)
			m.lastEngineEr = msg
		} else {
			m.logger.Debug("Engine queue not yet applied", "error", err)
		}
		return false
	}
	m.cur.Queue, m.engineOK, m.engineBooted, m.lastEngineEr = q, true, true, ""
	return true
}

// retryEngine repeats the queue application when the previous one failed, without touching the rest.
func (m *Manager) retryEngine(ctx context.Context) {
	if m.engineOK || !m.resolved {
		return
	}
	if m.applyEngine(ctx, m.target.Queue) {
		m.logger.Info("Engine queue applied", "values", m.cur.Values())
	}
}

// Applied returns the applied behavior and the CP version that originated it ("" = env).
func (m *Manager) Applied() (config.Behavior, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cur, m.version
}

func sortedPairs(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+": "+v)
	}
	sort.Strings(out)
	return out
}
