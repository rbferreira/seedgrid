package alert

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
	"github.com/rbferreira/seedgrid/control-plane/migrations"
)

// Validity of the reachability alerts (#211).
//
// Real case that originated the tests, measured on 24/09/2026: node-01 with 0 leechers on 116
// torrents, NODE_IPV6_CHANGED and NODE_INBOUND_SILENT open and the port checked FROM OUTSIDE,
// open. The three rules only resolve when an inbound connection arrives; on a Node with no demand
// it never arrives, and the alert stayed open forever.

// age simulates that the last alert notification happened `ago` (SQL interval).
// It is updated_at, not opened_at, that the validity reads.
func age(t *testing.T, pool *pgxpool.Pool, code, nodeID, ago string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE alerts SET updated_at = now() - $3::interval
		  WHERE code = $1 AND node_id = $2 AND status = 'OPEN'`, code, nodeID, ago); err != nil {
		t.Fatalf("age alert: %v", err)
	}
}

// countAlerts returns (open, total rows, expired) from the code for the Node. The
// total is what reveals reopening: each reopening is a new row.
func countAlerts(t *testing.T, pool *pgxpool.Pool, code, nodeID string) (open, total, expired int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FILTER (WHERE status = 'OPEN'), count(*), count(*) FILTER (WHERE expired)
		   FROM alerts WHERE code = $1 AND node_id = $2`, code, nodeID).Scan(&open, &total, &expired); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return open, total, expired
}

// Validity expires the alert WITHOUT notifying; the same episode does not reopen — the heart of
// #211. Without the episode, the condition (address changed, no inbound later) still holds
// after expiring, and the worker would reopen and notify on every validity cycle.
func TestReachabilityTTL_IPv6ExpiresWithoutNotifyingAndDoesNotReopen(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()
	spy := &spyNotifier{}
	w.alerts.SetNotifier(spy)

	w.evaluateIPv6Changed(ctx)
	if open, _, _ := countAlerts(t, pool, "NODE_IPV6_CHANGED", nodeID); open != 1 {
		t.Fatalf("expected 1 open alert, got %d", open)
	}

	age(t, pool, "NODE_IPV6_CHANGED", nodeID, "25 hours") // default validity: 24h
	for i := 0; i < 3; i++ {
		w.evaluateIPv6Changed(ctx)
	}

	open, total, expired := countAlerts(t, pool, "NODE_IPV6_CHANGED", nodeID)
	if open != 0 || expired != 1 {
		t.Fatalf("expected the expired alert (0 open, 1 expired), got %d open(s), %d expired(s)", open, expired)
	}
	if total != 1 {
		t.Fatalf("the same episode reopened after expiring: %d rows", total)
	}
	if got := len(spy.open); got != 1 {
		t.Fatalf("expected 1 notification (the open one), got %d", got)
	}
	if got := len(spy.closed); got != 0 {
		t.Fatalf("expiring cannot notify \"Resolved\" — nothing was fixed: %d", got)
	}
}

