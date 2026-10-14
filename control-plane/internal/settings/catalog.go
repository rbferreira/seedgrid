// Package settings is the single source of truth for Control Plane behavior tunables
// (ADR-023). The catalog (here) defines the metadata for each tunable — type, default,
// unit, description, limits, and the legacy env used only to seed the database on the 1st
// boot. The Store (store.go) resolves the effective value (database override → catalog
// default) and applies changes at runtime (hot-reload), without redeploying.
//
// Separation of concerns (centralize-tunables-not-logic memory): the catalog
// centralizes the PARAMETERS; the LOGIC that consumes them lives in the domain (workers). The
// doc "25 - Business Rules.md" mirrors this catalog in prose.
package settings

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Kind is the type of a tunable — it determines the parsing and validation of the value
// (which is transmitted and always persisted as a string).
type Kind string

const (
	KindDuration Kind = "duration" // ex.: "5m", "90s", "168h" (time.ParseDuration)
	KindInt      Kind = "int"      // decimal integer
	KindInt64    Kind = "int64"    // 64-bit integer (e.g.: bytes/s)
	KindFloat    Kind = "float"    // floating-point (e.g.: percentage)
	KindString   Kind = "string"   // free text (e.g., URL); Min/Max do not apply
	KindBool     Kind = "bool"     // "true"/"false" (on/off)
)

// Category groups the tunables in the Portal dashboard.
type Category string

const (
	CatAdmission    Category = "admission"
	CatAlerting     Category = "alerting"
	CatWorkers      Category = "workers"
	CatPublish      Category = "publish"
	CatAgent        Category = "agent"
	CatNotify       Category = "notify"
	CatIntegrations Category = "integrations"
)

// Spec describes a tunable: metadata for seeding, validation, and display in the dashboard.
type Spec struct {
	Key         string   // namespaced key, e.g., "admission.dead_grace"
	Kind        Kind     // value type
	Category    Category // group in the panel
	Label       string   // short label (PT-BR)
	Description string   // description displayed in the dashboard (the "why"/effect of the parameter)
	Unit        string   // human-readable unit, e.g., "bytes/s", "%" (empty when the value itself says it)
	Default     string   // serialized default value (this is what applies without a database override)
	EnvVar      string   // legacy env that SEEDS the database on the 1st boot; "" = no seed
	// Min/Max are optional limits for numbers. For KindDuration, the unit is
	// SECONDS (the duration is converted before comparing). nil = no limit.
	Min *float64
	Max *float64
	// AllowZero opens an exception to Min when the value 0 itself has a special meaning
	// (e.g., "0 disables the feature") but low-yet-non-zero values are
	// dangerous (false positives) and therefore the actual Min is higher than 0.
	AllowZero bool
	// Options restricts a KindString to a closed set (enum): the value is validated
	// against the list and the dashboard renders a select instead of free text. nil = free.
	Options []string
	// Choices + Multi transform a KindString into a MULTI-selection with labels: the persisted
	// value is a CSV of the chosen Values, but the dashboard shows CHECKBOXES with the Labels
	// (PT-BR) — the operator doesn't need to know the internal codes (admission_hold,
	// ENGINE_RESTART…). Validation: each token in the CSV must be a known Value (or "*").
	Multi   bool
	Choices []Choice
	// Secret marks an operator's third-party credential (bot token, webhook URL, and the
	// TMDB/IGDB/ptpimg integration keys — ADR-028 and ADR-029). The value NEVER comes back in the read
	// API (only `has_value`); PUT with an empty value KEEPS the current one (otherwise saving another
	// field would erase the token); to clear, DELETE (reset to empty default) — which is also
	// how a channel/integration is disabled.
	//
	// Only the OWN deployment's infra stays in env (Postgres, admin bootstrap, registration
	// token): it's not a third-party credential nor "the author's" (ADR-029).
	Secret bool
	// PerNode marks an AGENT/ENGINE behavior tunable (ADR-039): the Control Plane
	// DELIVERS it to the Agent in the heartbeat response and it allows overrides per Node (table
	// node_settings). The catalog value (and the database override, `settings`) is the CLUSTER
	// value — what applies to every Node without an override. The other tunables belong to the
	// Control Plane itself and have no notion of Node.
	PerNode bool
	// AgentEnv is the EQUIVALENT environment variable on the Node (PerNode only). It seeds nothing
	// on the Control Plane (EnvVar is left empty: the CP env is not the Node's) — it documents the
	// Agent's fallback and is what the Agent reports when seeding the overrides (ADR-039 §7).
	AgentEnv string
}

func f64(v float64) *float64 { return &v }

// Choice is an option for a multi-select field: Value is the persisted code (CSV), Label
// is the friendly text shown in the dashboard.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// catalog is the ordered list of tunables. The order is the display order in the dashboard.
// The defaults exactly reproduce the values in effect before ADR-023.
// eventChoices and activityChoices are the tracks offered to the operator. They are kept in a
// variable because the global filter AND the per-channel overrides (channelOverrideSpecs)
// need the SAME list — duplicating it would leave a channel without a new option.
var eventChoices = []Choice{
	{"NODE_REGISTERED", "Node joined the Cluster"},
	{"NODE_DELETED", "Node removed"},
	{"TORRENT_CREATED", "Torrent added"},
	{"TORRENT_REMOVED", "Torrent removed"},
	{"TORRENT_PAUSED", "Torrent paused"},
	{"TORRENT_RESUMED", "Torrent resumed"},
	{"TORRENT_RECHECK", "Torrent rechecking"},
	{"TORRENT_MIGRATION_STARTED", "Torrent migration started"},
	{"TORRENT_MIGRATED", "Torrent moved to another Node"},
	{"TORRENT_MIGRATION_FAILED", "Torrent migration failed"},
	{"TORRENT_RESEEDED", "Torrent reseeded"},
	{"TORRENT_PUBLISHED_EXTERNAL", "Torrent published to tracker"},
	{"ENGINE_RESTART", "Engine restarted"},
	{"AGENT_UPDATE", "Agent updated"},
	{"STORAGE_RECOVERY", "Node disk reconnected"},
}

