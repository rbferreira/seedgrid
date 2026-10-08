package alert

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rbferreira/seedgrid/control-plane/internal/capacity"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// Worker periodically evaluates deterministic alert rules (ADR-008).
//
// Implemented rules:
//   - NODE_OFFLINE (CRITICAL) — ONLINE Node whose last heartbeat is older than
//     offlineAfter is marked OFFLINE and an alert is opened; resolves when back ONLINE.
//   - DISK_IO_SATURATED (WARNING) — Node storage disk with utilization (busy%)
//     >= diskSatPct in a SUSTAINED manner (for `alerting.disk_saturated_for`, #150); resolves
//     below the slack (hysteresis) or when leaving ONLINE.
//   - NODE_FIREWALLED (WARNING) — ONLINE Node whose Engine reports 'firewalled' for more
//     than firewalledGrace: no one from the outside can open a connection to the BT port.
//
// NODE_FIREWALLED, NODE_INBOUND_SILENT and NODE_IPV6_CHANGED EXPIRE (#211, see expire):
// they infer reachability by the absence of input and would have no way to close on a Node with
// no demand.
//
// The DISK_LOW and SMART_CRITICAL rules depend on detailed inventory
// (disks/SMART), not yet collected by the Agent in V1 — they will be enabled when the
// Inventory Manager starts reporting this data.
type Worker struct {
	pool   *pgxpool.Pool
	alerts *Service
	logger *slog.Logger
	// cfg is the dynamic source of tunables (ADR-023): interval, offlineAfter, retention
	// and diskSatPct are read from the Store per pass → dashboard changes apply without redeploy.
	cfg *settings.Store

	// satSince tracks, per Node, since when the disk has been CONTINUOUSLY saturated (#150) —
	// the first pass that saw utilization >= threshold. Any pass without this reading
	// removes the entry (resets the count).
	//
	// It stays in memory, on purpose: it's derived state and cheap to rebuild, and a
	// Controller restart only DELAYS the alert by, at most, the grace period — it never creates
	// a false alarm, which is the safe side to err on. Persisting would require a migration and messing with
	// the hot path of the heartbeat to save a timestamp that the worker already sees on every
	// pass. Only the Run goroutine (and the tests, which call evaluate directly) access the
	// map; there is no lock because there is no concurrency.
	satSince map[string]time.Time
	// now is the grace clock; tests swap it to avoid actually sleeping.
	now func() time.Time
}

// Tunable accessors (ADR-023): read from the Store per call → hot-reload.
// retention <=0 disables purging; diskSatPct <=0 disables the DISK_IO_SATURATED rule.
func (w *Worker) interval() time.Duration     { return w.cfg.Duration("alerting.worker_interval") }
func (w *Worker) offlineAfter() time.Duration { return w.cfg.Duration("alerting.node_offline_after") }
func (w *Worker) retention() time.Duration    { return w.cfg.Duration("alerting.sample_retention") }
func (w *Worker) diskSatPct() float64         { return w.cfg.Float("alerting.disk_saturated_pct") }
func (w *Worker) diskMinFree() int64          { return w.cfg.Int64("admission.disk_min_free") }
func (w *Worker) firewalledGrace() time.Duration {
	return w.cfg.Duration("alerting.firewalled_grace")
}
func (w *Worker) storageGrace() time.Duration {
	return w.cfg.Duration("alerting.storage_grace")
}
func (w *Worker) incomingSilence() time.Duration {
	return w.cfg.Duration("alerting.incoming_silence")
}
func (w *Worker) incomingDemandSeedsPerLeecher() float64 {
	return w.cfg.Float("alerting.incoming_demand_seeds_per_leecher")
}
func (w *Worker) ipv6ChangeGrace() time.Duration {
	return w.cfg.Duration("alerting.ipv6_change_grace")
}
func (w *Worker) reachabilityTTL() time.Duration {
	return w.cfg.Duration("alerting.reachability_ttl")
}
func (w *Worker) diskSaturatedFor() time.Duration {
	return w.cfg.Duration("alerting.disk_saturated_for")
}

// clock returns the current instant, via the injectable clock (nil = real clock).
func (w *Worker) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

// episodeSQL formats a timestamptz as an alert episode key (#211): in explicit
// UTC and with microseconds, so the key doesn't depend on the session's TimeZone. The
// 00041 migration uses the SAME expression in the backfill of alerts that were already open —
// changing one without the other causes the first cycle after deploy to reopen everything.
func episodeSQL(col string) string {
	return `COALESCE(to_char(` + col + ` AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '')`
}

// expire closes by validity (`alerting.reachability_ttl`) the `code` alerts opened
// longer than it (#211). It applies to the three rules that infer reachability by the
// ABSENCE of inbound connection — NODE_FIREWALLED, NODE_INBOUND_SILENT and
// NODE_IPV6_CHANGED —, whose resolve depends on a connection that a Node with no demand never
// receives, even with the port open. Without validity they would stay open forever.
//
// The other rules don't expire: their resolve is the negation of the opening over a
// MEASURED variable (heartbeat, storage_ok, disk), so an open one is a condition
// that still exists.
//
// It runs before the "rule disabled" check of each caller: disabling the rule cannot
// leave an orphan alert with no path to close. Validity <= 0 = no validity.
func (w *Worker) expire(ctx context.Context, code string) {
	ttl := w.reachabilityTTL()
	if ttl <= 0 {
		return
	}
	ids, err := w.alerts.Expire(ctx, code, ttl)
	if err != nil {
		w.logger.Error("expire alerts", "error", err, "code", code)
		return
	}
	for _, id := range ids {
		w.logger.Info("alert expired after validity period", "code", code, "node_id", id, "ttl", ttl.String())
	}
}

