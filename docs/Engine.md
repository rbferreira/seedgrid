# **SeedGrid**
# **Torrent Engine Specification**
Version: 1.0
Status: Draft
Document: Engine.md
# **1. Purpose**
This document defines the role of the Torrent Engine in the SeedGrid architecture and the adaptation contract (Engine Adapter) that allows the platform to remain independent of any specific BitTorrent client.
# **2. Definition**
The Torrent Engine is the application responsible for running the BitTorrent protocol on a Seed Node.
Its responsibility is limited to torrent-related operations.
In version 1.0 the official Engine is qBittorrent.
# **3. Philosophy (Engine Agnostic)**
SeedGrid does not depend on any specific BitTorrent client.
The Engine is an implementation detail of the Seed Node.
No Cluster component (Control Plane, Scheduler, Portal, API) must depend directly on Engine-specific APIs.
All Engine dependencies remain isolated in the Agent, through the adaptation layer.
# **4. Responsibilities**
The Torrent Engine must:
- run the BitTorrent protocol;
- download and seed torrents;
- maintain the local state of torrents;
- expose operations (add, remove, pause, resume, recheck) and statistics to the Agent.
The Torrent Engine must NOT:
- communicate directly with the Control Plane;
- make Scheduler decisions;
- be accessed directly by the Portal or the API.
# **5. Relationship with the Agent**
The Torrent Engine is considered an external component to the Agent.
All interaction between SeedGrid and the Engine will occur exclusively through the Agent's Torrent Manager (see Agent.md §21, §32).
The rest of the Cluster will never communicate directly with the Engine.
Dependency on the Engine's WebUI for normal operations is considered an anti-pattern (Architecture.md §21).
# **6. Engine Adapter**
The Agent must have an adaptation layer for the Engine.
Each supported Engine must have its own adapter, exposing the same internal interface to the rest of the Agent.
Examples:
- qBittorrent Adapter (V1);
- Transmission Adapter (future);
- Deluge Adapter (future).
## 6.1 Supported qBittorrent versions (ADR-043)
The Nodes SeedGrid installs run qBittorrent 5.x. A Node that joins with the qBittorrent it already runs needs **4.5 or later**, the first with `torrents/export`, which migration and the `.torrent` backfill use. On 4.x the adapter falls back from `torrents/stop`/`torrents/start` (5.0) to `torrents/pause`/`torrents/resume` the first time the Engine answers 404, and keeps using them. Publishing from a Node (ADR-019) needs **5.0**, the first with the torrent creator (`torrentcreator/*`).
# **7. Adapter Contract**
Every Engine Adapter must expose at least the following operations to the Torrent Manager:
- add torrent (.torrent file or magnet). When the source is a `.torrent` file, the adapter must pass the **original bytes** to the Engine (do not reduce to a magnet/infohash): this preserves the metadata — preventing the Engine from needing to fetch it from the network (`metaDL` state) — and honors the torrent's `private` flag, which is essential for private trackers, where there is no DHT to recover metadata;
- remove torrent (with or without data);
- pause torrent;
- resume torrent;
- perform recheck;
- query state, files, trackers, and statistics;
- report Engine availability.
The adapter is responsible for translating this contract to the Engine's specific API.
# **8. State Mapping**
The adapter must map Engine-specific states to the official SeedGrid states (Database.md §36):
QUEUED, DOWNLOADING, SEEDING, PAUSED, CHECKING, MOVING, ERROR, ARCHIVED, DELETED, UNKNOWN.
The SeedGrid data model must never be coupled to the Engine's data model.
# **9. Independence**
Replacing the Engine (e.g., qBittorrent → Transmission → Deluge) must not require changes to the Control Plane, the Scheduler, the Portal, or the API.
Only a new adapter must be implemented, respecting the contract defined in this document.
# **10. Relationship with Other Documents**
This document complements:
- Vision.md (§5.2 Engine Agnostic);
- Architecture.md;
- Agent.md (Torrent Manager and Engine Adapters);
- Database.md (Torrent domain and states);
- Scheduler.md.
Whenever there is any doubt about the role of the Torrent Engine or the adaptation contract, this document must be considered the official reference.
# **11. Final Statement**
The Torrent Engine provides the BitTorrent capability of the Seed Node, remaining a replaceable implementation detail.
By concentrating all Engine dependencies in the Agent's adaptation layer, SeedGrid preserves its Engine independence principle and keeps the architecture decoupled and evolutionary.
