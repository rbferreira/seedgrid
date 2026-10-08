// Package api wires the Control Plane HTTP routing and the API v1 handlers.
// Every platform feature exists first in the API (Vision §5.5).
//
// This file holds the dependencies (Deps) and the routing (NewRouter). Handlers
// live in handlers_*.go per domain, in the SAME package (#118) — the router is the
// index of everything the API exposes; each route's implementation is in its domain file.

package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/activity"
	"github.com/rbferreira/seedgrid/control-plane/internal/agentconfig"
	"github.com/rbferreira/seedgrid/control-plane/internal/agentrelease"
	"github.com/rbferreira/seedgrid/control-plane/internal/alert"
	"github.com/rbferreira/seedgrid/control-plane/internal/analytics"
	"github.com/rbferreira/seedgrid/control-plane/internal/audit"
	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/enrich"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/health"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/igdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/migration"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/notify"
	"github.com/rbferreira/seedgrid/control-plane/internal/promquery"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
	"github.com/rbferreira/seedgrid/control-plane/internal/tmdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
	"github.com/rbferreira/seedgrid/control-plane/internal/users"
)

// Deps groups the handler dependencies.
type Deps struct {
	Logger     *slog.Logger
	Pool       *pgxpool.Pool
	Auth       *auth.Service
	Users      *users.Service
	Nodes      *node.Service
	Commands   *command.Service
	Torrents   *torrent.Service
	Trackers   *tracker.Service
	Profiles   *trackerprofile.Service // site profile (taxonomy, ADR-037): layer 2, separate from Tracker credentials
	Migrations *migration.Service
	Alerts     *alert.Service
	Events     *events.Emitter
	Activities *activity.Recorder
	Audit      *audit.Recorder
	Settings   *settings.Store
	Metrics    http.Handler
	Prom       *promquery.Client
	TMDB       *tmdb.Client
	IGDB       *igdb.Client
	Enrich     *enrich.Service // metadata enrichment in browse (#12): poster, PT title, genres
	Workers    *health.Registry
	// Analytics answers the catalog questions from the Analytics dashboard (#160).
	Analytics *analytics.Service
	// AgentRelease knows the published Agent version and the checksum per architecture (ADR-027).
	AgentRelease *agentrelease.Service
	// AgentConfig resolves the Agent/Engine behavior configuration per Node —
	// cluster value + per-Node override (ADR-039). nil = the heartbeat delivers no config
	// and the per-Node configuration routes respond 503.
	AgentConfig *agentconfig.Service
	// Notifier sends telemetry outward (ADR-028); nil = no channels.
	Notifier  *notify.Service
	Version   string
	StartedAt time.Time
	// IntegrationToken authenticates the internal webhooks (/integrations/*, #116);
	// empty = open (compat, with a warning at router boot).
	IntegrationToken string
	// The ptpimg key (screenshots, ADR-026) no longer lives here: it moved to the catalog
	// (ADR-029) and the handler reads it from Settings per call.
}

