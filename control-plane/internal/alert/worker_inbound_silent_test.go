package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// inbTestIPv6 is the Node address in tests (documentation range, RFC 3849).
const inbTestIPv6 = "2001:db8:0:1:be24:11ff:fe12:dee1"

// setupInboundWorker prepares an ONLINE Node that had its last incoming connection
// `lastIncoming` ago (SQL interval, e.g.: '20 hours'; empty = NEVER), with `incomingPeers`
// reported, and a torrent whose swarm has `leechers` leechers and `seeds` seeds (nil = NULL,
// a counter the Agent didn't report). `extra` overrides Store tunables. It returns the
// white-box Worker and the Node id.
func setupInboundWorker(t *testing.T, lastIncoming string, incomingPeers int, leechers, seeds any, silence string, extra ...map[string]string) (*Worker, *pgxpool.Pool, string) {
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
	ctx := context.Background()

	var clusterID, nodeID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO clusters (name, description) VALUES ('inb-itest', 'input silence test') RETURNING id::text`).
		Scan(&clusterID); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	// empty lastIncoming = last_incoming_at NULL (we have never seen an incoming connection).
	if err := pool.QueryRow(ctx,
		`INSERT INTO nodes (cluster_id, hostname, status, last_seen_at,
		                    incoming_peers, last_incoming_at, public_ipv6, bt_port)
		 VALUES ($1, 'inb-itest-node', 'ONLINE', now(), $2,
		         CASE WHEN $3 = '' THEN NULL ELSE now() - $3::interval END,
		         $4, 6881)
		 RETURNING id::text`,
		clusterID, incomingPeers, lastIncoming, inbTestIPv6).Scan(&nodeID); err != nil {
		t.Fatalf("create node: %v", err)
	}
	inbAddTorrent(t, pool, nodeID, "aa11bb22cc33dd44ee55ff6677889900aabbccdd", leechers, seeds)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM alerts WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM torrents WHERE node_id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
		_, _ = pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, clusterID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	overrides := map[string]string{
		"alerting.worker_interval":    "1s",
		"alerting.node_offline_after": "90s",
		"alerting.sample_retention":   "0s",
		"alerting.disk_saturated_pct": "90",
		"alerting.firewalled_grace":   "30m",
		"alerting.incoming_silence":   silence,
	}
	for _, m := range extra {
		for k, v := range m {
			overrides[k] = v
		}
	}
	return NewWorker(pool, NewService(pool), logger, settings.NewForTest(overrides)), pool, nodeID
}

// inbAddTorrent inserts a Node torrent with the given swarm (nil = NULL counter).
func inbAddTorrent(t *testing.T, pool *pgxpool.Pool, nodeID, hash string, leechers, seeds any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO torrents (node_id, hash, name, status, num_leechers, num_seeds)
		 VALUES ($1, $2, $3, 'SEEDING', $4, $5)`,
		nodeID, hash, hash[:8]+".mkv", leechers, seeds); err != nil {
		t.Fatalf("create torrent: %v", err)
	}
}

func inbOpenCount(t *testing.T, pool *pgxpool.Pool, nodeID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alerts WHERE code = 'NODE_INBOUND_SILENT' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

// inbOpenAlert returns the message and payload of the Node's open alert.
func inbOpenAlert(t *testing.T, pool *pgxpool.Pool, nodeID string) (string, map[string]any) {
	t.Helper()
	var msg, raw string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(message, ''), COALESCE(payload::text, '{}') FROM alerts
		  WHERE code = 'NODE_INBOUND_SILENT' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&msg, &raw); err != nil {
		t.Fatalf("read open alert: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	return msg, payload
}

// TestEvaluateIncomingSilence_SilenceWithDemand: 20h without incoming while there is demand in the
// swarm opens the alert, doesn't duplicate it, and resolves when incoming resumes. The resolve is the
// point of issue #155: it's only possible because the signal DECAYS — with a sticky engine_connectable
// the Node would remain 'connected' and none of this would happen.
func TestEvaluateIncomingSilence_SilenceWithDemand(t *testing.T) {
	// 3 leechers / 1 seed (probably just the Node itself): needy swarm.
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 3, 1, "12h")
	ctx := context.Background()

	w.evaluateIncomingSilence(ctx)
	if got := inbOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("expected 1 open NODE_INBOUND_SILENT alert, got %d", got)
	}

	// Dedup by code+node.
	w.evaluateIncomingSilence(ctx)
	if got := inbOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("dedup failed: expected 1, got %d", got)
	}

	// Incoming resumes → resolves.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET last_incoming_at = now(), incoming_peers = 2 WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("mark recent incoming: %v", err)
	}
	w.evaluateIncomingSilence(ctx)
	if got := inbOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected resolved alert (0 open), got %d", got)
	}
}