var activityChoices = []Choice{
	{"admission_hold", "Download paused to free a slot"},
	{"admission_resume", "Download resumed when slot opened"},
	{"scheduled", "Torrent assigned to Node"},
	{"downloading", "Torrent started downloading"},
	{"seeding", "Torrent started seeding"},
	{"node_blocked", "Torrent could not be scheduled (Node disk failing)"},
	{"screenshot_host_failed", "Image host failed to upload screenshot"},
}

var baseCatalog = []Spec{
	// ── Download admission (#21, #22, ADR-021) ────────────────────────────────
	{
		Key: "admission.interval", Kind: KindDuration, Category: CatAdmission,
		Label:       "Evaluation interval",
		Description: "Time between admission controller runs. Lower = reacts faster to changes; higher = less load on the database/Cluster.",
		Default:     "20s", EnvVar: "SEEDGRID_ADMISSION_INTERVAL",
		Min: f64(1), Max: f64(600),
	},
	{
		Key: "admission.max_cluster_downloads", Kind: KindInt, Category: CatAdmission,
		Label:       "Download limit (fallback)",
		Description: "Maximum torrents downloading in the Cluster when no tracker has reported an L. 0 disables the fallback (no limit).",
		Default:     "0", EnvVar: "SEEDGRID_MAX_CLUSTER_DOWNLOADS",
		Min: f64(0), Max: f64(1000),
	},
	{
		Key: "admission.dl_slot_limit_ttl", Kind: KindDuration, Category: CatAdmission,
		Label:       "Observed L validity",
		Description: "How long an observed L (slot limit) from a tracker message remains valid before expiring.",
		Default:     "6h", EnvVar: "SEEDGRID_DL_SLOT_LIMIT_TTL",
		Min: f64(60), Max: f64(604800),
	},
	{
		Key: "admission.min_dl_rate", Kind: KindInt64, Category: CatAdmission,
		Label:       "Minimum download rate",
		Description: "An active download sustained below this rate is considered slow and yields its slot during healthy-rate rotation. 0 disables slow rotation.",
		Unit:        "bytes/s", Default: "51200", EnvVar: "SEEDGRID_ADMISSION_MIN_DL_RATE",
		Min: f64(0), Max: f64(1073741824),
	},
	{
		Key: "admission.disk_min_free", Kind: KindInt64, Category: CatAdmission,
		Label:       "Free disk margin",
		Description: "Disk space never committed on a Node. A torrent only enters or migrates to a Node if the projected free space (current free minus what existing torrents still need to download) fits the torrent's TOTAL SIZE plus this margin. Below it, the Node triggers the \"disk full\" alert. Default 5 GiB.",
		Unit:        "bytes", Default: "5368709120", EnvVar: "SEEDGRID_ADMISSION_DISK_MIN_FREE",
		Min: f64(0), Max: f64(1099511627776),
	},
	{
		Key: "admission.slow_grace", Kind: KindDuration, Category: CatAdmission,
		Label:       "Slow grace period",
		Description: "Continuous time below the minimum rate before rotating out a slow download. Prevents rotation due to brief dips. Minimum 10min regardless of the configured rate — initial real peer discovery/negotiation can legitimately consume most of a short window. To disable slow rotation, use admission.min_dl_rate=0 (not this field).",
		Default:     "10m", EnvVar: "SEEDGRID_ADMISSION_SLOW_GRACE",
		Min: f64(600), Max: f64(86400),
	},
	{
		Key: "admission.slow_cooldown", Kind: KindDuration, Category: CatAdmission,
		Label:       "Slow cooldown",
		Description: "How long a download rotated for being slow stays out of the promotion queue, preventing immediate re-entry and thrashing.",
		Default:     "10m", EnvVar: "SEEDGRID_ADMISSION_SLOW_COOLDOWN",
		Min: f64(1), Max: f64(86400),
	},
	{
		Key: "admission.dead_grace", Kind: KindDuration, Category: CatAdmission,
		Label:       "Dead torrent grace period",
		Description: "With continuous 0 seeds AND 0 peers for this duration, the torrent is marked dead (will never complete) and yields its slot. Minimum 5min (below that causes false positives — real announce/peer discovery can legitimately take longer); 0 disables dead rotation.",
		Default:     "5m", EnvVar: "SEEDGRID_ADMISSION_DEAD_GRACE",
		Min: f64(300), Max: f64(86400), AllowZero: true,
	},
	{
		Key: "admission.dead_cooldown", Kind: KindDuration, Category: CatAdmission,
		Label:       "Dead torrent cooldown",
		Description: "How long a rotated dead torrent stays at the back of the queue — it won't be promoted again even with the highest progress.",
		Default:     "30m", EnvVar: "SEEDGRID_ADMISSION_DEAD_COOLDOWN",
		Min: f64(1), Max: f64(604800),
	},
	{
		Key: "admission.dead_backoff_max", Kind: KindDuration, Category: CatAdmission,
		Label:       "Dead torrent backoff cap",
		Description: "Each consecutive time the same torrent is rotated for being dead, the cooldown doubles (30m → 1h → 2h…) up to this cap — a chronically dead torrent backs itself out of the queue instead of returning every cooldown. Connecting to any peer resets the count. 0 disables backoff (cooldown stays fixed).",
		Default:     "4h", EnvVar: "SEEDGRID_ADMISSION_DEAD_BACKOFF_MAX",
		Min: f64(0), Max: f64(604800),
	},
	{
		Key: "admission.priority_persistent", Kind: KindBool, Category: CatAdmission,
		Label:       "Persistent priority mode",
		Description: "When enabled, a manually prioritized torrent (⚡ button) is NOT rotated for being dead (no seeds/peers) or slow — it holds the slot until the download finishes. Disabled (default): priority only skips the queue; once admitted, the torrent follows normal flow and loses priority if dead/slow rotation demotes it. Warning: in persistent mode, a prioritized torrent with no seeds occupies a slot without downloading until you remove it or seeds appear.",
		Default:     "false", EnvVar: "SEEDGRID_ADMISSION_PRIORITY_PERSISTENT",
	},
	// ── Agent (ADR-027) ───────────────────────────────────────────────────────
	{
		Key: "agent.artifacts_url", Kind: KindString, Category: CatAgent,
		Label:       "Agent artifacts URL",
		Description: "Where the Portal publishes Agent binaries and the agents.json manifest (e.g., http://controller.example/node). The Control Plane reads the manifest here to check available versions, and the Agent DOWNLOADS the binary from this same URL — it must be reachable from the Seed Nodes, not an internal address. Empty disables orchestrated updates (no update badge; the button is refused).",
		Default:     "", EnvVar: "SEEDGRID_AGENT_ARTIFACTS_URL",
	},

	// ── Agent and Engine behavior (ADR-039, #144) ───────────────────────────
	// CLUSTER value; each Node can have an override (node_settings). Delivered to the Agent in the
	// heartbeat response and applied on the fly. The defaults are what the MAJORITY of the fleet
	// used in .env before ADR-039 (inventory from 03/10/2026: heartbeat 10s, poll 5s,
	// downloads 10 on 3 of the 4 Nodes; the rest was never defined and fell back to the code default).
	// EnvVar is purposely left empty (the CP env is not the Node's); AgentEnv is the equivalent
	// on the Node, reported by the Agent during seeding. shared/agent-config.json mirrors this list.
	{
		Key: "agent.heartbeat_interval", Kind: KindDuration, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_AGENT_HEARTBEAT_INTERVAL",
		Label:       "Heartbeat interval",
		Description: "How often (in seconds) the Agent reports to the Control Plane that it's alive, sends resource stats (and receives this config). Lower = the Portal and alerts see the Node sooner, at the cost of more requests. Keep well below \"Node offline after\" (alerting.node_offline_after, 90s): above half of it, a single missed heartbeat may mark the Node offline.",
		Default:     "10s", Min: f64(2), Max: f64(60),
	},
	{
		Key: "agent.command_poll_interval", Kind: KindDuration, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_AGENT_COMMAND_POLL_INTERVAL",
		Label:       "Command poll interval",
		Description: "How often (in seconds) the Agent asks the Control Plane for commands to run (pause, migrate, update…). This is the latency from an operator action reaching the Node.",
		Default:     "5s", Min: f64(1), Max: f64(60),
	},
	{
		Key: "agent.log_level", Kind: KindString, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_LOG_LEVEL",
		Options:     []string{"debug", "info", "warn", "error"},
		Label:       "Agent log level",
		Description: "Verbosity of the Agent log on the Node. `debug` helps troubleshoot a problematic Node (a per-Node override avoids cluttering the entire fleet's logs) but generates a lot of text.",
		Default:     "info",
	},
	{
		Key: "engine.queueing_enabled", Kind: KindBool, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_QUEUEING_ENABLED",
		Label:       "Engine queue",
		Description: "Enables the Engine queue, which limits how many torrents are active at once. This protects against I/O saturation: without a queue, adding many torrents starts all downloads together (I/O at 100%, torrents stuck verifying, and USB HDDs dropping due to power starvation). When disabled, the «engine.max_active_*» limits below no longer apply.",
		Default:     "true",
	},
	{
		Key: "engine.max_active_downloads", Kind: KindInt, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS",
		Label:       "Simultaneous downloads per Node",
		Description: "Maximum downloads the Engine keeps active at once on a Node; the rest wait in the queue. Each concurrent download costs piece hashing and I/O: weaker hardware (Atom, USB HDD) needs a lower number than the fleet — set it with an override on that Node, don't lower the Cluster value. The admission controller enforces the tracker slot limit across the whole Cluster; this is the per-machine safety net.",
		Default:     "10", Min: f64(1), Max: f64(200),
	},
	{
		Key: "engine.max_active_uploads", Kind: KindInt, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS",
		Label:       "Simultaneous uploads per Node",
		Description: "Maximum torrents seeding at once. The default is intentionally near-unlimited: a queued seeder (queuedUP) sits idle, sends event=stopped to the tracker, stops accumulating seedtime, and gets Hit and Run. Seeders should never be queued.",
		Default:     "10000", Min: f64(1), Max: f64(100000),
	},
	{
		Key: "engine.max_active_torrents", Kind: KindInt, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS",
		Label:       "Total active torrents per Node",
		Description: "Maximum active torrents (downloading + seeding). For the same reason as uploads, the default is near-unlimited: only downloads and checks should be queued.",
		Default:     "10000", Min: f64(1), Max: f64(100000),
	},
	{
		Key: "engine.max_active_checking", Kind: KindInt, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING",
		Label:       "Simultaneous checks per Node",
		Description: "How many torrents the Engine verifies (piece hashing) at once. Verification reads the torrent's entire data from disk: more than one at a time saturates USB HDD I/O.",
		Default:     "1", Min: f64(1), Max: f64(10),
	},
	{
		Key: "engine.dont_count_slow", Kind: KindBool, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_DONT_COUNT_SLOW",
		Label:       "Slow downloads don't count toward ceiling",
		Description: "OFF (default) ON PURPOSE: when on, a stalled download (0 KB/s, waiting for a tracker slot) doesn't count toward the ceiling, so the Engine starts ALL downloads, all announce at once, the tracker refuses slots (thundering herd), and everything locks up. When off, the ceiling holds and the excess queues without announcing.",
		Default:     "false",
	},
	{
		Key: "engine.slow_dl_threshold", Kind: KindInt, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_SLOW_DL_THRESHOLD",
		Label:       "Slow download threshold",
		Description: "Below this download speed, a torrent is considered slow by the Engine. Only takes effect when «Slow downloads don't count toward ceiling» is on.",
		Unit:        "KB/s", Default: "1", Min: f64(0), Max: f64(1000000),
	},
	{
		Key: "engine.slow_ul_threshold", Kind: KindInt, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_SLOW_UL_THRESHOLD",
		Label:       "Slow upload threshold",
		Description: "Below this upload speed, a torrent is considered slow by the Engine. Only takes effect when «Slow downloads don't count toward ceiling» is on.",
		Unit:        "KB/s", Default: "1", Min: f64(0), Max: f64(1000000),
	},
	{
		Key: "engine.slow_inactive_timer", Kind: KindInt, Category: CatAgent,
		PerNode: true, AgentEnv: "SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER",
		Label:       "Slow time before freeing slot",
		Description: "How long continuously below the threshold the Engine waits before treating the torrent as slow. Only takes effect when «Slow downloads don't count toward ceiling» is on.",
		Unit:        "s", Default: "300", Min: f64(1), Max: f64(86400),
	},

	// ── Third-party integrations (ADR-029) ───────────────────────────────────
	// Credentials for external services tied to the OPERATOR'S ACCOUNT — not the deployment's
	// infra. They live in the dashboard (not in env) so the project can be opened without the
	// maintainer's personal key. EnvVar seeds from the current .env on the 1st boot (backward compat, ADR-023).
	// Secret: they never come back on read; hot-reload (clients re-read per call).
	{
		Key: "integrations.tmdb_api_key", Kind: KindString, Category: CatIntegrations, Secret: true,
		Label:       "TMDB — API key (v3)",
		Description: "Your TMDB (themoviedb.org) v3 API key to enrich metadata on publish. Empty disables enrichment. This is YOUR key — register at themoviedb.org.",
		Default:     "", EnvVar: "SEEDGRID_TMDB_API_KEY",
	},
	{
		Key: "integrations.igdb_client_id", Kind: KindString, Category: CatIntegrations,
		Label:       "IGDB — Client ID (Twitch)",
		Description: "Twitch app Client ID used by IGDB (game metadata). Without the ID+Secret pair, game publishing is disabled.",
		Default:     "", EnvVar: "SEEDGRID_IGDB_CLIENT_ID",
	},
	{
		Key: "integrations.igdb_client_secret", Kind: KindString, Category: CatIntegrations, Secret: true,
		Label:       "IGDB — Client Secret (Twitch)",
		Description: "Twitch app Client Secret (client-credentials, machine-to-machine). Never shown again.",
		Default:     "", EnvVar: "SEEDGRID_IGDB_CLIENT_SECRET",
	},
	{
		Key: "integrations.imgbb_api_key", Kind: KindString, Category: CatIntegrations, Secret: true,
		Label:       "imgbb — API key",
		Description: "imgbb.com API key to host publish screenshots. Create yours free at api.imgbb.com. Empty disables screenshot generation.",
		Default:     "", EnvVar: "SEEDGRID_IMGBB_API_KEY",
	},
	{
		Key: "integrations.google_books_api_key", Kind: KindString, Category: CatIntegrations, Secret: true,
		Label:       "Google Books — API key",
		Description: "Google Books API key, used as the SECOND book/audiobook metadata provider in the catalog — Open Library doesn't index much of the PT-BR collection or recent releases. PRACTICALLY REQUIRED: without a key the API returns 429 (Too Many Requests) after very few lookups, and a single catalog page already exceeds that — measured, not estimated. Empty = the provider stays idle and the book relies only on what Open Library confirms. Create yours at console.cloud.google.com (enable the Books API).",
		Default:     "", EnvVar: "SEEDGRID_GOOGLE_BOOKS_API_KEY",
	},

	// ── Outbound notifications / telemetry (ADR-028) ─────────────────────────
	// A channel with an EMPTY credential is disabled — there is no separate "enabled" flag
	// (a toggle that can diverge from the input is an inconsistency to maintain).
	// Secret fields never come back in the read API; to CLEAR (disable the channel),
	// use "Restore default" (DELETE), don't save empty.
	{
		Key: "notify.telegram_bot_token", Kind: KindString, Category: CatNotify, Secret: true,
		Label:       "Telegram — bot token",
		Description: "Bot token created via @BotFather (format 123456:ABC-DEF…). Filling it enables the channel; restoring the default disables it. Never shown again.",
		Default:     "", EnvVar: "SEEDGRID_NOTIFY_TELEGRAM_BOT_TOKEN",
	},
	{
		Key: "notify.telegram_chat_id", Kind: KindString, Category: CatNotify,
		Label:       "Telegram — destination chat_id",
		Description: "Where to send (your user, a group, or a channel). A number, possibly negative for groups (e.g., -1001234567890). The channel only sends when both token AND chat_id are filled.",
		Default:     "", EnvVar: "SEEDGRID_NOTIFY_TELEGRAM_CHAT_ID",
	},
	{
		Key: "notify.discord_webhook_url", Kind: KindString, Category: CatNotify, Secret: true,
		Label:       "Discord — webhook URL",
		Description: "Channel webhook URL (Edit Channel → Integrations → Webhooks). Filling it enables the channel. Never shown again.",
		Default:     "", EnvVar: "SEEDGRID_NOTIFY_DISCORD_WEBHOOK_URL",
	},
	{
		Key: "notify.webhook_url", Kind: KindString, Category: CatNotify, Secret: true,
		Label:       "Generic webhook — URL",
		Description: "Receives a raw JSON POST on every notification (fields: kind, severity, title, message, node, at). Works with Slack, n8n, Home Assistant, or anything that accepts HTTP — no specific SeedGrid support needed. Never shown again.",
		Default:     "", EnvVar: "SEEDGRID_NOTIFY_WEBHOOK_URL",
	},
	{
		Key: "notify.alerts_min_severity", Kind: KindString, Category: CatNotify,
		Options:     []string{"OFF", "CRITICAL", "WARNING", "INFO"},
		Label:       "Alerts — minimum severity",
		Description: "Which alerts (Node offline, storage, disk saturated) get notified. OFF disables the alert pipeline without touching channels. CRITICAL = only what needs immediate action; WARNING includes degradation; INFO sends everything.",
		Default:     "WARNING", EnvVar: "SEEDGRID_NOTIFY_ALERTS_MIN_SEVERITY",
	},
	{
		Key: "notify.alerts_on_resolve", Kind: KindBool, Category: CatNotify,
		Label:       "Alerts — notify on resolve",
		Description: "Also sends when an alert resolves (Node came back, storage recovered). Knowing it's back usually matters as much as knowing it went down — and saves pointless checking.",
		Default:     "true", EnvVar: "SEEDGRID_NOTIFY_ALERTS_ON_RESOLVE",
	},
	{
		Key: "notify.events", Kind: KindString, Category: CatNotify, Multi: true,
		Label:       "Events to notify",
		Description: "Everyday cluster events. Check what you want to receive; with nothing checked, none are sent.",
		Default:     "", EnvVar: "SEEDGRID_NOTIFY_EVENTS",
		Choices: eventChoices,
	},
	{
		Key: "notify.activities", Kind: KindString, Category: CatNotify, Multi: true,
		Label:       "Orchestration decisions to notify",
		Description: "What SeedGrid does on its own to balance downloads. ⚠️ Usually happens many times per hour — leave unchecked unless you want to follow closely.",
		Default:     "", EnvVar: "SEEDGRID_NOTIFY_ACTIVITIES",
		Choices: activityChoices,
	},
	{
		Key: "notify.language", Kind: KindString, Category: CatNotify,
		Label:       "Notification language",
		Description: "Language for channel messages (Telegram, Discord, webhook). It's per cluster, not per operator: notifications go to a channel. The Portal follows each user's language.",
		Default:     "en", EnvVar: "SEEDGRID_NOTIFY_LANGUAGE",
		Options: []string{"en", "pt-BR"},
	},
	{
		Key: "notify.rate_limit_per_min", Kind: KindInt, Category: CatNotify,
		Label:       "Messages per minute cap",
		Description: "Protects against flooding (e.g., a Node flapping between ONLINE/OFFLINE). Excess within the window is dropped and the suppressed total goes to the log. 0 disables the limit.",
		Unit:        "msg/min", Default: "20", EnvVar: "SEEDGRID_NOTIFY_RATE_LIMIT_PER_MIN",
		Min: f64(0), Max: f64(1000),
	},

	// ── Alerts / Monitoring (ADR-008) ────────────────────────────────────────
	{
		Key: "alerting.worker_interval", Kind: KindDuration, Category: CatAlerting,
		Label:       "Alert Worker interval",
		Description: "Period between evaluations of deterministic alert rules.",
		Default:     "30s", EnvVar: "SEEDGRID_ALERT_WORKER_INTERVAL",
		Min: f64(1), Max: f64(600),
	},
	{
		Key: "alerting.node_offline_after", Kind: KindDuration, Category: CatAlerting,
		Label:       "Node offline after",
		Description: "Time without a heartbeat before a Seed Node is considered offline (triggers alert).",
		Default:     "90s", EnvVar: "SEEDGRID_NODE_OFFLINE_AFTER",
		Min: f64(10), Max: f64(3600),
	},
	{
		Key: "alerting.disk_saturated_pct", Kind: KindFloat, Category: CatAlerting,
		Label:       "Disk saturation",
		Description: "Disk I/O utilization percentage above which the disk is considered saturated (triggers alert).",
		Unit:        "%", Default: "90", EnvVar: "SEEDGRID_DISK_IO_SATURATED_PCT",
		Min: f64(1), Max: f64(100),
	},
	{
		Key: "alerting.disk_saturated_for", Kind: KindDuration, Category: CatAlerting,
		Label:       "Sustained disk saturation",
		Description: "How long the disk must stay continuously above the saturation threshold before raising DISK_IO_SATURATED. Readings come from the heartbeat (~10s) and fluctuate; requiring continuous time avoids alerts from second-long spikes. An Alert Worker check below the threshold resets the counter; the alert fires on the first check after the time is met. The counter restarts if the Controller restarts. Resolution has no grace period. 0 = alert on first reading.",
		Default:     "90s", EnvVar: "SEEDGRID_DISK_IO_SATURATED_FOR",
		Min: f64(0), Max: f64(3600),
	},
	{
		Key: "alerting.storage_grace", Kind: KindDuration, Category: CatAlerting,
		Label:       "Failed storage grace",
		Description: "How long a Node's /data must fail before alerting (NODE_STORAGE). The heartbeat is 30s and the alert used to fire on ONE reading: an I/O hiccup—expected on underpowered USB HDDs—became a CRITICAL alert resolved 30 seconds later. Resolution has NO grace period: storage coming back is good news. 0 alerts on first reading, as before.",
		Default:     "1m", EnvVar: "SEEDGRID_STORAGE_GRACE",
	},
	{
		Key: "alerting.firewalled_grace", Kind: KindDuration, Category: CatAlerting,
		Label:       "Connectability grace",
		Description: "How long a Node can receive no incoming connections before alerting (NODE_FIREWALLED). Every Node starts 'firewalled' until the first peer arrives, so alerting immediately would trigger on every Engine restart. 0 disables the rule.",
		Default:     "30m", EnvVar: "SEEDGRID_FIREWALLED_GRACE",
	},
	{
		Key: "alerting.incoming_silence", Kind: KindDuration, Category: CatAlerting,
		Label:       "Allowed inbound silence",
		Description: "How long a Node can go with NO incoming connections when there is demand before alerting (NODE_INBOUND_SILENT). Unlike connectability grace: that uses the Engine verdict, which never reverses; this uses a decaying signal, catching Nodes that lose reachability in steady state. Incoming connections are rare in an idle collection: a healthy Node may go days without one, and a low value causes false alerts. What counts as demand is defined by \"Seeds per leecher that count as demand\". 0 disables the rule.",
		Default:     "240h", EnvVar: "SEEDGRID_INCOMING_SILENCE",
	},
	{
		Key: "alerting.incoming_demand_seeds_per_leecher", Kind: KindFloat, Category: CatAlerting,
		Label:       "Seeds per leecher that count as demand",
		Description: "A torrent only counts as demand for NODE_INBOUND_SILENT when the swarm has at most this many seeds per leecher (seeds <= leechers x value). In a well-seeded swarm, a leecher rarely connects to a specific Node, so no inbound traffic says nothing about the port. 1 counts only swarms where leechers are at least as numerous as seeds; 0 counts only swarms with no seeds; a very high value (e.g., 100000) counts any leecher again. Torrents missing seed or leecher counts don't count.",
		Unit:        "seeds/leecher", Default: "1", EnvVar: "",
		Min: f64(0), Max: f64(1000000),
	},
	{
		Key: "alerting.ipv6_change_grace", Kind: KindDuration, Category: CatAlerting,
		Label:       "IPv6 change grace",
		Description: "How long a new IPv6 address must persist before alerting (NODE_IPV6_CHANGED). Privacy extensions rotate the suffix automatically; without grace, a misconfigured Node would alert on every ephemeral change. 0 disables the rule and closes any open alerts for it.",
		Default:     "15m", EnvVar: "SEEDGRID_IPV6_CHANGE_GRACE",
	},
	{
		Key: "alerting.reachability_ttl", Kind: KindDuration, Category: CatAlerting,
		Label:       "Reachability alert TTL",
		Description: "How long NODE_FIREWALLED, NODE_INBOUND_SILENT and NODE_IPV6_CHANGED stay open before auto-closing. All three infer reachability from the ABSENCE of incoming connections and only resolve when one arrives—which a Node with no demand never gets, even with an open port. The notice fires on open; on expiry, the alert closes silently and won't reopen while the condition persists (same IPv6 change, same silence). Counted from the last notification. 0 = no TTL: only closes when an incoming connection arrives.",
		Default:     "24h", EnvVar: "SEEDGRID_REACHABILITY_TTL",
	},
	{
		Key: "alerting.sample_retention", Kind: KindDuration, Category: CatAlerting,
		Label:       "Sample retention",
		Description: "How long metric samples are kept before the worker cleans them up.",
		Default:     "168h", EnvVar: "SEEDGRID_SAMPLE_RETENTION",
		Min: f64(3600), Max: f64(7776000),
	},
	// ── Internal Workers (previously hardcoded) ──────────────────────────────
	{
		Key: "commands.redispatch_after", Kind: KindDuration, Category: CatWorkers,
		Label:       "Orphan command redelivery",
		Description: "Age after which a DISPATCHED command with no Agent confirmation is redelivered on the next poll (the Agent died between claiming it and reporting the result). Must be GREATER than the longest expected command: the Agent runs the batch serially and a large migrate_pull holds up the queue while copying data. 0 disables redelivery.",
		Default:     "1h", EnvVar: "",
		Min: f64(0), Max: f64(86400),
	},
	{
		Key: "commands.max_attempts", Kind: KindInt, Category: CatWorkers,
		Label:       "Max command deliveries",
		Description: "How many Agent deliveries a command can receive before being marked FAILED as abandoned — prevents infinite redelivery loops for commands that always crash the Agent.",
		Default:     "3", EnvVar: "",
		Min: f64(1), Max: f64(10),
	},
	{
		Key: "workers.maintenance_interval", Kind: KindDuration, Category: CatWorkers,
		Label:       "Maintenance Worker interval",
		Description: "Interval for retention pruning: terminal commands, events, activities, audit, and expired sessions.",
		Default:     "1h", EnvVar: "",
		Min: f64(60), Max: f64(86400),
	},
	{
		Key: "retention.commands", Kind: KindDuration, Category: CatWorkers,
		Label:       "Command retention",
		Description: "How long DONE/FAILED commands stay in the database before pruning. PENDING/DISPATCHED are never pruned. 0 disables.",
		Default:     "720h", EnvVar: "",
		Min: f64(0), Max: f64(31536000),
	},
	{
		Key: "retention.events", Kind: KindDuration, Category: CatWorkers,
		Label:       "Event retention",
		Description: "How long events stay in the database before pruning. 0 disables.",
		Default:     "2160h", EnvVar: "",
		Min: f64(0), Max: f64(31536000),
	},
	{
		Key: "retention.activities", Kind: KindDuration, Category: CatWorkers,
		Label:       "Activity feed retention",
		Description: "How long the orchestration trail stays in the database before pruning. 0 disables.",
		Default:     "2160h", EnvVar: "",
		Min: f64(0), Max: f64(31536000),
	},
	{
		Key: "retention.audit", Kind: KindDuration, Category: CatWorkers,
		Label:       "Audit retention",
		Description: "How long the audit trail stays in the database before pruning. Longer retention for accountability. 0 disables.",
		Default:     "8760h", EnvVar: "",
		Min: f64(0), Max: f64(63072000),
	},
	{
		Key: "workers.reconciler_interval", Kind: KindDuration, Category: CatWorkers,
		Label:       "Migration Reconciler interval",
		Description: "Interval for the reconciler that advances/finalizes in-flight torrent migrations.",
		Default:     "15s", EnvVar: "",
		Min: f64(1), Max: f64(600),
	},
	{
		Key: "migration.max_concurrent", Kind: KindInt, Category: CatWorkers,
		Label:       "Concurrent migrations",
		Description: "How many torrent migrations can transfer data at once. Others wait in queue. Keep low: each migration is a heavy disk copy, and draining an entire Node at once saturates the I/O of the very machine in trouble.",
		Default:     "1", EnvVar: "",
		Min: f64(1), Max: f64(10),
	},
	{
		Key: "migration.recheck_stall_window", Kind: KindDuration, Category: CatWorkers,
		Label:       "Migration recheck stall",
		Description: "How long the recheck on the target Node can make no progress before the migration is aborted as stalled. This is what matters: while progress increases, waiting continues regardless of disk speed—only a stall fails it. Monitored by the Control Plane on each reconciler pass.",
		Default:     "10m", EnvVar: "",
		Min: f64(60), Max: f64(86400),
	},
	{
		Key: "migration.recheck_max_wait", Kind: KindDuration, Category: CatWorkers,
		Label:       "Migration recheck ceiling",
		Description: "Absolute ceiling, counted from copy completion, for the destination to verify it has what the source had. Only catches an Engine reporting oscillating progress forever; stagnation (above) is the real safeguard. Since the wait no longer blocks the Node command queue, the ceiling can be generous — the cost of a high ceiling is that a migration.max_concurrent slot stays occupied.",
		Default:     "6h", EnvVar: "",
		Min: f64(600), Max: f64(604800),
	},
	{
		Key: "workers.prom_writer_interval", Kind: KindDuration, Category: CatWorkers,
		Label:       "Prometheus Targets Writer interval",
		Description: "Rewrite period for the Prometheus targets file (file_sd) with active Nodes.",
		Default:     "30s", EnvVar: "",
		Min: f64(1), Max: f64(600),
	},
	{
		Key: "metrics.torrent_sample_interval", Kind: KindDuration, Category: CatWorkers,
		Label:       "Per-torrent sample interval",
		Description: "Minimum interval between time-series sample batches per Node. The Agent syncs every ~15s; writing a sample on every sync is the biggest source of database growth. 0 = sample on every sync.",
		Default:     "60s", EnvVar: "",
		Min: f64(0), Max: f64(3600),
	},
	{
		Key: "metrics.cluster_sample_interval", Kind: KindDuration, Category: CatWorkers,
		Label:       "Cluster sample interval",
		Description: "Period between snapshots of cumulative cluster throughput (download/upload) recorded for the Metrics page trend chart (F2). Lower = smoother curve and more rows in the database.",
		Default:     "5m", EnvVar: "SEEDGRID_METRICS_CLUSTER_SAMPLE_INTERVAL",
		Min: f64(30), Max: f64(3600),
	},
	{
		Key: "metrics.cluster_sample_retention", Kind: KindDuration, Category: CatWorkers,
		Label:       "Cluster sample retention",
		Description: "How long cluster throughput snapshots are kept. 0 = keep forever (the table is tiny, ~1 row per interval). For the 1-year filter to work, keep ≥ 1 year or 0.",
		Default:     "0s", EnvVar: "SEEDGRID_METRICS_CLUSTER_SAMPLE_RETENTION",
		Min: f64(0), Max: f64(157680000),
	},
	// ── Tracker announce (ADR-020/ADR-026) ─────────────────────────────────────
	{
		Key: "publish.screenshot_count", Kind: KindInt, Category: CatPublish,
		Label:       "Screenshots per upload",
		Description: "Number of frames to extract from the video file when generating screenshots for publishing to a tracker.",
		Default:     "4", EnvVar: "SEEDGRID_SCREENSHOT_COUNT",
		Min: f64(1), Max: f64(10),
	},
}

