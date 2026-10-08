// Cluster observation and operation handlers: health, Nodes, alerts, events,
// activity feed (ADR-022), audit (ADR-025), metrics (ADR-016) and the internal
// integration webhooks (#116).

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/clusterstats"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/promquery"
)

func handleHealthz(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := d.Pool.Ping(ctx); err != nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "db_unavailable", "database unavailable")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": d.Version,
		})
	}
}

func handleListNodes(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodes, err := d.Nodes.List(r.Context())
		if err != nil {
			d.Logger.Error("list nodes", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list nodes")
			return
		}
		// Global gate (ADR-024) along with the list: the Dashboard shows the switch
		// Full auto/Flex and the "Mode" column in a single call. A failure to read the gate
		// does not break the listing (safe default full_auto).
		gate, err := d.Nodes.Gate(r.Context())
		if err != nil {
			d.Logger.Error("query cluster gate", "error", err)
			gate = node.GateFullAuto
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": nodes, "cluster_operation_mode": gate})
	}
}

func handleDeleteNode(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		force := r.URL.Query().Get("force") == "true"
		err := d.Nodes.Delete(r.Context(), r.PathValue("id"), force)
		switch {
		case errors.Is(err, node.ErrNodeNotFound):
			httpx.WriteError(w, http.StatusNotFound, "node_not_found", "node not found")
		case errors.Is(err, node.ErrNodeHasTorrents):
			httpx.WriteError(w, http.StatusConflict, "node_has_torrents",
				"node still has torrents; migrate/remove them first or use ?force=true")
		case err != nil:
			d.Logger.Error("remover node", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to remove node")
		default:
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
	}
}

func handleListAlerts(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		onlyOpen := r.URL.Query().Get("status") == "open"
		alerts, err := d.Alerts.List(r.Context(), onlyOpen)
		if err != nil {
			d.Logger.Error("list alerts", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list alerts")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": alerts})
	}
}

func handleListEvents(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := queryLimit(r, 100, 500)
		evs, err := d.Events.List(r.Context(), limit)
		if err != nil {
			d.Logger.Error("list events", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list events")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": evs})
	}
}

// handleListActivities (ADR-022): orchestration activity trail (feed).
// ?limit=N and ?torrent_id=UUID (filter by torrent).
func handleListActivities(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := queryLimit(r, 100, 500)
		torrentID := r.URL.Query().Get("torrent_id")
		if d.Activities == nil {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
			return
		}
		acts, err := d.Activities.List(r.Context(), torrentID, limit)
		if err != nil {
			d.Logger.Error("list activities", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list activities")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": acts})
	}
}

// handleListAudit returns the audit trail (ADR-025), most recent first.
// ?actor filters by operator; ?limit limits it (default 100, cap 500).
func handleListAudit(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := queryLimit(r, 100, 500)
		if d.Audit == nil {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": []any{}})
			return
		}
		entries, err := d.Audit.List(r.Context(), r.URL.Query().Get("actor"), limit)
		if err != nil {
			d.Logger.Error("list audit", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list audit")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": entries})
	}
}

func handleSystemHealth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		dbOK := d.Pool.Ping(ctx) == nil
		dbStatus := "ok"
		code := http.StatusOK
		if !dbOK {
			dbStatus = "down"
			code = http.StatusServiceUnavailable
		}
		// Workers: last actual execution of each internal loop (heartbeat, see
		// internal/health). status = running (healthy) | stale (delayed) | starting
		// (registered but still without 1st cycle).
		var workers []map[string]any
		if d.Workers != nil {
			for _, wk := range d.Workers.Snapshot() {
				status := "starting"
				if wk.LastRun != nil {
					if wk.Healthy {
						status = "running"
					} else {
						status = "stale"
					}
				}
				workers = append(workers, map[string]any{
					"name":             wk.Name,
					"status":           status,
					"last_run":         wk.LastRun,
					"interval_seconds": wk.Interval,
				})
			}
		}
		httpx.WriteJSON(w, code, map[string]any{
			"api":            "ok",
			"database":       map[string]string{"status": dbStatus},
			"workers":        workers,
			"version":        d.Version,
			"uptime_seconds": int64(time.Since(d.StartedAt).Seconds()),
		})
	}
}

// queryLimit reads ?limit= with default and CAP (#115): without the cap, ?limit=100000000
// returned the entire table — the audit comment promised a "cap of 500" which never
// existed in the code. max is the contract documented per route.
func queryLimit(r *http.Request, def, max int) int {
	limit := def
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > max {
		limit = max
	}
	return limit
}

