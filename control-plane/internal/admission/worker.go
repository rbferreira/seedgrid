// Package admission implements the download admission controller for the Control
// Plane (#21). The problem: the tracker limits simultaneous downloads PER ACCOUNT (DL
// slots), and the passkey is shared across the cluster Nodes. With many incomplete
// torrents announcing at the same time, the tracker refuses peers to almost all and
// everything stalls ("Download slot limit reached"). The controller ensures at most
// L torrents are DOWNLOADING across the ENTIRE cluster: it puts the excess on "hold" (pause) and
// promotes the next one when a slot opens (a download completes and turns to SEEDING).
//
// L is the account's DL slot limit. It is dynamic: it comes from the tracker's own message
// (UNIT3D error 164 "You may at most download N torrents"), which the Agent reads from the Engine
// and reports; while no msg has been observed, it falls back to the config fallback
// (SEEDGRID_MAX_CLUSTER_DOWNLOADS). If there is no known L (no observation and no
// fallback), the controller takes no action — it never risks pausing the cluster blindly.
//
// admission-hold ≠ user-pause: the hold is marked in torrents.admission_paused_at (a
// dedicated column, immune to SyncStates). The controller only promotes/resumes what IT
// paused; a torrent that the operator paused (status PAUSED, admission_paused_at null)
// does not count as an active download.
//
// Who has priority over this user-pause depends on the NODE MODE (ADR-024, clause
// "Node SG → system has priority"):
//   - Node SG (automatic): the system rules. A manually paused download is ADOPTED as
//     hold (adoptOrphanPauses) and goes back to the promotion queue. For a pause to stay
//     in effect, the Node must be set to Manual.
//   - Node Manual: the operator is sovereign — the pause stays until they resume OR until the Node
//     switches back to SG, when the system readopts what was stopped.
//
// Adoption is what implements both ends: since it runs over the Node SG predicate
// every cycle, the switch back from Manual → SG readopts on its own, with no hook on the mode change.
package admission

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rbferreira/seedgrid/control-plane/internal/activity"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// sgNodeJoin/sgNodePred restrict a torrents query (which MUST alias the torrents
// table as `t`) to the Nodes that the admission controller CAN touch — the SG
// (automatic) ones. A Node is Manual (operator sovereign — ADR-024) only when the cluster
// gate is 'flex' AND the Node mode is 'manual'; under 'full_auto' every Node is SG, so the
// predicate becomes always-true (pre-ADR-024 behavior). The predicate uses the
// Node's OWN cluster (not a global cluster), so it holds with any number of
// clusters — useful in integration tests, which create one cluster per case.
//
// Applied only to queries that ACT (hold/promote/rotate/reconcile). countActive DOES NOT use
// the predicate on purpose: a Manual Node's downloads COUNT towards the L ceiling (ADR-024
// invariant — pausing a Manual frees a slot that the system fills on an SG Node).
const (
	sgNodeJoin = " JOIN nodes n ON n.id = t.node_id JOIN clusters c ON c.id = n.cluster_id "
	sgNodePred = " AND (c.operation_mode = 'full_auto' OR n.operation_mode = 'sg') "

	// notMigratingPred excludes torrents with IN-FLIGHT migration (issue #8). Migration
	// pauses the torrent at the source before copying the data (ADR-018, step 3); to the
	// admission controller this pause looks like an "orphan manual pause", which it adopts and promotes in the
	// next cycle — resuming the download IN THE MIDDLE of the transfer, which is the in-flight
	// data the pause exists to prevent. Observed live on 07/08/2026: paused
	// at 06:27:41, admission resumes 15s later, with the pull in progress.
	//
	// While there is an in-flight migration, the migration orchestrator is in charge of that
	// torrent — not admission. Upon completion (COMPLETED/FAILED) the torrent goes back to
	// being a normal candidate, and the one resuming it is the migration's resumeSource().
	notMigratingPred = ` AND NOT EXISTS (
		SELECT 1 FROM torrent_migrations m
		 WHERE m.torrent_id = t.id AND m.status IN ('QUEUED','PENDING')) `
)

// commander queues durable commands for a Node (ADR-014, polling). Satisfied
// by *command.Service.
type commander interface {
	Enqueue(ctx context.Context, nodeID, cmdType string, payload json.RawMessage) (string, error)
}

// slotSource provides the observed L from the tracker message. Satisfied by
// *tracker.Service.
type slotSource interface {
	ObservedDLSlotLimit(ctx context.Context, ttl time.Duration) (int, bool)
}