// notifyChannels are the telemetry channels that accept their own filters.
var notifyChannels = []struct{ key, label string }{
	{"telegram", "Telegram"},
	{"discord", "Discord"},
	{"webhook", "Webhook"},
}

// channelOverrideSpecs replicates, per channel, the same set of filters as the global one.
//
// Without this, every configured channel receives EVERYTHING that passes the cluster filter —
// the operator cannot send just the IPv6 change to the webhook that triggers an
// automation, and the endpoint becomes a dumping ground for what the entire cluster notifies.
//
// The `custom` toggle exists because empty already has a meaning in feeds (`listMatches("")`
// = no event), so it cannot be used as "inherit". With `custom = false` (default)
// the channel behaves exactly as it does today; nothing breaks for existing users.
//
// Generated instead of hand-written: there are 6 keys per channel, and the panel (ADR-023) renders
// each Spec individually — making a mistake when copy-pasting would be silent.
func channelOverrideSpecs() []Spec {
	var out []Spec
	for _, ch := range notifyChannels {
		p := "notify." + ch.key + "."
		out = append(out,
			Spec{
				Key: p + "custom", Kind: KindBool, Category: CatNotify,
				Label: ch.label + " — custom filters",
				Description: "Enabled, this channel uses the filters below instead of the general ones. " +
					"Disabled (default), follows the general notification filters.",
				Default: "false",
			},
			Spec{
				Key: p + "alerts_min_severity", Kind: KindString, Category: CatNotify,
				Label:       ch.label + " — minimum alert severity",
				Description: "Only applies when custom filters are enabled. OFF sends no alerts on this channel.",
				Default:     "WARNING", Options: []string{"OFF", "INFO", "WARNING", "CRITICAL"},
			},
			Spec{
				Key: p + "alerts_on_resolve", Kind: KindBool, Category: CatNotify,
				Label:       ch.label + " — notify when alert resolves",
				Description: "Only applies when custom filters are enabled.",
				Default:     "true",
			},
			Spec{
				Key: p + "events", Kind: KindString, Category: CatNotify, Multi: true,
				Label:       ch.label + " — events",
				Description: "Only applies when custom filters are enabled. If nothing is checked, no events are sent on this channel.",
				Default:     "", Choices: eventChoices,
			},
			Spec{
				Key: p + "activities", Kind: KindString, Category: CatNotify, Multi: true,
				Label:       ch.label + " — orchestration decisions",
				Description: "Only applies when custom filters are enabled. If nothing is checked, none are sent on this channel.",
				Default:     "", Choices: activityChoices,
			},
			Spec{
				Key: p + "rate_limit_per_min", Kind: KindInt, Category: CatNotify,
				Label: ch.label + " — message limit per minute",
				Description: "Only applies when custom filters are enabled. 0 = no limit — use 0 on a channel that " +
					"triggers automation, so a burst of notifications doesn't drop the trigger. " +
					"⚠️ ALERTS never count towards this limit, on any channel: alerts exist to be annoying.",
				Default: "20", Min: f64(0), Max: f64(600),
			},
		)
	}
	return out
}