// A NEW rotation after expiring is a new episode and must alert: it is information that the
// operator (or n8n) has not received yet.
func TestReachabilityTTL_IPv6NewExchangeAfterExpiryReopens(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()
	spy := &spyNotifier{}
	w.alerts.SetNotifier(spy)

	w.evaluateIPv6Changed(ctx)
	age(t, pool, "NODE_IPV6_CHANGED", nodeID, "25 hours")
	w.evaluateIPv6Changed(ctx)

	const newestPrefix = "2001:db8:60f:200:be24:11ff:fe12:dee1"
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET previous_public_ipv6 = public_ipv6, public_ipv6 = $2,
		                   public_ipv6_changed_at = now() - interval '1 hour'
		   WHERE id = $1`, nodeID, newestPrefix); err != nil {
		t.Fatalf("simulate second rotation: %v", err)
	}
	w.evaluateIPv6Changed(ctx)

	open, total, _ := countAlerts(t, pool, "NODE_IPV6_CHANGED", nodeID)
	if open != 1 || total != 2 {
		t.Fatalf("new rotation should open a new alert: %d open alert(s), %d row(s)", open, total)
	}
	if got := len(spy.open); got != 2 {
		t.Fatalf("new rotation should notify: %d openings", got)
	}
}

// Reboot does not re-notify. Seen on 24/09/2026: the proxmox2 outage (03:42 → 07:38) closed the
// alert for "left ONLINE", recovery reopened it, and a new notification went out about a
// rotation from 22/09.
func TestReachabilityTTL_RebootDoesNotReopenSameEpisode(t *testing.T) {
	w, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()
	spy := &spyNotifier{}
	w.alerts.SetNotifier(spy)

	w.evaluateIPv6Changed(ctx)
	if _, err := pool.Exec(ctx, `UPDATE nodes SET status = 'OFFLINE' WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("take Node down: %v", err)
	}
	w.evaluateIPv6Changed(ctx)
	if _, err := pool.Exec(ctx, `UPDATE nodes SET status = 'ONLINE', last_seen_at = now() WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("bring Node back online: %v", err)
	}
	w.evaluateIPv6Changed(ctx)

	if open, total, _ := countAlerts(t, pool, "NODE_IPV6_CHANGED", nodeID); open != 0 || total != 1 {
		t.Fatalf("the Node's return reopened the same rotation: %d open alert(s), %d row(s)", open, total)
	}
	if got := len(spy.open); got != 1 {
		t.Fatalf("reboot notified the same rotation again: %d openings", got)
	}
}

// Validity 0 = no validity: the previous behavior, only closes with an inbound connection.
func TestReachabilityTTL_ZeroDoesNotExpire(t *testing.T) {
	_, pool, nodeID := setupIPv6Worker(t, oldPrefix, newPrefix, "2 hours", "", "15m")
	ctx := context.Background()
	w := NewWorker(pool, NewService(pool), slog.New(slog.NewTextHandler(io.Discard, nil)),
		settings.NewForTest(map[string]string{
			"alerting.node_offline_after": "90s",
			"alerting.ipv6_change_grace":  "15m",
			"alerting.reachability_ttl":   "0s",
		}))

	w.evaluateIPv6Changed(ctx)
	age(t, pool, "NODE_IPV6_CHANGED", nodeID, "30 days")
	w.evaluateIPv6Changed(ctx)

	if open, _, expired := countAlerts(t, pool, "NODE_IPV6_CHANGED", nodeID); open != 1 || expired != 0 {
		t.Fatalf("validity 0 expired the alert: %d open(s), %d expired(s)", open, expired)
	}
}

// The silence episode is last_incoming_at: expiring does not reopen while no inbound connection
// arrives; an inbound followed by new silence is a new episode and alerts again.
func TestReachabilityTTL_InboundSilentPerEpisode(t *testing.T) {
	w, pool, nodeID := setupInboundWorker(t, "20 hours", 0, 1, 1, "12h")
	ctx := context.Background()

	w.evaluateIncomingSilence(ctx)
	age(t, pool, "NODE_INBOUND_SILENT", nodeID, "25 hours")
	w.evaluateIncomingSilence(ctx)
	w.evaluateIncomingSilence(ctx)

	open, total, expired := countAlerts(t, pool, "NODE_INBOUND_SILENT", nodeID)
	if open != 0 || total != 1 || expired != 1 {
		t.Fatalf("expected 1 expired alert and none reopened: %d open(s), %d row(s), %d expired(s)",
			open, total, expired)
	}

	// Inbound arrived 13h ago and silence returned — another episode.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET last_incoming_at = now() - interval '13 hours' WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("simulate new silence: %v", err)
	}
	w.evaluateIncomingSilence(ctx)
	if open, total, _ := countAlerts(t, pool, "NODE_INBOUND_SILENT", nodeID); open != 1 || total != 2 {
		t.Fatalf("new silence should alert: %d open alert(s), %d row(s)", open, total)
	}
}

// The NODE_FIREWALLED episode is connectable_since: the same verdict does not reopen;
// an Engine restart (verdict restarts) is a new episode.
func TestReachabilityTTL_FirewalledPerEpisode(t *testing.T) {
	w, pool, nodeID := setupFirewalledWorker(t, "firewalled", "2 hours", "30m")
	ctx := context.Background()

	w.evaluateFirewalled(ctx)
	age(t, pool, "NODE_FIREWALLED", nodeID, "25 hours")
	w.evaluateFirewalled(ctx)
	w.evaluateFirewalled(ctx)

	if open, total, expired := countAlerts(t, pool, "NODE_FIREWALLED", nodeID); open != 0 || total != 1 || expired != 1 {
		t.Fatalf("expected 1 expired alert and none reopened: %d open(s), %d row(s), %d expired(s)",
			open, total, expired)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET connectable_since = now() - interval '1 hour' WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("simulate Engine restart: %v", err)
	}
	w.evaluateFirewalled(ctx)
	if open, total, _ := countAlerts(t, pool, "NODE_FIREWALLED", nodeID); open != 1 || total != 2 {
		t.Fatalf("new verdict should alert: %d open(s), %d line(s)", open, total)
	}
}

// The episode key is calculated in two places: in the worker (episodeSQL) and the backfill of
// migration 00041, which fills the alerts that were already open at deploy. Diverging makes
// the first cycle after the deploy treat each one of them as a new episode and notify.
func TestEpisodeSQL_EqualToMigrationBackfill(t *testing.T) {
	b, err := migrations.FS.ReadFile("00041_alert_episode_expiry.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for _, col := range []string{"n.public_ipv6_changed_at", "n.last_incoming_at", "n.connectable_since"} {
		expr := strings.TrimSuffix(strings.TrimPrefix(episodeSQL(col), "COALESCE("), ", '')")
		if !strings.Contains(string(b), expr) {
			t.Errorf("migration backfill does not use the same Worker key for %s: %s", col, expr)
		}
	}
}
