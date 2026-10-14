# **SeedGrid**
**Portal Specification**
Version: 1.0
Status: Draft
Document: 11 - Portal.md
# **1. Objective**
The Portal represents the official administration interface for SeedGrid.
Its objective is to provide the operator with a unified view of the Cluster, enabling monitoring, administration, and operation of the platform through a web interface.
# **2. Philosophy**
The Portal must present the Cluster as a single platform.
The operator must not need to know infrastructure details to perform routine tasks.
Whenever possible, the interface must answer questions, rather than just present information.
# **3. Responsibilities**
The Portal is responsible for:
- presenting the state of the Cluster;
- enabling administrative operations;
- displaying monitoring;
- presenting events;
- presenting alerts;
- managing settings;
- administering users.
The Portal must NOT:
- execute business rules;
- make Scheduler decisions;
- communicate directly with the Nodes;
- access the database.
All communication will occur exclusively through the API.
# **4. General Structure**
The Portal will be divided into modules.
Portal
├── Dashboard
├── Cluster
├── Nodes
├── Torrents
├── Monitoring
├── Feed  (live orchestration activity — ADR-022; separate from Events/Alerts)
├── Events
├── Alerts
├── Settings
├── Users
└── Administration
> **Implemented (Aug/2026) — screen organization.** Navigation currently has
> **Dashboard**, **Downloaded**, Add Torrent, Publish, Explore, Feed,
> Alerts & Events, **Health & Metrics**, Audit, Users, and Settings.
> The Torrents module is divided into two screens by state (queue on the Dashboard,
> collection in Downloaded — see §20) and the *listing* of Nodes lives inside
> Health & Metrics (§18/§24). The per-Node page (`/nodes/{id}`) remains.

# **5. Dashboard**
The Dashboard represents the overview of the Cluster.
Its objective is to allow the operator to quickly understand the state of the platform.
Suggested information:
Number of Nodes.
Online Nodes.
Nodes in maintenance.
Total capacity.
Available space.
Number of Torrents.
Aggregated upload.
Aggregated download.
Active alerts.
Recent events.
# **6. Cluster Module**
Responsible for presenting global information.
Examples:
Cluster name.
Version.
Number of Nodes.
Total capacity.
Used space.
Operational summary.
Overall Health.
# **7. Nodes Module**
Responsible for presenting all Nodes.
Each Node must have its own page containing:
General information.
State.
Hardware.
Inventory.
Disks.
Containers.
Capabilities.
Labels.
Tags.
Hosted Torrents.
Events.
Alerts.
> **Implemented (Aug/2026).** The per-Node page still exists at
> `/nodes/{id}` with inventory, resources, and hosted torrents. The **listing**
> of Nodes is no longer its own screen: the operational summary is in the
> "Seed Nodes" table on the Dashboard and the per-Node console (resources, aggregates, and actions)
> is in Health & Metrics. `/nodes` redirects there.

# **8. Torrents Module**
Represents the main operational area.
Planned features:
Search.
Filters.
Categories.
Tags.
Details.
Trackers.
Files.
History.
Administrative operations.
The operator must see all Torrents in the Cluster in a single list.
# **9. Monitoring Module**
Responsible for presenting the health of the platform.
Examples:
Health Score.
CPU utilization.
Memory utilization.
Disk space.
Temperature.
Availability.
This information will be consolidated per Node.
# **10. Events Module**
Allows querying registered events.
Suggested filters:
Period.
Node.
Torrent.
Type.
Severity.
Free text.
# **11. Alerts Module**
Presents active issues.
Each Alert must contain:
Title.
Description.
Severity.
Origin.
Creation date.
State.
Available actions.
# **12. Settings Module**
Centralizes the Control Plane behavior tunables (ADR-023), editable at runtime
with **hot-reload** (take effect without redeployment). The page (`/settings`) groups parameters by
category (Admission, Alerts, Workers); each field includes a **label + description** (from the catalog),
typed input (duration as text `5m`/`90s`; numbers), a "modified" indicator (override in the
database) and a **restore default** action. The effective value is the override from the `settings` table over
the catalog default; the env var only seeds on the 1st boot. Catalog in `25 - Business Rules.md`.
Parameter examples:
Admission (interval, minimum rate, graces/cooldowns).
Alerts (offline-after, disk saturation, retention).
Workers (reconciler and prometheus-writer intervals).
Agent / Engine (heartbeat, command polling, log level, and the Engine queue — see below).

