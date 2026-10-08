package remotecfg

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
	"github.com/rbferreira/seedgrid/agent/internal/config"
)

// recorder records the effects triggered by the Manager.
type recorder struct {
	logLevels []string
	heartbeat []time.Duration
	poll      []time.Duration
	engine    []engineCall
	engineErr error // error returned by ApplyEngine while != nil
}

type engineCall struct {
	q     config.QueueSettings
	first bool
}

func (r *recorder) hooks() Hooks {
	return Hooks{
		SetLogLevel:          func(l string) { r.logLevels = append(r.logLevels, l) },
		SetHeartbeatInterval: func(d time.Duration) { r.heartbeat = append(r.heartbeat, d) },
		SetPollInterval:      func(d time.Duration) { r.poll = append(r.poll, d) },
		ApplyEngine: func(_ context.Context, q config.QueueSettings, first bool) error {
			r.engine = append(r.engine, engineCall{q, first})
			return r.engineErr
		},
	}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// envBaseline is what the Agent uses without CP: node-04's env (3 downloads, 10s heartbeat).
func envBaseline() config.Behavior {
	return config.Behavior{
		HeartbeatInterval:   10 * time.Second,
		CommandPollInterval: 5 * time.Second,
		LogLevel:            "info",
		Queue: config.QueueSettings{
			Enabled: true, MaxActiveDownloads: 3, MaxActiveUploads: 10000, MaxActiveTorrents: 10000,
			MaxActiveChecking: 1, SlowDlThreshold: 1, SlowUlThreshold: 1, SlowInactiveTimer: 300,
		},
	}
}

func clusterValues(downloads string) map[string]string {
	return map[string]string{
		"agent.heartbeat_interval":    "10s",
		"agent.command_poll_interval": "5s",
		"agent.log_level":             "info",
		"engine.queueing_enabled":     "true",
		"engine.max_active_downloads": downloads,
		"engine.max_active_uploads":   "10000",
		"engine.max_active_torrents":  "10000",
		"engine.max_active_checking":  "1",
		"engine.dont_count_slow":      "false",
		"engine.slow_dl_threshold":    "1",
		"engine.slow_ul_threshold":    "1",
		"engine.slow_inactive_timer":  "300",
	}
}

func resp(version string, values map[string]string, reportLocal bool) client.HeartbeatResponse {
	return client.HeartbeatResponse{Status: "ok", AgentConfig: &client.AgentConfig{Version: version, Values: values, ReportLocal: reportLocal}}
}

var errBoom = errors.New("boom")

// Before the CP proves it understands the negotiation, the heartbeat body goes out as it ALWAYS did:
// no new fields. This is what protects a Node whose Agent was updated before the CP.
func TestPrepareDoesNotSendNewFieldWithoutCPProof(t *testing.T) {
	m := New(quiet(), envBaseline(), map[string]string{"engine.max_active_downloads": "3"}, (&recorder{}).hooks())
	var req client.HeartbeatRequest
	m.Prepare(&req)
	if req.ConfigVersion != "" || req.LocalConfig != nil {
		t.Errorf("heartbeat from an Agent that never talked to a new CP carries negotiation fields: %+v", req)
	}
}

// Old CP (response without agent_config): the Agent sticks to the env, applies it ONCE and never sends
// a new field — in any cycle.
func TestOldCPAgentContinuesInEnvAndNoNewField(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{"engine.max_active_downloads": "3"}, rec.hooks())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		var req client.HeartbeatRequest
		m.Prepare(&req)
		if req.ConfigVersion != "" || req.LocalConfig != nil {
			t.Fatalf("cycle %d: negotiation field sent to a CP that does not understand it: %+v", i, req)
		}
		m.Handle(ctx, client.HeartbeatResponse{Status: "ok"}, nil)
	}
	if len(rec.engine) != 1 || !rec.engine[0].first || rec.engine[0].q.MaxActiveDownloads != 3 {
		t.Errorf("Engine = %+v, want ONE application (the first, complete) with env: downloads=3", rec.engine)
	}
	if len(rec.logLevels) != 1 || len(rec.heartbeat) != 1 || len(rec.poll) != 1 {
		t.Errorf("hooks applied more than once: log=%v hb=%v poll=%v", rec.logLevels, rec.heartbeat, rec.poll)
	}
	if _, v := m.Applied(); v != "" {
		t.Errorf("version = %q, want empty (runs in env)", v)
	}
}

