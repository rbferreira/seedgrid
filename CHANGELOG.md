# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/). Until 1.0, minor versions may change the API and
the configuration; each release says how to upgrade.

## [Unreleased]

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

[Unreleased]: https://github.com/rbferreira/seedgrid/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/rbferreira/seedgrid/releases/tag/v0.2.0
[0.1.0]: https://github.com/rbferreira/seedgrid/releases/tag/v0.1.0
