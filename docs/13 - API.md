# **SeedGrid**
**API Specification**
Version: 1.0
Status: Draft
Document: 13 - API.md
# **1. Objective**
The API represents the official contract between the Control Plane and all platform consumers.
All access to SeedGrid must occur through the API.
No external component must directly access the database or internal services.
# **2. Consumers**
The API must serve the following consumers:
Web Portal
Agent
CLI
SDK
Future integrations
All will use the same API.
# **3. Principles**
The API must be:
Consistent.
Versioned.
Documented.
Deterministic.
Secure.
Stable.
# **4. Architecture**
Portal
│
──────────────────────────
CLI
│
──────────────────────────
Agent
│
──────────────────────────
Integrations
│
──────────────────────────
API Gateway
│
──────────────────────────
Control Plane
# **5. Versioning**
Every API must have an explicit version.
Example
/api/v1/
Incompatible changes must generate a new version.
# **6. Resources**
The main API resources will be:
Cluster
Nodes
Torrents
Trackers
Users
Alerts
Events
Settings
Authentication
Scheduler
Monitoring
Inventory
# **7. Organization**
The API must be organized by domain.
Example
/api/v1/nodes
/api/v1/torrents
/api/v1/alerts
/api/v1/events
/api/v1/activities   (orchestration feed — ADR-022; ?limit, ?torrent_id)