// handleClusterThroughput returns the CUMULATIVE throughput series of the cluster (F2):
// cumulative download/upload for the entire cluster over time, with downsampling per
// bucket to fit in long windows (1 year). Response in the format `{data: MetricSeries[]}`
// (two series — "Downloaded"/"Uploaded") — the same that the Portal Chart component consumes.
func handleClusterThroughput(d Deps) http.HandlerFunc {
	type point struct {
		T int64 `json:"t"`
		V int64 `json:"v"`
	}
	type series struct {
		Labels map[string]string `json:"labels"`
		Points []point           `json:"points"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		rng := 7 * 24 * time.Hour // default: 1 week
		if v := r.URL.Query().Get("range"); v != "" {
			if d2, err := time.ParseDuration(v); err == nil && d2 > 0 {
				rng = d2
			}
		}
		pts, err := clusterstats.Throughput(r.Context(), d.Pool, rng)
		if err != nil {
			d.Logger.Error("cluster throughput", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to read cluster throughput")
			return
		}
		// "transfer" carries the meaning (the Portal colors and translates by it); "instance" is
		// only the default legend, for a client that does not know the label.
		dl := series{Labels: map[string]string{"instance": "Downloaded", "transfer": "down"}, Points: make([]point, 0, len(pts))}
		ul := series{Labels: map[string]string{"instance": "Uploaded", "transfer": "up"}, Points: make([]point, 0, len(pts))}
		for _, p := range pts {
			dl.Points = append(dl.Points, point{T: p.T, V: p.Down})
			ul.Points = append(ul.Points, point{T: p.T, V: p.Up})
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": []series{dl, ul}})
	}
}

func handleMetricsQuery(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Prom == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "metrics_unavailable", "metrics query unavailable")
			return
		}
		metric := r.URL.Query().Get("metric")
		if metric == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "metric parameter is required")
			return
		}
		rng := 6 * time.Hour
		if v := r.URL.Query().Get("range"); v != "" {
			if d2, err := time.ParseDuration(v); err == nil && d2 > 0 {
				rng = d2
			}
		}
		series, err := d.Prom.QueryMetric(r.Context(), metric, rng)
		if err == promquery.ErrUnknownMetric {
			httpx.WriteError(w, http.StatusBadRequest, "unknown_metric", "unknown metric")
			return
		}
		if err != nil {
			d.Logger.Error("query metric", "error", err, "metric", metric)
			httpx.WriteError(w, http.StatusBadGateway, "prometheus_error", "failed to query metrics")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"metric": metric, "data": series})
	}
}

// handleNotifyTest sends a test message through the configured channels (ADR-028).
//
// It exists because setting up notifications without feedback is guesswork: the operator only finds out
// that the token is wrong on the first real incident — exactly when the message matters.
// It is SYNCHRONOUS and returns the channel error: "did not arrive" needs to say why.
func handleNotifyTest(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Notifier == nil {
			httpx.WriteError(w, http.StatusConflict, "notify_unavailable", "notifications unavailable")
			return
		}
		user, _ := httpx.OperatorFromContext(r.Context())
		who := "operator"
		if user != nil {
			who = user.Username
		}
		msg := d.Notifier.TestMessage(who)
		// `?channel=telegram|discord|webhook` tests ONE channel (button per channel); without it,
		// it triggers on all configured ones (compatibility).
		ch := r.URL.Query().Get("channel")
		var err error
		if ch != "" {
			if !d.Notifier.ChannelConfigured(ch) {
				httpx.WriteError(w, http.StatusConflict, "notify_channel_not_configured",
					"this channel is not configured — fill in its credentials above")
				return
			}
			err = d.Notifier.SendToChannel(r.Context(), ch, msg)
			httpx.SetAudit(r.Context(), "notify_test", map[string]any{"channel": ch}, "Sent test notification ("+ch+")")
		} else {
			if !d.Notifier.Configured() {
				httpx.WriteError(w, http.StatusConflict, "notify_no_channel",
					"no channel configured — fill in Telegram, Discord or webhook in Settings")
				return
			}
			err = d.Notifier.Send(r.Context(), msg)
			httpx.SetAudit(r.Context(), "notify_test_all", nil, "Sent test notification")
		}
		if err != nil {
			// The error comes formatted (without token) from the notify package.
			httpx.WriteError(w, http.StatusBadGateway, "send_failed", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleAlertmanagerWebhook(d Deps) http.HandlerFunc {
	type amAlert struct {
		Status      string            `json:"status"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	}
	type amPayload struct {
		Alerts []amAlert `json:"alerts"`
	}
	sev := func(s string) string {
		switch strings.ToLower(s) {
		case "critical":
			return "CRITICAL"
		case "warning":
			return "WARNING"
		default:
			return "INFO"
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Tolerant Decode: the Alertmanager payload has many fields besides
		// "alerts"; do not use DisallowUnknownFields (httpx.DecodeJSON) here.
		var p amPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		for _, a := range p.Alerts {
			code := a.Labels["alertname"]
			if code == "" {
				continue
			}
			// Resolves the Node by hostname (instance). Instances that are not Seed
			// Nodes (e.g.: the controller itself) are left without node_id.
			var nodeID string
			if inst := a.Labels["instance"]; inst != "" {
				if err := d.Pool.QueryRow(r.Context(),
					`SELECT id::text FROM nodes WHERE hostname = $1 AND deleted_at IS NULL LIMIT 1`,
					inst).Scan(&nodeID); err != nil {
					nodeID = ""
				}
			}
			if a.Status == "resolved" {
				if _, err := d.Alerts.Resolve(r.Context(), code, nodeID); err != nil {
					d.Logger.Error("resolve alert (alertmanager)", "error", err, "code", code)
				}
				continue
			}
			payload := map[string]any{"instance": a.Labels["instance"], "source": "alertmanager"}
			if _, err := d.Alerts.Open(r.Context(), code, sev(a.Labels["severity"]), nodeID, a.Annotations["summary"], payload); err != nil {
				d.Logger.Error("open alert (alertmanager)", "error", err, "code", code)
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleRecoveryEvent receives the trail of a storage self-recovery done OUTSIDE the
// Node (backlog #20): an operator script on the host — the project doesn't provide one; the
// recovery from inside the Node is seedgrid-storage-reattach — reports each step
// (start/reattach/reboot/verify/success/abort) per attempt. Each step becomes a SeedGrid
// Event (appears in the Portal). "abort" (after exhausting attempts) opens the
// CRITICAL STORAGE_RECOVERY_FAILED alert; "success" resolves it. Protected by the token of
// the internal webhooks (#116), like the Alertmanager one.
func handleRecoveryEvent(d Deps) http.HandlerFunc {
	type req struct {
		Node    string `json:"node"`    // hostname do Seed Node
		Level   string `json:"level"`   // INFO | WARNING | CRITICAL
		Step    string `json:"step"`    // start | reattach | reboot | verify | success | abort
		Attempt int    `json:"attempt"` // attempt number (1..)
		Message string `json:"message"`
	}
	norm := func(s string) string {
		switch strings.ToUpper(s) {
		case "CRITICAL":
			return "CRITICAL"
		case "WARNING":
			return "WARNING"
		default:
			return "INFO"
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if in.Message == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "message is required")
			return
		}
		level := norm(in.Level)
		var nodeID string
		if in.Node != "" {
			if err := d.Pool.QueryRow(r.Context(),
				`SELECT id::text FROM nodes WHERE hostname = $1 AND deleted_at IS NULL LIMIT 1`,
				in.Node).Scan(&nodeID); err != nil {
				nodeID = ""
			}
		}
		payload := map[string]any{"step": in.Step, "attempt": in.Attempt, "source": "storage-recovery", "hostname": in.Node}
		if err := d.Events.Emit(r.Context(), "STORAGE_RECOVERY", level, nodeID, in.Message, payload); err != nil {
			d.Logger.Error("emit recovery event", "error", err)
		}
		switch in.Step {
		case "abort":
			if _, err := d.Alerts.Open(r.Context(), "STORAGE_RECOVERY_FAILED", "CRITICAL", nodeID, in.Message, payload); err != nil {
				d.Logger.Error("open STORAGE_RECOVERY_FAILED alert", "error", err)
			}
		case "success":
			if _, err := d.Alerts.Resolve(r.Context(), "STORAGE_RECOVERY_FAILED", nodeID); err != nil {
				d.Logger.Error("resolve STORAGE_RECOVERY_FAILED alert", "error", err)
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleAnalytics returns the collection report (#160): announce health, duplicate
// titles, stalled collection and occupation per Node.
//
// Just one route, not four: the panels overlap (duplicate that is also stalled,
// stalled that is also dead on the tracker) and four routes would scan the same table
// four times to build ONE screen.
//
// Without `?limit`: the report no longer has an example list to truncate — the only
// consumer was `top_ratio`, which became a ranking on the Dashboard, calculated on the client over
// the full list of `GET /torrents` (#174). The counters were always for the entire
// collection; a truncated summary would lie about the size of the problem.
func handleAnalytics(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rep, err := d.Analytics.Build(r.Context())
		if err != nil {
			d.Logger.Error("build analytics", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to build collection report")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, rep)
	}
}