// Worker periodically evaluates the cluster download ceiling and reconciles holds.
type Worker struct {
	pool     *pgxpool.Pool
	trackers slotSource
	cmds     commander
	logger   *slog.Logger

	// cfg is the dynamic source for tunables (ADR-023): every parameter is read from the Store
	// ON EVERY pass (see accessors below), so dashboard changes take effect without
	// redeploy. The consumed keys: admission.interval, .max_cluster_downloads,
	// .dl_slot_limit_ttl, .min_dl_rate, .slow_grace, .slow_cooldown, .dead_grace,
	// .dead_cooldown.
	cfg *settings.Store

	// Rotation state (ephemeral, in memory — worker restart only zeroes the
	// counts; nothing persisted). The worker runs in a single goroutine → no locks.
	now       func() time.Time     // injectable for tests
	slowSince map[string]time.Time // torrentID → since when it has been below minDLRate
	deadSince map[string]time.Time // torrentID → since when it has had 0 seeds AND 0 peers
	cooldown  map[string]time.Time // torrentID → until when it cannot be promoted
	deadCount map[string]int       // torrentID → consecutive dead rotations (backoff); zeroes on download

	activity *activity.Recorder // activity trail (ADR-022); nil = disabled (best-effort)
}

// Tunable accessors (ADR-023): read from the Store per call → hot-reload. The
// semantics of the values (what each one disables with <=0) are in the catalog and in the
// comments of the methods that consume them.
func (w *Worker) interval() time.Duration     { return w.cfg.Duration("admission.interval") }
func (w *Worker) fallbackLimit() int          { return w.cfg.Int("admission.max_cluster_downloads") }
func (w *Worker) slotTTL() time.Duration      { return w.cfg.Duration("admission.dl_slot_limit_ttl") }
func (w *Worker) minDLRate() int64            { return w.cfg.Int64("admission.min_dl_rate") }
func (w *Worker) slowGrace() time.Duration    { return w.cfg.Duration("admission.slow_grace") }
func (w *Worker) slowCooldown() time.Duration { return w.cfg.Duration("admission.slow_cooldown") }
func (w *Worker) deadGrace() time.Duration    { return w.cfg.Duration("admission.dead_grace") }
func (w *Worker) deadCooldown() time.Duration { return w.cfg.Duration("admission.dead_cooldown") }
func (w *Worker) deadBackoffMax() time.Duration {
	return w.cfg.Duration("admission.dead_backoff_max")
}

// priorityPersistent enables the persistent mode of manual prioritization (F3): a prioritized
// torrent (non-null prioritized_at) becomes IMMUNE to dead/slow rotation — holds the
// slot until downloaded. Disabled (default), priority only jumps the promotion queue and the
// torrent goes back to normal if rotation demotes it.
func (w *Worker) priorityPersistent() bool { return w.cfg.Bool("admission.priority_persistent") }

// deadCooldownFor returns the cooldown for the n-th (1-based) consecutive dead hold of the same
// torrent: deadCooldown doubled on each recurrence, capped by deadBackoffMax (0 =
// backoff disabled → always the base cooldown). Thus a chronic dead torrent — typically one
// whose tracker announces a ghost seeder — distances itself from the queue on its own (30m → 1h → 2h
// → …) instead of reappearing at the top every fixed cooldown, while a one-off unlucky
// torrent (swarm that vanished for 5 min) comes back fast.
func (w *Worker) deadCooldownFor(n int) time.Duration {
	base, max := w.deadCooldown(), w.deadBackoffMax()
	if max <= 0 || base <= 0 {
		return base // backoff disabled: fixed cooldown (pre-refinement behavior)
	}
	d := base
	for i := 1; i < n && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// SetActivity enables the activity trail (ADR-022) — the worker starts logging
// hold/promotion as activity. Optional (injected in main); nil-safe.
func (w *Worker) SetActivity(rec *activity.Recorder) { w.activity = rec }

// NewWorker creates the admission controller. Tunables come from the Store (ADR-023) and
// are read per pass; see the catalog (internal/settings) for defaults/semantics.
func NewWorker(pool *pgxpool.Pool, trackers slotSource, cmds commander, logger *slog.Logger, cfg *settings.Store) *Worker {
	return &Worker{
		pool: pool, trackers: trackers, cmds: cmds, logger: logger, cfg: cfg,
		now:       time.Now,
		slowSince: make(map[string]time.Time),
		deadSince: make(map[string]time.Time),
		cooldown:  make(map[string]time.Time),
		deadCount: make(map[string]int),
	}
}

// Run executes the evaluation loop until the context is canceled.
// beat is optional (variadic to not break callers/tests): heartbeat logged every
// cycle, exposed in the health API (see internal/health).
func (w *Worker) Run(ctx context.Context, beat ...func()) {
	interval := w.interval()
	w.logger.Info("admission controller started",
		"interval", interval.String(), "fallback_limit", w.fallbackLimit(),
		"min_dl_rate_bps", w.minDLRate(), "slow_grace", w.slowGrace().String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	mark := func() {
		for _, b := range beat {
			if b != nil {
				b()
			}
		}
	}

	w.Evaluate(ctx) // first immediate evaluation
	mark()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("admission controller stopped")
			return
		case <-ticker.C:
			w.Evaluate(ctx)
			mark()
			// Hot-reload the interval (ADR-023): if the operator changed it in the panel, the
			// ticker starts ticking at the new rate already in the next cycle.
			if cur := w.interval(); cur != interval {
				interval = cur
				ticker.Reset(cur)
				w.logger.Info("admission interval updated", "interval", cur.String())
			}
		}
	}
}