// NewWorker creates the Alert Worker. Tunables come from the Store (ADR-023) and are read per
// pass; see the catalog (internal/settings) for defaults/semantics.
// The Alert Worker DOES NOT emit events: availability/saturation are ALERTS (open/resolve),
// not Events (point-in-time facts) — alert≠event separation.
func NewWorker(pool *pgxpool.Pool, alerts *Service, logger *slog.Logger, cfg *settings.Store) *Worker {
	return &Worker{pool: pool, alerts: alerts, logger: logger, cfg: cfg}
}

// Run executes the evaluation loop until the context is canceled.
// beat is optional (variadic to not break callers/tests): heartbeat marked on every
// cycle, exposed in the health API (see internal/health).
func (w *Worker) Run(ctx context.Context, beat ...func()) {
	interval := w.interval()
	w.logger.Info("alert worker started", "interval", interval.String(), "offline_after", w.offlineAfter().String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	mark := func() {
		for _, b := range beat {
			if b != nil {
				b()
			}
		}
	}

	w.evaluate(ctx) // immediate first evaluation
	mark()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("alert worker stopped")
			return
		case <-ticker.C:
			w.evaluate(ctx)
			mark()
			// Interval hot-reload (ADR-023).
			if cur := w.interval(); cur != interval {
				interval = cur
				ticker.Reset(cur)
				w.logger.Info("alert worker interval updated", "interval", cur.String())
			}
		}
	}
}

// evaluate runs a pass of all rules. Failures are logged, not propagated
// (controlled degradation: a rule with an error doesn't bring down the worker).
func (w *Worker) evaluate(ctx context.Context) {
	w.evaluateNodeOffline(ctx)
	w.evaluateDiskIOSaturated(ctx)
	w.evaluateStorageUnhealthy(ctx)
	w.evaluateDiskFull(ctx)
	w.evaluateFirewalled(ctx)
	w.evaluateIncomingSilence(ctx)
	w.evaluateIPv6Changed(ctx)
	w.purgeOldSamples(ctx)
}

// resolveVia closes the `code` alerts whose Node satisfies `nodesSQL` GOING THROUGH
// alert.Service — which is what triggers the AlertResolved hook (ADR-028).
//
// It exists because each rule did a direct UPDATE on the table, and the effect was that the
// resolution notification would NEVER go out (#161): the hook and the tunable
// `notify.alerts_on_resolve` (default **true**, promising "notifies when resolving")
// existed, but `Service.Resolve` didn't have a single caller in production. Opening
// notified; resolving didn't — no error in the log and nothing in the Portal indicating this.
//
// `nodesSQL` is a SELECT of node ids and keeps the $1..$N numbering of its own
// args; `code` comes in as the last parameter, so as not to renumber anything.
//
// Resolve is called ONCE PER NODE instead of a batch UPDATE. It costs one more query per
// closed alert, and it's what allows the Service to return severity, message and
// payload for EACH alert — without them the notification would go out without hostname or reason.
func (w *Worker) resolveVia(ctx context.Context, code, nodesSQL string, args ...any) {
	q := `SELECT DISTINCT a.node_id::text FROM alerts a
	       WHERE a.status = 'OPEN' AND a.code = $` + strconv.Itoa(len(args)+1) + `
	         AND a.node_id IN (` + nodesSQL + `)`
	rows, err := w.pool.Query(ctx, q, append(args, code)...)
	if err != nil {
		w.logger.Error("select alerts to resolve", "error", err, "code", code)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			w.logger.Error("read alert to resolve", "error", err, "code", code)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.logger.Error("iterate alerts to resolve", "error", err, "code", code)
		return
	}
	for _, id := range ids {
		if _, err := w.alerts.Resolve(ctx, code, id); err != nil {
			w.logger.Error("resolve alert", "error", err, "code", code, "node_id", id)
			continue
		}
		w.logger.Info("alert resolved", "code", code, "node_id", id)
	}
}