// The happy path with a new CP and the Node already seeded: it applies the effective values on the fly,
// and starts reporting the version — but only from the heartbeat AFTER the one that brought it.
func TestNewCPAppliesAndReportsVersion(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	ctx := context.Background()

	var req client.HeartbeatRequest
	m.Prepare(&req) // 1st heartbeat: still no proof
	if req.ConfigVersion != "" {
		t.Fatalf("1st heartbeat already sent config_version: %+v", req)
	}
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)

	if len(rec.engine) != 1 || !rec.engine[0].first || rec.engine[0].q.MaxActiveDownloads != 10 {
		t.Errorf("Engine = %+v, want the 1st application with downloads=10 (CP wins over env=3)", rec.engine)
	}
	if b, v := m.Applied(); v != "v1" || b.Queue.MaxActiveDownloads != 10 {
		t.Errorf("Applied = %+v / %q, want downloads=10 and v1", b, v)
	}

	req = client.HeartbeatRequest{}
	m.Prepare(&req)
	if req.ConfigVersion != "v1" {
		t.Errorf("config_version = %q, want v1 (CP proved it understands and Agent applied it)", req.ConfigVersion)
	}
	if req.LocalConfig != nil {
		t.Errorf("local_config sent without CP asking: %v", *req.LocalConfig)
	}
}

// The change arrives on the fly and ONLY what changed is reapplied: tweaking an Engine limit must not
// recreate the ticker or change the log level.
func TestHotChangeAppliesOnlyWhatChanged(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	ctx := context.Background()
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)
	rec.logLevels, rec.heartbeat, rec.poll, rec.engine = nil, nil, nil, nil

	// 1. Same content, again: nothing to do (the CP sends the entire state on every heartbeat).
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)
	if len(rec.engine)+len(rec.logLevels)+len(rec.heartbeat)+len(rec.poll) != 0 {
		t.Errorf("reapplied without change: %+v", rec)
	}

	// 2. The download limit changes: only the Engine, and it's NO LONGER the first application (does not resend
	// BT port nor save_path).
	m.Handle(ctx, resp("v2", clusterValues("6"), false), nil)
	if len(rec.engine) != 1 || rec.engine[0].first || rec.engine[0].q.MaxActiveDownloads != 6 {
		t.Errorf("Engine = %+v, want ONE non-first call with downloads=6", rec.engine)
	}
	if len(rec.logLevels)+len(rec.heartbeat)+len(rec.poll) != 0 {
		t.Errorf("changing the limit affected log/intervals: %+v", rec)
	}
	if _, v := m.Applied(); v != "v2" {
		t.Errorf("version = %q, want v2", v)
	}

	// 3. Intervals and log level: only them.
	rec.engine = nil
	vals := clusterValues("6")
	vals["agent.heartbeat_interval"] = "20s"
	vals["agent.command_poll_interval"] = "2s"
	vals["agent.log_level"] = "debug"
	m.Handle(ctx, resp("v3", vals, false), nil)
	if len(rec.engine) != 0 {
		t.Errorf("changing intervals reapplied Engine: %+v", rec.engine)
	}
	if !reflect.DeepEqual(rec.heartbeat, []time.Duration{20 * time.Second}) || !reflect.DeepEqual(rec.poll, []time.Duration{2 * time.Second}) ||
		!reflect.DeepEqual(rec.logLevels, []string{"debug"}) {
		t.Errorf("hooks = hb %v poll %v log %v", rec.heartbeat, rec.poll, rec.logLevels)
	}
}

// Seeding (ADR-039 §7): while the CP asks for the report, the Agent DOES NOT apply anything — the cluster
// value that just arrived might be exactly what would overload this Node — and reports the explicit env
// on the next heartbeat. Only then it applies what comes back (which already has the protected value).
func TestSeedingHoldsApplicationUntilReport(t *testing.T) {
	rec := &recorder{}
	explicit := map[string]string{"engine.max_active_downloads": "3"} // the .env for node-04
	m := New(quiet(), envBaseline(), explicit, rec.hooks())
	ctx := context.Background()

	var req client.HeartbeatRequest
	m.Prepare(&req)
	m.Handle(ctx, resp("v-cluster", clusterValues("10"), true), nil) // the CP requests the report
	if len(rec.engine) != 0 {
		t.Fatalf("the Agent applied the cluster value (10) BEFORE reporting env (3): %+v", rec.engine)
	}
	if !m.NeedsFollowUp() {
		t.Error("with the report pending, the Agent should send another heartbeat immediately, without waiting for the ticker")
	}

	req = client.HeartbeatRequest{}
	m.Prepare(&req)
	if req.LocalConfig == nil || (*req.LocalConfig)["engine.max_active_downloads"] != "3" {
		t.Fatalf("local_config = %v, want the explicit env (downloads=3)", req.LocalConfig)
	}
	if req.ConfigVersion != "" {
		t.Errorf("config_version = %q: nothing has been applied yet", req.ConfigVersion)
	}

	// The CP has seeded: returns node-04's effective value (override = 3) and stops asking for the report.
	m.Handle(ctx, resp("v-node04", clusterValues("3"), false), nil)
	if len(rec.engine) != 1 || rec.engine[0].q.MaxActiveDownloads != 3 || !rec.engine[0].first {
		t.Errorf("Engine = %+v, want ONE application, the first, with downloads=3 (the Node never saw 10)", rec.engine)
	}
	if m.NeedsFollowUp() {
		t.Error("the report was handled; no follow-up")
	}
	req = client.HeartbeatRequest{}
	m.Prepare(&req)
	if req.LocalConfig != nil || req.ConfigVersion != "v-node04" {
		t.Errorf("after seeding: %+v, want only config_version=v-node04", req)
	}
}

