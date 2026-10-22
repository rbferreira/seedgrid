# **SeedGrid**
# **Business Rules**
Version: 1.0
Status: Draft
Document: 25 - Business Rules.md

# **1. Objective**

This document is the **business rules and tunables catalog** for the Control Plane: every
parameter that governs orchestration behavior, with its identifier, default,
where it is applied, and the rationale. It is the prose counterpart of the **code catalog**
(`control-plane/internal/settings/catalog.go`) — the single source of truth for tunables (ADR-023).

> **Single source of truth.** The default and semantics of each tunable live in the code catalog.
> This document **mirrors** that catalog; when changing a `Spec`, update it here in the same
> delivery (DoD §28). The **effective** value in production is the database override (table
> `settings`), editable in the Portal's **Settings** panel with hot-reload — the corresponding
> env var only seeds the database on the 1º boot.

# **1.1 Access Rules (ADR-035)**

Authenticated operators act according to their assigned role:

| Role | Can do | Cannot do |
|---|---|---|
| **Administrator** | Everything (wildcard), including user management | — |
| **Publisher** | Add/remove/migrate torrents, publish to the tracker, generate screenshots, manage trackers | Manage users, change settings, change cluster mode |
| **Downloader** | Explore, add torrents for own download, download (download-ticket) | Publish, remove torrents, manage trackers/users |
| **Viewer** | Read-only: dashboard, nodes, torrents, alerts, feed, metrics, health | Any mutation |

Rules:

1. **Changing own password** (`POST /auth/password`) requires only a session — every
   operator can change their own password.
2. **User management** requires `users:read`/`users:write` (Administrator).
3. **Last admin protection**: it is not possible to disable (status or delete)
   or remove the `Administrator` role from the last **active** administrator —
   soft-deleted admins in the history do not count for the protection.
4. **Disabled user** (soft delete or non-ACTIVE status) cannot authenticate and
   loses sessions — revocation applies to already issued tokens immediately.
5. Denied permission returns `403 forbidden` and is logged in the audit
   (ADR-025); the actor is the operator's username.
6. **Creating .torrent from local content** (`POST /torrents/create`) requires
   `torrents:build` — the `Downloader` adds torrents for their own download
   (`torrents:create`), and does not publish content.

# **2. Configuration Precedence (ADR-023)**

For each behavior tunable, the effective value is resolved as follows:

1. **Database override** (`settings.value`) — the editable source of truth (Portal panel).
2. **Catalog default** (code) — when there is no override.

The **env var** (*Seed env* column below) is only read on the 1º boot: if there is no row in the database
and the env is set and valid, `Store.Load` records it as the initial seed. After that, changing
the `.env` **does not** affect anything — the panel rules. Corrupted effective values fall back to the catalog
default in the getter (the Worker never breaks due to invalid config).

Infra/secrets (DB URL, tokens, admin password, API keys, prom URL, http addr, log level,
migration port) **are not** tunables: they remain only in env (`config.go`), immutable at runtime.

## 2.1 Agent and Engine Tunables: cluster value with Node override (ADR-039)

Tunables marked `PerNode` in the catalog (§3.1.1b) govern the **Agent and Engine of each Seed
Node**, not the Control Plane. For them, precedence has **four tiers**, and the Node env ceases
to be a source and becomes a **fallback**:

1. **Node override** (`node_settings`) — the deliberate deviation for a SINGLE machine.
2. **Cluster value** — the override from the `settings` table or, without it, the catalog default.
3. **Node env** (`/etc/seedgrid-agent.env`) — only valid when the Control Plane sends nothing (CP
   prior to ADR-039, unreachable CP, or the Agent on boot before the first heartbeat).
4. **Code default** of the Agent — when not even the env exists.

The Control Plane resolves tiers 1–2 and delivers the **effective** value for each Node in the
heartbeat response; the Agent hot-applies what has changed. Removing the override returns the Node to the
cluster value. Who can: cluster value = `settings:write`; Node override = `nodes:write`.

> **Why a cluster value alone would be the mistake.** The fleet is truly heterogeneous: at the
> same ~13 MB/s, qBittorrent uses 0,5–2,9% of CPU on an i5 and **50,7% on an Atom** (piece hashing).
> Applying `engine.max_active_downloads = 10` "to align" node-04 was exactly the mistake on
> 05/09/2026 (reverted to 3). The cluster value serves the majority; the exception is an explicit
> override, visible on the Node screen and in the Settings list, and audited.

# **3. Tunables Catalog**

## 3.1 Download Admission (`admission.*`) — ADR-021 / #21 / #22

Controller that keeps ≤ **L** torrents downloading across the entire cluster (the tracker limits DL
slots per account; the passkey is shared). Applied in
`control-plane/internal/admission/worker.go`.

| Rule (key) | Default | Unit | Seed env | Description / rationale |
|---|---|---|---|---|
| `admission.interval` | 20s | duration | `SEEDGRID_ADMISSION_INTERVAL` | Controller evaluation period. Shorter reacts faster; longer reduces load. |
| `admission.max_cluster_downloads` | 0 | integer | `SEEDGRID_MAX_CLUSTER_DOWNLOADS` | **Fallback** ceiling for downloads when no tracker has reported an L. 0 disables the fallback. |
| `admission.dl_slot_limit_ttl` | 6h | duration | `SEEDGRID_DL_SLOT_LIMIT_TTL` | Validity of an L observed in the tracker message before expiring. |
| `admission.min_dl_rate` | 51200 | bytes/s | `SEEDGRID_ADMISSION_MIN_DL_RATE` | Below this rate (sustained), a download yields the slot in the rate-based rotation. 0 disables it. |
| `admission.slow_grace` | 5m | duration | `SEEDGRID_ADMISSION_SLOW_GRACE` | Continuous time below the minimum rate before rotating the slow one. |
| `admission.slow_cooldown` | 10m | duration | `SEEDGRID_ADMISSION_SLOW_COOLDOWN` | How long the one rotated for slowness stays out of the promotion queue (anti-thrashing). |
| `admission.dead_grace` | 3m | duration | `SEEDGRID_ADMISSION_DEAD_GRACE` | 0 seeds **AND** 0 **connected** peers continuously for this time → dead torrent, yields the slot. 0 disables it. |
| `admission.dead_cooldown` | 30m | duration | `SEEDGRID_ADMISSION_DEAD_COOLDOWN` | How long the rotated dead one goes to the end of the queue — it does not return even with higher progress. |
| `admission.dead_backoff_max` | 4h | duration | `SEEDGRID_ADMISSION_DEAD_BACKOFF_MAX` | Backoff ceiling: each dead rotation **followed** by the same torrent doubles the cooldown (30m → 1h → 2h…) up to here. Connecting to any peer resets the count. 0 disables the backoff (fixed cooldown). |
| `admission.disk_min_free` | 5 GiB (5368709120) | bytes | `SEEDGRID_ADMISSION_DISK_MIN_FREE` | Free disk margin that is never committed on a Node (ADR-032). A torrent only enters (add via `.torrent`) or migrates to a Node if the **projected free space** can accommodate the **total size** of the torrent + this margin; below it the Node triggers `NODE_DISK_FULL`. 0 disables the block and the alert. |
| `admission.priority_persistent` | false | bool | `SEEDGRID_ADMISSION_PRIORITY_PERSISTENT` | Persistent mode for manual prioritization (F3). Enabled: a prioritized torrent (`prioritized_at`) becomes **immune** to dead/slow rotation — holds the slot until downloaded. Disabled (default): priority only **skips the queue**; once admitted, the torrent follows the normal flow and loses priority if rotation demotes it. |

> ⚠️ **Disk-aware admission (ADR-032).** The **projected free space** of a Node = `heartbeats.disk_free_bytes` − Σ (live torrents on the Node) `size_bytes × (1 − progress)` — the current free space minus what the already present torrents **will still write** until they complete. It is measured by the **total size** (the torrent downloads in full), without double-counting what is already on disk. There is a single formula (`control-plane/internal/capacity`), consumed at three points:
> - **Scheduler** (`Select`): excludes Nodes without space when adding via `.torrent` (known size); if none fit → `no_node_with_space` (409). **Add via magnet** has no size in the CP → does not block (only wins the Node with the most headroom in a tiebreaker).
> - **Migration** (`Migrate`): refuses destination without space → `dest_no_space` (409). Destination size or disk unknown → does not block (no way to judge).
> - **Alert** `NODE_DISK_FULL` (see §3.2): catches the Node **already** compromised, which the block alone does not undo.