// Evaluate runs a single pass of the controller. Failures are logged, not propagated
// (controlled degradation: an error does not bring down the worker).
func (w *Worker) Evaluate(ctx context.Context) {
	limit, ok := w.effectiveLimit(ctx)
	if !ok {
		// No known L (no recent observation and no fallback): takes no action — does not
		// risk pausing the entire cluster without knowing the ceiling.
		return
	}

	// 1) Stale holds: a torrent on hold that somehow completed (SEEDING) or
	//    disappeared is no longer a candidate — clears the hold to avoid blocking the promotion queue.
	w.clearStaleHolds(ctx)

	// 1.5) Consumed priority (F3): a prioritized torrent that completed (SEEDING) is
	//      no longer prioritized — clears the mark (and the yellow in the Portal). Independent of hold.
	w.clearCompletedPriority(ctx)

	// 2) Drift: hold whose pause didn't take effect (still DOWNLOADING) → re-enqueues the pause
	//    (convergence; the command is durable but one-shot).
	w.reconcileDrift(ctx)

	// 2.5) Adoption: manually paused downloads on an SG Node become system holds (ADR-024,
	//      "SG Node → system has priority"). It also re-adopts what was left paused
	//      on a Node that switched back from Manual to SG.
	w.adoptOrphanPauses(ctx)

	// 3) Counts the active downloads in the cluster (DOWNLOADING without hold and without user-pause).
	active, err := w.countActive(ctx)
	if err != nil {
		w.logger.Error("count active downloads", "error", err)
		return
	}

	switch {
	case active > limit:
		w.hold(ctx, active-limit)
	case active < limit:
		w.promote(ctx, limit-active)
	}

	// 4) Rotation by DEAD torrent (0 seeds AND 0 peers): primary trigger — a torrent
	//    with no one in the swarm never completes, so it yields the slot immediately (before the rate
	//    rule) and goes to the end of the queue. Runs before rate-based rotation.
	w.rotateDead(ctx)
	// 5) Rate-based rotation (#22): at the ceiling, swaps "alive-but-slow" downloads (have peers,
	//    but below minDLRate) for others in the queue that can progress. Runs after the
	//    ceiling adjustment (only acts when there is no natural vacancy — otherwise step 3 already promoted).
	w.rotateSlow(ctx)
}

// effectiveLimit resolves L: prefers the one observed from the tracker message (recent);
// otherwise the config fallback. (0,false) when there is none → the controller takes no action.
func (w *Worker) effectiveLimit(ctx context.Context) (int, bool) {
	if l, ok := w.trackers.ObservedDLSlotLimit(ctx, w.slotTTL()); ok && l > 0 {
		return l, true
	}
	if fb := w.fallbackLimit(); fb > 0 {
		return fb, true
	}
	return 0, false
}

// countActive counts the "un-paused" torrents in the cluster: incomplete ones that qBt is
// running OR trying to run — status DOWNLOADING (announces as leecher) or QUEUED
// (queuedDL: qBt promotes to DOWNLOADING as soon as a local slot opens). It is this POOL that
// must stay ≤ L: counting only DOWNLOADING would not be enough, because pausing a DOWNLOADING
// torrent causes the local qBt to replace it by promoting a queuedDL (they would fight). CHECKING is transient
// (does not announce); SEEDING is a seeder; PAUSED (user or hold) does not count.
//
// Counts ALL Nodes (SG and Manual) — purposely without sgNodePred: the active ones on a
// Manual Node take up a slot from the L ceiling (ADR-024 invariant). Thus, pausing a download
// on a Manual Node frees a slot that the controller fills by promoting a hold on an SG Node.
func (w *Worker) countActive(ctx context.Context) (int, error) {
	var n int
	err := w.pool.QueryRow(ctx,
		`SELECT count(*) FROM torrents
		  WHERE deleted_at IS NULL AND admission_paused_at IS NULL
		    AND status IN ('DOWNLOADING', 'QUEUED')`).Scan(&n)
	return n, err
}