// NewRouter builds the root HTTP handler with routes and middlewares.
func NewRouter(d Deps) http.Handler {
	mux := http.NewServeMux()

	// Health (no authentication).
	mux.HandleFunc("GET /healthz", handleHealthz(d))

	// Prometheus metrics (internal scrape; no authentication).
	if d.Metrics != nil {
		mux.Handle("GET /metrics", d.Metrics)
	}

	// Internal webhooks (#116): Alertmanager (metric Alerts → SeedGrid Alerts)
	// and the storage auto-recovery trail (HOST udev script → Event/Alert).
	// They used to be open ("trust the private network") — any LAN host could open
	// and RESOLVE Alerts with a curl. Now they require the shared secret (dedicated header
	// or Bearer); without a configured token they remain open (compat) with a log warning.
	integrationMW := httpx.RequireIntegrationToken(d.IntegrationToken, d.Logger)
	mux.Handle("POST /api/v1/integrations/alertmanager",
		integrationMW(http.HandlerFunc(handleAlertmanagerWebhook(d))))
	mux.Handle("POST /api/v1/integrations/recovery",
		integrationMW(http.HandlerFunc(handleRecoveryEvent(d))))

	// Operator authentication.
	mux.HandleFunc("POST /api/v1/auth/login", handleLogin(d))

	// Content download via the Portal (#1): public GET authenticated by the one-time
	// TICKET in the query (the browser does not send the Bearer on navigation/anchor). The CP
	// proxies the Node stream (Node never exposed). The ticket is issued by a protected route.
	mux.HandleFunc("GET /api/v1/torrents/download", handleDownload(d))

	// Agents API (ADR-012: communication initiated by the Agent).
	// register does auth in the handler itself (accepts the shared token OR a
	// one-time join token — ADR-017). enroll is public (the join token in the body authenticates).
	mux.HandleFunc("POST /api/v1/agent/register", handleAgentRegister(d))
	mux.HandleFunc("POST /api/v1/agent/enroll", handleAgentEnroll(d))
	mux.Handle("POST /api/v1/agent/heartbeat",
		httpx.RequireAgent(d.Nodes)(http.HandlerFunc(handleAgentHeartbeat(d))))
	mux.Handle("GET /api/v1/agent/commands",
		httpx.RequireAgent(d.Nodes)(http.HandlerFunc(handleAgentCommands(d))))
	mux.Handle("POST /api/v1/agent/torrents",
		httpx.RequireAgent(d.Nodes)(http.HandlerFunc(handleAgentTorrents(d))))
	mux.Handle("POST /api/v1/agent/commands/{id}/result",
		httpx.RequireAgent(d.Nodes)(http.HandlerFunc(handleAgentCommandResult(d))))
	// .torrent backfill (ADR-018): the Agent asks what is missing and sends it back.
	mux.Handle("GET /api/v1/agent/torrent-files/pending",
		httpx.RequireAgent(d.Nodes)(http.HandlerFunc(handleAgentPendingTorrentFiles(d))))
	mux.Handle("POST /api/v1/agent/torrent-files",
		httpx.RequireAgent(d.Nodes)(http.HandlerFunc(handleAgentUploadTorrentFile(d))))

	// Protected routes (require an operator session; ADR-035 requires permission).
	// protected: requires an authenticated operator AND audits the request (ADR-025).
	// RequireOperator sits OUTSIDE (injects the operator into the context and blocks unauth);
	// AuditMutations inside records the action (mutations only; GET is ignored).
	auditMW := httpx.AuditMutations(d.Audit)
	protected := func(h http.Handler) http.Handler {
		return httpx.RequireOperator(d.Auth)(auditMW(h))
	}
	// permitted: protected + RBAC authorization (ADR-035). RequirePermission runs
	// INSIDE AuditMutations — so the 403 for denied permission also lands
	// in the Audit trail (ADR-025; review: without this order, denial was silent).
	permitted := func(perm string, h http.Handler) http.Handler {
		return httpx.RequireOperator(d.Auth)(auditMW(httpx.RequirePermission(perm)(h)))
	}
	// Change own password: any authenticated operator (ADR-035).
	mux.Handle("POST /api/v1/auth/password", protected(http.HandlerFunc(handleChangePassword(d))))
	// Authenticated operator profile (roles/permissions for the UI to hide actions).
	mux.Handle("GET /api/v1/auth/me", protected(http.HandlerFunc(handleMe(d))))
	// User management (admin only, ADR-035).
	mux.Handle("GET /api/v1/users", permitted("users:read", http.HandlerFunc(handleListUsers(d))))
	mux.Handle("POST /api/v1/users", permitted("users:write", http.HandlerFunc(handleCreateUser(d))))
	mux.Handle("GET /api/v1/users/{id}", permitted("users:read", http.HandlerFunc(handleGetUser(d))))
	mux.Handle("PUT /api/v1/users/{id}", permitted("users:write", http.HandlerFunc(handleUpdateUser(d))))
	mux.Handle("DELETE /api/v1/users/{id}", permitted("users:write", http.HandlerFunc(handleDeleteUser(d))))
	mux.Handle("POST /api/v1/users/{id}/reset-password", permitted("users:write", http.HandlerFunc(handleResetUserPassword(d))))
	mux.Handle("POST /api/v1/enroll-tokens", permitted("enroll:write", http.HandlerFunc(handleMintEnrollToken(d))))
	mux.Handle("GET /api/v1/nodes", permitted("nodes:read", http.HandlerFunc(handleListNodes(d))))
	mux.Handle("DELETE /api/v1/nodes/{id}", permitted("nodes:write", http.HandlerFunc(handleDeleteNode(d))))
	// Node nickname (#145): a name that lives only in SeedGrid; does not touch the Node.
	mux.Handle("PUT /api/v1/nodes/{id}/display-name", permitted("nodes:write", http.HandlerFunc(handleRenameNode(d))))
	mux.Handle("PUT /api/v1/cluster/operation-mode", permitted("nodes:operation-mode", http.HandlerFunc(handleSetClusterMode(d))))
	mux.Handle("PUT /api/v1/nodes/{id}/operation-mode", permitted("nodes:operation-mode", http.HandlerFunc(handleSetNodeMode(d))))
	mux.Handle("POST /api/v1/nodes/{id}/commands", permitted("nodes:command", http.HandlerFunc(handleEnqueueCommand(d))))
	mux.Handle("POST /api/v1/nodes/{id}/engine/restart", permitted("nodes:write", http.HandlerFunc(handleRestartEngine(d))))
	// Agent self-update (ADR-027): published version + manual trigger per Node.
	mux.Handle("GET /api/v1/agent/available", permitted("nodes:read", http.HandlerFunc(handleAgentAvailable(d))))
	mux.Handle("POST /api/v1/nodes/{id}/update-agent", permitted("nodes:write", http.HandlerFunc(handleUpdateAgent(d))))
	// Telemetry (ADR-028): sends a test message through the configured channels.
	mux.Handle("POST /api/v1/notify/test", permitted("notify:write", http.HandlerFunc(handleNotifyTest(d))))
	mux.Handle("GET /api/v1/commands/{id}", permitted("commands:read", http.HandlerFunc(handleGetCommand(d))))
	mux.Handle("POST /api/v1/torrents", permitted("torrents:create", http.HandlerFunc(handleAddTorrent(d))))
	// Creating a .torrent from local content is a PUBLICATION action (ADR-019):
	// own permission `torrents:build` (ADR-035 review, bug #6 — a downloader
	// adds a torrent for its own download, does not build content for publishing).
	mux.Handle("POST /api/v1/torrents/create", permitted("torrents:build", http.HandlerFunc(handleCreateTorrent(d))))
	// External registration on a UNIT3D tracker (ADR-020): posts the already-created torrent to the site.
	mux.Handle("POST /api/v1/torrents/{id}/publish-tracker", permitted("torrents:publish", http.HandlerFunc(handlePublishToTracker(d))))
	// Frozen preview of the payload (#188) — UNIT3D has no dry-run.
	mux.Handle("POST /api/v1/torrents/{id}/publish-tracker/preview", permitted("torrents:publish", http.HandlerFunc(handlePublishPreview(d))))
	// Automatic screenshots for the upload description (ADR-026): extracts via Agent + hosts on imgbb (ADR-030).
	mux.Handle("POST /api/v1/torrents/{id}/screenshots", permitted("torrents:publish", http.HandlerFunc(handleGenerateScreenshots(d))))
	// Step-by-step flow (F1): extract (returns command_id for the Portal to track the step on
	// the Node) then render (imgbb + BBCode). The combined route above remains for the standalone
	// "📸 Generate screenshots" button.
	mux.Handle("POST /api/v1/torrents/{id}/screenshots/extract", permitted("torrents:publish", http.HandlerFunc(handleExtractScreenshots(d))))
	mux.Handle("POST /api/v1/screenshots/render", permitted("torrents:publish", http.HandlerFunc(handleRenderScreenshots(d))))
	mux.Handle("GET /api/v1/torrents", permitted("torrents:read", http.HandlerFunc(handleListTorrents(d))))
	mux.Handle("GET /api/v1/torrents/pending-tracker", permitted("torrents:read", http.HandlerFunc(handlePendingTracker(d))))
	mux.Handle("POST /api/v1/torrents/{id}/download-ticket", permitted("torrents:download", http.HandlerFunc(handleDownloadTicket(d))))
	mux.Handle("GET /api/v1/torrents/{id}/samples", permitted("torrents:read", http.HandlerFunc(handleTorrentSamples(d))))
	mux.Handle("POST /api/v1/torrents/{id}/pause", permitted("torrents:write", http.HandlerFunc(handleTorrentAction(d, "pause"))))
	mux.Handle("POST /api/v1/torrents/{id}/resume", permitted("torrents:write", http.HandlerFunc(handleTorrentAction(d, "resume"))))
	mux.Handle("POST /api/v1/torrents/{id}/recheck", permitted("torrents:write", http.HandlerFunc(handleTorrentAction(d, "recheck"))))
	mux.Handle("POST /api/v1/torrents/{id}/prioritize", permitted("torrents:write", http.HandlerFunc(handleTorrentPriority(d, true))))
	mux.Handle("DELETE /api/v1/torrents/{id}/prioritize", permitted("torrents:write", http.HandlerFunc(handleTorrentPriority(d, false))))
	mux.Handle("DELETE /api/v1/torrents/{id}", permitted("torrents:delete", http.HandlerFunc(handleTorrentRemove(d))))
	mux.Handle("POST /api/v1/torrents/{id}/migrate", permitted("torrents:migrate", http.HandlerFunc(handleTorrentMigrate(d))))
	mux.Handle("POST /api/v1/nodes/{id}/drain", permitted("nodes:write", http.HandlerFunc(handleNodeDrain(d))))
	mux.Handle("GET /api/v1/migrations", permitted("migrations:read", http.HandlerFunc(handleListMigrations(d))))
	mux.Handle("POST /api/v1/migrations/{id}/cancel", permitted("torrents:migrate", http.HandlerFunc(handleMigrationCancel(d))))
	mux.Handle("GET /api/v1/alerts", permitted("alerts:read", http.HandlerFunc(handleListAlerts(d))))
	mux.Handle("GET /api/v1/events", permitted("events:read", http.HandlerFunc(handleListEvents(d))))
	// Activity Feed (ADR-022): orchestration trail, separate from Alerts/Events.
	mux.Handle("GET /api/v1/activities", permitted("activities:read", http.HandlerFunc(handleListActivities(d))))
	mux.Handle("GET /api/v1/audit", permitted("audit:read", http.HandlerFunc(handleListAudit(d))))
	mux.Handle("GET /api/v1/metrics/query", permitted("metrics:read", http.HandlerFunc(handleMetricsQuery(d))))
	mux.Handle("GET /api/v1/metrics/cluster-throughput", permitted("metrics:read", http.HandlerFunc(handleClusterThroughput(d))))
	// Library analytics (#160): announce health, duplicates, stalled and space.
	mux.Handle("GET /api/v1/analytics", permitted("torrents:read", http.HandlerFunc(handleAnalytics(d))))
	mux.Handle("GET /api/v1/system/health", permitted("system:read", http.HandlerFunc(handleSystemHealth(d))))
	// Dynamic tunables (ADR-023): configuration panel with hot-reload.
	mux.Handle("GET /api/v1/settings", permitted("settings:read", http.HandlerFunc(handleListSettings(d))))
	mux.Handle("PUT /api/v1/settings/{key}", permitted("settings:write", http.HandlerFunc(handleUpdateSetting(d))))
	mux.Handle("DELETE /api/v1/settings/{key}", permitted("settings:write", http.HandlerFunc(handleResetSetting(d))))
	// Agent/Engine behavior configuration (ADR-039): who has its own value, and the
	// per-Node override. Cluster = settings:write (routes above); override = nodes:write.
	mux.Handle("GET /api/v1/settings/overrides", permitted("settings:read", http.HandlerFunc(handleListSettingOverrides(d))))
	mux.Handle("GET /api/v1/nodes/{id}/settings", permitted("nodes:read", http.HandlerFunc(handleGetNodeSettings(d))))
	mux.Handle("PUT /api/v1/nodes/{id}/settings/{key}", permitted("nodes:write", http.HandlerFunc(handleSetNodeSetting(d))))
	mux.Handle("DELETE /api/v1/nodes/{id}/settings/{key}", permitted("nodes:write", http.HandlerFunc(handleResetNodeSetting(d))))
	// Operator Trackers (ADR-019): announce credentials to publish content.
	mux.Handle("GET /api/v1/trackers", permitted("trackers:read", http.HandlerFunc(handleListTrackers(d))))
	mux.Handle("POST /api/v1/trackers", permitted("trackers:write", http.HandlerFunc(handleCreateTracker(d))))
	mux.Handle("PUT /api/v1/trackers/{id}", permitted("trackers:write", http.HandlerFunc(handleUpdateTracker(d))))
	// Metadata inference for UNIT3D tracker publication (ADR-020): name parse + TMDB.
	mux.Handle("POST /api/v1/publish/infer", permitted("publish:write", http.HandlerFunc(handleInferPublish(d))))
	// BBCode upload description composition (#192), per tracker (profile description section).
	mux.Handle("POST /api/v1/publish/description", permitted("publish:write", http.HandlerFunc(handleComposeDescription(d))))
	// IGDB game search for ROM/game publication (#17): the operator picks the match.
	mux.Handle("GET /api/v1/publish/games/search", permitted("publish:read", http.HandlerFunc(handleSearchGames(d))))
	mux.Handle("DELETE /api/v1/trackers/{id}", permitted("trackers:write", http.HandlerFunc(handleDeleteTracker(d))))
	mux.Handle("POST /api/v1/trackers/{id}/default", permitted("trackers:write", http.HandlerFunc(handleSetDefaultTracker(d))))
	mux.Handle("GET /api/v1/trackers/{id}/browse", permitted("trackers:read", http.HandlerFunc(handleBrowseTracker(d))))
	// Duplicate evidence before publishing (#186) — never blocks, only informs.
	mux.Handle("GET /api/v1/trackers/{id}/dupes", permitted("trackers:read", http.HandlerFunc(handleTrackerDupes(d))))
	mux.Handle("POST /api/v1/trackers/{id}/seed", permitted("trackers:seed", http.HandlerFunc(handleSeedFromTracker(d))))
	// Site profile (ADR-037): taxonomy editing, discovery from the tracker's own
	// catalog (#196 — only PROPOSES) and proposal application.
	mux.Handle("PUT /api/v1/trackers/{id}/profile", permitted("trackers:write", http.HandlerFunc(handleEditTrackerProfile(d))))
	// Description composition (#192): the profile description section — PUT saves, DELETE reverts to default.
	mux.Handle("PUT /api/v1/trackers/{id}/profile/description", permitted("trackers:write", http.HandlerFunc(handleSetTrackerDescription(d))))
	mux.Handle("DELETE /api/v1/trackers/{id}/profile/description", permitted("trackers:write", http.HandlerFunc(handleResetTrackerDescription(d))))
	// Image host (#193): the profile image_host section — PUT saves, DELETE removes (falls back to global imgbb).
	mux.Handle("PUT /api/v1/trackers/{id}/profile/image-host", permitted("trackers:write", http.HandlerFunc(handleSetTrackerImageHost(d))))
	mux.Handle("DELETE /api/v1/trackers/{id}/profile/image-host", permitted("trackers:write", http.HandlerFunc(handleResetTrackerImageHost(d))))
	// Image host key (#193 / ADR-040): PUT saves (bound to the current upload_url host), DELETE removes.
	mux.Handle("PUT /api/v1/trackers/{id}/image-host-key", permitted("trackers:write", http.HandlerFunc(handleSetTrackerImageHostKey(d))))
	mux.Handle("DELETE /api/v1/trackers/{id}/image-host-key", permitted("trackers:write", http.HandlerFunc(handleClearTrackerImageHostKey(d))))
	mux.Handle("POST /api/v1/trackers/{id}/discover-taxonomy", permitted("trackers:write", http.HandlerFunc(handleDiscoverTaxonomy(d))))
	mux.Handle("POST /api/v1/trackers/{id}/apply-taxonomy", permitted("trackers:write", http.HandlerFunc(handleApplyTaxonomy(d))))
	// Profile export/import (#197): the file is layer 2 and never carries credentials. Exporting
	// is a read (trackers:read); previewing and applying the import are trackers:write — the
	// preview never writes, and only the second call (…/import) changes the profile.
	mux.Handle("GET /api/v1/trackers/{id}/profile/export", permitted("trackers:read", http.HandlerFunc(handleExportTrackerProfile(d))))
	mux.Handle("POST /api/v1/trackers/{id}/profile/import/preview", permitted("trackers:write", http.HandlerFunc(handlePreviewProfileImport(d))))
	mux.Handle("POST /api/v1/trackers/{id}/profile/import", permitted("trackers:write", http.HandlerFunc(handleImportTrackerProfile(d))))

	return httpx.Chain(mux, httpx.RequestLogger(d.Logger))
}