// TestEvaluateIncomingSilence_SeederWithoutLeecherNoAlert is the reason for the
// cross-check with demand, and the test explicitly requested by issue #155.
//
// With no leechers in the swarm, 0 incoming connections is the CORRECT state for a seeder: nobody
// wants the content. Measured on 07/09/2026, the four Nodes in the cluster — all healthy —
// had 0, 1, 1 and 0 incoming connections at the time of reading. Alerting on pure absence
// would turn an idle catalog into a permanent alarm.
func TestEvaluateIncomingSilence_SeederWithoutLeecherNoAlert(t *testing.T) {
	// No leechers, even with a swarm without seeds: there is no one asking for anything.
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 0, 0, "12h")
	w.evaluateIncomingSilence(context.Background())
	if got := inbOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts with no demand in the swarm, got %d", got)
	}
}

// TestEvaluateIncomingSilence_SaturatedSwarmNoTrigger is the first of the two cases that
// issue #206 requires: 1 leecher in a swarm with 63 seeds is NOT demand on this Node. The
// leecher connects to half a dozen of the 63, and going hours without being one of them is the
// expected behavior of a healthy Node — which is what triggered the alert on a Node with its
// port verified from the outside and open.
func TestEvaluateIncomingSilence_SaturatedSwarmNoTrigger(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 1, 63, "12h")
	w.evaluateIncomingSilence(context.Background())
	if got := inbOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("1 leecher / 63 seeds is not demand; expected 0 alerts, got %d", got)
	}
}

// TestEvaluateIncomingSilence_NeedySwarmArms is the second case from #206: 1 leecher and 1
// seed — the seed probably being the Node itself — is a swarm where the leecher needs
// us. With no incoming for longer than the window, this is a symptom.
func TestEvaluateIncomingSilence_NeedySwarmArms(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 1, 1, "12h")
	w.evaluateIncomingSilence(context.Background())
	if got := inbOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("1 leecher / 1 seed is demand; expected 1 alert, got %d", got)
	}
}

// TestEvaluateIncomingSilence_TorrentsThatArmedOnHealthyNode reproduces the table from #206,
// measured on 09/09/2026: the six torrents that triggered the alert on provenly
// healthy Nodes (port verified from the outside, open). With the default, none of them triggers.
func TestEvaluateIncomingSilence_TorrentsThatArmedOnHealthyNode(t *testing.T) {
	for _, c := range []struct{ leechers, seeds int }{
		{1, 2}, {1, 63}, {1, 20}, {3, 17}, {1, 15}, {1, 16},
	} {
		t.Run(fmt.Sprintf("%dleechers_%dseeds", c.leechers, c.seeds), func(t *testing.T) {
			w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, c.leechers, c.seeds, "12h")
			w.evaluateIncomingSilence(context.Background())
			if got := inbOpenCount(t, pool, nodeID); got != 0 {
				t.Fatalf("%d leechers / %d seeds should not arm the alert, got %d", c.leechers, c.seeds, got)
			}
		})
	}
}