// hold puts the n un-paused torrents with the LOWEST progress on hold (keeps the most
// advanced ones downloading — the inverse of promotion): enqueues pause_torrent and marks the hold. Acts
// on DOWNLOADING and QUEUED — stopping only DOWNLOADING would let qBt replace them with
// queuedDLs, without reducing the pool. PRIORITIZED torrents (F3) are held LAST — they only
// fall into the excess hold if there aren't enough non-prioritized ones to yield the slot.
func (w *Worker) hold(ctx context.Context, n int) {
	rows, err := w.pool.Query(ctx,
		`SELECT t.id::text, t.node_id::text, COALESCE(t.hash, ''), COALESCE(t.name, '')
		   FROM torrents t`+sgNodeJoin+`
		  WHERE t.deleted_at IS NULL AND t.admission_paused_at IS NULL
		    AND t.status IN ('DOWNLOADING', 'QUEUED')`+sgNodePred+notMigratingPred+`
		  ORDER BY (t.prioritized_at IS NOT NULL) ASC, t.progress ASC NULLS FIRST, t.created_at ASC
		  LIMIT $1`, n)
	if err != nil {
		w.logger.Error("select excess for hold", "error", err)
		return
	}
	for _, t := range scanTargets(rows) {
		w.holdOne(ctx, t, "surplus")
	}
}

// promote resumes the n held torrents with the HIGHEST progress (finishes the closest
// ones first) that are NOT in rotation cooldown (#22): enqueues resume_torrent and
// clears the hold. Looks for a margin beyond n to be able to skip those in cooldown.
func (w *Worker) promote(ctx context.Context, n int) {
	for _, t := range w.promotableHolds(ctx, n) {
		w.promoteOne(ctx, t)
	}
}