// New Node: the bootstrap doesn't write any behavior env, so the report goes EMPTY — and it
// needs to go ({} and not omitted), because it's what marks the seeding as done on the CP.
func TestEmptyReportFromNewNodeBecomesEmptyObject(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	m.Handle(context.Background(), resp("v1", clusterValues("10"), true), nil)

	var req client.HeartbeatRequest
	m.Prepare(&req)
	if req.LocalConfig == nil {
		t.Fatal("empty report was omitted: the CP would never mark the Node as seeded and would request the report forever")
	}
	if len(*req.LocalConfig) != 0 {
		t.Errorf("local_config = %v, want empty", *req.LocalConfig)
	}
}

// Without CP on boot the Agent must work: after a few retries it applies the env. Before
// that it waits (a slow CP on boot can't make the Engine blink to the old value).
func TestBootWithoutCPAppliesEnvAfterGracePeriod(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	ctx := context.Background()

	for i := 1; i < bootGrace; i++ {
		m.Handle(ctx, client.HeartbeatResponse{}, errBoom)
		if len(rec.engine) != 0 {
			t.Fatalf("applied the env already on attempt %d (grace = %d)", i, bootGrace)
		}
	}
	m.Handle(ctx, client.HeartbeatResponse{}, errBoom)
	if len(rec.engine) != 1 || !rec.engine[0].first || rec.engine[0].q.MaxActiveDownloads != 3 {
		t.Fatalf("Engine = %+v, want the env (downloads=3) applied after the grace", rec.engine)
	}

	// The CP comes back with the cluster value: the Agent switches to it on the fly.
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)
	if len(rec.engine) != 2 || rec.engine[1].first || rec.engine[1].q.MaxActiveDownloads != 10 {
		t.Errorf("Engine = %+v, want the 2nd call (non-first) with downloads=10", rec.engine)
	}
}

// The pending report also counts towards the grace period: a CP that asked for the report and never
// responded to the next one can't leave the Engine unconfigured.
func TestPendingReportAlsoHasGracePeriod(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	for i := 0; i < bootGrace; i++ {
		m.Handle(context.Background(), resp("v1", clusterValues("10"), true), nil)
	}
	if len(rec.engine) != 1 || rec.engine[0].q.MaxActiveDownloads != 3 {
		t.Errorf("Engine = %+v, want the env (3) after the grace period — never the cluster value that the CP held", rec.engine)
	}
}

// CP Rollback: the heartbeat failure zeroes the proof — the Agent goes back to not sending a new field (an
// old CP would respond 400 and the Node would go OFFLINE) — but what was already applied remains.
func TestHeartbeatFailureResetsProofAndKeepsApplied(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{"engine.max_active_downloads": "3"}, rec.hooks())
	ctx := context.Background()
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)

	var req client.HeartbeatRequest
	m.Prepare(&req)
	if req.ConfigVersion != "v1" {
		t.Fatalf("precondition: config_version = %q", req.ConfigVersion)
	}

	m.Handle(ctx, client.HeartbeatResponse{}, errBoom) // the CP went down / was rolled back: 400
	req = client.HeartbeatRequest{}
	m.Prepare(&req)
	if req.ConfigVersion != "" || req.LocalConfig != nil {
		t.Errorf("after the failure it still sends a new field: %+v", req)
	}
	if b, v := m.Applied(); b.Queue.MaxActiveDownloads != 10 || v != "v1" {
		t.Errorf("the failure undid what was applied: %+v / %q", b, v)
	}
	if len(rec.engine) != 1 {
		t.Errorf("the failure reapplied the Engine: %+v", rec.engine)
	}

	// An old CP in place (response without agent_config): it proceeds without proof and DOES NOT revert to the env.
	m.Handle(ctx, client.HeartbeatResponse{Status: "ok"}, nil)
	req = client.HeartbeatRequest{}
	m.Prepare(&req)
	if req.ConfigVersion != "" {
		t.Errorf("config_version = %q against a CP that does not speak the protocol", req.ConfigVersion)
	}
	if b, _ := m.Applied(); b.Queue.MaxActiveDownloads != 10 || len(rec.engine) != 1 {
		t.Errorf("a silent CP undid what was applied: %+v engine=%+v", b, rec.engine)
	}
}

