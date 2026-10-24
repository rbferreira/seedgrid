# **SeedGrid**
# **Architecture Decision Records (ADR)**
Version: 1.0
Status: Draft
Document: ADR.md
# **1. Objective**
This document records the main architectural decisions adopted during the development of SeedGrid.
Its goal is to preserve the technical context that motivated each decision, facilitating the evolution, maintenance, and future reviews of the platform.
# **2. Philosophy**
An architectural decision represents a conscious choice among different possible alternatives.
Each decision recorded in this document must answer:
- what problem existed;
- what alternatives were considered;
- what solution was adopted;
- what benefits were obtained;
- what limitations were accepted.
# **ADR-001**
# **Centralized Architecture**
**Decision**
SeedGrid uses a central Control Plane responsible for coordinating the entire Cluster.
**Context**
It was possible to adopt a distributed architecture, where each Node would make local decisions.
This approach would significantly increase the complexity of the platform.
**Decision**
All intelligence remains in the Control Plane.
Nodes perform only local operations.
**Consequences**
Benefits:
- simple architecture;
- consistent decisions;
- centralized monitoring;
- lower coupling.
Limitations:
- dependency on the Control Plane for administrative operations.
# **ADR-002**
# **Independent Nodes**
**Decision**
Seed Nodes never establish direct communication with each other.
**Context**
Distributed clusters normally use communication between the Nodes themselves.
However, this increases synchronization, service discovery, and fault handling.
**Decision**
All communication occurs through the Control Plane.
**Consequences**
Benefits:
- isolation;
- simplicity;
- scalability;
- ease of replacement.
Limitations:
- lack of direct coordination between Nodes.
# **ADR-003**
# **Stateless Agent**
**Decision**
The Agent does not store permanent Cluster information.
**Context**
It was possible to keep part of the state distributed among the Nodes.
This approach would make recovery and synchronization difficult.
**Decision**
The Agent keeps only the information necessary for its local operation.
The official State remains in the Control Plane.
**Consequences**
Benefits:
- fast replacement;
- simple recovery;
- lower risk of inconsistencies.
Limitations:
- dependency on the Control Plane for administrative synchronization.
# **ADR-004**
# **Centralized Scheduler**
**Decision**
All distribution decisions are executed by the central Scheduler.
**Context**
Another alternative would be to allow local decisions.
This would make global balancing difficult.
**Decision**
The Scheduler uses a complete view of the Cluster.
**Consequences**
Benefits:
- better distribution;
- consistent decisions;
- more sophisticated algorithms.
Limitations:
- need for communication with the Control Plane.
# **ADR-005**
# **Decoupled Portal**
**Decision**
The Portal communicates exclusively with the API.
**Context**
It was possible to allow direct access to the Nodes.
This approach would increase coupling and authentication complexity.
**Decision**
All administrative communication must pass through the API.
**Consequences**
Benefits:
- security;
- standardization;
- decoupling;
- simplicity.
Limitations:
- dependency on the API for all operations.
# **ADR-006**
# **PostgreSQL as Official State**
**Decision**
PostgreSQL stores the entire State of the Cluster.
**Context**
Evaluated alternatives:
- SQLite;
- distributed databases;
- file-based storage.
**Decision**
Use PostgreSQL as the single official source of persistence.
**Consequences**
Benefits:
- consistency;
- reliability;
- maturity;
- ease of backup.
Limitations:
- need for an additional service.
# **3. Principle**
Every decision recorded in this document must remain valid until a new ADR formally replaces it.
Architectural changes must never occur implicitly.

# **ADR-007**
# **Decoupled Monitoring**
**Decision**
The monitoring system remains independent of the platform's operation.
**Context**
It was possible to incorporate metrics and dashboards directly into the Control Plane.
This approach would increase the coupling between operation and observability.
**Decision**
Prometheus, Grafana, and other observability components remain separate from the operational core.
**Consequences**
Benefits:
- lower coupling;
- ease of replacement;
- adoption of widely used standards;
- greater flexibility.
Limitations:
- need for additional components.

> Refined and partially superseded by ADR-016: metric **collection** remains decoupled (Prometheus), but **visualization** now lives in the product (Portal, via the Control Plane API), and Grafana is removed.
# **ADR-008**
# **Alerts Based on Metrics and Events**
**Decision**
The Alert Service exclusively uses metrics, events, and known platform states.
**Context**
An alternative would be to analyze logs for alert generation.
This approach would increase complexity, processing cost, and unpredictability.
**Decision**
Logs remain dedicated exclusively to technical investigation.
Alerts are generated only from structured information.
**Consequences**
Benefits:
- deterministic rules;
- lower resource consumption;
- ease of testing;
- fewer false positives.
Limitations:
- some conditions can only be identified during manual log analysis.
# **ADR-009**
# **Tool-Agnostic Backup**
**Decision**
SeedGrid will not depend on a specific backup solution.
**Context**
There are several mature tools for data protection.
Incorporating a specific solution would create unnecessary dependencies.
**Decision**
The platform will define policies, track executions, and present information to the operator, remaining independent of the tool used.
**Consequences**
Benefits:
- flexibility;
- compatibility;
- freedom for the operator.
Limitations:
- initial deployment requires configuration of the chosen tool.
# **ADR-010**
# **Optional Torrent Backup**
**Decision**
Torrent data backup will be optional.
**Context**
Torrent files can represent large volumes of data.
In many scenarios, their protection cost outweighs their operational value.
**Decision**
The platform will primarily protect the Cluster State.
Torrent data may follow an independent policy defined by the operator.
**Consequences**
Benefits:
- simplicity;
- lower operational cost;
- greater flexibility.
Limitations:
- loss of files when no protection policy exists.
# **ADR-011**
# **Ephemeral Components**
**Decision**
Infrastructure components must be considered replaceable.
**Context**
It was possible to design the platform assuming permanent equipment.
This approach increases dependency on specific hardware.
**Decision**
Containers, Raspberry Pis, disks, and virtual machines can be replaced whenever necessary.
**Consequences**
Benefits:
- fast recovery;
- reduced downtime;
- resilient architecture.
Limitations:
- requires discipline in documentation and backup policies.
# **ADR-012**
# **Agent-Initiated Communication**
**Decision**
All communication between Seed Nodes and the Control Plane will be initiated by the Agent.
**Context**
Allowing connections initiated by the Control Plane would require opening additional ports, reverse NAT, or discovery mechanisms.
**Decision**
Agents will establish periodic and authenticated communication with the Control Plane.
**Consequences**
Benefits:
- simplified deployment;
- compatibility with home networks;
- reduced service exposure;
- enhanced security.
Limitations:
- commands depend on the availability of the Agent-initiated channel.
# **ADR-013**
# **Service-Oriented Architecture**
**Decision**
Each core responsibility will be implemented as an independent service.
**Context**
An alternative would be to develop a monolithic application.
Although simpler initially, this approach would hinder evolution and maintenance.
**Decision**
Portal, API, Scheduler, Alert Service, Monitoring, and other components will remain modularized.
**Consequences**
Benefits:
- low coupling;
- simplified maintenance;
- independent evolution;
- ease of testing.
Limitations:
- a larger number of running components.
# **ADR-014**
# **Command Delivery via Polling in V1**
**Decision**
In version 1.0, the Control Plane will deliver commands to Agents through a durable command queue, periodically polled by the Agent.
**Context**
Several documents describe the logical flow "Control Plane → Agent → Engine", while ADR-012 establishes that all communication is initiated by the Agent and that the Control Plane never opens connections directly to the Node.
It was necessary to explicitly define, for V1, how a command reaches the Agent without violating ADR-012.
Alternatives considered:
- polling a command queue (Agent-initiated);
- persistent WebSocket/SSE channel initiated by the Agent;
- connection initiated by the Control Plane towards the Agent.
**Decision**
The Agent will periodically poll a Control Plane command queue endpoint, execute the received commands, and confirm the result.
The persistent channel (WebSocket/SSE), also initiated by the Agent, is planned for V2 as an additive evolution on the same queue contract (see Roadmap §13).
The connection initiated by the Control Plane was discarded because it requires inbound ports on the Nodes, breaks scenarios with NAT/remote Nodes, and expands the security surface.
**Relationship with ADR-012**
This decision refines ADR-012 and preserves it entirely: polling is an Agent-initiated communication.
**Consequences**
Benefits:
- respects the Agent-initiated model of ADR-012;
- works behind NAT and with remote Nodes;
- durable, idempotent, and auditable queue;
- commands wait for the Agent when it is offline;
- migration to the persistent channel is additive, with no redesign.
Limitations:
- delivery latency limited to the polling interval in V1 (mitigated by the persistent channel in V2).
# **ADR-015**
# **V1 Technology Stack**
**Decision**
Version 1.0 will be implemented in Go (Control Plane and Agent) and React (Portal), on top of PostgreSQL 17 and the Docker ecosystem.
**Context**
The engineering documentation (Development Guide and Coding Standards) is purposely language-agnostic. However, the Agent has aggressive resource goals (CPU under 2% at idle, memory under 150 MB) and must run on Raspberry Pi 3B+ (ARM64), which favors a compiled language with a low footprint.
Alternatives considered for the backend/Agent: Go, Python, Node/TypeScript.
**Decision**
- Control Plane and Agent in Go (single static binary, trivial ARM64 cross-compilation, low memory consumption).
- Portal in React with Tailwind CSS.
- Persistence in PostgreSQL 17; observability with Prometheus, Grafana, and Loki; Traefik as a reverse proxy; packaging with Docker and Docker Compose (all already fixed in the Infrastructure documentation).
**Consequences**
Benefits:
- adequate footprint for low-cost Seed Nodes;
- simple distribution (single binary) and straightforward cross-compilation for ARM64;
- mature ecosystem for APIs, concurrency, and observability.
Limitations:
- officially defines technologies for V1; relevant changes will require a new ADR.
# **ADR-016**
# **Boundary between Cluster State (product) and Observability (infrastructure)**
**Status:** Approved

**Context**
V1 deployed Prometheus + Grafana + node-exporter (ADR-015) and a "Cluster" dashboard in Grafana with Node/torrent/Alert counts. At the same time, the Portal already presents this same state live by reading the API. The question arose: why have three systems (Portal, Prometheus, Grafana) displaying "Cluster health"? Shouldn't this live only in the Portal/Control Plane?
ADR-007 already keeps observability decoupled from the operational core; ADR-006 establishes PostgreSQL as the official state; ADR-005 decouples the Portal (it only consumes the API). Monitoring.md §8 states that "Grafana will not store permanent Cluster information".

**Problem**
There is real overlap: the Grafana dashboard duplicates the domain state that is already the responsibility of the Portal/API. This confuses "where the source of truth is" and forces the operator to switch between systems for the day-to-day view. It is necessary to define the boundary between (a) Cluster state/health — product data — and (b) infrastructure observability — time series, host metrics, logs.

**Considered alternatives**
1. Status quo (literal ADR-007): all visualization, including Cluster health, via Grafana; minimal Portal. Maintains duplication, makes the operator dependent on Grafana, and contradicts Monitoring.md §8.
2. Concentrate everything in the Control Plane: the CP collects, stores, and plots metrics; remove Prometheus and Grafana. Reinvents time-series database and scraping; loses mature PromQL/alerting; high cost.
3. Grafana for infra + Portal for domain: keeps the three systems with a boundary; still requires operating and maintaining Grafana just for host charts.
4. Prometheus (+ Alertmanager) as the collection/ingestion/alerting engine and the Control Plane/Portal as the visualization layer; **remove Grafana** (recommended/adopted).

**Adopted decision** (alternative 4)
- **Prometheus** remains as the **decoupled** engine for metric collection and storage (scraping `/metrics` from the controller and node-exporters); it is the official source of metrics. **Alertmanager** handles metric threshold alerts (e.g., disk, temperature, SMART).
- **Remove Grafana.** Metric visualization becomes the responsibility of the **Portal** (product), with a chart library.
- To preserve ADR-005 ("Portal only talks to the API"), **the Portal does not query Prometheus directly**: the **Control Plane** exposes metric endpoints (`/api/v1/metrics/...`) that query Prometheus (PromQL) on the server and return series in JSON. Flow: Portal → API → Prometheus.
- **Unified Alerts in the Portal:** domain alerts remain in the Alert Worker (ADR-008); metric alerts from Alertmanager reach the CP via webhook and are persisted as SeedGrid alerts, appearing in the same place.
- Cluster domain state/health (Nodes, torrents, alerts, events) continues to come from Postgres via the API (reaffirms ADR-006). Prometheus retention configured.
- **Loki (logs) — deferred with a re-evaluation trigger, not an open MVP backlog item.** Without Grafana, Loki would require a viewer (Grafana back or a log page in the Portal) and a shipper per Node — high RAM/CPU/disk cost on the 1 GB RPi — to solve cross-host log search, which the current scale (2–3 Nodes, one operator) covers with `journalctl`/`docker logs`. **Re-evaluate when:** the fleet grows (~5–10+ Nodes) and SSH searching becomes a recurring pain, **or** a need for log pattern alerting arises. If implemented, it must be **Portal-native** (Portal → CP → Loki, like metrics). Cheap middle ground until then: journald persistence + Docker log rotation.

