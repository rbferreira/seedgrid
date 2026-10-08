# SeedGrid — Similar Projects Landscape (open-source prep)

> **Non-normative document.** It is not part of the specification (it does not enter the precedence
> ADR → Architecture → Requirements). It is support material for the **open-source launch positioning**
> — what already exists out there, where SeedGrid is different, and who
> is the UX reference. Surveyed in **Jul/2026**, reviewed in **Sep/2026** (revalidate before
> publishing).

## TL;DR

SeedGrid lives on **two axes**, and the answer is different for each.

**Orchestration axis — the niche is empty.** No **maintained** project does what SeedGrid
does in its entirety. The space is divided into (1) **multi-instance management UIs** for qBittorrent
— they operate multiple clients, but **do not orchestrate**; (2) the **architectural twin**
(OpenSeedbox) — the same idea of a central brain + "dumb" nodes with placement, but
**abandoned** and with the Scheduler **never implemented**; (3) single-node remote clients.
SeedGrid fills the empty niche: an **orchestration platform** (Control Plane + Agents
+ Scheduler) that treats the nodes as **a Cluster** and actually decides (placement, admission,
migration, publishing).

**Publishing axis — the niche is contested, and we are behind.** The survey from
Jul/2026 did not look at this side. There are two mature tools for preparing and sending
releases (§4), one of them in Go, active, and which **already publishes to the same tracker as
SeedGrid** — territory that ADR-020 assumed without comparing. They do not orchestrate anything, but
their publishing lane is years more mature than ours.

## 1. Multi-instance management UIs (do not orchestrate)

| Project | Stack | Active | What it does | Why it is NOT SeedGrid |
|---|---|---|---|---|
| **autobrr/qui** | Go + TS, single-binary | **Yes** (v1.22.0, Jun/2026) | Panel to manage N qBittorrent instances; cross-seed; rule-based automations; backups | Treats each instance as a **separate entity** (you switch between them). No automatic placement, no balancing, no cluster-wide ceiling, no Agents deciding. It is a **dashboard**, not an orchestrator. |
| **qBitWebUI** (mkbula) | Modern Web | Yes | Multi-instance UI, cross-seed, Prowlarr integration, real-time monitor | Same: management/visualization, not orchestration. |
| **jesec/flood** | Node/React | Yes | Front-end for a client (multi-instance limited, under discussion) | Client front-end, does not decide placement. |
| **StuffAnThings/qbit_manage** | Python | Yes | Maintenance (tags, cleanup, cross-seed) via cron | Maintenance tool, single/multi instance; does not place torrents between nodes. |

**UX Reference:** **qui** is beautiful, fast (single-binary Go, like our CP) and
active — the **most direct competitor/benchmark** on the "operate multiple qBittorrent" side. It is worth
following its features and polish when we position SeedGrid.

## 2. Architectural twin — OpenSeedbox (abandoned)

- **Same central idea:** frontend stores **all state** + "dumb" backend nodes that only
  download, with **torrent placement between nodes** (`openseedbox/openseedbox`).
