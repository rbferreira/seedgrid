# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/). Until 1.0, minor versions may change the API and
the configuration; each release says how to upgrade.

## [Unreleased]

### Added

- **Update every Agent with one click (#86, ADR-046).** The Dashboard's Seed Nodes panel has an
  **Update all** button when two or more online Nodes run an older Agent. One confirmation; one
  Node goes first, and the others only after it is back on the new version. If it does not come
  back, the others are left alone.

### Changed

- **Trackers belong to the cluster; the Administrator manages them (#19, ADR-047).** Until now
  each tracker was visible only to whoever registered it, and registration lived in Settings,
  which only the Administrator opens. A Publisher never had a tracker to publish to.
  - Everyone who reads trackers now sees the same list.
  - The Publisher publishes and seeds with any tracker, but no longer creates, edits or removes
    them: migration 00050 takes `trackers:write` away from it.
  - Tracker names are unique across the cluster, and there is one default. On upgrade a
    duplicate name gets its owner's username appended, and the oldest default stays.
  - Where the Portal needs a tracker that is missing, whoever cannot manage trackers is told to
    ask an administrator.

## [0.4.0] — 2026-10-24

A Node can run rTorrent, and a Node can be installed without root, on a shared seedbox.

### Upgrading
- **Control Plane:** with the Compose file, download the `compose.yaml` of this release (or set
  `SEEDGRID_VERSION=0.4.0`) and `docker compose up -d`. With `bootstrap-controller.sh`, update as
  before (`git pull && git fetch --tags && sh scripts/bootstrap-controller.sh`). No migrations.
- **Nodes:** update the Agent from the Portal. Existing Nodes keep working as they are; the new
  Agent is what an rTorrent Node and a user install need.

### Added

- **A Node can run rTorrent (#31, ADR-045).** `bootstrap-node.sh` with `ENGINE=rtorrent` and
  `RTORRENT_URL` (its XML-RPC, over SCGI or `/RPC2`) joins a machine that runs rTorrent 0.9.8 or
  later the way `ENGINE=existing` joins a qBittorrent: nothing in rTorrent changes, the Node starts
  in Manual, and ruTorrent keeps working next to it. Once in the Cluster it is a Node like any
  other. The Agent supplies what rTorrent lacks: the download queue (the same `engine.*`
  settings, applied by the Agent; it starts off, as rTorrent ran), the `.torrent` creator for
  publishing, and the `.torrent` export from rTorrent's session folder.
- **Install without root, on a shared seedbox (ADR-045).** `ENGINE=existing` or `ENGINE=rtorrent`
  run without sudo install the Agent for that user, in the home folder, kept running by a systemd
  user service that lingers or else by cron; updating it from the Portal still works.
  `ENGINE_RESTART_CMD` gives the command that restarts the Engine when the provider owns it.

## [0.3.0] — 2026-10-23

A machine that already runs qBittorrent can join the Cluster without giving it up, and a removed
Node that joins again is recognized.

### Upgrading
- **Control Plane:** with the Compose file, download the `compose.yaml` of this release (or set
  `SEEDGRID_VERSION=0.3.0`) and `docker compose up -d`. With `bootstrap-controller.sh`, update as
  before (`git pull && git fetch --tags && sh scripts/bootstrap-controller.sh`). Migrations
  `00048` and `00049` run on start.
- **Nodes:** update the Agent from the Portal **after** the Control Plane. The download fix (#61) and
  the machine identity (#62) are in the Agent.
- **qBittorrent 4.x is no longer supported.** The Nodes SeedGrid installs already run 5.x; nothing
  changes for them.

### Added

- **A Node can join with the qBittorrent it already runs (#32, #64, ADR-043).** `bootstrap-node.sh`
  with `ENGINE=existing` and the WebUI password keeps the machine's qBittorrent instead of installing
  another one, and joining changes nothing in it. Its port forward, folders, queue settings and
  torrents all stay:
  - the bootstrap reserves the Engine's own port;
  - it reads the container's mounts to translate paths;
  - the Agent seeds the queue settings as the Node's settings in SeedGrid.

  The bootstrap installs only the Agent: no Docker, disk or network changes. The Node joins in
  Manual: review its settings, move it to SG, and from then on it is managed like any Node, restart
  from the Portal included. A Cluster in Full auto moves to Flex for it.

  Before enrolling, the bootstrap checks:
  - the WebUI and the version, which must be qBittorrent **5.0 or later**;
  - what restarts qBittorrent. It sets the container's restart policy or a systemd drop-in when
    needed, and refuses a qBittorrent run from a session.

  It also warns that UPnP is turned off, as on every Node. See the deployment guide, §50.6.

### Fixed

- **A removed Node that joins again is no longer invisible (#62, ADR-044).** The registration
  landed in the removed row, which no list showed. Now the hostname is unique among live Nodes
  only, and the Control Plane tells the machine by its `machine-id` (or the Node id the Agent kept):
  - **the same machine, through the bootstrap:** it comes back as itself, with its history, mode,
    settings, the torrents removed with it and, when free, its old port;
  - **the same machine's Agent retrying on its own:** it is refused, so a removal sticks;
  - **a different machine:** it joins as a new Node.

  Update the Agents after the Control Plane: an older Agent does not send its machine.

- **Downloading from the Portal no longer stops the torrent (#61).** The download reused the
  migration's authorization, which stops the torrent on its Node before serving it, and nothing
  started it again: a seeding torrent stayed paused. The download now only reads the files. The
  fix is in the Agent: update the Nodes' Agents from the Portal after updating the Control Plane.

## [0.2.0] — 2026-10-21

The Control Plane can now be installed from published images with a single Compose file — see the
Quick start in the README.

### Upgrading
- **Installed with `bootstrap-controller.sh`:** update as before (`git pull && git fetch --tags &&
  sh scripts/bootstrap-controller.sh`). The host's node-exporter moves off `172.18.0.1` and the
  bootstrap restarts Prometheus and Alertmanager so they pick up the new configuration.
- **Moving to the Compose file:** copy `deploy/control-plane/.env` next to `compose.yaml`, run
  `docker compose down` in `deploy/control-plane`, `deploy/observability` and `deploy/portal`, then
  `docker compose up -d`. The database, the metrics and the tokens are kept (Deployment §43).
- **Nodes:** update the Agent from the Portal, as usual.

### Added
- `deploy/compose.yaml`: the whole Control Plane from the published images in one file, with only
  `SEEDGRID_ADMIN_PASSWORD` to set. The other secrets are generated on the first start into the
  `seedgrid-secrets` volume; only the Portal publishes a port (`SEEDGRID_HTTP_PORT`, default `80`). An
  install made by `bootstrap-controller.sh` can move to it and keep its database and metrics (#30).
- Release images: every `vX.Y.Z` tag publishes the controller and Portal images to GHCR for
  `amd64` and `arm64` (`ghcr.io/rbferreira/seedgrid-controller` and `-portal`, tagged `X.Y.Z`, `X.Y`
  and `latest`). The Portal image carries the Agent of the same version for onboarding and
  self-update (#30).
- Control Plane: the secret variables (`SEEDGRID_DATABASE_URL`, `SEEDGRID_AGENT_REGISTRATION_TOKEN`,
  `SEEDGRID_INTEGRATION_TOKEN`, `SEEDGRID_ADMIN_PASSWORD`) also accept the `_FILE` form, reading the
  value from a file (#30).
- Portal: **Downloaded** shows when each torrent was added, sorts by it and filters by period
  (last 24 hours / 7 days / 30 days). The screen now opens newest first.
- Portal: the theme can follow the operating system — a light / system / dark switch in the
  sidebar and the mobile header. **System** is now the default for anyone who never chose; a
  saved light or dark choice is kept.
- Portal: the Dashboard **Admission** switch and the per-Node **SG / Manual** switch use the same
  pill, with the knob amber while a Node is in Manual.
- Portal: form controls follow the same pill language — text inputs, selects and checkboxes on the
  filter screens (Dashboard, Downloaded, Analytics, Alerts), the chart range and alert severity as
  pill switches, and the Feed and Audit filters as chips.
- Portal: forms (users, login, profile, settings, Node configuration, the Node dialogs) use the
  same fields, and every action button is rounded to match.
- Portal: the tracker editors, Publish, Add Torrent and Explore follow too — every form control in
  the Portal now uses the same fields.

### Fixed
- Portal: the Dashboard's admission switch (and the per-Node SG / Manual switch) no longer shows the
  old mode for a moment after a change, when a timer refresh that started before the change
  answered after it (#34).
- Control Plane host: node-exporter no longer listens on `172.18.0.1`, an address that only existed when
  Docker happened to give that range to `seedgrid-core`; it joins the network and Prometheus reaches it
  by name. The bootstrap now restarts Prometheus and Alertmanager on every update, so they read the
  new configuration (#30).
- Control Plane: building the controller with `docker compose up --build` gave it a different user
  (UID 10001) from the one the bootstrap gives the Prometheus targets volume to (65532), so on that
  path it could not write the Nodes' scrape targets. Both Dockerfiles now run it as 65532.
- Portal: the sidebar language list no longer truncates in English, and scrollbars follow the
  theme (thin, transparent track) instead of the browser's wide grey bar.
- Audit: requesting an Agent update and seeding from a tracker say which Node, versions, torrent
  and tracker; the API path moved from every entry to its tooltip.
- Portal: the language selector no longer spills out of the sidebar; it sits on its own row
  under the theme switch.
- Portal: **Catalog** and **Add Torrent** sent you to Profile to register a tracker; they now
  point to Settings › Trackers, where trackers are registered.
- Portal and Control Plane: user-facing text no longer cites internal design records (ADRs)
  or issue numbers — Users, Dashboard, Health & Metrics, Publish, trackers and most setting
  descriptions did. A catalog test now rejects them.

## [0.1.0] — 2026-10-08

SeedGrid orchestrates a BitTorrent seeding infrastructure: a central **Control Plane**
coordinates several **Seed Nodes**, each running a Torrent Engine (qBittorrent) and an
**Agent**; the operator manages one logical cluster from a web **Portal**.

### Cluster and Nodes
- Zero-touch Node onboarding: one command on the new machine, with a join token issued by the
  Portal; the Control Plane assigns the BitTorrent port and the Node registers itself.
- Heartbeat, inventory and per-Node metrics (disk, I/O, network, peers); Node nicknames;
  storage health with a declared dedicated-disk or system-disk mode.
- Agent self-update orchestrated from the Portal, with a smoke test before switching binaries.
- Deterministic IPv6 suffix per Node (`::5eed:<N>`), so firewall rules survive prefix changes.
- Durable command queue (the Agent polls it), executed in order per Node.

### Torrents
- Add by magnet or `.torrent` upload; a disk-aware Scheduler places each torrent on a Node that
  can hold it with a safety margin.
- Admission control that shares the tracker's download slots across the cluster, rotating slow
  and dead downloads; manual or automatic mode per Node and per cluster.
- Migration between Nodes with verification at the destination, cancellation and draining.
- Per-torrent time series, rankings and duplicate detection (Analytics).

### Publishing
- Publishing to UNIT3D-family trackers: release-name parsing, metadata from TMDB, IGDB,
  Open Library and Google Books, screenshots, and an upload description composed per site, in
  the site's own language (English or Brazilian Portuguese).
- Shareable **site profiles** (taxonomy, description layout, image host): discover them from
  the site, adjust them, export and import them. See [`profiles/`](profiles/), including a
  template for writing one by hand.

### Observability and operations
- Alerts (Node offline, storage, disk full, unreachable Node, IPv6 change, I/O saturation) plus
  Prometheus/Alertmanager thresholds, shown in the Portal; domain events and an orchestration
  activity feed.
- Notifications to Telegram, Discord or a webhook, with filters and a rate cap per channel.
- Audit trail of operator actions; role-based access (Administrator, Publisher, Downloader,
  Viewer).
- Live settings with hot reload, including per-Node values for Agent and Engine tunables.

### Portal and languages
- The Portal is available in **English** and **Brazilian Portuguese**; alerts, events, audit
  entries and API errors are rendered in the reader's language from codes and data
  (ADR-041). Notifications follow the cluster's language setting.
- Documentation in both languages (`docs/` and `docs/pt-BR/`).

### Project
- CI runs formatting, vet, lint and tests for every module, plus policy checks: architecture
  boundaries, commit message format and the code language (comments, messages and identifiers
  in English).

[Unreleased]: https://github.com/rbferreira/seedgrid/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/rbferreira/seedgrid/releases/tag/v0.3.0
[0.2.0]: https://github.com/rbferreira/seedgrid/releases/tag/v0.2.0
[0.1.0]: https://github.com/rbferreira/seedgrid/releases/tag/v0.1.0