// evaluateIPv6Changed opens NODE_IPV6_CHANGED (WARNING) when the global IPv6 address of
// a Node changes — a sign that the provider rotated the delegated prefix and that the
// firewall rules that allowed the old address now point to nowhere.
//
// The incident that originated the rule (05-06/09/2026): prefix changed from
// 2001:db8:601:d300::/64 to 2001:db8:60b:c000::/64 between 31/07 and 01/09; the gateway
// rules used the LITERAL address; three Nodes were unreachable for weeks with 553
// torrents without a single connection — and the Portal was green all along. Fixing the rule is
// on the gateway's side; the failure being SILENT is on SeedGrid.
//
// It is an ALERT and not an Event (the alert/worker.go separation): the change itself is a
// point-in-time fact, but the CONSEQUENCE — "the firewall rules are outdated" — is a
// condition that persists until someone acts, and it has to stay noisy until then.
//
// Cooldown (anti-flapping): IPv6 privacy extensions rotate the suffix on their own.
// The Agent already orders to report the stable address, but a Node with the wrong
// configuration would oscillate; requiring the new address to hold for a while prevents an alarm on
// every ephemeral change. Cooldown <= 0 disables the rule.
//
// RESOLVE: only possible because of #155. The right question is not "did the address stop
// changing" but "is the firewall allowing traffic again" — and it is last_incoming_at, which decays,
// that answers this. With sticky engine_connectable the Node would keep saying
// 'connected' and the alert would only clear manually.
//
// INVARIANT (#204): opening and resolving MUST be mutually exclusive. Here this does not
// come for free, because the two conditions look at different variables: the opening is a
// permanent state ("the address is different from the previous one", which nothing ever undoes) and the
// resolving is an event that comes later ("it started receiving inbound traffic again"). When both
// applied at the same time, the worker opened and closed the alert in the same pass, every 30s,
// notifying on all of them — the other rules in this worker don't have this problem because their
// resolving is the negation of the opening on the SAME variable.
func (w *Worker) evaluateIPv6Changed(ctx context.Context) {
	w.expire(ctx, "NODE_IPV6_CHANGED")
	grace := w.ipv6ChangeGrace()
	if grace <= 0 {
		// A disabled rule holds no opinion: it closes whatever was left open. Without this,
		// disabling the rule to stop a problem leaves ORPHAN alerts on the dashboard, with
		// no path to resolve them — that was the side effect of the #204 mitigation.
		w.resolveVia(ctx, "NODE_IPV6_CHANGED", `SELECT id FROM nodes WHERE deleted_at IS NULL`)
		return
	}
	rows, err := w.pool.Query(ctx,
		`SELECT id::text, hostname, COALESCE(previous_public_ipv6, ''),
		        COALESCE(public_ipv6, ''), COALESCE(bt_port, 0),
		        `+episodeSQL("public_ipv6_changed_at")+`
		   FROM nodes
		  WHERE deleted_at IS NULL AND status = 'ONLINE'
		    AND previous_public_ipv6 IS NOT NULL AND previous_public_ipv6 <> ''
		    AND public_ipv6 IS NOT NULL AND public_ipv6 <> ''
		    AND public_ipv6 IS DISTINCT FROM previous_public_ipv6
		    AND public_ipv6_changed_at IS NOT NULL
		    AND public_ipv6_changed_at < now() - make_interval(secs => $1)
		    -- Mutual exclusion with the RESOLVE just below (#204): nothing clears
		    -- previous_public_ipv6, so "the address changed" is PERMANENT. Without this
		    -- line, a Node that starts receiving input again satisfies the opening and resolves in the
		    -- SAME pass; since the Open dedup only applies while the alert is OPEN, the
		    -- worker reopens every cycle and always notifies.
		    AND (last_incoming_at IS NULL OR last_incoming_at <= public_ipv6_changed_at)`,
		grace.Seconds())
	if err != nil {
		w.logger.Error("detect IPv6 change", "error", err)
		return
	}
	// episode = the ROTATION (public_ipv6_changed_at, #211): one alert per change, which does not
	// reopen after expiring nor after a reboot. New change, new key.
	type chg struct {
		id, hostname, prev, cur, episode string
		port                             int
	}
	var changes []chg
	for rows.Next() {
		var c chg
		if err := rows.Scan(&c.id, &c.hostname, &c.prev, &c.cur, &c.port, &c.episode); err != nil {
			rows.Close()
			w.logger.Error("read IPv6 change", "error", err)
			return
		}
		changes = append(changes, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.logger.Error("iterate IPv6 changes", "error", err)
		return
	}
	for _, c := range changes {
		msg := fmt.Sprintf("Node IPv6 changed — firewall rules that allowed %s "+
			"are no longer valid; allow [%s]:%d (TCP+UDP)", c.prev, c.cur, c.port)
		payload := map[string]any{
			"hostname":      c.hostname,
			"node_id":       c.id,
			"previous_ipv6": c.prev,
			"current_ipv6":  c.cur,
			"bt_port":       c.port,
		}
		opened, err := w.alerts.OpenEpisode(ctx, "NODE_IPV6_CHANGED", "WARNING", c.id, c.episode, msg, payload)
		if err != nil {
			w.logger.Error("open alert NODE_IPV6_CHANGED", "error", err, "node_id", c.id)
			continue
		}
		if opened {
			w.logger.Warn("Node IPv6 changed",
				"node_id", c.id, "hostname", c.hostname, "previous", c.prev, "current", c.cur)
			continue
		}
		// There was already an OPEN alert for this Node — it could be the SAME rotation (nothing to do,
		// UpdatePayload is a no-op because the message hasn't changed) or a NEW rotation that arrived
		// while the previous one hadn't resolved, because the Node hasn't received inbound traffic for days (#209).
		// Without this, whoever consumes the alert (n8n via webhook, Portal) never knows the most
		// recent address: they get stuck reacting to an address that is no longer the current problem.
		updated, err := w.alerts.UpdatePayload(ctx, "NODE_IPV6_CHANGED", c.id, "WARNING", c.episode, msg, payload)
		if err != nil {
			w.logger.Error("update alert NODE_IPV6_CHANGED", "error", err, "node_id", c.id)
			continue
		}
		if updated {
			w.logger.Warn("Node IPv6 changed again with previous alert still open",
				"node_id", c.id, "hostname", c.hostname, "previous", c.prev, "current", c.cur)
		}
	}

	// Resolves when the Node starts RECEIVING inbound connections again after the change — proof
	// that the firewall rule was redone. It's the condition that #155 unlocked; without a decaying
	// signal there was no way for the CP to know the firewall was fixed.
	w.resolveVia(ctx, "NODE_IPV6_CHANGED", `SELECT id FROM nodes
		                     WHERE deleted_at IS NULL
		                       AND (status <> 'ONLINE'
		                            OR (last_incoming_at IS NOT NULL
		                                AND public_ipv6_changed_at IS NOT NULL
		                                AND last_incoming_at > public_ipv6_changed_at))`)

}

// inboundSilentMessage is the text for NODE_INBOUND_SILENT, and it is CONSTANT on purpose (#203).
//
// The text is written once, on opening, and the alert can stay open for days. Everything it
// said that changes over time grew stale along with it: the IPv6 address (the Node changed
// address almost 38h after the alert opened and the dashboard kept telling to allow an address that
// no longer existed) and the "12h0m22s ago" (frozen at the opening value). What varies per
// alert goes in the payload — address, port, silence start, demand that triggered the rule —
// and whoever needs the CURRENT address reads the Node: the Portal shows the Node's, and the notification
// (which is an instant, not a state) carries the payload's.
const inboundSilentMessage = "Node has no inbound connections beyond tolerance, despite demand — " +
	"may be unreachable: check if the Node's BT port is open for its current address"

// firewalledMessage is the text for NODE_FIREWALLED, and it is CONSTANT on purpose (#223).
//
// The text is written once, on opening, and the alert can stay open for days. Everything it
// said that changes over time grew stale along with it: the IPv6 address (the Node changed
// address while the alert was open and the dashboard kept telling to allow an address that no
// longer existed) and the "Xs ago" (frozen at the opening value). What varies per alert goes in the
// payload — address, port, verdict start — and whoever needs the CURRENT address reads the
// Node: the Portal shows the Node's, and the notification (which is an instant, not a state) carries
// the payload's.
const firewalledMessage = "Node Engine reports firewalled beyond grace period — " +
	"no one from the outside can open a connection to the BT port: " +
	"check if the Node's BT port is open for its current address"

// evaluateIncomingSilence opens NODE_INBOUND_SILENT (WARNING) for ONLINE Nodes that
// spent the entire window with NO incoming connection, PROVIDED there is demand.
//
// This is the rule requested by #155, and it exists because NODE_FIREWALLED below cannot
// do this: it reads engine_connectable, which comes from has_incoming_connections
// from libtorrent — a STICKY boolean, which turns 'connected' on the first connection of
// the session and never goes back. Measured on 06/09/2026: node-01 and node-03 reported
// 'connected' with the Engine up for 1 month and 5 days, spanning the entire period they
// were unreachable. Here the signal DECAYS: it is the age of last_incoming_at.
//
// Division of labor between the two rules, deliberate:
//   - NODE_FIREWALLED  → ONBOARDING. A Node whose port was never opened is born
//     'firewalled' and never leaves; the sticky verdict serves this case well.
//   - NODE_INBOUND_SILENT → REGIME. A Node that WAS reachable and stopped being — the case
//     that slipped through completely, and which motivated the issue.
//
// Consolidating both into a single code is the natural step once this signal proves
// it behaves in the field; until then it's worth being able to compare both verdicts side by side.
//
// Why require demand: with no leecher in the swarm, 0 incoming connections is the
// CORRECT state of a seeder — nobody wants the content. Measured on 07/09/2026, the four
// Nodes (all healthy) had 0, 1, 1 and 0 incoming connections at the instant of the
// read; alerting on pure absence would yield a false positive on a healthy Node.
//
// Why WEIGH the demand (#206): "there is a leecher" is not demand on THIS Node. A
// leecher in a swarm with 63 seeds connects to half a dozen of them, and staying hours without
// being one of the chosen ones is expected behavior, not a symptom. Measured on 09/09/2026,
// the six torrents that armed the rule on healthy Nodes had 2 to 63 seeds per
// leecher; out of the 27 alerts opened between 08/09 and 30/09, only the first has a real
// documented problem. The useful question is "would someone probably need me?", so a torrent only
// counts when num_seeds <= num_leechers × alerting.incoming_demand_seeds_per_leecher
// (default 1: leechers are at least as numerous as seeds, meaning there are more
// mouths than servers). num_seeds is the SWARM total (tracker scrape) and may
// include the Node itself — so 1 leecher / 1 seed arms (probably just us) and
// 1 leecher / 2 seeds does not. A swarm with NO seeds (0) arms for any value >= 0: the
// leecher has no one to download from. A NULL counter on either side leaves the
// torrent out — unknown never triggers — and 0 leechers is never demand.
//
// ⚠️ Known limitation: demand is evaluated at the INSTANT of the run, not accumulated over
// the window — there is no leecher history in the database (torrent_stat_samples stores bytes and
// speed, not peers). A Node with no demand at all at the moment of evaluation does not
// alert, even if there was demand during the window. It prefers silence over shouting
// wrong; accumulating demand requires a new column and is left for when this signal matures.
// The weighing reduces the noise of this limitation, it doesn't eliminate it: a leecher that just
// arrived in a needy swarm still arms a rule whose window was already expired.
//
// NULL semantics: incoming_peers NULL (Agent that doesn't sample) never alerts.
// last_incoming_at NULL = we haven't seen an incoming since the column exists — doesn't alert
// here, because we can't distinguish "newly deployed" from "always unreachable";
// this is exactly the onboarding case that NODE_FIREWALLED covers.
// Window <= 0 disables the rule.
func (w *Worker) evaluateIncomingSilence(ctx context.Context) {
	w.expire(ctx, "NODE_INBOUND_SILENT")
	silence := w.incomingSilence()
	if silence <= 0 {
		return
	}
	rows, err := w.pool.Query(ctx,
		`SELECT n.id::text, n.hostname, COALESCE(n.public_ipv6, ''), COALESCE(n.bt_port, 0),
		        EXTRACT(EPOCH FROM (now() - n.last_incoming_at))::bigint,
		        `+episodeSQL("n.last_incoming_at")+`,
		        d.torrents, d.leechers, d.seeds
		   FROM nodes n
		   JOIN LATERAL (
		        -- Weighted demand (#206). NULL in num_leechers or num_seeds makes the
		        -- predicate NULL, and the torrent is left out: unknown never flags.
		        SELECT count(*) AS torrents,
		               COALESCE(sum(t.num_leechers), 0)::bigint AS leechers,
		               COALESCE(sum(t.num_seeds), 0)::bigint AS seeds
		          FROM torrents t
		         WHERE t.node_id = n.id AND t.deleted_at IS NULL
		           AND t.num_leechers > 0
		           AND t.num_seeds <= t.num_leechers * $3::float8) d ON d.torrents > 0
		  WHERE n.deleted_at IS NULL AND n.status = 'ONLINE'
		    AND n.last_seen_at IS NOT NULL AND n.last_seen_at > now() - make_interval(secs => $1)
		    AND n.incoming_peers IS NOT NULL
		    AND n.last_incoming_at IS NOT NULL
		    AND n.last_incoming_at < now() - make_interval(secs => $2)`,
		w.offlineAfter().Seconds(), silence.Seconds(), w.incomingDemandSeedsPerLeecher())
	if err != nil {
		w.logger.Error("detect nodes without incoming connection", "error", err)
		return
	}
	// episode = the START of the silence (last_incoming_at, #211). It only changes when an incoming
	// connection arrives, so a new silence after it is a new episode.
	type silent struct {
		id, hostname, ipv6, episode string
		port                        int
		secs                        int64
		// The demand that armed the rule: how many torrents, and the total leechers and seeds
		// of them. It goes in the payload and log — with no peer history in the database, it's the only
		// trace of WHY the alert opened, and what allows calibrating the tunable later.
		torrents, leechers, seeds int64
	}
	var quiet []silent
	for rows.Next() {
		var q silent
		if err := rows.Scan(&q.id, &q.hostname, &q.ipv6, &q.port, &q.secs, &q.episode,
			&q.torrents, &q.leechers, &q.seeds); err != nil {
			rows.Close()
			w.logger.Error("read node without incoming connection", "error", err)
			return
		}
		quiet = append(quiet, q)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.logger.Error("iterate nodes without incoming connection", "error", err)
		return
	}
	for _, q := range quiet {
		// The address and port only go in the payload (#203): they are what the operator needs
		// to unblock, but the text is written once and the address changes when the provider
		// rotates the prefix. public_ipv6 here is from the OPENING; the current one is in the Node.
		payload := map[string]any{
			"hostname":         q.hostname,
			"public_ipv6":      q.ipv6,
			"bt_port":          q.port,
			"last_incoming_at": q.episode,
			"demand_torrents":  q.torrents,
			"demand_leechers":  q.leechers,
			"demand_seeds":     q.seeds,
		}
		opened, err := w.alerts.OpenEpisode(ctx, "NODE_INBOUND_SILENT", "WARNING", q.id, q.episode,
			inboundSilentMessage, payload)
		if err != nil {
			w.logger.Error("open alert NODE_INBOUND_SILENT", "error", err, "node_id", q.id)
			continue
		}
		if opened {
			w.logger.Warn("node without incoming connection with demand present",
				"node_id", q.id, "hostname", q.hostname, "silence", time.Duration(q.secs)*time.Second,
				"demand_torrents", q.torrents, "demand_leechers", q.leechers, "demand_seeds", q.seeds)
		}
	}

	// Resolves as soon as it starts receiving incoming again (or the Node leaves ONLINE): here the
	// resolve is honest precisely because the signal decays — last_incoming_at becomes
	// recent on its own, without depending on restarting the Engine.
	w.resolveVia(ctx, "NODE_INBOUND_SILENT", `SELECT id FROM nodes
		                     WHERE deleted_at IS NULL
		                       AND (status <> 'ONLINE'
		                            OR (last_incoming_at IS NOT NULL
		                                AND last_incoming_at >= now() - make_interval(secs => $1)))`, silence.Seconds())

}

// evaluateFirewalled opens NODE_FIREWALLED (WARNING) for ONLINE Nodes whose Engine
// reports 'firewalled' in a SUSTAINED way — no one from outside managed to open a connection
// to the BitTorrent port for longer than the grace period.
//
// This is the alert that didn't exist on 05-06/09/2026, when the provider rotated the IPv6
// prefix and three Nodes were UNREACHABLE for WEEKS (553 torrents without a single connection)
// with the Portal showing them all green: ONLINE status, reachable Engine, high health.
// Nothing in the product measured if anyone could reach the Node — only the tracker knew.
//
// Why SUSTAINED and not immediate: 'firewalled' is the has_incoming_connections from
// libtorrent, which starts false. Every newly spun-up Node is firewalled until the first
// peer hits the port; alerting on the first cycle would guarantee an alarm on every Engine
// restart — and an alert that shouts for no reason is an alert the operator learns to ignore.
// connectable_since (migration 00035) stores since when the current verdict is valid.
//
// NULL semantics: unknown (Agent that doesn't report the field) never alerts.
// Grace period <= 0 disables the rule.
func (w *Worker) evaluateFirewalled(ctx context.Context) {
	w.expire(ctx, "NODE_FIREWALLED")
	grace := w.firewalledGrace()
	if grace <= 0 {
		return
	}
	rows, err := w.pool.Query(ctx,
		`SELECT id::text, hostname, COALESCE(public_ipv6, ''), COALESCE(bt_port, 0),
		        EXTRACT(EPOCH FROM (now() - connectable_since))::bigint,
		        `+episodeSQL("connectable_since")+`
		   FROM nodes
		  WHERE deleted_at IS NULL AND status = 'ONLINE'
		    AND last_seen_at IS NOT NULL AND last_seen_at > now() - make_interval(secs => $1)
		    AND engine_connectable = 'firewalled'
		    AND connectable_since IS NOT NULL
		    AND connectable_since < now() - make_interval(secs => $2)`,
		w.offlineAfter().Seconds(), grace.Seconds())
	if err != nil {
		w.logger.Error("detect firewalled nodes", "error", err)
		return
	}
	// episode = the VERDICT (connectable_since, #211): restarts when the Engine changes its
	// mind — an Engine restart goes back to 'firewalled' and is a new episode.
	type fw struct {
		id, hostname, ipv6, episode string
		port                        int
		secs                        int64
	}
	var blocked []fw
	for rows.Next() {
		var f fw
		if err := rows.Scan(&f.id, &f.hostname, &f.ipv6, &f.port, &f.secs, &f.episode); err != nil {
			rows.Close()
			w.logger.Error("read firewalled node", "error", err)
			return
		}
		blocked = append(blocked, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.logger.Error("iterate firewalled nodes", "error", err)
		return
	}
	for _, f := range blocked {
		// The address and port only go in the payload (#223): they are what the operator needs
		// to unblock, but the text is written once and the address changes when the provider
		// rotates the prefix. public_ipv6 here is from the OPENING; the current one is in the Node.
		payload := map[string]any{
			"hostname":          f.hostname,
			"public_ipv6":       f.ipv6,
			"bt_port":           f.port,
			"connectable_since": f.episode,
		}
		opened, err := w.alerts.OpenEpisode(ctx, "NODE_FIREWALLED", "WARNING", f.id, f.episode,
			firewalledMessage, payload)
		if err != nil {
			w.logger.Error("open alert NODE_FIREWALLED", "error", err, "node_id", f.id)
			continue
		}
		if opened {
			// The log is an instant, not a state: here the address doesn't age and saves the trip
			// to the Node.
			w.logger.Warn("node firewalled",
				"node_id", f.id, "hostname", f.hostname, "public_ipv6", f.ipv6, "bt_port", f.port,
				"firewalled_ha", time.Duration(f.secs)*time.Second)
		}
	}

	// Resolves as soon as the Engine stops reporting 'firewalled' (received connection, became
	// unknown, or the Node left ONLINE) — no grace period on the way back.
	w.resolveVia(ctx, "NODE_FIREWALLED", `SELECT id FROM nodes
		         WHERE deleted_at IS NULL
		           AND (engine_connectable IS DISTINCT FROM 'firewalled' OR status <> 'ONLINE')`)

}

// evaluateStorageUnhealthy opens NODE_STORAGE (CRITICAL) for ONLINE Nodes (recent
// heartbeat) whose storage (/data) the Agent reported as NOT mounted/writable
// (storage_ok = false), and resolves when it recovers (storage_ok true/NULL) or the Node leaves
// ONLINE. It's the signal that avoids accessing each Node via SSH to see if the drive crashed.
// NULL semantics: unknown (old agent) → IS NOT FALSE → never alerts.
//
// Requires SUSTAINED failure (#177): the heartbeat is 30s and the rule opened on ONE reading,
// so an I/O hiccup — expected on underpowered USB drives, the case of node-03 — turned into
// CRITICAL resolved 30 seconds later. `storage_failing_since` (00040) says since
// when it failed, and the AGE of the timestamp is compared against `alerting.storage_grace` — 1m by
// default, enough for the failure to survive more than one measurement.
//
// RESOLUTION still has no grace period: storage that recovered is good news, and holding it
// back protects no one — it's the same asymmetry as NODE_FIREWALLED.
func (w *Worker) evaluateStorageUnhealthy(ctx context.Context) {
	grace := w.storageGrace()
	// 1) Open for Nodes with SUSTAINED failed storage and recent heartbeat.
	rows, err := w.pool.Query(ctx,
		`SELECT id::text, hostname, COALESCE(storage_detail, '') FROM nodes
		  WHERE deleted_at IS NULL AND status = 'ONLINE'
		    AND last_seen_at IS NOT NULL AND last_seen_at > now() - make_interval(secs => $1)
		    AND storage_ok IS FALSE
		    AND (storage_failing_since IS NULL
		         OR storage_failing_since <= now() - make_interval(secs => $2))`,
		w.offlineAfter().Seconds(), grace.Seconds())
	if err != nil {
		w.logger.Error("detect faulty storage", "error", err)
		return
	}
	type bad struct{ id, hostname, detail string }
	var unhealthy []bad
	for rows.Next() {
		var b bad
		if err := rows.Scan(&b.id, &b.hostname, &b.detail); err != nil {
			rows.Close()
			w.logger.Error("read node with failed storage", "error", err)
			return
		}
		unhealthy = append(unhealthy, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.logger.Error("iterate nodes with failed storage", "error", err)
		return
	}
	for _, b := range unhealthy {
		// The detail does NOT go in the message: it already travels in the payload, and the notifier
		// renders it as "Detail:" — concatenating it here made the operator read the same
		// sentence twice in the same notification (#178).
		const msg = "Node storage is not mounted/writable"
		opened, err := w.alerts.Open(ctx, "NODE_STORAGE", "CRITICAL", b.id,
			msg, map[string]any{"hostname": b.hostname, "detail": b.detail})
		if err != nil {
			w.logger.Error("open alert NODE_STORAGE", "error", err, "node_id", b.id)
			continue
		}
		if opened {
			w.logger.Warn("faulty storage", "node_id", b.id, "hostname", b.hostname, "detail", b.detail)
		}
	}

	// 2) Resolve when the storage recovered (true/NULL) or the Node is no longer ONLINE.
	w.resolveVia(ctx, "NODE_STORAGE", `SELECT id FROM nodes
		         WHERE deleted_at IS NULL AND (storage_ok IS NOT FALSE OR status <> 'ONLINE')`)

}

// evaluateDiskFull opens NODE_DISK_FULL (WARNING) for ONLINE Nodes (recent heartbeat,
// with disk data) whose PROJECTED FREE SPACE fell below the admission.disk_min_free margin
// (ADR-032) — meaning the Node committed to more torrents than the disk can hold.
// It's the warning that catches the already overflowing Node (the case of node-03), which the add/migration block
// alone doesn't undo. Resolves when it returns above the margin, the Node leaves ONLINE, or loses the
// disk data. Deduplicates by (code, node_id). minFree<=0 disables the rule.
func (w *Worker) evaluateDiskFull(ctx context.Context) {
	minFree := w.diskMinFree()
	if minFree <= 0 {
		return
	}
	pf := capacity.ProjectedFreeExpr()
	rows, err := w.pool.Query(ctx,
		`SELECT n.id::text, n.hostname, `+pf+`::bigint AS proj_free
		   FROM nodes n JOIN heartbeats h ON h.node_id = n.id
		  WHERE n.deleted_at IS NULL AND n.status = 'ONLINE'
		    AND n.last_seen_at IS NOT NULL AND n.last_seen_at > now() - make_interval(secs => $1)
		    AND h.disk_total_bytes > 0
		    AND `+pf+` < $2`,
		w.offlineAfter().Seconds(), minFree)
	if err != nil {
		w.logger.Error("detect full disk", "error", err)
		return
	}
	type full struct {
		id, hostname string
		projFree     int64
	}
	var over []full
	for rows.Next() {
		var f full
		if err := rows.Scan(&f.id, &f.hostname, &f.projFree); err != nil {
			rows.Close()
			w.logger.Error("read node with full disk", "error", err)
			return
		}
		over = append(over, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.logger.Error("iterate nodes with full disk", "error", err)
		return
	}
	for _, f := range over {
		// Deficit = how much is missing to return within the margin (projFree can be negative:
		// in this case the Node is already committed beyond its own free disk).
		shortfall := minFree - f.projFree
		msg := fmt.Sprintf("Node %s does not have enough space to complete all queued torrents (~%s short). Migrate torrents to another Node.",
			f.hostname, capacity.HumanBytes(shortfall))
		opened, err := w.alerts.Open(ctx, "NODE_DISK_FULL", "WARNING", f.id,
			msg, map[string]any{"hostname": f.hostname, "projected_free_bytes": f.projFree, "min_free_bytes": minFree})
		if err != nil {
			w.logger.Error("open NODE_DISK_FULL alert", "error", err, "node_id", f.id)
			continue
		}
		if opened {
			w.logger.Warn("disk full", "node_id", f.id, "hostname", f.hostname, "projected_free_bytes", f.projFree)
		}
	}

	// Resolve: returned above the margin, lost disk data, or left ONLINE.
	w.resolveVia(ctx, "NODE_DISK_FULL", `SELECT n.id FROM nodes n
		        LEFT JOIN heartbeats h ON h.node_id = n.id
		         WHERE n.deleted_at IS NULL
		           AND ( n.status <> 'ONLINE'
		              OR h.disk_total_bytes IS NULL OR h.disk_total_bytes = 0
		              OR `+pf+` >= $1 )`, minFree)

}

// evaluateDiskIOSaturated opens DISK_IO_SATURATED (WARNING) for ONLINE Nodes (with
// recent heartbeat) whose storage disk has utilization >= diskSatPct in a
// SUSTAINED way, and resolves when it drops below the slack threshold (15pp hysteresis) or the
// Node leaves ONLINE. Deduplicates by (code, node_id) while OPEN. Signal reported by the
// Agent in the heartbeat (busy% between heartbeats) — see [[Pi USB drive incident]].
//
// Requires SUSTAINED saturation (#150): the reading is the ~10s average between two heartbeats, and
// the rule opened on ONE of them. On a USB drive — jagged by nature: 87% in one interval,
// 13% five minutes later, measured on node-04 on 06/09/2026 — a seconds-long spike turned into
// an alert. Measured in the production DB, out of 18 alerts opened between 12/09 and 02/10/2026,
// 16 lasted exactly 30s — one Worker pass; the other two lasted 9min and 4m30,
// the kind of episode the rule still catches if the saturation holds. The Portal graph uses a 2-minute window
// and never showed these spikes, so the alert looked false.
//
// The Node must be >= threshold on ALL passes during `alerting.disk_saturated_for`
// (w.satSince keeps the start of the sequence). One pass below the threshold — or with the Node
// out of ONLINE/without recent heartbeat — resets the count. Granularity is the Worker's
// (`alerting.worker_interval`): the alert fires on the first pass where the time has already been
// met. Grace <= 0 alerts on the first reading, as before.
//
// RESOLUTION gets no grace period: disk that eased up is good news. The 15pp hysteresis
// still applies — a reading between slack and threshold doesn't resolve the open alert, but
// it also doesn't count as saturated (resets the sequence).
func (w *Worker) evaluateDiskIOSaturated(ctx context.Context) {
	diskSatPct := w.diskSatPct()
	if diskSatPct <= 0 {
		w.satSince = nil // disabled rule doesn't keep the sequence: re-enabling starts from zero
		return
	}
	resolvePct := diskSatPct - 15 // hysteresis: prevents flapping at the threshold edge
	if resolvePct < 0 {
		resolvePct = 0
	}

	// 1) Open for saturated Nodes (recent heartbeat to avoid alerting on stale reading).
	rows, err := w.pool.Query(ctx,
		`SELECT id::text, hostname, disk_io_util_pct FROM nodes
		  WHERE deleted_at IS NULL AND status = 'ONLINE'
		    AND last_seen_at IS NOT NULL AND last_seen_at > now() - make_interval(secs => $1)
		    AND disk_io_util_pct >= $2`,
		w.offlineAfter().Seconds(), diskSatPct)
	if err != nil {
		// Without a reliable reading the sequence can't keep counting: keeping it would make a
		// DB failure in the middle of it look like "continuously saturated".
		w.satSince = nil
		w.logger.Error("detect saturated disk", "error", err)
		return
	}
	type sat struct {
		id, hostname string
		util         float64
	}
	var saturated []sat
	for rows.Next() {
		var s sat
		if err := rows.Scan(&s.id, &s.hostname, &s.util); err != nil {
			rows.Close()
			w.satSince = nil
			w.logger.Error("read saturated node", "error", err)
			return
		}
		saturated = append(saturated, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.satSince = nil
		w.logger.Error("iterate saturated nodes", "error", err)
		return
	}

	// Continuous sequence: those still saturated keep the start; those who just entered start
	// at `now`; those who left the set disappear from the map (rebuilt on each pass, which
	// also discards removed Nodes).
	grace := w.diskSaturatedFor()
	now := w.clock()
	seq := make(map[string]time.Time, len(saturated))
	for _, s := range saturated {
		since, ok := w.satSince[s.id]
		if !ok {
			since = now
			w.logger.Debug("disk above threshold, waiting for sustainment",
				"node_id", s.id, "hostname", s.hostname, "util_pct", s.util, "grace", grace.String())
		}
		seq[s.id] = since
	}
	for id, since := range w.satSince {
		if _, still := seq[id]; !still {
			w.logger.Debug("disk no longer above threshold",
				"node_id", id, "since", since.UTC().Format(time.RFC3339),
				"so_far", now.Sub(since).Round(time.Second).String(), "grace", grace.String())
		}
	}
	w.satSince = seq

	// Slack of 1/10 of the Worker interval. Passes are periodic, but the instant each
	// one reads the clock varies by a few milliseconds (query latency, scheduling);
	// without slack, a grace MULTIPLE of the interval — the 90s over 30s default — would fall
	// on a tie and the opening would be a whole pass late about half the
	// time, because 89.99s < 90s. The slack only decides this tie: observed times are
	// multiples of the interval, so nothing below it goes through it.
	slack := w.interval() / 10
	for _, s := range saturated {
		if now.Sub(seq[s.id])+slack < grace {
			continue // it might still just be a spike
		}
		opened, err := w.alerts.Open(ctx, "DISK_IO_SATURATED", "WARNING", s.id,
			"Node disk saturated (I/O)", map[string]any{"hostname": s.hostname, "util_pct": s.util})
		if err != nil {
			w.logger.Error("open DISK_IO_SATURATED alert", "error", err, "node_id", s.id)
			continue
		}
		if opened {
			// Condition lives in the ALERT (opens/resolves), not in Events.
			w.logger.Warn("disk saturated", "node_id", s.id, "hostname", s.hostname, "util_pct", s.util,
				"saturated_for", now.Sub(seq[s.id]).Round(time.Second).String())
		}
	}

	// 2) Resolve when the disk eased up (below the slack) or the Node is no longer ONLINE.
	w.resolveVia(ctx, "DISK_IO_SATURATED", `SELECT id FROM nodes
		         WHERE deleted_at IS NULL AND (disk_io_util_pct < $1 OR status <> 'ONLINE')`, resolvePct)

}

// purgeOldSamples removes time-series samples older than the configured
// retention (best-effort: failure is logged, doesn't crash the Worker).
func (w *Worker) purgeOldSamples(ctx context.Context) {
	if w.retention() <= 0 {
		return
	}
	if _, err := w.pool.Exec(ctx,
		`DELETE FROM torrent_stat_samples WHERE sampled_at < now() - make_interval(secs => $1)`,
		w.retention().Seconds()); err != nil {
		w.logger.Error("purge torrent samples", "error", err)
	}
}

// evaluateNodeOffline detects Nodes whose heartbeat expired and resolves alerts for
// Nodes that came back.
func (w *Worker) evaluateNodeOffline(ctx context.Context) {
	// 1) Newly-offline Nodes: ONLINE with expired heartbeat -> OFFLINE + alert.
	rows, err := w.pool.Query(ctx,
		`UPDATE nodes SET status = 'OFFLINE', updated_at = now()
		  WHERE deleted_at IS NULL AND status = 'ONLINE'
		    AND last_seen_at IS NOT NULL
		    AND last_seen_at < now() - make_interval(secs => $1)
		 RETURNING id::text, hostname`,
		w.offlineAfter().Seconds())
	if err != nil {
		w.logger.Error("detect offline nodes", "error", err)
		return
	}
	type off struct{ id, hostname string }
	var offline []off
	for rows.Next() {
		var o off
		if err := rows.Scan(&o.id, &o.hostname); err != nil {
			rows.Close()
			w.logger.Error("read offline node", "error", err)
			return
		}
		offline = append(offline, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		w.logger.Error("iterate offline nodes", "error", err)
		return
	}

	for _, o := range offline {
		opened, err := w.alerts.Open(ctx, "NODE_OFFLINE", "CRITICAL", o.id,
			"Node without heartbeat: marked OFFLINE", map[string]any{"hostname": o.hostname})
		if err != nil {
			w.logger.Error("open NODE_OFFLINE alert", "error", err, "node_id", o.id)
			continue
		}
		if opened {
			// Node availability (offline/online) lives in ALERTS (opens/resolves),
			// not in Events — avoids duplicating the same info on both screens.
			w.logger.Warn("node offline", "node_id", o.id, "hostname", o.hostname)
		}
	}

	// 2) Resolve NODE_OFFLINE for Nodes that returned to ONLINE.
	w.resolveVia(ctx, "NODE_OFFLINE", `SELECT id FROM nodes WHERE status = 'ONLINE' AND deleted_at IS NULL`)

}
