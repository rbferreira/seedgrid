package agentconfig_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/agentconfig"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

type env struct {
	svc   *agentconfig.Service
	store *settings.Store
	nodes *node.Service
	pool  *pgxpool.Pool
}

// setup opens the ISOLATED test Postgres (SEEDGRID_TEST_DATABASE_URL) — never production.
// Each test creates its own Nodes (unique hostname) and returns the database as it found it.
func setup(t *testing.T) env {
	t.Helper()
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	emitter := events.NewEmitter(pool)
	nodes := node.NewService(pool, emitter)
	if err := nodes.EnsureDefaultCluster(context.Background()); err != nil {
		t.Fatalf("default cluster: %v", err)
	}
	store := settings.New(pool)
	svc := agentconfig.New(pool, store)
	svc.SetEvents(emitter)
	return env{svc: svc, store: store, nodes: nodes, pool: pool}
}

func (e env) newNode(t *testing.T, prefix string) string {
	t.Helper()
	res, err := e.nodes.Register(context.Background(), node.RegisterInput{
		Hostname: fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()),
		Engine:   "qbittorrent",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return res.NodeID
}

// setCluster defines a cluster value (override in `settings`) and undoes it at the end — the
// tests from other packages share the database.
func (e env) setCluster(t *testing.T, key, value string) {
	t.Helper()
	ctx := context.Background()
	if err := e.store.Set(ctx, key, value, "test"); err != nil {
		t.Fatalf("Set %s: %v", key, err)
	}
	t.Cleanup(func() { _ = e.store.Reset(context.Background(), key, "test") })
}

func item(t *testing.T, st agentconfig.NodeState, key string) agentconfig.Item {
	t.Helper()
	for _, it := range st.Items {
		if it.Spec.Key == key {
			return it
		}
	}
	t.Fatalf("key %s missing", key)
	return agentconfig.Item{}
}

func TestOverrideLifecycle(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newNode(t, "ac-lifecycle")
	const key = "engine.max_active_downloads"

	st, err := e.svc.Node(ctx, id)
	if err != nil {
		t.Fatalf("Node: %v", err)
	}
	if it := item(t, st, key); it.Source != agentconfig.SourceCluster || it.Value != "10" {
		t.Fatalf("new Node: %+v, want cluster value 10", it)
	}
	if st.Status() != agentconfig.StatusUnsupported {
		t.Errorf("Node that never reported: status %q, want unsupported", st.Status())
	}

	// Defines the override (the node-04 case): it only applies to this Node.
	ch, err := e.svc.SetOverride(ctx, id, key, "3", "rafael")
	if err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if ch.Before != nil || ch.After == nil || *ch.After != "3" {
		t.Errorf("Change = %+v, want before=nil after=3", ch)
	}
	st, _ = e.svc.Node(ctx, id)
	if it := item(t, st, key); it.Source != agentconfig.SourceNode || it.Value != "3" || it.ClusterValue != "10" {
		t.Errorf("with override: %+v, want 3 from Node and cluster 10", it)
	}

	// Changing the override returns the previous value (which is what Audit stores).
	ch, err = e.svc.SetOverride(ctx, id, key, "4", "rafael")
	if err != nil {
		t.Fatalf("SetOverride 2: %v", err)
	}
	if ch.Before == nil || *ch.Before != "3" {
		t.Errorf("Change.Before = %v, want 3", ch.Before)
	}

	// Changing the cluster value does NOT move the Node with an override; it moves the one without.
	other := e.newNode(t, "ac-lifecycle-b")
	e.setCluster(t, key, "8")
	st, _ = e.svc.Node(ctx, id)
	if it := item(t, st, key); it.Value != "4" || it.ClusterValue != "8" || !it.ClusterExplicit {
		t.Errorf("Node with override after changing cluster: %+v, want 4 (cluster 8, explicit)", it)
	}
	stOther, _ := e.svc.Node(ctx, other)
	if it := item(t, stOther, key); it.Value != "8" || it.Source != agentconfig.SourceCluster {
		t.Errorf("Node without override after changing cluster: %+v, want 8 from cluster", it)
	}

	// Removing the override returns the Node to the cluster value.
	ch, err = e.svc.ClearOverride(ctx, id, key)
	if err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
	if ch.Before == nil || *ch.Before != "4" || ch.Effective != "8" {
		t.Errorf("Change = %+v, want before=4 and effective=8 (cluster)", ch)
	}
	st, _ = e.svc.Node(ctx, id)
	if it := item(t, st, key); it.Source != agentconfig.SourceCluster || it.Value != "8" || it.Override != nil {
		t.Errorf("after removing: %+v, want cluster 8", it)
	}
	// Removing it again is idempotent.
	if _, err := e.svc.ClearOverride(ctx, id, key); err != nil {
		t.Errorf("repeated ClearOverride: %v", err)
	}
}

func TestOverrideValidation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newNode(t, "ac-valid")

	var invalid *agentconfig.InvalidValueError
	for _, c := range []struct{ key, value string }{
		{"engine.max_active_downloads", "0"},
		{"engine.max_active_downloads", "-1"},
		{"engine.max_active_downloads", "many"},
		{"agent.heartbeat_interval", "0s"},
		{"agent.log_level", "verbose"},
		{"engine.queueing_enabled", "maybe"},
	} {
		_, err := e.svc.SetOverride(ctx, id, c.key, c.value, "rafael")
		if !errors.As(err, &invalid) {
			t.Errorf("SetOverride(%s=%q): %v, want InvalidValueError (same rule as cluster)", c.key, c.value, err)
		}
	}
	if _, err := e.svc.SetOverride(ctx, id, "nonexistent.key", "1", "rafael"); !errors.Is(err, agentconfig.ErrUnknownKey) {
		t.Errorf("unknown key: %v, want ErrUnknownKey", err)
	}
	// A Control Plane tunable has no notion of Node.
	if _, err := e.svc.SetOverride(ctx, id, "admission.interval", "30s", "rafael"); !errors.Is(err, agentconfig.ErrNotPerNode) {
		t.Errorf("the Control Plane tunable: %v, want ErrNotPerNode", err)
	}
	// None of this was recorded.
	st, _ := e.svc.Node(ctx, id)
	for _, it := range st.Items {
		if it.Override != nil {
			t.Errorf("%s left with override %q after invalid attempts", it.Spec.Key, *it.Override)
		}
	}

	// Non-existent Node — valid uuid and text that isn't even a uuid (Portal path param).
	for _, bad := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		if _, err := e.svc.SetOverride(ctx, bad, "engine.max_active_downloads", "3", "rafael"); !errors.Is(err, agentconfig.ErrNodeNotFound) {
			t.Errorf("SetOverride(node %q): %v, want ErrNodeNotFound", bad, err)
		}
		if _, err := e.svc.Node(ctx, bad); !errors.Is(err, agentconfig.ErrNodeNotFound) {
			t.Errorf("Node(%q): %v, want ErrNodeNotFound", bad, err)
		}
	}
}