> ⚠️ **Scheduler eligibility requires healthy storage (`storage_ok IS NOT FALSE`).** A Node whose Agent reports `/data` as unwritable — or unmounted, when the Node **declared a dedicated disk** (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`, the default) — **receives no work**, even if ONLINE, with reachable Engine and high health. Reason: without `/data` mounted, the Engine writes to the **system** disk, and `heartbeats.disk_total_bytes` starts measuring the rootfs — the Node appears to have space, passes the disk filter (ADR-032), and downloads until it fills the root. The filter is symmetrical to `engine_reachable IS NOT FALSE`: `NULL` (Agent not reporting the field) remains eligible, only `false` excludes. Motivating case: node-04 on 05/09/2026 received two torrents with `/data` still on the 15 GB eMMC, while the CP already recorded `storage_ok = false` and only used it for health score, alert, and metrics. This filter is one of the protection layers against missing, delayed, or remounted data disks — the full chain (mounting by UUID, Agent check, alert, reattachment by udev, mount propagation to the Engine) is in `docs/06 - Storage.md` §35. A **single-disk** Node (declared `false`) has no disk that can be missing: the Agent only checks for write access, it reports `storage_ok = true` and is normally eligible (§35.3) — the space, in this case, is the system disk's and is monitored by the space filter and `NODE_DISK_FULL`.

> ⚠️ **"No peers" is measured by CONNECTED peers, not the swarm.** `torrents.num_seeds`/`num_leechers` track the **swarm** (tracker scrape — what the Portal displays); `num_seeds_connected`/`num_leechers_connected` (migration 00030) track who is **actually connected**. The dead rotation uses `COALESCE(num_seeds_connected, num_seeds, 0)` — the COALESCE falls back to the swarm when the Agent predates the field (NULL), preserving old behavior. Rationale: the scrape might announce a **ghost seeder** (registered, but never responds). Judged by the swarm, this torrent looks alive, is never marked as dead, and since it is "viable" and usually has high progress, it returns to the **top** of the queue on every cooldown — burning an L ceiling slot forever without downloading 1 byte. Real case that motivated the refinement: `Blade.Runner.2049` on node-02, 98,79% and **0 connections for 5 days**, cycling resume→hold every ~15 min (jul/2026).
>
> The **promotion queue** (`promotableHolds`) continues to judge viability by the **swarm**, on purpose: it sorts torrents in **hold** (paused), and paused has 0 connected by definition — using connected there would zero out the peer-aware criterion and the queue would revert to being sorted by pure progress (the bug that ADR-021 fixed). For the chronic repeater, it is the **backoff** of `dead_cooldown` that takes action, not the sorting.

> ⚠️ **Manually pausing a download DOES NOT remove it from the queue — it depends on the Node mode (ADR-024).**
>
> | Node Mode | What happens with the operator's pause |
> |---|---|
> | **SG** (automatic) | The system has priority: the pause is **adopted** as an admission-hold (`adoptOrphanPauses`) and the torrent returns to the queue, being promoted when a slot opens. It appears in the Feed as a hold with the reason "manual pause adoption (SG Node)". |
> | **Manual** | The operator is sovereign: the pause stands until they resume **or** until the Node returns to SG — at which point the system readopts what was stopped. |
>
> To truly hold a download, set the **Node to Manual** (Dashboard → "Mode" column); pausing on an SG Node only frees up the slot temporarily. **A completed paused torrent (`progress >= 1`) is never adopted** — it is a seeder that the operator stopped, not a download in the queue.
>
> Rationale: without adoption, manual pause was a **point of no return** from orchestration — the torrent does not count in `countActive` nor enters `promotableHolds`, becoming invisible to the controller. The symptom is the cluster running below L with the queue appearing stuck and the Feed showing no admission decisions.
>
> **A Node that joins with its own qBittorrent starts in Manual (ADR-043).** Its torrents show in the Cluster as they are, and the controller does not pause, start or rotate any of them until the operator moves the Node to SG. Manual only exists under Flex: if the Cluster is in Full auto when such a Node registers, the gate moves to Flex (Event `NODE_ADOPTED_MANUAL`, with `gate_moved`), and the other Nodes stay SG — the Cluster behaves as before. Its active downloads count towards L, like any Manual Node's. Registering the same Node again (a lost credential) does not change the mode the operator chose afterwards.

> ⚡ **Manual queue prioritization (F3) — operator override over the peer-aware order.** The operator prioritizes a torrent via the ⚡ button on the Dashboard (`POST /torrents/{id}/prioritize`; remove = `DELETE`). It marks `torrents.prioritized_at` — state only in the Control Plane, **does not** queue a command on the Node; the controller applies the effect on the next pass. Two effects:
> - **Queue jumping:** in `promotableHolds`, `prioritized_at IS NOT NULL` is the **first** sorting criterion (before viable→progress) and **ignores the rotation cooldown** — the prioritized one is the next to be admitted when a slot opens. In the surplus hold (`hold`), it is the **last** to be held.
> - **Persistence (tunable `admission.priority_persistent`):** when enabled, the prioritized torrent is **excluded** from the dead/slow rotation — holds the slot until completion. When disabled (default), the rotation treats it normally and, upon **demoting it**, clears `prioritized_at` (the torrent "returns to normal").
>
> Priority is also cleared when the torrent **completes** (`clearCompletedPriority`: SEEDING/`progress >= 1`) — there is no longer a queue to jump. The Portal shows the prioritized torrent in **yellow**. ⚠️ In persistent mode, a prioritized torrent with no seeds **occupies a slot without downloading** until the operator removes it or seeds appear — it is a conscious override.

## 3.1.1 Agent (`agent.*`) — ADR-027

Agent update orchestrated by the CP (self-update via command, **manual** rollout).

| Rule (key) | Default | Unit | Seed Env | Description / rationale |
|---|---|---|---|---|
| `agent.artifacts_url` | *(empty)* | text | `SEEDGRID_AGENT_ARTIFACTS_URL` | Base URL where the Portal publishes the binaries and `agents.json` (e.g., `http://controller.example/node`). The CP reads the manifest from here; the **Agent downloads the binary from this same URL** → it must be reachable **by the Nodes** (internal Docker network addresses are not valid). **Empty disables** the feature: no badge in the Portal and the endpoint returns 503 `not_configured`. |

> **How the update happens:** the Portal compares `nodes.agent_version` against the manifest version and shows the button only when there is a binary published **for that Node's architecture** (`nodes.architecture`). The `update_agent` command takes `{version, url, sha256}`; the Agent downloads it, checks the **SHA-256**, runs the candidate with **`--version`** (smoke test: proves it executes on that host), saves the `.prev`, performs an atomic `rename`, **confirms to the CP**, and only then exits — systemd (`Restart=always`) brings it back up on the new binary.
>
> ⚠️ **One binary per architecture.** `/node/seedgrid-agent-amd64` and `-arm64`. The single path `/node/seedgrid-agent` (ARM64, legacy from RPi) **no longer exists**: with the fleet on amd64 it would serve the wrong binary — `bootstrap-node.sh` now resolves the arch via `uname -m`.
>
> **No automatic rollback via heartbeat** (out of scope for ADR-027): the smoke test prevents switching to a binary that does not execute, and the unit's `ExecStartPre` restores the `.prev` if the installed one breaks later. A binary that starts but fails at runtime requires logging into the Node and manually restoring the `.prev`.

## 3.1.1b Agent and Engine per Node (`agent.*`, `engine.*`) — ADR-039 / #144

**Cluster** value; each Node can have an override (§2.1). The **Env on the Node** is the Agent's fallback
(only valid without the Control Plane) — it does **not** seed anything in the Control Plane (the catalog has no `EnvVar`
for these keys); it is what the Agent reports during seeding (below). The **defaults** are what the
**majority of the fleet** used in the `.env` (inventory from 03/10/2026); the default in the Agent's **code**
(the fallback without env) remains the old one and **has not changed**. The contract with the Agent is
`shared/agent-config.json`, compared by tests on both sides.

| Rule (key) | Default | Range | Env on Node | Application in Agent | Description / rationale |
|---|---|---|---|---|---|
| `agent.heartbeat_interval` | 10s | 2s–60s | `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | recreates the ticker | Heartbeat period. Lower = the Portal and alerts see the Node sooner. Keep it well below `alerting.node_offline_after` (90s). |
| `agent.command_poll_interval` | 5s | 1s–60s | `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | recreates the Poller ticker | Latency from an operator action to the Node (ADR-014). |
| `agent.log_level` | info | debug/info/warn/error | `SEEDGRID_LOG_LEVEL` | `slog.LevelVar` | Agent log verbosity. A per-Node override prevents polluting the fleet's log to debug a single machine. |
| `engine.queueing_enabled` | true | bool | `SEEDGRID_ENGINE_QUEUEING_ENABLED` | `setPreferences` (queue) | Engine queue: the protection against I/O saturation. If disabled, the limits below no longer apply. |
| `engine.max_active_downloads` | 10 | 1–200 | `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | `setPreferences` (queue) | Simultaneous downloads per Node. **This is the parameter that differs per machine**: the Atom on node-04 handles 3. Belt-and-suspenders for the admission controller (ADR-021), which guarantees the tracker's L in the cluster. |
| `engine.max_active_uploads` | 10000 | 1–100000 | `SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS` | `setPreferences` (queue) | Practically unlimited **on purpose**: a queued seeder sends `event=stopped` to the tracker and gets a Hit and Run. |
| `engine.max_active_torrents` | 10000 | 1–100000 | `SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS` | `setPreferences` (queue) | Same: only downloads and checks should be queued. |
| `engine.max_active_checking` | 1 | 1–10 | `SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING` | `setPreferences` (queue) | Simultaneous (hash) checks: each one reads the entire torrent from disk; more than one saturates a USB drive. |
| `engine.dont_count_slow` | false | bool | `SEEDGRID_ENGINE_DONT_COUNT_SLOW` | `setPreferences` (queue) | `false` **on purpose**: if enabled, a stalled download does not count towards the limit, the Engine activates all of them, they all announce at once, and the tracker refuses slots (thundering herd). |
| `engine.slow_dl_threshold` | 1 KB/s | 0–1000000 | `SEEDGRID_ENGINE_SLOW_DL_THRESHOLD` | `setPreferences` (queue) | Only with `dont_count_slow` enabled. |
| `engine.slow_ul_threshold` | 1 KB/s | 0–1000000 | `SEEDGRID_ENGINE_SLOW_UL_THRESHOLD` | `setPreferences` (queue) | Only with `dont_count_slow` enabled. |
| `engine.slow_inactive_timer` | 300 s | 1–86400 | `SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER` | `setPreferences` (queue) | Only with `dont_count_slow` enabled. |

**No behavior requires a restart.** Applying `engine.*` sends **only the queue keys**: the
BitTorrent port and `save_path` belong to the machine (they match the port forward and the mounted disk) and are only
set on the process's first application. If the Engine is down, the application is repeated
on every heartbeat until it succeeds.

**Validation on both sides.** The `Spec` above is the operator's rule (the Control Plane refuses the value at the
source: `400 invalid_value`). The Agent still validates what it receives — an interval ≤ 0 would crash a
ticker, a download limit ≤ 0 is not "unlimited" in qBittorrent — and refuses the bad value while keeping the
previous one; a key it does not recognize (newer CP) is ignored.

**"Up to date" seal.** The Agent returns in the heartbeat the `version` (hash of the values) it applied **with
success**; the Portal compares it with the actual one: **Up to date**, **Waiting for Agent** (changed and has not
arrived yet) or **Unsupported Agent** (prior to ADR-039, never reported; remains in `.env`).

**Seeding overrides (migration, ADR-039 §7).** A running Node has values in `.env`; a
new Agent that only obeyed the Cluster would change them on the first heartbeat. Therefore the Control Plane
**seeds once per Node** (`nodes.agent_config_seeded_at`), from the `.env` the Agent reports,
**only** when all are true: (1) it is the Node's first report; (2) the Agent reported the key because the
operator **defined** it in `.env` (code default doesn't count); (3) there is **no Node override**
for the key; (4) the Cluster value for the key **was not defined by the operator** (it is still the default);
(5) the value is **valid** according to the `Spec` and **differs** from the Cluster one. Each seeding generates the Event
`NODE_CONFIG_SEEDED`. Until seeding has happened the Agent **applies nothing** (the Cluster
value could overwhelm precisely the Node that had its own value). It is a safety net — the
recommended procedure remains creating the overrides **before** updating each Agent
(`19 - Deployment.md` §50.5).

## 3.1.2 Notifications / telemetry (`notify.*`) — ADR-028

Sends Alerts/Events/orchestration outside the Portal (Telegram, Discord, generic webhook). Applied in `control-plane/internal/notify`, triggered at points that already generate an Alert (ADR-008), Event (ADR-006) and Activity (ADR-022).

| Rule (key) | Default | Type | Description / rationale |
|---|---|---|---|
| `notify.telegram_bot_token` | *(empty)* | **secret** text | Bot token (@BotFather). **Never returned in the read API** (only `has_value`); empty PUT = keep; "Restore default" = clear/disable. |
| `notify.telegram_chat_id` | *(empty)* | text | Destination (user/group/channel). The channel only sends with token **AND** chat_id. |
| `notify.discord_webhook_url` | *(empty)* | **secret** text | Channel webhook. Filling it enables it; it is never displayed back. |
| `notify.webhook_url` | *(empty)* | **secret** text | Raw JSON POST (`{kind,severity,title,message,node,fields,at}`) — Slack/n8n/HA without new code. |
| `notify.alerts_min_severity` | `WARNING` | enum `OFF/CRITICAL/WARNING/INFO` | Severity floor for notified Alerts. OFF disables the track without affecting the channels. |
| `notify.alerts_on_resolve` | `true` | bool | Also notifies when the Alert **resolves** (Node is back, storage recovered). |
| `notify.events` | *(empty)* | **multi** (checkboxes) | Events to notify. Persisted as a CSV of codes, but the panel shows **boxes with PT-BR labels** (the operator does not type `TORRENT_RECHECK`). Options = `events.NotifiableCodes()`; empty = none. |
| `notify.activities` | *(empty)* | **multi** (checkboxes) | Orchestration decisions to notify. Same here, friendly labels; options = `activity.AllKinds()`. ⚠️ High volume. |
| `notify.rate_limit_per_min` | `20` | integer | Anti-flood ceiling **for Events and Activities**. Excess is discarded, counted in the log and in the `seedgrid_notify_suppressed_total` metric. 0 = no limit. ⚠️ **Alerts do not count towards this ceiling** (see below). |

**Filters per channel.** Each channel repeats the same set of keys, prefixed by `notify.<canal>.` (`telegram`, `discord`, `webhook`): `custom`, `alerts_min_severity`, `alerts_on_resolve`, `events`, `activities`, `rate_limit_per_min`. The **`custom`** toggle (default `false`) decides whether the channel uses its own filters or the global ones — a channel without it behaves exactly as before routing existed.

It exists because, without it, **every configured channel receives everything** that passes the Cluster filter: it was impossible to send only the IPv6 change to the webhook that triggers an automation without the endpoint also receiving everything the Cluster notifies. `custom` is an explicit toggle, not "empty = inherits", because empty **already has a meaning** in the tracks (empty list = no Events).

> ⚠️ **ALERTS NEVER count towards the per-minute ceiling, in any channel.** Alerts exist to be annoying; a limiter that **discards** might eat precisely the warning that mattered, and which one dies is arbitrary (whichever arrives after the quota bursts). Previously, the ceiling was a single window shared by all tracks: a burst of "download completed" could swallow a `CRITICAL`, and the discard only appeared in a `logger.Warn` inside the container log.
>
> The remedy for an Alert storm is something else, and it already partially exists: **dedup in the database** (index `uk_alerts_open_condition`, `code` + `node_id` while `OPEN`) prevents re-notifying while the Alert is open, and **a noisy rule is fixed in the rule itself** — not in a limiter that would hide the symptom along with the rest. If protection is ever needed, let it be through **coalescence** ("N Alerts"), never discarding.

> **Secret in the catalog.** The operator's third-party credentials (bot token, webhook URL) live in the panel, with `Secret` masking them on read just as is already done with `trackers.api_token`. ADR-029 extended this to **all** third-party keys (including TMDB/IGDB/ptpimg — see §3.1.3); only the own deployment infrastructure remains in env. Rationale in ADR-028 (mechanism) and ADR-029 (scope).
>
> **Audit is left out** (these are the operator's own actions — notifying would be an echo). The value of a secret field is **not** audited (the fact of the change is audited). The "Send test" button (`POST /api/v1/notify/test`) proves delivery before the first incident. Best-effort: an external channel being down **never** breaks the flow that generated the fact.
>
> **Icon color (Telegram/Discord)** — 🟢 green = explicit success (`Severity: SUCCESS`: "seeding" Activity, `TORRENT_MIGRATED` and `NODE_REGISTERED` Events, and resolved Alert); 🔴 red = `CRITICAL`; 🟠 orange = `WARNING` (e.g., aborted torrent migration, full/saturated disk, Engine restart, `NODE_DELETED` — removing a Node is too serious to be blue); 🔵 blue = default (`INFO` or missing severity — never turns green by omission). The body is `Título` + the template `Fields` (`Rótulo: valor`, in order) + `Node: <hostname>` + `Data:` at the end (the `Node` comes **after** the `Fields`: right below the title the hostname was read as if it were the file name) (skipped when a Field already covers it, e.g., "Completed at"). `Node` is always the **hostname**, resolved by a simple query (`SELECT hostname FROM nodes WHERE id=$1`) at the emission point — the same pattern the Alert Worker already used — never the raw Node UUID.
>
> **Every notifiable Alert/Event/Activity has a structured template** in `notify/hooks.go` (`applyAlertTemplate`/`applyEventTemplate`/`applyActivityTemplate`) — not just a handful of examples. Activities: **seeding** (Name/Size/Added at/Completed at, success); admission_hold/scheduled get Reason (and scheduled also gets Score/Candidates); downloading/admission_resume get at least File. Events: TORRENT_CREATED/PAUSED/RESUMED/RECHECK/RESEEDED/PUBLISHED_EXTERNAL, the three migration phases (STARTED/MIGRATED with Origin-Destination/FAILED with Reason), TORRENT_REMOVED (Author: `"SeedGrid"` in automatic reconciliation, operator's username in manual removal via the Portal), NODE_REGISTERED/DELETED, ENGINE_RESTART, AGENT_UPDATE and STORAGE_RECOVERY — all with File/Node/specific fields instead of a single sentence. Alerts: NODE_STORAGE (Detail), NODE_DISK_FULL (Missing/Minimum floor), DISK_IO_SATURATED (Utilization) and STORAGE_RECOVERY_FAILED (Step/Attempt) add fields to the standard body (NODE_OFFLINE doesn't need to — the Title already says it all).

## 3.1.3 Third-party integrations (`integrations.*`) — ADR-029

External service keys tied to the **operator's account**. They are kept in the catalog (not in env) so the project can be open-sourced without the maintainer's personal key. All of them use seed-from-env on the 1st boot (backward compatible) and masking on read.

| Rule (key) | Default | Type | Description |
|---|---|---|---|
| `integrations.tmdb_api_key` | *(empty)* | **secret** text | TMDB v3 key (movie/series metadata, ADR-020). Empty disables enrichment. |
| `integrations.igdb_client_id` | *(empty)* | text | Twitch app Client ID for IGDB (games, #17). |
| `integrations.igdb_client_secret` | *(empty)* | **secret** text | Client Secret for the same app. Without the pair, games are disabled. |
| `integrations.imgbb_api_key` | *(empty)* | **secret** text | imgbb.com key (screenshot host, ADR-030 — replaces ptpimg, which went offline). Free at api.imgbb.com. Empty disables screenshots. |

> **Why they were moved out of env (ADR-029).** They were read from `SEEDGRID_TMDB_API_KEY` etc. on boot (ADR-026, "system secret"). But these keys belong to the **personal account** of the operator — open-sourcing the repository with them in env/example leaks credentials. Now they live in the panel, editable with hot-reload (clients re-read the key per call; emptying it disables it immediately). Automatic migration: the current `.env` seeds the database on the 1st boot and is then ignored. **What remains in env** is only the infrastructure of the deployment itself (`POSTGRES_*`, `SEEDGRID_ADMIN_*`, `SEEDGRID_AGENT_REGISTRATION_TOKEN`) — owned by the operator of the instance, with no external owner.

## 3.2 Alerts / Monitoring (`alerting.*`) — ADR-008

Alert Worker: evaluates deterministic rules in the background. Implemented in
`control-plane/internal/alert/worker.go`.

| Rule (key) | Default | Unit | Seed env | Description / rationale |
|---|---|---|---|---|
| `alerting.worker_interval` | 30s | duration | `SEEDGRID_ALERT_WORKER_INTERVAL` | Interval between alert rule evaluations. |
| `alerting.node_offline_after` | 90s | duration | `SEEDGRID_NODE_OFFLINE_AFTER` | Time without heartbeat for a Seed Node to be marked OFFLINE (triggers `NODE_OFFLINE`). |
| `alerting.disk_saturated_pct` | 90 | % | `SEEDGRID_DISK_IO_SATURATED_PCT` | I/O utilization at which the disk is considered saturated (`DISK_IO_SATURATED`); 15pp hysteresis on resolution. 0 disables. |
| `alerting.disk_saturated_for` | 90s | duration | `SEEDGRID_DISK_IO_SATURATED_FOR` | How long the disk must stay **continuously** above `disk_saturated_pct` before opening `DISK_IO_SATURATED` (#150). A single Alert Worker pass below the threshold resets the count; the alert fires on the first pass after the time is fulfilled (every 30s by default). Resolution has no grace period. The count is kept in memory and restarts if the Controller restarts. 0 fires on the first read, as before. Max. 1h. |
| `alerting.firewalled_grace` | 30m | duration | `SEEDGRID_FIREWALLED_GRACE` | Time a Node can go without receiving **any** inbound connections before opening `NODE_FIREWALLED`. Every Node starts as `firewalled` until the first peer arrives, so alerting immediately would cause an alarm on every Engine restart. 0 disables the rule. |
| `alerting.incoming_silence` | 240h | duration | `SEEDGRID_INCOMING_SILENCE` | Time without **any** inbound connections, **provided there is demand** (defined by the tunable below), before opening `NODE_INBOUND_SILENT`. Unlike `firewalled_grace`, it uses a signal that **decays**. Inbound connections are a rare event in an idle collection: a Node with no port issues goes 2 to 9 days without receiving any, and a low value causes false alarms (the default was 12h; calibrated in `16 - Alerting.md §48.2`). The value saved in the database overrides the default. 0 disables the rule. |
| `alerting.incoming_demand_seeds_per_leecher` | 1 | seeds/leecher | — | How much concurrency a swarm can have and still count as **demand** for `NODE_INBOUND_SILENT` (#206): the torrent counts when `num_leechers > 0` **and** `num_seeds <= num_leechers × valor`. In a plentiful swarm, a leecher connects to half a dozen seeds, so not being one of them says nothing about the port. `1` = leechers at least as numerous as the seeds; `0` = only swarms with no seeds (**not** "disabled"); a very high value (e.g., 100000) goes back to counting any leecher. `num_seeds` or `num_leechers` NULL = unknown, does not count. Accepts decimals (e.g., 2.5). |
| `alerting.ipv6_change_grace` | 15m | duration | `SEEDGRID_IPV6_CHANGE_GRACE` | How long the new IPv6 must hold before opening `NODE_IPV6_CHANGED`. Privacy extensions rotate the suffix on their own; without a grace period, a misconfigured Node would alert on every ephemeral change. 0 disables the rule **and closes any open alerts for it** (#204). |
| `alerting.reachability_ttl` | 24h | duration | `SEEDGRID_REACHABILITY_TTL` | **Validity** of `NODE_FIREWALLED`, `NODE_INBOUND_SILENT` and `NODE_IPV6_CHANGED` (#211). All three infer reachability from the absence of inbound connections and only resolve when one arrives — which a Node without demand never receives, even with the port open. Counts from the **last notification**; upon expiration, closes **without notifying** and the same episode does not reopen. 0 = no validity. |
| `alerting.sample_retention` | 168h | duration | `SEEDGRID_SAMPLE_RETENTION` | Retention of time series samples (`torrent_stat_samples`) before pruning. 0 disables purging. |

**Alerts emitted by the Alert Worker** (deduplicated by `(code, node_id)`, automatically resolved; the three reachability ones also **expire** — see the note for #211 below):

| Code | Severity | Triggers when | Resolves when |
|---|---|---|---|
| `NODE_OFFLINE` | CRITICAL | Node has been ONLINE without a heartbeat for more than `node_offline_after` | Node starts reporting heartbeats again |
| `NODE_STORAGE` | CRITICAL | Agent reports `/data` as unwritable, or — with a declared dedicated disk — as unmounted (`storage_ok=false`) | `storage_ok` returns to true/NULL or Node leaves ONLINE |
| `DISK_IO_SATURATED` | WARNING | Disk I/O utilization ≥ `disk_saturated_pct` **continuously** for `alerting.disk_saturated_for` (#150) | Drops below the margin (15pp hysteresis) or Node leaves ONLINE |
| `NODE_DISK_FULL` | WARNING | Projected free space < `admission.disk_min_free` (ADR-032) — the Node has committed to more torrents than the disk can complete | Returns above the margin, loses disk data, or Node leaves ONLINE |
| `NODE_FIREWALLED` | WARNING | Engine reports `firewalled` (no INBOUND connections accepted) for more than `alerting.firewalled_grace` | Engine starts reporting `connected` again, the verdict becomes unknown, or Node leaves ONLINE |
| `NODE_INBOUND_SILENT` | WARNING | No inbound connections for more than `alerting.incoming_silence` **and** at least one of the Node's torrents has weighted demand: `num_leechers > 0` and `num_seeds <= num_leechers × alerting.incoming_demand_seeds_per_leecher` (#206) | Starts receiving inbound connections again, or Node leaves ONLINE |
| `NODE_IPV6_CHANGED` | WARNING | `public_ipv6` changed from one address to another, the new one has held for more than `alerting.ipv6_change_grace` **and** the Node still has not received an inbound connection since the change | Node starts receiving inbound connections again **after** the change (`last_incoming_at > public_ipv6_changed_at`), or leaves ONLINE |

> ⚠️ **`DISK_IO_SATURATED` requires sustained saturation (#150).** The reading (`nodes.disk_io_util_pct`) is the ~10s average between two heartbeats, and the rule used to trigger on just **one** of them. On a USB drive, which is spiky by nature (87% in one interval, 13% five minutes later — node-04, 06/09/2026), a seconds-long spike would trigger an alert: of the 18 opened between 12/09 and 02/10/2026, **16 lasted exactly 30s** (one Worker pass); the other two lasted 9min and 4m30, the kind of episode the rule still catches if saturation is sustained. The Portal graph uses a 2-minute window and never showed these spikes, so the alert looked like a false positive. Now the Node must be ≥ threshold on **all** passes during `alerting.disk_saturated_for` (90s by default); any pass below — or with the Node out of ONLINE/without a recent heartbeat — resets the count. Behavior notes:
>
> - **The granularity is that of the Alert Worker** (`alerting.worker_interval`, 30s): with the defaults, it triggers on the 4th consecutive reading (0, 30, 60, 90s). The comparison has a 1/10 interval margin so that millisecond jitter from the passes doesn't delay the trigger by a full pass when the grace period is a multiple of the interval.
> - **Resolution has not changed**: no grace period, with 15pp hysteresis. A reading between the margin and the threshold keeps the alert open, but does not count as saturated; reopening after resolution requires a completely new sequence.
> - **The start of the sequence lives in memory** in the Worker (`satSince`), with no migration: it is derived state, and a Controller restart only **delays** the alert by up to the grace period — it never creates a false alarm. An already open alert is not affected (it is in the database). A grace period of 0 keeps the old behavior.
> - **Out of scope, discarded:** persisting `disk_io_util_pct` in `heartbeats` (the history of the series that triggers the alert) and showing this series on the Node card. The cause of the noise was triggering on a single reading, which the sustained rule fixes; the history would be a separate feature.

> ⚠️ **Reachability alerts expire (#211).** `NODE_FIREWALLED`, `NODE_INBOUND_SILENT`, and `NODE_IPV6_CHANGED` are **inferences**: they deduce that the Node is unreachable because no inbound connection has arrived, and only resolve when one does. On a Node with no demand, one never arrives, whether the port is open or not — measured on 24/09/2026, node-01 with 0 leechers across 116 torrents, two of these alerts open and the port verified **from the outside** as open. What matters is the **outbound call** on trigger (webhook, Telegram, Discord); after `alerting.reachability_ttl`, the alert closes itself. Three design rules:
>
> - **Closes without notifying.** "Resolved: ..." would be a lie — nothing was fixed. The alert gets `expired = true` and the Portal shows *expired*.
> - **The same episode does not reopen.** The condition remains true after expiring; without this, the Worker would reopen it on the next pass and notify on every cycle of the validity period. Each alert records the condition's identity in `episode_key`: the rotation (`public_ipv6_changed_at`), the start of the silence (`last_incoming_at`), the verdict (`connectable_since`). This also applies to **reboots**: leaving ONLINE closes the alert, and returning finds the recorded episode instead of re-notifying an old rotation. A new rotation, an inbound connection followed by another silence, or an Engine restart change the key and trigger alerts again.
> - **Counts from the last notification** (`updated_at`), not from the trigger: a new rotation with an open alert (#209) notifies again and renews the validity period.
>
> The other rules do not expire: their resolution is the negation of the trigger condition based on a **measured** variable (heartbeat, `storage_ok`, disk), so an open alert for them is a condition that still exists. An external probe to confirm the port was considered and discarded as a prerequisite: a regular user will not spin up a system that talks to an integration endpoint.

> ⚠️ **`NODE_FIREWALLED` requires sustained `firewalled`, and the verdict is STICKY.** The signal starts false (every Node is `firewalled` until the first peer hits the port), hence the grace period; and once `connected`, it does not revert as long as the Engine does not restart — meaning the rule catches Nodes whose opening **never worked**, not Nodes that **stopped** being reachable. `NULL` is unknown and never alerts. The full rationale, the discarded alternatives (CP active probe, external probe, tracker API) and the incident that originated the rule are in **ADR-036**; the sticky signal gap is in issue **#155**.

> ⚠️ **`NODE_FIREWALLED` and `NODE_INBOUND_SILENT` are complementary, not redundant.** The first covers **onboarding** (port never opened: the Node starts `firewalled` and never leaves it — the sticky verdict works well here). The second covers **steady state** (a Node that WAS reachable and stopped being), which was the case that completely slipped through: measured on 06/09/2026, node-01 and node-03 reported `connected` with the Engine up for 1 month and 5 days, spanning the entire period they were demonstrably unreachable. The second rule reads the **age** of `last_incoming_at`, a timestamp that ages on its own.
>
> **Why it requires demand:** with no leechers in the swarm, 0 inbound connections is the *correct* state of a seeder. Measured on 07/09/2026, the four Nodes — all healthy — had 0, 1, 1, and 0 inbound connections at the time of the reading; alerting on pure absence would turn an idle inventory into a permanent alarm.
>
> **Why ponder demand (#206):** "there is a leecher" is not demand for *this* Node. A leecher in a swarm with 63 seeds connects to half a dozen of them, and going hours without being one of the chosen ones is the expected behavior of a healthy Node. Measured on 09/09/2026, the six torrents that armed the alert on Nodes with the externally verified and open port had 2 to 63 seeds per leecher; of the 27 alerts opened between 08/09 and 30/09, only the first has a documented real issue. The rule asks *"would someone probably need me?"*, not *"does someone want this content?"*. A torrent only counts when `num_seeds <= num_leechers × alerting.incoming_demand_seeds_per_leecher` (default 1):
>
> | Swarm (leechers / seeds) | Counts with the default? |
> |---|---|
> | 1 / 0 | **yes** — the leecher has no one to download from |
> | 1 / 1 | **yes** — the only seed is probably the Node itself |
> | 3 / 3 | **yes** — equality counts (the rule is `<=`) |
> | 1 / 2, 3 / 17, 1 / 63 | no |
> | `num_leechers` or `num_seeds` NULL | no — unknown never triggers |
>
> `num_seeds` is the swarm total according to the tracker and may already include the Node itself. The demand is for the **Node**, not the torrent: just **one** needy swarm among its own is enough. The alert carries, in the payload, only what armed the rule (`demand_torrents`, `demand_leechers`, `demand_seeds`) — it is the only trace of *why* it opened, since there is no peer history in the database.
>
> **Text that does not age (#203).** The `NODE_INBOUND_SILENT` message is fixed and does not include address or duration: it is recorded once, at opening, and the alert can stay open for days — the Node's address changed almost 38h after opening and the panel kept telling to open an address that no longer existed. What varies stays in the payload (`public_ipv6` and `bt_port` **from the opening**, `last_incoming_at`, `hostname`, the demand). The **Portal** shows the Node's **current** address next to the open alert, and the **notification** (Telegram/Discord/webhook) carries the one from the payload as the "Node Address" field — it is an instant, so the one from the opening is correct there.
>
> ⚠️ **`NODE_IPV6_CHANGED` is an ALERT, not an Event.** The change itself is a point-in-time fact, but the CONSEQUENCE — "the firewall rules are outdated" — is a condition that persists until someone acts. The resolve is only possible because of the decaying signal from #155: the right question is not "did the address stop changing" but "did the firewall start letting traffic in again", and it is a `last_incoming_at` posterior to the change that answers this. A Node's first address (NULL → value) is **discovery**, not rotation, and never alerts.
>
> ⚠️ **Subsequent rotation with the alert still `OPEN` UPDATES, it is not swallowed (#209).** With no demand in the swarm, a Node might not receive inbound traffic for days, and the original alert stays `OPEN` waiting for the resolve. If the IPv6 rotates again in this interval, the dedup by `(code, node_id)` in `Open()` would silently discard the detection — the n8n webhook would never know about the newest address, and would keep reacting forever to an address that is no longer the problem. `evaluateIPv6Changed` covers this by calling `Service.UpdatePayload`, which rewrites the message/payload of the SAME row (preserving `opened_at`) and notifies again via the opening channel — never the resolution one, because nothing was fixed.
>
>
> ⚠️ **The opening carries the negation of the resolve, and this is not redundancy (#204).** Nothing
clears `previous_public_ipv6`, so "the address changed" is true **forever** after
a change — unlike all other rules in this worker, whose resolve is the
negation of the opening on the same variable. Without the `AND` that excludes those that already received
inbound traffic, a Node that becomes reachable again satisfies opening and resolve in the SAME
pass: since the opening dedup only applies while the alert is OPEN, it reopens
on every worker cycle and notifies on all of them. Measured on 09/09/2026: one notification every
30s per Node.

> **Machine contract (`schema: 1`).** Only this alert carries a `data` block in the webhook channel, because it exists to trigger automation. `fields` is for humans to read and changes when someone improves a sentence; `data` is stable. `data.ipv6.prefix_previous`/`prefix_current` come **calculated** by the Control Plane: the firewall rule needs the `/64` network object, and it was precisely the literal address that caused the incident — letting the consumer derive it via string manipulation would plant the same bug on the other side. `schema` only goes up on breaking changes.

> **Known limitation:** demand is evaluated at the *instant* of the pass, not accumulated over the window — there is no leecher history in the database (`torrent_stat_samples` stores bytes and speed, not peers). A Node with no demand at the time of evaluation does not alert, even if there was demand during the window: it is better to stay silent than to cry wolf. Concurrency weighting reduces the noise from this limitation, but does not eliminate it: a leecher that just arrived at a needy swarm still triggers a rule whose window has already expired. `last_incoming_at` NULL (we have never seen an incoming connection) also does not alert, because it does not distinguish "recently deployed" from "always unreachable" — this is the onboarding case, from the previous rule.

> `DISK_LOW`/`SMART_CRITICAL` (disk/SMART inventory) are left for the F1 backlog — hardware ≠ lack of space; `NODE_DISK_FULL` covers the lack of space with the data the heartbeat already brings.

## 3.3 Internal Workers (`workers.*`)

Intervals previously hardcoded in `main.go`, now tunable (read by a *provider* every cycle).

| Rule (key) | Default | Unit | Seed env | Description / rationale |
|---|---|---|---|---|
| `workers.reconciler_interval` | 15s | duration | — | Period of the Migration Reconciler (ADR-018) that advances/finalizes in-flight migrations **and promotes the queue** (ADR-031). |
| `workers.prom_writer_interval` | 30s | duration | — | Rewrite period for the Prometheus targets file (file_sd) (ADR-017). |
| `migration.max_concurrent` | 1 | integer | — | How many migrations transfer data **at the same time** (ADR-031); the rest remain `QUEUED`. **Cluster-wide** ceiling — the protected resource is disk I/O, and an unlocked drain triggers N simultaneous copies right on the Node in trouble. |
| `migration.recheck_stall_window` | 10m | duration | — | How long the recheck on the target Node can go **without advancing** before the migration is aborted as stalled (#79/#87). This is the real protection: as long as progress goes up, the wait continues, no matter how slow the disk is (10 GB on the USB drive of node-03 take ~15 min to complete). The **Control Plane** is the one observing, on every reconciler pass — not the Agent. Minimum of 60s: there is no "turning off", without a watchdog a stalled recheck would occupy the `migration.max_concurrent` slot forever. |
| `migration.recheck_max_wait` | 6h | duration | — | Absolute ceiling, counted from the **end of the copy** (`DONE` of `migrate_pull`), for the target to prove it has what the source had (#87). It only catches an Engine that reports oscillating progress forever. It could go from 2h to 6h because the wait stopped holding the Node's command queue; the cost of a high ceiling is the `migration.max_concurrent` slot being occupied, not a deaf Node. |
| `metrics.cluster_sample_interval` | 5m | duration | `SEEDGRID_METRICS_CLUSTER_SAMPLE_INTERVAL` | Period between snapshots of the cluster's accumulated throughput (F2), recorded in `cluster_stat_samples` by the `cluster-stats` worker. Basis for the evolution chart in the Dashboard. |
| `metrics.cluster_sample_retention` | 0 (forever) | duration | `SEEDGRID_METRICS_CLUSTER_SAMPLE_RETENTION` | Retention of cluster snapshots. **0 = keep forever** (the table is tiny, ~1 row/interval). For the **1 year** filter to work, keep ≥ 1 year or 0. |

> 📈 **Accumulated cluster throughput (F2).** The `cluster-stats` worker records, every `metrics.cluster_sample_interval`, the **sum of the cumulative totals** (`downloaded_total`/`uploaded_total`) of **all** torrents — including those removed via soft-delete — in `cluster_stat_samples`. Summing the removed ones keeps the curve **monotonic** ("accumulated growth"): a deleted torrent freezes its contribution instead of disappearing from the sum. Unlike `torrent_stat_samples` (per torrent, session bytes, 7d) and Prometheus (30d, no byte metric) — neither covers 6 months/1 year; hence the dedicated table with long retention. The `GET /api/v1/metrics/cluster-throughput?range=<dur>` endpoint reads the series with **downsampling via `date_bin`** (≤ ~400 points, bucket ≥ 1 min) and returns two series (Downloaded/Uploaded) in the format of the `Chart` component. The **Dashboard** shows the chart with period filters (1h … 1 year). ⚠️ Migration counts ~2× (soft-deleted source + target) — it is an aggregated trend, not an accounting one.

## 3.3.1 Torrent migration — non-parameterized rules (ADR-018/031)

Rules that live in the domain (`control-plane/internal/migration`, `agent/internal/commands`,
`agent/internal/transfer`) and are **not** tunable, indexed here as per §1.

| Rule | Where | Rationale |
|---|---|---|
| Source is only removed after the target confirms | `migration.complete()`, called only by `verifyAfterPull()` | Lossless migration (ADR-018): if the transfer fails, the torrent remains on the source. Since #87, "confirm" means the CP observing the target — the pull `DONE` alone is not enough (see below). |
| Source is **stopped before** serving the content — and without a confirmed pause it is not authorized | `commands.migrateServe()` → `pauseForMigration()` | Copying a torrent that the Engine is still writing to reads blocks halfway through: the tar comes out inconsistent and the target only finds out during the recheck. The pause comes **before** `Authorize` (without a token, the target gets a 401 and there is no way to pull in-flight data); if the Engine does not confirm the stopped state within 10s, the command fails — an aborted migration is recoverable, corrupted data is not. |
| Torrent with **in-flight migration** falls out of admission reach | `admission.notMigratingPred` (in `hold`/`promotableHolds`/`adoptOrphanPauses`/`rotateDead`/`rotateSlow`) | The migration pause looks like an "orphaned manual pause" to the Admission Worker, which adopted it and **promoted it in the next cycle** — resuming the download in the middle of the copy, exactly the in-flight data that the pause prevents (observed live on 07/08/2026: pause at 06:27:41, admission resume 15s later). While the migration is `QUEUED`/`PENDING`, the migration orchestrator is in charge of that torrent; once finished (COMPLETED/FAILED), it becomes a normal candidate again. `countActive` **does not** apply the filter: if the torrent is still announcing, it occupies a real slot on the tracker and must count towards the L. |
| Aborted migration **resumes** the torrent on the source | `migration.resumeSource()` (called by `fail()`) | Complement to the rule above: "remains on the source" is only true if it starts running again, otherwise an unsuccessful drain leaves torrents silently stopped. It does not resume a torrent in `admission_paused_at` (the Admission Worker is in charge of the queue, ADR-021/024) nor a migration that was not even promoted (without `serve_command_id`, nothing was paused). |
| Success criteria = **source progress**, not "is seeding" | `migration.judgeRecheck()` + `min_progress` in the payload (in the Agent, only the legacy path: `commands.waitProgress()`) | ADR-031: a 60% torrent migrates at 60% and keeps downloading on the target. Requiring SEEDING failed every partial migration, which is exactly the case of draining a Node out of space. |
| 0,5% tolerance on the progress floor | `migration.pieceTolerance` (mirrors `commands.pieceTolerance`, used in the legacy path) | The recheck invalidates the **boundary piece** when the download stopped in the middle of it; losing a piece on the edge is normal. |
| Missing `min_progress` ⇒ requires complete torrent | `commands.minProgressOf()` | Backward compatibility: new Agent with old CP falls back to the ADR-018 behavior. If this regressed to 0, an empty migration would pass as successful. |
| Empty content on the source ⇒ **409 before any header** | `transfer.handleTransfer()` + `isEmptyContent()` | Root cause of the 2026-07-18 incident: the 200 was written before the stat, and the target received a valid, empty tar — a "successful" pull without a single byte, followed by a full re-download from the swarm (burns ratio). |
| 409 on pull is **not** retried | `commands.pullWithRetry()` + `transfer.ErrNothingToTransfer` | Permanent condition; insisting for 90s only delays the error the operator needs to see. 401 remains retryable (normal race between the target's pull and the source's serve). |
| Token issued only on **promotion** | `migration.promoteOne()` | Its TTL (30min) would run during the queue wait and expire before the transfer starts. Since #87 the TTL only covers the **transfer**: with the new Agent the pull `DONE` comes at the end of the copy, and the recheck wait has its own deadline (below). |
| One in-flight migration per torrent covers `QUEUED`+`PENDING` | `torrent_migrations_one_inflight` index | Prevents two targets from competing for the same torrent, even while it waits for a slot. |
| **`DONE` of `migrate_pull` ≠ target verified** | `migration.Reconcile()` → `verifyAfterPull()` | #87: the new Agent responds `DONE` as soon as the copy finishes and the recheck is triggered, so as not to hold the Node's command queue (the Poller is serial). The CP observes the hash row on the **target Node** (fed by the inventory) and only concludes when it proves the floor; the old Agent, which only responds `DONE` after the recheck, falls into the same path and already finds the target ready. The CP **does not branch** by Agent version. |
| Only a target read **after `DONE`** is valid | `migration.judgeRecheck()` (`last_synced_at > finished_at` of the pull) | A row synced before might be from before the recheck (or from another copy). Without a new read, it waits — and the absence of a read counts as stagnation. |
| Only a state that **proves progress** proves the floor | `migration.provesProgress()` (allowlist: `SEEDING`/`DOWNLOADING`/`QUEUED`/`PAUSED`) | During the recheck (`CHECKING`) qBittorrent reports the progress of the **verification** itself as progress: a 60% partial with the recheck at 70% would "pass" the floor without having finished verifying. `MOVING`/`UNKNOWN`/`ERROR` also prove nothing — an allowlist, so that an unmapped state does not become proof by omission. |
| **Recheck watchdog lives in the CP**: stagnation + ceiling | `migration.judgeRecheck()`, `migration.recheck_stall_window` / `migration.recheck_max_wait` | #79 moved by #87. Stopped going up for `recheck_stall_window` → `FAILED` "recheck on target stalled"; exceeded `recheck_max_wait` since `DONE` → `FAILED` "target did not reach source progress". Same effect as before: `fail()` resumes the source and discards the target copy. In-memory state: restarting the CP only renews the stagnation window. |
| The Agent only returns `DONE` early if the CP declared **`async_recheck`** | `migration.promoteOne()` (payload) + `commands.migratePull()` | Additive field. Without it (CP prior to #87) the Agent keeps waiting and only responds `DONE` with the target verified — otherwise that CP would read `DONE` as "verified" and remove the source without proof. |
| In-flight migration **cancels itself**, via the same safe path as the watchdog | `migration.Cancel()` → `abort()` (`POST /api/v1/migrations/{id}/cancel`, `torrents:migrate`) | #87: the refusal above is only acceptable if the operator has an out — and the verification can last up to `migration.recheck_max_wait`. `FAILED` with `error = "cancelada pelo operador"` (no new state in the enum): source resumed, target copy discarded. `serve`/`pull` not yet claimed by the Agent are downloaded and never run (nothing to undo for them); during the copy the command is not interrupted and the cleanup is queued behind it. `409 migration_finished`/`migration_already_cancelled` if already finished; repeating does not re-execute anything. |
| Completing and canceling **never win together** | `complete()` (`WHERE status = 'PENDING'`) and `abort()` (`WHERE status IN ('QUEUED','PENDING')`) | Without the condition, a cancellation arriving at the instant the reconciler completes would overwrite `COMPLETED` and order the deletion of the ONLY copy (the target's), with the source already removed. The loser has no effect whatsoever (rollback / nothing queued). |
| Torrent in `PENDING` migration **refuses** operator action | `torrent.inMigration()` in `Command()`/`Remove()` → `409 migration_in_flight`; `migration.Migrate()` → `ErrAlreadyMigrating` | #87: without the Poller block, a pause/resume/recheck/remove would execute **in the middle** of the target's recheck. Applies to the migrated torrent and to the **copy of the same hash** that the target adopted. `QUEUED` does not lock (nothing has moved yet; a drain waits for hours). Complements the admission rule above, which only covered the source row. The 409 message says that the migration can be canceled and which one. |

## 3.4 Tracker publishing (`publish.*`) — ADR-020 / ADR-026

Parameters for the UNIT3D tracker registration flow. Applied in
`control-plane/internal/settings/catalog.go`.

| Rule (key) | Default | Unit | Seed env | Description / rationale |
|---|---|---|---|---|
| `publish.screenshot_count` | 4 | integer | `SEEDGRID_SCREENSHOT_COUNT` | Screenshots per upload — how many frames to extract from the video file (via Agent/ffmpeg) when generating screenshots for publishing (ADR-026). |

> **Screenshot BBCode (backlog F1).** Screenshots are added to the description in the standard private tracker format: `[b]Screenshots[/b]` header + `[center]` block with **clickable thumbnails** `[url=imagem-cheia][img]thumbnail[/img][/url]` (one per line) — consistent with the poster/sections of the rest of the release form and avoids giant inline images (a common cause of "postponed"). `imgbb` returns the thumbnail (`data.thumb.url`, with `medium`→full fallback). In the Portal, the **✨ Generate BBCode** button assembles the **full release form** in one click (poster + synopsis + tags + screenshots + MediaInfo) with a **step bar** showing each actual stage: extracting frames on the Node (`extract_screenshots` command with its own cycle, `POST .../screenshots/extract` → command_id, tracked by `GET /commands/{id}`), hosting on imgbb + assembling the BBCode (`POST /screenshots/render`). Screenshots are **best-effort** — if extraction fails (incomplete torrent, missing ffmpeg, missing imgbb key), the step turns amber and the release form comes without them + a warning, since screenshots do not block the upload. The standalone **📸 Generate screenshots** button follows the flow in one step (`POST .../screenshots`).

## 3.5 Tracker profile and taxonomy discovery — non-parameterized rules (ADR-037 / #196)

Code constants, not tunable: they are **input safety** limits and scan
shape, and loosening them is a code decision, not an operator preference.
Applied in `control-plane/internal/trackerprofile` and `internal/unit3d/discover.go`.

| Rule | Value | Where | Rationale |
|---|---|---|---|
| Discovery only **proposes**; applying is another operation | — | `discover-taxonomy` × `apply-taxonomy` | The catalog is third-party and incomplete: nothing becomes a profile without the operator checking it. Only applying is logged in Audit (ADR-025). |
| Discovery **never overwrites** a `manual` term, never removes, and never touches `kind` | — | `trackerprofile.Merge` (on the server, over the locked state) | Re-running must be safe. Checking only in the Portal would not be enough: the state might have changed between proposing and applying. |
| Term missing from the catalog remains **untouched** | — | `Propose`/`Merge` | Absence is not evidence — the catalog only shows what someone has posted. |
| Scanned pages | default **8**, ceiling **20** (50 items each) | `unit3d.DefaultDiscoverPages`/`MaxDiscoverPages` | ~1,3 s per page on samaritano (measured); the request quota per token belongs to the operator and is shared with the Portal. |
| Scan time budget | **20 s** | `unit3d.DiscoverBudget` | The HTTP server closes the response in 30 s: a slow scan finishes earlier, with a partial result, instead of being cut off with no response. |
| Rate limit (`429`) or deadline exceeded midway | returns the **partial** result (`stopped_reason`) | `unit3d.Discover` | Already collected pages remain valid. It only fails (`502`) if none arrived. |
| Term `id` | integer in `1..1000000`, unique per axis | `trackerprofile.ValidateInput`/`CleanName` | UNIT3D IDs are auto-incrementing from small tables; the ceiling blocks garbage and overflows. |
| Term name | 1 to 64 characters, valid UTF-8, collapsed spaces, **no control or invisible formatting** (U+202E, zero-width…) | `trackerprofile.CleanName` | It is just a label, but a hostile label reverses text on screen, makes two "identical" terms different, and breaks logs/terminals. |
| Terms per axis | maximum **256** | `trackerprofile.MaxTermsPerAxis` | Document size limit. |
| Axes | **closed** set (`category`, `type`, `resolution`) | `trackerprofile.Axes` | A new axis is code and `schema_version`, not a free field arriving in a JSON. |
| `kind` | `movie`/`tv`/`game`, only in `category` | `trackerprofile.validateKind` | The Portal acts on it (season/episode, game search); values outside the set cannot reach it. |
| Profile request body | maximum **1 MiB**; unknown field is `400` | `decodeProfileBody` | Profile is untrusted content. |
| Catalog pair that fails validation | **discarded and counted** (`rejected`) | `trackerprofile.Propose` | The operator sees that a discard occurred, instead of a proposal that looks complete. |
| Tracker request error does not carry the URL | — | `unit3d.stripURL`, in `Upload`, `Browse`, `FetchTorrent` and `fetchTorrentBytes` (#220) | The request URL carries the `api_token` in the query (and the passkey, in the `download_link`); the `http.Client` error — and the `url.Parse` error when building the request — included it in full, and the message goes to the Portal and the logs. |

> **Discovery is incomplete by definition.** It only sees what has already been posted: a category, type, or
> resolution that no one has used does not appear, and the catalog goes from newest to oldest. Every
> response brings a `notice`, and the interface cannot suggest otherwise.

## 3.6 Screenshot upload chain — non-parameterized rules (ADR-040 / #193)

Applied in `control-plane/internal/imagehost` and `internal/torrent`.

| Rule | Value | Where | Rationale |
|---|---|---|---|
| Chain order | tracker host → imgbb | `torrent.uploadChain` | The site host has priority (site rule, resilience); imgbb is the last link (ADR-030 global contract unchanged). |
| Tracker host entry in the chain | `image_host` section present + registered key + `Host()` == `image_host_key_host` (key without recorded host is also skipped) | `torrent.trackerCandidate` | The credential goes **only where the operator saw it going**; a profile that changes host disables the key until the operator registers it again. |
| All or nothing per host | failure on any frame passes the turn to the next, which restarts from frame 1 | `imagehost.Chain` | Frames from a single release never mix hosts — better no description than a half-done one (same rule as ADR-030). |
| Total upload budget | **25 s** | `torrent.uploadChain` | Below the CP's 30 s `WriteTimeout` and the proxy's 60 s on `/api/`. |
| Time distribution | fair share of the remainder per host; what one host does not use passes to the next | `imagehost.Chain` | A host that hangs does not consume the fallback's time. |
| Failure reason | **closed and sanitized** set (`chave cadastrada para outro host`, `HTTP <status>`, `redirecionamento recusado`, `tempo esgotado`, `falha de conexão`, `resposta sem URL`, `URL inválida na resposta`) | `imagehost.reasonFor` | Never the raw error text: it carries the URL (in imgbb, the key in the query) and may contain third-party text. |
| Validate URL before accepting | absolute, `https`, up to 2048 characters, no spaces/control/`[`/`]`/quotes | `imagehost.ValidateURL` | The URL is interpolated into `[url=…][img]…[/img][/url]`; without validation, a host's response would inject markup into the description. Applies to imgbb as well. |
| All hosts failed | `502 screenshots_failed` with per-host reasons in the message | screenshot handler | The operator can still paste the BBCode manually (ADR-026). |
| No host configured | the same error as before #193 (`subir screenshots: imgbb sem api_key configurada`) | `torrent.uploadChain` | Without a `tracker_id` and without an imgbb key, there is nothing to try. |
| Missing `tracker_id` in the request | only imgbb | screenshot handlers | Legacy behavior preserved for those calling the API without the field. |
| Every failure becomes a Feed entry | kind `screenshot_host_failed`, with torrent (in `render`, found by the Node and command hash), `Detail` `{tracker_id, host, reason, next}`; `next` = the next candidate in the chain order, empty on the last | `torrent.screenshotFailureEntries` | The site host failure stops being invisible just because imgbb saved the release. |
| `host` in the Feed and response | the **DNS name** of the `upload_url`, never the profile's `name` | `Spec.Host()` | The `name` is third-party text (label); the DNS name is the host identifier. |
| imgbb failure reason | non-2xx status becomes `HTTP <status>`; 2xx without URL becomes `resposta sem URL` | `torrent.imgbbChainError` | The imgbb client has typed errors (`imgbb.StatusError`, `imgbb.ErrNoURL`); without translation, all its failures would come out as "connection failure". |
| imgbb network error without the key | `stripURL` strips the URL from the `*url.Error` | `imgbb.uploadOne` | The key travels in the query (`?key=`); the `http.Client` error carried the entire URL (defect (i) of ADR-040). |
| Redirects are not followed | `CheckRedirect` returning an error; 3xx is a failure | `imagehost.Upload` | Go passes custom headers on redirect; following it would take the key to where no one registered it. |

The **format** rules for the `image_host` section (fields, sizes, `https`, rejected hosts) are not
here: the rules from `13 - API.md` ("Closed format and validation") and ADR-040 apply, enforced by
`trackerprofile.CleanImageHost` on every write (edit, import).

> **Key↔host binding.** The image host key (`trackers.image_host_key`, layer 3) is saved
> along with the current `upload_url` host (`image_host_key_host`). When used, the host of the
> tracker only enters the chain if the profile's current host is **exactly** the saved one; otherwise it is skipped
> with the reason "key registered for another host". This is what prevents an imported profile from changing the
> `upload_url` and taking the key along.

# **4. Validation**

Every write (`PUT /api/v1/settings/{key}`) is validated against the catalog `Spec`: parsing
by type (duration accepts `5m`/`90s`/`168h`; integers and decimal floats) and `Min`/`Max` limits
(for duration, in seconds). Values outside the range are rejected with `400 invalid_value` and nothing is
written. Changes are audited as `SETTING_UPDATED` / `SETTING_RESET` Events.

# **5. Relationship with other documents**

- **ADR-023** — architectural decision (catalog in code + database overrides + hot-reload).
- **07 - Workers** — the Workers that consume these tunables.
- **13 - API** — `/api/v1/settings` endpoints.
- **11 - Portal** — Settings page.
- **12 - Database / Schema** — `settings` table.