> **Internal webhooks (#116):** `POST /api/v1/integrations/alertmanager` and `/recovery`
> require the shared secret `SEEDGRID_INTEGRATION_TOKEN` (header `X-Seedgrid-Integration-Token`
> or `Authorization: Bearer`). The `scripts/bootstrap-controller.sh` generates the token and shares it with
> Alertmanager (`credentials_file`); without a configured token the endpoints remain open
> (compat) with a warning in the log.

> **Limits and pagination (#115):** every listing with `?limit` has a ceiling of **500** per
> request (default 100). `GET /api/v1/torrents` accepts optional `?limit`/`?offset`
> (absent = full list), returns `total` in the body and uses **conditional ETag**
> (`If-None-Match` → 304 when nothing has changed — Portal polling stops re-downloading the
> entire list on every tick).
/api/v1/settings     (dynamic tunables — ADR-023)

**Settings (ADR-023).** Configuration panel with hot-reload:
- `GET /api/v1/settings` — tunables grouped by category (admission/alerting/workers), each with `key`, `type`, `label`, `description`, `unit`, `value` (effective), `default`, `source` (`db`/`default`), `min`/`max`.
- `PUT /api/v1/settings/{key}` — body `{"value": "..."}`; validates (type + min/max), saves, applies at runtime and audits (Event `SETTING_UPDATED`). `400 invalid_value` if out of range.
- `DELETE /api/v1/settings/{key}` — resets to the catalog default (Event `SETTING_RESET`).

All protected (authenticated operator). See `25 - Business Rules.md` for the catalog.

- `GET /api/v1/settings` now also returns `per_node: true` on items that are **cluster value** of an Agent/Engine tunable (ADR-039, below), and the `agent` category is now called **Agent / Engine**.
- `GET /api/v1/settings/overrides` (`settings:read`) — the Nodes with **custom value**: `{"data": [{"node_id", "node_label", "key", "label", "unit", "value", "cluster_value", "updated_at", "updated_by"}]}`. This answers, on the Settings screen, "if I change the cluster value, who is **not** affected?".

**Agent/Engine configuration per Node (ADR-039, #144).** The behavior of the Agent and Engine (`agent.heartbeat_interval`, `agent.command_poll_interval`, `agent.log_level`, queue `engine.*`) is a **cluster value** (the `/settings` routes above, `settings:write`) with **per-Node override**. Precedence: Node override > cluster value > Node env (Agent fallback) > code default. Key table in `25 - Business Rules.md` §3.1.1b.
- `GET /api/v1/nodes/{id}/settings` (`nodes:read`) — what applies **to this Node** and where it comes from: `{"node_id", "status", "effective_version", "applied_version", "applied_at", "seeded_at", "items": [...]}`. Each item: `key`, `type`, `label`, `description`, `unit`, `min`/`max`, `options`, `value` (**effective** — what the Agent receives), `source` (`node`|`cluster`), `cluster_value` (what the Node would have without override), `cluster_source` (`db` = set by the operator | `default` = catalog), `override` (`null` if none). `status`: `in_sync` (the Agent applied today's version), `pending` (it changed and the Agent has not confirmed yet) or `unsupported` (the Agent never reported it — prior to ADR-039, remains in `.env`). `404` if the Node does not exist (or the `id` is not a UUID).
- `PUT /api/v1/nodes/{id}/settings/{key}` (`nodes:write`) — body `{"value": "..."}`. Validates against the **same `Spec`** as the cluster value (type, range, options): `400 invalid_value`. `400 not_per_node` for a Control Plane tunable (e.g., `admission.interval`); `404 setting_not_found` for an unknown key and `404 node_not_found` for a non-existent Node; body with unknown field is `400 invalid_body`. Response: the resolved item. Takes effect on the next Agent heartbeat, no restart required. **Audited with before and after** (ADR-025): "Set the «Simultaneous downloads per Node» setting of Node «node-04» to 3 (previously followed the cluster)".
- `DELETE /api/v1/nodes/{id}/settings/{key}` (`nodes:write`) — removes the override: the Node reverts to the cluster value. Idempotent. Audited ("Removed the custom value of «…» from Node «…» (was 3; reverts to cluster: 10)").
- Changing the **cluster value** of a `per_node` key (`PUT /settings/{key}`) applies to every Node **without** an override; the audit trail states this.

**Operation mode (ADR-024).** Operator control over admission (global gate + per-Node mode):
- `GET /api/v1/nodes` — in addition to `data` (Nodes, each with `operation_mode` `sg`/`manual`), returns `cluster_operation_mode` (`full_auto`/`flex`), the global gate.
- `PUT /api/v1/cluster/operation-mode` — body `{"mode": "full_auto"|"flex"}`. Toggles the gate; when returning to `full_auto`, resets all Nodes to `sg` (Event `CLUSTER_MODE_UPDATED`). `400 invalid_mode` for invalid value.
- `PUT /api/v1/nodes/{id}/operation-mode` — body `{"mode": "sg"|"manual"}`. Only allowed when the cluster is in `flex` (otherwise `409 gate_locked`); `404` if the Node does not exist (Event `NODE_MODE_UPDATED`).
- A Node is treated as **Manual** (operator is sovereign; the admission controller does not touch it) only when the gate is `flex` AND the Node is `manual`. The downloads of a Manual Node **count** towards the L ceiling (invariant).

**Node Alias (#145).** The Node joins the cluster with the OS hostname, and there is no reason to mess with the machine just to name it. The **alias** (`display_name`) is a name that lives **only within SeedGrid**: the OS hostname remains the technical data, untouched, and renaming sends nothing to the Node nor restarts the Agent. The Portal displays the alias where it shows a Node's name, falling back to the hostname when there is none.
- `POST /api/v1/enroll-tokens` (`enroll:write`) — **optional** body `{"display_name": "..."}`. The alias travels **in the join token** and is applied when the Node registers: the token already binds the bootstrapping Node to the operator's intent, so the name reaches registration without `bootstrap-node.sh` or the Agent loading anything new. No body is the original usage (no alias). The response echoes the already normalized `display_name`. `409 display_name_taken` if the name already belongs to a Node; `400 invalid_display_name` if it fails validation.
- `PUT /api/v1/nodes/{id}/display-name` (`nodes:write`) — body `{"display_name": "..."}`. The field is **mandatory**: `""` removes the alias (reverts to the hostname), but a body without the field is `400 invalid_request` — deleting by omission would be too easy to do by mistake. `404` if the Node does not exist; `409 display_name_taken`; `400 invalid_display_name`. Response `{"display_name": "<valor gravado>"}`.
- `GET /api/v1/nodes` already returned `display_name`; it is now populated, and the list is sorted by the **displayed name** (alias, or hostname if none). Payloads that carry a Node's hostname gained the alias alongside it: `GET /api/v1/activities` → `node_display_name`; `GET /api/v1/analytics` → `display_name` on each torrent and in `nodes[]`.

Alias rules:
- **Normalization:** leading and trailing spaces are removed, and any sequence of space/tab/line break becomes **a single space** (two names that differ only in invisible whitespace are the same name for the operator). Maximum of **64 characters** (not bytes). Only printable characters: control and zero-width characters are rejected — they would create visually identical "duplicates" that the uniqueness check would miss.
- **Uniqueness — decided: yes.** Two non-removed Nodes never display the same label. The comparison is **case-insensitive** and applies against the alias **and against the hostname** of other Nodes (an alias matching another machine's hostname would be the same problem). Reason: the name is what the operator reads and selects in destructive action pickers (migrate, drain, remove Node) and is the series key in charts — a repeated name is ambiguity right where a mistake costs data. A removed Node's name returns to the pool. A partial unique index (`uk_nodes_display_name_per_cluster`) guarantees against race conditions; the friendly check (409) is done beforehand by the Control Plane.
- **Registration never fails because of the name.** Between issuing the token and the Node registering, the name might have been taken (or two open tokens requesting the same name — allowed on purpose, otherwise closing the modal without copying the command would block the name for 1h). In these cases the Node **joins without an alias** — the first to register gets the name —, the `NODE_REGISTERED` Event notifies, and the operator renames it later via the Portal. On registration with a join token, **the token's name** takes precedence; the `display_name` sent by the Agent in the body is ignored (operator's decision, not the machine's). Re-registration without a name does not erase the existing alias.
- **Audit (ADR-025):** the change is logged with the before and after — "Renamed Node 'Room' to 'Bedroom'", "Assigned alias...", "Removed alias..." — because the old value no longer exists anywhere after the UPDATE. Issuing the token with an alias logs "Generated a join token for Node 'X'". Attempts without `nodes:write` are denied with 403 and also recorded in the trail.
- **Out of scope (remain by hostname):** the texts for Alerts, Events, and external notifications (Telegram/Discord/webhook) — the ~30 points that build messages read `nodes.hostname` directly, and mixing the two names in the same trail would be worse than keeping the technical identifier stable in it.

**Audit (ADR-025).** Trail of operator actions — who did what, when, from where:
- `GET /api/v1/audit` — most recent trail first. `?actor` filters by operator; `?limit` (default 100, cap 500). Each item: `actor`, `action` (friendly PT-BR description, e.g., "Changed the 'Node offline after' setting to 120s" — the fallback), `action_code` and `action_params` (where the reader builds the phrase in their language, ADR-041; absent on records older than #57), `method`, `path`, `status_code`, `source_ip`, `created_at`.
- Recording is **automatic**: a middleware over protected routes logs every **mutating** request (POST/PUT/DELETE/PATCH) from the operator; **login** (success and failure) is logged explicitly. GET is not audited. Best-effort — never blocks the response. Separate trail from Alerts/Events/Activity.

**Screenshots for publishing (ADR-026/030/040).** Extends publishing to UNIT3D tracker (ADR-020). There are three routes:

- `POST /api/v1/torrents/{id}/screenshots` — one-step flow. Optional body `{"tracker_id": "…"}` (empty still applies; malformed body or with unknown field is `400 invalid_body`); the CP triggers the `extract_screenshots` command (queue ADR-014) against the Agent, waits for the frames, uploads to the **host chain** (tracker host → imgbb, ADR-040), and returns the BBCode. Success (200): `{"bbcode": "…", "host": "…", "failures": []}`.
- `POST /api/v1/torrents/{id}/screenshots/extract` — queues the extraction on the Node and returns the `command_id` for the Portal to track the "Extracting on Node" step via `GET /commands/{id}`. Response 202: `{"command_id": "…"}`.
- `POST /api/v1/screenshots/render` — reads the frames from a completed `extract_screenshots` command, uploads to the host chain, and returns the BBCode. Body: `{"command_id": "…", "tracker_id": "…"}` (`tracker_id` optional). Success (200): `{"bbcode": "…", "host": "…", "failures": []}`.

**Host chain (ADR-040, #193):** the order is tracker host (`image_host` section of the profile + registered key + verified host) → imgbb (global key). The first host that hosts **all** frames wins; frames from a single publication **never mix hosts** (all or nothing). Each failure becomes a Feed entry (`screenshot_host_failed`) with the sanitized reason and the next host in the queue. Total budget of **25 s**, split among the hosts. `host` in the response is the DNS name of the winner; `failures` is **always** a list, `[{"host": "…", "reason": "…"}]`, with the hosts that failed or were skipped before the winner (`[]` when the first one won). In the Feed entry, `next` is the next host in the chain order (whether it failed or won), empty on the last one; the torrent also goes along in the `render` (found by the Node and the command hash). Without `tracker_id`, only imgbb (legacy behavior); without `tracker_id` with its own host and without the imgbb key, the response is the pre-#193 error, `502 screenshots_failed` with `subir screenshots: imgbb sem api_key configurada`.

**Errors:** `404 torrent_not_found` for non-existent torrent and `404 tracker_not_found` for non-existent, removed, another operator's, or malformed id tracker — tracker ownership is verified **before** extracting frames or uploading anything; `409 no_hash` for a torrent that still has no hash; `502 screenshots_failed` when **all** hosts have failed (message lists the reasons per host). The returned BBCode goes through human review in the Portal before `POST /api/v1/torrents/{id}/publish-tracker`.

**Duplicate evidence on the tracker (#186).** `GET /api/v1/trackers/{id}/dupes?name=…` — requires `trackers:read`. Searches the tracker catalog for releases that might be duplicates of what is about to be published, and returns `{"query": "…", "candidates": [{…, "same_year": true}]}`. The search uses the **inferred title** from the name (`release.Parse`), not the entire standard name: the full name is too specific and the UNIT3D name filter would not match anything.

The contract has two deliberate properties, and both exist because the search is an **aid for human review, not a verdict**:

- **Never fails "upstream".** Tracker down, network error, or unavailable catalog respond with **200** with an empty `candidates` and a `note` field explaining. A 5xx here would cause an aid failure to prevent publication — and publication is what the operator came to do.
- **The year marks, it does not filter.** `same_year` signals a match with the inferred year, but no candidate is hidden for diverging: the catalog sometimes has the wrong year, and the cost of a false positive is the operator taking a look, while the cost of a false negative is publishing a duplicate on a private tracker.

The Portal displays the candidates and requires an explicit "publish anyway" before enabling the button — the same pattern as `allow_duplicate` in cluster duplicates (#11). The decision remains with the operator; the endpoint only ensures they are informed.

**Frozen payload preview (#188).** `POST /api/v1/torrents/{id}/publish-tracker/preview` — requires `torrents:publish`. Receives **the same body** as `publish-tracker` and returns the exact payload that the submit would send, without touching the network: `{"endpoint", "tracker", "fields": {…}, "torrent_name", "torrent_size", "missing": [], "token"}`.

It exists because the **UNIT3D API has no dry-run**. Without the preview, the only way to find out what will be posted is to post it — and a wrong field becomes a wrong record in a private tracker's catalog, with no undo. This weighs more here than in a common upload tool: the `.torrent` going into the POST is already **seeding** on a Node, so a wrong post is not just a bad record, it is a published infohash that the cluster is already serving.

What makes "frozen" mean anything is the `token`, not the screen:

- `token` is the **payload fingerprint** — all text fields plus the `.torrent` (name, size, and content). Changing the file while keeping the form identical would publish different content under the same review, so the file is included in the calculation.
- The submit accepts an optional `preview_token` and, when it is provided, **verifies** it: if the form changed after the review, it responds with **409 `preview_stale`** instead of posting what no one approved. It is optional to avoid breaking existing clients; the Portal always sends it.
- `fields` comes from `unit3d.Fields()`, the **same** function that `Upload` uses to build the multipart. A second build would allow the preview to show one thing and the POST to send another — exactly the failure the preview was supposed to eliminate.

`missing` lists the required fields that are still empty (`name`, `description`, `category_id`, `type_id`). The preview does not reject because of them: it warns, so the operator can fix it on the screen instead of getting a 400 on submit.

**Site profile and taxonomy discovery (ADR-037, #190/#196).** What belongs to the **site** — currently the taxonomy (the `category`, `type`, and `resolution` IDs, which vary from site to site: `Encode` is 3 on samaritano and 12 on Blutopia) — lives in the tracker **profile**, separate from the operator's credential. The profile comes **inside** the tracker, with no extra call:

- `GET /api/v1/trackers` (and the responses from `POST`/`PUT /api/v1/trackers`) — each tracker gets a `profile`: `{"family": "unit3d", "schema_version": 1, "taxonomy": {"category": [{"id": 1, "name": "Filmes", "kind": "movie", "origin": "seed"}, …], "type": […], "resolution": […]}}`. The three axes are **always** present (empty list, never `null`), sorted by `id`. A tracker without a profile row comes out with an empty profile. The `description` section (#192) only appears when the operator has customized the description composition — see "Description composition per tracker", below. This is where the publish panel builds the options from — there is no longer a hardcoded list in the Portal. **No `announce`, passkey, or `api_token`**: the profile is layer 2 of ADR-037 and the credential is layer 3, which never leaves the API.
- `origin` says where the term came from — `seed` (migration), `discovered` (discovery), `manual` (the operator typed or renamed it), or `imported` (came from a profile file, #197) — and is **decided by the server**. It is **local** state: it does not travel in the exchange file. `kind` (`movie`, `tv`, `game`; only in `category`) is the role of the category: it is what the Portal uses to show season/episode and game search, instead of the hardcoded category number.
- `PUT /api/v1/trackers/{id}/profile` — requires `trackers:write`. Body `{"taxonomy": {"type": [{"id": 50, "name": "PC"}, …]}}`. The **sent axes replace** the current ones (even if empty); the **missing ones are untouched** — a client does not erase the taxonomy by forgetting a field. A new or renamed term becomes `manual`; what has not changed keeps its origin. Responds with `200` and the profile. Audited: "Edited profile of tracker 'X'".
- `POST /api/v1/trackers/{id}/discover-taxonomy` — requires `trackers:write`. Optional body `{"pages": N}` (default **8**, ceiling **20**, pages of 50 items). Scans the **tracker's own** catalog (`GET /api/torrents/filter`, the same as browse), with the operator's own token, and aggregates the distinct `(id, nome)` pairs per axis. **Only proposes — never saves.** Response: `pages_requested`, `pages_scanned`, `items_scanned`, `stopped_reason` (`end_of_catalog` | `max_pages` | `rate_limited` | `time_budget` | `error`), `proposal` (per axis, items `{id, name, seen, status}` with `status` `new` | `same` | `renamed`; `renamed` brings `current_name`, `current_origin`, and `protected`), `rejected`, `warnings`, and `notice`. The scan has a budget of **20 s** (the server closes the response in 30 s): if the tracker rate limits (`429`) or time runs out, the response is **`200` with what has been collected so far** and the `stopped_reason` says it stopped. It only fails (`502 discover_failed`) if no page was reached. `400 tracker_without_api` without `api_url`/token. Not audited: it does not change state.
- `POST /api/v1/trackers/{id}/apply-taxonomy` — requires `trackers:write`. Body `{"items": [{"axis": "type", "id": 50, "name": "PC", "origin": "discovered"}]}` (`origin` `discovered` — default — or `manual`, when the operator edited the name in the proposal; `seed` is rejected). Applies **on the server, on the locked state** (`SELECT … FOR UPDATE`), the rule that makes it safe to run discovery again: id missing from profile → **adds**; id present with the same name → nothing; id present with a different name → **renames, unless the origin is `manual`** (then it is ignored as `protected`); what the catalog did not show remains **untouched** — absence is not evidence. Discovery never removes and never touches `kind`. Response: `{"profile", "applied": [{axis, id, name, action: "added"|"renamed"}], "skipped": [{axis, id, name, reason: "same"|"protected"}]}`. Audited: "Applied discovered taxonomy to tracker 'X': N new, M renamed".

⚠️ **The limit the contract carries:** discovery only sees **what has already been posted**. Category, type, or resolution that no one has used does not appear, and the catalog is scanned from newest to oldest — the discovered profile **is not complete by definition**. Therefore, every response includes a `notice` and the Portal repeats the warning; whatever is missing is added manually (`PUT …/profile`). Discovery also does **not** reach site rules (wiki text) or name format (note) — these depend on whoever holds the account, and circulate via export/import (below).

**Profile is untrusted content.** Pairs arrive from a third-party site's catalog, from the operator, and from an imported file, and all go through the same validation before touching the database: integer `id` in `1..1000000`; name with 1 to 64 characters, valid UTF-8, collapsed spaces, **no control or invisible formatting characters** (the U+202E direction override, zero-width…); at most 256 terms per axis; axes from a **closed** set (`category`, `type`, `resolution`); `kind` only in `category`; request body limited to 1 MiB; unknown field in JSON is `400`. Invalid input responds with `400 invalid_profile` with the field path (`taxonomy.type[2].name: …`). A catalog pair that fails is **discarded and counted** in `rejected` — the operator sees that a discard occurred instead of a seemingly complete proposal. No free-form field goes into a tracker request: the publish POST carries only the **IDs**; the name is a label.

**Profile export and import (ADR-037, #197).** The site profile is what whoever holds an account on the site discovers, adjusts, and **shares**: the export generates a file, the import applies it to another tracker — without the second user needing an account anywhere. The file is layer 2 plus an identification envelope; it **never** carries `api_token`, passkey, announce, `api_url`, or `source` — not because of a filter, but because the type that generates it has no fields for them.

> **Contributed profiles.** Profiles shared by the community are versioned in `profiles/<family>/<site>.json` (see `profiles/README.md`); they are files generated by this export, validated in CI by the same rules as the import.

- `GET /api/v1/trackers/{id}/profile/export` — requires `trackers:read`. Responds with the file (`application/json`, `Content-Disposition: attachment; filename="seedgrid-profile-unit3d-<host>.json"`, only `[a-z0-9-]`; `Cache-Control: no-store`). Read: **not** audited.
- `POST /api/v1/trackers/{id}/profile/import/preview` — requires `trackers:write`. The **body is the file**, byte by byte (`curl --data-binary @perfil.json`). Validates and returns `{"plan", "notice"}`. **Never writes** (nor creates the profile row) and is **not** audited.
- `POST /api/v1/trackers/{id}/profile/import` — requires `trackers:write`. Same body. Applies and returns `{"profile", "plan"}`, where `plan` is what was **executed** (calculated on the locked row, `SELECT … FOR UPDATE`; may differ from what the preview showed if the profile changed in the interval). Audited: "Imported a file profile on tracker 'Y': N new, M renamed, K role(s) filled; P kept because they belong to the operator or were already confirmed by their site" — nothing that came from the file enters the trail.
- Both import endpoints accept `?overwrite=<eixo>:<id>` (repeatable, e.g., `?overwrite=category:2`) — and `?overwrite=description` (description composition) or `?overwrite=image_host` (image host), each section as a unit —: the request, term by term, to overwrite a **protected** name — what the operator adjusted manually (`manual`) or what their site confirmed in discovery (`discovered`). Without it, a `manual` or `discovered` term is **never** overwritten. Malformed value is `400`; a request that does not match any protected term or section the file brings becomes a warning in `plan.warnings`, never an effect.

*Previewing and applying are two routes, on purpose:* the route easy to call by mistake never writes, and the audit trail (which decides by method and path) does not log a read as if it were an import.

The file (real example, generated by the code from a profile of three categories, two types, and two resolutions — starting from terms with `origin` `seed`, `manual`, `discovered`, and `imported`, **none** of which are output in the file):

```json
{
  "format": "seedgrid.tracker-profile",
  "site": {
    "name": "Samaritano",
    "host": "tracker.exemplo.org"
  },
  "family": "unit3d",
  "schema_version": 1,
  "taxonomy": {
    "category": [
      {
        "id": 1,
        "name": "Filmes",
        "kind": "movie"
      },
      {
        "id": 2,
        "name": "Séries",
        "kind": "tv"
      },
      {
        "id": 5,
        "name": "Games",
        "kind": "game"
      }
    ],
    "resolution": [
      {
        "id": 2,
        "name": "2160p"
      },
      {
        "id": 3,
        "name": "1080p"
      }
    ],
    "type": [
      {
        "id": 3,
        "name": "Encode"
      },
      {
        "id": 4,
        "name": "WEB-DL"
      }
    ]
  }
}
```

- `format` — always `seedgrid.tracker-profile`. `schema_version` — the **format** version (currently `1`; increases when a field changes shape or a **mandatory** section is added — an optional and additive section, which the export omits when at default, like the `description` from #192 and the `image_host` from #193, does not increase: see "Description composition per tracker" and "Image host per tracker"). `family` — currently only `unit3d`; must be that of the destination tracker (`409 incompatible_family` if not). `site` — identification only: `name` (the label the operator gave the tracker — **check before sharing**; if it fails name rules or **looks like it contains a credential** — slash, `?`, `=`, UUID, 20 or more consecutive letters/digits —, the public host is output, or `tracker`: the name is never truncated) and `host` (the DNS name of the announce host, without port or path; omitted if not a valid hostname). `site` **is not applied to anything**: it serves for the operator to know what they are importing and for the preview to warn when the file is from **another site** (the IDs are from the site; a wrong ID publishes in the wrong category). `taxonomy` — per axis (`category`, `type`, `resolution`; missing axis is not an error), terms `{id, name, kind?}`. `description` — optional, only when the operator customized the upload description composition (#192): `{media_blocks, game_blocks, media_info_tag}`, validated as in `PUT …/profile/description`. `image_host` — optional, only when the tracker has its own image host (#193): `{name, upload_url, file_field, auth_header, auth_prefix, url_path, thumbnail_path?}`, validated as in `PUT …/profile/image-host`. **The file does not carry `origin`** (it is local to each installation) and the import **rejects** a file that brings it; the import assigns the `imported` origin to everything it writes.
- Plan (`plan`): `site`, `family`, `schema_version`, `items` (per axis, `{axis, id, name, kind?, status, current_name?, current_origin?, sets_kind?, overwrites_protected?, warnings?}` with `status` `new` | `same` | `renamed` | `protected`), `counts` (`new`, `renamed`, `same`, `protected`, `kind_set`, `local_only`), `description` (only when the file brings the section: `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`), `image_host` (only when the file brings the section: `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`) and `warnings`. The preview also brings `notice`: the warning that the content came from a file and was not checked against the operator's site.
- **Coexistence rule** (the one from `apply-taxonomy`, extended and stricter where discovery is already opt-in): missing id → **adds** (`imported`); same name → nothing; different name → **renames only what is `seed` or `imported`**; a `manual` or `discovered` term (the name the operator's site confirmed) is `protected` and stays as is — just like an origin the code does not know (fail closed, in import and discovery); profile term that the file does not bring → **untouched** — the import **never removes**. The file's `kind` only **fills** an empty role, never replaces an existing one, and a `protected` term does not even have its role touched. Importing the same file twice is incremental: the second time changes nothing.
- **Collision (a new term cannot obscure the right one).** The publish panel matches by name and pre-fills the first category of each role. Therefore, a file with two names that differ only in case or accent **on the same axis** is rejected (`400`, `taxonomy.category[1].name: nome repetido no eixo: igual ao do id 1…`), and the preview warns, in `items[].warnings`, about the new or renamed term whose name collides with another id's name on the axis (regarding the state the axis would be in) and what would become the **first** category of a role that already had an owner; `plan.warnings` brings the summary. For the same reason, **`GET …/export` responds with `409 profile_not_exportable`** (with the reason) for a local profile with repeated names: the export only delivers a file that the import itself would accept.
- **Validation (the file is untrusted content).** Body up to **512 KiB** (`413 payload_too_large`); valid UTF-8; JSON with **a single** value; keys only `[a-z0-9_]` and **no repetition** (the standard decoder matches keys ignoring case and Unicode folding and keeps the last of two repeated ones — the reviewed file would be different from the imported one); at most 8 levels of nesting; **unknown field is an error**; `format` checked; integer `schema_version` from `1` up to the current version (`400 unsupported_schema_version`, stating which one this SeedGrid understands, when the file is from a newer version); known `family`; `site.name` with name rules (1 to 64 characters, no control, no invisible formatting, and no runes that look like spaces but are not spaces — U+3164, U+115F, U+1160, U+FFA0, U+2800…); `site.host` a DNS name; and, in the terms, the same rules as `PUT …/profile`. A **profile section that this SeedGrid exports but does not yet know how to import** is rejected if it comes filled (fail closed). Error: `400 invalid_profile` with the field path — never the received value, which can be megabytes. Tracker from another operator or malformed id: `404`.
**Description composition per tracker (ADR-037, #192).** The BBCode description of an upload was assembled in the Portal (`bbcode.ts`): out of reach of any other API client, with nowhere for two sites with different markup to diverge, and without Go tests — being the field the site publishes in full. Now it is composed by the Control Plane (`internal/description`), via the **`description` section of the tracker profile**: it belongs to the site (layer 2 of ADR-037), not the operator, and therefore travels in the export/import. The Portal only requests the text and delivers it to the field, which the operator **continues editing** before publishing — the human review from ADR-020 does not change.

- `POST /api/v1/publish/description` — requires `publish:write` (the same as `publish/infer`). Body `{"tracker_id": "…", "kind": "media" | "game", "media": {"poster", "overview", "keywords", "screenshots", "mediainfo"}, "game": {"cover_url", "name", "year", "summary", "genres", "platforms"}}` — only the object for the requested `kind` is read (`screenshots` is the BBCode block returned by `…/screenshots/render`). Responds with `200 {"description": "<BBCode>"}`, composed using the `description` section of the tracker profile or, if absent, with the default. Errors: `404 tracker_not_found` for a non-existent, removed tracker, one belonging to another operator, or with a malformed id; `400 invalid_request` for a `kind` that is neither `media` nor `game` (the check comes **before** the tracker lookup), missing `tracker_id`, malformed JSON, and unknown or incorrectly typed fields (including inside `media`/`game`); `413 payload_too_large` for payloads over 4 MiB (the MediaInfo of a long release exceeds 100 KiB). **Only composes text from the body**: it does not read the torrent, does not query TMDB or IGDB, and does not change state — therefore it stays **outside the audit trail** (ADR-025), like `publish/infer`. `POST /api/v1/publish/infer` accepts an optional `tracker_id` (#74): the TMDB synopsis comes in that tracker's content language; without it, English.
- `PUT /api/v1/trackers/{id}/profile/description` — requires `trackers:write`. The body is the config: `{"media_blocks": ["poster", "synopsis", "tags", "screenshots", "mediainfo"], "game_blocks": ["cover", "title", "about", "genres", "platforms"], "media_info_tag": "mediainfo"}` (this is the default). **Missing** list = the default; **empty** list (`[]`) = no blocks of that type; missing or empty tag = `mediainfo`. Saves the **explicit** section (all three keys always populated) and responds with `200` along with the profile. Does not touch the taxonomy. Audited: "Edited the description composition of tracker 'X'". Optional `language` (`en` | `pt-BR`; missing = `en`, and `en` is not written back): the site's content language (#74) — description headings and the TMDB synopsis; `400 invalid_profile` for any other value.
- `DELETE /api/v1/trackers/{id}/profile/description` — requires `trackers:write`. Deletes the section: the tracker reverts to the default. Idempotent (deleting what does not exist also returns `200`). Audited: "Restored the description composition of tracker 'X' to default".
- `GET /api/v1/trackers` (and the responses to `POST`/`PUT /api/v1/trackers`) — the `profile` gains a `description` **only when the operator has customized it** (the key does not even exist in the default profile), and the tracker gains, alongside `profile`, a `description_default`: the composition that applies without the section — which, by listing **all** blocks in the original order, is also the **catalog** that an editor provides to enable, disable, and reorder. It comes from the server so no client duplicates the default.

The blocks are a **closed** set (a new block is a code change, not a free-form id in a JSON); the composition joins, **in the list order**, the blocks that have content, separated by a blank line, and **omits** those that are empty:

| id | type | output |
|---|---|---|
| `poster` | media | `[center][img]URL[/img][/center]` — the URL is inserted as received, without trim |
| `synopsis` | media | `[b]Sinopse[/b]` + the text (with trim) |
| `tags` | media | `[b]Tags[/b]` + the keywords (with trim) |
| `screenshots` | media | the ready block from the CP, with trim — without its own label |
| `mediainfo` | media | `[b]MediaInfo[/b]` + the dump inside the config **tag** (`mediainfo`, `code`, `pre` or `spoiler`) |
| `cover` | game | `[center][img]URL[/img][/center]` |
| `title` | game | `[b]Nome (ano)[/b]` — **always outputs** if the block is in the list, even without a name; the year only appears if it is non-zero |
| `about` | game | `[b]Sobre[/b]` + the summary (with trim) |
| `genres` | game | `[b]Gêneros[/b]` + the items separated by `, ` (only the list size decides: `[""]` still emits the block) |
| `platforms` | game | `[b]Plataformas[/b]` + the items separated by `, ` |

**Parity with what the Portal built.** With the **default** config, the output is **byte-for-byte** identical to the old TypeScript — labels, order, separator, and omission of empties. Parity is not asserted, it is measured: the expected test strings were calculated by **running** the original `buildBBCode`/`buildGameBBCode` (`git show 0525f0f:portal/src/pages/publish/bbcode.ts`) on 44 inputs, and pasted into the test. What JS `trim()` removes is **not** what Go `strings.TrimSpace` removes: JS strips the BOM (U+FEFF) and does not strip the NEL (U+0085), Go does the opposite — which is why the CP has its own `jsTrim`, and there are golden lines just for that (BOM, non-breaking space, U+2028/2029, the Zs, U+0085, U+180E, U+200B).

**The config is untrusted content** (ADR-037) — it may arrive in an imported file — and is validated at **every write point** (the `PUT`, the import, and the export): `media_blocks` and `game_blocks` only accept ids from the closed set of **their** type (a game block in a media list is rejected), without repetition and with a maximum size of the set (a list of thousands of items is not even iterated); `media_info_tag` only accepts `mediainfo`, `code`, `pre` or `spoiler` — the tag is **interpolated in BBCode**, so nothing outside this list passes (`mediainfo][img=…` is rejected, and the composition still falls back to the default if an invalid tag appears in a manually edited database). Rejection: `400 invalid_profile` with the field path (`description.media_blocks[1]: bloco "x" desconhecido (use poster, …)`); the message never returns the received value in full.

**Where it lives.** In the **same** JSONB column of the profile (`tracker_profiles.taxonomy`), as the `description` key **alongside the axes** — without migration, without a new column, and without rewriting any rows: the existing document is, byte-for-byte, a new document without the section. `schema_version` **does not change** (remains `1`): the section is optional and additive, and the export omits it as long as the profile does not have it, so a profile without it remains exactly the v1 file. The accepted cost: a SeedGrid **prior to #192** rejects a file containing `description` with `campo desconhecido` — and **not** with `unsupported_schema_version` — and a Control Plane prior to #192 does not read the profile of a tracker whose composition was customized (restore the default — `DELETE` above — before downgrading).

**On export and import.** The file gets the `description` key after `taxonomy`, **only when the operator customized it** — the Document embeds the Profile, so the section is exported without modifying the export, and the export goes through the same validation as the import (an invalid section manually recorded in the database causes the export to fail with `409 profile_not_exportable` and the reason). On import the section is **a single unit**, with no per-term origin, and the rule is that of `manual`, applied to it entirely: profile **without** its own composition (or with the default) → **applies** (`new`); equal → nothing (`same`); **different → protected** (`protected`): it stays as is and `plan.warnings` says it kept it, unless the operator requests `?overwrite=description` — then it **replaces** (`replaced`). A file that does not bring the section **does not touch** the profile's; a file **only** with the section is a valid import (it does not output the "no term" warning). The plan gets `description` — `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`, present only when the file brings the section. The taxonomy and the section come from the same plan, calculated on the locked row, and are recorded **together**. Audit: applying adds "; description composition: applied | replaced on request | kept the operator's | was already equal" **only when the file brought the section**.

**Image host per tracker (ADR-040, #193).** The image host policy for screenshots — currently the global imgbb, a single key in the catalog — becomes **tracker profile data** (layer 2 of ADR-037): the optional `image_host` section describes the site's own host, and the upload chain (tracker host → imgbb) uses whatever is configured. The section has a **closed form** (seven fields, ADR-040) and travels in export/import like `description`. The **operator's key** for the host is layer 3 (`trackers` table, #193 / ADR-040) and is never exposed in the API nor travels in the profile.

- `PUT /api/v1/trackers/{id}/profile/image-host` — requires `trackers:write`. Body is the image host Spec (#193, ADR-040): `{"name", "upload_url", "file_field", "auth_header", "auth_prefix", "url_path", "thumbnail_path"?}`. Records the `image_host` section in the profile and responds `200` with the profile. Does not touch the taxonomy nor the `description`. Audited: "Edited the image host of tracker 'X'". The Spec is validated at every write point (the `PUT`, import, and export): `name` with name rules (and recorded normalized, like the term name: collapsed spaces); absolute `upload_url`, `https`, DNS host (no literal IPv4 or IPv6 IP, no `localhost` nor `*.localhost`, no host whose last label is only digits or `0x…` — `127.1`, `0x7f.1` —, no port, no userinfo, no fragment), up to 2048 characters; `file_field` `[A-Za-z0-9_.-]{1,64}`; `auth_header` header token `[A-Za-z0-9-]{1,64}`, excluding `Host`, `Content-Type`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Cookie`, `Upgrade`, `TE`, `Trailer`, and `Proxy-*` (case-insensitive); `auth_prefix` 0 to 32 printable ASCII characters; `url_path` 1 to 5 segments `[A-Za-z0-9_]{1,64}` separated by dots; `thumbnail_path` optional, same rules. Rejection: `400 invalid_profile` with the field path (`image_host.upload_url: scheme "http" (só https)`); the message never returns the received value in full.
- `DELETE /api/v1/trackers/{id}/profile/image-host` — requires `trackers:write`. Deletes the section: the tracker starts using only the global imgbb in the screenshot chain. Idempotent (deleting what does not exist also yields `200`). Audited: "Removed the image host of tracker 'X'".
- `GET /api/v1/trackers` (and the responses of `POST`/`PUT /api/v1/trackers`) — the `profile` gets `image_host` **only when the tracker has its own host** (the key does not even exist in the default profile).
**Operator key bound to the host (#193 / ADR-040).** The key that the operator registers for the tracker's image host is **layer 3** (operator credential, `trackers` table), never layer 2 (site profile). It is **bound to the host**: when recording, the CP stores along with it the DNS name of the current `upload_url` (`Spec.Host()`), and the screenshot chain checks if the profile's host still matches before using the key — a key registered for another host is skipped with a reason. The key is **never** exposed in the API: the listing only exposes the boolean `has_image_host_key` and the host (`image_host_key_host`).

- `PUT /api/v1/trackers/{id}/image-host-key` — requires `trackers:write`. Body `{"key": "..."}`; the key is recorded **trimmed** (no spaces or line breaks at the ends). Empty key (or only spaces) → `400 invalid_request`; profile without `image_host` → `409 profile_without_image_host`; tracker from another operator or malformed id → `404 tracker_not_found`. Records the key **along with the host of the current `upload_url`** (`Spec.Host()`). Responds `{"status": "ok"}`. Audited: "Registered the image host key 'host' on tracker 'X'" — the text includes the host, **never** the key.
- `DELETE /api/v1/trackers/{id}/image-host-key` — requires `trackers:write`. Deletes the key and the host bound to the tracker. Idempotent on the absence of the key (deleting what is already deleted also yields `200`); the tracker must exist and belong to the operator (`404` otherwise). Responds `{"status": "ok"}`. Audited: "Removed the image host key of tracker 'X'".
- `GET /api/v1/trackers` — each tracker gets `has_image_host_key` (boolean) and `image_host_key_host` (the DNS host for which the key was registered, empty if there is no key).

**Closed form and validation.** The Spec describes an image host of the "multipart upload" type: always `POST multipart/form-data` with a file part (the JPEG frame, `image/jpeg`); the key goes **only** in the `auth_header` header, preceded by `auth_prefix`; the response is JSON and the image URL comes out via a **dotted path** (`data.link`, `image.url`; a numeric segment indexes a list). No field carries the key, and no field name suggests it (`auth_header` says **where** the key goes, not what it is). Validation is on the **name** (DNS), not the address: a name that resolves to an internal IP passes — the real defense is the key, which the operator registers **for that name** (#193 / ADR-040). A host outside this form (key in the query or body, non-JSON response, two-step upload) is a **new family**: PR with code, as ADR-037 already foresees.

**Where it lives.** In the **same** JSONB column of the profile (`tracker_profiles.taxonomy`), as the `image_host` key **alongside the axes** — without migration, without a new column, and without rewriting any rows: the document that already exists is, byte by byte, a new document without the section. `schema_version` **does not change** (remains `1`): the section is optional and additive, and the export omits it while the profile does not have it, so a profile without it remains exactly the v1 file. The accepted cost: a SeedGrid **prior to #193** rejects a file that contains `image_host` with `campo desconhecido` — and **not** with `unsupported_schema_version`.

**In export and import.** The file gains the `image_host` key after `description` (or after `taxonomy`, without `description`), **only when the tracker has its own host** — the Document embeds the Profile, so the section is exported without changing the export, and the export goes through the same validation as the import (an invalid section manually written to the database causes the export to fail with `409 profile_not_exportable` and the reason). In the import, the section is **a single unit**, with no per-term origin, and the rule is that of the `manual`, applied to it as a whole: profile **without** its own host → **apply** (`new`); equal → nothing (`same`); **different → protected** (`protected`): it stays as is and `plan.warnings` says it was kept, unless the operator requests `?overwrite=image_host` — then it **replaces** (`replaced`). A file that does not contain the section **does not touch** the profile's section. The plan gains `image_host` — `{status: new | same | protected | replaced, config, current?, overwrites_protected?}`, present only when the file contains the section. The taxonomy and sections come from the same plan, calculated on the locked row, and are written **together**. Audit: the apply operation appends «; image host: applied | replaced on request | kept the operator's | was already equal» **only when the file contained the section**. The operator's key for the host (layer 3) **never** goes into the file: it stays in `trackers` (#193 / ADR-040) and the export only reads `tracker_profiles`.

# **8. Node API**
Responsible for communication between the Agent and the Control Plane.
Planned operations:
Registration.
Heartbeat.
Inventory.
Events.
State update.
Command reception.
The Agent must never use endpoints intended for the Portal.

## 8.1 Heartbeat and behavior configuration (ADR-039, #144)
`POST /api/v1/agent/heartbeat` is also the configuration channel: the **response** delivers the effective values for the Node, and the **body** of the next heartbeat reports what the Agent applied.

Response (`200`):
```json
{
  "status": "ok",
  "agent_config": {
    "version": "a1b2c3d4e5f6",
    "values": {
      "engine.max_active_downloads": "3",
      "agent.heartbeat_interval": "10s",
      "...": "..."
    },
    "report_local": false
  }
}
```
- `values`: **all** `PerNode` keys from the catalog, with the **effective value for that Node** (override > cluster), serialized as in the catalog (`"10s"`, `"true"`, `"3"`). The Control Plane sends the **entire** desired state on every heartbeat: missing one costs nothing, the next one corrects it.
- `version`: hash (12 hex) of the values. The Agent returns it in `config_version` **after successfully applying everything**.
- `report_local: true`: the **seeding** of the overrides has not yet happened for this Node (ADR-039 §7). The Agent **applies nothing**, and reports its explicit env in `local_config` on the next heartbeat.
- The block is **omitted** if the Control Plane failed to resolve it (the heartbeat never fails because of configuration): the Agent keeps what it has already applied.

Additive fields in the heartbeat **body**:
- `config_version` (string): the `version` that the Agent successfully applied.
- `local_config` (key→value object): the **explicit** behavior env of the Node — only what the operator defined (not the code defaults), in canonical form. `{}` is a valid report (new Node, only identity in `.env`) and **must** be sent: it is what marks the seeding as done.

> ⚠️ **Compatibility — the heartbeat body is decoded with `DisallowUnknownFields`.** A new field sent by a new Agent to an **old** Control Plane would result in `400` and the Node would go `OFFLINE`. Therefore, the Agent only sends `config_version`/`local_config` **after the last successful response has brought `agent_config`** (the proof that the CP understands them), and any heartbeat failure clears this proof. Consequences: **old Agent + new CP** ignores `agent_config` and continues on env; **new Agent + old CP** does not receive `agent_config`, continues on env, and never sends a new field; a **CP rollback** with Agents already up to date costs **one** heartbeat (the only one that still carries `config_version`), not the Node (the offline limit is 90 s). Deployment order (ADR-027): Control Plane first, Agents one by one.
# **9. Administrative API**
Responsible for operations performed by the Portal.
Examples
Manage Nodes.
Manage Torrents.
Manage users.
Query events.
Query alerts.
Manage configurations.
Execute administrative operations.
# **10. Public API**
The architecture must allow future availability of a public API.
In version 1.0 it will remain disabled.
# **11. Cluster Resources**
Available operations:
Query state.
Query capacity.
Query statistics.
Query version.
Query configuration.
# **12. Node Resources**
Planned operations:
List.
Query.
Register.
Update.
Change state.
Query inventory.
Query capabilities.
Query Labels.
Query Tags.
Query history.
# **13. Torrent Resources**
Planned operations:
Add.
Query.
Search.
Pause.
Resume.
Remove.
Recheck.
Change category.
Change Tags.
Query files.
Query history.
Query statistics.
## 13.1 Inventory Analytics (#160)

`GET /api/v1/analytics` — requires `torrents:read`. Answers, in a single call, the inventory questions
the Portal could not answer: what the tracker no longer recognizes, what
is duplicated, what occupies disk without yielding ratio and where to reclaim space on each Node.

**One route only, not four.** The panels overlap (duplicated that is also
idle, idle that is also dead on the tracker) and four routes would scan the same
table four times to build ONE screen. The Control Plane does one read of the inventory and
derives all panels in memory.

**No parameters.** There is no `?limit`: the report has no example list to truncate.
The counters are always for the entire inventory — a truncated summary would lie about the size
of the problem — and `flagged` brings **all** flagged torrents, without truncating, so that
every counter on the screen has a corresponding list (#169). Previously the response brought
complete counters and lists cut at 20, and the operator saw "157 with dead swarm"
with no way to reach the 157.

Response body:

| Field | Type | Content |
|---|---|---|
| `total` / `total_bytes` | `int` / `int64` | entire inventory |
| `tracker_unknown` | `int` | still without tracker verdict (null ≠ dead) |
| `duplicates` | `DuplicateSet[]` | titles with more than one copy, from the one that frees the most to the one that frees the least |
| `idle` | `IdleSummary` | `never_uploaded{,_bytes}` and `dead_swarm{,_bytes}` |
| `nodes` | `NodeUsage[]` | per Node: `torrents`, `bytes`, `idle_bytes`, `reclaim_bytes` |
| `flagged` | `Torrent[]` | **all** flagged ones, with the flags that classify them |

Each `Torrent` in `flagged` carries the flags `never_uploaded`, `dead_swarm`,
`tracker_dead` and `duplicate_extra` (plus `duplicate_of`, the group label). These are what
link the counter to the list.

`DuplicateSet` groups by **title, year, season/episode and part**. The resolution stays
OUT of the key on purpose — it is precisely what distinguishes the copies within a group.
The **part** is included because `release.Parse` cuts the alternative title after " - " (the
tracker rule, ADR-020) and collapsed "Fase 1" and "Fase 2" into the same title, marking one
work as an excess copy of the other (#172). `identical` marks the only case of pure waste
— same release, same size, on different Nodes; different resolution or release
the tracker accepts as two torrents and both yield ratio.

**There is no ratio ranking in this route.** The `top_ratio` was removed: it was the only example list
of the report, and the ranking started to be calculated in the Portal over the complete list
of `GET /api/v1/torrents` (which without `?limit` returns the entire inventory), alongside the other
Dashboard rankings (#174).

**Read-only** route: nothing here removes nor queues a command. Removal continues
being the torrent `DELETE`, through the same audited path (ADR-025).

## 13.2 Torrent in migration (#87)

`POST /api/v1/torrents/{id}/pause`, `/resume`, `/recheck` and `DELETE /api/v1/torrents/{id}` respond
**`409 migration_in_flight`** while the torrent participates in a `PENDING` migration (transferring or with the
destination recheck in verification) — be it the migrated torrent, at the source, be it the **copy of the same hash** that the
destination Node adopted. The refusal leaves no trace: nothing is queued nor soft-deleted. Try again when the
migration finishes (`COMPLETED` or `FAILED`, see `GET /api/v1/migrations`). Migration still `QUEUED` does not
block anything, and other torrents from the same Nodes remain free.

The 409 message says it is possible to **cancel the migration** and points out which one (the `id` in the message itself — it applies
also when the action was on the destination copy, whose migration the operator would have no way to discover).

`POST /api/v1/migrations/{id}/cancel` — requires `torrents:migrate` (Administrator and Publisher); audited
(ADR-025). Ends the `QUEUED` or `PENDING` migration through the **same safe watchdog path**: the migration becomes
`FAILED` with `error = "cancelada pelo operador"` (and `cancelled: true` in the response), the source is resumed and the copy
of the destination is discarded. Responds `200` with the migration. There is no new state in the enum.

- **During the copy** (the destination Agent is in `migrate_pull`): the command is not interrupted; the migration becomes
  `FAILED` right away and the cleanup (`remove_torrent` with data) is queued behind the pull — the copy finishes and is
  discarded next. Commands that the Agent has not yet claimed are downloaded and never run.
- **`404 migration_not_found`** — non-existent migration (or malformed id).
- **`409 migration_finished`** — the migration has already finished (completed, or failed on its own): there is nothing to cancel.
- **`409 migration_already_cancelled`** — it was already cancelled. **Repeating is safe:** nothing is re-executed.
- Finishing and cancelling at the same time never win both: the loser has no effect (see ADR-018 addendum).

`POST /api/v1/torrents/{id}/migrate` also refuses (`409 already_migrating`) to migrate the copy back while
the outbound is in flight.

Why: the Agent stopped holding the Node command queue while waiting for the recheck, so an action on
the copy would be executed **in the middle** of the verification (pausing takes it down; removing erases what the Control Plane is
about to consider valid). See the addendum (#87) of ADR-018.

# **14. User Resources**
Planned operations:
Create.
Edit.
Disable.
Change password.
Query permissions.
Query sessions.

## 14.1 Role-based authorization (ADR-035)

Since ADR-035, every administrative API resource requires operator authentication
**and** a specific permission in the format `resource:action`. The Control Plane resolves
the operator permissions from the associated roles (`user_roles` →
`role_permissions` → `permissions`).

System roles:

| Role | Access |
|---|---|
| `Administrator` | Total (wildcard) |
| `Publisher` | Manages content: torrents, trackers, publishing |
| `Downloader` | Reads and downloads content |
| `Viewer` | Read-only |

User management endpoints (require `users:read`/`users:write`, admin):

- `GET /api/v1/users` — lists operators with roles
- `GET /api/v1/users/{id}` — operator with roles
- `POST /api/v1/users` — creates operator (username, password, roles)
- `PUT /api/v1/users/{id}` — updates name/email/status/roles
- `DELETE /api/v1/users/{id}` — disables (soft delete) and invalidates sessions
- `POST /api/v1/users/{id}/reset-password` — resets password
- `GET /api/v1/auth/me` — authenticated operator profile (roles/permissions)
- `POST /api/v1/auth/password` — changes own password (any authenticated user)

Protections:
- It is not possible to disable or remove the administrator role from the **last**
  active administrator (`409 last_admin`) — the count ignores soft-deleted
  administrators and non-ACTIVE users.
- A disabled user cannot authenticate; existing sessions are invalidated
  (changing status to non-ACTIVE revokes access immediately, even with a
  live token — `Authenticate` filters `status = 'ACTIVE'`).
- Permission denied responds with `403 forbidden` and **enters the audit trail**
  (ADR-025): `RequirePermission` runs inside `AuditMutations`.
- `POST /torrents/create` (create .torrent from local content, ADR-019) requires
  its own `torrents:build` permission — distinct from `torrents:create`
  (add magnet/file for download).
# **15. Monitoring Resources**
Query:
Health Score.
Heartbeats.
Node State.
Cluster State.
Operational summary.
# **16. Configuration Resources**
Query settings.
Change settings.
Query Feature Flags.
Query system variables.
# **17. Event Resources**
Query events.
Search events.
Filter by Node.
Filter by Torrent.
Filter by period.
Filter by severity.
# **18. Alert Resources**
Query alerts.
Acknowledge alert.
Resolve alert.
Query history.
# **19. Goals**
The API must allow any client to fully administer the Cluster.
No Portal feature must depend on internal Control Plane mechanisms.
Every feature must be available through the API.
# **20. Fundamental Principle**
The API represents the official boundary of SeedGrid.
All integration with the platform must occur through it.
It defines the architecture contracts and must remain stable throughout the system's evolution.
# **21. General Standards**
All operations must follow consistent conventions.
The API must use HTTPS exclusively.
Data must be serialized in JSON.
All timestamps must use UTC.
All responses must have a uniform structure.
# **22. Resource Structure**
All resources must follow the same logical organization.
Example:
/api/v1/
├── auth/
├── cluster/
├── nodes/
├── torrents/
├── trackers/
├── users/
├── events/
├── alerts/
├── monitoring/
├── scheduler/
├── settings/
└── system/
Each domain will be responsible only for its own set of operations.
# **23. Methods**
The API must follow REST conventions.
GET
Query information.
POST
Create resources.
Execute operations.
PUT
Completely replace a resource.
PATCH
Partially update a resource.
DELETE
Request logical removal of a resource.
# **24. Pagination**
Operations that return collections must support pagination.
Examples:
Nodes.
Torrents.
Events.
Alerts.
Users.
Pagination must be consistent across all resources.
# **25. Search**
Whenever applicable, the API must allow:
Text search.
Sorting.
Filters.
Pagination.
Field selection.
These mechanisms must follow common conventions across all endpoints.
# **26. Asynchronous Operations**
Some operations may require a long time.
Examples:
Recheck.
Torrent migration.
Inventory update.
Backup.
In these cases, the API must immediately return the request confirmation.
The progress of the operation will be queried later through the appropriate resources.
# **27. Errors**
Every error response must be consistent.
Minimum information includes:
Code.
Message.
Description.
Timestamp.
Operation identifier.
When applicable, suggestions for resolution may be included.

**The code is the contract; the message is the fallback (ADR-041).** In the response `{"error": {"code", "message", "timestamp"}}`, the `code` is stable and is what a client must interpret. The `message` is a human-readable sentence in the cluster's language — it serves those reading the raw response and clients that do not know that code — and its wording may change. The Portal translates known codes to the operator's language and uses the `message` as a fallback and as detail. When the sentence depends on a number or a name, it comes in `error.params`, not embedded in the text.
# **28. Idempotency**
Operations that can be repeated without side effects must be idempotent.
Examples:
Pause an already paused torrent.
Put a Node in DRAIN when it is already in DRAIN.
These operations must return success without generating additional changes.
# **29. Agent Resources**
Communication between Agent and Control Plane will use specific endpoints.
Responsibilities:
Registration.
Heartbeat.
Inventory.
Events.
Command reception.
Synchronization.
These resources must not be used by the Portal.
# **30. Portal Features**
The Portal will use exclusively administrative endpoints.
Examples:
Dashboard.
Search.
Configuration.
Torrent Operations.
Node Operations.
User Management.
# **31. Scheduler Features**
The Scheduler will be accessed only by the Control Plane.
There will be no public endpoints for direct calculation of decisions.
The information made available to the operator will be read-only.
Examples:
Decision history.
Criteria used.
Statistics.
Algorithm version.
# **32. Monitoring Features**
The API must allow querying:
Overall Cluster state.
State of each Node.
Operational summary.
Health Scores.
Heartbeats.
Active Alerts.
This information will be used primarily by the Portal.
# **33. Event Features**
The API must provide efficient mechanisms for querying events.
Recommended filters:
Node.
Torrent.
User.
Type.
Severity.
Period.
Free text.
# **34. Configuration Features**
All Cluster configuration must be managed by the API.
Examples:
Scheduler.
Alerts.
Authentication.
Retention.
Feature Flags.
Integrations.
No global configuration must depend on direct database access.
# **35. Security**
All endpoints must require authentication, except those explicitly intended for the initial registration of an Agent.
Authorization must be based on permissions defined by the Control Plane.
# **36. Audit**
Every administrative operation initiated by users must generate an audit log.
Automatic operations between the Agent and the Control Plane must not generate audit logs, only operational events.
# **37. Compatibility**
New API versions must preserve compatibility whenever possible.
Incompatible changes must result in a new API version.
Previous versions may coexist during a transition period.
# **38. Principles**
The API represents a contract.
Internal implementations may evolve freely, as long as the contracts remain stable.
This separation must allow independent evolution of the Portal, the Control Plane, and the Agents.
# **39. Communication Flows**
The API must support the platform's main operational flows.
**Node Registration**
Agent
↓
Registration
↓
API Gateway
↓
Authentication Service
↓
Node Service
↓
Database
↓
Response
**Heartbeat**
Agent
↓
Heartbeat
↓
API Gateway
↓
Monitoring Service
↓
Database
**Torrent Addition**
Portal
↓
API Gateway
↓
Torrent Service
↓
Scheduler
↓
Node Service
↓
Agent
↓
Engine
**Information Query**
Portal
↓
API Gateway
↓
Responsible Service
↓
Database
↓
Response
Queries must preferably use persisted data.
# **40. Response Times**
Version 1.0 goals.
Authentication
Less than 500 ms.
Node Query
Less than 300 ms.
Torrent Query
Less than 500 ms.
Dashboard
Less than 1 second.
Heartbeat Registration
Less than 100 ms.
These values represent architectural goals.
# **41. Rate Limiting**
The API must provide request rate limiting mechanisms.
Goals:
Protect the Control Plane.
Prevent abusive use.
Prevent accidental loops.
Limiting criteria may vary according to the consumer type.
Examples:
Portal.
Agent.
CLI.
Integrations.
# **42. Availability**
Temporary API unavailability must not interrupt the operation of the Nodes.
Agents must continue operating locally until communication is restored.
After reconnection, synchronization must occur automatically.
# **43. Documentation**
Every feature provided by the API must be documented.
The documentation must include:
Description.
Parameters.
Required permissions.
Possible responses.
Usage examples.
This documentation must be generated automatically whenever possible.
# **44. Evolution**
New features must be added without changing existing contracts.
When incompatible changes are inevitable:
New API version.
Coexistence period.
Documented migration.
Planned deprecation.
# **45. Future Integrations**
The architecture must allow integration with external tools.
Examples:
Official CLI.
SDK.
Mobile apps.
External dashboards.
Automation.
Webhooks.
These integrations will use exactly the same contracts defined in this document.
# **46. Dependencies**
The API will depend only on the Control Plane.
All business rules will remain located in internal services.
The API must never implement its own business logic.
Its responsibility is limited to:
Receive.
Validate.
Forward.
Respond.
# **47. Monitoring**
The API must provide sufficient information for operational tracking.
Examples:
Number of requests.
Average response time.
Errors.
Authentications.
Active sessions.
Running version.
This information will be consumed by the monitoring system.
# **48. Testability**
All contracts must be testable independently of the implementation.
Whenever possible, automated tests must validate:
Response structure.
Authorization.
Compatibility.
Versioning.
Error handling.
# **49. Architecture Summary**
Portal
│
────────────┼────────────
CLI
│
────────────┼────────────
Agent
│
────────────┼────────────
Future Integrations
│
════════════════════════════
API Gateway
════════════════════════════
│
Control Plane
════════════════════════════
Internal Services
════════════════════════════
PostgreSQL
# **50. Final Statement**
The API constitutes the official communication contract for SeedGrid.
It decouples consumers and internal services, preserving stability, security, and consistency throughout the platform's evolution.
All interactions with SeedGrid must occur exclusively through this interface, ensuring that the architecture remains modular, extensible, and independent of internal implementations.
