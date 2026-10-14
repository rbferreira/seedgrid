# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/). Until 1.0, minor versions may change the API and
the configuration; each release says how to upgrade.

## [Unreleased]

### Added
- Portal: **Downloaded** shows when each torrent was added, sorts by it and filters by period
  (last 24 hours / 7 days / 30 days). The screen now opens newest first.

### Fixed
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

[Unreleased]: https://github.com/rbferreira/seedgrid/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rbferreira/seedgrid/releases/tag/v0.1.0