// TestEvaluateIncomingSilence_RatioEdge fixes which side of the boundary each ratio falls on: the
// rule is seeds <= leechers × value, so equality TRIGGERS. The tunable is a float, and 0 is not
// "disabled" — it is the strictest reading (only swarm without seed).
func TestEvaluateIncomingSilence_RatioEdge(t *testing.T) {
	cases := []struct {
		name            string
		leechers, seeds int
		ratio           string // "" = catalog default (1)
		arms            bool
	}{
		{"default: equality arms", 1, 1, "", true},
		{"default: one extra seed does not arm", 1, 2, "", false},
		{"default: 3 leechers and 3 seeds arms", 3, 3, "", true},
		{"default: 3 leechers and 4 seeds do not arm", 3, 4, "", false},
		{"fractional 2.5: 2 leechers and 5 seeds arm", 2, 5, "2.5", true},
		{"fractional 2.5: 2 leechers and 6 seeds do not arm", 2, 6, "2.5", false},
		{"fractional 0.5: 2 leechers and 1 seed arms", 2, 1, "0.5", true},
		{"fractional 0.5: 2 leechers and 2 seeds do not arm", 2, 2, "0.5", false},
		{"0: seedless swarm arms", 1, 0, "0", true},
		{"0: one seed already does not arm", 1, 1, "0", false},
		{"high value reverts to old behavior", 1, 63, "100000", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var extra map[string]string
			if c.ratio != "" {
				extra = map[string]string{"alerting.incoming_demand_seeds_per_leecher": c.ratio}
			}
			w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, c.leechers, c.seeds, "12h", extra)
			w.evaluateIncomingSilence(context.Background())
			want := 0
			if c.arms {
				want = 1
			}
			if got := inbOpenCount(t, pool, nodeID); got != want {
				t.Fatalf("%d leechers / %d seeds with reason %q: expected %d alert(s), got %d",
					c.leechers, c.seeds, c.ratio, want, got)
			}
		})
	}
}

// TestEvaluateIncomingSilence_SwarmWithoutSeedTriggers: no seeds in the swarm (not even the Node, which the
// scrape might not count) means the leecher has no one to download from — it is the strongest
// demand there is, for any value of the tunable.
func TestEvaluateIncomingSilence_SwarmWithoutSeedTriggers(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 2, 0, "12h")
	w.evaluateIncomingSilence(context.Background())
	if got := inbOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("2 leechers / 0 seeds is demand; expected 1 alert, got %d", got)
	}
}

// TestEvaluateIncomingSilence_NullCounterDoesNotArm: unknown never accuses. An Agent that didn't
// report the swarm counters doesn't allow weighing the competition — and without being able to weigh, the
// rule doesn't declare demand. This applies if either side is NULL.
func TestEvaluateIncomingSilence_NullCounterDoesNotArm(t *testing.T) {
	cases := []struct {
		name            string
		leechers, seeds any
	}{
		{"seeds NULL", 3, nil},
		{"leechers NULL", nil, 0},
		{"both NULL", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, c.leechers, c.seeds, "12h")
			w.evaluateIncomingSilence(context.Background())
			if got := inbOpenCount(t, pool, nodeID); got != 0 {
				t.Fatalf("a NULL counter is not demand; expected 0 alerts, got %d", got)
			}
		})
	}
}

// TestEvaluateIncomingSilence_OneNeedyTorrentIsEnough: the demand is on the Node, not the torrent —
// ONE needy swarm among its torrents is enough, no matter how plentiful the others are. The
// payload only records what triggered the rule: saturated torrents and those without leechers are left
// out of the count.
func TestEvaluateIncomingSilence_OneNeedyTorrentIsEnough(t *testing.T) {
	// The setup torrent is saturated (1 leecher / 63 seeds).
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 1, 63, "12h")
	// No leechers, and another saturated (3 leechers / 17 seeds): neither is demand.
	inbAddTorrent(t, pool, nodeID, "bb11bb22cc33dd44ee55ff6677889900aabbccdd", 0, 0)
	inbAddTorrent(t, pool, nodeID, "cc11bb22cc33dd44ee55ff6677889900aabbccdd", 3, 17)
	ctx := context.Background()

	w.evaluateIncomingSilence(ctx)
	if got := inbOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("only saturated torrents or torrents without leechers: expected 0 alerts, got %d", got)
	}

	// A needy swarm (1 / 1) and one with no seeds (2 / 0): both trigger.
	inbAddTorrent(t, pool, nodeID, "dd11bb22cc33dd44ee55ff6677889900aabbccdd", 1, 1)
	inbAddTorrent(t, pool, nodeID, "ee11bb22cc33dd44ee55ff6677889900aabbccdd", 2, 0)
	w.evaluateIncomingSilence(ctx)
	if got := inbOpenCount(t, pool, nodeID); got != 1 {
		t.Fatalf("with two swarms in need expected 1 alert, got %d", got)
	}
	_, payload := inbOpenAlert(t, pool, nodeID)
	for key, want := range map[string]float64{"demand_torrents": 2, "demand_leechers": 3, "demand_seeds": 1} {
		if got, _ := payload[key].(float64); got != want {
			t.Errorf("payload[%s] = %v, expected %v (only armed torrents are counted)", key, payload[key], want)
		}
	}
}

