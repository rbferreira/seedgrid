# **SeedGrid**
**Deployment Specification**
Version: 1.0
Status: Draft
Document: 19 - Deployment.md
# **1. Objective**
This document defines the official deployment process for SeedGrid.
Its goal is to standardize the platform installation, reduce differences between environments, and ensure that all components are deployed consistently.
# **2. Philosophy**
The deployment must be:
Simple.
Reproducible.
Documented.
Automatable.
The architecture must allow a new environment to be created by following only this document.
# **3. Scope**
This document covers:
Environment preparation.
Control Plane installation.
Seed Node installation.
Node registration.
Platform validation.
Updates.
Rollback.
# **4. Prerequisites**
Minimum infrastructure.
Available Control Plane.
Operational network.
Available Docker.
Supported Operating System.
IP connectivity between all components.
# **5. Components**
Deployed (V1):
Control Plane (API + internal services: Scheduler, Alert Worker, Inventory/Monitoring).
Portal (React SPA served by nginx, which also proxies `/api/` and `/healthz` to the Control Plane).
PostgreSQL 17.
Prometheus.
Alertmanager.
node-exporter (one per host).
SeedGrid Agent.
Torrent Engine (qBittorrent).

> **ADR-016:** **Grafana has been removed** — metrics visualization now lives in the Portal (the Control Plane queries Prometheus via PromQL and serves the series in JSON; flow Portal → API → Prometheus). **Loki (logs) is deferred** (reevaluation trigger in ADR-016), as well as **Traefik/TLS**: in V1 there is no dedicated reverse proxy or TLS termination — the Portal serves directly on `:80`. References to Grafana/Loki/Traefik/certificates in this document are from the reference architecture, **not** active V1 components.
# **6. Organization**
The repository is cloned **on the Control Plane host** — wherever the operator prefers; the examples use `/opt/seedgrid` — and is the single source of deployment artifacts. Seed Nodes do not clone anything: they receive what they need from the Control Plane (§50). Relevant structure:
```
/opt/seedgrid
├── control-plane/       # Go service (cmd, internal, migrations/, Dockerfile*)
├── agent/               # Go service (cmd, internal, engine adapters)
├── portal/              # React SPA (Vite)
├── deploy/
│   ├── control-plane/   # docker-compose.yml + .env.example (Postgres + Controller)
│   ├── portal/          # docker-compose.yml + Dockerfile + nginx.conf
│   ├── observability/   # docker-compose.yml + prometheus.yml + rules.yml + alertmanager.yml
│   └── agent/           # docker-compose.yml (+ .env.example), seedgrid-agent.service, seedgrid-agent.env.example
└── scripts/             # automation: build-*.sh, bootstrap-controller.sh, bootstrap-node.sh, acceptance.sh
```
Configuration is always external to the containers, via `.env` files generated from `.env.example` (see §16). There are no versioned `config/`, `logs/`, or `backups/` directories: persistent data lives in named Docker volumes (Postgres, Prometheus, Alertmanager) and the Agent's local state in `StateDirectory`/`SEEDGRID_AGENT_DATA_DIR`.
# **7. Control Plane Preparation**
Minimum steps:
Install Operating System.
Update packages.
Configure network.
Configure time.
Install Docker.
Install Docker Compose.
Create directories.
Prepare volumes.
# **8. Control Plane Deployment**
Simplified flow.
Operating System
↓
Docker
↓
Volumes
↓
Database
↓
Control Plane
↓
Portal
↓
Monitoring
Each step must be validated before the next.
# **9. Seed Node Preparation**
Each Node must follow the same procedure.
Operating System.
Network.
Docker.
Torrent Engine.
SeedGrid Agent.
Exporters.
Storage.
# **10. Registration**
After Agent installation.
Flow.
Node
↓
Agent
↓
Control Plane
↓
Registration
↓
Inventory
↓
Heartbeat
↓
Operation
# **11. First Startup**
After deployment, the following must be verified:
Portal.
API.
Database.
Scheduler.
Monitoring.
Alerts.
Agents.
Heartbeats.
Inventory.
All components must be operational before the platform is released.
# **12. Expansion**
Adding new Nodes will follow exactly the same process.
Prepare equipment.
Install Agent.
Register.
Validate.
The Scheduler can use it immediately after registration.
# **13. Standardization**
All environments must use:
Same directory structure.
Same naming conventions.
Same policies.
Same configuration mechanisms.
This standardization reduces operational errors and simplifies maintenance.
# **14. Objectives**
The deployment must be repeatable.
Any operator must be able to completely rebuild an environment using only this documentation.
# **15. Fundamental Principle**
SeedGrid deployment must prioritize automation and simplicity.
Whenever possible, manual procedures must be replaced by reproducible and documented mechanisms.
# **16. Configuration**
All platform configuration must be external to the containers.
Examples:
- `.env` files (from `.env.example` — see §45);
- YAML files (Prometheus/Alertmanager);
- Portal configuration (nginx);
- Control Plane configuration;
- Monitoring configuration.
Replacing a container must never result in configuration loss.
# **17. Container Management**
Each platform component must have its own container.
V1 containers (ADR-016):
- Portal (nginx serving the SPA + API proxy);
- Control Plane (`seedgrid-controller`);
- PostgreSQL (`seedgrid-postgres`);
- Prometheus (`seedgrid-prometheus`);
- Alertmanager (`seedgrid-alertmanager`);
- node-exporter (one per host);
- Torrent Engine (`seedgrid-engine`, on the Seed Node — neutral engine name; the Agent looks for it by this name, see `SEEDGRID_ENGINE_CONTAINER` in §45).