**Agent / Engine (ADR-039, #144).** The category holds the **cluster value** for the Seed
Nodes behavior: each field bears the **cluster · per Node** badge (this is the value for all Nodes that do not have their own
value) and, since it reaches **all** these Nodes on the next heartbeat, **saving or restoring requires confirmation**.
Right below, the **Nodes with custom value** list (`GET /api/v1/settings/overrides`) answers "if I change
this, who is **not** affected?", with a link to the details of each Node, where the custom value is edited.
# **13. Users Module**
Allows administration of operators.
Examples:
Registration.
Permissions.
Roles.
Sessions.
Audit.

## 13.1 System roles (ADR-035)

The Portal filters navigation and actions based on the roles/permissions of the authenticated operator
(`GET /api/v1/auth/me`). System roles:

- **Administrator** — full access; sees everything, including the **Users** page.
- **Publisher** — manages content: Add Torrent, Publish, Explore, Trackers.
- **Downloader** — consumes: Explore and download torrents.
- **Viewer** — read-only: Dashboard, Nodes, Explore, Feed, Alerts, Metrics.

The **Users** page (admin) lists operators, creates new ones (username, password,
roles), edits roles/status, resets passwords, and disables accounts. Menu items
without permission do not appear; direct URL access is blocked by the backend (403).

**Button gate** (refines ADR-035, no new ADR). Route and menu item gating are not
enough: inside a page that multiple roles can reach, there are buttons from other
domains. The Health & Metrics screen is opened by `metrics:read`, but hosts Node
actions (`nodes:write`) and the join token (`enroll:write`); the **Dashboard** is the
*fallback* page for every denied route — whatever is on it is exposed to **all** roles,
including those with no permissions at all.

The rule has three parts:
1. **Every button that triggers a protected route only appears for those with the SAME
   permission that the backend requires for that route.** Neither looser (the button appears and
   the click gets a 403) nor stricter (hides what the role could use). The
   button's permission is that of the `permitted("…")` for the route in `api.go`.
2. **Hide the button, never the data.** The read-only operator still sees
   disk, memory, I/O, health, the queue, the operation mode, and that there is a new
   Agent version — they just do not see what they cannot execute. Where the button was the only way to
   show data (the operation mode switch, the "↑ version" badge), the data
   stays in an inert element (text/badge), it does not disappear along with it. An actions column that
   would be empty for the role disappears entirely.
3. **The page receives `can`** from `App.tsx` (which allows it while the profile does not resolve,
   so the UI does not flash empty — bug #5 from the ADR-035 review) instead of querying the
   profile on its own; this way, the criterion is a single one for the entire Portal. Every
   new page with an action button is born with `can` — and every route that falls to the Dashboard
   also takes it.

Gated buttons, by screen (permission = that of the route they call):

| Screen | Button | Permission |
|---|---|---|
| Dashboard | Full auto/Flex admission | `nodes:operation-mode` |
| Dashboard | SG/Manual per Node mode | `nodes:operation-mode` |
| Dashboard | "↑ version" (update Agent) | `nodes:write` |
| Dashboard | Queue: prioritize, pause/resume, reverify | `torrents:write` |
| Dashboard | Queue: download content | `torrents:download` |
| Dashboard | Queue: migrate | `torrents:migrate` |
| Dashboard | Queue: delete | `torrents:delete` |
| Node Detail | Restart Engine, Rename | `nodes:write` |
| Node Detail | Configuration: custom value / change / revert to cluster | `nodes:write` |
| Node Detail | pause/resume, reverify torrent | `torrents:write` |
| Node Detail | delete torrent | `torrents:delete` |
| Torrent Detail | Download | `torrents:download` |
| Torrent Detail | Pause/Resume, Reverify | `torrents:write` |
| Torrent Detail | Delete | `torrents:delete` |
| Downloaded | download / pause·resume·reverify / migrate / delete | `torrents:download` / `write` / `migrate` / `delete` |
| Health & Metrics | Add Node | `enroll:write` |
| Health & Metrics | Drain, Restart Engine, Remove, Rename | `nodes:write` |
| Catalog (Explore) | Download & Seed | `trackers:seed` |
| Alerts & Events | Cancel migration (in-flight migration) | `torrents:migrate` |
| Settings → Trackers → profile | Export profile | `trackers:read` |
| Settings → Trackers → profile | Import profile (file → preview → apply) | `trackers:write` |
| Settings → Trackers → profile | Description composition: enable/disable/reorder blocks, MediaInfo tag, Save and Restore default | `trackers:write` |
| Settings → Trackers → profile | Image host (#193): edit/remove the `image_host` section, register/remove the host key | `trackers:write` |

The backend remains the authority: the UI gate prevents the click that would get a 403 — it is **not**
access control. Settings, Users, and Audit remain gated only by route: today
only the Administrator role can reach them, and their internal actions (`settings:write`,
`trackers:write`, `notify:write`, `users:write`) are also only for it. The export and import tracker profile buttons are the exception: they are born with `can`, based on the permission of the route they call (table above). If a custom
role with partial access to these screens ever exists, their buttons will be added to the table above.
# **14. Navigation**
Navigation must prioritize speed and simplicity.
The operator must reach any main functionality with few clicks.
The navigation structure must remain consistent across all modules.
# **15. Objectives**
The Portal must:
- present information clearly;
- facilitate diagnosis;
- reduce repetitive operations;
- centralize all administrative functionalities.
# **16. Fundamental Principle**
The Portal exclusively represents the operator's interface.
All intelligence remains in the Control Plane.
All execution remains in the Agents.
The Portal observes, manages, and presents the Cluster as a single environment.
# **17. Dashboard**
The Dashboard must be the platform's home page.
Its objective is to allow the operator to understand the Cluster's situation in a few seconds.
The Dashboard must prioritize relevant information and avoid excessive details.
# **Indicators**
Suggested indicators:
Online Nodes
Nodes in Maintenance
Offline Nodes
Total Capacity
Used Space
Number of Torrents
Total Upload
Total Download
Active Alerts
Recent Events
> **Implemented (Aug/2026).** In addition to the indicators, the Dashboard provides the
> cluster aggregates (downloaded, uploaded, disk), the **cluster accumulated throughput
> chart** with periods from 1h to 1 year (source `cluster_stat_samples`
> via `/metrics/cluster-throughput`), the "Seed Nodes" table, the
> **"Queue & Downloads"** table (§20), and the most uploaded/downloaded rankings — whose
> names lead to the torrent details.

# **Overview**
The Dashboard must answer questions such as:
Is the Cluster healthy?
Is any Node unavailable?
Are there any critical alerts?
Is there a risk of running out of space?
Are there any ongoing operations?
# **18. Nodes Screen**
The Nodes listing must present an operational summary.
Each row must contain, at a minimum:
Name
State
Health Score
Free Space
Number of Torrents
Agent Version
Last Heartbeat
Labels
Tags
From this screen it will be possible to access the full details of the Node.

> **Implemented (Aug/2026) — no dedicated screen, in two places.** The columns
> above continue to be met, divided by question:
> - **Dashboard → "Seed Nodes" table**: Node name (the nickname, if any —
>   see below), state, operation mode (ADR-024), health, Engine, BT endpoint,
>   Agent version (with the update button — ADR-027) and last heartbeat.
> - **Health & Metrics → cards per Node**: free space, memory, disk I/O,
>   CPU/load, uptimes, storage OK/FAILED with the reason, number of torrents and
>   traffic aggregates, plus the actions (Drain, Restart Engine,
>   Remove, Add Node, Details →).
>
> Labels and Tags remain unimplemented. Destructive buttons are gated by
> `nodes:write`/`enroll:write` — see §13.1.
>
> **Node Nickname (#145).** The Node joins with the OS hostname; the operator can give
> it a **nickname** that lives only within SeedGrid (the machine's hostname is not touched —
> it prefers numbered hostnames, like `seedgrid-node-01`).
> - **When adding:** the "Add Node" modal asks for the name (optional) **before**
>   generating the command. The name travels in the join token and is applied to the record; the copied
>   command is the same as always.
> - **Afterwards:** "Rename" on the Node card (Health & Metrics) and in the Node details,
>   with `nodes:write`. It does not touch the Node nor restart the Agent; empty reverts to the hostname.
> - **Where it appears:** on every screen that shows a Node's name — Dashboard, Health
>   (cards, confirmations, chart legends), Downloaded, torrent details, Alerts
>   & Events, Feed, Analytics, Node selector when adding a torrent, and the migration
>   destination picker. A single rule in the code (`lib/node-label.ts`): nickname when
>   it exists, hostname otherwise.
> - **The hostname remains visible as technical data:** secondary next to the name on the card
>   and in the Node details (which also lists it in Inventory), in tooltips on tables,
>   and alongside the name in destructive confirmations (remove, restart Engine,
>   update Agent) — the operator recognizes the Node by the nickname, but the
>   irreversible action shows the actual machine that will undergo it.
> - **Out of scope:** the text of Alert/Event messages and external
>   notifications continues to use the hostname (see `13 - API.md`).

# **19. Node Details**
Each Node will have its own page.
Suggested sections:
Summary
Hardware
Operating System
Storage
Network Interfaces
Containers
Torrent Engine
Inventory
Events
Alerts
History
Available operations:
Enter maintenance.
Exit maintenance.
Update inventory.
View logs.
View metrics.

**This Node's Configuration (ADR-039, #144).** Section in the Node details between Inventory/Resources and the torrent
list. It is a table with **Parameter · Effective Value · Origin · Actions**:
- **Effective Value** is what the Agent receives; **Origin** is *cluster* or **this Node's value** (highlighted — it is
  the deliberate difference from the fleet — and shows what the cluster would say: "cluster: 10").
- The badge in the title indicates whether the Agent is **up to date**, **waiting for the Agent** (it changed and it hasn't confirmed yet:
  applies on the next heartbeat) or **unsupported** (Agent prior to ADR-039, which remains in the local `.env` —
  it is not an error).
- **Custom Value** / **Change** / **Revert to cluster** require `nodes:write` (gated by button, §13.1: without the
  permission the button **and the Actions column** disappear; the data remains). The backend validates using the same cluster `Spec`
  and the change is audited with the before and after. Browser validation is just for quick feedback.
# **20. Torrents Screen**
This will be the main operational area of the Portal.
The listing must allow:
Instant search.
Sorting.
Filters.
Multiple selection.
Pagination.
Automatic refresh.
Suggested columns:
Name
Node
Status
Progress
Seeds
Peers
Ratio
Upload
Download
Category
Tags

> **Implemented (Aug/2026) — two screens by state.** In steady state almost every
> torrent is seeding, and a single list buried what required action. The split
> uses a single predicate (`isDone`) and its complement, so that every torrent
> appears in exactly one of the two screens — including a new Agent status.
> - **Dashboard → "Queue & Downloads"**: what is not seeding (downloading,
>   waiting, paused, rechecking, with error). Actions: prioritize in queue,
>   download content, pause/resume, recheck, migrate, delete.
> - **Downloaded** (`/downloaded`): the seeding collection, with **instant search
>   by name, filters (Node, status, minimum size, when it was added) and sorting
>   by all data columns** — including the **Node** column. It opens sorted by
>   **Added**, newest first: whoever lands on the screen is usually looking for
>   what just finished downloading. Actions: download content,
>   pause/resume, recheck, migrate, delete.
>
> Columns in Downloaded: Name, Status, Size, Node, Added, Speed, Seeds/Peers,
> Downloaded/Uploaded, Ratio. **Added** is the torrent's `created_at` in the
> Control Plane — relative up to 30 days ("3 h ago"), a date after that, the full
> instant on hover. Migration between Nodes keeps the date; a torrent *adopted*
> from the Engine carries the instant of adoption, not when it entered the
> Engine. The period filter uses rolling windows (last 24 hours / 7 days / 30 days). Progress is left out (in SEEDING it is always 100%).
> Filters and sorting are client-side on `GET /api/v1/torrents` — the endpoint
> has no pagination or filter, and at this order of magnitude it doesn't need it.
> Multiple selection and pagination remain unimplemented (see §34).

# **21. Torrent Details**
Each Torrent must have a dedicated page.
Suggested information:
Summary
Trackers
Files
Statistics
History
Events
Operations performed
Responsible Node
Physical location
Available operations:
Pause
Resume
Recheck
Remove
Change Category
Change Tags

**Screenshots in publishing (ADR-026, ADR-040).** In the UNIT3D tracker registration flow (ADR-020),
the description field gets a **"Generate screenshots"** button: it calls the CP with the selected `tracker_id`
(which orchestrates extraction on the Agent + upload across the host chain) and inserts the returned BBCode at the beginning of the
description, without overwriting anything already there (e.g.: TMDB synopsis). The operator still
**reviews/edits/removes** the BBCode before publishing — the human check from ADR-020
remains the only gate for submission to the tracker. After upload, the panel shows **where the images
ended up** (the host that served them) and any **failures** from earlier hosts in the chain, if applicable. The
"Generate BBCode" button (full sheet) also sends the `tracker_id` and displays the same result in the step bar.

**Tracker taxonomy comes from the profile (ADR-037, #196).** Category, type, and resolution in the UNIT3D tracker
registration **are not** Portal lists: the panel renders what the tracker's `profile` declares
(`GET /api/v1/trackers`), because the IDs belong to the site — `Encode` is 3 on one and 12 on another. The category
role (`kind`: series shows season/episode, game opens the IGDB search) also comes from the
profile, and pre-filling from the name matches by **label** ("1080p", "WEB-DL"), never
reusing the number the parser returns (which is samaritano's). Anything that does not match stays at
"— select —": nothing is chosen behind the scenes. A tracker with a profile lacking categories and types
shows a warning, in the panel and in the tracker list, instead of offering empty selects with no explanation.

In **Settings → Publishing Trackers**, each tracker's **profile** button opens the taxonomy
editor (id, name, role, and origin of each term) and the **"Discover taxonomy"** button: it scans N pages of the
tracker's own catalog and shows an **editable proposal** — how many pages were scanned, why
it stopped, and what is new, what is already in the profile, and what the site calls by a different name. Only what is
new comes pre-checked; renaming is opt-in; anything the operator adjusted manually appears as protected. The screen states,
prominently, that discovery **only sees what has already been posted**: the discovered profile is not complete,
and anything missing must be added manually. Nothing is saved until "Apply selected".

**Exporting and importing the profile (ADR-037, #197).** In the same **profile** panel, the **Share this
site's profile** block has two buttons. **Export profile** downloads the JSON file (the Portal fetches it with the
session token and delivers it to the browser as a file) and states, upon completion, what it contains — the name the
operator gave the tracker and the host — and that it **never** includes a token, passkey, or announce: the name is a free-form label
and is worth checking before publishing the file. **Import profile** opens the file picker and follows three
steps, always in this order:

1. **Preview** (saves nothing): which site the file is from, the warning that the content came from
   outside and **has not been checked against the operator's site** (a wrong id publishes in the wrong category),
   the server warnings — especially the one for a file from **another site** —, and each term with what
   happens to it: *new*, *already present*, *renamed* (with the old name struck through) or *kept* —
   *you called it «X»* (manual) or *your site uses «X»* (confirmed by discovery). A term that
   would end up sharing its name with another, or becoming the first category of a role, shows an amber
   warning **on its own line**; and the other-site file warning also appears when the file (or the
   tracker) has no host to compare against. The summary states how many profile terms the file does not include and that the import **never
   removes** anything.
2. **The choice**: only *kept* terms (the operator's or their site's) have a checkbox — "use the one from the file". Checking it
   updates the count immediately; anything not checked stays as is.
3. **Apply import**: disabled when there is nothing to change. The result is what the server
   **executed** (which may differ from the preview if the profile changed in the meantime).

Anything that comes in gets the **imported** tag (violet): the id came from outside and the operator's site has not yet
confirmed it; running discovery is what confirms it. Both actions are **disabled when there are unsaved changes**
in the editor — the file would carry the saved profile, and the import would swap it out from under what is being
typed. The Portal refuses to read an empty file or one above 512 KiB (the same ceiling as the server)
and sends the file text **as is**: the server is what validates it, and it is the server's message that is displayed.
The label check in the publishing panel (`findByLabel`) only pre-fills when the label is
**unambiguous**: with two terms of the same name on the axis (ignoring case and accents), neither is chosen and the
operator selects manually.
**Upload description composed by the Control Plane (ADR-037, #192).** The Portal **no longer assembles** the
description BBCode (`buildBBCode`/`buildGameBBCode` were removed from `bbcode.ts`, which now only contains the MediaInfo and
size helpers): the **"Step 3: assemble the sheet"** of the full sheet button and the selection of a game
on IGDB (`pickGame`) call `POST /api/v1/publish/description` (`composeMediaDescription` and
`composeGameDescription`, in `api.ts`) and deliver the text to the field, which the operator **continues editing**.
The composition follows the profile of the chosen **tracker** — two sites may require different MediaInfo
order and tags. **Failure does not erase anything:** if the call fails, the error is displayed (in the step bar and in the
sheet's error text, or next to the game search) and the description field stays **exactly as it was**; when
selecting a game, the IGDB ID is filled in immediately, and two consecutive selections count as the last one (a delayed
response does not overwrite the description of the game chosen afterward).

**Description composition in the profile editor (ADR-037, #192).** In the **profile** panel of each tracker, the
**Description composition** block shows whether the composition is **default** or **custom** and, for those who
can write, allows **enabling and disabling** each block (poster, synopsis, tags, screenshots and MediaInfo; cover,
title, about, genres and platforms for games), **reordering** (↑ ↓) and choosing the **MediaInfo tag**
(`[mediainfo]`, `[code]`, `[pre]` or `[spoiler]`). **Save composition** only enables upon change; **Restore
default** only appears when there is a custom composition. The block catalog and the default come from the server
(`description_default` for each tracker): the Portal does not duplicate the list. A disabled block is struck through and
**is not included** in the description; the screen order is the saved order. Unsaved changes **block
exporting and importing** the profile, just like taxonomy changes. Those who can only read the tracker see the composition as
**text** (the data remains; the control disappears — §13.1). In the **import preview**, a file that brings the
composition gains the **Description composition** block: what the file brings in words (order and tag), what
happens to the profile's — *will be applied*, *is already your profile's*, *replaces the one you configured* — and,
when the operator's would be **kept**, the "use the file's" checkbox (the same choice, but for the entire
section: `?overwrite=description`).

**Image host in the profile editor (ADR-037/040, #193).** In the **profile** panel of each tracker, the
**Tracker image host** block shows whether the host is **configured** or **imgbb only** and, for those who
can write, allows editing the seven fields in the section (`name`, `upload_url`, `file_field`, `auth_header`,
`auth_prefix`, `url_path`, `thumbnail_path`). **Save host** only enables upon change; **Remove host**
only appears when there is a configured host. The operator's key (layer 3) is in a separate block: a password-type
field (without an offer to save as a password) with the warning "the key is only sent to `<host>`", the **key
registered** badge (when it already exists) and the warning **the key's host diverges from upload_url — it will not be sent**
when the host of the **saved** `upload_url` (not the one in the form being edited) differs from the `image_host_key_host`
recorded. An **orphan** key — registered, but without the section in the profile — appears with the badge and the **Delete
key**, so it can be removed; registering a new one requires the section. **Register key** and **Delete key**
change state and are gated by `can("trackers:write")`; those who can only read see the section as text, with the badge
and the divergence warning (the data remains; the control disappears — §13.1).
Unsaved changes **block exporting and importing** the profile, just like the other sections. In the
**import preview**, a file that brings the section gains the **Image host** block: what the
file brings in words (name and upload URL), what happens to the profile's — *will be applied*, *is already
your profile's*, *replaces the one you configured* — and, when the operator's would be **kept**, the
"use the file's" checkbox (the same choice, but for the entire section: `?overwrite=image_host`).
# **22. Events Screen**
Events must function as an operational history of the Cluster.
Recommended filters:
Period
Node
Torrent
User
Type
Severity
Text
Events must have links to related resources.
# **23. Alerts Screen**
Alerts represent active issues.
Each alert must display:
Title
Description
Origin
Severity
Date
State
Available actions
Filters:
State
Severity
Node
Category

> **Implemented (#87) — Recent migrations and Cancel migration.** The
> Alerts & Events screen (`/alerts`) has the **Recent migrations** panel
> (`GET /api/v1/migrations`): Source Node → destination, state and, when applicable, the
> reason. States in the operator's language: **Queued**, **In progress** (covers the
> transfer *and* the recheck verification at the destination, which can take hours),
> **Completed**, **Failed** and **Canceled** (failure terminated by the operator, coming from
> `cancelled: true` — is not painted red like a failure).
>
> Every **in-flight** migration (Queued / In progress) shows the **Cancel
> migration** button, which is the way out for the torrent that actions refuse
> (`409 migration_in_flight`) while the migration lasts. It follows the button gate:
> it only appears with `torrents:migrate` (Administrator and Publisher) — the **data** of the
> migration remains visible to those who can read. It asks for **confirmation**, because canceling **discards the
> copy** already made on the destination Node (and, if the copy is still in progress, it
> finishes and only then is discarded); the torrent remains on the source Node. A migration
> still in the queue warns that nothing was copied. The result becomes a toast, and the panel
> reloads.
# **24. Monitoring Screen**
This screen consolidates operational information of the Cluster.
Suggested information:
CPU
Memory
Disk
Network
Temperature
Health Score
Availability
This information can be presented through charts and indicators.

> **Implemented (Aug/2026).** The **Health & Metrics** screen has: Control
> Plane health (API, database, Workers) with CP CPU/memory; **one card per Seed Node**
> (heartbeat resources + aggregates of its torrents + Node actions); and
> "Trends per Node" with Prometheus series (CPU, memory, network, disk,
> disk utilization). The **cluster accumulated throughput** chart moved
> to the Dashboard (§17): "how the cluster has been behaving" is an overview
> question. Temperature remains unimplemented.

# **25. Settings**
Settings must be organized by category.
Example:
Cluster
Scheduler
Monitoring
Security
Integrations
Retention
Feature Flags
Whenever possible, changes must report their impact before confirmation.
# **26. Administration**
Area dedicated to platform management.
Examples:
Users
Roles
Permissions
Sessions
Tokens
Audit
Platform Version
# **27. Global Search**
The Portal must provide a unified search.
The search must locate:
Nodes
Torrents
Events
Alerts
Users
Settings
Results must be grouped by resource type.
# **28. Navigation**
Navigation must remain consistent throughout the platform.
Principles:
Few hierarchical levels.
Permanent sidebar menu.
Breadcrumbs on internal pages.
Search always accessible.
Cross-links between related resources.
# **29. Responsiveness**
The Portal must be usable on:
Desktop
Laptop
Tablet
Mobile devices will only be supported for query and simple administration operations.
Complex operations must prioritize the experience on larger screens.
# **30. Principles**
The Portal must prioritize:
Clarity.
Speed.
Consistency.
Low learning curve.
Contextualized information.
The interface must convey to the operator the feeling of managing a single Cluster, rather than multiple independent devices.
# **31. Operator Experience**
The Portal must be designed to reduce operational complexity.
The operator must be able to quickly locate information, understand the Cluster state, and perform administrative actions without needing to know internal architecture details.
# **32. Visual Organization**
All screens must follow a consistent visual standard.
Princípios:
- same menu organization;
- same visual components;
- same color patterns;
- same navigation conventions;
- same behaviors for recurring actions.
The experience must be uniform across the platform.

## 32.1 Transfer color semantics

The transfer axis must be colored by the **meaning of the data**, never by its
position on the screen, in the table, or in the chart palette:

| Semantics | Color | Where it appears |
| --- | --- | --- |
| **Inbound** — download, downloaded, leechers/peers | **blue** | speed ↓, "Total downloaded", Downloaded column, "Download" action, `DOWNLOADING` badge, "Downloading" activity |
| **Outbound** — upload, uploaded, seeds, seeding | **green** | speed ↑, "Total uploaded", Uploaded column, `SEEDING` badge, "Seeding" activity |

The rule applies to both themes and also to **chart series**: a series carrying the label
`transfer: "down"` is blue and one with `transfer: "up"` is green, even if the categorical
palette were to assign it another color based on arrival order. The meaning travels in that
label, not in the series name — the legend is translated (`/metrics/cluster-throughput` sends
`transfer` next to a default `instance`, which the Portal swaps for the text in the operator's
language). Series without their own semantics
(one per Seed Node, for example) continue to receive distinct hues from the categorical
palette, whose only requirement is that they are distinguishable from each other.

In a pair displayed together ("Downloaded / Uploaded", "Seeds / Peers", "Seeding / Downloading"), each
half receives its color — a pair with a single color reads as a single value.

> **Implementation.** The convention lives in a single place: `TRANSFER` (classes) and `TRANSFER_HEX`
> (hex per theme, for Recharts, which writes `stroke` as an SVG attribute and does not resolve
> `var()`), in `portal/src/components/ui.tsx`. New screens consume the constant instead of
> repeating the color class by hand.
# **33. Interface States**
Every operation must clearly display its state.
Examples:
Running.
Completed.
Failed.
Pending.
Canceled.
Whenever possible, the operator must receive immediate feedback on the submitted request.
# **34. Batch Operations**
Whenever applicable, the Portal must allow operations on multiple resources.
Examples:
Pause multiple Torrents.
Add Tags.
Change Category.
Place multiple Nodes in maintenance.
Update inventory for multiple Nodes.
Operations must be presented clearly, informing the number of affected items.
# **35. History**
Important resources must provide history.
Examples:
Nodes.
Torrents.
Settings.
Users.
Alerts.
The history must use the information already recorded by the Control Plane.
# **36. Audit**
The Portal must provide access to audit logs.
Suggested filters:
User.
Period.
Operation.
Resource.
Result.
The audit must remain separate from operational events.
# **37. Feedback**
The Portal must clearly report the result of operations.
Examples:
Operation completed.
Operation scheduled.
Operation rejected.
Insufficient permission.
Communication error.
Messages must be concise and contextualized.
# **38. Accessibility**
Whenever possible, the interface must follow accessibility best practices.
Examples:
Adequate contrast.
Keyboard navigation.
Alternative text.
Consistent visual hierarchy.
# **39. Internationalization**
The architecture must support multiple languages.
The initial version will use a single configurable language.
New languages can be added without changing the Portal architecture.
# **40. Personalization**
The Portal may allow individual operator preferences.
Examples:
Theme: light, dark or the operating system's (the default, followed live).
Number of items per page.
Home page.
Default sorting.
Visible columns.
These preferences must not change the global behavior of the platform.

> **Status (ago/2026).** Only the theme is persisted (`localStorage`, `sg-theme`: `light`, `dark` or
> `system`; nothing saved = `system`). The filters and
> sorting on the Downloaded screen are only valid while the page is open: the
> router uses pure hash (no query string for deep-linking) and persisted filters
> often become a trap — days later the list appears "empty" with no explanation.
# **41. Performance**
Version 1.0 goals:
Dashboard opening in under 2 seconds.
Search in under 500 ms.
Page switching in under 300 ms.
Information updates without the need to reload the entire interface, whenever possible.
**Language (ADR-041).** The Portal is multilingual (`pt-BR` and `en` in V1) using a custom catalog in `portal/src/i18n/`, without a library: one TypeScript object per language, where the `pt-BR` one defines the type and the others must satisfy it — a missing or extra key will not compile. The language comes from the operator's choice (stored in the browser under the `sg-lang` key, like the theme), then the browser language, then English. No screen writes interface text directly in the component; alerts, events, and activities have their messages assembled here, from `code` + `payload`. The mechanism uses `useT()` and `translate()` in the components, `detectLang()` resolves the initial language, and the guard test `no-literals.test.ts` prevents an already converted file from having literals again.

# **42. Dependencies**
The Portal depends exclusively on the Control Plane API.
No information is obtained directly from the Agents or the database.
This separation allows independent evolution between frontend and backend.
# **43. Evolution**
New modules may be added while maintaining the same navigation structure and the same principles defined in this document.
The architecture must favor expansion without compromising the operator experience.
# **44. Architecture Summary**
Operator
│
Browser
│
Web Portal
│
API Gateway
│
Control Plane
│
┌─────────────┼─────────────┐
│             │             │
Agent 01      Agent 02      Agent 03
The Portal represents only the presentation layer.
All logic remains centralized in the Control Plane.
# **45. Final Statement**
The Portal was designed to provide a unified view of the Cluster, abstracting the complexity of the infrastructure and allowing the operator to focus on platform operations.
Its function is to present information, facilitate administration, and provide a consistent experience for managing SeedGrid, preserving the separation between presentation, decision, and execution established by the platform architecture.