// TestEvaluateIncomingSilence_TextDoesNotAge (#203): the alert is logged once and can
// stay open for days. Address and duration in the text would age — the Node changed
// address almost 38h after opening and the dashboard kept telling to release an address that no
// longer existed. The text now carries nothing that changes; whatever varies goes into the payload.
func TestEvaluateIncomingSilence_TextDoesNotAge(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 1, 1, "12h")
	ctx := context.Background()

	w.evaluateIncomingSilence(ctx)
	msg, payload := inbOpenAlert(t, pool, nodeID)

	if msg != inboundSilentMessage {
		t.Fatalf("message = %q, expected package constant", msg)
	}
	// No aging fragments: address, port, duration. No digits at all, in fact — if
	// anyone interpolates a value in the text again, this test breaks.
	for _, frag := range []string{inbTestIPv6, "2001:", "[", "6881", " ago "} {
		if strings.Contains(msg, frag) {
			t.Errorf("text contains %q, which goes stale: %q", frag, msg)
		}
	}
	if strings.ContainsAny(msg, "0123456789") {
		t.Errorf("text contains a number, which goes stale: %q", msg)
	}

	// What the operator needs goes into the payload.
	if payload["public_ipv6"] != inbTestIPv6 {
		t.Errorf("payload.public_ipv6 = %v, expected %s", payload["public_ipv6"], inbTestIPv6)
	}
	if got, _ := payload["bt_port"].(float64); got != 6881 {
		t.Errorf("payload.bt_port = %v, expected 6881", payload["bt_port"])
	}
	if payload["hostname"] != "inb-itest-node" {
		t.Errorf("payload.hostname = %v", payload["hostname"])
	}
	if s, _ := payload["last_incoming_at"].(string); s == "" {
		t.Errorf("payload.last_incoming_at missing: the silence start is what the duration stopped reporting")
	}

	// The Node changes address with the alert open: the text remains true.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET public_ipv6 = '2001:db8:0:2::1' WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("change address: %v", err)
	}
	w.evaluateIncomingSilence(ctx)
	if got, _ := inbOpenAlert(t, pool, nodeID); got != msg {
		t.Fatalf("text changed with the alert open: %q -> %q", msg, got)
	}
}

// TestEvaluateIncomingSilence_WithinWindowDoesNotAlert: silence shorter than the window is not
// a verdict. Incoming connection is a sparse event; requiring one every few minutes
// would trigger an alarm on a healthy Node.
func TestEvaluateIncomingSilence_WithinWindowDoesNotAlert(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "2 hours", 0, 5, 1, "12h")
	w.evaluateIncomingSilence(context.Background())
	if got := inbOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts within window, got %d", got)
	}
}

// TestEvaluateIncomingSilence_NeverSawIncomingNoAlert: last_incoming_at NULL does not
// distinguish "newly deployed Node, new column" from "always unreachable Node" — and
// unknown cannot become an accusation. This case is ONBOARDING, covered by
// NODE_FIREWALLED, which uses the Engine's verdict.
func TestEvaluateIncomingSilence_NeverSawIncomingNoAlert(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "", 0, 5, 1, "12h")
	w.evaluateIncomingSilence(context.Background())
	if got := inbOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("expected 0 alerts with last_incoming_at NULL, got %d", got)
	}
}

// TestEvaluateIncomingSilence_ZeroWindowDisables: 0 disables the rule, like the other
// alert tunables.
func TestEvaluateIncomingSilence_ZeroWindowDisables(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 5, 1, "0s")
	w.evaluateIncomingSilence(context.Background())
	if got := inbOpenCount(t, pool, nodeID); got != 0 {
		t.Fatalf("window 0 should disable the rule, got %d alerts", got)
	}
}