**Consequences**
Benefits:
- Single, cohesive UI (Portal), with product auth/UX; less operational surface (no Grafana);
- Prometheus continues doing what it does best (scraping, TSDB, PromQL, alerting), remaining decoupled and replaceable;
- single source of truth for Cluster state (Postgres via API); metrics via the CP keep the Portal talking only to the API.
Limitations:
- the Portal now builds and maintains charts (loses ad-hoc exploration and Grafana's dashboard library);
- the Control Plane gains coupling to the Prometheus query API (switching TSDBs would require rewriting the metric endpoints);
- partially reverses ADR-007 regarding visualization (dashboards now live in the product), although collection remains decoupled.

**Relationship with other ADRs**
- Refines and **partially supersedes ADR-007**: keeps observability collection decoupled (Prometheus), but moves **visualization** to the product core (Portal), reversing the ADR-007 point that kept dashboards outside the Control Plane. Removes Grafana.
- Adjusts ADR-015: **removes Grafana** from the stack, keeps Prometheus, and **adds Alertmanager**; Loki deferred. Other technologies unchanged.
- Reaffirms ADR-005 (Portal only talks to the API — the CP is the gateway to Prometheus) and ADR-006 (Postgres as the official state).

# **ADR-017**
# **Zero-touch Seed Node onboarding (join token, CP-assigned port, auto-registration in Prometheus)**
**Status:** Approved

**Context**
Adding a new Seed Node (e.g., the first Raspberry Pi) to the Cluster today requires several manual and error-prone steps: copying the registration token, **choosing a unique BitTorrent port** per Node, setting the qBittorrent WebUI password, **adding the target in Prometheus** by hand, and creating the port-forward in the firewall. The operator wants the opposite: **run a command on the Node and have it appear in the Control Plane fully ready**. Existing decisions help — ADR-012 (Agent initiates communication), ADR-006 (Postgres is the official state), ADR-016 (Prometheus is the collection engine, the CP is the gateway) — but they do not cover the *provisioning* of a Node.

**Problem**
Define an automated and secure onboarding flow: how the Node proves it can join; who decides the BitTorrent port (and how to reuse it when a Node leaves); how the Node starts being scraped by Prometheus without manual config editing; and how the Node obtains the binary/artifacts without depending on the private repository.

**Alternatives considered**
1. Keep everything manual (status quo): simple in code, but does not scale even with 3 Nodes; contradicts the operator's goal.
2. Single shared token for all Nodes: already exists (`SEEDGRID_AGENT_REGISTRATION_TOKEN`), but a single secret for the entire fleet is fragile and untraceable.
3. **One-time join tokens (kubeadm style)** issued by the CP, port assigned by the CP, auto-registration in Prometheus via file-based service discovery, artifacts served by the CP (recommended/adopted).
4. Dynamic port with qBittorrent on host network (no fixed mapping): would unlock the port post-registration, but reduces container network isolation; discarded in favor of keeping the bridge network with the port assigned **before** `compose up`.

**Adopted decision** (alternative 3)
- **One-time join token:** the operator generates a disposable token in the CP (protected endpoint; eventually a button in the Portal), with single-use validity. Onboarding uses this token — not the fleet's shared secret. The token is the "generation/verification hash": the Node presents it, the CP validates and consumes it.
- **BitTorrent port assigned by the CP:** the Agent no longer chooses the port. During **enroll**, the CP assigns the **lowest available port** starting from `6881`, considering active Nodes (not removed) **and** pending reservations. When a Node is removed, its port returns to the pool (reuse) — prevents the sequence from escaping to infinity with Nodes joining and leaving. The port is returned during enroll, **before** bringing up qBittorrent (keeps the bridge network with fixed mapping — alternative 4 discarded).
- **Prometheus auto-registration (file_sd):** `prometheus.yml` now uses `file_sd_configs`; the CP **writes the targets file** (on a shared volume with Prometheus) whenever the list of Nodes changes (registration/heartbeat/removal). Prometheus reloads the file on its own — no config editing or restarting required. It is the "new Node trigger → register in Prometheus".
- **Secrets:** the Agent credential remains **issued by the CP** during registration (ADR-012, current model). The **qBittorrent WebUI password is generated on the Node itself** and never leaves it (the CP neither knows nor needs it). The only secret the operator injects is the join token.
- **Artifact delivery:** the CP (via the Portal's nginx) serves the **bootstrap script** and the Agent's **ARM64 binary**. The Node runs a single command (`curl … | sh`), without cloning the private repository. It also avoids build leftovers (Go toolchain/cache) on the Node.
- **Flow:** the script calls `POST /api/v1/agent/enroll` (with the join token) → the CP consumes the token, reserves the lowest available port, and returns a disposable `{bt_port, registration_token}` → the script brings up qBittorrent on this port and writes the `registration_token` to the Agent's env → the Agent registers normally (ADR-012), the CP finalizes the Node with the reserved port and starts serving it in Prometheus. The operator is left only with the **firewall port-forward** (agnostic, their task — the Portal shows IP:port).

**Consequences**
Benefits:
- **single-command** onboarding; the Node appears in the CP already with port, credential, and metrics;
- port as a CP-managed resource (single source, with reuse), eliminating manual collisions;
- Prometheus always consistent with the fleet, without config editing;
- secrets with a smaller attack surface: disposable join token + Engine password local to the Node.
Limitations:
- more state/surface area in the CP: join tokens table, port reservation/assignment, and Prometheus targets writer;
- requires a **shared volume** between the CP and Prometheus containers (the CP writes the targets file);
- serving the binary/script via the CP couples the ARM64 artifact to the CP deployment.

**Addendum (Sep/2026) — storage is no longer a one-time decision.** The `bootstrap-node.sh` used to decide `/data` at a single instant: if the disk was not present when the command ran, the Node was initialized with `/data` on the SD card and **there was no turning back** — worse, the hotplug auto-recovery (udev rule) was only installed when `/data` already had a UUID at that moment, meaning a Node with a disk attached later was initialized precisely without the protection it needs most. Real case: during the onboarding of node-04, the HDD was plugged in 9 minutes after the bootstrap, and the storage had to be mounted, the container recreated, and the udev rule installed manually. The script now accepts **`--storage-only`**, which re-runs only the storage step (mounting by UUID in `fstab`, installing the hotplug recovery, and recreating the Engine so the bind mount sees the new disk), is idempotent, and discovers the CP via `/etc/seedgrid-agent.env` — no enroll, no token. It does not change the onboarding flow or the enroll contract.

**Addendum (Oct/2026) — enroll also returns the Node index (#202).** The response from `POST /api/v1/agent/enroll` gains `node_index`, alongside `bt_port`: the Node's position in the port sequence (`bt_port − 6880`, calculated by `node.IndexForPort`). It is the source of the Node's deterministic IPv6 suffix (ADR-038). The field is additive — those who only read `bt_port` won't notice — and there is no new state: the index inherits the port's uniqueness (unique among active Nodes and live reservations) and returns to the pool along with it when the Node is removed.

**Addendum (Oct/2026) — the bootstrap declares whether the Node has a dedicated disk (#221).** The Agent required `/data` to be a *mountpoint* on **every** Node, while the bootstrap accepted `/data` on the system disk when there was no dedicated disk: a single-disk Node (VM, mini PC, NAS) would forever stay with `storage_ok = false`, out of the Scheduler and with `NODE_STORAGE` open, without the operator having done anything wrong. The requirement only makes sense for those who **declared** a dedicated disk, and inferring this at runtime would fail precisely where the rule protects — an unmounted disk looks like a local folder. The bootstrap now **records the declaration** in the Agent's env, `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT`: `true` when it mounted or found a dedicated disk at `/data` (and always on `--storage-only`), `false` when `/data` remained on the system disk, with a warning in the terminal. The Agent reads it as data and never re-evaluates it; **if absent, it defaults to `true`**, so existing Nodes do not change and the error, if any, fails safe. A second pass of the bootstrap never downgrades a Node that has already declared a disk. It does not change the enroll or heartbeat contracts (`storage_ok`/`storage_detail` remain as they were) and does not touch the Control Plane. See `docs/06 - Storage.md` §35.3.

**Relationship with other ADRs**
- Respects **ADR-012** (Agent initiates communication — the Node pulls the enroll and registration; the CP never connects to the Node).
- Extends **ADR-016**: Prometheus remains as a decoupled engine; adds file-based service discovery fed by the CP.
- Reaffirms **ADR-006** (the Node's port and state live in Postgres) and **ADR-005** (the operator operates via Portal/API; artifacts served by the product's frontend).

# **ADR-018**
# **Torrent migration between Seed Nodes (physical copy, direct pull orchestrated by the CP)**
**Status:** Approved

**Context**
The operator needs to **relocate torrents between Seed Nodes**: migrate content from test Nodes to new hardware (e.g., the first Raspberry), drain a Node with health issues before removing it (Node deletion ADR), or rebalance when a disk fills up. Today a torrent is added to a Node by the Scheduler and remains there; there is no way to move it.

**Problem**
Moving a torrent (metadata + **downloaded data**) from a source Node to a destination Node, orchestrated by the Control Plane, deciding (a) how the data travels, (b) where the metadata comes from (`.torrent`), (c) how to avoid data loss if the migration fails, and (d) how this coexists with the "no direct communication between Nodes" principle.

**Considered alternatives**
1. **Re-download via swarm:** the CP adds the torrent to the destination, which downloads from the swarm (including from the source, which is seeding); upon completion, it removes it from the source. Respects "no direct communication between Nodes" (the transport is BitTorrent itself) and is simple. **Rejected** because it re-downloads the data: wastes bandwidth/time and, on a **private tracker**, registers a new download (affects ratio; risk of hit-and-run rule).
2. **Data relay via CP:** the CP intermediates the copy source→CP→destination. Keeps Nodes from talking to each other, but turns the **Control Plane into a data-plane** — GBs passing through it, becoming a bottleneck and a single point of failure. **Rejected** (anti-pattern; explicit operator decision: "no data relay via CP").
3. **Physical copy with direct pull destination←source, orchestrated and authorized by the CP** (recommended/adopted).

**Adopted decision** (alternative 3)
- **Physical copy, no re-download:** the already downloaded data is transferred from the source's disk to the destination; the destination performs a **recheck** (verifies 100% against the hashes) and starts **seeding immediately**, without downloading from the swarm. Preserves ratio on a private tracker and is fast (LAN).
- **Transport: direct pull destination←source, orchestrated by the CP.** The CP does not intermediate the bytes. Flow:
  1. Prerequisite: the CP has the `.torrent` persisted (see "Metadata" below).
  2. The CP issues a **single-use transfer authorization** (token + source Node endpoint + torrent hash) and commands the **destination Agent** to pull the data.
  3. The **source Agent** **stops the torrent** and only then exposes, temporarily and **only for that token/torrent**, the files of that torrent on an authenticated channel (HTTP on the Agent, minimum scope). The **destination Agent** pulls it to its storage path. *(**Refined after issue #8:** the stop was added because the copy was reading data that the Engine was still writing. Order matters — the pause happens **before** the token is authorized, so there is no window where the destination can pull in-flight data; without confirmation of the stopped state the command fails, and the migration aborts without loss. If the migration fails later, the **CP resumes** the torrent on the source, except when it is held by admission, whose decision is up to the Admission Worker, ADR-021/024.)*
  4. The destination adds the `.torrent` pointing to the copied data and performs a **recheck**; upon reporting `SEEDING`, the CP considers the copy valid. *(**Extended by ADR-031:** the criterion became "the destination has at least what the source had" (`min_progress`), to support **incomplete** torrent migration. Complete torrents behave as described here.)* *(**Moved by #87:** what waits for the recheck and verifies the criterion is no longer the Agent — it is the CP, observing the torrent on the destination; the `DONE` from `migrate_pull` now means "copy done, recheck triggered". See the **addendum at the end of this ADR**.)*
  5. Only then does the CP command the **source to remove** the torrent (with data) and updates the state (`torrents.node_id` → destination; event `TORRENT_MIGRATED`).
- **Explicit and scoped exception to "no direct communication between Nodes":** the principle applies to **steady-state operation** (Nodes do not coordinate with each other; the CP decides). A **one-off migration, initiated by the operator, authorized by a single-use token issued by the CP and restricted to one torrent**, is a separate case — the CP orchestrates and authorizes; the data travels directly for efficiency. Documented here as a conscious exception.
- **Metadata (`.torrent`):** the CP now **persists the `.torrent`** (Postgres, `bytea` — aligns with ADR-006, files from KBs to a few MBs). For torrents coming from magnets, the Agent **exports the `.torrent` from the engine** (`BT_backup`) after the metadata resolves and sends it to the CP. This way every torrent is migratable and the Cluster is resilient (a new Node can re-seed from the CP).
- **Data safety / rollback:** the source **only removes after the destination confirms `SEEDING`** *(ADR-031: confirm **`min_progress`** — having at least what the source had)*. If the transfer or the recheck fails, the torrent remains on the source (migration is aborted, without loss). Idempotent operation per (torrent, destination). *(**Refined after issue #79:** upon aborting, the CP also **discards the copy left on the destination** (`remove_torrent` with data). "No loss" still holds — the good copy is the one on the source, which is preserved and resumed; what remained on the destination is a draft of a transfer that didn't work out, and previously no one removed it (11 GB orphaned in the incident that originated the issue, plus a second record of the same hash in the Cluster). The cleanup only occurs when the pull was actually queued: a migration that dies before that copied nothing, and sending a remove there would risk deleting a legitimate pre-existing copy on the destination.)*
- **Operation scope:** migrate **all** torrents from a Node (drain) or a **selected subset**; manual triggers (operator) and, in the future, automatic ones (health/disk, via Scheduler).

**Consequences**
Benefits:
- efficient migration (no re-download; preserves ratio on private tracker); the CP does not become a data-plane; drain enables clean Node deletion; `.torrent` in the CP provides resilience (re-provision Node from scratch).
Limitations:
- introduces a **documented and scoped exception** to "nothing direct between Nodes"; the Agent gains a new capability (serving/pulling files under CP authorization), with a surface to protect (single-use token, scoped per torrent, short TTL); the CP now stores the `.torrent` files (more state, but small).

**Relationship with other ADRs**
- Reaffirms **ADR-006** (Postgres is the official state — the persisted `.torrent` lives in it) and **ADR-012** (the CP orchestrates; Agents act under command — the pull is initiated by the target Agent on the CP's command).
- Opens a **scoped exception** to the principle of isolation between Nodes (Architecture/Vision) for authorized migration; steady-state operation remains without direct communication between Nodes.
- Complements Node deletion: the **drain** (migrating everything) is the clean step before removing a Node.

> **Addendum (#87) — the target recheck is observed by the Control Plane; `migrate_pull` does not wait for it.**
>
> **Problem.** `migrate_pull` waited for the target recheck in a loop within the command itself (the watchdog from #79, with a 2 h ceiling). The Agent Poller executes the command batch **serially** and only fetches the next one when the current one finishes — so, during the wait, the entire Node went deaf: `pause`/`resume`/`remove`/`update_agent` for **other** torrents, and until the next batch. The 2 h limit wasn't even technical: it was the cost of blocking the Node.
>
> **Alternatives.** (1) *Asynchronous command* (`IN_PROGRESS` + outcome query): changes the command contract from ADR-014 — new command state, new query channel. (2) *Batch with limited concurrency*: adds concurrency to the Poller and requires serialization per torrent. (3) *Remove the wait from the command* — **adopted**: the CP already decides the migration outcome (only it removes the source), so it now also waits for the recheck. Smallest contract change, and the only one of the three that doesn't touch the Poller.
>
> **Decision.**
> - `migrate_pull` responds `DONE` as soon as the copy has finished and the recheck has been triggered (and the torrent started, #79). **`DONE` no longer means "the target is verified".**
> - The reconciler now observes the torrent **on the target Node** — the live hash line there, fed by inventory (the `SyncStates` adopts it as soon as the target Engine has the hash) — and only then concludes. Proven = **read after `DONE`** + state that **proves progress** (`SEEDING`, `DOWNLOADING`, `QUEUED` or `PAUSED` — allowlist; never `CHECKING`, `MOVING`, `UNKNOWN` or `ERROR`) + progress ≥ floor (`min_progress` − 0,5 %, ADR-031). The state condition is necessary: during the recheck qBittorrent reports the **verification** advancement as progress, not what the torrent actually has. The previous Agent only required `SEEDING` at the fallback floor (0,999); with a lower floor it would accept the verification's own progress.
> - **The #79 watchdog changed address, not rules.** Stagnation (default **10 min** without progress increasing) → fails with the usual diagnosis, *"target recheck stuck: no progress for Xm"*. Absolute ceiling, counted from the pull's `DONE` (default **6 h**, previously 2 h) → *"target did not reach source progress"*. The effect is the same as before: `fail()` — the source is resumed and the target copy is discarded, without loss. Both are tunable via hot-reload (ADR-023): `migration.recheck_stall_window` and `migration.recheck_max_wait`. The watchdog state (best progress, time of last advancement) is ephemeral, in-memory, like that of the Admission Worker: restarting the CP only opens a new stagnation window — it never drops a healthy migration — and the ceiling comes from the database.
> - The token expiration (30 min) now covers only the **transfer**. Previously, since the recheck wait was inside the command, it would cut the migration at 30 min even while the recheck was running — the 2 h ceiling was never reachable.
>
> **CP × Agent Contract and Compatibility.** An **additive** field in the `migrate_pull` payload: `async_recheck: true` — "I am the one observing the recheck". The CP **does not branch** on the Agent version or the response type: it observes the torrent state in both cases.
>
> | | Old Agent (ignores the field) | New Agent |
> |---|---|---|
> | **New CP** | waits for the recheck and responds `DONE` already verified; the CP finds the target ready and concludes on the next inventory read (≤ ~15 s). Nothing changes for the operator — the Node remains busy during the recheck until the Agent is updated | responds `DONE` early; the CP observes and concludes |
> | **Old CP** | original contract, intact | does not receive the field → **keeps the wait in the Agent** and only responds `DONE` with the target verified |
>
> The "New Agent + Old CP" cell is the reason the field exists: without it the Agent would return `DONE` early to a CP that reads it as "verified" and would remove the source without proof — the central invariant of this ADR. Normal rollout (ADR-027: the CP publishes the binary, so it is updated first) does not exercise this cell, but data safety does not depend on the order being respected. The Agent's legacy path (wait + watchdog constants) remains until the old contract no longer needs to be satisfied.
>
> **Contention for the same torrent.** Without the block, a command on the torrent would be executed on the target **during** the recheck (previously it stayed in the queue, behind `migrate_pull`): pausing drops the verification, removing deletes the copy the CP is about to consider valid. The CP did not prevent actions during migration — only admission (ADR-021/024, `notMigratingPred`) and only on the source line. It now refuses with **409 `migration_in_flight`** operator actions (`pause`/`resume`/`recheck`/`remove`) on (a) the migrated torrent and (b) the **copy of the same hash on the target**, while the migration is `PENDING`, and the migration of that copy back (`ErrAlreadyMigrating`). A `QUEUED` migration does not block anything: nothing has moved yet, and a drain can wait in the queue for hours. The Agent preserves the batch order (serial): a command for the same hash runs after the pull's `Start`, never in the middle of it.
>
> **Consequences and limits.**
> - **The copy still occupies the target Poller** (proportional to size and disk bandwidth). The recheck was the Engine's only long, unpredictable-time process; the copy remains inside the command. Removing it would also require an asynchronous command (alternative 1), not adopted.
> - With the recheck outside the command, the ceiling could be raised; the cost of a high ceiling is the **`migration.max_concurrent` slot** remaining occupied until then (the migration queue moves slower), not a deaf Node.
> - Verification now depends on the target inventory reaching the CP (every ~15 s). Without a read, it becomes stagnation and the migration aborts without loss.
> - No schema changes: the verification state is not persisted, and `PENDING` now covers the transfer **and** the verification.
> - **Without an exit, the 409 would be a trap** — and it now lasts for the duration of the verification (up to 6 h). See "Operator cancellation", below.
> - Verification is legitimately long (hours, on slow disks). The stuck `PENDING` migration backstop provided by ADR-033 (`maintenance.migration_stuck_after`, 30 min) **is not implemented** — the current Maintenance Worker only does retention —; if it comes, it must respect the verification window, otherwise it will drop the migration that the watchdog here keeps alive.
>
> **Operator cancellation (`POST /api/v1/migrations/{id}/cancel`).** Refusing actions during migration is only acceptable if the operator has a way out: in `main` there was no way to cancel a migration (only create and list), and with verification potentially lasting hours the torrent would get stuck. Cancellation is the **same safe watchdog termination** — a single path (`abort`) serves the watchdog, the token expiration, the pull failure, and the operator: `FAILED` migration with the reason, source resumed, target copy discarded.
> - **State: `FAILED` with reason `cancelada pelo operador`, not a new state.** The enum does not change (there is no `CHECK`; no migration), and that is what matters: many readers treat "terminal" as `COMPLETED`/`FAILED` — inventory deletion reconciliation (`reconcileMissing`), the Portal, pruning —, and a third state would be read as "in flight" by everyone who doesn't know it. The API exposes `cancelled: true` (derived from the reason) for the client to distinguish "cancelled" from "failed"; the Portal shows "Cancelled".
> - **`torrents:migrate` permission** (Administrator and Publisher) — the same as migrating: cancelling discards the target copy, it is the opposite of migration, and should not sit behind a weaker permission. Audited (ADR-025): *"Cancelled migration of torrent «X» from Node «A» to Node «B»"*; rejected attempts (409) are also logged, with the route label.
> - **In flight = `QUEUED` or `PENDING`.** Anything else responds **409**: `migration_finished` (completed, or failed on its own) or `migration_already_cancelled`. **Repeating is safe** — the effect (downloading commands, queuing cleanup) happens only once; repeating only returns the 409.
> - **What happens in each phase.** `QUEUED`: nothing was queued; it becomes `FAILED` and is never promoted. `PENDING` with `migrate_serve`/`migrate_pull` **not yet claimed** by the Agent: they are downloaded as `FAILED` to **never run** (a forgotten pull in the queue would copy GBs of a torrent no one wants to migrate anymore), and since nothing ran there is no source to resume nor copy to discard — sending a `remove_torrent` to the target would risk deleting a legitimate copy. **During the copy** (the target Agent is inside `migrate_pull`): the command **is not interrupted** — the Agent has no cancellation channel in the middle of a command and ADR-014 does not provide for it. The migration becomes `FAILED` immediately, the source is resumed, and the `remove_torrent` (with data) is queued **behind** the pull in the target's serial Poller: the copy finishes and is discarded right after, the same effect as the watchdog. **During the recheck verification**: same, and the `remove_torrent` interrupts the recheck and frees the disk.
> - **Cancelling vs completing never win together.** The reconciler completing and the operator cancelling can cross paths; without protection, a cancellation arriving at the instant the migration completes would overwrite `COMPLETED` and order the deletion of the **only** copy (the target one), with the source already removed. Both transitions are **conditional** (`PENDING → COMPLETED` and `QUEUED/PENDING → FAILED`, in the `UPDATE`): the loser has no effect whatsoever — the losing `complete()` rolls back (the torrent replacement is undone and the source `remove`, queued last, never existed); the losing `abort()` queues nothing. The transition, command download, and cleanup commit in a **single transaction**.
> - **Limit:** the transfer token (30 min) is not revoked on cancellation — the source Agent has no revocation command. It remains single-use and scoped to the hash, and the source has already been resumed; a pull still in progress reads changing data, but will be discarded.
> - **The refusal points to the exit.** The 409 `migration_in_flight` says it is possible to cancel and **which** migration (even when the action was on the target copy, whose migration the operator would have no way to discover): *"…wait for completion or cancel the migration (Portal: Alerts & Events → Recent Migrations → Cancel Migration; API: POST /api/v1/migrations/{id}/cancel)"*.
# **ADR-019**
# **Local content publishing: upload → `.torrent` creation in the Engine + seeding, with operator trackers (streaming via CP)**
**Status:** Approved

**Context**
The operator wants to **publish their own content** (ISOs, files) directly from their PC via the Portal: the Cluster generates the `.torrent`, announces it, and starts **seeding**, without needing a local BitTorrent client or an existing `.torrent`. During publishing, they choose **public** or **private** — the latter announces to an **operator's own tracker** (announce URL + passkey). Today, SeedGrid only *consumes* torrents (magnet or existing `.torrent`); it does not *create* them.

**Problem**
(a) How the **data** (potentially several GB) flows from the browser to the Node, without the browser talking directly to the Node (isolation principle); (b) **where and how** the `.torrent` is created; (c) how to **announce** (public vs private tracker with passkey) without leaking the secret; (d) **where** to store the tracker credentials; (e) which Node hosts it.

**Transport alternatives considered**
1. **Browser → Node directly:** efficient, but **exposes the Node** to the browser (breaks isolation; same objection as "direct download"). **Rejected.**
2. **Staging in CP + Node pulls** (reuses the pull from ADR-018): robust to busy/offline Nodes, but the **CP accumulates the entire file on disk**. **Rejected as default** (disk spike in the CP for a one-off upload).
3. **Streaming via CP** (browser → CP → Node in stream), **adopted:** the CP receives the multipart and **streams it** to the Agent (does not persist the file). The CP sits in the byte path **only during the upload** — **transient** data-plane, not steady-state relay. Consciously accepted: it is a **one-off operation, initiated by the operator**, analogous to the scoped exception in ADR-018 (the "no relay via CP" that ADR-018 forbids is the **continuous** migration traffic between Nodes, not an interactive operator upload).

**Adopted decision**
- **Transport:** streaming `browser → CP → Node`. The CP reads the multipart parts and repackages them as a **tar stream** to the Agent (reuses `extractTar` from ADR-018); the Agent extracts to storage (`/data`). Authorization via a **single-use token** issued by the CP; **receiving** channel in the Agent (same family as `transfer.Server` from ADR-018, now with a *push* endpoint), with **path confined** to storage and **sanitized name** (anti-traversal).
- **`.torrent` creation: the Engine itself creates it** (qBittorrent `torrentcreator`, `startSeeding=true`) — no external library. The Agent triggers the creation (`sourcePath` in `/data`, `trackers`, `isPrivate`), waits for completion, **exports the generated `.torrent`**, and returns it to the CP, which **persists** it in `torrent_files` (`source='created'`, aligns with ADR-018/ADR-006). The torrent is born already **SEEDING** on the Node.
- **Node selection:** the **Scheduler** decides by default (same eligibility/score as normal add); the operator can **force** a target Node (explicit Scheduler bypass, logged in the decision) — useful to pin to a specific disk.
- **Operator trackers (announce):** credentials registered **per operator** in Profile (name + announce URL containing the passkey). On publishing: **Public** (no tracker — discovery via DHT/PeX) **or** a registered tracker (**private**, `isPrivate=true`, DHT off). The announce is stored **in plaintext** (operator's decision — no encryption at rest) and **is not returned in the API listing** (only name + masked host); the CP injects the announce only in the command to the Agent at publish time.
- **Persistence:** new `trackers` relation (per user, plaintext announce, default flag) in Postgres (ADR-006). The created `.torrent` goes into `torrent_files` like any other → the published content is **migratable** (ADR-018) and re-provisionable.

**Consequences**
Benefits:
- the Portal becomes a **publishing** point (not just consumption); reuses the Engine to create torrents (no new dependency) and the transfer/`.torrent` infrastructure from ADR-018; published content is migratable and resilient.
Limitations/costs:
- the CP sits **in the byte path during the upload** (transient; not a continuous relay, but consumes CP bandwidth/CPU in the interval); the Agent gains a **receiving capability** (surface to protect: single-use token, short TTL, confined path); the CP now **stores operator secrets** (tracker passkeys) in plaintext (conscious decision — no encryption at rest), mitigated by **not returning the announce in the API** (only name + masked host).

**Relationship with other ADRs**
- **Extends ADR-018:** same principle of a data channel that is **direct and authorized by a CP token**; here the CP is the *source* (streaming push to the Agent) instead of orchestrating a pull between Nodes. Reinforces that "no relay via CP" forbids **continuous** traffic between Nodes, not the operator's one-off upload.
- Reaffirms **ADR-006** (`.torrent`, `trackers`, and secrets live in Postgres) and **ADR-012** (the CP orchestrates; the Agent acts under authorization).
- Complements **ADR-017** (the operator's Profile gains tracker management).

# **ADR-020**
# **Integration with external tracker (UNIT3D): automatic upload via API, with enriched metadata (mediainfo + TMDB)**
**Status:** Approved — **partially superseded by ADR-037**

> ADR-037 supersedes the part of this ADR that fixed the **taxonomy and naming format of samaritano as if they were product-owned**: they now become a tracker profile, contributable data. The **contract with the UNIT3D v8 API** defined here (endpoint, required fields, token auth in query) remains fully in effect.

**Context**
Some of the operator's trackers are **UNIT3D sites** (e.g., samaritano.cc). On them, publishing is not just announcing on an announce URL: the release must be **registered on the site** (catalog) via a form/**API** that requires metadata — category, type, resolution, TMDB/IMDB IDs, mediainfo, description. Today ADR-019 creates the `.torrent` and seeds it, but site registration is 100% manual and laborious. The operator wants SeedGrid to **automatically post** to the site when publishing, filling in the fields by inference (with human review before sending).

**Problem**
(a) The `.torrent` must be generated **identical to what the site expects** (`source` tag, `private=1`, announce with passkey) otherwise the infohash will not match; (b) **mediainfo** can only be extracted **where the file is** (on the Node), not in the CP or the browser; (c) the UNIT3D category/type/resolution **IDs** and the **TMDB/IMDB** must be inferred from the name/file; (d) where to store the site **credentials** (API token) and the **TMDB** key; (e) how to avoid **polluting the catalog** of the private tracker with test uploads.

**Alternatives Considered**
1. **Manual site registration** (status quo): zero automation, high friction. **Rejected** as a goal, kept as a fallback.
2. **External tool** (e.g., SamUploadAssistant running separately): solves it, but stays **outside** SeedGrid — duplicates infrastructure, does not reuse the Engine/Scheduler or the already created `.torrent`. **Rejected.**
3. **Native integration in SeedGrid** (**adopted**): the publishing flow itself (ADR-019) gains an optional step to **post via the tracker API**. The Agent extracts the mediainfo (it has the file); the CP infers IDs/queries TMDB and builds the payload; the operator **reviews/adjusts** in the Portal; the CP performs the `POST /api/torrents/upload`.

**Adopted Decision**
- **Compatible `.torrent`:** the `trackers` list gains integration fields — `source` (tag, e.g., `SAMARITANO`), `api_url` (site base) and `api_token` (operator secret, **text**, never returned in the API). When publishing to a tracker with `source`, the Engine creates the `.torrent` with this `source` (qBittorrent `torrentcreator` already accepts `source`), matching the infohash with the site's.
- **mediainfo on the Node:** the Agent runs the `mediainfo` binary on the published file and returns the text to the CP (it is the only point with access to the data — consistent with "only the Agent talks to the Engine/data").
- **TMDB in the CP:** the query is a **global** Control Plane service (`TMDB_API_KEY` key in the CP config, not per tracker), used to resolve `tmdb`/`imdb` from the name+year parsed from the release.
- **Inference + review:** the CP parses the name (resolution → `resolution_id`, type → `type_id`, category → `category_id` via UNIT3D maps) and pre-fills the Portal form; **the operator reviews and confirms** before sending (nothing is posted without explicit action). *Since ADR-037 (#63), the parser outputs only labels (`1080p`, `WEB-DL`) and whether it is a series; the site's profile turns the label into that site's id.*
- **Name enriched by mediainfo:** when the file name is poor (e.g., just "A Bruxa 2015"), the parser lacks resolution/codec/audio for the standardized name. `release.EnrichFromMediaInfo` reads the mediainfo text (width/height → resolution; Writing library/Format → `x264`/`x265`/`H.264`…; Format+channels of the 1st audio track → `DDP5.1`…) and **fills in what was missing** (the name takes priority — it may bring the **source**, which the mediainfo does not know). Result: "A Bruxa 2015" → "A Bruxa 2015 1080p DDP5.1 x264". The `POST /api/v1/publish/infer` receives the mediainfo and returns the already enriched `standard_name`.
- **Sending:** `POST {api_url}/api/torrents/upload?api_token=…` (multipart) with the `.torrent` + fields. The response brings the download URL of the `.torrent` already with the passkey (confirmation). SeedGrid **is already seeding** since creation (ADR-019).
- **Post-publication infohash verification (#23):** right after registration, the CP downloads the **official** `.torrent` (the returned URL) and compares the infohash with what the Node is seeding. If they diverge (the site regenerates the `.torrent` with its own `source`/piece size — if ours went out without this, the tracker responds `InfoHash not found` and the Node never counts as a seed), the CP **automatically re-seeds**: updates the row to the official hash (preserves `tracker_id`/`published`/`media_info`), persists the official `.torrent` and orchestrates the swap on the Node — adds the official one (recheck against the data already in `/data` → SEEDING, without re-download) and removes the old infohash **without deleting the data**. It emits `TORRENT_RESEEDED` (WARNING). It is the safety net that prevents the "I published but the site shows 0 seeds" case.
- **Anti-pollution:** the actual POST sits **behind explicit confirmation** in the Portal and is only exercised with **real releases**; there are no test uploads in the catalog (UNIT3D has no dry-run).

**Consequences**
Benefits:
- publishing to a UNIT3D tracker becomes **a single flow** within SeedGrid (creates + seeds + registers on the site), reusing the Engine/Scheduler/`.torrent` from ADR-019; metadata filled automatically with the operator in control.
Limitations/costs:
- the CP now stores **one more secret** for the operator (tracker API token, in text — same decision as ADR-019 for the passkey) and a third-party key (TMDB) in the config; the Agent gains a dependency on the **`mediainfo`** binary; ID inference is **heuristic** (which is why human review is mandatory); it couples to the **UNIT3D API v8** contract (tracker version changes may require adjustment).

**Relationship with other ADRs**
- **Extends ADR-019:** it is the optional "register on the site" step after creating+seeding; reuses `trackers`, the Engine, and the persisted `.torrent`.
- Reaffirms **ADR-012** (the CP orchestrates and holds secrets; the Agent acts locally on the data) and **ADR-006** (secrets in Postgres).

# **ADR-021**

**Title:** Download admission controller (DL slot limit per account, cluster-wide)

**Status:** Approved

**Context**
The tracker (UNIT3D site, e.g.: samaritano.cc) limits **simultaneous downloads per ACCOUNT** (DL slots), not per client. The passkey is **shared** across the cluster's Seed Nodes (same account). With many incomplete torrents announcing at the same time, the tracker refuses peers for almost all of them (announce `status 164` / "Download slot limit reached") and the entire cluster's download **freezes**. The qBittorrent internal queue (`max_active_downloads`) is **per Node** and does not see the cluster total; with `dont_count_slow_torrents=true` it further **amplifies** the problem (stopped downloads do not count towards the ceiling → qBt activates all of them → *thundering herd*).

**Problem**
Ensure that at most **L** torrents are downloading across the ENTIRE cluster (L = account DL slot limit), promoting the next one when a slot opens, without exceeding the tracker limit or relying on Node-to-Node coordination ([ADR-012] forbids Node↔Node communication).

**Considered alternatives**
1. **Statically divide the ceiling per Node** (`max_active_downloads = L/N` in each qBt): does not track dynamic L, wastes slots with integer division, and does not reallocate when a Node becomes idle.
2. **Rely solely on qBt's anti-slow rotation** (`dont_count_slow_torrents`): it is per Node and caused the *thundering herd* that originated the incident.
3. **Admission controller in the Control Plane (adopted):** a Worker (such as the Alert Worker) maintains the cluster-wide ceiling by placing the excess on *hold* (pause) and promoting from the queue; the Nodes only execute `pause_torrent`/`resume_torrent` ([ADR-014], polling).

**Adopted decision**
- **Worker in the CP** periodically evaluates the pool of "non-stopped" torrents (`DOWNLOADING` + `QUEUED`) in the cluster. If `> L`, it places the excess with the **lowest progress** on **hold** (`pause_torrent` + sets `torrents.admission_paused_at`); if `< L`, it **promotes** the holds with the **highest progress** (`resume_torrent` + clears the flag). It counts `DOWNLOADING`+`QUEUED` (not just `DOWNLOADING`): stopping only the active ones would let qBt replenish them from `queuedDL`.
- **admission-hold ≠ user-pause:** the hold lives in `admission_paused_at` (its own column, **immune to SyncStates**, which overwrites `status`). The controller only resumes what **it** paused; a torrent paused by the operator is never touched and does not count as active.
- **L is dynamic:** extracted from the tracker's own message (UNIT3D error 164 "You may at most download **N** torrents"), which the Agent reads from the Engine (`/api/v2/torrents/trackers`) and reports to the CP; persisted in the default tracker. As long as no message has been observed (or the TTL expires), it falls back to the `SEEDGRID_MAX_CLUSTER_DOWNLOADS` config. **Without a known L, the controller takes no action** (it never blindly pauses the cluster). *The UNIT3D `/api/user` API DOES NOT expose the limit — validated live; therefore the source is the announce message itself.*
- **Belt-and-suspenders in the Agent:** default `max_active_downloads=2` per Node and `dont_count_slow_torrents=false` (the ceiling actually holds; the excess stays in `queuedDL`, which does not announce as a leecher).

**Consequences / Limitations**
- If the L non-stopped torrents concentrate on a single Node (limited by `max_active_downloads`), the cluster may slightly underutilize L — acceptable (the critical part is not exceeding the limit). Future refinement: the Scheduler distributes them better.
- The observed L depends on the cluster having hit the limit at least once (otherwise it uses the fallback).
- ~~Promotion priority **by progress** (highest first) is the V1 policy.~~ **Replaced by peer-aware eligibility (below).**
- **Healthy rate rotation ([#22], implemented):** at the ceiling, an active download whose `dl_speed` drops below `SEEDGRID_ADMISSION_MIN_DL_RATE` (default 50 KiB/s) for a continuous `SEEDGRID_ADMISSION_SLOW_GRACE` (5 min) yields the slot to one in the queue that can progress (1:1 swap, ceiling preserved); the rotated torrent stays out of the promotion queue for `SEEDGRID_ADMISSION_SLOW_COOLDOWN` (10 min) to prevent oscillation (thrashing). Worker in-memory state (ephemeral). `MIN_DL_RATE=0` disables it. Future evolution: expose the threshold in a CP configuration panel.
- **Peer-aware eligibility + dead torrent rotation (evolution, implemented):** the pure progress promotion criterion was **blind to peers** — a high-progress torrent but **with no one in the swarm** (0 seeds and 0 peers) monopolized a slot without ever completing, while torrents with seeds rotted in the queue (observed live: one at 70% stuck at 0/0, and holds with 1-2 seeds stopped). Two-part fix: **(1) peer-aware promotion** — the promotion queue sorts **viable first** (`num_seeds>0 OR num_leechers>0`), then highest progress as a tie-breaker (finishes first the one closest to completion **among those that can download**). **(2) dead rotation** — an active download with **0 seeds AND 0 peers** for a continuous `SEEDGRID_ADMISSION_DEAD_GRACE` (default 3 min) is rotated immediately (primary trigger, before the rate rule) and sent to the **back of the queue** for `SEEDGRID_ADMISSION_DEAD_COOLDOWN` (default 30 min) — it does not return even if it has the highest progress, giving way to the others; if the ready queue is exhausted (everyone has had a turn), the parked ones return so no slot is left idle ("or until the queue rotates"). `DEAD_GRACE=0` disables it. In-memory state (ephemeral).
- **Connected vs swarm + dead backoff (refinement, implemented jul/2026 — migration 00030):** dead rotation measured "0 seeds AND 0 peers" by the **swarm** (tracker scrape), and the Agent collapsed swarm and connected into a single field (`adapter.pick`), so the CP never saw how many peers were **actually connected**. This let the worst-case scenario slip through: a tracker announcing a **ghost seeder** (registered in the scrape, but never responds) yields `num_seeds=1` → the torrent is **never** considered dead, and since it is "viable" and usually has high progress, it returns to the **top** of the queue every `SLOW_COOLDOWN`, burning a slot from the L ceiling indefinitely without downloading 1 byte (observed live: `Blade.Runner.2049` at 98,79% with 0 connections for 5 days, cycling resume→hold every ~15 min). Two-part fix: **(1)** the Agent now reports connected peers in dedicated fields (`num_seeds_connected`/`num_leechers_connected`, new columns; `num_seeds`/`num_leechers` remain the swarm displayed by the Portal) and `rotateDead` judges by the **connected** ones — `COALESCE(num_seeds_connected, num_seeds, 0)` falls back to the swarm for older Agents, so the change is **additive and backward-compatible**, with no deployment window required; **(2)** **recurrence backoff** — each consecutive dead rotation for the same torrent doubles the cooldown (30m → 1h → 2h…) up to `SEEDGRID_ADMISSION_DEAD_BACKOFF_MAX` (default 4h; 0 disables), and connecting to any peer resets the count: a chronically dead torrent moves away from the queue on its own instead of reappearing at every fixed cooldown. **The promotion queue (`promotableHolds`) continues to judge by the swarm on purpose:** it sorts torrents on **hold** (paused), which have 0 connected by definition — using connected there would zero out the peer-aware criterion and revert to pure progress sorting, the bug this ADR fixed.

**Benefits**
- Resolves the *thundering herd*/stall permanently and cluster-wide, respecting ADR-012/ADR-014 (the CP decides, the Agent executes via polling); L without hardcoding.

**Relation to other ADRs**
- Reaffirms **ADR-012** (the CP orchestrates; the Agent acts) and **ADR-014** (commands via durable queue/polling).
- Complements the **Scheduler** (placement) by acting on **admission** (when each download can run).

# **ADR-022**

**Title:** Activity Feed — orchestration activity trail (separate from Events and Alerts)

**Status:** Approved

**Context**
The decisions the Control Plane makes about torrents — placing a download on *hold* (pause) due to the admission ceiling (ADR-021), promoting the next one in the queue when a slot opens up, scheduling a torrent on a Node (Scheduler), and lifecycle transitions (started downloading, completed, started seeding) — are currently **invisible** to the operator: the `admission` worker and the `scheduler` emit nothing, and status transitions are just a silent `UPDATE` in `SyncStates`. There are only **Alerts** (issues requiring attention) and **Events** (infra lifecycle facts: Node registered/removed, torrent created/migrated/removed/re-seeded). An "under the hood" view is missing — seeing orchestration happening in real time. Beyond the operational value (confidence, debugging), it is a strong showcase for the project as open source.

**Problem**
Expose, in a dedicated live trail, the orchestration decisions and torrent lifecycle transitions, **without** mixing them with Alerts (noise for those monitoring issues) or Events (infra semantics), and without violating ADR-012 (the CP decides/records; the Agent executes).

**Considered Alternatives**
1. **Reuse the `events` table with a `category` column:** less code, but conceptually mixes the orchestration trail with infra events and pollutes the Events page; the operator explicitly asked for something **separate**.
2. **Logs only (`journalctl`):** they already exist, but are not navigable by the operator nor presentable in the Portal; the "magic" disappears.
3. **Dedicated `activities` trail + dedicated Portal page (adopted):** a new domain, append-only, recorded by the CP at decision points, exposed via its own endpoint and a live timeline page — visually and conceptually separate from Alerts/Events.

**Adopted Decision**
- **Dedicated `activities` domain** (append-only): each record has a `kind` (e.g., `admission_hold`, `admission_resume`, `scheduled`, `downloading`, `seeding`), loose references to `torrent_id`/`node_id` (soft-delete preserves names via JOIN), denormalized `torrent_name` (survives hard-delete), readable `message`, and `detail` (JSONB) with the *why* (reason for the hold, Scheduler score/ratio, etc.).
- **Recorded by the CP at decision points**, always **best-effort** (failure to record never brings down orchestration): in the `admission` worker (`holdOne`/`promoteOne` — covers overflow and ratio rotation), in the creation flow (Scheduler decision), and in `SyncStates` (observed transitions → `DOWNLOADING`/`SEEDING`, comparing the previous status).
- **Trail separate from Alerts and Events**: dedicated table, endpoint (`GET /api/v1/activities`), and Portal page. No reuse of `events`.
- **Read-only for the operator**; no new Agent→CP channel (respects ADR-012/ADR-014). Portal updates via short **polling** (no SSE in V1; same pattern as other screens).

**Consequences / Limitations**
- The append-only trail grows over time; volume is low (only decisions/transitions, not samples). Retention/pruning is left as a future evolution (analogous to `torrent_stat_samples` pruning).
- Some redundancy between `admission_resume` (decision) and the observed `downloading` transition (effect) is possible; mitigated by recording the transition to `DOWNLOADING` only from `QUEUED`/`CHECKING` (the genuine "started downloading"), not from `PAUSED` (resumption, already covered by the decision).
- It does not replace logs or metrics; it is a **narrative** layer of the orchestration.

**Benefits**
- Real-time orchestration transparency (operational confidence + debugging) and an open source showcase, without polluting Alerts/Events.

**Relation to other ADRs**
- Provides visibility into the decisions of **ADR-021** (admission) and the Scheduler, and into lifecycle transitions.
- Respects **ADR-012** (the CP decides and records; the Agent only executes) and **ADR-014** (no new channel; read via polling).
- Complements, without replacing, the **Events** (infra) trail and **Alerting** (ADR-008).

# **ADR-023**

**Title:** Dynamic configuration of tunables via database, with hot-reload (configuration panel)

**Status:** Approved

**Context**
All Control Plane behavior tunables — Worker intervals, admission controller thresholds (ADR-021: fallback ceiling, minimum ratio, ratio and dead torrent rotation graces/cooldowns), alert rules (offline-after, disk saturation, sample retention) — were read **only once at boot** from environment variables (`config.go`). Adjusting any parameter required editing the `.env` and **redeploying** the container; moreover, several numbers (reconciler interval, prometheus-writer interval) were **hardcoded**, without even an env var. The operator wants a **Portal panel** to edit these parameters and see the effect **without redeploying**, each with a description.

**Problem**
Make behavior tunables **editable at runtime** (hot-reload), with a predictable source of truth and without violating conventions (database only via migrations; config external to code; Portal only talks to the API; only the CP accesses the database). Without turning `config.go` into a god-module or spreading magic numbers.

**Considered Alternatives**
1. **Continue with env + redeploy only:** simple, but does not meet the requirement (changing requires rebuild/restart; there is no panel).
2. **YAML file reloaded via filesystem watch:** hot-reload without a database, but the file is not accessible by the Portal (which only talks to the API), disappears from the UI, and reintroduces state outside of Postgres (the official source of state is the database — ADR-015).
3. **Code catalog + database overrides + atomic Store (adopted):** a metadata catalog in the code (single source: type, default, unit, description, limits, seed env), a `settings` table (KV) with the overrides, and a `Store` that resolves the effective value and applies it at runtime. Editable via the Portal API; state in Postgres; instant hot-reload (single process).

**Adopted Decision**
- **Code catalog** (`internal/settings/catalog.go`): each tunable is a `Spec{Key, Kind, Category, Label, Description, Unit, Default, EnvVar, Min, Max}`. It is the **single source** of metadata — it feeds the seed, validation, panel, and the `25 - Business Rules.md` doc. Memory separation *centralize-tunables-not-logic*: the catalog centralizes the **parameters**; the **logic** that consumes them stays in the domain (Workers).
- **`settings` table** (migration `00027`, KV `key/value/updated_at/updated_by`): overrides that supersede the catalog defaults. Reset to default = remove the row.
- **Precedence: the database rules; the env only seeds.** On the 1st boot, for each tunable without a row yet whose `EnvVar` is set and valid, `Store.Load` records this value as the initial seed (compatible with current `.env` files). After that, the Portal is the source of truth — changing the `.env` no longer has any effect.
- **Thread-safe Store with hot-reload** (`internal/settings/store.go`): caches overrides in an atomic pointer (lock-free reads in Worker loops); `Set`/`Reset` validate against the `Spec` (type + min/max), write to the database, and swap the pointer (copy-on-write) → the change takes effect on the very **next pass** of any Worker.
- **Workers read the value per pass** (they do not capture it in the constructor): admission and alerts receive the `Store`; those with a ticker recreate the interval (`ticker.Reset`) when the tunable changes. The reconciler and the prometheus-writer receive a *provider* `func() time.Duration`.
- **API + Portal:** `GET /api/v1/settings` (grouped by category), `PUT /api/v1/settings/{key}` (validates, writes, applies), `DELETE /api/v1/settings/{key}` (reset to default). **Settings** page in the Portal, each field with the catalog description. *(Tunable changes are recorded in the **Audit** trail — ADR-025; see note below.)*
- **Infra and secrets remain in env** (immutable): DB URL, tokens, admin password, API keys, prom URL, http addr, log level, migration port remain in `config.go`, outside the panel.

**Consequences / Limitations**
- Corrupted effective values (e.g., manual database edits outside the API) fall back to the **catalog default** in the getter — the Worker never breaks due to invalid config.
- Hot-reload is in-process (the CP is a single process — ADR-015); there is no pub/sub. If the CP ever becomes HA (multiple instances), each one would need to reload (future evolution: LISTEN/NOTIFY or cache TTL).
- ~~Tunable changes are audited as **Events** (`SETTING_UPDATED`/`SETTING_RESET`).~~ **Superseded by ADR-025:** tunable changes are **operator actions** → recorded in the **Audit** trail, not in Events (which are now strictly domain facts). The `SETTING_UPDATED`/`SETTING_RESET` emissions have been removed.

**Benefits**
- Operational tuning (including the admission tunables from ADR-021) **without redeploy**, with per-parameter descriptions; no more magic numbers; a single catalog mirrored in the business rules doc.

**Relation to other ADRs**
- Makes the tunables from **ADR-021** (admission) and **ADR-008** (alerts) dynamic, previously fixed at boot.
- Respects **ADR-015** (official state in Postgres; CP as the sole database accessor) and the conventions for migrations/external config.
- Audit reuses the **Events** trail (infra), distinct from the **Activity Feed** (ADR-022) and **Alerting**.

**Addendum (Oct/2026) — the Agent also reads the catalog (ADR-039, #144).** The catalog is no longer exclusive to the Control Plane: keys marked `PerNode` (`agent.heartbeat_interval`, `agent.command_poll_interval`, `agent.log_level`, and the Engine queue's `engine.*`) are **delivered to the Agent** in the heartbeat response and allow **per-Node overrides** (`node_settings` table, migration `00044`). The precedence above (database > env > default) gains a step at the top — **Node override > cluster value > Node env > code default** — and the Node env shifts from being a source to a **fallback** (it only applies when the Control Plane sends nothing). These keys **do not have a seed `EnvVar`** in this catalog: the env that would seed them would be the **Node**'s, not the Control Plane's, and it is reported by the Agent itself, once per Node (ADR-039 §7). The Control Plane Workers do not change; what changes is that the consumer of a tunable may now be on the other side of the network, and therefore the value is validated **on both sides** (the `Spec` in the CP, the sanity check in the Agent).

# **ADR-024**

**Title:** Manual/Automatic operation mode — operator control over admission (per-node, with global gate)

**Status:** Approved — **implemented**. Build versioning (git tags → "Agent" column) delivered (v1.0.0). Manual/Automatic Mode: migration `00028` (`clusters.operation_mode`/`nodes.operation_mode`); admission worker filters by Node mode (full_auto = current behavior; Manual counts towards L); fix for asymmetry in operator resume (clears `admission_paused_at`); API (`PUT /cluster/operation-mode`, `PUT /nodes/{id}/operation-mode`, gate exposed in `GET /nodes`); Portal (global Full auto/Flex toggle + "Mode" SG/Manual column in the Dashboard). Tested in an isolated DB. Optional follow-up: "above L" alert when all Nodes are in Manual (today the system simply takes no action).

> **Compliance fix (Jul/2026).** The original implementation respected the operator's pause on **any** Node, contradicting the "SG Node → system has priority" clause. The effect was an irreversible exit from orchestration: the manually paused torrent did not count in `countActive` (it is not DOWNLOADING/QUEUED) nor did it appear in `promotableHolds` (which requires `admission_paused_at` to be set), so it ended up in limbo — neither running nor a candidate — and the cluster operated below L with the queue appearing stuck. Diagnosed with 16 downloads in this state and no admission decisions in the feed for ~30h. Fixed with `adoptOrphanPauses` in the admission worker; the wording of the "asymmetry fix" below was qualified by Node mode to avoid reintroducing the ambiguity.

**Context**
Download admission (ADR-021, evolved with peer-aware rotation) is **autonomous**: the controller keeps the cluster ≤ **L** (simultaneous download limit of the tracker account, shared across Nodes) by putting the excess on *hold* and promoting/rotating on its own. The operator wants to be able to **take control** — manually stop a download, or manually promote/start one — without the system undoing it. Today the operator-vs-system priority is **inconsistent**: a manual *pause* is respected (the controller ignores `status=PAUSED` with null `admission_paused_at`, neither counting nor touching it), but a manual *resume* of a held torrent is **undone** by the "drift fix" (on each cycle the worker re-pauses any hold that has returned to `DOWNLOADING`, assuming the hold itself escaped).

**Problem**
Give the operator **explicit and predictable** control over admission, with a clear priority rule, **without breaking the tracker's ≤ L invariant** (exceeding L causes the tracker to refuse peers and stall downloads).

**Alternatives considered**
1. **Global switch only (cluster):** simple and trivially preserves L, but all-or-nothing — the operator cannot control a specific Node while keeping the rest automatic.
2. **Per-node only:** granular and intuitive in the UI, but L is account-wide (cluster-wide) — a manual node can exceed the ceiling; and there is no deliberate top-level state ("I am in control of some nodes").
3. **Global gate + per-node control (adopted):** a top-level **Full auto/Flex** state that unlocks **SG/Manual** per-Node control. Combines the clarity/safety of the global with the granularity of the per-node.

**Decision adopted**
- **Global gate "Full auto / Flex"** (cluster state):
  - **Full auto:** SG controls everything; per-Node toggles are **locked** (ensures no manual Node is forgotten — full-auto is full-auto).
  - **Flex:** unlocks; each Node becomes either **SG** (automatic) or **Manual** (operator's). Nodes left as SG remain automatic.
- **Priority defined by Node mode:**
  - **Manual Node → sovereign operator.** The controller **never** pauses, promotes, rotates, or re-pauses torrents on this Node. The operator's actions (pause/resume/start) stand, period.
  - **SG Node → system has priority** (manages holds/rotations as in ADR-021).
- **Manual counts towards L (invariant preserved):** active downloads on a Manual Node **count** towards the cluster ceiling; the controller reconciles using **only SG Nodes**. Consequence: pausing a download on a Manual Node frees a slot that the system **fills by promoting a hold on an SG Node** — the cluster stays maxed out (≤ L) without wasting a slot.
- **Edge case (operator's responsibility):** if **all** Nodes are in Manual, or a single Manual Node exceeds L, the system has no way to compensate → it **only warns** (alert "above tracker L"), **does not act**. This is the accepted price of per-Node control.
- **Current asymmetry fix (qualified by Node mode):** on a **Manual Node**, a *resume* of a held torrent (`admission_paused_at` set) **clears `admission_paused_at`** (the operator has taken over that torrent) → the drift fix does not re-pause it; it starts counting towards L and the controller rebalances the rest.
  - ⚠️ **This fix DOES NOT promote the operator to sovereign on an SG Node** — the priority clause above rules there. The original wording said "'manual resume wins' is consistent with 'manual pause wins'" without qualifying by mode, which contradicted the Decision itself and was read as a global rule in the implementation (see "Compliance fix" below).
- **Manual pause on an SG Node is ADOPTED by the system** (`adoptOrphanPauses`): it becomes an admission-hold and returns to the promotion queue, being promoted when a slot opens. Direct corollary of "SG Node → system has priority". For a pause to stand, the Node must be set to **Manual**. A **completed** torrent (`progress >= 1`) that is paused is left out: it is a seed that the operator stopped, not a download in the queue.
- **Return from Manual → SG readopts:** what was manually stopped while the Node was Manual re-enters the queue as soon as it returns to SG. There is no hook on the mode switch — adoption runs on the SG Node predicate on each cycle, so the transition resolves itself on the next cycle.
- **Reuse and scope:** the mode state (global gate + per-Node mode) is **cluster/Node state with hot-reload** (pattern from **ADR-023**); the admission worker queries the Node mode **before** acting. The Scheduler (Node choice on *add*) does not change (`force_node_id` already exists).
- **UI (controls on the Dashboard, for overview):** the global **Full auto/Flex** switch at the top of the Dashboard + a **"Mode" column (SG/Manual) in the Dashboard Nodes table** (chip + toggle per row) — so the operator sees the mode of **all** Nodes at once, without scrolling through the large cards on the Nodes page. The "Agent" column (`agent_version`) is **kept** — it was decided to **actually version the builds** with **version tags** (e.g., `git describe --tags --abbrev=0` → `v1.0.0`, without ugly hashes) in the build scripts, instead of the "dev" default — so the version becomes useful information. The Node card/detail can mirror the mode chip, but the **control and overview live on the Dashboard**.

**Consequences / Limitations**
- In Flex with Manual Node(s), the operator **can** exceed L (by design) — mitigated by alert, not by block.
- The admission worker now **filters by Node mode** in all queries (count active, choose holds, promote, rotate) — changes ADR-021 without breaking it (Full auto mode = current behavior).
- Additional state to persist/expose (global + per-Node mode) and test (priority, L count, edge case).

**Benefits**
- **Clear and predictable** priority rule (defined by Node mode), fixing the current asymmetry; control/security escape hatch ("I take the wheel") without giving up the ≤ L invariant on automatic Nodes.

**Relationship with other ADRs**
- **Extends ADR-021** (admission): adds the operator control axis; Full auto = current behavior.
- **Reuses ADR-023** (dynamic config/state with hot-reload) for the mode state.
- Does not create a new Agent→CP channel (respects **ADR-012/ADR-014**): the CP decides, the Agent executes.

# **ADR-025**

**Title:** Audit Log — audit trail of operator actions (at the API boundary, separate from Events/Activity/Alerts)

**Status:** Approved

**Context**
SeedGrid already has three trails: **Alerts** (issues), **Events** (infra/domain lifecycle facts — some triggered by the operator, with a `by` in the payload, many by the system) and **Activity Feed** (orchestration decisions — ADR-022). What a multi-operator / open source product expects is missing: an **audit trail** — "who did what, when, from where" — uniform and centered on the **actor**, covering every operator action on the API, including login and failed attempts. Today this is partial and scattered: only a subset of mutations emits an Event (and not every Event has an actor), and login is only tracked inside `auth`.

**Problem**
Record, in a **uniform and complete** way, all authenticated mutating actions of the operator (and login), with actor/route/result/source/time, without instrumenting handler by handler and without mixing with the existing trails (Events = domain facts; Activity = orchestration; Alerts = issues).

**Considered Alternatives**
1. **Add `actor` column to `events` and treat events with actor as audit:** reuses infra, but mixes audit (access/accountability lens) with domain facts, and **is not complete** — not every mutation emits an Event, and login/attempts are left out. Same objection as ADR-022 (do not pollute Events).
2. **Instrument each mutating handler with an audit call:** complete, but repetitive and easy to forget in a new handler (the audit would become unreliable).
3. **Automatic middleware at the boundary + own `audit_log` domain (adopted):** an interceptor on protected routes records every **mutating** request (POST/PUT/DELETE) from the operator; own domain/table/endpoint/page. Complete by construction (a new route is already audited) and conceptually separate.

**Adopted Decision** (alternative 3)
- **Own `audit_log` domain** (append-only): `actor` (authenticated username), `action` (**friendly description in PT-BR** — e.g., "Changed the «Node offline after» setting to 120s", "Logged into the Portal"), `method`, `path` (concrete, with ids), `status_code`, `source_ip`, `created_at`. No request body (privacy/size — e.g., never the new password).
- **Friendly action, not raw:** the handler sets the description via `httpx.SetAuditAction` (reuses the label/value it already knows — settings catalog, chosen mode, etc.); if missing, the middleware derives a PT-BR label from the route (map `method`+`path` → verb). The `method`/`path` remain recorded as **technical detail** (discrete footer in the Portal), so the reading is friendly without losing traceability. Complements Events, which store the semantic diff (`from`/`to`).
- **Automatic middleware:** `protected` now wraps handlers with a recorder that, **after** the handler, records the entry when the method is mutating (POST/PUT/DELETE/PATCH) and there is an operator in the context. Captures the status via the existing `statusWriter` (same one used by `RequestLogger`). **Best-effort and decoupled:** the recording uses a detached context with a short timeout (records even if the client disconnected) and **never** alters or blocks the response; audit failure is only logged. GET is not audited (read noise).
- **Login explicitly audited** in `handleLogin` (success and failure, with the attempted username), since the login route does not go through `protected` (there is no operator in the context yet). Complements the IP/UA tracking that `auth` already does.
- **Read-only for the operator:** `GET /api/v1/audit` (protected) + **Audit** page in the Portal (timeline, filter by actor/action). No new Agent→CP channel (respects ADR-012/ADR-014).

**Consequences / Limitations**
- Append-only trail grows over time; moderate volume (only mutations + login, no reads). Retention/pruning remains as a future evolution (analogous to the other trails).
- The friendly description carries the essentials (label + value, e.g., "…to 120s"); the `method`/`path`/`status` remain recorded for the technical detail. The operator's **systemic** changes (tunables, cluster/Node mode) stop emitting Events — they now live **only** in Audit (Events = domain facts: Node/torrent registered/removed/migrated, storage, etc.).
- Partial overlap with operator-initiated Events is accepted and intentional (different lenses: access vs domain fact).

**Benefits**
- Real accountability ("who did what") — value that increases with multi-operator and open source openness; complete by construction (middleware), without relying on per-handler discipline; separate and without polluting Alerts/Events/Activity.

**Relationship with other ADRs**
- Complements, without replacing, **Events** (domain events), **ADR-022** (Activity Feed, orchestration), and **Alerting** (ADR-008).
- Respects **ADR-012/ADR-014** (polling reads; no new channel) and **ADR-006** (state in Postgres).

# **ADR-026**

**Title:** Automatic screenshots on UNIT3D tracker publishing (extraction via ffmpeg on the Agent + upload to image host)

**Status:** Approved

**Context**
UNIT3D trackers (e.g., samaritano.cc) require content screenshots in the upload description — a rule discovered live (`postpone` of a real upload: "Seven Pounds 2008 480p BDRip AAC2.0 H.264-NewArtRiot" rejected citing the standardization wiki, screenshots as an example). Today, ADR-020 builds the description using only the TMDB synopsis; no image is generated or accounted for. The operator needs to extract frames and paste the `[img]` BBCode manually before each publish — 100% manual, easy to forget, and blocks releases already approved in the rest of the process (nomenclature, mediainfo).

**Problem**
(a) Extracting frames requires access to the **media file**, which only exists on the Seed Node (ADR-012: only the Agent talks to the data); (b) the Agent needs a new binary (**ffmpeg**/ffprobe) in addition to the `mediainfo` already required by ADR-020, including on ARM64 Nodes (RPi 3B+); (c) the images need an external **host** (BBCode references a URL, it cannot embed binary) — another third-party secret to store; (d) the result still goes through the mandatory **human review** in the Portal (same gate as ADR-020) before being sent to the tracker.

**Alternatives considered**
1. **Status quo (manual):** zero automation — maintains the friction and the risk of forgetting (root cause of the real `postpone`). Rejected as a goal.
2. **CP extracts the frames:** would violate ADR-012 (the CP does not have access to the Node's data; it would have to pull the entire file just to extract frames — expensive and out of standard). Rejected.
3. **Agent extracts (ffmpeg) + CP uploads/injects BBCode (adopted):** the Agent, which already has the file, extracts N JPEG frames and returns the bytes to the CP via the existing command queue (ADR-014); the CP (which already stores the third-party secrets — TMDB, ADR-020) uploads each frame to the image host and builds the BBCode. Maintains the separation of responsibilities (Agent = local data, CP = orchestration/secrets) and reuses the existing infrastructure.

**Adopted decision** (alternative 3)
- **New `extract_screenshots` command** in the ADR-014 queue (`payload: {hash, screenshot_count}`): the Agent resolves the torrent's `ContentPath` in the Engine (same `ContentPathOf` method already used in the migration, ADR-018), finds the largest regular file under that path (heuristic already used by `mediainfo`, ADR-020), and runs `ffprobe` (duration) + `ffmpeg` (one frame per timestamp) at **N points evenly distributed between 10% and 90%** of the duration (avoids opening logo and end credits). Returns the JPEGs in base64 in the command's `result` (no new column — `commands.result` is already `jsonb` with no practical limit). Best-effort per frame: isolated failure does not abort the others; total failure (e.g., missing ffmpeg) marks the command `FAILED` with the reason.
- **CP waits for the result in a blocking manner** (reuses/generalizes the `waitCommandDone` already used in ADR-019's `prepare_upload` — extracted to `command.Service.WaitDone`, with a new `ResultOf` to read the `result` of a completed command), decodes the frames, and calls the **ptpimg** client (`internal/ptpimg`, same style as the `unit3d` client): multipart upload authenticated by `api_key`, returns the direct URL of each image. Builds the BBCode (`[img]url[/img]`, one per line).
- **Image host secret:** `PTPIMG_API_KEY` follows the **same pattern as TMDB** (ADR-020) — env var read once on CP boot (`internal/config`), **not** added to the tunables catalog (it is not behavior, it is a secret).
- **Number of screenshots is tunable** (`publish.screenshot_count`, default 4, new category `CatPublish`) in the existing catalog (ADR-023) — editable in the Portal panel without redeployment, without new code in the Portal for the tunable itself (the panel is already generic).
- **Human review preserved:** the Portal exposes a "Generate screenshots" button in the publish flow (same screen as ADR-020) that calls the new endpoint and **inserts** the returned BBCode into the description field — the operator reviews/edits/removes before publishing, exactly as they already do with inferred name/category/IDs. Nothing is generated or sent to the tracker without this explicit action.
- **Scope:** applies to both new publications (ADR-019) and **already seeding** torrents (the real case that motivated this ADR) — unlike mediainfo, which only runs at the moment of content upload, the screenshots command can be triggered at any time, against the torrent already in the Engine.

**Consequences**
Benefits:
- Eliminates the root cause of `postpone` due to missing/incomplete screenshots, without giving up human review; reuses the command queue (ADR-014) and the third-party secret pattern (ADR-020) — no new Agent↔CP channel.
Limitations/costs:
- New mandatory binary on the Agent (**ffmpeg**/ffprobe) on all Nodes, including ARM64 (RPi 3B+) — same class of operational cost already accepted for `mediainfo` (ADR-020).
- **ptpimg does not have an officially documented public API**; the format used (multipart, `file-upload[]`, response `[{code, ext}]`) is the one agreed upon by auto-upload tools for private trackers and requires **live validation** on the first rollout (same caution already recorded in ADR-021 for UNIT3D's `SlotLimit`). Image host failure must not block publishing — the operator can always paste manual BBCode as a fallback.
- Frames are generated **on-demand**, not persisted (no new table/column) — generating again costs a new command+upload round.

> **Note (2026-07-18):** the risk recorded above materialized — **ptpimg.me went down** and never had a documented API. **ADR-030 supersedes the host choice** (ptpimg → **imgbb**, with a public API), and the key moved from env to the catalog (ADR-029). Everything else in this ADR (Agent extracts via ffmpeg, CP hosts and assembles BBCode, human review) remains valid.

**Relationship with other ADRs**
- Extends **ADR-020** (publishing to a UNIT3D tracker): one more inferred field (BBCode for screenshots) subject to the same human review before `POST /api/torrents/upload`.
- Reuses **ADR-014** (command queue via polling) and generalizes the blocking wait introduced by **ADR-019** (`prepare_upload`/`waitCommandDone`).
- Reuses **ADR-023** (tunable catalog, hot-reload) for `publish.screenshot_count`. The image host key, which was kept in env here, moved to the catalog via **ADR-029**.
- Reaffirms **ADR-012** (Agent only acts on local data; CP centralizes secrets/orchestration).
- **Partially superseded by ADR-030** regarding the image host (ptpimg → imgbb).

# **ADR-027**

**Title:** Agent update orchestrated by the Control Plane (self-update via command, manual rollout)

**Status:** Approved

**Context**
Updating the Agent is currently 100% manual: build on node-01, `pct pull` the binary, `pct push` to each Node (and `scp` in between when the Node is on another Proxmox host), `systemctl restart` one by one. The procedure is documented in the `seedgrid-ops` skill precisely because it is not obvious — it includes the trap of stopping the Agent before overwriting (an in-use binary throws `Text file busy` and the push fails silently, leaving the OLD binary in place). The cost is felt with every release (3× in a single session: v1.0.0 → v1.1.0 → v1.1.1) and scales linearly with the fleet. During the delivery of the ADR-021 refinement (Jul/2026) the effect became explicit: node-01 and node-02 received the new binary and **node-03 was left behind** (host inaccessible at the time), with nothing in the system recording or signaling the divergence — the operator has to remember.

**Problem**
(a) The CP **decides** and the Agent **executes** (ADR-012), and the CP never connects to the Node — so the update must be **Agent-initiated**, like everything else (ADR-014, polling); (b) the binary currently published at `/node/seedgrid-agent` (ADR-017) is **ARM64**, a legacy from the RPi 3B+, while the current fleet is **amd64**: an update that downloaded this path would break all Nodes at once — serving by **architecture** is a prerequisite, not a refinement; (c) swapping the binary of a running process and restarting it is delicate (binary in use, non-atomic replace, permissions for `systemctl`); (d) a bad binary (wrong architecture, corrupted download) brings down the Node, and the Agent — being the process itself — cannot fix itself after it dies; (e) the operator needs to **see** that an update is available, per Node, without manually checking the git tag.

**Considered alternatives**
1. **Status quo (manual):** maintains the toil and silent divergence among Nodes. Rejected as a goal.
2. **CP pushes the binary (SSH/remote agent):** would violate ADR-012 (the CP would start connecting to the Node) and require access credentials for each Node stored in the CP — a new and unnecessary attack surface. Rejected.
3. **Package manager / Agent container image:** would solve distribution, but the Agent runs as a static binary under systemd (not in a container) precisely to access the Engine and the Node's disk; migrating the execution model is a larger scope than the problem. Rejected for now.
4. **`update_agent` command in the queue, manual rollout (adopted):** the Agent pulls the command (ADR-014), downloads the binary from the URL indicated by the CP, proves it runs, swaps and exits — systemd brings it back up. Reuses all existing infrastructure (durable queue, artifacts served by the Portal in ADR-017, tunable catalog from ADR-023) without a new channel.

**Adopted decision** (alternative 4)
- **New `update_agent` command** (`payload: {version, url, sha256}`): the CP queues it; the Agent, during polling, downloads the URL, checks the **SHA-256**, and only then acts. The CP does **not** send the binary through the queue (a jsonb payload is no place for 6 MB): it sends the URL, and the Agent fetches it — Agent-initiated, consistent with ADR-012.
- **Artifacts per architecture:** the build publishes `/node/seedgrid-agent-<arch>` (amd64, arm64) + a manifest `/node/agents.json` (`{version, binaries: {arch: {file, sha256}}}`), served by the Portal as already done in ADR-017. The ambiguous path `/node/seedgrid-agent` ceases to exist and `bootstrap-node.sh` now resolves the architecture (`uname -m`) — this **incidentally fixes** the onboarding of amd64 Nodes, which today would download an ARM64 binary. The CP reads the manifest via the configured base URL and compares it with `nodes.architecture`/`nodes.agent_version` that it **already** persists from the inventory — no new columns.
- **Manual rollout, one Node at a time:** the Portal shows "update available" per Node (comparing reported version × manifest version) and an **Update** button; nothing moves on its own. Automatic rollout is rejected in this ADR: without heartbeat rollback, a bad binary would propagate through the entire fleet without human supervision — and the gain (not clicking) does not outweigh the risk in a fleet of 3 Nodes with 1 operator. The hook to automate later is the endpoint itself.
- **Swap safety (the critical point):** the Agent (1) downloads to the **same directory** as the current binary, resolved by `os.Executable()` — `rename` is only atomic within the same filesystem; (2) checks the SHA-256 against what the CP sent; (3) runs the downloaded binary with **`--version`** (new flag) and requires exit code 0 — this is the **smoke test** that proves that binary RUNS on that host, catching wrong architecture, corruption, and libc incompatibility **before** any swap; (4) saves the current one as `.prev`; (5) performs an atomic `rename`; (6) **acknowledges the command to the CP** and only then **exits with code 0** — systemd (`Restart=always`, already in the unit) brings the process back up with the new binary. There is no need to invoke `systemctl` or grant new permissions: exiting is enough.
- **Protection against bad binary:** the pre-swap smoke test is the main defense (nothing is swapped for a binary that doesn't run). As suspenders, the unit gets an `ExecStartPre` that tests `--version` of the installed binary and, if it fails, restores `.prev` — it covers post-swap corruption. **Heartbeat rollback** (restoring when the new Agent comes up but stops reporting) is **out** of scope: it would require a supervisor external to the Agent (systemd timer) and covers a case the smoke test doesn't catch — a runtime bug — whose cost today is logging into the Node and restoring `.prev` manually.
- **Confirm before dying:** the ack goes to the CP **before** `exit(0)`, otherwise the command would remain `DISPATCHED` forever and the Portal would show a failure for an update that succeeded.
- **Artifact URL is tunable** (`agent.artifacts_url`, ADR-023): it must be reachable **by the Nodes** (it is not an internal Docker network address). Empty **disables** the feature — no badge, endpoint refuses — a deliberate choice so the CP doesn't invent its own public URL and tell the Node to download from a wrong address.

**Consequences**
Benefits:
- Eliminates manual toil and, most importantly, **silent divergence**: the out-of-date fleet becomes visible in the Portal instead of relying on the operator's memory.
- Fixes the latent onboarding bug (ADR-017) that would serve an ARM64 binary to an amd64 Node.
- Reuses the queue (ADR-014), artifacts (ADR-017), and catalog (ADR-023); no new components or channels; nothing violates ADR-012.
Limitations/costs:
- **Chicken-and-egg**: an Agent prior to this ADR does not know `update_agent` and responds with `FAILED tipo de comando desconhecido` (observed live on node-02 during delivery) — harmless (the CP logs the failure; nothing is swapped), but every existing Node requires **one** manual deploy until the first version with self-update; from the next one onward, it's a button click. This also applies to Nodes whose bootstrap ran before this ADR.
- **No automatic rollback via heartbeat**: a binary that runs but fails at runtime leaves the Node down until manual intervention (`.prev` is still there).
- The `ExecStartPre` lives in the **systemd unit**, which the self-update does not update (it swaps the binary, not the unit): existing Nodes only get the safety net when the unit is reinstalled — the pre-swap smoke test, however, is valid from the very first update.
- An **offline** Node does not update (the command waits in the durable queue and is executed when it comes back online — desired behavior, but the operator sees "update available" until then).
- The CP now depends on the Portal to read the manifest (HTTP); Portal down = no badge (the cluster continues operating normally).

**Relationship with other ADRs**
- Reuses **ADR-014** (durable command queue via polling) and reaffirms **ADR-012** (Agent pulls; the CP never connects to the Node).
- Extends **ADR-017** (zero-touch onboarding): the same artifacts served by the Portal are now versioned by architecture and described by a manifest; fixes the single ARM64 binary.
- Reuses **ADR-023** (tunable catalog with hot-reload) for `agent.artifacts_url`.
- Complements **ADR-024** (actual build versioning via git tag): without a reliable `agent_version` there is no way to compare versions — it is the prerequisite that made it possible.

# **ADR-028**

**Title:** Telemetry outside the Portal — notifications via Telegram, Discord, and generic webhook

**Status:** Approved

**Context**
All of SeedGrid's knowledge about itself dies inside the Portal: alerts (ADR-008), events (ADR-006), and the orchestration feed (ADR-022) only exist for those with the tab open. There is **no** outbound channel — a Node offline at 3 AM, a failing storage, or a filling disk are only discovered when the operator decides to look. The Portal is a consultation dashboard, not a surveillance one: it requires presence. For a cluster that runs 24/7 with no one in front of it, the right information needs to go after the person.

**Problem**
(a) Which facts deserve notification varies by operator and by moment — sending everything becomes a flood (the admission controller alone generates several holds per hour) and sending too little defeats the purpose; (b) each channel has its own credentials and format (Telegram: bot token + chat_id, Markdown; Discord: webhook URL, embed; generic: raw JSON), and none can be hardcoded; (c) **where to store these credentials** conflicts with precedent: ADR-026 established that secrets **do not** go into the tunable catalog (TMDB and ptpimg are env vars, read at boot); (d) an external channel being down (network, Telegram rate-limit, dead webhook) **must not** break the flow that generated the alert — notifying is a side effect, not part of the transaction; (e) an alert that opens and resolves in a loop (Node flapping) must not turn the operator's phone into a machine gun.

**Alternatives considered**
1. **Status quo (Portal only):** requires presence; the alert exists but reaches no one. Rejected as a goal.
2. **Alertmanager as an outbound channel:** Alertmanager is already in the stack (ADR-016) and knows how to notify. But today it talks to the CP in the opposite direction (webhook `POST /api/v1/integrations/alertmanager`), and not every notifiable fact is a Prometheus metric — events and orchestration are born in the domain, not in the scrape. Routing them to Alertmanager just to route them back means modeling everything as a metric and maintaining a second routing configuration outside the Portal. Rejected for inverting the source of truth.
3. **A separate notifier binary/service:** a new component to operate, with its own config and deploy, for a function that is intrinsic to the CP (it already knows what happened and already has the config catalog with hot-reload). Rejected due to disproportionate operational cost.
4. **`internal/notify` package in the CP, triggered at the points that already produce alerts/events/activities (adopted):** reuses what exists (ADR-023 catalog, ADR-006/008/022 tracks), adds no new processes or inbound channels, and keeps the decision of what goes out on a single screen.

**Adopted decision** (alternative 4)
- **Three channels, one `Sender` each** (`internal/notify`): **Telegram** (Bot API `sendMessage`), **Discord** (POST to the webhook URL), and **generic webhook** (POST of raw JSON). The generic one is deliberate: it unlocks Slack, n8n, Home Assistant, and whatever comes next **without new code per integration** — Discord is already a webhook, so the marginal cost of the generic channel is almost zero. **Channel with empty credential = disabled**; there is no separate "enabled" flag (a toggle that can diverge from the field being filled is one more inconsistency to maintain).
- **Credentials in the tunable catalog (ADR-023), with hot-reload — EXPLICIT exception to the ADR-026 precedent.** The criterion that ADR-026 actually established is not "every secret goes to env", but rather that a **system credential** (TMDB, ptpimg: product infrastructure, the same for any operator, without which the feature does not exist) is not behavior and does not belong in the panel. A Telegram bot token is of a different nature: it is **user configuration** — it belongs to the person, changes whenever they want, and each operator has their own. Requiring editing the `.env` on the Control Plane host and restarting the controller to change one's own bot is friction with no security payoff (the `.env` is no more protected than the database — both live on the same machine, and the database already stores the tracker's `api_token`). Mandatory consequence: `Spec.Secret` marks the field, and the value **never** comes back in the read API — `GET /settings` returns only `has_value` (boolean), exactly as `trackers.api_token` (ADR-020) already does. `PUT` with an **empty** value = **keep** (otherwise saving any other field would erase the token); to actually **clear** it, `DELETE /settings/{key}` (reset to default, which is empty) — which is also how a channel is disabled.
- **The operator chooses what arrives, per track:** `notify.alerts_min_severity` (`OFF|CRITICAL|WARNING|INFO`, default `WARNING`), `notify.alerts_on_resolve` (default `true` — knowing it **came back** matters as much as knowing it went down), `notify.events` and `notify.activities` (list of types/kinds, `*` for all, **empty by default**). The last two are born disabled on purpose: they are the high-volume tracks, and the default must be useful without being unbearable. **Audit (ADR-025) is left out**: these are the operator's own actions — notifying them of what they just did is an echo, not telemetry.
- **Anti-flood:** `notify.rate_limit_per_min` (default 20) discards the excess in the window and logs how many messages were suppressed (silence without explanation is worse than the missing message). It covers the real-world case of a Node flapping between ONLINE/OFFLINE.
- **Best-effort, always:** sending is asynchronous and no external channel failure propagates to whoever generated the fact — a down Telegram cannot prevent the Alert Worker from registering an offline Node in the database. Failure becomes a log, not a domain error.
- **"Send test" button** (`POST /api/v1/notify/test`): the only honest way for the operator to know the token is correct is to see the message arrive — without this, configuration is guesswork until the first real incident.

**Consequences**
Benefits:
- SeedGrid no longer requires presence: the incident goes after the operator, and information that already existed now has a destination.
- The generic webhook turns "integrating with X" into configuration, not a CP release.
- No new components, processes, or input channels; reuses the existing catalog and tracks.
Limitations/costs:
- **Opens an exception to the ADR-026 precedent**: the catalog can now store secrets. Mitigated by the distinction (system credential × user configuration), read masking, and the `trackers.api_token` precedent. Consciously accepted — if the distinction proves loose, the next ADR will supersede it.
- Tokens now live **in the database** (and its backups) in addition to `.env`; the exposure radius of a dump grows.
- Delivery **without proof of life from the real channel** while there is no credential: tests use a fake server per channel (guaranteeing the payload per channel, masking, rate-limiting, and filters), but "the message reaches your Telegram" is only demonstrated by the test button with a real token.
- Notification is **at-most-once**: without a durable queue, a message lost in a network window is lost (the fact remains in the Portal). Persistent queue/retry is left for if/when it hurts.

**Addendum (Sep/2026) — per-channel filters, and alert exempt from anti-flood.** The original decision had filters and ceiling per **cluster**, with fan-out dispatch to every channel with a credential. Two unforeseen consequences emerged during use:

1. **Without routing, an automation channel becomes a trash can.** When planning to trigger an external flow (n8n) from a notification, the endpoint would receive everything the cluster notifies and discard the rest. Each channel can now have **its own** filters, under a `notify.<canal>.custom` toggle — turned off by default, so that those already using it see no difference. The toggle exists instead of "empty = inherits" because empty **already means** "none" in the tracks.
2. **The anti-flood could discard an alert.** The ceiling was a single window shared by all tracks and applied before fan-out: a burst of activity could consume the quota and cause a `CRITICAL` to be discarded — silently, since suppression only turned into `logger.Warn`. **Alerts are now exempt from the ceiling on any channel**, the ceiling becomes per-channel, and suppression becomes a metric (`seedgrid_notify_suppressed_total`). The principle: an alert exists to bother, and a limiter that discards eats precisely what mattered, arbitrarily. Against a storm, the remedy is dedup (which already exists) and fixing the noisy rule — if delivery protection is ever needed, let it be **coalescence**, never discarding.

This does not change the ADR mechanism (channels, tracks, secret in catalog), only the granularity of the choice and the priority between tracks.

**Relationship with other ADRs**
- Consumes **ADR-008** (alerts), **ADR-006** (events), and **ADR-022** (orchestration feed) — it is an output of these tracks, without altering them.
- Reuses **ADR-023** (catalog with hot-reload) and extends it: `KindBool`, `Options` (validated enum), and `Spec.Secret`.
- **Opens an explicit exception to ADR-026** regarding secrets in the catalog, with the criterion recorded above; the ADR-026 standard remains valid for system credentials (TMDB, ptpimg).
- Follows the API secret masking established by **ADR-020** (`trackers.api_token`).
- Does not interfere with **ADR-016** (Prometheus/Alertmanager), which continues to cover metrics telemetry.

> **Note (2026-07-17):** the distinction "system credential stays in env / user credential goes to the catalog" used by this ADR was **superseded by ADR-029**: to enable the open-source release, EVERY third-party credential tied to the operator's account — including TMDB and ptpimg, cited here as "system" — now lives in the catalog. The mechanism (Secret + masking + seed-from-env) is the same one introduced here.

# **ADR-029**

**Title:** Every third-party credential in the Portal — prerequisite for the open-source release

**Status:** Approved

**Context**
SeedGrid is moving towards being published as open-source. Today, the third-party service credentials consumed by the Control Plane — **TMDB** (metadata enrichment, ADR-020), **IGDB/Twitch** (games, #17), and **ptpimg** (screenshot host, ADR-026) — are read from environment variables at boot (`SEEDGRID_TMDB_API_KEY`, etc.), per the ADR-026 decision ("system secret does not enter the catalog"). ADR-028 just opened an exception for notification channels, but kept TMDB/ptpimg in env, treating them as "system credentials".

The problem with this boundary only becomes visible in an open-source context: **these keys belong to the maintainer's personal account**. A TMDB API key is linked to a TMDB registration; the IGDB pair is someone's Twitch app; ptpimg is a user's account on a private tracker. Publishing the repository with these keys in example env, or worse, hardcoded, means **leaking personal credentials** or delivering a product that "only works with the author's account". No operator who clones SeedGrid has — nor should they inherit — the maintainer's keys.

**Problem**
(a) The "system × user" distinction from ADR-028 does not survive the open-source criterion: from the perspective of whoever clones it, **every** third-party key is theirs, not the "system's"; (b) a key in env requires editing `.env` on the host and restarting the controller — friction for the operator and, for the open project, an invitation to accidentally commit `.env`; (c) the migration cannot break the current deploy, which has had the keys in env for months.

**Considered Alternatives**
1. **Keep in env, document "fill in your keys" in the README (status quo):** it does not leak if `.env` is gitignored, but it maintains friction and the arbitrary boundary, and a `.env.example` with placeholders is easy to fill out incorrectly/commit. Rejected: it does not solve the root cause (third-party config outside the place where the operator configures everything else).
2. **Keys only in the catalog, no seed:** clean for those who clone, but **would break the current deployment** (keys in env would no longer be read). Rejected.
3. **Catalog (ADR-023) with Secret + seed-from-env on 1st boot (adopted):** reuses exactly the mechanism introduced by ADR-028 (`Spec.Secret`, masking on read, seed from `EnvVar` on first startup). The current deployment migrates automatically: on the next boot, each key in env seeds its row in the database and becomes editable in the Portal; from then on, `.env` is ignored. Those who clone start with empty fields and fill them in the UI.

**Adopted Decision** (alternative 3)
- **New `integrations.*` category in the catalog:** `integrations.tmdb_api_key` (Secret), `integrations.igdb_client_id`, `integrations.igdb_client_secret` (Secret), `integrations.ptpimg_api_key` (Secret). Each with an `EnvVar` pointing to the current variable — seed-from-env (ADR-023) migrates the existing deployment on 1st boot, without intervention.
- **Clients now read the key from the Store per call** (no longer a fixed value from boot): `tmdb`/`igdb` receive a `func() string` tied to the catalog; `Enabled()` and each request re-read — so changing the key in the Portal takes effect **without a redeploy** (hot-reload), and clearing it **disables** the integration immediately. `ptpimg` is resolved from the Store in the screenshots handler.
- **Masking identical to ADR-028:** the value never returns in the read API (only `has_value`); empty PUT keeps it; DELETE clears it. Audit logs the event, not the value.
- **The boundary of what stays in env shrinks to the infrastructure of the deployment ITSELF**, which is neither a third-party credential nor "the author's": `POSTGRES_*` (the database is local to the deployment), `SEEDGRID_ADMIN_*` (1st admin bootstrap) and `SEEDGRID_AGENT_REGISTRATION_TOKEN` (shared secret for enrollment). These belong to whoever operates the instance, are generated by them, and have no external owner — they remain in env.

**Consequences**
Benefits:
- The repository can be made public without any of the maintainer's personal keys: `.env.example` contains only the deployment infrastructure, and all third-party integrations are configured in the UI.
- A new operator has **one** place for everything that is theirs (keys, channels, tunables) — consistency that the previous boundary broke.
- Automatic and reversible migration; the current deployment is unaffected.
Limitations/costs:
- Reverts part of ADR-026 and the distinction from ADR-028 — consciously assumed: the open-source criterion is stronger than the "system × user" aesthetic.
- More secrets now live in the database (and its backups). Same class of exposure already accepted in ADR-028; mitigated by masking.
- Integration clients gain an indirection (provider instead of fixed value) — small code cost to gain hot-reload.

**Relationship with other ADRs**
- **Supersedes** the decision to store third-party credentials in env from **ADR-026** and the "system × user" distinction from **ADR-028**; the mechanism (Secret/masking/seed) is from ADR-028.
- Reuses **ADR-023** (catalog, hot-reload, seed-from-env) and the masking from **ADR-020** (`trackers.api_token`).
- Enables the **open-source release** track (pre-release checklist in the backlog): generalizing the keys was one of the items.

# **ADR-030**

**Title:** Screenshot host: switch ptpimg to imgbb

**Status:** Approved

**Context**
ADR-026 chose **ptpimg.me** to host the screenshots that UNIT3D trackers require in the upload description. That ADR already explicitly recorded the risk: *"ptpimg does not have an officially documented public API; the format used is the one conventionally used by auto-upload tools and requires live validation"*. The risk materialized in the worst way — **ptpimg.me has been down for months**, and the integration was never truly exercised (the key was not even configured). Choosing a service without a public contract was a judgment error: without a documented API there is no way to rely on it or know when it breaks.

**Problem**
We need an image host that: (a) has a **public and documented API** (stable contract, no reverse engineering); (b) accepts what the Agent already produces — **base64 JPEG** frames (ADR-026), to avoid changing the Agent→CP flow; (c) returns a **direct URL** to build the `[img]` BBCode; (d) authenticates via the **operator's account key**, fitting the ADR-029 pattern (key in the panel, masked, not in env/code); (e) is accepted in practice by UNIT3D trackers.

**Considered Alternatives**
1. **Stick with ptpimg:** down, no documented API. Rejected — it is the cause of this ADR.
2. **imgbb (adopted):** public and documented v1 API (`POST https://api.imgbb.com/1/upload?key=`), accepts `image` in base64, returns `data.url` (direct link), free, authentication via API key. Confirmed as the host that the samaritano community actually uses. Meets all criteria.
3. **Self-host (serve images from the CP/Portal itself):** would avoid relying on a third party, but the URL needs to be public and stable for the external tracker to fetch — reintroduces CP exposure to the internet (against the current design, internal CP) and becomes an image storage/retention responsibility. Rejected for now; re-evaluate if the need to not rely on an external host arises.

**Adopted decision** (alternative 2)
- **New `internal/imgbb` client** with the same interface as the removed one: `Upload(ctx, apiKey, frames [][]byte) ([]string, error)`. For each frame, it performs a `POST multipart` with the `image` field = **JPEG base64** to `https://api.imgbb.com/1/upload?key=<API_KEY>`, reads `data.url` from the response, and returns the URLs **in input order** (order matters: screenshots appear sequentially in the description). imgbb sends **one image per request** — the client iterates; failure of a single frame aborts with a clear error (a consistent upload is better than a half-baked description).
- **No `expiration`** in the call = **permanent** image (upload screenshots cannot disappear later).
- **Key in the catalog (ADR-029):** `integrations.imgbb_api_key` (Secret, masked, hot-reload), **replaces** `integrations.ptpimg_api_key`. Empty disables screenshot generation, as before.
- **The `ptpimg` package and its key are removed** — dead code pointing to a defunct service only leads to errors.
- **Agent→CP flow unchanged:** the Agent continues extracting N JPEG frames via ffmpeg and returning base64 (ADR-026); only **where** the CP sends them and how it reads the URL back changes.

**Consequences**
Benefits:
- Integration now depends on a **public contract** and a live service; fixes the root cause of the ADR-026 error.
- Zero changes to the Agent; the swap is contained to the client and the screenshot handler.
- Key follows the ADR-029 standard (panel, masked) — consistent with TMDB/IGDB.
Limitations/costs:
- Still **depends on a third party** (imgbb) — if it goes down, screenshots stop. Mitigated by being a service with a stable and widely used API; the self-hosting alternative is recorded just in case it hurts someday.
- **Live validation pending until the operator creates the imgbb key** (free) and runs a real publish — this time with a service that has a documented API, unlike ADR-026.
- imgbb does **one upload per request** (no batch): N frames = N calls. Irrelevant cost for the few screenshots per publish.

**Relationship with other ADRs**
- **Supersedes the host choice of ADR-026** (ptpimg → imgbb); everything else in ADR-026 (Agent extracts via ffmpeg, CP hosts and builds the BBCode, human review) remains valid.
- Follows **ADR-029** for the credential (operator's third-party key in the catalog, masked).

> **Note (2026-10-04):** the limitation "if it goes down, screenshots stop" was addressed in **ADR-040**: the host is no longer single. A tracker can declare its **own site's image host** in its profile, and imgbb becomes the **last link in a chain with fallback**, with the contract and global key of this ADR unchanged. Upon implementation, ADR-040 also closes the imgbb key leak through the network error text (the key travels in the query, and the `*url.Error` carries the entire URL).

# **ADR-031**

**Title:** Incomplete (partial) torrent migration and concurrency lock

**Status:** Approved

**Context**
ADR-018 designed the migration for **complete** torrents: the destination pulls the data, performs a recheck, and the CP only finalizes when the destination reports `SEEDING`. In practice, the operator migrates **incomplete** torrents — which is exactly what they need most when a Node fills up (node-03 at 97% disk, incident on 2026-07-18): to take a torrent from there that **cannot complete due to lack of space** and finish it on a Node with space. It was done manually with `Os.Vingadores.2012` (stuck in the download→no space→queue cycle on node-03) and, migrated to node-01, **completed normally**.

Today this path fails, and fails **silently and badly**.

**Problem**
Three chained defects, confirmed in the code:

1. **Empty tar returned as success (root cause).** `handleTransfer` (`agent/internal/transfer/transfer.go:198`) writes the HTTP 200 header and only then calls `tarPath`. If the content does not yet exist on disk (download not started), the `os.Stat` for `tarPath` fails **after** the 200 — but the `defer tw.Close()` still emits the tar footer. The destination receives **a valid and empty tar with status 200**; `extractTar` reads `io.EOF` on the first entry and returns `nil`. **The pull "succeeds" without having copied a single byte.**
2. **Swarm re-download (backlog bug B3b — same root cause).** With the "successful" pull, `migratePull` proceeds to `AddFile` + `Recheck`. The recheck finds 0% and qBittorrent starts **downloading everything from the swarm** — exactly what ADR-018 exists to prevent (burns ratio on private tracker). Only after 10 minutes does `waitSeeding` time out and the command becomes `FAILED`, with the download already underway.
3. **Success criterion inadequate for partial.** Even with the partial data copied correctly, `waitSeeding` (`agent/internal/commands/commands.go:311`) requires `SEEDING && progress >= 0.999`. A torrent at 60% never satisfies this: the migration is marked `FAILED` even though the copy was perfect.

Add to this the lack of a **concurrency lock**: `DrainNode` queues one migration per torrent all at once, and all of them pull in parallel — which trashes the disk I/O precisely on the Node that is already in trouble.

**Considered alternatives**
1. **Block incomplete torrent migration** with a clear error in the API/Portal. Simple and consistent with a literal reading of ADR-018. **Rejected:** prohibits exactly the use case that motivates the migration (draining a full Node). It would treat the symptom and leave the operator with no way out in the scenario that hurts the most.
2. **Let the destination re-download from the swarm** when the torrent is incomplete (accept the current behavior and just document it). **Rejected:** violates the central principle of ADR-018 (no re-download, preserves ratio) and is the opposite of what the operator wants.
3. **Support first-class partial migration** (adopted): copy what exists, verify via recheck, and let the destination continue the download from where it left off.

**Decision adopted** (alternative 3)

- **Fail early and loud when there is nothing to copy.** The handler now performs `os.Stat` **before** writing any headers: non-existent or zero-length content returns **`409 Conflict`** with a readable message ("torrent has not downloaded data yet — nothing to migrate"), never a 200 with an empty tar. A pull that copies nothing is an **error**, never a silent success.
- **Success criterion proportional to source progress.** The CP now sends the **`min_progress`** field in the `migrate_pull` payload — the torrent's progress on the source at the time the migration was created, minus a tolerance. The destination Agent, after the recheck, waits for the progress to reach `min_progress` (instead of requiring `SEEDING`), and only then responds `DONE`. A complete torrent continues to behave as before (`min_progress ≈ 1.0` ⇒ requires seeding); a 60% torrent completes the migration at 60%. *(**#87:** waiting for the recheck and checking the floor moved from the Agent to the CP — see the addendum at the end of ADR-018; the floor is the same, and the pull's `DONE` no longer means "verified".)*
  - The tolerance exists because the recheck may invalidate the **boundary piece** (partially written when the download stopped). Losing a piece at the edge is normal and does not invalidate the migration.
- **The destination continues the download.** After the recheck, the torrent becomes active on the destination and resumes what is missing from the swarm — the correct and desired behavior here, unlike the full re-download in issue 2. The ratio savings from ADR-018 are proportionally preserved: only what was **missing** is downloaded.
- **Source removal follows the operator's rule.** The CP's `complete()` continues to remove the torrent from the source along with its data after the destination's `DONE` — the destination has the same bytes and is the one that will keep downloading. The rule registered by the operator ("keep destination, delete source if 100% seeding") now reads, in its general form: **keep the destination, delete the source when the destination confirms it has at least what the source had.**
- **Concurrency lock.** New tunable **`migration.max_concurrent`** in the dynamic catalog (ADR-023, default **1**), and migrations gain the **`QUEUED`** state: the reconciler promotes `QUEUED → PENDING` (queuing the serve/pull commands) only while there is headroom under the ceiling. The reconciler now acts as a **scheduler in addition to a reconciler**. A drain of 40 torrents no longer turns into 40 simultaneous pulls.

**Consequences**
Benefits:
- Unlocks the real use case: draining a full Node moves incomplete torrents, which **finish** on the destination with space. It is the missing piece to address the root of the disk incident.
- Eliminates silent re-downloads from the swarm — the most expensive defect, because it burns ratio on private trackers without the operator noticing.
- Errors become readable ("nothing to migrate") instead of "zero-length file" followed by a failure 10 minutes later.
- `max_concurrent` protects disk I/O precisely on the Node in trouble, and is adjustable in the dashboard without redeployment.
Limitations/costs:
- **Sparse file transfer:** with pre-allocation turned off (qBittorrent default), a 10% torrent takes up little disk space but has a full `size`; tar transfers the **full** size (holes become zeros on the network). Migrating a sparsely downloaded partial is therefore more expensive in network and destination disk than the progress suggests. Accepted for now — it is a LAN and the alternative (sparse tar / piece map) is much more complex. **Registered as a known limitation**, and it is one more reason for conscious disk admission (backlog item B1) to check the destination space **by total size**, not by what has been downloaded.
- The `min_progress` criterion is a snapshot of the moment the migration was created: if the source advances between creation and the pull, the destination may arrive with progress greater than required — harmless (the test is a floor, not an equality).
- `QUEUED` adds a state to the migration state machine (one more transition to maintain and test).

**Relationship with other ADRs**
- **Extends ADR-018**, which remains fully valid in its essentials (direct destination←source pull authorized by a single-use token, CP out of the data-plane, no loss in case of failure). What changes is the **completion criterion**: from "destination is SEEDING" to "destination has at least what the source had".
- Uses the **ADR-023** catalog for `migration.max_concurrent` (hot-reload, no redeployment).
- Practical prerequisite for **conscious disk admission** (backlog B1): without migrating partials, draining a full Node does not resolve the torrent that is stuck precisely due to lack of space.

> **Robustness fix — atomic `complete()` (jul/2026).** Finalizing a migration has three effects: queuing `remove_torrent` on the source, restoring/soft-deleting the torrent row, and marking the migration `COMPLETED`. The reconciler only removes the migration from `PENDING` when it becomes `COMPLETED`; meanwhile, it re-executes `complete()` every cycle (~15s). In the original implementation, the `remove` was queued **first** and outside a transaction, so any failure in the subsequent steps left the migration `PENDING` **with the remove already queued** — and the next cycle would queue another, indefinitely. This generated **27.293 `remove_torrent`** for a single torrent (`Rick.and.Morty.S09E01`) in 5 days: the restore collided with `uk_torrents_node_hash` because the destination already had a live copy of the hash (the `destHasRow` fix closed this specific collision, but not the structural fragility). Now the three effects run in a **single transaction** and the `remove` is queued **last**: failed → rollback, nothing queued, clean retry; succeeded → 1 remove + `COMPLETED` commit together, making the loop impossible. Transactional enqueue via `command.Service.EnqueueTx`. Regression covered by reconciler idempotency tests (counted by payload hash).

# **ADR-032**

**Title:** Disk-aware admission (block + alert on space)

**State:** Approved

**Context**
Nothing in the system looks at disk space when deciding where a torrent goes. The Scheduler chooses the Node by health score and number of torrents (ADR-021 does not touch disk); migration (ADR-018/031) only validates ONLINE + endpoint. The result was the **root of a cascade** observed in production (incident 2026-07-18): a Node at 97% disk (node-03) received/held large torrents that **never completed** due to lack of space — the `download → sem espaço → volta pra fila` cycle still fed "ghost seeds" (torrent stuck at 0% looked alive) and migrations that re-downloaded. The proof: `Os.Vingadores.2012`, stuck in this cycle on node-03, **completed on the first try** when migrated to a Node with space.

**Problem**
Prevent a torrent from being **placed** (new add or migration) on a Node that does not have space for it, and **alert** when a Node is already committed beyond what the disk can handle — deciding (a) which number to use to measure space, (b) where the disk data comes from, (c) what to do when it does not fit.

**Decisions (confirmed by the operator, 2026-07-18)**

- **(a) Always measure by the TOTAL SIZE of the torrent.** An incomplete torrent **will download entirely** and occupy full `size_bytes` — reserving by downloaded amount underestimates and recreates the problem. Formalization that avoids **double counting** what is already on disk:
  > **projected free space (Node)** = `heartbeats.disk_free_bytes` − Σ (per live torrent on the Node) `size_bytes × (1 − progress)`
  >
  > The `disk_free_bytes` already reflects the bytes **already written**; the sum is what the already present torrents **will still write** until they complete. A complete torrent contributes 0 (it is already in free). A new torrent (or a migration) "fits" if `espaço projetado livre ≥ size_bytes + margem`.
- **(b) Disk data comes from the HEARTBEAT.** `heartbeats.disk_free_bytes`/`disk_total_bytes` already exist (the Agent reports `/data` every ~5s). **Does not** wait for the detailed SMART inventory (backlog F1) — this solves the pain immediately with what we already have.
- **(c) BLOCK *and* ALERT** (the operator asked for both, not one or the other):
  - **Block** at the decision point:
    - **Migration (`Migrate`):** the size is always known (the torrent was already on a Node). If the destination cannot hold `size_bytes + margem`, refuse with `ErrDestNoSpace` — readable message, torrent stays where it is.
    - **Add via `.torrent` (`AddFile`):** the size comes from parsing (new `Meta.Size` field). The Scheduler now **excludes** Nodes without space; if **none** fit, refuse the add (`ErrNoNodeWithSpace`).
    - **Add via magnet:** the size is **not** known in the CP until metadata resolves in the Agent → **does not block on add**; the Scheduler only **prefers the Node with the most free space** (ranking by projected space). Covered by the alert + migration later. Conscious limitation (see Consequences).
  - **Alert** continuously: new **`NODE_DISK_FULL`** alert (WARNING, ADR-008) for every ONLINE Node whose **projected free space < margin** — that is, it has committed to more than the disk can finish. Message in the operator's language: *"Node X does not have space to complete all queued torrents (~N missing). Migrate torrents to another Node."* Deduplicates by `(code, node_id)` and auto-resolves when there is slack. Catches the Node that is **already** maxed out (the node-03 case), which blocking alone does not undo.
- **Safety margin** = tunable **`admission.disk_min_free`** (bytes, ADR-023, default **5 GiB**): slack that is not allowed to be committed, so the OS/engine can breathe. It is the same number used in the "fits?" check and the alert trigger.
- **Where the logic lives:** an `internal/capacity` package with the projected space expression (single source of truth) — consumed by the Scheduler, migration, and alert, so the three never diverge in the calculation.

**Alternatives considered**
1. **Alert only** (do not block). Simpler and never blocks legitimate operations, but lets the operator place torrents where they don't fit and deal with it later. **Rejected** by explicit decision ("block and alert").
2. **Measure by downloaded** (`size × (1−progress)` as the new torrent's requirement). Underestimates: the torrent grows to the total and reintroduces the overflow. **Rejected** — the operator was direct: "it will download entirely and occupy the equivalent space".
3. **Wait for the SMART inventory (F1)** for a per-device disk view. Richer, much larger. **Deferred** — the heartbeat already brings `/data`, which is what matters here.
4. **Transactional space reservation** (debit a "budget" on admission and credit on completion/removal). Precise under concurrency, but requires a new reservation table and reconciliation. **Rejected for V1** — projected space calculated on demand from the real state (`torrents` + heartbeat) is sufficient for 3 Nodes/1 operator and does not accumulate state to synchronize.

**Consequences**
Benefits:
- Cuts the root of the cascade: large torrents are no longer placed where they won't complete; the already maxed out Node is signaled with a clear action (migrate).
- Reuses existing data (heartbeat) — zero changes to the Agent.
- A single number (`disk_min_free`) governs blocking and alerting, adjustable in the dashboard without redeployment.
Limitations/costs:
- **Magnets without size are not blocked on add** — they only get the Node with the most free space and fall into the alert/migration net. Private trackers use `.torrent` (with size), so the common path is covered; public magnets are the conscious gap.
- **Unknown `size_bytes`** (newly added torrent, unresolved metadata) → the check is **skipped** (cannot judge) instead of blocking — avoids false positives that would block legitimate operations. The alert covers the Node after the size appears.
- **Projected space is an estimate under concurrency:** two nearly simultaneous adds/migrations might both "fit" and together overflow (there is no reservation lock). Accepted in V1 (`migration.max_concurrent=1` already serializes migrations; concurrent adds from 1 operator are rare); the alert catches the result.
- **Sparse file (ADR-031):** a partial takes up less disk space than `size_bytes` suggests right now, but **will** take up the total — which is exactly why it is measured by the total. Consistent.

**Relationship with other ADRs**
- Complements **ADR-021** (admission): that one handles *how many* are downloading (L ceiling, peers); this one handles *where they fit* (disk). Orthogonal.
- Pairs with **ADR-031**: partial migration unblocks draining a full Node; disk-aware admission **chooses a destination that fits** and **flags** which Node to drain.
- Uses the catalog from **ADR-023** for `admission.disk_min_free` and the alert pattern from **ADR-008**.
- **Anticipates, rather than replaces, the SMART inventory (F1):** when the Agent reports disks/SMART, the `DISK_LOW`/`SMART_CRITICAL` rules add to this one (hardware failure ≠ lack of space).

# **ADR-033**

**Title:** Maintenance Worker — scheduled self-healing of the Cluster State (dead GC, command reaper, orphan migration reconciliation)

**Status:** Proposed

**Context**
The Cluster State accumulates **residue** that no existing path cleans up. Three forms observed in production (incident 2026-07-18):
- **Soft-deleted torrent rows that never disappear.** Every removal (operator, completed migration, hash collision on `SyncStates` adoption) performs a *soft delete* (`deleted_at`), by design — but nothing performs the *hard delete* afterward. The `status` of these rows freezes at the last synchronized value (`SyncStates` filters `deleted_at IS NULL`), creating the confusing effect of "SEEDING" or "DOWNLOADING" rows that no longer exist. **68** of these were found, from 05/07 to 18/07.
- **Commands stuck in `DISPATCHED`.** A command delivered to the Agent that is never acknowledged (Agent restarted, torrent disappeared before execution) stays `DISPATCHED` **forever** — there is no timeout to move it to `FAILED`. Two `pause_torrent` commands have been stuck like this since 13/07 (5 days), targeting an already removed ghost torrent.
- **Stuck `PENDING` migration.** The 15s reconciler reprocesses `PENDING` migrations, but if `complete()`/`fail()` fail persistently, the migration never leaves `PENDING`. This was the mechanics behind the loop of **27.293 `remove_torrent`** commands in 5 days (the specific cause was fixed by making `complete()` atomic — ADR-031 —, but nothing guarantees that *another* future cause won't trap a migration the same way).

The common pattern: **the desired state diverges from the actual state and no loop reconverges them.** Each root bug was (or will be) fixed, but the **safety net** that cleans up what slips through is missing — without requiring an operator watching the database.

**Problem**
Give the Control Plane a **periodic and idempotent self-healing** loop that (a) purges dead residue beyond a retention window, (b) expires stuck commands, and (c) unblocks orphan migrations — without introducing a new external process/scheduler, without deleting what is still useful history, and without the risk of touching live state.

**Considered Alternatives**
1. **External cron / manual cleanup script.** A `cron` on the host running GC SQL. Simple, but: it lives outside the CP (another thing to operate, version, monitor), it does not read the config catalog with hot-reload (ADR-023), and raw SQL without domain invariants is dangerous. **Rejected** for fragmenting operations and duplicating schema knowledge outside the CP.
2. **Fix only the root causes, without a safety net.** Already done for the remove loop (ADR-031). But it treats each bug reactively and does not clean up existing residue or that of future yet-unknown bugs. **Rejected** as a complete solution — necessary, but not sufficient.
3. **Immediate purge (hard delete instead of soft delete).** It would eliminate the accumulation at the source, but it destroys the audit/rollback window that the soft delete exists to provide, and couples the decision to "remove" with the decision to "forget". **Rejected** — the value of soft delete is precisely the deferral.
4. **Internal Maintenance Worker with scheduled idempotent routines (adopted).** A background worker in the CP, following the same pattern as existing ones (Admission 20s, Migration reconciler 15s, Alert 30s): goroutine with ticker, interval and thresholds as tunables (ADR-023, hot-reload), heartbeat exposed in the health API. It reuses everything that already exists; the **Maintenance Worker** concept is already foreseen in the Glossary.

**Adopted Decision** (alternative 4)
A new `internal/maintenance` worker, started by the `seedgrid-controller` like the others, running every `maintenance.interval` (default **1h** — residue is not urgent; the goal is hygiene, not latency). Each pass executes three **independent and idempotent** routines (one failing does not prevent the others; none act on live state):

1. **GC of soft-deleted torrents** — *hard delete* of rows with `deleted_at < now() − maintenance.soft_delete_retention` (default **7 days**: keeps a recent audit/rollback window in the database; long-term documentation lives elsewhere, in the incident log). The physical delete triggers the existing `ON DELETE CASCADE` (`torrent_files`, `torrent_migrations`, `torrent_stat_samples`); `NO ACTION` references that preserve history (`activities`, and `events` if applicable) have their link **set to NULL** beforehand, keeping the cluster log intact; `torrent_scheduler` (ephemeral) is deleted along with it. **Hard safeguard:** every `DELETE` carries `AND deleted_at IS NOT NULL` — impossible to touch a live row.
2. **Stuck command reaper** — a command in `DISPATCHED` (delivered, never acknowledged) for more than `maintenance.command_stuck_after` (default **15m**) becomes `FAILED` with an explanatory error. Idempotent by nature (only acts on `DISPATCHED`). Does not touch `PENDING` (not yet delivered — it might legitimately be waiting for polling).
3. **Orphan migration reconciliation** — `PENDING` migration stuck for more than `maintenance.migration_stuck_after` (default **30m**) without converging → `fail()` with reason, releasing the torrent (which remains at the source, without loss — ADR-018 invariant). It is a *backstop* for the 15s reconciler, not a replacement: it only acts on what got stuck far beyond the normal cycle.

All thresholds are tunables from the catalog (ADR-023), editable in the dashboard with hot-reload; `maintenance.interval = 0` disables the entire worker (escape hatch). The first run in production cleans up the already accumulated residue (the 64 remaining dead rows after the manual incident cleanup).

**Consequences / Limitations**
- **Deliberate loss of database history beyond 7 days.** A row dead for more than 7 days and its stats/migration/file disappear for good. This is the accepted trade-off: long-term recording is the responsibility of incident documentation, not the operational state. Adjustable via the tunable if the window proves too short.
- **A new worker to operate and test** (additional state: none; additional behavior: 3 routines). Mitigated by following the already established pattern and by each routine being independent and idempotent.
- **Does not replace fixing root causes.** The reaper and reconciliation handle bug *symptoms*; the root cause of each must still be fixed (as ADR-031 did). The worker ensures that a symptom does not persist for days while the cause is not found.
- **GC competes for I/O with the operation.** Mitigated by the long cadence (1h) and by operating in small batches (the volume of dead rows per hour is low); it can get a `LIMIT` per pass if necessary.

**Benefits**
- **Self-healing without an operator:** the residue of the three types no longer requires someone watching the database — which motivated the request ("how do we ensure it doesn't repeat and, if it does, how to clean it up").
- **Safety net for future bugs:** even a still unknown cause that stalls a migration or command is contained in minutes/hours, not days.
- **Total reuse:** no new process/scheduler; same worker pattern, same config catalog, same health API.

**Relationship with other ADRs**
- **Safety net for ADR-031** (migration): ADR-031 fixed the root cause of the loop (`complete()` atomic); this one contains any migration that still gets stuck.
- **Reuses ADR-023** (tunables catalog with hot-reload) for interval and thresholds, and **ADR-014** (durable command queue) — the reaper operates on the command lifecycle without creating a new channel.
- **Does not alter ADR-006** (events) nor soft delete as a concept: soft delete continues to act as if it "removes"; this ADR only defines **when** the soft-deleted record is finally forgotten.
- Fills in the **Maintenance Worker** already named in the Glossary (until now, a concept without implementation).

# **ADR-034**

**Title:** Seed Nodes connectivity via IPv6 under CGNAT — qBittorrent in `network_mode: host` + `accept_ra=2`

**Status:** Approved

**Context**
The Seed Nodes run behind a residential connection under **CGNAT** (carrier IPv4 in `100.64.0.0/10`): there is no public IPv4 of their own and **no inbound v4 port is forwardable** — WAN→Node port-forwarding is ineffective. Inbound connectability (being *connectable* to the tracker/seeders) depended, therefore, **exclusively on IPv6**, delivered by the ISP via SLAAC (RA announcing `2001:db8:633:8e00::/64`, `auto` flag).

Two factors broke IPv6 in practice (Jul/2026 incident):
- **Proxmox/LXC did not form a global v6 address.** Since the host routes (`net.ipv6.conf.<iface>.forwarding=1`) and Docker turns on forwarding inside the LXC, and `accept_ra` was set to `1` (host) / `0` (LXC), the kernel **ignores the Router Advertisement** — rule: an interface that does forwarding only accepts RA with `accept_ra=2`.
- **qBittorrent runs in Docker, which does not expose IPv6 to the container by default** — even with the LXC having v6, the qBt process had no v6 address to **announce** to the tracker.

Observed symptom: **private** tracker torrents stopped at 0% (public ones downloaded normally). The tracker flagged the account as "port closed"; the seeders (seedboxes) **rejected** the unreachable client (RST μTP), and qBittorrent reacted by banning the peer — blocks that appeared as **"IP filter"** even with the qBt filter turned off (symptom, not cause). The root was **loss of IPv6 connectivity**, not filter/IDS/network.

**Problem**
Restore the connectability of the Seed Nodes under CGNAT, where the only possible inbound path is IPv6, deciding: (a) how the host/LXC obtains a global v6 address persistently, and (b) how the qBittorrent process (in a container) starts having and announcing this v6.

**Alternatives Considered**
1. **IPv4 port-forwarding on the router.** Ineffective under CGNAT (no public IP). Rejected.
2. **VPN with port-forward bound to qBt** (AirVPN/Proton). Works regardless of the ISP, but adds monthly cost, an extra hop, and bind/route complexity per Node. **Rejected as the default** (Plan B if the ISP removes IPv6).
3. **IPv6 on the Docker daemon** (`ipv6: true` + `fixed-cidr-v6` + NDP proxy). With a single `/64` via SLAAC (no prefix delegation), it requires `ndppd` and fragile subnetting. Rejected due to complexity.
4. **`network_mode: host` on the qBt container.** The container inherits the LXC network stack (including global v6), announces v6 naturally, and becomes reachable at `[v6]:BTport`. The Agent already talks to qBt at `127.0.0.1:8080` (does not use service name), so host networking breaks nothing; `node-exporter` already used this mode. **Adopted.**

**Adopted Decision**
- **IPv6 on the host and LXC via `accept_ra=2`**, persisted in `/etc/sysctl.d/99-seedgrid-ipv6.conf` with `net.ipv6.conf.default.accept_ra=2` (ensures interfaces created after the sysctl) **+** `net.ipv6.conf.<iface>.accept_ra=2` (vmbr0 on the host, eth0 on the LXC). The host and LXC now form SLAAC on the ISP's `/64`.
- **qBittorrent in `network_mode: host`** in `deploy/agent/docker-compose.yml` (the `ports:` mapping is removed; qBt binds 8080/WebUI and the BT port directly to the LXC interfaces, v4 and v6). The `listen_port` per Node remains set by the Agent (ADR-014/017).
- **Network requirement (manual, outside code):** IPv6 has no NAT/port-forwarding, but the router firewall blocks inbound v6 by default → a **v6 firewall rule allowing inbound** on the BT port of each Node (`[v6-do-node]:BTport` TCP+UDP) is required. Replaces the old "forward WAN v4 → Node" (ineffective under CGNAT).

**Consequences**
Benefits:
- Restores connectability at no cost (uses the carrier's native IPv6); private torrents start downloading again.
- `network_mode: host` is simpler than port NAT (one less layer) and was already the default for `node-exporter`.
- Robust persistence across reboots via `default.accept_ra=2`.

Limitations/costs:
- **Depends on the carrier's IPv6.** If they stop announcing the `/64`, the Nodes become unreachable again (plan B = VPN with port-forward).
- **SLAAC prefix may rotate** (RA lifetime ~2h): the Node's v6 address may change; the v6 firewall rule should target the Node by something stable (host/MAC), not the literal address — the qBt announce updates itself.
- **With no v6 NAT**, the Node is directly reachable via v6 — security now relies entirely on the router's v6 firewall rule.

**Relationship with other ADRs**
- **Complements ADR-017** (single BT port per Node): the port remains the same; only the inbound transport changes (v6 instead of v4 port-forward).
- **Does not change ADR-015** (stack) nor the Agent↔qBt contract (ADR-014): the Agent remains at `127.0.0.1:8080`.
- Documents the reason `deploy/agent/docker-compose.yml` uses host networking.

**Addendum (Oct/2026) — privacy extensions disabled (#142).** The IPv6 requirement above has an unwritten demand: the Node must have **a single global address in use**. With IPv6 privacy extensions (RFC 4941) enabled — the default on several distributions, which set `use_tempaddr = 2` in `sysctl.d` — the Node gets, in addition to the stable address, a **temporary address that rotates automatically**, and outbound traffic uses the temporary one. The operator whitelists the stable one in the firewall; the tracker sees the other, and the rule no longer matches what is actually announced.

The `bootstrap-node.sh` script now writes `net.ipv6.conf.{all,default,<interface>}.use_tempaddr = 0` to `/etc/sysctl.d/99-seedgrid-ipv6.conf` (the same file that persists the `accept_ra=2` from this ADR), applies the keys immediately, and removes any existing temporary addresses. It is idempotent and preserves the other lines in the file. It does not change this ADR's decision (host networking + `accept_ra`): it completes its network requirement. See `docs/19 - Deployment.md` §50.2.

**Addendum (Oct/2026) — deterministic suffix (ADR-038, #202).** The limitation above ("the v6 firewall rule should target the Node by something stable, not the literal address") has been addressed: the Node now has a **deterministic** IPv6 suffix (`<prefixo>::5eed:<N>`, with `N` coming from the Control Plane's port sequence), so the address no longer depends on the MAC, survives Node recreation, and is known before the Node exists. The **prefix** rotation by the provider still applies and remains covered by IPv6 change detection (ADR-036).

# **ADR-035**

**Title:** Role-based authorization (RBAC) for operators — admin, publisher, downloader, and viewer profiles

**Status:** Approved

**Context**
Since V1, the Control Plane has had operator authentication (Bearer session, `users`/`sessions`) but **no authorization**: any authenticated user has full access to all protected routes (`RequireOperator` only injects the operator into the context). The identity schema was born with the `roles`, `permissions`, `role_permissions`, and `user_roles` tables (migration 00001), but they were never populated or checked — `Bootstrap` only creates the `Administrator` role and the initial admin user. The actual operation of SeedGrid involves more people (family/acquaintances consuming content), and there is no way to delegate limited access: a `viewer` should not be able to remove torrents, nor should a `downloader` publish to the tracker.

**Problem**
Define an authorization model for Portal/API operators that: (a) preserves the existing admin's full access; (b) introduces lower-privilege roles (publisher, downloader, viewer); (c) is mandatory on all protected routes; (d) allows user management via the API without breaking existing workflows.

**Considered alternatives**
1. **Authorization via a boolean flag on the user** (is_admin). Simple, but does not scale to 3 lower-privilege roles and does not express granular permissions per resource/action. Rejected.
2. **Role middleware per route** (e.g., `RequireRole("publisher")`). It works, but it binds each route to a single role — a future intermediate role would require rewriting the routes. The existing schema (`permissions` + `role_permissions`) allows mapping routes to permissions and roles to sets of permissions, decoupling route ↔ role. Rejected as the primary mechanism.
3. **Permission-based RBAC on the existing schema**: each protected route requires a permission (`resource:action`); roles aggregate permissions via `role_permissions`; the middleware checks `user.HasPermission(perm)`; admin resolves via wildcard. Reuses the schema from migration 00001, maintains granularity, and allows new roles without touching routes. **Adopted.**

**Adopted decision**
- **Four system roles** (seeded in migration 00034): `Administrator` (full access), `Publisher` (manages content: torrents, trackers, publishing), `Downloader` (consumes: reads and downloads), `Viewer` (read-only).
- **Permissions in `resource:action` format**, seeded in the same migration, associated with roles via `role_permissions`. Administrator resolves via wildcard (`*`) — no explicit seed is needed for each new permission.
- **`auth.User` now loads `Roles` and `Permissions`** in `Authenticate` (JOIN with user_roles/role_permissions); gains the `HasPermission(perm)` method.
- **`httpx.RequirePermission(perm)` middleware** runs INSIDE `RequireOperator` (which already has the user in context) and returns **403** when the permission is missing. Each protected route declares its permission when registered; routes requiring more than one permission combine middlewares.
- **`auth/password` (change own password) requires only a session** — any authenticated operator can change their own password.
- **User management (admin)**: new `internal/users` package + `GET/POST /api/v1/users` and `PUT/DELETE /api/v1/users/{id}` routes (create, list, update, disable, assign roles, reset password) — all with `users:*` permission.
- **Bootstrap** now ensures the four system roles (idempotent) in addition to the admin user.

**Consequences**
Benefits:
- Least privilege by default: viewer cannot mutate anything, downloader cannot publish, publisher cannot manage users/settings.
- Reuses the existing identity schema (migration 00001); migration 00034 is just an idempotent seed.
- Routes ↔ permissions ↔ roles are decoupled: new role = combine existing permissions, without touching routes.
- Audit (ADR-025) keeps working: the actor is the already-injected `auth.User.Username`.

Limitations/costs:
- Permissions are global per operator (not per resource/object) — e.g., a publisher cannot be restricted to "only node-02". If that becomes necessary, it will be addressed in a future ADR (per-object ACL).
- Every new route must declare its permission explicitly — the absence of a permission on a protected route is a review error, not a deny-by-default (deny-by-default on new routes depends on registration discipline).
- Requires care when migrating the existing admin: `Bootstrap` already associates `Administrator`, and the wildcard ensures nothing breaks.

**Relationship with other ADRs**
- **Complements ADR-025** (audit): authorization is the other half of access control; the actor remains the operator's username.
- **Does not alter ADR-014** (commands) or Agent authentication (the Node's own credential) — RBAC applies to Portal/API operators.
- **Reuses the schema from migration 00001** (identity) — no table changes, just a seed.

# **4. Principles**
Every ADR must:
- record the context;
- justify the decision;
- document the consequences;
- remain versioned.
An ADR does not describe implementation, only architectural decisions that guide the platform's overall evolution.

# **5. ADR Evolution**
ADRs represent current architectural decisions.
Architecture evolution must occur through the creation of new ADRs, preserving the history of previous decisions.
An existing ADR must not be modified to alter its original meaning.
When a decision is no longer valid, a new ADR must explicitly supersede it.
# **6. When to Create a New ADR**
A new ADR must be created whenever changes affect the platform architecture.
Examples:
- introduction of new components;
- significant change in inter-service communication;
- replacement of fundamental technologies;
- change in storage strategy;
- change in architectural principles;
- relevant security changes;
- changes to the deployment model.
Implementation changes do not require new ADRs.
# **7. Lifecycle**
Each ADR must have a status.
Suggested statuses:
Proposed.
Approved.
Superseded.
Obsolete.
Archived.
The history must be preserved for traceability purposes.
# **8. Numbering**
ADRs must use sequential numbering.
Examples:
ADR-001.
ADR-002.
ADR-003.
Numbering must never be reused.
# **9. Supersession**
When one ADR supersedes another, the relationship must be explicitly documented.
Example:
ADR-018
Supersedes:
ADR-009
Likewise, the superseded ADR must record that it was replaced by a later decision.
# **ADR-036**

**Title:** How the Control Plane learns that a Seed Node is reachable from outside (connectability)

**Status:** Approved — **retroactively recorded**

> ⚠️ **This ADR was written AFTER the implementation (#141), in violation of §11 of this document.** The record stands: the decision was made in code first, and the main consequence — the *sticky* semantics of the chosen signal — only surfaced during production verification, whereas it would have appeared in the "Alternatives considered" section had the ADR come first. The fix for this gap is tracked in issue #155.

**Context**
ADR-034 established that, under CGNAT, a Seed Node's inbound connectability depends on IPv6: IPv4 port-forwarding is ineffective and the operator must open inbound IPv6 on the BitTorrent port. What it **did not** define was how the Control Plane *knows* whether this opening is actually in effect.

The gap came at a high cost. Between 07/31 and 09/01, 2026, the provider rotated the delegated IPv6 prefix (`2001:db8:601:d300::/64` → `2001:db8:60b:c000::/64`). The gateway firewall rules used literal addresses and started allowing nonexistent addresses. Three Nodes went **weeks** without receiving a single connection — 553 torrents stalled — while the Portal showed them as ONLINE, green, and with high health. The only system that knew the truth was the tracker, outside the product.

**Problem**
Define (a) which signal the Control Plane uses to judge a Node's inbound reachability, (b) who produces that signal, in compliance with ADR-012 (the Agent initiates communication; the CP never connects to the Node), and (c) what information the operator receives to take action — knowing it is unreachable is not enough if the operator does not know **what to open**.

**Alternatives considered**

1. **Active probe from the Control Plane** — the CP opens a TCP connection to each Node's BT port. It provides a **definitive** and unambiguous verdict, which is exactly what is missing. Rejected because it directly contradicts ADR-012 (the CP would start connecting to the Node) and, worse, because it measures the wrong path: the CP is on the **same LAN**, and in IPv6 two hosts on the same `/64` reach each other via neighbor discovery, without going through the WAN firewall rule. The probe would return green precisely in the scenario we want to detect.

2. **External probe (third-party service or own VPS)** — measures what matters, from outside the edge. Rejected for V1: it introduces an external dependency in a product that runs entirely in a homelab, and exposes the fleet's addresses and ports to third parties. It remains the only way to get a true active verdict, and is recorded for future reevaluation.

3. **Query the tracker (UNIT3D)** — it already polls for announce and knows the state per torrent. Rejected as the primary mechanism: it couples platform health to a specific tracker, and the information arrives at the pace of someone else's announce. A Node with no published torrents would be left without a verdict.

4. **Self-reported signal by the Engine (adopted)** — the Agent reads the `connection_status` from the qBittorrent WebUI, which exposes libtorrent's `has_incoming_connections`, and sends it in the heartbeat along with the interface's global IPv6. It respects ADR-012 (Agent-initiated, ADR-014), depends on nothing external, and uses data the Node already has on hand.

**Adopted decision** (alternative 4)
- The **Agent** reports two fields in the heartbeat: `public_ipv6` (routable global address of the physical interface, discarding link-local, ULA, and virtual interfaces) and `engine_connectable` (`connected`/`firewalled`/`disconnected`).
- The **Control Plane** persists both in `nodes`, plus `connectable_since` — the instant the current verdict started (migration 00035).
- The **`NODE_FIREWALLED`** rule (WARNING) requires `firewalled` to be **sustained** for `alerting.firewalled_grace` (default 30m). Reason: the signal starts out false — every Node is `firewalled` from the moment the Engine comes up until the first peer knocks on the door. Without a grace period, the alert would trigger on every Engine restart, and an alert that cries wolf is an alert the operator learns to ignore.
- **`NULL` is unknown, never "closed".** An Agent that does not report the field does not generate an alert, and the `seedgrid_node_connectable` metric is **omitted** instead of emitting `0` — lack of data cannot turn into an accusation.
- The **Portal** displays the address to allow (`Inbound IPv6: [addr]:porta`), because an alert without this information would force the operator to log into the machine to take action.

**Consequences**

Benefits:
- the CP gains a notion of reachability without violating ADR-012 or depending on third parties;
- the operator discovers **what to allow** without accessing the Node, and address changes (prefix rotation) become visible;
- a new Node whose firewall rule never existed, or was created incorrectly, is detected — the session starts `firewalled` and never leaves it.

Limitations — **the main one invalidates part of the motivation**:
- ⚠️ **The signal is sticky.** `has_incoming_connections` turns `true` on the first accepted inbound connection and does not revert as long as the Engine does not restart. It answers *"has this session ever received anyone"*, not *"am I reachable now"*. Direct consequence: **the rule would not have caught the incident described in the Context**. Verified on 06/09/2026 — node-01 and node-03, with the Engine up since 31/07, reported `connected` having gone through the entire period they were unreachable. Closing the gap requires a signal that **decays** (e.g., inbound peer count, `I` flag), crossed with actual demand in the swarm, because a seed without a leecher legitimately has zero inbound connections. See **#155**.
- the verdict is per **Node** (Engine session), not per torrent as in the tracker;
- with privacy extensions enabled there is more than one global IPv6; the choice is ordered to be stable across heartbeats, and `bootstrap-node.sh` disables `use_tempaddr` so the announced address is the same one that is allowed.

**Relation to other ADRs**
- Complements **ADR-034**, which established the IPv6 dependency under CGNAT but did not define how to observe it.
- Respects **ADR-012** (Agent initiates communication) and **ADR-014** (polling): this is precisely why alternative 1 was rejected.
- Uses the tunables catalog from **ADR-023** for the grace period (`alerting.firewalled_grace`).
- The alert follows the model of **ADR-008** (deterministic rules evaluated by the Alert Worker, dedup by `code`+`node_id`).
# **ADR-037**

**Title:** Tracker profile as contributable data, not as code

**Status:** Approved — **partially implemented**: data model, taxonomy discovery, export/import, description composition, and image host (#190, #196, #197, #192, #193) delivered; versioned profiles in the repository and other child issues (#191) pending

**Context**
SeedGrid has been publishing to the UNIT3D tracker since ADR-020, with real POSTs validated in production against samaritano. What ADR-020 did not define was **where the site-specific knowledge lives** — and the result is that it is scattered across four places, none of which are data:

| knowledge | where it is today |
|---|---|
| taxonomy (category/type/resolution IDs) | `portal/src/pages/publish/TrackerUploadPanel.tsx:22` — hardcoded **in the frontend** |
| release name format | `control-plane/internal/release/standard.go` — the samaritano standard inside the generic package |
| API shape | `control-plane/internal/unit3d/unit3d.go` — fixed UNIT3D v8 |
| credential | `trackers` table (migration 00018): `name`, `announce`, `host`, `api_token`. No profile fields |

As long as the system has one operator and one tracker, this is a wart. The set/2026 survey (`docs/Landscape.md` §4, issue #185) changed the question: **the project will open-source the code**. In an open-source product, the trackers that matter are not the maintainer's — they are the users', on sites where the maintainer does not and will not have an account.

`autobrr/upbrr` solves the same problem with one Go package per site (40+ of them). It works because each one was written by a contributor **who had a login on that site**. This is the model described by alternative 2, and the reason it does not work here.

**Problem**
Define (a) where the site-specific knowledge for each tracker lives, (b) how this knowledge enters the system **without depending on the maintainer having an account on the site**, and (c) how it is shared across installations without carrying anyone's credentials along.

**Alternatives Considered**

1. **Keep as is** — taxonomy in the frontend, one tracker at a time. Rejected: tracker nº 2 is a new release, and another UNIT3D site with different IDs fails silently, because IDs are numbers and the POST accepts any of them.

2. **Go package per site (upbrr model)** — expressive, typed, testable, and their separation of concerns is good (`profile.go` / `taxonomy.go` / `rules.go` / `validation.go` / `description.go`). Rejected for three reasons, in order of weight: **(i)** only what the maintainer can write makes it into the product, and they don't have an account on most sites — the catalog ceiling becomes one person's login list; **(ii)** contributing requires writing Go and opening a PR, and each new site requires a release; **(iii)** deriving the tables from their source is a derived work discussion in an open source context (upbrr is GPL-2.0-or-later, UNIT3D is AGPL-3.0).

3. **Delegate publishing to an external tool** (upbrr as a dependency) — would bring in 100+ trackers without maintaining any. Rejected: external dependency in alpha; the release would no longer be born seeded in the cluster, which is what ADR-019 built; and publishing would cease to be an orchestration step and become an adjacent step — losing precisely the integration that differentiates the product (`docs/Landscape.md` §4).

4. **Profile as data, in three layers (adopted)** — site knowledge becomes an exportable record, produced by whoever has the account, and the code only stores what is genuinely common across the entire family.

**Adopted Decision** (alternative 4)

Tracker knowledge is divided into three layers, with an explicit boundary:

| layer | where it lives | who produces it | shareable |
|---|---|---|---|
| **family** — API shape, auth, multipart | Go code (`internal/unit3d`) | maintainer | n/a: it is identical across every site in the family |
| **site profile** — taxonomy, name format, rules, image host policy | **data**: its own table, exportable in JSON | any user with an account on the site | **yes — that's the point** |
| **operator config** — `api_token`, announce with passkey, `anon`, `modq` | **data**: never exported | the operator | **never** |

- **`family` in the profile since the first version.** V1 only accepts `unit3d`; the field exists so that a new family (Gazelle) doesn't require reshaping the record. Cheap now, expensive later.

- **Automatic discovery as profile bootstrap.** The UNIT3D `TorrentResource` returns, in the same item, the name **and** the id of each axis (`category`/`category_id`, `type`/`type_id`, `resolution`/`resolution_id`). Crawling a few pages of the catalog **from the tracker itself**, with the operator's own token, builds the ID table without logging into a third party and without copying anyone else's table. `unit3d.Browse` already consumes this endpoint (`GET /api/torrents/filter`) and currently discards the `*_id`s.

- **Rules are optional and start empty.** Publishing does not depend on them; they prevent errors, they don't enable uploading. Evidence that this is feasible in practice: the samaritano profile in upbrr **has no `rules.go`** — no rules, and it publishes. Strict sites (Aither, Blutopia) have them; ours doesn't need them to work.

- **Profile export/import in JSON.** Layer 3 is **out by design** — separate table and structure, not a filter in serialization. A profile that leaks `api_token` due to a forgotten filter is the obvious failure mode, and it is eliminated by the format, not by the discipline of the exporter.

- **Distribution: versioned profiles in the repository, contributed via PR.** Site profiles live as JSON files in the repo (`profiles/<family>/<site>.json`), and contributing a tracker means opening a PR with a file — without writing Go, without the maintainer having an account on the site, without waiting for a release for those who clone. It is the definition model of Prowlarr/Jackett, and it closes the loop opened by layer 2: discover (bootstrap) → adjust → export → PR.

- **Two sources, explicit precedence:** the repo **seeds** the available profiles; the database stores the profile in use and the operator's local edits. The database rules — the same precedence as ADR-023. Updating SeedGrid brings new profiles without overwriting what the operator has adjusted.

- **Import treats the profile as untrusted content:** type and range validation on input, nothing executable, no free-form field going straight to the request. This applies to manual import **and** to repo profiles — a profile arrives via a third-party PR, and PR review does not replace runtime validation.

- **The taxonomy leaves the Portal.** `UNIT3D_CATEGORIES` ceases to exist in `TrackerUploadPanel.tsx`; the panel now renders what the profile declares. `standard.go` becomes the **UNIT3D family default**, and the site-specific format now comes from the profile.

- **Migration with no effort for the operator:** the existing samaritano tracker becomes the first profile, seeded from what is currently hardcoded, without reconfiguring anything.

**Consequences**

Benefits:
- a new tracker is a **new line, not a new release** — and doesn't go through the maintainer;
- contributing a tracker doesn't require writing Go, having an account known to anyone, or waiting for a release: just export the JSON;
- the credential/profile separation makes the contribution **secure by design**, which is a prerequisite for it to happen;
- the need to derive tables from GPL/AGPL projects disappears — profiles are born from the user's own site;
- unlocks, without a new decision, issues #191 (rules), #192 (description per tracker), and #193 (image host per tracker), which currently have nowhere to hook into.

Limitations:
- ⚠️ **Discovery only sees what is in the catalog.** A category that no one has ever posted in does not appear. Mitigation: incremental discovery (run the full one again) and manual editing of what is missing. In practice, what is in the catalog is what the operator will use — but a newly discovered profile **is not complete by definition**, and the UI cannot suggest that it is.
- **Rules and name format are not discovered.** They come from the wiki and observation, meaning: they depend on whoever holds the account. A contributed profile without rules publishes just the same — it just doesn't protect against errors.
- **Wrong profile publishes wrong**, and now the error can come from outside. The counterweight is the frozen preview (#188): the operator sees the exact payload before the POST, which the UNIT3D API does not allow testing any other way (there is no dry-run).
- **New family still requires code.** Gazelle, standalone, cookie and TOTP auth remain out of scope for this ADR; the `family` field only ensures they will fit later.
- One more surface of user-editable data — validation on import becomes a requirement, not just diligence.

**Implementation status**

Delivered in #190 + #196 (migration 00043):
- **Data model.** `tracker_profiles` table, **separate** from `trackers` — layer 3 is kept out by design, because the profile table has no credential column (and two tests guard this: one on the schema and type tree, another end-to-end with a tracker that has a passkey and token). Columns: `tracker_id` (one live profile per tracker), `family`, `schema_version`, and `taxonomy` in **JSONB** — chosen over one row per term because the unit of export, import, and validation is the document, because new axes and sections (rules, image host, name format) come in as columns or keys without remodeling the table, and because the taxonomy is never queried by content. The integrity that the relational model would provide for free (unique id per axis, ranges) is kept in `internal/trackerprofile`, at every write point, with the write serialized by `FOR UPDATE`.
- **Origin per term** (`seed` | `discovered` | `manual`) decides who can change the name: discovery **never overwrites `manual`** and never removes anything. The rule runs on the server, on the locked state, and not in the Portal.
- **`kind` in category** (`movie`/`tv`/`game`): the publish panel used to deduce "series" and "game" from the category number (2/3 and 5) — another form of taxonomy hardcoded in the frontend, and only true for one site. It became profile data.
- **Samaritano migration with no work for the operator:** the migration seeds the already registered tracker with the taxonomy that was in the Portal (same ids, labels, and order). **Only Samaritano** — seeding the same list on another UNIT3D site would perpetuate the error this ADR exists to eliminate. *Revised in #63: the seed was removed so the product carries no site's data; the same taxonomy ships as `profiles/unit3d/samaritano-cc.json`, imported like any other profile.*
- **Discovery** (`POST /trackers/{id}/discover-taxonomy`) only proposes; `apply-taxonomy` applies, and it is the audited action (ADR-025). The Portal no longer has `UNIT3D_CATEGORIES` and reads the options from the profile; the pre-filling from the parser now matches by **label** and by **role**, because the parser returns Samaritano's IDs.

Measured during implementation (Samaritano, 03/10/2026, 3 pages = 150 torrents): the taxonomy that was in the Portal **was already incomplete and diverged from the site**. The catalog shows the types `PC` (50) and `Windows` (71), which the panel never offered (games and programs are posted with them), and different names — `Series` and `Other` where the Portal said `Séries` and `Outra`. It is the concrete evidence of the limitation already recorded above, seen from the other side: **the catalog reveals what manual transcription missed, but only what was posted**.

Delivered in #197 (**no migration**: the new origin lives inside the JSONB, and the file is a read of what already exists):
- **The exchange format (`schema_version` 1).** A JSON file — the identification envelope plus the profile itself. This is the file the code generates (nothing manual):

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

  `format` (`seedgrid.tracker-profile`) identifies the file: without it, any JSON with the same keys would be accepted. `site` is **just identification** — the name the operator gave to the tracker and the public host (no port, path, or credential) — and **is not applied to anything**: it serves for the operator to know what they are importing and for the import to warn when the file is from another site (the ids are from the site). `family`, `schema_version`, and `taxonomy` are the profile. Keys in lowercase ASCII; axes in alphabetical order (that's how `encoding/json` sorts a map) and terms by `id`, so the same profile always generates the same bytes — which makes the file versioned in the repository reviewable by diff.
- **Layer 3 is kept out by design, and the export has no field list.** The file is the serialization of the `Document` type, which **embeds** the `Profile` instead of listing its fields. Two consequences, both deliberate: (i) the exported type has no field capable of carrying a credential — there is no serialization filter for someone to forget, just a type with no place for the secret; (ii) a new profile section (rules, description, image host) **is exported without touching the export**, and no one needs to remember to export it. What the export reads from `trackers` is the name and the host — never announce, `api_url`, token, or `source` — and the host is reduced to the DNS name: a value that fails validation comes out empty, not dirty. Two tests guard this, and they both look at different things. The **structural** one walks the `Document` type tree by reflection and fails fields with credential names, **opaque** fields (`interface{}`, bytes, `json.RawMessage` — they would carry anything without a name), and types with their own `MarshalJSON` (it would emit whatever it wanted without the reflection seeing it); there is also a test **of the test**, which proves that the checker fails each of these ways to leak. The **textual** one creates a tracker with `api_token`, announce with passkey, `api_url`, `source`, and the entire profile filled out, exports it, and looks for the **values** in the file — the field name says what the code intended, the value says what came out —, as well as checking that only the keys from the list appear, at any level.
- **`origin` does not travel.** The file does not carry the origin of the terms (`seed`/`discovered`/`manual`), the export zeroes it out, and the import **rejects** a file that brings it. Three reasons, in order of weight: **(i)** `origin` answers "who can change this name *here*" — it is the term's relationship to **this** operator; one user's `manual` is third-party content for another; **(ii)** the file is untrusted, and letting a file declare `origin: manual` would be letting its author decide what the operator's discovery can or cannot correct, or `origin: seed` to pass itself off as the migration; **(iii)** the precedence already exists on the importing side. Therefore, the import assigns **its own** origin, `imported`: a fourth value, with the same effect as `seed`/`discovered` on the merge (it was not typed by this operator, so discovery can propose the name their site actually uses) but distinct, because the operator needs to know that that id **came from outside and their site has not yet confirmed it** — the counterweight that the limitation "wrong profile publishes wrong, and now the error can come from outside" demands. It requires no migration: `origin` lives in the JSONB, without `CHECK`.
- **Import: `ParseDocument` is the only gateway.** Untrusted content, in this order: size (512 KiB; `413`), UTF-8, **shape** (keys only `[a-z0-9_]`, no repeated keys, at most 8 levels, only one value at the top), types (`DisallowUnknownFields`: unknown field is an error), and content — `format`, `schema_version`, `family`, `site`, the axes (closed set), `id` in `1..1000000` and unique per axis, name with 1 to 64 characters without control characters, without invisible formatting, and without the runes that **look like spaces without being spaces** (the Hangul fillers U+115F, U+1160, U+3164, and U+FFA0, the empty Braille U+2800, the Khmer inherent vowels, the grapheme joiner — a name made only of them looks empty on the screen and passes as "1 to 64 characters"), `kind` only in `category`, at most 256 terms per axis. The shape rules exist because `encoding/json` matches keys to fields **ignoring case and even Unicode folds** (a key with the Kelvin sign, U+212A, matches `kind`) and silently keeps the **last** of two repeated keys: a file reviewed in a PR by whoever read the first one would be imported by the second one. Error messages never return the received value (it could be megabytes). Nothing from the file turns into a request to the tracker: the name is a label, and the ids only travel in the publish POST — where the frozen preview (#188) shows the exact payload.
- **An unhandled section is rejected (fail closed).** Exporting a new section is free; **importing it** requires validation and a merge rule. The import only applies the sections registered in `handledSections`; a `Profile` section that is not there and comes **filled** in a file is rejected at runtime, and a test fails the suite when someone adds a section to the `Profile` without handling it — with a message on what to do. It is the assumed price of a new section not being able to be accepted without checking.
- **`schema_version`.** It goes up when the format changes in a way that an old import wouldn't understand: a field that changes shape **or a new section** — except for the **optional and additive** section (the `description`, #192: see below). The export stamps the version the **code** emits (not the one in the database row); the import accepts from `MinSchemaVersion` up to the current one and rejects the rest with `unsupported_schema_version`, saying which version this SeedGrid understands. It is what turns "new profile on an old SeedGrid" into a clear message instead of "unknown field". Bumping `SchemaVersion` without lowering `MinSchemaVersion` forces you to write the upgrade from the previous version; today there is only one version, and therefore no upgrade.
- **Preview and apply are two routes** (`…/profile/import/preview` and `…/profile/import`), not one route with `dry_run`. Three reasons: **(i)** the audit trail (ADR-025) decides by method and path — discovery is already its own route for the same reason —, so a read route stays out of it without patching, and a single route with a flag would audit the preview as if it were an import; **(ii)** with a flag, "I forgot the flag" **saves**; with two routes, the one that is easy to call accidentally never saves; **(iii)** the Portal already has two steps (file → preview → confirm), and each one is a call. Both receive **the same body: the file, byte by byte** (`curl --data-binary @perfil.json`). The preview returns the plan — term by term `new`, `same`, `renamed`, or `protected`, plus counts, warnings (including the one for a file from **another site**, which **also comes out when comparison is not possible** — file without host or tracker without valid host —, so a file doesn't turn off the check just by omitting the field) and what the file doesn't bring (`local_only`) — and the warning that the content came from outside; **it does not save nor create the profile row**. A token tying the preview to the apply was not adopted (the pattern from #188): there the payload goes to a third party and cannot be undone, and the token prevents posting what no one checked; here the guarantee that matters — not overwriting `manual` — runs **on the server, on the locked row**, not on what the screen saw, and the apply response brings the plan as it was **executed**. A token would only add a `409` for a risk the rule already neutralizes.
- **The coexistence rule is the discovery merge rule (#219), extended — and stricter where discovery is already opt-in.** Missing id → **adds** (`imported`); same name → nothing; different name → **renames only what came from the migration (`seed`) or a previous import (`imported`)**. What the operator typed (`manual`) **and what their site confirmed in discovery (`discovered`)** is `protected`: the operator's site name outweighs a third-party file's, and discovery already treats rename as opt-in — the import cannot be more permissive than it. **Fail closed:** an origin the code doesn't know (row written by another version, or manually in the database) is also protected, in import and discovery. What the file doesn't bring remains **untouched** — the import never removes, because absence in the file is not evidence (the file could be from an incomplete profile, like the discovered one). "Unless they ask" has a shape: the operator asks **term by term** (`?overwrite=category:2`, repeatable) and it only applies to a protected term the file brings; a request with no match becomes a warning, never an effect. `kind` only **fills** an empty role and never swaps an existing one — discovery doesn't create `kind`, so filling is the most common gain from importing over a discovered profile — and filling only the `kind` doesn't swap the origin (the same rule as manual editing). A `protected` term doesn't even have its role touched.
- **A new term cannot shadow the right one.** The publish panel matches the release name with the term's (`findByLabel`) and pre-fills the first category of each role. A hostile — or just careless — file that brings `{id: 1, "Séries", tv}` in a profile that already has Séries = 2 would change what the panel picks, without the operator having decided anything. Three defenses: **(i)** a file with two names that only differ in case or accent **within the same axis** is rejected (`taxonomy.category[1].name: nome repetido no eixo…`), with the same normalization as the Portal (`NormalizeLabel` ↔ `normalizeLabel`: NFD without accent marks, lowercase, collapsed spaces — there is a parity test); **(ii)** the preview warns, **on each line**, the new or renamed term whose name collides with another id's in the axis — calculated on the state the axis WOULD BE in, not the current one — and the term that would become the **first** category of a role that already had an owner (multiple terms with the same role are normal — series and anime are `tv` —; only changing the default's owner warns); **(iii)** in the Portal, `findByLabel` only returns a term when the label is **unambiguous** — with two ids for the same name, it doesn't pre-fill and the operator chooses. Since the export goes through the same validation, a local profile with repeated names **is not exported** (`409 profile_not_exportable`, with the reason): the export never delivers a file that the import itself would reject.
- **The site name in the file is the operator's free-form label — and it may contain a credential.** Whoever pastes the announce (or just the passkey) into the tracker name would take the passkey into a file made to be shared. `siteLabel` **does not clean or truncate** (truncating a 90-character announce would leave the first ~27, passkey included): a name that fails `CleanName`, or that **looks like it contains a credential** — slash, `\`, `?`, or `=`, a UUID, or 20 or more consecutive letters and digits —, is replaced by the **public host** (if it fits in 64 characters) or by `tracker`. The 20 limit is a heuristic, and the decision is the operator's: a smaller fragment pasted into the name passes, which is why the Portal says what the file carries and asks to check before sharing.
- **Audit (ADR-025):** only the applied import is logged («Imported a profile from file on tracker «Y»: N new, M renamed, K role(s) filled; P kept for being the operator's or already confirmed by their site»). **Nothing that came from the file enters the trail**: their site name is third-party text and, with the «» quotes, would forge the rest of the line. The tracker name goes through the same safe label and loses the quotes. Preview and export are reads and stay out.
- **RBAC in the standard for tracker endpoints:** exporting is `trackers:read` (the content is already in the tracker listing, which the same role reads); previewing and importing are `trackers:write`. Tracker belongs to whoever registered it: another operator's doesn't exist for whoever asks (`404`).

Delivered in #192 (**no migration**, **no new column**): the upload **description composition** moves out of the Portal and becomes a **section of the tracker profile**.
- **What moved.** `portal/src/pages/publish/bbcode.ts` assembled the BBCode (poster, synopsis, tags, screenshots, and MediaInfo; cover, title, about, genres, and platforms) — out of reach of any API client, with nowhere for two sites to diverge and no Go tests, being the field the site publishes in full. Now it is `internal/description` (`ComposeMedia`/`ComposeGame`), called via `POST /api/v1/publish/description` (`publish:write`, outside of audit: it only composes text). The Portal requests the text and hands it to the field, which the operator continues to edit — the human review from ADR-020 remains unchanged — and `buildBBCode`/`buildGameBBCode` were removed.
- **What the profile decides — and only that.** The `description` section is `{media_blocks, game_blocks, media_info_tag}`: **which blocks are included and in what order** (omitting from the list disables it; `[]` means none) and the **tag wrapping the MediaInfo** (`mediainfo`, `code`, `pre`, or `spoiler`), which is what varies from site to site. The block IDs are a **closed** set and the tag is a closed list: **there is no free-form template**. This alternative was deliberately rejected — a profile is third-party content, the tag is interpolated into BBCode that the site publishes, and a text field that arrives in a file and turns into markup is exactly the "free-form field going straight into the request" that this ADR forbids. Anyone who needs a block that doesn't exist opens a PR with the block, not with a field. *Since #74 the section also has `language` (`en` | `pt-BR`, empty = `en`): the content language of the site — the block headings and the language TMDB answers in (synopsis and genres, at publishing and on the Explore screen). A closed list, like the tag.*
- **Measured parity, not asserted.** With the default config, the output is **byte-for-byte** identical to the TypeScript one. The test strings were calculated by **running** the original TS code on 44 inputs, not written by hand; the JS `trim()` and Go `strings.TrimSpace` diverge (BOM and NEL), so the CP has a `jsTrim` with the JS character set.
- **Where it lives.** In the same JSONB column (`tracker_profiles.taxonomy`), as the reserved `description` key **alongside the axes**. The document every row already has is, byte-for-byte, a new document without the section — hence no migration or rewrite. The choice was between this and an envelope (`{"taxonomy": …, "description": …}`), which would require migrating all rows or guessing the format of each one on read; the reserved key has no ambiguity (an axis is never named `description`, the set of axes is closed). **Strict reading:** a document with a `description` of the wrong type is an error, never an "empty profile" — the next edit would overwrite it.
- **Editing via dedicated route.** `PUT`/`DELETE /trackers/{id}/profile/description` (`trackers:write`), and not an optional field in `PUT …/profile`: the profile `PUT` replaces submitted axes, and an absent field deleting the section would be exactly the oversight the "absent is untouched" contract exists to prevent. `Edit`, `apply-taxonomy`, and the import pass the section along as they read it. Validation runs **in the service**, at every write point — not just in the handler.
- **Export/import.** The `Document` embeds the `Profile`, so the section is **exported without touching the export**; the import needed validation (the same as manual editing, in `ParseDocument`) and a merge rule — `description` was added to `handledSections`, which is what the fail-closed test enforces. The section is **a single unit** with no per-term origin, and the rule is the `manual` one applied to it entirely: profile without its own composition (or with the default) → applies; identical → nothing; **different → protected**, and only `?overwrite=description` replaces it. A file that doesn't bring it leaves it untouched.
- **`schema_version` did not change.** The section is optional and additive, and the export omits it by default, so a profile without it remains exactly the v1 file; bumping the version would invalidate every already shared file for an addition that almost no profile uses. **This opens an exception to the "a new section bumps the version" rule**, written above for #197 — it applies to **optional and additive** sections; a section that changes the meaning of what already exists still bumps it. The assumed cost: a SeedGrid prior to #192 rejects a file with `description` due to `campo desconhecido`, not `unsupported_schema_version`.
- **Limitation: downgrading.** A Control Plane prior to #192 reads the entire column as taxonomy and **does not read** the profile of a tracker whose composition was customized (the `description` object is not a list of terms). This is the trade-off for having no migration; mitigation: `DELETE …/profile/description` (or delete the key) before downgrading. Those who never customized are not affected.

Delivered in #193 (migration `00045`): the **tracker image host** moves from being global and single to being profile data, with a fallback to imgbb. The `image_host` section is optional and additive, in a closed format (multipart POST with a file part, key in a custom header, response URL via dot-notation path over JSON). The operator's key is layer 3 — it comes in via a dedicated route and is tied to the current `upload_url` host. The upload chain has a time budget, and every failure becomes an entry in the Feed (kind `screenshot_host_failed`). The section travels in the profile export/import with the same single-unit section rule as `description`; `schema_version` does not bump. Details in ADR-040.

Pending: the versioned profiles in the repository — the **format and validator are ready** (the file at `profiles/<family>/<site>.json` is exactly this file, and a CI test that passes each of them through `ParseDocument` is what's missing, along with the seeding and the directory) —, `standard.go` as the family default with the name format coming from the profile, and rules (#191).

**Relation to other ADRs**
- **Partially replaces ADR-020**, which established the taxonomy and naming format for samaritano as if they were from the product. The contract with the UNIT3D v8 API remains fully valid.
- **Affects ADR-030** (single screenshot host): the image host policy becomes a profile field (#193).
- **Preserves ADR-019** — content continues to be created, seeded, and published in the same flow, with the Scheduler choosing the Node. This was the main reason for rejecting alternative 3.
- **Follows ADR-029** in the separation between secret and configuration: tracker credentials never appear in the API or export.

# **ADR-038**

**Title:** Stable and predictable IPv6 address per Seed Node — deterministic suffix `::5eed:<N>`

**Status:** Approved — implementation in #202

**Context**
ADR-034 established that a Seed Node behind CGNAT is reached via IPv6, and that the operator allows IPv6 inbound traffic on the Node's BitTorrent port in their firewall. This rule points to an **address**. By default, the SLAAC address is **EUI-64**, derived from the interface's MAC, which creates three problems:

1. **The address does not survive Node recreation.** Recreating the system container, swapping the network card, or migrating the Node to a different host changes the MAC and, with it, the address — and the firewall rule silently ends up pointing to nowhere.
2. **The address is unpredictable before the Node exists.** Zero-touch onboarding (ADR-017) cannot prepare the allow rule in advance: the operator only discovers the address after the Node comes up, and between coming up and being allowed, the Node remains `firewalled`.
3. **There is no way to change the address when something upstream poisons it.** This has happened: an edge device started dropping exactly the EUI-64 address of a Node, while another global address on the **same** interface, in the same `/64`, reached the internet normally. The workaround was to change the Node's suffix — but figuring out the mechanism on the fly, with the Node unreachable.

**Problem**
Define how the Node chooses the lower half (64 bits) of its IPv6 address so that it is **stable** (independent of MAC, NIC, and host), **predictable before the Node exists**, **unique** in the Cluster, and **continues to track the provider's prefix rotation** — which is the upper half and controlled by the provider.

**Considered alternatives**

1. **Keep EUI-64 (status quo).** Rejected due to the three problems in the Context. It works as long as nothing changes, and it is precisely when something changes that the rule stops working.

2. **Fully static address** (`Address=`, `ip addr add`). Stable and predictable, but **freezes the prefix**: when the provider rotates the `/64`, the Node keeps announcing an address that no longer exists, and the failure is silent — the scenario that IPv6 change detection (ADR-036) exists to catch. Rejected.

3. **Stable privacy address (RFC 7217, `stable-privacy`).** Stable per network and tracks the prefix, but is **opaque** by design: it cannot be calculated before the Node exists, which is requirement 2. Rejected.

4. **DHCPv6 reservation.** Would solve all three, but depends on the operator's router offering stateful DHCPv6 — which the typical residential connection behind CGNAT (SLAAC only) does not offer. Rejected for requiring infrastructure the product cannot assume.

5. **Interface token with deterministic suffix (adopted).** The Node keeps the prefix from the Router Advertisement and fixes the lower half. It is supported by systemd-networkd (`IPv6Token=`) and NetworkManager (`ipv6.token`), and does not touch the part of the address the provider controls.

Within alternative 5, **where the Node number (`N`) comes from**:

- *The operator types it in.* Prone to error and collision; contradicts the "one command" approach of ADR-017. Rejected.
- *Hash of the hostname or MAC.* Can collide, and the MAC is exactly what we want to stop using. Rejected.
- *A new counter in the Control Plane (column in `nodes`).* Solves it, but introduces **new state** with migration, backfill, and a second sequence to keep consistent with the first, given that the Control Plane **already has** a unique resource per Node, assigned during enrollment and returned at bootstrap: the BitTorrent port (ADR-017). Rejected.
- *Derive from the BitTorrent port (adopted).* The port is already unique among active Nodes and live reservations, comes from the same enrollment response, and returns to the pool when the Node is removed. Reusing this guarantee gives index uniqueness for free.

**Adopted decision** (alternative 5)

- **Convention.** The Node's address is `<prefixo /64 anunciado no RA>::5eed:<N>`. The `5eed` suffix is visually recognizable in `ip -6 addr` and does not collide with usual static addresses (`::1`, `::100`) nor with an EUI-64 (which contains `ff:fe`). `N` is written **in decimal digits** in the last group of the address — Node 12 is `::5eed:12` — so that the operator reads the Node number directly from the address without converting to hexadecimal. It fits in four digits (`N` ≤ 9999); the Control Plane port sequence has 1000 positions.
- **The Node's index is its position in the BitTorrent port sequence**: `N = porta_bt − 6880`. The Control Plane returns it during enrollment as `node_index`, along with `bt_port` — same response, same reservation, same uniqueness. **There is no migration:** no new state is stored, and the `node.IndexForPort` function is the only place that knows the rule.
- **Existing Nodes do not change address.** Those that have already applied the convention manually — `::5eed:1` to `::5eed:4`, on Nodes with ports 6881 to 6884 — stay at exactly the same address: the rule above reproduces these numbers. No backfill, no firewall rule changes.
- **Mechanism, by network manager** (`bootstrap-node.sh`, after enrollment and before bringing up the Engine):
  - **systemd-networkd** — drop-in `IPv6Token=static:::5eed:<N>` (alongside the `IPv6AcceptRA=true` already required by ADR-034);
  - **NetworkManager** — the connection's keyfile gets `addr-gen-mode=eui64` and `token=::5eed:<N>`, with backup and rollback if the application fails (see `docs/19 - Deployment.md` §50.2 for the pitfalls, which are not obvious);
  - **other** — the script does not guess: it prints instructions for the operator's manager, which also allows those who prefer to configure the network manually to do so with the correct address.
- **Existing is not enough: the address must renew.** The acceptance criterion for any procedure that applies the token is to observe the address's `valid_lft` **increase** after a Router Advertisement. An address that exists but does not renew disappears on its own when the lifetime expires; `autoconf=0` produces exactly this state (the address stays up with the lifetime it already had and expires in about two hours), and therefore **is not** the way to remove the leftover EUI-64 address — that residue is deleted directly, with `autoconf` preserved.
- **Opt-out.** `IPV6_SUFFIX=off` in the bootstrap keeps the address chosen by the network manager. It is the fallback for those using delegated addressing, stateful DHCPv6, or a custom policy.
- **It does not replace IPv6 change detection (ADR-036).** The token only fixes the **suffix**; the prefix still comes from the provider, and a `/64` rotation still changes the address of all Nodes and still invalidates the rules. `NODE_IPV6_CHANGED` continues to cover this case. What changes is the **cost of the fix**: only **one** unknown remains (the new prefix) and the rest is arithmetic — `<prefixo>::5eed:<N>`.

**Consequences**

Benefits:
- inbound rules **survive recreating the Node**, replacing the board, or migrating hosts: the address no longer depends on the MAC;
- the operator can **prepare the release before the Node exists** — the next Node receives the lowest free port in the sequence (the Portal's Node list shows the ports in use), and the address derives from it;
- removing a Node returns the port and the index to the pool; the Node that replaces it inherits the **same address**, and the rules remain valid without editing (this is intentional, and it is the reason the index is derived from the port);
- an address poisoned upstream has a known remedy: changing the suffix is a procedure, not an investigation;
- the `public_ipv6` that the Agent reports (ADR-036) becomes predictable, and prefix rotation becomes easier to handle — the Control Plane may, in the future, calculate new addresses instead of only alerting.

Limitations:
- **The prefix remains the provider's.** This ADR does not solve prefix rotation; it only makes the fix cheaper.
- **A predictable address is an address easy to target.** Anyone who knows the prefix can guess `::5eed:1`, `::5eed:2`… Security already depended entirely on the router firewall (ADR-034: there is no NAT v6), and only the Node's BitTorrent port is opened; the gain from enumerating Nodes is small, but it exists, and whoever does not accept the cost uses the opt-out.
- **The index is derived from the port.** A Node registered by the shared token, with a port outside the Control Plane's sequence (manual installation), has no index — the bootstrap does not apply the token to it, and the operator uses the opt-out or the address they prefer. Changing the port assignment rule changes the address assignment rule: the two go together on purpose.
- **The NetworkManager path is the most fragile.** The keyfile is written by hand because the command-line interface cannot do it (two validations that exclude each other), and it depends on the keyfile being the connection write backend; on a system where it is generated (for example by netplan), the script falls back to manual instructions.

**Relationship with other ADRs**
- **Complements ADR-034**: it established that the operator opens the Node's IPv6 address in the firewall; this one makes it stable and predictable. Its addendum (privacy extensions disabled, [#142]) is a prerequisite: the token fixes the stable SLAAC address, but a temporary address still needs to be disabled for outgoing traffic to use it.
- **Extends ADR-017**: the BitTorrent port assigned in the enroll now also becomes the source of the Node index (`node_index` in the enroll response).
- **Preserves ADR-036**: `public_ipv6` and `NODE_IPV6_CHANGED` remain as they are.
- **Respects ADR-012**: the Control Plane only returns a number; the one that configures the network is the Node, which pulled the enroll.

# **ADR-039**

**Title:** Agent and Engine behavior configuration comes from the Control Plane — cluster value with override per Node

**Status:** Approved — implementation in [#144]

**Context**
The Agent and Engine behavior adjustments (`MAX_ACTIVE_DOWNLOADS`, `HEARTBEAT_INTERVAL`, `COMMAND_POLL_INTERVAL`, `LOG_LEVEL`, and the Engine queue) were **environment variables per machine**, read once at Agent boot (`agent/internal/config/config.go`). The `bootstrap-node.sh` writes only the identity keys; the rest falls into the code default. Consequence: **every new Node is born different from the others and no one notices**. node-04 came in with `MAX_ACTIVE_DOWNLOADS=2` against `10` on the other three, and without `HEARTBEAT_INTERVAL`/`COMMAND_POLL_INTERVAL`/`LOG_LEVEL` in the `.env`.

These values are **cluster** behavior, not machine behavior — and ADR-023 already created the catalog of tunables with hot-reload exactly for this. But the catalog was the Control Plane's: the Agent did not read it.

But the fleet is **truly heterogeneous**. Field measurement (06/09/2026, issue [#144]): with the same ~13 MB/s download, qBittorrent consumes 0,5–2,9% CPU on the three Nodes with Core i5 and **50,7% on node-04** (Atom x5-Z8350 @ 1,44 GHz — the cost of piece hashing), with 809 MB in swap. On 05/09 someone applied `MAX_ACTIVE_DOWNLOADS=10` on node-04 "to align with the others" — exactly what a naive automation would do — and it was necessary to revert to 3. A single cluster value **reproduces the error at scale**.

**Problem**
Make the Agent/Engine behavior be decided in **a single place** (the Control Plane, ADR-001), editable by the Portal and visible per Node, **without** erasing the legitimate difference between machines, and **without** taking down or overloading the Nodes that are already in production on the migration day.

**Considered alternatives**

1. **Continue with env per machine and only automate the bootstrap** (write the same values on every new Node). Rejected: it remains configuration spread across N files that no one audits, and the value the bootstrap would write would be the same node-04 error.
2. **A single cluster value, without exception.** Rejected by the measurement above: the Atom and the i5 cannot handle the same number of downloads.
3. **Derive the ceiling from the capacity the Node reports** (`cpu_cores`, `mem_total_bytes`, `disk_io_util_pct`). Attractive, but it is an opaque heuristic: the operator does not know why the Node received 3 and not 4, and the right number depends on things the heartbeat does not see (the Atom on node-04 also runs other services). It remains as a future evolution — and the override per Node, which this ADR creates, is the mechanism it would rely on.
4. **Cluster value (catalog, ADR-023) with override per Node (adopted).** The catalog remains the single source of metadata and provides the cluster value; a new table stores the deliberate deviations for each Node.

Within alternative 4, **the delivery channel to the Agent**:

- *Dedicated endpoint `GET /api/v1/agent/config`, polled in its own loop.* It works, but it is one extra request per cycle, one extra route, and one extra loop in the Agent to deliver something that changes once a month.
- *`apply_config` command on the queue (ADR-014).* The queue is for **actions** (pause, migrate, update), not for state: a lost command, discarded by retention pruning, or never queued (existing Node) would leave the Node with an old value **without anyone knowing**. Config is state that needs to converge, not an event that can be lost.
- *Heartbeat response (adopted).* The Agent already calls the Control Plane every cycle, the channel is already authenticated and Agent-initiated (ADR-012), and the Node receives the **full desired state on every heartbeat** — if one is missed, the next one fixes it. Zero new requests. The cost is coupling config to the response of a liveness endpoint; it remains contained because the resolution is read-only and **never drops the heartbeat** (if it fails, the response is sent without the config block and the Agent keeps what it has).

**Adopted decision** (alternative 4, delivery via heartbeat response)

**1. Precedence** (from strongest to weakest):

> **Node override > cluster value (catalog, ADR-023) > Node env > code default**

The Node env ceases to be a source and becomes a **fallback**: it is valid only when the Control Plane sends nothing (CP prior to this ADR, unreachable CP, or a newly started Agent before the first heartbeat). The code default is valid when even the env does not exist.

**2. Key classification.** The criterion: does the value answer *"how THIS machine connects to the rest"* (identity/machine → stays in `.env`) or *"how the cluster wants the Agent/Engine to behave"* (behavior → catalog)? A behavior key **can** have a different value per Node — that is what the override is for — which does not make it a machine key.

| Environment variable | Class | Where it lives |
|---|---|---|
| `SEEDGRID_CONTROL_PLANE_URL` | identity (how to talk to the CP — cannot come from the CP) | `.env` |
| `SEEDGRID_AGENT_REGISTRATION_TOKEN` | identity / secret | `.env` |
| `SEEDGRID_AGENT_DATA_DIR` | machine | `.env` |
| `SEEDGRID_AGENT_ENGINE` | machine (which Engine adapter the Node runs) | `.env` |
| `SEEDGRID_ENGINE_QBT_URL`, `_USERNAME`, `_PASSWORD` | machine / secret (Engine credential) | `.env` |
| `SEEDGRID_AGENT_STORAGE_PATH` | machine | `.env` |
| `SEEDGRID_ENGINE_BT_PORT` | identity (assigned by the CP on enroll, ADR-017; matches the operator's port forward) | `.env` |
| `SEEDGRID_AGENT_MIGRATION_PORT` | machine / network | `.env` |
| `SEEDGRID_ENGINE_CONTAINER` | machine (Engine container name; contract with compose, #146) | `.env` |
| `SEEDGRID_DATA_PROPAGATION` | machine (mount propagation in compose, #143) | `deploy/agent/.env` |
| `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` (#221) | machine (operator declaration: does the Node have a dedicated disk at `/data`?) | `.env` |
| `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | **behavior** | catalog `agent.heartbeat_interval` |
| `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | **behavior** | catalog `agent.command_poll_interval` |
| `SEEDGRID_LOG_LEVEL` | **behavior** | catalog `agent.log_level` |
| `SEEDGRID_ENGINE_QUEUEING_ENABLED` | **behavior** | catalog `engine.queueing_enabled` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | **behavior** | catalog `engine.max_active_downloads` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS` | **behavior** | catalog `engine.max_active_uploads` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS` | **behavior** | catalog `engine.max_active_torrents` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING` | **behavior** | catalog `engine.max_active_checking` |
| `SEEDGRID_ENGINE_DONT_COUNT_SLOW` | **behavior** | catalog `engine.dont_count_slow` |
| `SEEDGRID_ENGINE_SLOW_DL_THRESHOLD` | **behavior** | catalog `engine.slow_dl_threshold` |
| `SEEDGRID_ENGINE_SLOW_UL_THRESHOLD` | **behavior** | catalog `engine.slow_ul_threshold` |
| `SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER` | **behavior** | catalog `engine.slow_inactive_timer` |

**3. Node override.** New `node_settings` table (migration `00044`): one row per `(node_id, key)`. The value is validated by the **same `Spec`** as the catalog (type, range, options) — there is no second standard. Only keys marked `PerNode` in the catalog accept overrides. **Removing the override returns the Node to the cluster value**; there is no intermediate state.

**4. Delivery and application.** The `POST /api/v1/agent/heartbeat` response gains the `agent_config` block with the **effective values for that Node** (precedence 3 > 2 already resolved) and a `version` (hash of the values). The Agent applies it **on the fly**, without restarting:

| What | How the Agent applies it |
|---|---|
| `engine.*` (queue and anti-stall rotation) | Engine `setPreferences`, only for queue keys — **does not** touch BT port or `save_path` (they are machine-level) |
| `agent.heartbeat_interval` | recreates the heartbeat ticker |
| `agent.command_poll_interval` | recreates the command Poller ticker |
| `agent.log_level` | `slog.LevelVar` |

**No behavior requires a restart.** What requires a restart is what remains in the `.env` (CP URL, credentials, ports, paths) — and that only changes intentionally, with intervention on the Node. The Agent **validates what it receives** (type and sanity: an interval ≤ 0 would crash a ticker) and rejects the bad value while keeping the previous one; a key it does not know (newer CP) is ignored. If the Engine is down at the time of application, the application is **repeated on every heartbeat** until it succeeds (previously, a boot failure was never retried).

**5. The Agent reports what it applied.** The Agent's heartbeat carries `config_version` — the same `version` it received, **only after successfully applying everything**. The Control Plane stores it in `nodes.agent_config_version` and the Portal compares it with the effective version: **up to date**, **waiting for the Agent** (changed and hasn't arrived yet) or **unsupported** (Agent prior to this ADR, which never reported). This is what makes the divergence that motivated the issue visible.

**6. Bidirectional compatibility — the heartbeat decodes with `DisallowUnknownFields`.** A new field in the heartbeat **body**, sent by a new Agent to an old Control Plane, would be rejected with a 400 and the Node would go `OFFLINE`. Therefore, the Agent only sends new fields **after the Control Plane proves it understands them**: `config_version` and `local_config` only enter the body when the last successful response brought `agent_config`, and any heartbeat failure resets this state (a Control Plane rollback prevents the Agent from sending a field the CP doesn't know). Result:

- **Old Agent + new CP:** the Agent ignores `agent_config` and continues on the env. Nothing changes.
- **New Agent + old CP:** the response doesn't bring `agent_config`; the Agent continues on the env and **does not send** the new field. Nothing changes.
- **Control Plane rollback with Agents already up to date:** the Agent only finds out the CP regressed when a heartbeat fails — the only one that still carries `config_version` and gets a `400` —, and on the next attempt it no longer sends the new field. It costs **one** heartbeat, not the Node (the offline limit is 90 s). What was already applied remains.
- **Deploy order** (ADR-027): Control Plane first; Agents afterwards, one by one.

**7. Seeding the overrides — migrating the existing Nodes.** A Node in production already has values in the `.env` (today, node-04: 3 downloads; the others: 10). A new Agent that started obeying only the cluster **would change its behavior on the first heartbeat** — the opposite of "migration without breaking", and the 05/09 error repeated by a deploy. Decision: the Control Plane **seeds** the overrides **once per Node**, based on what the Agent reports, with the same caution that ADR-023 already applies when seeding the catalog from the env on the first boot (*"the database rules; the env only seeds"*). Key K of Node N is seeded **only if all** the following conditions hold:
   - it is the **first report** from N (`nodes.agent_config_seeded_at` is null; after seeding — even zero keys — never again);
   - the Agent reported K because the operator **explicitly defined** it in the `.env` (a code default is no one's decision — if it were, every new Node would be born stuck on the code defaults, the issue's bug);
   - **there is no override** from N for K (the operator who anticipated and created the override rules);
   - the cluster value for K **was not explicitly defined** by the operator (it is still the catalog default) — an explicit cluster value is a fleet decision, and a machine's old `.env` does not contradict it;
   - the value is **valid** according to the `Spec` and **differs** from the cluster value (an equal value does not become an override: it would be noise that would prevent the Node from following the cluster later).

   While the Control Plane has not yet seeded, it sends `report_local: true` in the response and the Agent **applies nothing**: it reports the explicit values from its own env in the next heartbeat and only then applies what comes back. This closes the window in which the Node would adopt the cluster value before the seeding protects it. Each seeding generates a `NODE_CONFIG_SEEDED` Event with the keys and values. Undo = remove the override (and it does not reappear: seeding is a one-time thing).

   Rejected alternatives: *always seed, even against an explicit cluster value* — it would silently contradict a deliberate decision by the operator; *do not seed and require the operator to manually create the overrides before updating each Agent* — it is the **recommended** procedure (`docs/19 - Deployment.md`), but it depends on no one forgetting, and forgetting costs an overloaded Node (the lesson from node-04). Seeding is the safety net, not the main path.

**8. RBAC and auditing.** Cluster value: `settings:write` (`settings` routes from ADR-023, no change). Override: `nodes:write` on `PUT`/`DELETE /api/v1/nodes/{id}/settings/{key}`; read with `nodes:read`. Every change enters the **Audit** trail (ADR-025) with the before and after, following the pattern of the rest: operator action → Audit; seeding, which is the system → Event. Portal: buttons gated by `can()` (hides the button, not the data — pattern from #216).

**Consequences**

Benefits:
- The Agent/Engine behavior is no longer scattered configuration: a cluster value, visible and audited exceptions per Node, and an "up to date" stamp per Node.
- A new Node **is born identical to the cluster** without the operator having to remember anything.
- Atom and i5 coexist: the cluster value serves the majority and the deviation of node-04 is an explicit override, visible in the Portal, and not a hidden difference in a file.
- The change takes effect within one heartbeat, without SSH and without a restart.
- Reuses the catalog (ADR-023), validation (`Spec`), auditing (ADR-025), and RBAC (ADR-035); no new channel or component. Respects ADR-012: the Agent pulls, the Control Plane never connects to the Node.

Limitations / costs:
- **Changing the cluster value affects all Nodes without an override** on the next heartbeat. This is the goal — and that is why the Settings screen lists the Nodes with their own value. A bad cluster value propagates quickly; the safety net is validation by `Spec` and the range of each key.
- **Without the Control Plane on boot, the env rules.** If the Node restarts while the Control Plane is down (a power outage that takes everything down together), it applies the `.env` value after a few heartbeat attempts, and switches to the CP value when it comes back. Therefore, the `.env` must continue to have a sensible value (seeding leaves it equal to the effective one in the cases that matter). There is no disk cache of the last CP value — it was considered and left out to avoid creating a third source of truth.
- **The heartbeat now carries config**: ~12 keys per response (a few hundred bytes). Accepted.
- **Seeding is a one-time thing**: a `.env` edited after the first report **does not** become an override (the precedence is override > cluster > env). Anyone who wants a specific value for the Node creates the override in the Portal.
- **The override is read on every heartbeat** (one query per Node per cycle). For the dozens of Nodes the product targets, it is irrelevant; if one day it isn't, it's a cache invalidated on write.
- Agents prior to this ADR continue **entirely on the env** until they are updated (ADR-027); the Portal marks them as "unsupported".

**Relationship with other ADRs**
- **Extends ADR-023**: the catalog now has a value **per Node** (`PerNode`) in addition to the cluster one, and the consumer of a tunable also becomes the Agent, not just the Workers. The "Agent" category in the dashboard becomes "Agent / Engine".
- **Respects ADR-012** (the CP decides, the Agent executes and pulls) and **does not use the ADR-014 queue**, for the reason above (state converges, event is lost).
- **Depends on the ADR-027 deployment order**: Control Plane first, Agents one by one.
- **Complements ADR-017**: the `.env` generated during bootstrap keeps only identity/machine; what was behavior is removed from there.
- Audit (**ADR-025**) and RBAC (**ADR-035**) follow the existing standard.

# **ADR-040**

**Title:** Image host per tracker — closed-form description in the profile, operator key tied to the host, chain with fallback to imgbb

**Status:** Approved — implemented (#193)

**Context**
ADR-030 fixed **one** host for the publication screenshots (imgbb, global key in the catalog) and recorded the cost: if it goes down, the screenshots stop. ADR-037 decided that the image host policy is **tracker profile data** (layer 2), and left the form decision to #193. Before designing, what actually exists was verified (04/10/2026):

- **UNIT3D does not block any image host.** An image from a host outside the site's allowed list is served by an image proxy, not rejected (`app/Helpers/Bbcode.php`, `whitelisted-image-urls` list). imgbb works on any site in the family. A custom host is a matter of **site rules** (written by the staff, not enforced by the code) and **resilience** — not compatibility.
- **Sites maintain their own host.** samaritano runs an image host on a subdomain, with its own accounts; `autobrr/upbrr` loads dedicated uploaders for the hosts of three trackers.
- **The contract varies from host to host**, even among those that claim to be "ShareX compatible":

| host | key goes in | file field | URL in response |
|---|---|---|---|
| samaritano host | header `Authorization: Bearer <k>` | `file` | `url` (thumbnail in `thumbnail_url`) |
| Generic ShareX (upbrr) | header `Authorization: <k>` | `file` (+ `title` field) | `data.link` or `link` |
| Chevereto (upbrr) | header `X-API-Key: <k>` | `source` | `image.url` |

- **The host itself provides the description.** The samaritano host dashboard offers the download of an `.sxcu` file — ShareX's declarative "custom uploader" format (method, URL, headers, file field, where the URL is in the response).
- **The credential belongs to the host, not the tracker.** The token is created in the image host account; it is not the UNIT3D `api_token`.
- **Two latent defects in the current path**, found during the survey: (i) the imgbb client network error is returned as `err.Error()` to the Portal and the log, and Go's `*url.Error` brings the entire URL — with the key in the query (`?key=`); this is the same leak that #220 closed in the UNIT3D client; (ii) the upload takes 60 s per frame, in series, inside a handler with a 30 s `WriteTimeout`, behind a proxy that gives 60 s to `/api/`. A chain of hosts would aggravate (ii).
- **The screenshot flow does not know which tracker the publication is for.** None of the routes receive the tracker; the publication dashboard has the tracker on hand and does not send it.

**Problem**
Define (a) how a tracker declares its own site's host **without code per host**; (b) how the operator's credential reaches a host described by **third-party content** (the profile is importable) without the profile becoming a credential exfiltration channel; (c) how the publication continues when a host fails, **within the request time limit**; and (d) how the failure becomes visible instead of silent.

**Alternatives Considered**

1. **Keep the single host.** imgbb works across all UNIT3D, so nothing breaks today. Rejected as a complete solution: it does not serve sites whose rules require a custom host, and it maintains the single point of failure that ADR-030 already recorded. **imgbb stays** — as the default and as the last link.

2. **One Go uploader per host (upbrr model).** Rejected for the same reason as alternative 2 of ADR-037: only the host where the maintainer has an account gets in, and each new host requires a release.

3. **The entire `.sxcu` as a template in the profile** — method, URL, headers, and arbitrary fields, free body, regular expression over the response. Expressive, but it is exactly the "free field going straight to the request" that ADR-037 prohibits: an imported profile would assemble any request, and a third-party regex is code with unpredictable execution cost. Rejected.

4. **Tie the image host to the tracker's domain** (the host must be the `api_url` domain or a subdomain of it). Stateless and simple. Rejected as the primary defense: it excludes hosts on their own domain (one of upbrr's three tracker uploaders is on a separate domain from the tracker), and "same registrable domain" requires the public suffix list to avoid mistakes on `co.uk`.

5. **Closed-form description + key tied to the host it was registered for + chain with time budget (adopted).**

**Adopted Decision** (alternative 5)

**1. Family in code, host in data.** The code gains **one** generic uploader — multipart, one file per request, key in a header — alongside the existing imgbb client. The profile gains the optional `image_host` section:

```json
"image_host": {
  "name": "Host do site",
  "upload_url": "https://img.tracker.exemplo.org/api/v1/sharex/upload",
  "file_field": "file",
  "auth_header": "Authorization",
  "auth_prefix": "Bearer ",
  "url_path": "url",
  "thumbnail_path": "thumbnail_url"
}
```

The format is **closed**: always `POST multipart/form-data` with **one** file part (the JPEG frame, `image/jpeg`); the key goes **only** in the `auth_header` header, preceded by `auth_prefix`; no extra headers, extra fields, query strings, or free body; the response is JSON and the URL is extracted via a **dot-separated path** (`data.link`, `image.url`; numeric segment indexes a list) — no regex and no template. The three contracts in the table fit, except for the extra `title` field in the generic ShareX, which is left out until a real host requires it. A host outside this format (key in query or body, non-JSON response, two-step upload) is a **new family**: a PR with code, as ADR-037 already foresees for the tracker family.

No field in the section carries the key, and no field name suggests it (`auth_header` says **where** the key goes, not what it is). The export structural test (ADR-037) already rejects fields named after credentials; the section passes it without exception.

**Validation** — in `ParseDocument` (import) **and** at every service write point, like the `description` section:
- `name`: the same rule as profile names (`CleanName`). It is a label: it does not go into the request, Feed, or audit.
- `upload_url`: up to 2048 characters, absolute, `https`, host as a **DNS name** (no literal IP and no `localhost`), no explicit port, no user/password, no fragment.
- `file_field`: `[A-Za-z0-9_.-]{1,64}`.
- `auth_header`: header token (`[A-Za-z0-9-]{1,64}`), excluding `Host`, `Content-Type`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Cookie`, `Upgrade`, `TE`, `Trailer`, and `Proxy-*`, case-insensitive.
- `auth_prefix`: 0 to 32 printable ASCII characters.
- `url_path`/`thumbnail_path`: 1 to 5 segments `[A-Za-z0-9_]{1,64}` separated by dots; `thumbnail_path` is optional (without it, the thumbnail is the full image, as in imgbb).

**Edit, export, and import** following the `description` section standard (ADR-037, #192): reserved key `image_host` in the profile JSONB, **no migration** for the section; dedicated route `PUT`/`DELETE /api/v1/trackers/{id}/profile/image-host` (`trackers:write`, audited); exported without touching the export (the `Document` embeds the `Profile`); imported only after entering `handledSections`, with the single-section rule — no local section → apply; equal → nothing; **different → protected**, and only `?overwrite=image_host` replaces it. **`schema_version` does not increment**: the section is optional and additive — the same exception recorded for `description`, with the same cost (an older SeedGrid rejects the file due to "unknown field").

**2. The operator's key is bound to the host it was registered for.** The key is layer 3: columns `trackers.image_host_key` and `trackers.image_host_key_host` (new migration). It enters via a dedicated route — `PUT /api/v1/trackers/{id}/image-host-key` with `{"key": "…"}` and `DELETE` on the same route to delete (`trackers:write`, audited; the audit text includes the host, never the key) — and not via the tracker `PUT`: the key only makes sense with an `image_host` section in the profile (without it, `409`), and a dedicated route gives the operation an audit line and an error that state what happened.

When saving the key, the Control Plane also saves the **current `upload_url` host** in `image_host_key_host`. At the time of use, the tracker host only enters the chain if the current `upload_url` host is **exactly** the saved one. If the profile changed hosts — via edit or import — the key **is not sent**: the host is skipped with the reason "key registered for another host", and the operator registers it again. The tracker read returns `has_image_host_key` and `image_host_key_host` (the host is not a secret), and the Portal shows, in the key field, where it goes ("sent only to `img.tracker.exemplo.org`").

This addresses item (b): the credential goes **only where the operator saw it going**. An imported profile can change the `upload_url`, but does not take the key along — matching by identity (the host at the time of registration), not by trusting the profile content. And, unlike alternative 4, it works for a host on any domain.

**Redirection is not followed**: a `3xx` response is a failure. Go discards `Authorization` on a redirect to another domain, but passes along custom headers (`X-API-Key`) — following the redirect would take the key to where no one registered it.

**3. Chain with fallback.** Order: **tracker host** (section present, key registered, and host matched) → **imgbb** (global key configured). The first host that successfully hosts **all** frames wins; the frames of a post **never mix hosts**, and the "all or nothing" per host is the same as ADR-030 (better no description than a half-done one). A host that fails on frame *k* passes the turn, and the next one starts from the first frame; the images the failed host already accepted remain **orphaned** there — accepted: there is no common removal API, and there are few.

- **Failure reason in a closed and sanitized set:** `chave cadastrada para outro host`, `HTTP <status>`, `redirecionamento recusado`, `tempo esgotado`, `falha de conexão`, `resposta sem URL`, `URL inválida na resposta`. **Never** the raw error text: it carries the URL (in imgbb, the key in the query — defect (i), closed here as well) and, in a host message, third-party text.
- **Each failure becomes a Feed entry** (ADR-022), a new type (`screenshot_host_failed`) with the torrent, the tracker, the host (the **DNS name** of the `upload_url`, never the profile `name` label, which is third-party text), the reason, and the next host in the queue. This is item (d): the site host failure stops being invisible just because imgbb saved the post.
- **All failed** → the same error as today (`502 screenshots_failed`), now with the list of reasons per host; the operator can still paste the BBCode manually (ADR-026). **No host configured** → the same error as today.
- **The response gains `host`** (who hosted it) **and `failures`** (`[{host, reason}]`), additive fields. The Portal shows where the images ended up and what failed before.
- **The tracker enters the request:** screenshot routes that upload images accept an optional `tracker_id`. A tracker from another operator does not exist for the requester (`404`, as in profile routes). **Without `tracker_id`, only imgbb** — the current behavior, preserved for those calling the API without the field. The publish panel now sends it.
- **The URL returned by any host is validated** before entering the BBCode: absolute, `https`, up to 2048 characters, no spaces, control characters, `[`, `]`, or quotes. It is interpolated into `[url=…][img]…[/img][/url]`; without validation, a host's response would inject markup into the description the site publishes. This applies to imgbb as well, which today is interpolated without checking.

**4. Time budget.** The upload phase has **25 s in total**, below the 30 s of the Control Plane's `WriteTimeout` and the 60 s of the proxy at `/api/`. Each host receives the **fair share of what remains** — the remainder divided by the remaining hosts —, and the time a host does not use is passed to the next one: with two hosts, the tracker one has up to 12,5 s, and a host that hangs does not consume the fallback's time. The timeout for each request is limited by the host's deadline. Extending the write deadline only for these routes (`http.ResponseController`) and opening a dedicated `location` in the proxy was considered; it was left out because it changes infrastructure for a phase that usually takes just a few seconds. If a real host proves to be slow, the `tempo esgotado` reason in the Feed is the signal to revisit. The route that extracts **and** uploads in a single call (`POST /torrents/{id}/screenshots`) keeps the limitation it already had (the extraction alone might exceed the deadline); the Portal uses extraction and upload in two calls.

**Consequences**

Benefits:
- Tracker with its own host comes in **without code and without a release**, and anyone with an account on the site contributes the host along with the profile (export/import, repository profiles) — the goal of ADR-037 extended to the image host.
- **Real Plan B**: imgbb covers the site host failure, and vice-versa when imgbb goes down on a tracker with its own host.
- **Visible** failure in the Feed, with a readable reason.
- The host credential **is not exfiltrable by an imported profile**, in any domain.
- Closes the imgbb key leak through the error text and the markup injection through the response URL, which existed before this ADR.
- The chain fits within the time the request already has; no infrastructure changes.

Limitations / costs:
- **Only the closed form fits.** A host that requires an extra field, a key outside the header, a non-JSON response, or a two-step flow needs code.
- **A profile that changes host disables the key** until the operator registers it again. It is the price of defense, and it is deliberate.
- **Orphaned images** on the host that failed in the middle of the series.
- **Validation is by name, not by address.** A DNS name that resolves to an internal IP passes (there is no defense against DNS rebinding). Mitigations: no literal IP, no `localhost`, no port, and nothing is sent without a key that the operator registered **for that name**.
- **25 s may be too short** for a slow host with many frames; the symptom appears in the Feed.
- **No `.sxcu` converter** in this delivery: whoever describes the host transcribes six fields from the file the host provides. A converter is the natural improvement — and it needs to separate, on input, the token that the `.sxcu` brings embedded in the headers (layer 3) from the description (layer 2).
- **Live validation depends on an account on the site host.**

**Relationship with other ADRs**
- **Extends ADR-030**: single host → chain; imgbb becomes the last link, with contract and global key unchanged.
- **Implements the "image host policy" of ADR-037** as an optional and additive section of the profile — same edit, export, import, and merge form as `description`, same `schema_version` exception, same `handledSections` fail-closed. The layer separation remains intact: the host description is layer 2; the key, layer 3; the link between the two is the host, recorded in the registration.
- Failure in the **Feed (ADR-022)**; new routes with **audit (ADR-025)** and **RBAC (ADR-035)** in the existing standard; secret masked on read as mandated by **ADR-029**.
- Preserves **ADR-026**: the Agent keeps extracting the frames, the Control Plane hosts and assembles the BBCode, and the operator reviews before publishing.

# **ADR-041**

**Title:** Internationalization — text is assembled where it is read, from code + data; its own catalog in the Portal, without a library

**Status:** Approved — implementation in slices (#31, #32, #33, #34, #57)

**Context**
SeedGrid currently exists only in Portuguese, and will now need at least Portuguese and English. The text the operator reads is generated today in three places, via three paths:

- **In the Portal**, as literals: about 900 snippets across 49 files, with no layer in between.
- **Recorded by the Control Plane**: alerts, events, and activities store a ready-made sentence (`message`), in Portuguese, composed at the moment of recording. Next to it already exist the **code** (`code`: `NODE_OFFLINE`, `NODE_DISK_FULL`, …) and the **data** (`payload`). The Portal displays `message || code`.
- **Composed by the Control Plane for external use**: notifications (Telegram/webhook), assembled via template from code + payload — but with Portuguese labels hardcoded in the code —, and API error messages (61 codes, 268 emission points), where `error.code` is stable and `error.message` is a sentence in Portuguese, often with dynamic details.

Two facts weigh heavily on the decision:

- **A recorded sentence cannot be translated.** What is in the database was written in one language; the reader might want another, and two operators in the same cluster might want different languages. Translating at recording time would require recording in all languages, or choosing one for everyone.
- **A recorded sentence ages.** The project has already paid for this: the Node address written in the alert text became wrong when the address changed, and the fix was to remove the data from the sentence and leave it only in the `payload`. The text is a *presentation* of the data; storing it alongside the data is storing it twice, and one of the copies gets old.

There is no user preference on the server: the Portal stores the theme in the browser (`localStorage`).

**Decision**

1. **Text is assembled where it is read.** The contract between the Control Plane and the display layer is **code + data**. This applies to alerts, events, activities, errors, and audit actions. The Control Plane does not translate for the Portal.

2. **Portal: custom catalog, no library.**
   - One catalog per language in `portal/src/i18n/` (`pt-BR.ts`, `en.ts`), as TypeScript objects. The `pt-BR` one defines the type; the `en` one must satisfy it — **a missing or extra key in a language fails to compile**. A test also checks the parameters for each phrase.
   - `t(chave, parâmetros)` with named interpolation and pluralization via `Intl.PluralRules`. Dates, numbers, and sizes go through `Intl` with the active language.
   - A value with its own markup inside a sentence (a hostname in a monospace font) stays a `{marker}` in the catalog, and `rich(t(key), { marker: <span>…</span> })` puts the element in its place. The whole sentence is translated, so the value lands where each language puts it; the markup never becomes catalog text.
   - Emphasis on a *fragment of the sentence itself* stays inside the sentence: `{strong:…}`, `{em:…}` and `{code:…}` ("Here you choose {strong:which ones go in}"), rendered by `rich`. The fragment is translated with the rest of the sentence; splitting it into a separate key would hide its context from whoever translates.
   - **Where the language comes from:** the operator's choice, stored in the browser (`sg-lang`, like the theme) → the browser's language, if it is one of the supported ones → **English**. A selector is in the menu; changing it does not reload the page.
   - Key named by location and role (`nodes.add.title`), not by the phrase.

3. **Alerts, events, and activities: `code` + `payload` are the source; `message` becomes the fallback.**
   - The Portal builds the title and phrase from the catalog, based on the `code` and `payload`.
   - The Control Plane **continues recording `message`**, in the cluster's default language: this is what API consumers without a catalog read, and what the Portal displays for a code it does not yet know (Portal older than the Control Plane). No data can exist **only** in the `message`.
   - **Contract test:** the list of codes emitted by the Control Plane and the `payload` keys used by each phrase are checked against both catalogs. A new code without an entry, or a phrase that requests data not provided by the `payload`, fails CI.
     The list lives in `control-plane/internal/messages` (`Registry`: kind, code, the `payload` keys every emission writes, and — when one code describes different situations — the `payload` key whose value picks the phrase). A test reads the Control Plane source and fails on a code emitted without an entry, an entry nothing emits, or an emission whose `payload` lacks a promised key; the registry is exported to `shared/messages.json`, which the Portal checks its catalogs against. Reasons that used to be Portuguese text (admission, Scheduler, image host) gain a stable `reason_code` next to the old text.
     On the Portal side, `lib/messages.ts` (`messageText`) builds the phrase from `alerts.messages.<kind>.<code>` in the catalog — choosing the variant by its `payload` key — and falls back to the recorded `message` when the code is unknown or an old record's `payload` lacks a value the phrase needs; `messages-contract.test.ts` checks both catalogs against `shared/messages.json`.
     **Audit (#57):** an operator action follows the same shape. Each `audit_log` row records `action_code` and `action_params` next to `action`, which stays as the Portuguese fallback. The handler names the action with `httpx.SetAudit(ctx, code, params, text)`; when it did not (the request failed before it), the middleware takes the code from the route (`route_*`, the `routeActions` table). These codes are the `audit` kind of `messages.Registry`, and the Portal's phrases live in `alerts.messages.audit.<code>`. A parameter that names a setting is translated through the setting's key, not through the recorded label.

4. **What the Control Plane outputs externally follows the cluster's language.**
   - **Notifications:** a `notify.language` setting (`pt-BR` | `en`, default `en`) and a Go catalog with the same keys as the alerts/events. A notification is a message for a channel, not for a reader: the language belongs to the cluster, not to each operator.
     The catalog is `control-plane/internal/notify/catalog.go`: a short title per code (every alert, event and activity code in `messages.Registry`, checked by a test), the field labels and the rendering lines; templates read the language the hook stamped on the message. Activity reasons come from `reason_code`; a record without it shows the recorded text.
   - **API errors:** `error.code` is the contract. The Portal translates the codes it knows; `error.message` continues to exist, in the cluster's language, as a fallback and as a detail. When the phrase needs a number or name, it will now come in `error.params`, rather than embedded in the text.
     The error codes are part of `messages.Registry` (kind `error`; the AST test reads the code of every `WriteError`/`WriteErrorParams`). Only `invalid_request` is still `generic`: it covers "field X is required" checks that the Portal's forms make before sending, so the operator only meets it when calling the API directly; the Portal shows its `error.message`. Every other code has its own phrase (missing resources are `<resource>_not_found`, an unreadable body is `invalid_body`, an internal failure points to the Control Plane log). In the Portal, `api.ts` builds `ApiError` with the phrase from `api.errors.<code>`, so every existing `err.message` is already translated; `{detail}` in a phrase is the server's `error.message` (third-party error text).
   - **Settings text:** the Control Plane still sends each setting's label and description (in Portuguese) for API consumers; the Portal shows its own, by setting key (`settings.specs.<key>`, a `{channel}` template for the per-channel ones, plus the choice labels and category names). The catalog is exported to `shared/settings.json` (`TestCatalogoExportado`) and `settings-contract.test.ts` fails on a setting without both texts in both languages. Constant notices the API sends with a response (the profile-import warning) are likewise the Portal's own text.

5. **V1 languages:** `pt-BR` and `en`. Adding a third one means adding a catalog file in the Portal and one in the Control Plane.

**Consequences**

*Positive*
- Two operators in the same cluster each read in their own language, including the history: a one-year-old alert appears in English for someone who changed their language today.
- Text no longer ages by design: the phrase is always derived from the current data in the record.
- Incomplete translation is a compilation or test error, not a half-translated screen discovered in production.
- No new dependencies in the Portal, no new file formats, no new build steps.

*Accepted costs and limits*
- **Old records only get new text if their `payload` has the data.** Where it does not, the Portal falls back to the recorded `message`, in Portuguese. Accepted: it is history, and the past is not rewritten.
- The custom catalog lacks what libraries provide for free: automatic key extraction, full ICU format, on-demand loading. With two languages and about a thousand phrases, the entire catalog fits in the bundle; if the number of languages grows, on-demand loading is a change local to `i18n/`.
- There are **two catalogs** for alert/event texts (Portal and Control Plane), because notifications originate from the Control Plane. The contract test keeps them with the same keys; the phrase may diverge between the two, because notifications and screens have different sizes.
- Extraction is laborious (49 files) and is done in slices. While it lasts, the Portal remains mixed; a CI guard prevents an already converted screen from reverting to literals.

**Alternatives considered**
- **i18n library in the Portal (i18next, react-intl, Lingui).** Solves interpolation and pluralization, which `Intl` already solves, and brings a runtime, a custom catalog format, and, in two of them, an extraction/compilation step. Compiler key checking — which best protects against incomplete translation — would come from extra configuration, not for free. Rejected for two languages.
- **The Control Plane translates, via `Accept-Language`.** Keeps the Portal dumb, but the recorded text remains in a single language (the one at the time of recording) and continues to age; it would solve API errors and nothing else.
- **Record the phrase in all languages.** Multiplies what ages and requires migrating records for every new language.
- **Language per user, stored on the server.** Would follow the operator across browsers, but requires a table, API, and migration for a small gain; the browser already handles the theme this way. Left for when there are user preferences for another reason.
- **Default language Portuguese.** The project is born public and in English; whoever installs and chooses nothing sees English, and a Portuguese browser already defaults to Portuguese without anyone configuring it.

**Implementation (slices)**
1. Infrastructure in the Portal + entry screens (#31).
2. Remaining screens, in batches per screen, with the CI guard (#32).
3. Alerts, events, and activities assembled in the Portal + contract test (#33).
4. `notify.language`, catalog in the Control Plane, and `error.params` (#34).
5. Audit actions as code + data (#57).

# **ADR-042**

**Title:** Control Plane installed from published images and a single Compose file; building on the host stays as the contributor path

**Status:** Approved — implementation in slices (#30)

**Context**
Today the Control Plane is installed by cloning the repository and running `scripts/bootstrap-controller.sh`. The script builds the images on the host itself (`seedgrid/controller:dev`, `seedgrid/portal:dev`, Deployment §42) and starts three Compose projects — `deploy/control-plane`, `deploy/observability` and `deploy/portal` — that only work together because the script prepares them first:

- it creates the external volume `seedgrid-prom-targets` and hands it to the controller's user;
- the `seedgrid-core` network is created by one project and declared external by the other two;
- it generates the database password, the Agent registration token, the admin password and the internal webhook token, writes them to `.env`, and copies the webhook token into a file Alertmanager reads;
- it exports `VERSION` from git, because Compose does not run git.

The Control Plane host's node-exporter listens on `172.18.0.1`, the gateway of `seedgrid-core`. The observability project pins that network to `172.18.0.0/16`, but declares it external, and Compose does not apply the address plan of a network it did not create. The address exists only when Docker happens to give that range to `seedgrid-core`: a clean host does, a host already running other networks (a NAS with a dozen containers) does not.

People who self-host on unRAID, ZimaOS, TrueNAS, Portainer or a plain VM expect something else: one Compose file that pulls published images. The first request for SeedGrid on a community forum was exactly that ("just having a compose file to run would already be great").

**Problem**
(a) Without published images every install compiles Go and the SPA on the host, which needs `git`, a clone, network access for the module downloads and minutes of CPU; on a NAS none of that is a given. (b) Three projects glued together by a script cannot be pasted into a NAS interface that takes one Compose file. (c) The secrets come from the script, so without it the operator has to invent four values, two of which (database password, webhook token) they never use again. (d) The Portal image is also the Nodes' download server (ADR-017, ADR-027): an image built somewhere else still has to carry the Agent binaries of the same version.

**Alternatives considered**
1. **Status quo, better documented.** Keeps the build on the host. Answers neither (a) nor (b). Rejected.
2. **Publish images, keep the three projects and the script.** Answers (a), not (b): the script still glues them together.
3. **One all-in-one image** (Postgres, controller, nginx, Prometheus and Alertmanager under a supervisor). The format NAS templates like best. Rejected: it hides five processes behind one health status, turns a Postgres upgrade into a SeedGrid release, and breaks the one-process-per-container model the rest of the deploy follows. A NAS template can be written on top of the Compose file instead.
4. **Publish to Docker Hub.** Pull limits for anonymous users, and a second account to manage. GHCR lives next to the repository and the CI that builds the images.
5. **Published images + one Compose file (adopted).**

**Decision**

1. **Images on GHCR, built by CI on every release tag.**
   - `ghcr.io/rbferreira/seedgrid-controller` and `ghcr.io/rbferreira/seedgrid-portal`, multi-arch (`linux/amd64`, `linux/arm64`).
   - Tags: the exact version (`0.2.0`), the minor (`0.2`) and `latest`. Nothing is published from a branch; a release tag is the only way an image comes to exist.
   - Go cross-compiles on the build platform (`GOARCH` from the target platform), without emulation.
   - The version comes from the tag and is the same in the controller, in the Portal and in the Agent binaries the Portal carries.

2. **The Portal image carries the Node artifacts.** `/node/` serves the Agent binaries for both architectures, `agents.json` and `bootstrap-node.sh` exactly as today (ADR-017, ADR-027), built in the same CI run as the controller. Updating the Control Plane images therefore also publishes the Agent of that version for self-update.

3. **One Compose file, `deploy/compose.yaml`, attached to each GitHub release with the images pinned to that release.**
   - One project (`seedgrid`): Postgres, controller, Portal, Prometheus, Alertmanager and the host's node-exporter.
   - The Prometheus and Alertmanager configuration goes inline (Compose `configs` with `content`), so the file runs on its own, with no clone next to it. This requires Docker Compose 2.23.1 or later.
   - The network and the volumes belong to the project: nothing to create by hand. The volume names stay the ones used today, so an existing install can move to the new file and keep its database and metrics.
   - The Portal's port is `SEEDGRID_HTTP_PORT` (default `80`): on unRAID and on several NAS systems port `80` belongs to the system's own web interface.
   - The other services' images (Postgres, Prometheus, Alertmanager, node-exporter) are pinned to a version, not `latest`.

4. **Secrets: the operator only chooses what they type.**
   - `SEEDGRID_ADMIN_PASSWORD` is the only required value. It is the one the operator types into the Portal.
   - The database password, the Agent registration token and the internal webhook token are generated on the first start by a one-shot `init` service (an existing public image, no build), into a volume of their own. The same service hands the targets volume to the controller's user. It never overwrites a value that already exists.
   - Postgres reads its password from the file (`POSTGRES_PASSWORD_FILE`). The controller gains the `_FILE` form of its secret variables, the Docker convention: `SEEDGRID_X` wins when set, otherwise `SEEDGRID_X_FILE`. An install made by `bootstrap-controller.sh` has the values in `.env` and sees no change.

5. **No address fixed by hand.** The Control Plane host's node-exporter leaves the host network and joins the project's network, reading the host's `/proc`, `/sys` and `/` read-only, without publishing a port. The `172.18.0.1` listen address and the network's fixed address plan go away.
   Accepted cost: that host's network counters become the container's. The network metrics the Portal shows are the Seed Nodes', and those keep their node-exporter on the host network.

6. **Updating is `docker compose pull && docker compose up -d`.** Migrations already run when the controller starts. The Agents are then updated from the Portal, as before (ADR-027).

7. **Building on the host stays.** `bootstrap-controller.sh`, the `build-*.sh` scripts and `Dockerfile.runtime` keep working for contributors, for anyone running an unreleased commit, and for Docker inside a system container (LXC), where `docker build` with `RUN` fails. Pulling a published image is also a way out for that last case: running an image does not hit the AppArmor restriction that building does.

**Out of scope**
- **The Agent in a container.** It reads the Node's disk metrics, touches torrent files directly and replaces its own binary. The reasons ADR-027 gave for keeping it a systemd binary still hold; a containerised Agent is a decision of its own.
- **NAS templates** (unRAID Community Applications, ZimaOS, TrueNAS). They wrap the Compose file and the images, so they come after them.
- **Joining a Node with the qBittorrent it already runs** (#32) and **other Engines** (#31). They change the Node side; this ADR only changes how the Control Plane is installed.

**Consequences**

*Positive*
- Installing the Control Plane needs Docker and one file. No clone, no Go, no Node.js, no build time on a NAS CPU.
- The images are the same everywhere: a bug report names a version, and that version is one set of bytes.
- An `arm64` Control Plane (a Raspberry Pi 4/5, an ARM NAS) becomes possible without building on it.
- The dependency on `172.18.0.1`, which fails on any host whose Docker networks did not start at that range, disappears.

*Accepted costs and limits*
- **The release depends on CI and on GHCR.** A broken release workflow means no images for that tag; building on the host is still there.
- **The secrets volume becomes part of the backup.** If it is lost and the Postgres volume is not, the controller can no longer open its database. The deployment guide says so next to the backup instructions.
- **Compose 2.23.1 or later** for the inline configuration. On an older Compose, building on the host still works.
- **Two install paths to keep working.** A CI job starts the stack from the Compose file and checks `/healthz`, so the published path cannot drift silently from the one developers use.
- An install moving from `bootstrap-controller.sh` to the Compose file needs `docker compose down` in the three old projects first: the container names are the same, and Docker does not create a second container with a name already in use.

**Relationship with other ADRs**
- **ADR-015** chose Docker and Compose for packaging; this ADR decides where the images come from.
- **ADR-017** and **ADR-027**: the Portal image keeps serving onboarding and self-update, now built by CI.
- **ADR-029** moved every third-party credential to the Portal; what is left in `.env` is the deploy's own infrastructure, and this ADR reduces what the operator fills in to the admin password.

**Implementation (slices)**
1. This ADR.
2. Release workflow: multi-arch images on GHCR on every tag, with the Node artifacts in the Portal image.
3. `deploy/compose.yaml` with the `init` service, the controller's `_FILE` variables, the node-exporter without a fixed address and the CI job that starts the stack.
4. Deployment guide and README: the Compose file becomes the Quick start; building on the host moves to a contributor section.

# **ADR-043**

**Title:** A Seed Node joins with the qBittorrent it already runs — nothing changes at entry, and once in the Cluster it is a Node like any other

**Status:** Approved — revised before release (#64). The first design (#32) kept the Engine the operator's for good; it is replaced by this one.

**Context**
Onboarding (ADR-017) assumes an empty machine. `bootstrap-node.sh` installs Docker, mounts a disk at `/data`, starts **its own** qBittorrent in the `seedgrid-engine` container with a password it generates, changes the network (ADR-034, ADR-038) and installs the Agent. On first contact the Agent then sets the Engine's preferences: the BitTorrent port the Control Plane assigned at enroll, `save_path=/data`, no UPnP, no random port, and the download queue (ADR-039).

Most people who try SeedGrid already run qBittorrent, often for years, with thousands of torrents, a port forward, a firewall rule, a folder layout their media server reads, and their own queue settings. On such a machine the bootstrap installs a second qBittorrent that fights the first for the WebUI port, and an Agent pointed at the existing one by hand rewrites its port and download folder. The forum user who asked for rTorrent (#31) asked for this first: coordinate the qBittorrents they already have.

The first version of this ADR treated such a Node as a second kind of Node: SeedGrid never wrote its preferences or restarted it, and the Portal marked it and struck its settings through. That is not what joining means. The machine joins to be orchestrated; what must not happen is the joining itself breaking what the operator has.

**Problem**
(a) The bootstrap has no path that leaves an existing Engine in place. (b) The Agent's first contact overwrites the Engine's port, download folder and queue with the Cluster's values. (c) The Agent reads torrent data directly (migration, screenshots, MediaInfo, storage health), and when the Engine runs in a container the paths it reports (`/downloads/...`) are not the paths the host sees (`/mnt/user/torrents/...`). (d) On the first admission pass, an Engine with dozens of active downloads would have most of them paused by the Control Plane to fit the tracker limit (ADR-021). (e) Restarting the Engine from the Portal relies on the container's restart policy; on an Engine SeedGrid did not install, nothing guarantees it comes back. (f) The machine's network and disks are already set up and work; changing them breaks what the operator has.

**Alternatives considered**
1. **Documentation only** (point the Agent at the existing Engine by hand). Leaves (b) to (f) as they are. Rejected.
2. **Import:** install SeedGrid's own Engine and move the existing torrents into it. Takes the machine over, doubles the disk work, and the operator loses the setup they wanted to keep. Rejected.
3. **An adopted kind of Node** (the first version of this ADR): the Agent orchestrates torrents but never writes the Engine's preferences nor restarts it. Two kinds of Node to keep working, settings that silently do not apply on some Nodes, and a Node that is never really the Cluster's. Rejected.
4. **A way in, not a kind of Node.** The bootstrap reads what the Engine has instead of choosing it; what it read becomes the Node's settings in SeedGrid, so applying them changes nothing; the Node starts in Manual for the operator to review; after that it is managed like every other Node.

**Decision** (alternative 4)

1. **A bootstrap mode for an existing Engine: `ENGINE=existing`.**
   - Takes the WebUI address and credentials (`QBT_URL`, default `http://127.0.0.1:8080`; `QBT_USERNAME`, `QBT_PASSWORD`). It checks them, the Engine version and the restart (item 7) **before** talking to the Control Plane: nothing is enrolled with an Engine the Agent cannot use, and a refusal changes nothing.
   - It reads from the Engine its listening port (`listen_port`) and its default download folder (`save_path`).
   - It does not install the Engine, does not change its password, does not mount disks or install the disk hot-plug recovery, and does not touch the network (privacy extensions and the IPv6 suffix). Docker is not installed. `--network-only` stays available for an operator who wants ADR-038 on that machine.
   - The Node's metrics exporter: if something already answers on port 9100, it is used; otherwise, if Docker is present, only the exporter is started; without Docker the bootstrap says so, and the Node joins without host metrics.
   - Supported Engine: qBittorrent **5.0 or later**. 5.0 is the first with the torrent creator, which publishing (ADR-019) needs; a Node that cannot publish would be a second kind of Node again. A 4.x is refused with the reason and the advice to update.

2. **The port is the one the Engine already has.** The enroll accepts the port the bootstrap read and reserves **that** port for the Node, instead of the next free one in the sequence; the Agent then pins it, as on every Node, so the operator's port forward keeps working. A port outside the sequence is accepted; the Node then has no index, and no deterministic IPv6 suffix (ADR-038), which the bootstrap does not apply anyway. A port already used by another Node is accepted with a warning naming that Node: two Nodes behind the same IPv4 cannot share a port forward, but two on different networks can, and the Control Plane cannot tell.

3. **The download folder is the one the Engine already has.** The Node's storage is the host folder of the Engine's `save_path`, and the Agent pins `save_path` to it, as it pins `/data` on the Nodes it installs. Storage health checks that this folder is writable; there is no dedicated-disk requirement (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false`), because the disk is not one SeedGrid mounted.

4. **The Engine's queue becomes the Node's settings.** On its first contact the Agent reports the Engine's current queue settings (`engine.*`, ADR-039) instead of its environment, and the Control Plane seeds them as the Node's overrides (ADR-039 §7). Two differences from the seeding of an ordinary Node, both because the value comes from a running Engine and not from a file someone wrote: a cluster value the operator set explicitly does not win over it (otherwise joining would change the Engine), and a value outside the catalog's range is brought into it instead of skipped (qBittorrent's "unlimited", `-1`, becomes the catalog's maximum). The Agent holds the first application until the Engine has been read, as it already holds it until the seeding. The effective values are then what the Engine had, so the first application changes nothing; the operator edits or clears them on the Node like any other override. The Control Plane records that the Node joined this way (`nodes.engine_adopted`), which is what tells the seeding and the first mode apart; nothing else reads it.

5. **One thing does change, and the bootstrap says so before enrolling: UPnP and the random port are turned off,** as on every Node — the Cluster relies on a fixed port with a manual port forward. When the Engine had UPnP on, the bootstrap warns that a mapping the router made through UPnP goes away with it and the port must be forwarded by hand; the connectability alerts (ADR-034) say so too if it is not.

6. **An adopted Node joins in Manual.** Its torrents show in the Cluster as they are (the inventory already adopts the hashes the Control Plane does not know), and the admission controller does not pause, start or rotate any of them until the operator moves the Node to SG (ADR-024). Manual only exists under the Flex gate: if the Cluster is in Full auto, the registration of the first such Node moves the gate to Flex, with an event saying why. The other Nodes stay SG, so the Cluster behaves as before. Manual is a review step, not a state the Node is meant to stay in: after the operator moves it to SG it is orchestrated like any Node.

7. **Restart works as on every Node, and the bootstrap makes sure it can.** The Portal's "restart Engine" asks qBittorrent to shut down and relies on something bringing it back. The bootstrap finds what runs the Engine and makes it restart it: a container gets the restart policy `unless-stopped` when its policy would not restart it (and the bootstrap reminds that a Compose file needs the same line, or the next `docker compose up` undoes it); a system service gets a drop-in with `Restart=always`. When it finds neither — qBittorrent started by hand, from a desktop session, or as a user service — it refuses, says how to run it, and changes nothing.

8. **Path mapping between the Engine and the host.** When the Engine runs in a container, the bootstrap reads the container's bind mounts and writes the mapping (`SEEDGRID_ENGINE_PATH_MAP`, pairs `engine path=host path`); `ENGINE_PATH_MAP` overrides it. The Agent translates every path the Engine reports before reading data (migration source, screenshots, MediaInfo), and every path it gives the Engine (`save_path`, migration destination, publishing source). Data can be served for a migration from the Node's storage or from any mapped host folder; a torrent outside all of them is refused with that reason, as today. On the Nodes SeedGrid installs the map is empty and every path is the same on both sides.

9. **Data written by the Agent belongs to whoever owns the destination.** The Agent runs as root; an Engine in a container usually does not. Files the Agent extracts (migration, publishing upload) get the owner of the folder they are written into, so the Engine can keep writing to a partially migrated torrent. On the Nodes SeedGrid installs everything is root's, and nothing changes.

10. **Nothing in the Portal tells such a Node apart.** No badge, no hidden button, no setting shown as not applied: the folder and the port are the Node's, as every Node has its own.

**Out of scope**
- **Other Engines** (#31). This ADR keeps the Engine interface qBittorrent's; rTorrent will reuse the way in.
- **Several download folders as storage.** Free space and disk health are read from the Node's storage only; a torrent kept on another disk is seeded and migrated, but that disk is not measured.
- **The Engine's other preferences** (speed limits, connection limits, categories). SeedGrid does not manage them on any Node, and does not start doing so here.

**Consequences**

*Positive*
- The common case — a machine already running qBittorrent — joins without losing its port forward, its folders or its queue settings, and without the first admission pass pausing its downloads.
- There is still one kind of Node: one Agent path, one Portal, settings that apply everywhere.
- The Agent learns to work with paths it does not own, which an Engine in a container (and #31) needs anyway.

*Accepted costs and limits*
- UPnP is turned off at entry; an operator who relied on it must forward the port by hand.
- qBittorrent 4.x is not supported: the operator updates before joining.
- The bootstrap changes the restart policy of the operator's container or service. It is the smallest change that makes the Portal's restart true, and the bootstrap says it before doing it.
- Without the deterministic suffix, such a Node's IPv6 address is whatever the machine already had; the connectability alerts (ADR-034) still read the address it reports.

**Relationship with other ADRs**
- **ADR-017**: a second way in; the join token and the registration do not change.
- **ADR-024**: the Node starts in Manual, and the gate moves to Flex for it.
- **ADR-034** and **ADR-038**: not applied by the bootstrap on such a Node; `--network-only` remains.
- **ADR-039**: the seeding reads the Engine instead of the environment, with the two differences of item 4; everything after it is the same.

**Implementation (slices)**
1. This revision.
2. Control Plane: the seeding of a Node that joined with its Engine (item 4).
3. Agent: the queue read from the Engine for the seeding, `save_path` through the path map, restart allowed, no exceptions left for such a Node; `pause`/`resume` of qBittorrent 4.x removed.
4. `bootstrap-node.sh ENGINE=existing`: 5.0 or later, the restart (item 7), the UPnP warning.
5. Portal: the badge, the hidden restart and the struck-through settings removed. Guides, and a test on the bench with an existing qBittorrent 5 in a container.

# **ADR-044**

**Title:** A removed Node that joins again comes back as itself when it is the same machine, and as a new Node when it is not

**Status:** Approved (#62)

**Context**
Removing a Node (soft delete) revokes its Agent credential, returns its BitTorrent port to the pool and, with `force`, removes its torrents from the Cluster. The row stays, for history. The registration upserts by `(cluster_id, hostname)`, a constraint that also covers removed rows, and never clears `deleted_at`: a removed machine that joins again with the same hostname lands in the removed row and stays invisible — its Agent gets a credential and its heartbeats are accepted, but no list shows it. Renaming the machine was the only way back.

A machine with the hostname of a removed Node may be that Node coming back (it was removed by mistake, or to move it between networks) or a different machine that got the same name (a replacement, or a fresh install on new hardware). The two want opposite outcomes: the first wants its history, its torrents, its mode, its overrides and its port back; the second wants none of them.

**Problem**
(a) Tell the same machine from a different one with the same hostname, without asking the operator something they cannot know better than the machine itself. (b) A removal must stick: an Agent still running on a removed Node retries its registration on its own, and that must not undo the removal. (c) A different machine must not inherit anything from the removed row.

**Alternatives considered**
1. **Bring the removed row back whenever the hostname matches.** A replacement machine inherits torrents it does not have. Rejected.
2. **Always start a new Node** (free the hostname on removal). The same machine coming back loses its history and its overrides, and gets a new port that its port forward does not point at. Rejected.
3. **Ask the operator** at join time. The bootstrap runs unattended (`curl | sh`), and the operator would answer with what the machine can tell for itself. Rejected.
4. **Identify the machine.** The Agent sends what identifies the machine: `/etc/machine-id`, which every systemd installation has and keeps across reboots and SeedGrid reinstalls, and the Node id from its identity file, when it still has one. Same machine → the Node comes back; anything else → a new Node.

**Decision** (alternative 4)

1. **The Agent identifies the machine at registration**: `machine_id` (from `/etc/machine-id`, or `/var/lib/dbus/machine-id`) and `node_id` (the id in its identity file, when there is one). The Control Plane stores the `machine_id` of every Node (`nodes.machine_id`). The bootstrap sends the `machine_id` and the hostname on enroll too.

2. **The hostname is unique among live Nodes only.** The constraint becomes a partial unique index (`WHERE deleted_at IS NULL`); removed rows keep their hostname for history.

3. **Same machine** — a removed row with the same hostname whose `machine_id` equals the one sent, or whose id equals the `node_id` sent:
   - **through a join token** (the operator ran the bootstrap again): the Node comes back as it was — same id, history, mode, overrides — and the torrents removed with it (those whose `deleted_at` is the Node's) are restored. The enroll gives it back its old BitTorrent port when no live Node took it meanwhile, so the port forward still points at it. Event `NODE_RETURNED`.
   - **through the shared registration token** (an Agent retrying on its own after the removal revoked its credential): refused with `410 node_removed`. The removal sticks; the Agent logs why and says how to bring the Node back, and retries slowly.

4. **A different machine** — no removed row with that hostname matches — registers as a new Node: a new row, nothing inherited. A legacy removed row, from before `machine_id` was stored, only matches by `node_id`; without it, the machine joins as new, which loses history but never mixes two machines.

**Out of scope**
- **Two live machines with the same hostname.** The registration keeps updating the live row, as today.
- **A reinstalled operating system** gets a new `machine-id` and no identity file: it joins as a new Node. Its disk was wiped; what the Cluster knew about it is history.

**Consequences**

*Positive*
- Removing a Node by mistake is undone by running the bootstrap again, with the port forward still valid.
- A replacement machine can take the name of the one it replaces.
- A removal can no longer be undone by an Agent nobody stopped.

*Accepted costs and limits*
- Cloned virtual machines or containers that share a `machine-id` and a hostname look like the same machine. Cloning both is already a misconfiguration for systemd.

**Relationship with other ADRs**
- **ADR-017**: the join token is what tells an operator's return from an Agent's retry.
- **ADR-043**: unaffected; a Node that joined with its own Engine comes back the same way.

**Implementation (slices)**
1. This ADR.
2. Control Plane: migration (`machine_id`, partial unique index), registration and enroll, `NODE_RETURNED`, `410 node_removed`.
3. Agent: `machine_id` and `node_id` at registration, the `410` handled; bootstrap: `machine_id` and hostname on enroll. Guides.

# **ADR-045**

**Title:** rTorrent as a Torrent Engine — one more adapter, the same Node

**Status:** Approved — implementation in slices (#31)

**Context**
The Agent has one Engine adapter, qBittorrent's. Many operators seed with rTorrent, usually behind the ruTorrent web UI, and most seedbox providers ship it, often on shared machines where the user has no root. The forum user who asked for #31 coordinates local qBittorrent instances and remote ruTorrent seedboxes.

`Engine.md` §9 says replacing the Engine must not touch the Control Plane, the Scheduler, the Portal or the API. ADR-043 (revised) adds the principle the operator set for #32: once in the Cluster a Node is a Node like any other, and a Node that would lack a capability (publishing, restart) is not supported rather than supported with a hole.

**Problem**
(a) The Agent is written against the qBittorrent adapter directly: the type is used in `main` and its state strings leak into the sync loop. (b) rTorrent speaks XML-RPC, over SCGI (a Unix socket or a TCP port) or through a web server, not a JSON WebUI API. (c) rTorrent has no single state field. (d) What SeedGrid uses from qBittorrent that rTorrent does not have: a download queue (`engine.*`, ADR-039), a torrent creator (publishing, ADR-019), a `.torrent` export, a connectability verdict (ADR-034). (e) On a shared seedbox the Agent runs without root, systemd system units or Docker, and something else may own rTorrent's lifecycle.

**Alternatives considered**
1. **Talk to ruTorrent's HTTP endpoints.** They are a UI backend: undocumented, plugin-dependent, and absent where rTorrent runs without ruTorrent. Rejected.
2. **Support rTorrent as a reduced Node** (no queue, no publishing, no restart), with the Portal saying which settings do not apply. That is the second kind of Node ADR-043 rejected. Rejected.
3. **Wrap rTorrent in a qBittorrent-compatible shim.** Another daemon to install and keep in step with two APIs. Rejected.
4. **An rTorrent adapter behind an Engine interface, with the Agent supplying what rTorrent lacks.** The queue and the torrent creator are implemented in the Agent, once, for any Engine without them.

**Decision** (alternative 4)

1. **An Engine interface in the Agent.** The Agent's code depends on `engine.Engine`, not on a concrete adapter: torrents with the SeedGrid state already mapped (Database.md §36), add/stop/start/recheck/delete, export, content path, trackers, version, availability, connectability, preferences (port, folder, queue), shutdown. The qBittorrent adapter moves behind it unchanged. `SEEDGRID_ENGINE` (`qbittorrent`, the default, or `rtorrent`) chooses the adapter.

2. **Protocol: XML-RPC, local.** `SEEDGRID_ENGINE_RTORRENT_URL` is `scgi:///path/to/rpc.socket`, `scgi://127.0.0.1:5000`, or `http(s)://…/RPC2` (with basic auth) for setups that only expose it through a web server. The XML-RPC codec is small and in the Agent: no dependency. Supported: rTorrent **0.9.8 or later** (`d.multicall2`, `system.client_version`).

3. **State mapping.** `d.hashing` ≠ 0 → CHECKING; `d.message` set and the tracker refusing → ERROR (the message is the reason, as with qBittorrent); stopped (`d.state` 0) and complete → PAUSED; stopped, incomplete and marked queued by the Agent → QUEUED; stopped, incomplete otherwise → PAUSED; started and complete → SEEDING; started and incomplete → DOWNLOADING. The swarm and connected peer counts come from the trackers' scrape (`t.scrape_complete`/`t.scrape_incomplete`) and from the peer list.

4. **The queue is the Agent's when the Engine has none.** rTorrent starts every torrent it holds. For an Engine without a queue the Agent applies the `engine.*` settings itself, on a short loop: it keeps at most `max_active_downloads` downloads running (and the upload and total limits), stops the rest and marks them as queued (`d.custom` key `seedgrid`), starts the oldest queued one when a slot frees, and with `dont_count_slow` does not count a download below the slow thresholds for longer than the timer. It only ever starts torrents it queued: a torrent stopped by the operator or by the admission controller (ADR-021) stays stopped. Checking is serial in rTorrent; `max_active_checking` has nothing to limit there. The settings, their meaning and the Portal stay the same for every Node.

5. **The torrent creator is the Agent's.** rTorrent cannot create a `.torrent`. The Agent builds it (BitTorrent v1, piece size chosen by total size as qBittorrent does, the private flag and `source` as requested), and the adapter loads it pointing at the data, so the Engine checks it and seeds it — the same result publishing has today (ADR-019). The qBittorrent adapter keeps using qBittorrent's creator.

6. **Export.** rTorrent keeps each torrent's `.torrent` in its session folder. The adapter reads it and removes the keys rTorrent adds (`rtorrent`, `libtorrent_resume`), which leaves the original bytes the Control Plane stores (ADR-018).

7. **Connectability.** rTorrent has no "firewalled" verdict. The adapter reports none (the Control Plane reads it as unknown, ADR-034) and reports incoming peers from the peer list (`p.is_incoming`), which is what the inbound-silence alert reads.

8. **How it joins: the way in of ADR-043.** SeedGrid does not install rTorrent; a machine running it joins with `ENGINE=rtorrent` on the same path as `ENGINE=existing`: nothing changes at entry, the port and folder are read from rTorrent (`network.port_range`, `directory.default`), the Node starts in Manual. Its queue settings are seeded as off (`engine.queueing_enabled=false`), because rTorrent ran without one; the operator turns the Agent's queue on when reviewing the Node. UPnP does not exist in rTorrent; nothing to warn about.

9. **Restart.** The Portal's restart calls `system.shutdown.normal` and relies on something bringing rTorrent back, as with qBittorrent: a container restart policy, a systemd unit (system, or user with lingering), or — on a seedbox whose provider owns rTorrent — a command the operator gives at install (`ENGINE_RESTART_CMD`, e.g. the provider's `app-rtorrent restart`), which the Agent runs instead. Without any of these the bootstrap refuses, as it does for qBittorrent.

10. **A Node without root.** `bootstrap-node.sh` run as a regular user installs the Agent for that user: the binary in `~/.local/bin`, the environment in `~/.config/seedgrid-agent.env`, the identity in `~/.local/share/seedgrid-agent`, started by a systemd user unit with lingering when available, otherwise by a crontab entry that starts it at boot and every minute if it is not running. Without a supervisor that restarts it, the self-update (ADR-027) re-executes the new binary instead of exiting. It works for either Engine. Host metrics need a node-exporter the user can run; without one the Node joins without them, as an adopted Node without Docker does today.

**Out of scope**
- Installing rTorrent. SeedGrid still installs only qBittorrent.
- ruTorrent: it keeps working next to the Agent, as it did; SeedGrid does not depend on it.
- Transmission and Deluge: the interface of item 1 is what they would need, but this ADR does not add them.

**Consequences**

*Positive*
- An rTorrent Node is a Node like any other: same states, settings, publishing, migration and Portal.
- The Agent stops depending on qBittorrent's types; the next Engine is an adapter.
- The rootless install serves any Engine on a machine where the operator is not root.

*Accepted costs and limits*
- The Agent's queue reacts on its own loop (seconds), not inside the Engine.
- The Agent hashes the data when creating a `.torrent` for rTorrent: the same disk read qBittorrent's creator does, in another process.
- On a seedbox the provider may limit resources, ports and processes; the Agent asks for nothing privileged, but the operator's plan must allow a long-running process.

**Relationship with other ADRs**
- **ADR-017 / ADR-043**: rTorrent joins only through the way in of ADR-043.
- **ADR-019**: publishing on rTorrent uses the Agent's creator.
- **ADR-021 / ADR-024**: admission stops and starts torrents through the same interface; the Agent's queue never starts what admission stopped.
- **ADR-027**: the self-update re-executes when no supervisor restarts the Agent.
- **ADR-039**: the `engine.*` settings mean the same on every Node; for rTorrent the Agent applies them.

**Implementation (slices)**
1. This ADR.
2. The Engine interface; the qBittorrent adapter behind it, no change in behavior.
3. XML-RPC over SCGI/HTTP and the rTorrent adapter, read side (torrents, states, trackers, version, peers).
4. The rTorrent adapter, write side (add, stop/start, recheck, delete, export, folder).
5. The Agent's queue.
6. The Agent's torrent creator.
7. `bootstrap-node.sh ENGINE=rtorrent`, the rootless install and the self-update by re-exec.
8. Guides, and a test on the bench with rTorrent (and ruTorrent) in a container and as a rootless install.

# **10. Official Template**
Every ADR must follow a standardized structure.
Minimum fields:
- Identifier.
- Title.
- Context.
- Problem.
- Alternatives considered.
- Adopted decision.
- Consequences.
- Benefits.
- Limitations.
This standard facilitates reading, reviewing, and comparing decisions.
# **11. Governance**
ADRs represent the official documentation of SeedGrid's architectural decisions.
Significant changes to the architecture must be preceded by the creation or update of a corresponding ADR.
This practice ensures transparency, traceability, and preservation of the project's technical knowledge.
# **12. Relationship with Documentation**
ADRs complement the other platform documents.
While functional documents describe **how** the platform operates, ADRs record **why** certain decisions were adopted.
This separation prevents information duplication and preserves the context of architectural choices.
# **13. Summary**
ADRs constitute the official history of SeedGrid's architectural evolution.
They allow us to understand:
- which problems motivated each decision;
- which alternatives were evaluated;
- which benefits were expected;
- which limitations were accepted.
This history must accompany the platform throughout its evolution.
# **14. Final Statement**
SeedGrid's architecture must evolve in a deliberate and documented manner.
Architecture Decision Records represent the official mechanism for recording structural decisions, preserving technical knowledge, facilitating future reviews, and ensuring that relevant changes occur in a transparent and well-founded manner.