- **But:** Java/**Play Framework** + **transmission-daemon** (not qBittorrent); oriented towards
  **multi-user** (hosting product); **dormant/experimental** (README: "for
  developers"); and — most importantly — **"the node selection algorithm was never
  implemented"**. The *heart* (Scheduler) never left the paper.
- **Does not have:** admission/ceiling per tracker account, migration between nodes, publishing/mediainfo,
  UNIT3D, health monitoring, config hot-reload.

## 3. Self-hosted remote clients (single-node, out of scope)

boypt/simple-torrent (formerly Cloud Torrent), jpillora/cloud-torrent, exatorrent, docker seedbox
stacks (plcharriere/seedbox). They are **a single** remote client with a UI — there is no notion of Cluster.

## 4. Upload assistants — the publishing axis (surveyed Sep/2026)

Category that the Jul/2026 survey **did not cover**. These are tools that prepare and
send a release to a private tracker: media analysis, metadata, screenshots,
description, dupe check, `.torrent` creation, and POST. It is exactly the lane of ADR-019 /
ADR-020 / ADR-026 / ADR-030.

| Project | Stack | Active | What it does | Relationship with SeedGrid |
|---|---|---|---|---|
| **wastaken7/Upload-Assistant** (Audionut fork) | Python | **Yes** | **105 trackers** + 3 Usenet indexers. ffmpeg/MediaInfo/BDInfo/DVD, TMDB/IMDB/TVDB/MusicBrainz/Discogs/IGDB, screenshots + spectrograms, **17-block** description builder with per-tracker override, batch queues, CLI + Web UI + Docker | Covers books, music, games, and Usenet — outside our use case. But the **description builder** and **coordination with qBittorrent bandwidth** are directly applicable |
| **autobrr/upbrr** | **Go** | **Yes** (alpha, daily push) | **8-stage** flow with human review at each stage; **per-site profile** (`profile.go` / `taxonomy.go` / `rules.go` / `validation.go` / `auth.go` / `description.go`); dupe check as *evidence*, not as a block; frozen payload preview; client injection; config migration from Upload-Assistant | **From the same org as `qui`** (§1). And **already publishes to samaritano**: `internal/trackers/impl/unit3d/sites/sam/profile.go` declares `Name: "SAM", BaseURL: "https://samaritano.cc"` |

**Neither of them orchestrates anything.** They are single-host tools that prepare a release: there is
no Node, Cluster, placement, admission, or migration. The overlap with SeedGrid is
strictly the publishing lane — but there the difference is large and to our disadvantage.

### Where we are behind (set/2026)

| | SeedGrid today | UA / upbrr |
|---|---|---|
| dupe check on the tracker | **does not exist** (`unit3d.Browse` is ready, only Explore uses it) | own stage, result treated as evidence |
| eligibility rules | **none** — validates name/description/category/type and fires | `rules.go` + `validation.go` per site, banned groups |
| tracker profile | scattered: taxonomy **in the frontend**, name format in the generic package, API shape fixed | manifest per site; new site = ~25 lines |
| description builder | 82 lines of TS **in the Portal** | 17 blocks (UA) / package per site (upbrr), server-side and testable |
| metadata | TMDB + IGDB (`tvdb` is always sent as zero) | IMDB, TMDB, TVDB, TVMaze, TheXEM, BluRay.com |
| image host | one, fixed | policy with fallback; the SAM profile states that **the site itself hosts it** |
| submission preview | there is none — "without dry-run, a real POST is the only test" | **frozen** operation: the preview and submit use the same state |

Remediation and sequencing plan: issue **#184** (epic) and its children #185–#195.

### The strategic decision — resolved in ADR-037

ADR-020 built its own UNIT3D integration without comparing it to what already existed. With
upbrr on the map the choice became real, and there were two options:

- **A — in-house publishing.** SeedGrid publishes on its own. Cost: the knowledge of each
  site needs to enter the system somehow. Gain: publishing is **part** of the
  orchestration — the `.torrent` is created, seeded, and published in the same flow, with the
  Scheduler choosing the Node.
- **B — delegate.** SeedGrid orchestrates, upbrr publishes. Gain: 100+ trackers without maintaining
  any. Cost: external dependency in alpha, and the release is no longer seeded upon creation in the
  cluster — the ADR-019/020 integration, validated in production, is lost.

**Option A adopted**, in **ADR-037**. What decided it was not the tracker count: it was that
publishing is a step of the orchestration, not a step alongside it.

The cost of A — "and the knowledge of each site, where does it come from?" — is what ADR-037 answers,
and the answer is what makes A viable in an open-source project: the tracker profile is
**contributable data**, not maintainer code. Anyone with an account on the site discovers the
taxonomy from the tracker's own catalog (the UNIT3D API returns name and id side
by side), adjusts, exports, and shares. The maintainer never needs to have an account
anywhere — which is exactly the ceiling that the upbrr model, with one Go package per site, imposes
on itself.

## SeedGrid Differentiation (the empty niche)

> This applies to the **orchestration axis**. In the publishing axis (§4) the differentiation is
> lower and SeedGrid is the entrant, not the incumbent.

The *building blocks* (managing multiple qBittorrent, distributed seedbox) exist as **management/UI
products**. SeedGrid is an **orchestration platform** with a combination that no
active project delivers:

- **Control Plane + Agents** (ADR-012, agent-initiated by polling) and a **Scheduler** that
  decides placement — which OpenSeedbox promised and did not deliver.
- **Cluster-wide admission** tied to the tracker **account's DL slots limit** (ADR-021),
  peer-aware, with slow/dead rotation — not found in any other project.
- **Torrent migration between Nodes** (ADR-018), **publishing/creation** with mediainfo +
  **UNIT3D integration** (ADR-019/020), **config hot-reload** (ADR-023), **Manual/Auto mode**
  (ADR-024), **Orchestration Feed** (ADR-022), **Audit** (ADR-025), health/observability.

## Implications for the open-source release

- **Positioning:** do not compete as "just another qBittorrent UI" (qui already does this well),
  but rather as the **"Kubernetes of seeding"** / BitTorrent cluster orchestrator — the Scheduler +
  cluster-wide admission + migration are the pitch.
- **Risk:** qui is active and may grow towards orchestration; keep an eye on it. The same org
  launched **upbrr** (§4) — autobrr is occupying the two axes adjacent to ours.
- **Opportunity:** the only one that aimed for orchestration (OpenSeedbox) is dead → there is room and an
  orphaned user base (distributed seedbox with placement).
- **Do not sell publishing as a differentiator.** It is part of the product, but compared to UA and
  upbrr our publishing lane is shallow (§4). The pitch is orchestration.
- Revalidate this survey before publishing (the landscape changes fast).

## Sources

**jul/2026:**

- autobrr/qui — https://github.com/autobrr/qui
- qBitWebUI — https://github.com/Maciejonos/qbitwebui
- qbit_manage — https://github.com/StuffAnThings/qbit_manage
- flood — https://github.com/jesec/flood
- OpenSeedbox — https://github.com/openseedbox/openseedbox · https://www.openseedbox.com/
- simple-torrent — https://github.com/boypt/simple-torrent · cloud-torrent — https://github.com/jpillora/cloud-torrent

**set/2026 (publishing axis):**

- wastaken7/Upload-Assistant — https://github.com/wastaken7/Upload-Assistant
- autobrr/upbrr — https://github.com/autobrr/upbrr
  (Samaritano profile: `internal/trackers/impl/unit3d/sites/sam/profile.go`;
  profile template: `ADDING_TRACKERS.md`; 8-stage workflow:
  `documentation/docs/workflow/index.md`)