The SeedGrid Agent **does not** run in a container: it is a static binary executed by systemd on the Seed Node host. There is no Grafana, Loki, or dedicated reverse proxy in V1 (the Portal's nginx serves as the HTTP front). This separation favors maintenance, updates, and fault isolation.
# **18. Persistence**
All persistent data must use dedicated volumes.
V1 named volumes:
- `seedgrid-postgres-data` — Official state of the Cluster (PostgreSQL);
- `seedgrid-prometheus-data` — Prometheus TSDB (30d retention);
- `seedgrid-alertmanager-data` — Alertmanager state.

The Agent's local state is kept on the host, in `SEEDGRID_AGENT_DATA_DIR`/`StateDirectory` (identity after the first registration). Torrent data is stored in `/data` on the Seed Node. Volumes must remain independent of the container lifecycle.
# **19. Update**
Updates must follow a standardized procedure.
Recommended flow:
Backup.
↓
Backup validation.
↓
Container update.
↓
Initialization.
↓
Platform validation.
↓
Release for operation.
If any step fails, it must be possible to perform a rollback.
# **20. Rollback**
Every update must have a documented rollback procedure.
The rollback must preserve:
- Cluster state;
- configurations;
- inventory;
- events;
- audit.
Whenever possible, it must be executed using the same mechanisms planned for deployment.
# **21. Adding a New Node**
Summary procedure:
Prepare hardware.
Install Operating System.
Install Docker.
Configure storage.
Install Torrent Engine.
Install SeedGrid Agent.
Register Node.
Validate inventory.
Validate Heartbeat.
Release for production.
The procedure must be identical for any number of Nodes.
# **22. Node Replacement**
Node replacement must follow the same steps as the initial deployment.
If torrent backups exist, they can be restored before releasing the Node.
The architecture must not require special procedures for hardware replacement.
# **23. Cluster Expansion**
Cluster growth will occur only by adding new Nodes.
No structural changes to the Control Plane will be required.
After registration, the Scheduler can immediately use the newly available resources.
# **24. Validation**
After any deployment, at least the following must be verified:
- Portal accessible;
- API operational;
- Database intact;
- Scheduler active;
- Prometheus collecting metrics;
- Alertmanager operational (webhook reaching the Control Plane);
- Agents connected;
- Heartbeats updated.

This check is automated by `scripts/acceptance.sh` (see §44).
# **25. Production Checklist**
Before going into production, the following must be confirmed:
- backups configured;
- operational monitoring (Prometheus + Alertmanager);
- alerts configured (rules in `deploy/observability/rules.yml`);
- default credentials changed (admin/admin → see §46);
- time synchronization;
- firewall configured (including BitTorrent port-forwards per Node — see §47);
- updated documentation;
- basic tests executed (`scripts/acceptance.sh`).

> In V1 there are no TLS/certificates to validate (no dedicated reverse proxy; ADR-016). TLS termination enters in a later phase.
# **26. Environments**
The architecture must support different environments using the same deployment process.
Examples:
Homelab.
Lab.
Development.
Production.
Differences must be limited to external configurations, preserving the same architecture and component organization.
# **27. Principles**
Every deployment must be:
- reproducible;
- documented;
- automatable;
- validatable;
- reversible.
These principles ensure predictability and reduce operational risks throughout the platform's lifecycle.
# **28. Deployment Documentation**
Every deployment must produce minimum environment documentation.
Recommended information:
- Cluster name;
- SeedGrid version;
- deployment date;
- responsible parties;
- number of Nodes;
- infrastructure inventory;
- backup policies;
- monitoring policies.
This documentation will facilitate maintenance, audit, and future expansions.
# **29. Versioning**
All platform components must have explicit versioning.
Examples:
- SeedGrid version;
- Agent version;
- Control Plane version;
- Portal version;
- API version;
- Scheduler version.
The Portal must clearly display the running version.

**Build version scheme (ADR-024).** The version of the binaries comes from the repository's **git tags**, not a fixed value. The `scripts/version.sh` helper derives the value with `git describe --tags`, removing the commit hash (the chosen "merge"): on the exact tag → `v1.0.0`; N commits after the tag → `v1.0.0-N`; no tag/git → `dev` (fallback). This value is injected into the binary at build time via `-ldflags "-X main.version=…"` (Control Plane and Agent), consumed by the `build-*.sh`. The Agent reports the version as `agent_version` on registration/heartbeat ("Agent" column in the Dashboard); the Control Plane exposes its own via the API. To publish a new version, create the tag (`git tag vX.Y.Z`) and rebuild.
# **30. Compatibility**
The Control Plane must validate the compatibility of connected Agents.
When an incompatibility exists:
- the operator must be informed;
- details of the incompatibility must be presented;
- the expected behavior must be documented.
The platform must prioritize compatibility between consecutive versions whenever possible.
# **31. Upgrade**
The upgrade process must follow low-risk principles.
Objectives:
- preserve configurations;
- preserve Cluster State;
- minimize downtime;
- allow rollback.
Upgrades must be documented and versioned.
# **32. Post-Deployment**
After deployment completion, operational validations must be performed.
Suggested items:
- Portal login;
- Node registration;
- Heartbeat reception;
- metrics collection;
- test Event generation;
- log querying;
- test Alert creation;
- Scheduler validation.
These tests represent the minimum acceptance of the environment.
# **33. Audit**
Deployments, upgrades, and rollbacks must generate audit records.
Examples:
- date;
- operator;
- installed version;
- result;
- duration;
- remarks.
These records aid traceability and governance.
# **34. Scalability**
The architecture must allow gradual platform growth.
Adding new Nodes must not require changes to the Control Plane deployment process.
Likewise, upgrading the Control Plane must not require reinstalling the Nodes.
# **35. Security**
Before the environment is released, the following must be verified:
- default credentials changed (the initial `admin/admin` operator: login returns `default_credentials: true` and the Portal displays a warning until changed via `POST /api/v1/auth/password`);
- strong and unique `SEEDGRID_AGENT_REGISTRATION_TOKEN` (protects the registration of new Agents);
- restricted administrative access;
- unused ports disabled;
- operational firewall (Postgres exposed only on `127.0.0.1`; BitTorrent port-forwards only to Node IPs);
- backups configured.
The deployment must only be considered complete after these validations.

> TLS/certificates are not part of V1 (ADR-016: no Traefik/dedicated reverse proxy). When TLS termination is adopted, it will require an ADR and an update to this document.
# **36. Automation**
All procedures described in this document must be automatable.
The documentation represents the official process; future tools may partially or fully automate its execution, preserving the same principles and results.
# **37. Evolution**
The deployment process must evolve alongside the platform.
New components must include deployment, upgrade, and removal instructions before being considered an official part of SeedGrid.
# **38. Architecture Summary**
Operator
│
▼
Infrastructure Preparation
│
┌─────────────┴─────────────┐
▼                           ▼
Control Plane                Seed Nodes
│                           │
▼                           ▼
Initial Validation          Agent Registration
└─────────────┬─────────────┘
▼
Operational Cluster
The deployment must result in a consistent, reproducible environment ready for expansion.
# **39. Best Practices**
Every deployment must observe the following principles:
- use versioned configurations;
- keep infrastructure documented;
- validate all components before production;
- automate repetitive tasks;
- test upgrade procedures;
- keep rollback procedures up to date.
# **40. Final Statement**
The SeedGrid deployment strategy is designed to provide predictability, simplicity, and reproducibility.
By clearly separating Control Plane components from Seed Nodes and keeping configurations external to containers, the platform favors automation, facilitates maintenance, and reduces operational risk throughout its lifecycle.

# **41. Actual procedure (V1) — reference topology**
An installation has **one host for the Control Plane** and **one host per Seed Node**. Any Linux machine with Docker works — server, VM, mini PC, Raspberry Pi (the Agent and Engine run on `amd64` and `arm64`).

| Host (example) | Role | What runs |
|---|---|---|
| `controller` · `192.168.0.10` | Control Plane | `seedgrid-controller`, `seedgrid-postgres`, `seedgrid-prometheus`, `seedgrid-alertmanager`, `seedgrid-node-exporter`, `seedgrid-portal` (nginx `:80`) |
| `node-01` · `192.168.0.11` | Seed Node 1 | `seedgrid-engine` (BitTorrent 6881), `seedgrid-node-exporter`, Agent (systemd) |
| `node-02` · `192.168.0.12` | Seed Node 2 | `seedgrid-engine` (BitTorrent 6882), `seedgrid-node-exporter`, Agent (systemd) |

The addresses are illustrative. The Control Plane and Nodes only need to reach each other via HTTP: each Node talks to port `80` of the Control Plane (Agent → API, via the Portal's nginx), and the Control Plane's Prometheus scrapes `9100` from each Node. Nothing else is exposed between them.

**The Control Plane and Node are on different hosts.** The Node's Engine and node-exporter use the host network and occupy ports `8080` and `9100`; an install made by `bootstrap-controller.sh` keeps the controller's API on `127.0.0.1:8080` of its host.

# **42. Build flow**
Images come from two places. On every release tag, CI publishes the controller and Portal images to GHCR (see the end of this section). Without a release — a contributor, an unreleased commit — they are built on the Control Plane host itself, by the `build-*.sh` scripts, which are called by `bootstrap-controller.sh`. Each one compiles the artifact in an ephemeral container (`docker run`) and builds the final image with a Dockerfile **without `RUN` steps**:
- `build-controller.sh` — compiles the Go binary in `golang:1.26-alpine` (`CGO_ENABLED=0`) and builds `seedgrid/controller:dev` (`Dockerfile.runtime`).
- `build-portal.sh` — runs `npm ci && npm run build` in `node:22-alpine` and builds `seedgrid/portal:dev` (nginx), already with the Node artifacts in `/node/` (§50).
- `build-agent.sh` — compiles the Agent static binary (`agent/bin/seedgrid-agent`); `GOARCH=arm64` cross-compiles to ARM.

All three derive the binary version via `scripts/version.sh` (git tags — see §29); overriding with `VERSION=…` in the environment forces a value. `build-portal.sh` reuses `build-agent.sh` for the binaries served to Nodes (one per architecture), inheriting the same version.

The Dockerfile without `RUN` exists so the image can also be built where `docker build` with `RUN` does not work — typically Docker running inside a system container, where the AppArmor profile does not apply to the build. In these environments you may need to pass extra options to the `docker run` in the scripts: export `SEEDGRID_DOCKER_RUN_OPTS` (e.g., `--security-opt apparmor=unconfined`) before running. On a regular Docker host nothing is needed.

Those who prefer the standard Docker path can use the multi-stage Dockerfiles: `docker compose build` in `deploy/control-plane` for the controller, `docker build -f deploy/portal/Dockerfile .` from the repository root for the Portal. To inject the actual version in this path, export `VERSION=$(scripts/version.sh)` before the build (the controller's `docker-compose.yml` passes it through as a build-arg; for the Portal, pass `--build-arg VERSION`).

**Published images (#30).** The `Release images` workflow builds `ghcr.io/rbferreira/seedgrid-controller` and `ghcr.io/rbferreira/seedgrid-portal` for `linux/amd64` and `linux/arm64` from those multi-stage Dockerfiles, and publishes them only from a `vX.Y.Z` tag, as `X.Y.Z`, `X.Y` and `latest`; a pre-release (`vX.Y.Z-rc.N`) gets only its exact tag. The build stages run on the CI machine's platform and Go cross-compiles, so nothing is emulated. The Portal image carries the Agent binaries for both architectures, `agents.json` and the bootstrap files of the same version. They are written by `scripts/node-artifacts.sh`, the same script `build-portal.sh` uses, so both paths serve the same `/node/`. The controller's two Dockerfiles run it as the same user (UID 65532, the one `bootstrap-controller.sh` gives the targets volume to). On a pull request that touches what goes into the images, the workflow builds them for both architectures without publishing them, and checks the version in the controller, its user, and the Node artifacts in the Portal.

# **43. Installation and Update**
**Prerequisites:** on the Control Plane host, Docker with Compose 2.23.1 or later (building on the host also needs `git`); on Nodes, a Linux with `curl` and `root` access (onboarding installs Docker if missing).

**Control Plane — from scratch (#30, ADR-042).** `deploy/compose.yaml` installs the Control Plane from the release images (§42), with no clone and no build. It needs Docker Compose 2.23.1 or later, because the Prometheus and Alertmanager configuration goes inside the file. In a directory of its own:
```sh
curl -fsSLO https://github.com/rbferreira/seedgrid/releases/latest/download/compose.yaml
echo 'SEEDGRID_ADMIN_PASSWORD=<the password you will type into the Portal>' > .env
docker compose up -d
```
- **Only the admin password is required.** On the first start, a one-shot `init` service generates the database password, the Agent registration token and the internal webhook token into the `seedgrid-secrets` volume, and the controller and Postgres read them from there (the `_FILE` variables, §45). Each file is readable only by the container that needs it. `init` never overwrites a value that already exists.
- **Back up `seedgrid-secrets` with the database.** If that volume is lost and `seedgrid-postgres-data` is not, the controller can no longer open its database.
- **Only the Portal publishes a port**, `SEEDGRID_HTTP_PORT` (default `80`; on unRAID and several NAS systems `80` is the system's own interface). The API, Prometheus, Alertmanager and Postgres are reachable only inside the project's network.
- **Update:** download the `compose.yaml` of the new release (each one is pinned to its version), or set `SEEDGRID_VERSION` in `.env`, then `docker compose pull && docker compose up -d`. Migrations run when the controller starts; the Agents are then updated from the Portal.
- **Coming from `bootstrap-controller.sh`:** copy `deploy/control-plane/.env` next to `compose.yaml`, run `docker compose down` in `deploy/control-plane`, `deploy/observability` and `deploy/portal` (the container names are the same), then `docker compose up -d`. The volumes keep their names, so the database and the metrics stay, and `init` takes the database password and the tokens from that `.env` instead of generating new ones.
- **The host's node-exporter** reads the host's `/`, `/proc` and `/sys` read-only from inside the project's network; its network counters are the container's. It is mounted without `rslave`, which Docker refuses on hosts whose `/` is not a shared mount.

CI runs `scripts/test-compose.sh` on a clean machine for every pull request that touches the images or the Compose file, and on every release before `compose.yaml` is attached to it. The script starts the stack, checks the login, `/node/`, the secrets and their owners, the webhook token, the Prometheus targets and the published ports, restarts it, and removes it with its volumes. It refuses to run when a SeedGrid database volume already exists.

**Control Plane — building on the host**, for contributors and for running a commit that is not a release yet. From the repository clone, as `root` or with `sudo`:
```sh
git clone <repository-url> /opt/seedgrid && cd /opt/seedgrid
sh scripts/bootstrap-controller.sh
#   creates deploy/control-plane/.env and generates the secrets (§46); compiles the controller,
#   starts Postgres+Controller, Observability and Portal; at the end displays the Portal URL
#   and the admin login.
```
There is nothing to edit beforehand. Those who prefer to choose the values create the `.env` from the
`.env.example` before running: whatever is already filled in is respected (§45). Only the Portal
(`:80`) is exposed on the network — the API is served through it at `/api`, and ports `8080` (API) and
`9090` (Prometheus) listen only on `127.0.0.1` on the host.

`bootstrap-controller.sh` orchestrates the three composes (`deploy/control-plane`, `deploy/observability`, `deploy/portal`).

**Seed Node — from scratch:** a single command, run on the Node as `root`, without cloning the repository (zero-touch
onboarding, §50). The Portal displays it ready, with the token, under **Health & Metrics → Add Node**:
```sh
curl -fsSL http://<control-plane>/node/bootstrap-node.sh | sudo CP_URL=http://<control-plane> JOIN_TOKEN=<token> sh
```
The `sudo` is for those not logged in as `root`; without privilege the script stops at the first line and tells you how to run it.

**Control Plane update — building on the host:** `git pull && git fetch --tags && sh scripts/bootstrap-controller.sh` — the same script as installation. It rebuilds the images and recreates only what changed; the `.env`, the secrets and the volumes (database, metrics) are preserved. The `fetch --tags` ensures the displayed version tracks the new tag. Prometheus and Alertmanager are always restarted: their configuration is mounted from files that `git pull` replaces, and a running container keeps reading the file it started with (#30).

**The host's node-exporter left `172.18.0.1` (#30).** It used to run on the host network listening on `172.18.0.1`, the gateway of `seedgrid-core`, an address that only existed when Docker happened to give that range to the network. It now joins `seedgrid-core` like the other services, publishes no port, and Prometheus reaches it by name (`seedgrid-node-exporter:9100`). Its network counters are the container's.

**Cleanup after the update (#43).** Each update rebuilds the images, and the previous ones stay on disk untagged. At the end, the bootstrap runs `docker image prune -f --filter label=seedgrid.image`. SeedGrid's runtime images carry that label, so only they go; untagged images from anything else on the host aren't touched. A failed cleanup doesn't fail the update. The Docker build cache isn't cleared, for the same reason: it is shared with the host's other builds. Those who want to reclaim it can run `docker builder prune --filter until=168h` by hand, which keeps the last week's cache so the next build stays fast. Images built before this change have no label and have to be removed once by hand (`docker image prune`).

**Node update:** via the Portal — the Control Plane publishes the new Agent binary and offers self-update per Node (ADR-027). For everything that is not the binary (network, storage, Engine compose), see §50.4.

> **Migrations applied at boot.** There is no separate `migrate` script: the controller applies the embedded goose migrations (`control-plane/migrations/`, embedded via `embed.go`) during startup. Deploying a new controller version automatically applies pending migrations.

# **44. Validation (end-to-end acceptance)**
`scripts/acceptance.sh` runs against an already running environment (on the Control Plane host) and validates the happy path, exiting `!= 0` if anything fails (22 checks). It covers: `healthz`; `/metrics` exposing `seedgrid_*`; operator login (token) + `default_credentials` flag; Nodes `ONLINE` with `health_score`; host resources (disk/mem) reported; unique `bt_port` per Node; **metrics served by the CP from Prometheus** (`/api/v1/metrics/query`); `system/health` (API + Database); `GET /events` and `GET /alerts`; Scheduler flow (add torrent → Node chosen) and the full torrent cycle (**pause/resume/time series/remove**); `samples` for a nonexistent torrent → `404`; password change rejecting a weak password (`400`); **Prometheus** scraping targets (`up`); and the **Portal** serving (`HTTP 200`) with `/healthz` proxy. It reads credentials from `deploy/control-plane/.env` automatically (`API`/`PORTAL`/`PROM` variables override the defaults).

# **45. Environment Variables**
Configuration is always external, via `.env` generated from the `.env.example`.

**Control Plane** (`deploy/control-plane/.env` → container `seedgrid-controller`; see `control-plane/internal/config/config.go`):

| Variable | Default | Required | Description |
|---|---|---|---|
| `SEEDGRID_DATABASE_URL` | — | **yes** | PostgreSQL DSN (assembled in the compose from `POSTGRES_*`). |
| `SEEDGRID_HTTP_ADDR` | `:8080` | no | API listen address. |
| `SEEDGRID_LOG_LEVEL` | `info` | no | Log level. |
| `SEEDGRID_AGENT_REGISTRATION_TOKEN` | — | **yes** | Token required for the initial registration of an Agent. |
| `SEEDGRID_ADMIN_USERNAME` | `admin` | no | Initial administrator operator. |
| `SEEDGRID_ADMIN_PASSWORD` | — | **yes** | Initial admin password (created on first boot if it does not exist). |
| `SEEDGRID_ALERT_WORKER_INTERVAL` | `30s` | no | Alert Worker evaluation frequency. |
| `SEEDGRID_NODE_OFFLINE_AFTER` | `90s` | no | Heartbeat expired beyond this ⇒ Node `OFFLINE`. |
| `SEEDGRID_DISK_IO_SATURATED_PCT` | `90` | no | Disk utilization (busy%) ≥ this ⇒ `DISK_IO_SATURATED` alert, provided it is sustained for `alerting.disk_saturated_for` (default `90s`, #150 — see `25 - Business Rules.md`); resolves with 15pp hysteresis. `0` disables. |
| `SEEDGRID_SAMPLE_RETENTION` | `168h` | no | History retention per torrent (7 days). |
| `SEEDGRID_PROMETHEUS_URL` | `http://seedgrid-prometheus:9090` | no | Prometheus queried by the CP (metrics endpoints). |

> In compose, `SEEDGRID_DATABASE_URL` is derived from `POSTGRES_DB`/`POSTGRES_USER`/`POSTGRES_PASSWORD` (defined in `.env`).

**Secrets from files (#30).** `SEEDGRID_DATABASE_URL`, `SEEDGRID_AGENT_REGISTRATION_TOKEN`, `SEEDGRID_INTEGRATION_TOKEN` and `SEEDGRID_ADMIN_PASSWORD` also accept the `_FILE` form, the Docker convention for secrets kept out of the environment: `SEEDGRID_ADMIN_PASSWORD_FILE=/run/secrets/admin-password` reads the value from that file. The variable wins when both are set. The line break at the end of the file is not part of the value. A `_FILE` that cannot be read stops the controller at startup, with an error that names the variable.

**Agent** (`/etc/seedgrid-agent.env` on the Seed Node; see `agent/internal/config/config.go`). Since **ADR-039** (#144), the `.env` stores only **identity and machine** — Control Plane URL, credentials, ports, paths, Engine container name. The **behavior** (intervals, log level, Engine queue) comes from the Control Plane and its env is just a **fallback** (next table).

| Variable | Default | Required | Description |
|---|---|---|---|
| `SEEDGRID_CONTROL_PLANE_URL` | — | **yes** | Control Plane URL — port `80` of the host, served by the Portal's nginx (e.g., `http://controller.example`). |
| `SEEDGRID_AGENT_REGISTRATION_TOKEN` | — | 1st registration | Registration token (after the 1st registration, the identity lives in `StateDirectory`). |
| `SEEDGRID_AGENT_DATA_DIR` | `/var/lib/seedgrid-agent` | no | Agent local state. |
| `SEEDGRID_AGENT_ENGINE` | `qbittorrent` | no | Torrent Engine. |
| `SEEDGRID_ENGINE_QBT_URL` | `http://127.0.0.1:8080` | no | qBittorrent WebUI API. |
| `SEEDGRID_ENGINE_QBT_USERNAME` | `admin` | no | WebUI user. |
| `SEEDGRID_ENGINE_QBT_PASSWORD` | — | recommended | WebUI password. |
| `SEEDGRID_AGENT_STORAGE_PATH` | `/data` | no | Torrent data path. |
| `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` | `true` | no | Is `/data` a **dedicated disk**? `true`: storage is only healthy if it is a *mountpoint* and writable (protects against an unmounted disk). `false`: Single-disk Node — `/data` is a folder on the system disk and only writing is checked. **Declared by `bootstrap-node.sh` during installation, never inferred by the Agent**; if absent (or invalid value) it defaults to `true`, which is the safe side and what Nodes prior to the variable always had. See `docs/06 - Storage.md` §35.3 and §50.1. |
| `SEEDGRID_ENGINE_BT_PORT` | `6881` | no | BitTorrent port for this Node (unique in the cluster — see §47). |
| `SEEDGRID_ENGINE_CONTAINER` | `seedgrid-engine` | no | Torrent Engine container name — the Agent runs `docker inspect` on it to report the Engine's uptime (`engine_started_at`). It must be the `container_name` in the Node's compose; a name that doesn't match **does not throw an error**, it just leaves the field empty. |
| `SEEDGRID_AGENT_MIGRATION_PORT` | `6900` | no | Migration transfer server port (ADR-018). The same across all Nodes. |

**Agent and Engine behavior — fallback (ADR-039).** These variables **are no longer the source**: the Control Plane delivers the **effective** value for each Node (Node override > cluster value, in Settings → Agent / Engine) in the heartbeat response and the Agent applies it on the fly. The env is only valid **when the Control Plane sends nothing** — pre-ADR-039 CP, unreachable CP, or the Agent at boot before the first heartbeat — and the code default when not even the env exists. The bootstrap **does not** write them.

| Variable | Code default | Catalog key (**cluster** default) |
|---|---|---|
| `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | `30s` | `agent.heartbeat_interval` (`10s`) |
| `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | `5s` | `agent.command_poll_interval` (`5s`) |
| `SEEDGRID_LOG_LEVEL` | `info` | `agent.log_level` (`info`) |
| `SEEDGRID_ENGINE_QUEUEING_ENABLED` | `true` | `engine.queueing_enabled` (`true`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | `2` | `engine.max_active_downloads` (`10`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_UPLOADS` | `10000` | `engine.max_active_uploads` (`10000`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_TORRENTS` | `10000` | `engine.max_active_torrents` (`10000`) |
| `SEEDGRID_ENGINE_MAX_ACTIVE_CHECKING` | `1` | `engine.max_active_checking` (`1`) |
| `SEEDGRID_ENGINE_DONT_COUNT_SLOW` | `false` | `engine.dont_count_slow` (`false`) |
| `SEEDGRID_ENGINE_SLOW_DL_THRESHOLD` | `1` (KB/s) | `engine.slow_dl_threshold` (`1`) |
| `SEEDGRID_ENGINE_SLOW_UL_THRESHOLD` | `1` (KB/s) | `engine.slow_ul_threshold` (`1`) |
| `SEEDGRID_ENGINE_SLOW_INACTIVE_TIMER` | `300` (s) | `engine.slow_inactive_timer` (`300`) |

The **code** default (`MAX_ACTIVE_DOWNLOADS=2`, `HEARTBEAT_INTERVAL=30s`) is deliberately the old one: the fallback without a Control Plane is the usual behavior. The **cluster** default is what most of the fleet used in `.env`. A practical consequence: **without the Control Plane at boot** (a power outage that takes everything down together) the Node starts with the `.env` value and switches to the Control Plane's when it comes back up — keep a sensible value for the machine in `.env`. Complete table, ranges, and the seeding of overrides in `25 - Business Rules.md` §3.1.1b.

Additionally, the Engine's compose `.env` on the Node defines **only** `SEEDGRID_ENGINE_BT_PORT` — the same value as in `/etc/seedgrid-agent.env`. Both are written by `bootstrap-node.sh` with the port assigned by the Control Plane (§47).

# **46. Initial credential and password change**
On a new installation, `bootstrap-controller.sh` **generates** any secrets that are empty or have the example value in `.env` — database password, Agent registration token, admin password, and internal webhook tokens — and displays the admin login when finished (the password is saved in `deploy/control-plane/.env`). It only generates them when the database does not yet exist: with the Postgres volume already created, the database password and the admin operator are already written there, and the script only warns if it finds the example value.

The first boot creates the operator defined in `SEEDGRID_ADMIN_USERNAME`/`SEEDGRID_ADMIN_PASSWORD`. If someone manually sets `admin/admin`, the login returns `default_credentials: true` and the **Portal displays a warning** until the password is changed via **`POST /api/v1/auth/password`** (**Profile** page). Changing the password is a mandatory item on the production checklist (§25, §35).

# **47. BitTorrent port per Node and inbound allowance (manual, firewall-agnostic)**
Each Seed Node needs a **unique BitTorrent port** in the cluster to receive inbound connections (essential for seeding and maintaining ratio). **The Control Plane assigns the port** during onboarding — the lowest available starting from `6881` — and `bootstrap-node.sh` writes it to the two places that must agree on the Node: `/etc/seedgrid-agent.env` (the Agent pins the qBittorrent `listen_port` to it and disables UPnP) and the Engine's compose `.env`. The operator neither chooses nor types the port.

What **remains manual** is the firewall: SeedGrid is router-agnostic and creates no rules. Depending on the Node's network:

- **Public IPv4:** port-forward `WAN <port> → <node-ip>:<port>` (TCP+UDP).
- **Behind CGNAT (no public IPv4):** port-forwarding is harmless; allow **IPv6 inbound** on the Node's port (TCP+UDP) to its address. The Node's IPv6 address follows the same sequence as the port: `<prefixo>::5eed:<porta − 6880>` (ADR-038, see §50.2) — Node 1's rule points to `::5eed:1`, Node 4's to `::5eed:4` —, and it does not change if the Node is recreated.

- **Discovery by the operator:** the Agent reports the port in the heartbeat; the Control Plane exposes `bt_port` + `last_ip` in `GET /api/v1/nodes`, and the **Portal shows "BT Endpoint" (IP:port)** per Node (Dashboard and Health page). This is where the operator sees what to allow; `bootstrap-node.sh` also prints the instruction upon completion.
- **Origin of `last_ip` (proxy-aware):** the Agent reaches the CP via nginx (the CP listens only on 127.0.0.1), so the CP uses the proxy's `X-Real-IP` header — which nginx sets with the Node's actual `$remote_addr` — to record the `last_ip` (fallback: `X-Forwarded-For`, then `RemoteAddr`). Without this, all Nodes would register the proxy's IP (incident #84).
- Without inbound allowed, the Node still downloads (outbound/DHT), but seeds poorly.

# **48. Additional Seed Node**
Same as the first: the same onboarding command (§43, §50), with a new token. The Control Plane
assigns a distinct BitTorrent port to each Node and starts collecting its metrics on its own
(ADR-017) — there is no manual step per Node nor binary to copy between machines.

# **49. V1 features relevant to deployment/operation**
- **Add torrents** via **magnet** and **upload of `.torrent` file** — including **multiple `.torrent` files at once**; the Scheduler chooses the Node.
- **Time series per torrent** (`GET /api/v1/torrents/{id}/samples`), with retention controlled by `SEEDGRID_SAMPLE_RETENTION`.
- **Network metrics per Node** and rankings presented in the Portal.
- **Two alert sources**, unified in the Portal (ADR-016): (1) internal CP **Alert Worker** (domain rule `NODE_OFFLINE`); (2) **Prometheus/Alertmanager** (rules in `deploy/observability/rules.yml`: `NodeExporterDown`, `DiskSpaceLow`, `HighMemory`), which send webhooks to `POST /api/v1/integrations/alertmanager`.
- **Portal** with sidebar navigation and URL per page: Dashboard, Add, Alerts & Events, Health & Metrics (with the list and detail of each Node), Profile, and Torrent detail.

# **50. Seed Node onboarding — what `bootstrap-node.sh` configures**
Zero-touch onboarding (ADR-017) is a single command, executed on the new Node:

```sh
curl -fsSL http://<cp>/node/bootstrap-node.sh | sudo CP_URL=http://<cp> JOIN_TOKEN=<token> sh
```

The script and everything it installs are **served by the Control Plane** at `/node/*` (Engine compose, Agent unit, storage scripts, Agent binary per architecture — ADR-027); the Node does not clone the repository. This section lists what the script leaves configured and how to update a Node that already existed when any of these things changed.

## 50.1 Bootstrap steps

| # | Step | Result |
|---|---|---|
| 1 | Docker | Installed if missing. |
| 2 | **Network (IPv6)** | SLAAC guaranteed (ADR-034) and privacy extensions disabled; see §50.2. |
| 3 | **Enroll** | The Control Plane validates the join token and assigns the BitTorrent port (lowest available in the sequence, ADR-017) **and the Node index** (`node_index`, ADR-038). |
| 4 | Artifacts | Engine compose, Agent unit, storage scripts, and the Agent binary for the Node's architecture, downloaded from `/node/`. The binary runs `--version` before the unit is installed. |
| 5 | **Storage** | `/data` mounted by UUID and hotplug auto-recovery (layers 1 and 5 of `docs/06 - Storage.md` §35); the result becomes the `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` statement in the Agent env (step 8). **Without a disk**, the Node comes up with `/data` on the system disk — a supported configuration: the script writes `false`, warns in the terminal, and the Node works normally (§50.1.1). |
| 6 | **IPv6 suffix** | The Node's address becomes `<prefixo /64>::5eed:<N>` (§50.2). It comes **before** the Engine, so it announces the definitive address to the tracker from the first announce. |
| 7 | **Engine** | `seedgrid-engine` container (§50.3), on the assigned BitTorrent port; the WebUI password is generated **on the Node** and never leaves it. |
| 8 | Agent | `/etc/seedgrid-agent.env` (including `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT`, from step 5) and systemd service; the Agent registers with the Control Plane. The `.env` carries only **identity and machine**: the behavior (intervals, logging, Engine queue) comes from the Control Plane (§45, ADR-039) — the new Node **comes up identical to the cluster**. |

The same script, with `--storage-only`, re-executes only step 5 (and reconnects the Engine to the new `/data`, and writes `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`) on an already registered Node; with `--network-only`, steps 2 and 6 (§50.2).

### 50.1.1 Storage during onboarding: dedicated disk or system disk
The script detects the largest USB disk and mounts its existing filesystem at `/data` (non-destructive), and **records what it did in the Agent's env**:

| What the script found | `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` | Result |
|---|---|---|
| Disk mounted/found at `/data` | `true` | The Agent requires `/data` to be a *mountpoint* and writable. |
| No disk (single disk: VM, mini PC, NAS) | `false` + **terminal warning** | `/data` stays on the system disk; the Agent only checks for write access; `NODE_DISK_FULL` monitors the space. The Node is eligible in the Scheduler. |
| `--storage-only` with disk | `true` | The Node got a dedicated disk; the Agent now requires it. |

Bootstrap overrides, all optional:

| Variable | Effect |
|---|---|
| `STORAGE_DEVICE=/dev/sdX1` | Points to the partition to mount at `/data`, instead of looking for the largest USB disk. |
| `STORAGE_FORMAT=1` | Formats as ext4 if the partition has no filesystem (**destructive**). |
| `STORAGE_REQUIRE_MOUNT=true` | Declares a dedicated disk even if there is no disk **right now**: the Node stays out of the Scheduler (`storage_ok=false`) until the disk is mounted. This is used when the disk arrives after installation. |
| `STORAGE_REQUIRE_MOUNT=false` | Declares a single-purpose disk (incompatible with `--storage-only`). |

A second bootstrap pass **does not downgrade** a Node that has already declared a disk: if the disk is not found, but the previous env is `true` (or `/data` is in `fstab`), the value remains `true` — the disk is gone, the Node did not become "single-disk". The full reasoning, and why the requirement is declared instead of inferred, is in `docs/06 - Storage.md` §35.3.

## 50.2 Network: a single, stable IPv6 address
The Node needs to **announce** to the tracker, and be **reached** at, a single IPv6 address: this is the address the operator allows in their firewall (ADR-034). The bootstrap takes care of what, without it, makes the address unpredictable. The considered interface is the one for the default route (IPv6 first, IPv4 as fallback); `NODE_IFACE=<interface>` fixes it manually.

- **Guaranteed SLAAC (ADR-034).** If the Node's network manager disables Router Advertisements (the case of a system container whose hypervisor generates `IPv6AcceptRA=false`), the bootstrap installs a systemd-networkd drop-in that re-enables it and survives the base file regeneration.
- **Privacy extensions disabled (#142).** With them enabled — the default in many distributions (`use_tempaddr = 2`) — the Node gets, in addition to the stable address, a **temporary address that rotates on its own**, and it is the temporary one that outbound traffic uses. The result is that what the tracker sees in the announce is not the address the operator allowed. The bootstrap writes, in `/etc/sysctl.d/99-seedgrid-ipv6.conf`:

  ```
  net.ipv6.conf.all.use_tempaddr = 0
  net.ipv6.conf.default.use_tempaddr = 0
  net.ipv6.conf.<interface>.use_tempaddr = 0
  ```

  applies the three keys immediately (`sysctl -w`: only them, never the whole file) and removes any existing temporary addresses. `all` and `default` cover interfaces created after boot (a late-rising WiFi, a VLAN); the interface one applies right away. It is idempotent and **preserves the other lines** in the file (for example, a manually set `accept_ra`). To check: `sysctl net.ipv6.conf.all.use_tempaddr` must return `0`, and `ip -6 addr show scope global` must not list any `temporary` address.
- **Deterministic suffix `::5eed:<N>` (ADR-038, #202).** The default SLAAC address is EUI-64, derived from the MAC: recreating the Node, swapping the NIC, or migrating hosts changes it, and the operator's firewall rule ends up pointing to nowhere — besides being **unpredictable before the Node exists**. The bootstrap fixes the lower half of the address and lets the **/64 prefix come from the Router Advertisement**, so a prefix change from the provider is still tracked.

### 50.2.1 The convention

| | |
|---|---|
| Address | `<prefixo /64>::5eed:<N>` — for example `2001:db8:1:2::5eed:4` for Node 4. |
| `N` | The **Node index**: its position in the BitTorrent port sequence, `N = porta BT − 6880` (port 6881 is Node 1; 6884 is Node 4). The Control Plane returns it in the enroll as `node_index`; since the port is unique in the cluster, the index is too. It goes in **decimal digits** in the last group — Node 12 is `::5eed:12` — so you can read the Node number directly from the address. It ranges from 1 to 1000 (the port sequence). |

- **Prepare the firewall rule before the Node exists.** The next Node gets the lowest free port in the sequence (the Portal's Node list shows the ports in use); its address is `<prefixo>::5eed:<porta − 6880>`, and the rule can be created before the bootstrap runs — the Node does not go through the window where it is already up, announcing, and still without a rule (where the tracker sees it as unreachable).
- **Replace a Node.** Removing a Node returns the port (and the index) to the pool. The Node that replaces it gets the lowest free port — in the common case, the one just returned — and **inherits the same address**: the rules remain valid without editing.
- **The token does not replace IPv6 change detection.** It only fixes the **suffix**; a prefix rotation by the provider changes the address of all Nodes, and the `NODE_IPV6_CHANGED` alert remains how you find out (`docs/16 - Alerting.md`). What changes is the cost of the fix: one unknown remains, the new prefix.
- **Opt-out:** `IPV6_SUFFIX=off` in the bootstrap keeps the address chosen by the network manager (delegated addressing, stateful DHCPv6, custom policy). A Node whose BitTorrent port is outside the Control Plane sequence (manual installation with a shared token) has no index, and the script does not apply the token to it.

### 50.2.2 How the script applies it, per network manager

The script discovers which manager handles the default route interface and uses that manager's mechanism; **it does not guess** — if it is not one of the two below, it prints the instruction and moves on. No failure here interrupts onboarding: the Node works with whichever address the manager chooses.

| Manager | What the script does |
|---|---|
| **systemd-networkd** | Drop-in at `/etc/systemd/network/<file>.network.d/99-seedgrid-ipv6-ra.conf` (the file reported by `networkctl status`, including one generated by netplan in `/run`) with `IPv6AcceptRA=true` and `IPv6Token=static:::5eed:<N>`. Then `networkctl reload` and `networkctl reconfigure <interface>` (restarts the service only if `reconfigure` does not exist). The drop-in survives base file regeneration. |
| **NetworkManager** | Edits the active connection's **keyfile** (`/etc/NetworkManager/system-connections/`): in the `[ipv6]` section, `addr-gen-mode=eui64` and `token=::5eed:<N>`, preserving the rest of the file (a WiFi connection's file has the password). Flow: backup to `/var/backups/seedgrid-node/` → edit → `nmcli connection reload` → **checks what NM loaded** → `nmcli device reapply`. If any step fails, the keyfile is **restored**. Connections that are not keyfiles in `/etc` (generated by netplan, in `/run`) or with `ipv6.method` other than `auto` are not modified. |
| **Other** | Prints instructions for the operator to apply in their manager. |

**NetworkManager: what is not obvious** (and why the script does what it does):

1. **`nmcli` cannot configure the token.** There is a deadlock between two validations: NM only accepts `ipv6.token` with `addr-gen-mode=eui64`, and netplan (the writing backend on Ubuntu) rejects the pair (`ipv6-address-generation` and `ipv6-address-token` are mutually exclusive). Furthermore, a failing `nmcli modify` **still writes the keyfile**, leaving disk and memory divergent — a reboot would apply the change on its own, without anyone having edited the firewall rule. That is why the script writes the keyfile and, if anything fails, restores it.
2. **`::5eed:4` is written as `::94.237.0.4`.** It is the same address (`0x5eed` = 94.237): when the top 96 bits are zero, NM uses dotted notation. It is not corruption — and a comparator that only recognizes one of the forms will take an already applied token as unapplied. The script normalizes both sides to the full form before comparing (`nmcli -g` still escapes the `:` with a backslash).
3. **After `reapply`, a parallel EUI-64 is left over, and `autoconf=0` is the wrong fix.** The **kernel** is what generates the token address, from the Router Advertisement's Prefix Information Option. Turning off `autoconf` kills renewal along with it: the address stays up with its existing lifetime and **expires in about two hours**. The EUI-64 is just a leftover from the `reapply` moment; deleting it with `ip -6 addr del` fixes it, and it does not come back with `autoconf` preserved. The script deletes the leftovers (EUI-64, stable privacy address, and temporary addresses from the same `/64`, only the dynamic ones) **after** the token address is up — never before.

### 50.2.3 How to verify

**Existing is not enough: the address must renew.** The acceptance criterion for any procedure that applies the token is to observe the address's `valid_lft` **go up** after a Router Advertisement (observe for about 7 minutes). This alone is what separated "it worked" from "it will drop on its own in two hours".

```sh
ip -6 -o addr show dev <interface> scope global | grep '::5eed:'    # the address and valid_lft
# ... wait for a Router Advertisement (~7 min) and repeat: valid_lft must have gone back up
```

After changing the address of a Node that **was already running** (`--network-only`), **restart the Engine** (it needs to reopen the socket on the new address) and **reannounce** to the trackers: without the reannouncement, the tracker keeps the "unconnectable" verdict collected in the window between the address change and the firewall rule edit.

## 50.3 Engine and `/data`
- **The container name is a contract.** The Engine runs in the `seedgrid-engine` container, defined by the `container_name` in the compose file served by the Control Plane. The Agent runs `docker inspect` on this name to report the Engine's uptime (`engine_started_at`); the default is the same (`SEEDGRID_ENGINE_CONTAINER`, §45). If the two diverge **nothing fails** — the field just stays empty — which is why the name lives in a constant and a test checks the compose, bootstrap, reattachment script, and Agent default. The name is engine-agnostic: the Engine is a product concept, qBittorrent is today's implementation.
- **The service key in the compose file remains `qbittorrent`** (it is the image it runs). Do not rename it on a Node that is already running: the compose would start treating the old container as orphaned and leave it running alongside the new one, competing for the same ports (host network).
- **`/data` can be a dedicated disk or a folder on the system disk** — the bootstrap records which one in `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` (§50.1.1). Mount propagation downwards, hotplug auto-recovery, and the *mountpoint* requirement only matter for a dedicated disk; with `/data` on the system disk they are harmless: the bootstrap does not install the reattachment (there is no disk UUID), and `rslave` propagation on a `/data` that is just a folder in the shared root simply propagates nothing.
- **The `/data` bind uses `propagation: rslave`** — a disk mounted after the Engine comes up appears inside the container. What this solves and what it does not solve, and the prerequisite regarding the mount that contains `/data`, are in `docs/06 - Storage.md` §35.1. The bootstrap records the choice (`rslave`, or `rprivate` when the host does not support it) in `SEEDGRID_DATA_PROPAGATION` in the compose `.env`.

## 50.4 Update an existing Node
Onboarding saves a copy of the script at `/opt/seedgrid-node/bootstrap-node.sh`; the commands below use it (as `root`). On a Node installed before this, the copy does not exist — download it again, with the mode after `sh -s --`: `curl -fsSL http://<cp>/node/bootstrap-node.sh | sudo sh -s -- --network-only`.


### Network (privacy extensions and IPv6 suffix)
`sh /opt/seedgrid-node/bootstrap-node.sh --network-only` applies steps 2 and 6 on an already registered Node: it disables privacy extensions and sets the suffix `::5eed:<N>`, with `N` calculated from the BitTorrent port in `/etc/seedgrid-agent.env` (`IPV6_NODE_INDEX=<N>` sets it manually). It is idempotent: if the address is already up it changes nothing, and repeating it does not restart the network. **One Node at a time**, in this order, because the address changes:

1. **create the firewall rule for the new address** (`<prefixo>::5eed:<N>`, BT port, TCP+UDP) — this can be done before running the script;
2. run `--network-only` and confirm the address (§50.2.3), including `valid_lft` increasing;
3. **restart the Engine and re-announce** (§50.2.3);
4. confirm external reach (the new rule taking effect) and only then delete the old address rule;
5. move on to the next Node.

A Node that has already applied the convention manually (`::5eed:<N>` with the right `N`) is recognized: the script sees the address is up and **does not touch the network** — it only ensures `use_tempaddr = 0` (adding the keys to the sysctl file without touching the other lines) and removes any EUI-64 residue, if present.

### Storage (dedicated disk or system disk)
**No existing Node requires action:** without `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` the Agent requires the *mountpoint*, which is the usual behavior. Two cases require a deliberate change:

- **Single-disk Node that was left out of the cluster** (`storage_ok=false`, `não é mountpoint`, `NODE_STORAGE` open, not receiving torrents), because it was installed when the requirement applied to all: declare the single disk in the env and restart the Agent —
  ```sh
  grep -q '^SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=' /etc/seedgrid-agent.env \
    && sed -i 's/^SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=.*/SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false/' /etc/seedgrid-agent.env \
    || echo 'SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false' >> /etc/seedgrid-agent.env
  systemctl restart seedgrid-agent
  ```
  The Agent reports `storage_ok=true` in the next heartbeat; `NODE_STORAGE` resolves itself. Confirm that `/data` is, in fact, a folder on the system disk and that its free space is enough for what the Node will store — from now on it is `NODE_DISK_FULL` that monitors it.
- **Single-disk Node that got a dedicated disk:** `sh /opt/seedgrid-node/bootstrap-node.sh --storage-only` with the disk connected. It mounts, installs the reattachment, recreates the Engine, and writes `true`.

### Engine (container name and mount propagation)
The compose is only read when the container is created, so a Node created before a compose change keeps the old container until it is recreated. For the container name changes (`seedgrid-qbittorrent` → `seedgrid-engine`) and mount propagation (`rprivate` → `rslave`), **one Node at a time**:

1. **Find the Node's compose** (the directory where it was created):
   `docker inspect <container-atual> -f '{{index .Config.Labels "com.docker.compose.project.working_dir"}}'`
2. **Replace the `docker-compose.yml`** in that directory with the current one — `git pull`, if it is a clone of the repository, or `curl -fsSL http://<cp>/node/docker-compose.yml -o docker-compose.yml`.
3. **Set the container name in the Agent env**, explicitly. This applies to the old Agent (whose default is different) and the new one, so the binary update order does not matter:
   ```sh
   grep -q '^SEEDGRID_ENGINE_CONTAINER=' /etc/seedgrid-agent.env \
     && sed -i 's/^SEEDGRID_ENGINE_CONTAINER=.*/SEEDGRID_ENGINE_CONTAINER=seedgrid-engine/' /etc/seedgrid-agent.env \
     || echo 'SEEDGRID_ENGINE_CONTAINER=seedgrid-engine' >> /etc/seedgrid-agent.env
   ```
4. **Check the `rslave` prerequisite**: `findmnt -no PROPAGATION -T /data` must show `shared`, `slave`, or `shared,slave`. If it shows `private`, set `SEEDGRID_DATA_PROPAGATION=rprivate` in the compose `.env` (Docker would refuse the container).
5. **Recreate the Engine** with the new compose: `docker compose up -d`. Since the service key has not changed, the compose recreates the container in place — the old container exits and `seedgrid-engine` starts. qBittorrent restarts (a few seconds without seeding; the torrent state is resumed from disk).
6. **Reinstall the storage reattachment script — on Nodes that have it** (`ls /usr/local/bin/seedgrid-storage-reattach.sh`: these are the ones created by the bootstrap with a known UUID disk; a manually mounted Node, in a system container, does not have it, and its disk recovery is handled by the hypervisor). In older versions the container name was hardcoded in `docker restart`: without replacing the script, udev reattachment will start failing silently after the rename.
   `curl -fsSL http://<cp>/node/seedgrid-storage-reattach.sh -o /usr/local/bin/seedgrid-storage-reattach.sh && chmod 0755 /usr/local/bin/seedgrid-storage-reattach.sh`
7. **Restart the Agent** (`systemctl restart seedgrid-agent`: it rereads the env and, on the first heartbeat, reapplies to the Engine the configuration delivered by the Control Plane — ADR-039) and **check**:
   - `docker inspect seedgrid-engine -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Propagation}}{{end}}{{end}}'` → `rslave`;
   - `docker ps --format '{{.Names}}'` shows `seedgrid-engine` and **not** the old name;
   - in the Portal (or in `GET /api/v1/nodes`), the Node goes back to ONLINE and `engine_started_at` is populated.

## 50.5 Migrate Nodes to Control Plane configuration (ADR-039, #144)

**What changes.** The behavior parameters for the Agent and Engine are no longer determined by the `.env` on each machine and now come from the Control Plane (**cluster** value in Settings → Agent / Engine, with **per-Node override**). The `.env` keeps only identity and machine settings (§45); behavior variables become a **fallback**.

**The danger, and why the procedure has an order.** A new Agent starts obeying the Control Plane. A Node whose `.env` had a **different** value from the cluster would change behavior on the first heartbeat — this was exactly the error on 05/09/2026, when `MAX_ACTIVE_DOWNLOADS=10` "to align" overloaded node-04 (Atom). Therefore, the Node override must exist **before** its Agent is updated.

**Inventory (reading of `/etc/seedgrid-agent.env` from the four Nodes, 03/10/2026; no secrets):**

| Variable | node-01 | node-02 | node-03 | node-04 | Cluster default |
|---|---|---|---|---|---|
| `SEEDGRID_AGENT_HEARTBEAT_INTERVAL` | `10s` | `10s` | `10s` | `10s` | `10s` |
| `SEEDGRID_AGENT_COMMAND_POLL_INTERVAL` | `5s` | `5s` | `5s` | `5s` | `5s` |
| `SEEDGRID_LOG_LEVEL` | `info` | `info` | `info` | `info` | `info` |
| `SEEDGRID_ENGINE_MAX_ACTIVE_DOWNLOADS` | `10` | `10` | `10` | **`3`** | `10` |
| `QUEUEING_ENABLED`, `MAX_ACTIVE_UPLOADS`, `MAX_ACTIVE_TORRENTS`, `MAX_ACTIVE_CHECKING`, `DONT_COUNT_SLOW`, `SLOW_DL/UL_THRESHOLD`, `SLOW_INACTIVE_TIMER` | *(not set — code defaults apply)* | ditto | ditto | ditto | = code defaults (`true`, `10000`, `10000`, `1`, `false`, `1`, `1`, `300`) |

The cluster defaults in the catalog were chosen to **match what most use**. Result: **only node-04 needs an override** — `engine.max_active_downloads = 3`. The other three follow the cluster without changing anything. (Take note: `SEEDGRID_ENGINE_CONTAINER=seedgrid-qbittorrent` remains in all four `.env` files — it is identity/machine and **does not** change with this migration.)

**Procedure — Control Plane first, then Agents one by one (ADR-027):**

1. **Control Plane deployment** (done by CI). Migration `00044` creates `node_settings` and the version columns (applied on controller boot, after the tracker profile `00043`). Nothing changes on the Nodes: current Agents do not know about `agent_config` and ignore the new field in the heartbeat response.
2. **Check the cluster values** in Settings → Agent / Engine: they must be the defaults from the table above (no editing required).
3. **Create the override for node-04 — before updating its Agent.** Portal → Node `seedgrid-node-04` → *This Node Configuration* → **Custom Value** for *Simultaneous Downloads per Node* → `3` → Save. (Or `PUT /api/v1/nodes/<id>/settings/engine.max_active_downloads` with `{"value":"3"}`.) Confirm in the *Nodes with Custom Value* list, in Settings.
4. **Update one Agent at a time** ("↑ version" button, ADR-027), starting with an i5 (node-01) and leaving node-04 for last. After each one, check: the badge in the *This Node Configuration* section turns **Up to date**; the Agent log says `configuração do Agent aplicada` with `origem: control-plane`; and, on node-04, the Engine continues with **3** downloads (effective `engine.max_active_downloads` = 3, origin *this Node's value*).
5. **Nothing to clean up in the `.env` files.** Behavior variables remain there as a fallback (Control Plane down on boot). If you want to remove the ones that match the cluster, remove them — the Node remains the same.

**Safety net: seeding.** If step 3 is forgotten, the new Agent on node-04 does not adopt the cluster value: the Control Plane requests the `.env` report (`report_local`) and the Agent **applies nothing** before reporting; with `MAX_ACTIVE_DOWNLOADS=3` in the `.env` and the cluster value still at the default, the Control Plane **seeds** the `3` override (Event `NODE_CONFIG_SEEDED`, visible in Alerts & Events) and only then does the Agent apply it. The conditions (and why the recommended procedure remains manual override) are in `25 - Business Rules.md` §3.1.1b. **Seeding does not take effect** if the cluster value for the key has already been edited by an operator — in this case, the Node follows the cluster, which is the documented precedence: check before updating.

**Revert.** Restore the previous Agent binary (`seedgrid-agent.prev`, ADR-027): the old Agent goes back to reading only the `.env`, which was not touched. To undo **only** a Node's custom value, use *Revert to Cluster*. What seeding created is also removed this way — and **does not reappear** (seeding is a one-time operation per Node).

**If the Control Plane is reverted** with new Agents already up to date, one heartbeat fails (the only one that still carries `config_version`) and subsequent ones already go out without new fields; the Node does not end up going offline (90 s limit). The Agents keep the last applied value.

## 50.6 A machine that already runs qBittorrent (ADR-043, #32)
A Node can join with the qBittorrent the machine already runs, keeping its port forward, folders, settings and torrents. The same command, with `ENGINE=existing` and the WebUI password:

```sh
curl -fsSL http://<cp>/node/bootstrap-node.sh | sudo CP_URL=http://<cp> JOIN_TOKEN=<token> \
  ENGINE=existing QBT_PASSWORD=<WebUI password> sh
```

`QBT_URL` (default `http://127.0.0.1:8080`) and `QBT_USERNAME` (default `admin`) point at the WebUI; `QBT_CONTAINER` chooses the container when Docker runs more than one qBittorrent; `ENGINE_PATH_MAP` overrides the path mapping read from Docker.

**Before enrolling**, the script checks that the WebUI accepts the credentials and that the version is **4.5 or later**, and reads the Engine's listening port and download folder. If any of this fails, or the download folder does not exist on the host, it stops with "Nothing was changed". If qBittorrent runs in a container, the script finds it (the only running container whose image is a qBittorrent) and reads its mounts as the path mapping, leaving `/config` out.

**What it does:** the enroll reserves the **Engine's own port** (§50.1 step 3 assigns the next free one only on the normal path) and warns when another Node already has it; it installs the Agent with `SEEDGRID_ENGINE_ADOPTED=true`, the mapping, the storage (the host folder of the Engine's `save_path`) and `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false`. For host metrics it uses whatever already answers on port 9100; otherwise, with Docker present, it starts only the node-exporter, from `/opt/seedgrid-node/node-exporter.yml`.

**What it does not do:** install Docker or the Engine, change the WebUI password, mount disks or install the disk recovery, or touch the network (§50.2: no IPv6 suffix, no privacy extensions — `--network-only` remains available). `--storage-only` refuses to run on such a Node.

**In the Portal:** the Node joins in **Manual** — its torrents appear as they are, and admission does not pause, start or rotate any of them until it is moved to SG. If the Cluster was in Full auto, it moves to Flex (Event `NODE_ADOPTED_MANUAL`); the other Nodes stay SG. The Node's `engine.*` settings are not applied, the Engine is not restarted from the Portal, and on qBittorrent 4.x the Node cannot publish to a tracker (5.0 needed).