// catalog = base + the per-channel overrides. Built here (and not in an init) because the
// specByKey below is initialized as a package variable: an append in init() would run
// LATER and the generated keys would be left out of the index.
var catalog = append(append([]Spec{}, baseCatalog...), channelOverrideSpecs()...)

// specByKey indexes the catalog for O(1) lookup. Populated in init.
var specByKey = func() map[string]Spec {
	m := make(map[string]Spec, len(catalog))
	for _, s := range catalog {
		if _, dup := m[s.Key]; dup {
			panic("settings: duplicate key in catalog: " + s.Key)
		}
		// Ensures, during initialization, that every default is valid according to its own spec.
		if err := s.Validate(s.Default); err != nil {
			panic(fmt.Sprintf("settings: invalid default for %s: %v", s.Key, err))
		}
		m[s.Key] = s
	}
	return m
}()

// Specs returns the catalog in display order.
func Specs() []Spec { return catalog }

// Lookup returns the spec for a given key.
func Lookup(key string) (Spec, bool) {
	s, ok := specByKey[key]
	return s, ok
}

// NodeSpecs returns, in catalog order, the Agent/Engine behavior tunables
// (PerNode) — the set that the Control Plane delivers to each Node and that allows override
// per Node (ADR-039).
func NodeSpecs() []Spec {
	var out []Spec
	for _, s := range catalog {
		if s.PerNode {
			out = append(out, s)
		}
	}
	return out
}