// Seeding (ADR-039 §7) is the migration safety net: a Node that already runs with a value
// in .env cannot adopt the cluster value on the first heartbeat of the new Agent.
func TestSeedRules(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newNode(t, "ac-seed")

	local := map[string]string{
		"engine.max_active_downloads": "3",      // unlike the cluster (10) → seeds (the case of node-04)
		"agent.log_level":             "debug",  // different → seeds
		"engine.dont_count_slow":      "true",   // different → seeds
		"agent.heartbeat_interval":    "10s",    // equal to cluster → does NOT become an override
		"agent.command_poll_interval": "5000ms", // same duration, different spelling → does NOT become an override
		"engine.max_active_checking":  "0",      // invalid (min. 1)
		"nonexistent.key":             "x",
		"admission.interval":          "30s", // CP tunable, not Node
	}
	res, err := e.svc.Seed(ctx, id, local)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	wantSeeded := map[string]string{
		"engine.max_active_downloads": "3",
		"agent.log_level":             "debug",
		"engine.dont_count_slow":      "true",
	}
	if len(res.Seeded) != len(wantSeeded) {
		t.Errorf("seeded %v, want only %v", res.Seeded, wantSeeded)
	}
	for k, v := range wantSeeded {
		if res.Seeded[k] != v {
			t.Errorf("%s seeded = %q, want %q", k, res.Seeded[k], v)
		}
	}
	for _, k := range []string{"agent.heartbeat_interval", "agent.command_poll_interval", "engine.max_active_checking", "nonexistent.key", "admission.interval"} {
		if _, ok := res.Skipped[k]; !ok {
			t.Errorf("%s should have been skipped (Skipped = %v)", k, res.Skipped)
		}
	}

	st, _ := e.svc.Node(ctx, id)
	if st.SeededAt == nil {
		t.Error("the Node should be marked as seeded")
	}
	if it := item(t, st, "engine.max_active_downloads"); it.Source != agentconfig.SourceNode || it.Value != "3" {
		t.Errorf("downloads after seeding: %+v, want override 3", it)
	}
	if it := item(t, st, "agent.heartbeat_interval"); it.Source != agentconfig.SourceCluster {
		t.Errorf("heartbeat equal to cluster must not become override: %+v", it)
	}

	// The system seeded it, and this appears as an Event (domain fact), with the keys.
	var n int
	if err := e.pool.QueryRow(ctx,
		`SELECT count(*) FROM events ev JOIN event_types et ON et.id = ev.event_type_id
		  WHERE et.code = 'NODE_CONFIG_SEEDED' AND ev.node_id = $1::uuid`, id).Scan(&n); err != nil || n != 1 {
		t.Errorf("NODE_CONFIG_SEEDED events = %d (err=%v), want 1", n, err)
	}
	var by string
	if err := e.pool.QueryRow(ctx,
		`SELECT updated_by FROM node_settings WHERE node_id = $1::uuid AND key = 'engine.max_active_downloads'`, id).Scan(&by); err != nil || by != agentconfig.SeededBy {
		t.Errorf("updated_by of seeded override = %q (err=%v), want %q", by, err, agentconfig.SeededBy)
	}

	// ONCE per Node: a second report does not seed anything, even if the operator has deleted the
	// seeded override (migration, not synchronization).
	if _, err := e.svc.ClearOverride(ctx, id, "engine.max_active_downloads"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
	res2, err := e.svc.Seed(ctx, id, local)
	if err != nil {
		t.Fatalf("Seed 2: %v", err)
	}
	if !res2.AlreadySeeded || len(res2.Seeded) != 0 {
		t.Errorf("second Seed = %+v, want AlreadySeeded and nothing seeded", res2)
	}
	st, _ = e.svc.Node(ctx, id)
	if it := item(t, st, "engine.max_active_downloads"); it.Source != agentconfig.SourceCluster {
		t.Errorf("the override removed by the operator reappeared: %+v", it)
	}
}

// The operator who acts first rules: it does not seed over an existing override nor against a
// cluster value they set themselves.
func TestSeedDoesNotContradictOperatorDecision(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// 1. A Node override already exists.
	withOverride := e.newNode(t, "ac-seed-ov")
	if _, err := e.svc.SetOverride(ctx, withOverride, "engine.max_active_downloads", "2", "rafael"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	res, err := e.svc.Seed(ctx, withOverride, map[string]string{"engine.max_active_downloads": "3"})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(res.Seeded) != 0 || res.Skipped["engine.max_active_downloads"] == "" {
		t.Errorf("Seed over existing override: %+v, want the key ignored", res)
	}
	st, _ := e.svc.Node(ctx, withOverride)
	if it := item(t, st, "engine.max_active_downloads"); it.Value != "2" {
		t.Errorf("the operator override was overwritten: %+v, want 2", it)
	}

	// 2. The cluster value was defined by the operator: it's a fleet decision, the old .env
	// of a machine does not contradict it.
	e.setCluster(t, "engine.max_active_downloads", "8")
	clusterExplicit := e.newNode(t, "ac-seed-cl")
	res, err = e.svc.Seed(ctx, clusterExplicit, map[string]string{"engine.max_active_downloads": "3"})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(res.Seeded) != 0 || res.Skipped["engine.max_active_downloads"] == "" {
		t.Errorf("Seed against explicit cluster value: %+v, want the key ignored", res)
	}
	st, _ = e.svc.Node(ctx, clusterExplicit)
	if it := item(t, st, "engine.max_active_downloads"); it.Value != "8" || it.Source != agentconfig.SourceCluster {
		t.Errorf("Node should follow the explicit cluster: %+v", it)
	}
}

// New Node (bootstrap only writes identity): no explicit behavior env, the report
// comes empty — yet it must still mark the Node as seeded, else the CP would ask for the report
// forever and the Agent would never apply anything.
func TestSeedEmptyReportMarksNode(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newNode(t, "ac-seed-empty")

	d, err := e.svc.Exchange(ctx, id, agentconfig.Report{})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if !d.ReportLocal {
		t.Fatal("Node never seeded: the Control Plane should request the report (report_local)")
	}

	d, err = e.svc.Exchange(ctx, id, agentconfig.Report{Local: map[string]string{}})
	if err != nil {
		t.Fatalf("Exchange with empty report: %v", err)
	}
	if d.ReportLocal {
		t.Error("after the report (even empty) the Control Plane must not request it again")
	}
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM node_settings WHERE node_id = $1::uuid`, id).Scan(&n)
	if n != 0 {
		t.Errorf("empty report created %d overrides", n)
	}
}

func TestExchangeDeliversEffectiveStateAndRecordsWhatWasApplied(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newNode(t, "ac-exchange")
	// Skips seeding: this test is about the delivery and the version.
	if _, err := e.svc.Seed(ctx, id, map[string]string{}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := e.svc.SetOverride(ctx, id, "engine.max_active_downloads", "3", "rafael"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	// The delivery brings ALL keys, with the EFFECTIVE values of this Node.
	d, err := e.svc.Exchange(ctx, id, agentconfig.Report{})
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if len(d.Values) != len(settings.NodeSpecs()) {
		t.Errorf("values = %d, want %d (all PerNode keys)", len(d.Values), len(settings.NodeSpecs()))
	}
	if d.Values["engine.max_active_downloads"] != "3" || d.Values["agent.heartbeat_interval"] != "10s" {
		t.Errorf("values = %v, want downloads=3 (override) and heartbeat=10s (cluster)", d.Values)
	}
	if d.ReportLocal {
		t.Error("Node already seeded: should not request the report")
	}
	if d.Version != agentconfig.Version(d.Values) {
		t.Errorf("version %q does not match the hash of delivered values", d.Version)
	}

	// Before the Agent reports: unsupported. After applying the version: up to date, with the time.
	st, _ := e.svc.Node(ctx, id)
	if st.Status() != agentconfig.StatusUnsupported {
		t.Errorf("before the report: %q, want unsupported", st.Status())
	}
	if _, err := e.svc.Exchange(ctx, id, agentconfig.Report{Version: d.Version}); err != nil {
		t.Fatalf("Exchange with version: %v", err)
	}
	st, _ = e.svc.Node(ctx, id)
	if st.Status() != agentconfig.StatusInSync || st.AppliedAt == nil {
		t.Fatalf("after the report: status %q appliedAt=%v, want in_sync with time", st.Status(), st.AppliedAt)
	}
	firstApplied := *st.AppliedAt

	// Repeating the same version on every heartbeat does not change applied_at (it's the time it was
	// applied, not the time of the last heartbeat).
	time.Sleep(20 * time.Millisecond)
	if _, err := e.svc.Exchange(ctx, id, agentconfig.Report{Version: d.Version}); err != nil {
		t.Fatalf("repeated Exchange: %v", err)
	}
	st, _ = e.svc.Node(ctx, id)
	if !st.AppliedAt.Equal(firstApplied) {
		t.Errorf("applied_at changed from %v to %v without version change", firstApplied, *st.AppliedAt)
	}

	// The operator changes the override: the effective version changes and the Node stays "waiting for the Agent"
	// until it applies and reports the new one.
	if _, err := e.svc.SetOverride(ctx, id, "engine.max_active_downloads", "5", "rafael"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	st, _ = e.svc.Node(ctx, id)
	if st.Status() != agentconfig.StatusPending {
		t.Errorf("after changing the override: %q, want pending", st.Status())
	}
	d2, _ := e.svc.Exchange(ctx, id, agentconfig.Report{Version: d.Version}) // still reports the old one
	if d2.Version == d.Version {
		t.Error("the delivered version should have changed along with the override")
	}
	if st, _ = e.svc.Node(ctx, id); st.Status() != agentconfig.StatusPending {
		t.Errorf("Agent still on old version: %q, want pending", st.Status())
	}
	if _, err := e.svc.Exchange(ctx, id, agentconfig.Report{Version: d2.Version}); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if st, _ = e.svc.Node(ctx, id); st.Status() != agentconfig.StatusInSync {
		t.Errorf("Agent applied the new one: %q, want in_sync", st.Status())
	}
}

func TestOverridesListNodesWithOwnValue(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newNode(t, "ac-list")
	if _, err := e.svc.SetOverride(ctx, id, "engine.max_active_downloads", "3", "rafael"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	rows, err := e.svc.Overrides(ctx)
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.NodeID == id && r.Key == "engine.max_active_downloads" {
			found = true
			if r.Value != "3" || r.ClusterValue != "10" || r.Label() == "" {
				t.Errorf("row = %+v, want value 3, cluster 10 and label", r)
			}
		}
	}
	if !found {
		t.Error("the override does not appear in the list")
	}
}

// A report with too many keys isn't from a healthy Agent: rejected without marking the Node as seeded
// (the next legitimate report still counts) and without recording anything.
func TestSeedRejectsGiantReport(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	id := e.newNode(t, "ac-seed-big")

	big := map[string]string{}
	for i := 0; i < 500; i++ {
		big[fmt.Sprintf("key.%d", i)] = "x"
	}
	if _, err := e.svc.Seed(ctx, id, big); !errors.Is(err, agentconfig.ErrReportTooLarge) {
		t.Fatalf("Seed(500 keys) = %v, want ErrReportTooLarge", err)
	}
	st, _ := e.svc.Node(ctx, id)
	if st.SeededAt != nil {
		t.Error("the rejected report marked the Node as seeded")
	}
	// The next legitimate report still works.
	res, err := e.svc.Seed(ctx, id, map[string]string{"engine.max_active_downloads": "3"})
	if err != nil || res.Seeded["engine.max_active_downloads"] != "3" {
		t.Errorf("legitimate report after the rejected one: %+v err=%v", res, err)
	}
}

// A Node that joined with the qBittorrent it already ran (ADR-043) reports its engine.* values
// from that Engine. They win over an explicit cluster value, and a value outside the catalog
// range is brought into it: the first application must change nothing in the Engine. The
// agent.* keys of the same report still follow the ordinary rule.
func TestSeedFromAdoptedEngine(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var gate string
	if err := e.pool.QueryRow(ctx, `SELECT operation_mode FROM clusters WHERE name = 'default'`).Scan(&gate); err != nil {
		t.Fatalf("read gate: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), `UPDATE clusters SET operation_mode = $1 WHERE name = 'default'`, gate)
	})
	res, err := e.nodes.Register(ctx, node.RegisterInput{
		Hostname: fmt.Sprintf("ac-seed-adopted-%d", time.Now().UnixNano()), Engine: "qbittorrent", EngineAdopted: true,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	id := res.NodeID
	e.setCluster(t, "engine.max_active_downloads", "8")
	e.setCluster(t, "agent.log_level", "warn")

	seed, err := e.svc.Seed(ctx, id, map[string]string{
		"engine.max_active_downloads": "3",     // the Engine's value wins over the explicit cluster 8
		"engine.max_active_uploads":   "-1",    // qBittorrent's "no limit" → the catalog maximum
		"engine.slow_inactive_timer":  "0",     // below the minimum → the minimum
		"engine.queueing_enabled":     "false", // the Engine had no queue
		"engine.max_active_torrents":  "10000", // same as the cluster → no override
		"agent.log_level":             "debug", // not from the Engine: the explicit cluster wins
	})
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	want := map[string]string{
		"engine.max_active_downloads": "3",
		"engine.max_active_uploads":   "100000",
		"engine.slow_inactive_timer":  "1",
		"engine.queueing_enabled":     "false",
	}
	if len(seed.Seeded) != len(want) {
		t.Errorf("seeded %v, want %v", seed.Seeded, want)
	}
	for k, v := range want {
		if seed.Seeded[k] != v {
			t.Errorf("%s seeded = %q, want %q", k, seed.Seeded[k], v)
		}
	}
	for _, k := range []string{"engine.max_active_torrents", "agent.log_level"} {
		if _, ok := seed.Skipped[k]; !ok {
			t.Errorf("%s should have been skipped (Skipped = %v)", k, seed.Skipped)
		}
	}
	st, _ := e.svc.Node(ctx, id)
	if it := item(t, st, "engine.max_active_downloads"); it.Source != agentconfig.SourceNode || it.Value != "3" {
		t.Errorf("downloads after seeding: %+v, want the Engine's 3 as a Node override", it)
	}
	var n int
	if err := e.pool.QueryRow(ctx,
		`SELECT count(*) FROM events ev JOIN event_types et ON et.id = ev.event_type_id
		  WHERE et.code = 'NODE_ENGINE_SEEDED' AND ev.node_id = $1::uuid`, id).Scan(&n); err != nil || n != 1 {
		t.Errorf("NODE_ENGINE_SEEDED events = %d (err=%v), want 1", n, err)
	}
}