// The Agent defends itself against a bad value: it doesn't apply what would crash it and DOES NOT declare the version as
// applied — the Portal shows "waiting" instead of "up to date" for a value that didn't take effect.
func TestRejectedValueDoesNotDeclareVersion(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	ctx := context.Background()

	vals := clusterValues("10")
	vals["agent.heartbeat_interval"] = "0s"   // ticker.Reset(0) panics
	vals["engine.max_active_checking"] = "-1" // invalid
	vals["engine.future_key"] = "any"         // key this Agent doesn't know: ignored
	m.Handle(ctx, resp("v-bad", vals, false), nil)

	if len(rec.heartbeat) != 1 || rec.heartbeat[0] != 10*time.Second {
		t.Errorf("applied interval = %v, want the baseline one (10s) — the 0s was rejected", rec.heartbeat)
	}
	if len(rec.engine) != 1 || rec.engine[0].q.MaxActiveChecking != 1 || rec.engine[0].q.MaxActiveDownloads != 10 {
		t.Errorf("Engine = %+v, want checking=1 (rejected, keeps the baseline) and downloads=10 (valid, applied)", rec.engine)
	}
	if _, v := m.Applied(); v != "" {
		t.Errorf("version = %q, want empty: what arrived was not fully applied", v)
	}
	var req client.HeartbeatRequest
	m.Prepare(&req)
	if req.ConfigVersion != "" {
		t.Errorf("config_version = %q sent after rejecting values", req.ConfigVersion)
	}

	// When the CP corrects the value, it applies and declares it.
	m.Handle(ctx, resp("v-bom", clusterValues("10"), false), nil)
	if _, v := m.Applied(); v != "v-bom" {
		t.Errorf("version = %q, want v-bom", v)
	}
}

// Engine down: the application DOES NOT count as done, it is repeated on every heartbeat (before, the
// boot failure was never retried) and the version is only declared after it succeeds.
func TestEngineDownRetriesAndVersionWaitsForSuccess(t *testing.T) {
	rec := &recorder{engineErr: errBoom}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	ctx := context.Background()

	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)
	if _, v := m.Applied(); v != "" {
		t.Errorf("version = %q with the Engine down, want empty", v)
	}
	// The very next heartbeat repeats it — with a full response, or even a silent one.
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)
	m.Handle(ctx, client.HeartbeatResponse{}, errBoom)
	if len(rec.engine) != 3 {
		t.Fatalf("attempts on the Engine = %d, want 3 (one per heartbeat)", len(rec.engine))
	}
	if !rec.engine[0].first || !rec.engine[1].first || !rec.engine[2].first {
		t.Errorf("while the 1st application has not succeeded, all must also pin port and save_path: %+v", rec.engine)
	}

	rec.engineErr = nil // the Engine is back
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)
	if _, v := m.Applied(); v != "v1" {
		t.Errorf("version = %q, want v1 after the Engine accepted", v)
	}
	n := len(rec.engine)
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)
	if len(rec.engine) != n {
		t.Error("with the queue already applied the Agent must not keep calling the Engine")
	}
	// ...and from here on out changes are only from the queue.
	m.Handle(ctx, resp("v2", clusterValues("4"), false), nil)
	if last := rec.engine[len(rec.engine)-1]; last.first || last.q.MaxActiveDownloads != 4 {
		t.Errorf("last call = %+v, want non-first with downloads=4", last)
	}
}

// A CP that, after applied, starts responding without agent_config but with the Engine failing,
// also retries the queue — the retry doesn't depend on the CP speaking.
func TestSilentCPDoesNotPreventEngineRetry(t *testing.T) {
	rec := &recorder{}
	m := New(quiet(), envBaseline(), map[string]string{}, rec.hooks())
	ctx := context.Background()
	m.Handle(ctx, resp("v1", clusterValues("10"), false), nil)

	rec.engineErr = errBoom
	m.Handle(ctx, resp("v2", clusterValues("4"), false), nil) // changes the limit; the Engine fails
	rec.engineErr = nil
	m.Handle(ctx, client.HeartbeatResponse{Status: "ok"}, nil) // CP is silent
	last := rec.engine[len(rec.engine)-1]
	if last.q.MaxActiveDownloads != 4 {
		t.Errorf("last call = %+v, want the pending queue (downloads=4) reapplied", last)
	}
}

func TestNoHooksDoesNotBreak(t *testing.T) {
	m := New(quiet(), envBaseline(), nil, Hooks{})
	m.Handle(context.Background(), resp("v1", clusterValues("10"), false), nil)
	if b, v := m.Applied(); v != "v1" || b.Queue.MaxActiveDownloads != 10 {
		t.Errorf("Applied = %+v / %q", b, v)
	}
}