// Same tells if a and b are the SAME value for this type, ignoring formatting: "10s" and "10000ms"
// are the same duration, "08" and "8" the same integer. It decides whether a value reported
// by the Node differs from the cluster one (seeding, ADR-039 §7) — comparing strings would seed a
// "different" override that changes nothing. A value that fails to parse is never equal to anything.
func (s Spec) Same(a, b string) bool {
	switch s.Kind {
	case KindBool:
		x, errA := strconv.ParseBool(a)
		y, errB := strconv.ParseBool(b)
		return errA == nil && errB == nil && x == y
	case KindString:
		return a == b
	default:
		x, errA := s.numeric(a)
		y, errB := s.numeric(b)
		return errA == nil && errB == nil && x == y
	}
}

// numeric converts a value to a float64 comparable with Min/Max. For duration, the
// result is in SECONDS.
func (s Spec) numeric(value string) (float64, error) {
	switch s.Kind {
	case KindDuration:
		d, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", value, err)
		}
		return d.Seconds(), nil
	case KindInt, KindInt64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid integer %q: %w", value, err)
		}
		return float64(n), nil
	case KindFloat:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number %q: %w", value, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("unknown type %q", s.Kind)
	}
}

// Validate checks if value is parseable for the spec type and respects Min/Max.
func (s Spec) Validate(value string) error {
	// Text has no magnitude: Min/Max do not apply and numeric() would reject any
	// value. Empty is legitimate — it's how a string tunable is "turned off" (see
	// agent.artifacts_url, ADR-027; and the channels from ADR-028).
	if s.Kind == KindString {
		if s.Multi {
			// CSV of codes: each token must be a known Choice (or "*" = all).
			// Empty = none, legitimate.
			for _, tok := range strings.Split(value, ",") {
				tok = strings.TrimSpace(tok)
				if tok == "" || tok == "*" {
					continue
				}
				known := false
				for _, ch := range s.Choices {
					if ch.Value == tok {
						known = true
						break
					}
				}
				if !known {
					return fmt.Errorf("unknown option %q", tok)
				}
			}
			return nil
		}
		if len(s.Options) > 0 && value != "" {
			if !slices.Contains(s.Options, value) {
				return fmt.Errorf("invalid value %q (accepted: %s)", value, strings.Join(s.Options, ", "))
			}
		}
		return nil
	}
	if s.Kind == KindBool {
		if value != "true" && value != "false" {
			return fmt.Errorf("invalid value %q (accepted: true, false)", value)
		}
		return nil
	}
	n, err := s.numeric(value)
	if err != nil {
		return err
	}
	if s.Min != nil && n < *s.Min && !(s.AllowZero && n == 0) {
		return fmt.Errorf("value %s below minimum (%v)", value, *s.Min)
	}
	if s.Max != nil && n > *s.Max {
		return fmt.Errorf("value %s above maximum (%v)", value, *s.Max)
	}
	return nil
}