// promotableHolds returns up to n promotable holds, with PEER-AWARE eligibility
// (ADR-021 evolution): sorts by PRIORITIZED first (F3 — manual operator override
// cuts the line), then VIABLE (has seeds OR peers → can download), then by
// HIGHEST PROGRESS (finishes the closest ones first). Those in cooldown
// ("end of the line" — rotated by dead/slow) only come in as FALLBACK, when
// there aren't enough ready ones and an idle slot would be left — it's the
// "or until the queue rotates": all others had their turn before the parked
// one returns. A prioritized torrent IGNORES the cooldown: always "ready" and top.
func (w *Worker) promotableHolds(ctx context.Context, n int) []target {
	if n <= 0 {
		return nil
	}
	rows, err := w.pool.Query(ctx,
		`SELECT t.id::text, t.node_id::text, COALESCE(t.hash, ''), COALESCE(t.name, ''),
		        (t.prioritized_at IS NOT NULL)
		   FROM torrents t`+sgNodeJoin+`
		  WHERE t.deleted_at IS NULL AND t.admission_paused_at IS NOT NULL
		    AND t.status <> 'SEEDING' AND (t.progress IS NULL OR t.progress < 1)`+sgNodePred+notMigratingPred+`
		  ORDER BY (t.prioritized_at IS NOT NULL) DESC,
		           (COALESCE(t.num_seeds, 0) > 0 OR COALESCE(t.num_leechers, 0) > 0) DESC,
		           t.progress DESC NULLS LAST, t.created_at ASC`)
	if err != nil {
		w.logger.Error("select hold to promote", "error", err)
		return nil
	}
	defer rows.Close()
	now := w.now()
	var ready, parked []target // ready = prioritized + out of cooldown; parked = in cooldown
	for rows.Next() {
		var t target
		var prioritized bool
		if err := rows.Scan(&t.id, &t.nodeID, &t.hash, &t.name, &prioritized); err != nil {
			break
		}
		if !prioritized { // prioritized bypasses the cooldown; the others respect the "end of the queue"
			if until, ok := w.cooldown[t.id]; ok && now.Before(until) {
				parked = append(parked, t)
				continue
			}
		}
		ready = append(ready, t)
	}
	out := ready
	if len(out) < n {
		out = append(out, parked...) // queue exhausted → frees the parked ones to avoid leaving an idle slot
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// holdOne enqueues the pause of a torrent and marks the admission-hold. reason just labels the log.
func (w *Worker) holdOne(ctx context.Context, t target, reason string) bool {
	if t.hash == "" {
		return false
	}
	payload, _ := json.Marshal(map[string]string{"hash": t.hash})
	if _, err := w.cmds.Enqueue(ctx, t.nodeID, "pause_torrent", payload); err != nil {
		w.logger.Error("enqueue admission pause", "error", err, "torrent_id", t.id)
		return false
	}
	if _, err := w.pool.Exec(ctx,
		`UPDATE torrents SET admission_paused_at = now(), updated_at = now() WHERE id = $1`, t.id); err != nil {
		w.logger.Error("mark admission-hold", "error", err, "torrent_id", t.id)
		return false
	}
	w.logger.Info("download on hold by admission", "torrent_id", t.id, "node_id", t.nodeID, "name", label(t.name, t.hash), "reason", reason)
	if w.activity != nil {
		msg, code := "Download on hold — admission cap reached", "cap"
		switch reason {
		case "slow rate":
			msg, code = "Download on hold — slow rate, yielded slot", "slow"
		case "no peers":
			msg, code = "Download on hold — no seeds/peers (dead torrent), moved to back of queue", "dead"
		}
		_ = w.activity.Record(ctx, activity.Entry{
			Kind: activity.KindAdmissionHold, TorrentID: t.id, TorrentName: label(t.name, t.hash),
			NodeID: t.nodeID, Message: msg, Detail: map[string]any{"reason": reason, "reason_code": code},
		})
	}
	return true
}

// promoteOne enqueues the resume of a held torrent and clears the admission-hold.
func (w *Worker) promoteOne(ctx context.Context, t target) bool {
	if t.hash == "" {
		return false
	}
	payload, _ := json.Marshal(map[string]string{"hash": t.hash})
	if _, err := w.cmds.Enqueue(ctx, t.nodeID, "resume_torrent", payload); err != nil {
		w.logger.Error("enqueue admission resume", "error", err, "torrent_id", t.id)
		return false
	}
	if _, err := w.pool.Exec(ctx,
		`UPDATE torrents SET admission_paused_at = NULL, updated_at = now() WHERE id = $1`, t.id); err != nil {
		w.logger.Error("clear admission-hold", "error", err, "torrent_id", t.id)
		return false
	}
	w.logger.Info("download promoted by admission", "torrent_id", t.id, "node_id", t.nodeID, "name", label(t.name, t.hash))
	if w.activity != nil {
		_ = w.activity.Record(ctx, activity.Entry{
			Kind: activity.KindAdmissionResume, TorrentID: t.id, TorrentName: label(t.name, t.hash),
			NodeID: t.nodeID, Message: "Download promoted — slot released",
		})
	}
	return true
}

// rotateDead (ADR-021 evolution) removes DEAD downloads from the slot: 0 seeds AND 0
// peers for continuous deadGrace — with no one to download from, the torrent never completes,
// so it yields the slot immediately (primary trigger, before rate-based rotation). The dead one goes
// to the END OF THE QUEUE: long cooldown (deadCooldown) so it doesn't return even if it has the highest
// progress, giving way to the others (peer-aware). Only acts if there is a viable hold to
// promote (otherwise keeping it is better than an idle slot). Does not judge QUEUED or newly started
// ones (deadGrace, counted from the 1st observation without peers, covers the initial connection to the swarm).
//
// "No one" is measured by CONNECTED peers, not the swarm: the tracker scrape may
// announce a GHOST seeder (registered, but never responds), and judging by it lets
// the worst case slip through — a torrent with num_seeds=1 and 0 connections for days, which is never marked as
// dead, returns to the top of the queue every cooldown (it's "viable" and has high progress) and burns
// a slot from the L ceiling forever. COALESCE falls back to the swarm when the Agent is older than the field
// (NULL) — backwards compatible, no deploy window.
//
// Recurrence has BACKOFF: each consecutive dead rotation doubles the cooldown (deadBackoffMax
// caps it). A chronic dead torrent distances itself from the queue instead of returning every fixed deadCooldown;
// a torrent that starts downloading again resets the count (see Evaluate).
func (w *Worker) rotateDead(ctx context.Context) {
	now := w.now()
	// Purges expired cooldowns here too: rotateSlow only purges if enabled, but
	// dead holds use cooldown even with rate-based rotation disabled.
	for id, until := range w.cooldown {
		if now.After(until) {
			delete(w.cooldown, id)
		}
	}
	deadGrace := w.deadGrace()
	if deadGrace <= 0 {
		return
	}
	rows, err := w.pool.Query(ctx,
		`SELECT t.id::text, t.node_id::text, COALESCE(t.hash, ''), COALESCE(t.name, ''),
		        COALESCE(t.num_seeds_connected, t.num_seeds, 0),
		        COALESCE(t.num_leechers_connected, t.num_leechers, 0), COALESCE(t.progress, 0),
		        (t.prioritized_at IS NOT NULL)
		   FROM torrents t`+sgNodeJoin+`
		  WHERE t.deleted_at IS NULL AND t.admission_paused_at IS NULL AND t.status = 'DOWNLOADING'`+sgNodePred+notMigratingPred)
	if err != nil {
		w.logger.Error("select active for dead rotation", "error", err)
		return
	}
	type act struct {
		t           target
		seeds       int
		peers       int
		prog        float64
		prioritized bool
	}
	var actives []act
	live := make(map[string]bool)
	for rows.Next() {
		var a act
		if err := rows.Scan(&a.t.id, &a.t.nodeID, &a.t.hash, &a.t.name, &a.seeds, &a.peers, &a.prog, &a.prioritized); err != nil {
			break
		}
		actives = append(actives, a)
		live[a.t.id] = true
	}
	rows.Close()

	// Updates the "since when it has no peers" mark and collects the dead ones (>= grace).
	persistent := w.priorityPersistent()
	var dead []act
	for _, a := range actives {
		if persistent && a.prioritized {
			// Persistent mode (F3): prioritized torrent is never rotated out as dead —
			// holds the slot until downloaded. Resets the clock so it isn't "almost dead" if
			// the operator disables persistent mode or removes the priority later.
			delete(w.deadSince, a.t.id)
			continue
		}
		if a.seeds == 0 && a.peers == 0 {
			if _, ok := w.deadSince[a.t.id]; !ok {
				w.deadSince[a.t.id] = now
			}
			if now.Sub(w.deadSince[a.t.id]) >= deadGrace {
				dead = append(dead, a)
			}
		} else {
			// Connected to someone: resets the clock AND the backoff — getting peers again is
			// proof that it's not chronically dead, so next time it starts from the
			// base cooldown.
			delete(w.deadSince, a.t.id)
			delete(w.deadCount, a.t.id)
		}
	}
	for id := range w.deadSince {
		if !live[id] {
			delete(w.deadSince, id)
		}
	}
	if len(dead) == 0 {
		return
	}
	fresh := w.promotableHolds(ctx, len(dead))
	if len(fresh) == 0 {
		return // no viable candidates to promote → keeping it is better than an idle slot
	}
	n := len(dead)
	if len(fresh) < n {
		n = len(fresh)
	}
	for i := 0; i < n; i++ {
		if !w.holdOne(ctx, dead[i].t, "no peers") {
			continue
		}
		// NON-persistent mode: demoting a prioritized one makes it "go back to being a normal
		// torrent" (F3) — it loses priority. (In persistent mode it never reaches here.)
		if dead[i].prioritized {
			w.clearPriority(ctx, dead[i].t.id)
		}
		w.deadCount[dead[i].t.id]++
		// End of the queue (long), doubling on each recurrence: a chronically dead one is pushed back.
		w.cooldown[dead[i].t.id] = now.Add(w.deadCooldownFor(w.deadCount[dead[i].t.id]))
		delete(w.deadSince, dead[i].t.id)
		w.promoteOne(ctx, fresh[i])
	}
}

// rotateSlow (#22) swaps "alive-but-stalled" downloads for others in the queue. An active
// (DOWNLOADING without hold) whose dl_speed drops below minDLRate for a continuous slowGrace
// is rotated: it gets a hold and a promotable hold (out of cooldown, higher progress)
// takes its place — 1:1 swap, the L ceiling is preserved. The rotated one goes into cooldown
// (slowCooldown) out of promotion, to avoid returning immediately and oscillating (thrashing). It only
// acts when there is someone to promote (otherwise keeping the slow one is better than an idle slot).
// It doesn't judge QUEUED (queuedDL doesn't download by design) nor newly started ones (the slowGrace,
// counted from the 1st slow observation, covers the initial connection / piece exchange).
func (w *Worker) rotateSlow(ctx context.Context) {
	minDLRate, slowGrace := w.minDLRate(), w.slowGrace()
	if minDLRate <= 0 || slowGrace <= 0 {
		return
	}
	now := w.now()
	rows, err := w.pool.Query(ctx,
		`SELECT t.id::text, t.node_id::text, COALESCE(t.hash, ''), COALESCE(t.name, ''),
		        COALESCE(t.dl_speed, 0), COALESCE(t.progress, 0),
		        (t.prioritized_at IS NOT NULL)
		   FROM torrents t`+sgNodeJoin+`
		  WHERE t.deleted_at IS NULL AND t.admission_paused_at IS NULL AND t.status = 'DOWNLOADING'`+sgNodePred+notMigratingPred)
	if err != nil {
		w.logger.Error("select active for rate rotation", "error", err)
		return
	}
	type act struct {
		t           target
		dl          int64
		prog        float64
		prioritized bool
	}
	var actives []act
	live := make(map[string]bool)
	for rows.Next() {
		var a act
		if err := rows.Scan(&a.t.id, &a.t.nodeID, &a.t.hash, &a.t.name, &a.dl, &a.prog, &a.prioritized); err != nil {
			break
		}
		actives = append(actives, a)
		live[a.t.id] = true
	}
	rows.Close()

	// Updates the "since when it's slow" mark and collects the dead ones (slow >= grace).
	persistent := w.priorityPersistent()
	var dead []act
	for _, a := range actives {
		if persistent && a.prioritized {
			delete(w.slowSince, a.t.id) // persistent (F3): prioritized is not rotated out for slowness
			continue
		}
		if a.dl < minDLRate {
			if _, ok := w.slowSince[a.t.id]; !ok {
				w.slowSince[a.t.id] = now
			}
			if now.Sub(w.slowSince[a.t.id]) >= slowGrace {
				dead = append(dead, a)
			}
		} else {
			delete(w.slowSince, a.t.id) // started moving again: resets the clock
		}
	}
	// In-memory state hygiene: discards those no longer active and expired cooldowns.
	for id := range w.slowSince {
		if !live[id] {
			delete(w.slowSince, id)
		}
	}
	for id, until := range w.cooldown {
		if now.After(until) {
			delete(w.cooldown, id)
		}
	}
	if len(dead) == 0 {
		return
	}
	fresh := w.promotableHolds(ctx, len(dead))
	if len(fresh) == 0 {
		return // no one to promote → keeping the slow ones is better than idleness
	}
	// Rotates the dead with LOWEST progress first (loses less work done).
	sort.Slice(dead, func(i, j int) bool { return dead[i].prog < dead[j].prog })
	n := len(dead)
	if len(fresh) < n {
		n = len(fresh)
	}
	for i := 0; i < n; i++ {
		if !w.holdOne(ctx, dead[i].t, "slow rate") {
			continue
		}
		// Non-persistent: demoted for slowness loses priority (F3).
		if dead[i].prioritized {
			w.clearPriority(ctx, dead[i].t.id)
		}
		w.cooldown[dead[i].t.id] = now.Add(w.slowCooldown())
		delete(w.slowSince, dead[i].t.id)
		w.promoteOne(ctx, fresh[i])
	}
}

// reconcileDrift re-queues the pause for holds that started running again (status
// DOWNLOADING or QUEUED: the previous pause didn't take or qBt repromoted it) — ensures
// convergence to the desired state (hold ⇒ stopped). Restricted to SG Nodes (sgNodePred):
// on a Manual Node (ADR-024) the operator's resume is NOT undone — that's exactly the fix
// for the asymmetry "manual pause wins, but manual resume was undone by drift".
func (w *Worker) reconcileDrift(ctx context.Context) {
	rows, err := w.pool.Query(ctx,
		`SELECT t.id::text, t.node_id::text, COALESCE(t.hash, ''), COALESCE(t.name, '')
		   FROM torrents t`+sgNodeJoin+`
		  WHERE t.deleted_at IS NULL AND t.admission_paused_at IS NOT NULL
		    AND t.status IN ('DOWNLOADING', 'QUEUED')`+sgNodePred)
	if err != nil {
		w.logger.Error("detect hold drift", "error", err)
		return
	}
	for _, t := range scanTargets(rows) {
		if t.hash == "" {
			continue
		}
		payload, _ := json.Marshal(map[string]string{"hash": t.hash})
		if _, err := w.cmds.Enqueue(ctx, t.nodeID, "pause_torrent", payload); err != nil {
			w.logger.Error("requeue hold pause on drift", "error", err, "torrent_id", t.id)
		}
	}
}

// adoptOrphanPauses adopts INCOMPLETE downloads that are PAUSED as admission-hold
// on an SG Node without `admission_paused_at` — typically because the operator paused them via the
// Portal, or because the Node was in Manual and returned to SG.
//
// Why it exists: without this, a manual pause is a ONE-WAY exit from orchestration. The
// torrent doesn't count in countActive (it's not DOWNLOADING/QUEUED) and doesn't show up in
// promotableHolds (which requires the hold to be marked), so it ends up in limbo where it neither runs nor
// is a candidate — the cluster operates below L with the queue seeming stuck. ADR-024 gives
// priority to the system on an SG Node, so here it reclaims ownership.
//
// Adopting ≠ promoting: it only marks the hold. The promotion step decides if the torrent resumes,
// respecting the L ceiling and peer-aware order. It also does NOT queue a pause — the
// torrent is already stopped in the Engine; queuing would be a redundant command to the Node.
//
// Completed torrents (progress >= 1) are intentionally excluded: PAUSED at 100% is a
// SEEDER that the operator stopped, not a queued download — adopting it would make the controller
// send a resume for something it never held.
func (w *Worker) adoptOrphanPauses(ctx context.Context) {
	rows, err := w.pool.Query(ctx,
		`SELECT t.id::text, t.node_id::text, COALESCE(t.hash, ''), COALESCE(t.name, '')
		   FROM torrents t`+sgNodeJoin+`
		  WHERE t.deleted_at IS NULL AND t.admission_paused_at IS NULL
		    AND t.status = 'PAUSED' AND (t.progress IS NULL OR t.progress < 1)`+sgNodePred+notMigratingPred)
	if err != nil {
		w.logger.Error("select orphaned pauses for adoption", "error", err)
		return
	}
	for _, t := range scanTargets(rows) {
		if _, err := w.pool.Exec(ctx,
			`UPDATE torrents SET admission_paused_at = now(), updated_at = now()
			  WHERE id = $1 AND admission_paused_at IS NULL`, t.id); err != nil {
			w.logger.Error("adopt orphaned pause", "error", err, "torrent_id", t.id)
			continue
		}
		w.logger.Info("manual pause adopted as hold (Node SG)", "torrent_id", t.id,
			"node_id", t.nodeID, "name", label(t.name, t.hash))
		if w.activity != nil {
			_ = w.activity.Record(ctx, activity.Entry{
				Kind: activity.KindAdmissionHold, TorrentID: t.id, TorrentName: label(t.name, t.hash),
				NodeID: t.nodeID, Message: "Manual pause adopted — Node in SG mode, returned to queue",
				Detail: map[string]any{"reason": "manual pause adoption (Node SG)", "reason_code": "manual_pause_adopted"},
			})
		}
	}
}

// clearStaleHolds clears the hold of torrents that shouldn't be in the promotion
// queue anymore (they completed/turned into SEEDING while marked).
func (w *Worker) clearStaleHolds(ctx context.Context) {
	if _, err := w.pool.Exec(ctx,
		`UPDATE torrents SET admission_paused_at = NULL, updated_at = now()
		  WHERE admission_paused_at IS NOT NULL AND deleted_at IS NULL
		    AND (status = 'SEEDING' OR progress >= 1)`); err != nil {
		w.logger.Error("clear obsolete holds", "error", err)
	}
}

// clearPriority removes the manual prioritization mark (F3) from a torrent — used when
// dead/slow rotation demotes it in NON-persistent mode ("goes back to normal").
// Best-effort: failure only logs, it doesn't interrupt the rotation.
func (w *Worker) clearPriority(ctx context.Context, id string) {
	if _, err := w.pool.Exec(ctx,
		`UPDATE torrents SET prioritized_at = NULL, updated_at = now()
		  WHERE id = $1 AND prioritized_at IS NOT NULL`, id); err != nil {
		w.logger.Error("clear priority on demotion", "error", err, "torrent_id", id)
	}
}

// clearCompletedPriority clears the priority (F3) of torrents that have already completed: once
// SEEDING/100%, prioritizing no longer makes sense (there's no queue to skip). Runs every cycle.
func (w *Worker) clearCompletedPriority(ctx context.Context) {
	if _, err := w.pool.Exec(ctx,
		`UPDATE torrents SET prioritized_at = NULL, updated_at = now()
		  WHERE prioritized_at IS NOT NULL AND deleted_at IS NULL
		    AND (status = 'SEEDING' OR progress >= 1)`); err != nil {
		w.logger.Error("clear priority of completed torrents", "error", err)
	}
}

type target struct{ id, nodeID, hash, name string }

// scanTargets reads the rows (id, node_id, hash, name) and closes the cursor. Scan
// errors interrupt the read (what has already been read is returned).
func scanTargets(rows interface {
	Next() bool
	Scan(...any) error
	Close()
}) []target {
	defer rows.Close()
	var out []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.nodeID, &t.hash, &t.name); err != nil {
			break
		}
		out = append(out, t)
	}
	return out
}

// label returns a human-readable label for the torrent (name, or short hash).
func label(name, hash string) string {
	if name != "" {
		return name
	}
	if len(hash) >= 8 {
		return hash[:8]
	}
	return hash
}
