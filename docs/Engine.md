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
- rTorrent Adapter (ADR-045);
- Transmission Adapter (future);
- Deluge Adapter (future).
## 6.1 Supported qBittorrent versions (ADR-043)
Every Node runs qBittorrent **5.0 or later**: the Nodes SeedGrid installs run 5.x, and a Node that joins with the qBittorrent it already runs must too. 5.0 is the first with the torrent creator (`torrentcreator/*`), which publishing (ADR-019) needs, and with `torrents/stop`/`torrents/start`, which the adapter uses. The bootstrap refuses a 4.x and asks to update it; a Node that could not publish would be a second kind of Node.
## 6.2 rTorrent (ADR-045)
A Node can run rTorrent **0.9.8 or later**, joined with the rTorrent the machine already runs (`bootstrap-node.sh ENGINE=rtorrent`, Deployment §50.8). SeedGrid does not install it. The adapter speaks rTorrent's XML-RPC, over SCGI (a Unix socket or `host:port`) or through a web server's `/RPC2`, with its own small codec. ruTorrent, if present, keeps working next to it, and SeedGrid does not depend on it.

An rTorrent Node is a Node like any other: same states, settings, publishing, migration and Portal. What rTorrent does not have, the Agent supplies:
- **The download queue.** rTorrent starts every torrent it holds. The Agent applies the `engine.*` settings itself, every few seconds: oldest first, at most so many downloads, uploads and torrents active, and with `dont_count_slow` a slow torrent stops counting. It marks what it stops (`d.custom` key `seedgrid`) and only ever starts what it marked: a torrent the operator or admission stopped stays stopped. With the queue on, a torrent added from a `.torrent` comes in stopped and queued. `max_active_checking` has nothing to limit: rTorrent checks one torrent at a time. The queue acts only once the Control Plane's settings arrived.
- **The torrent creator.** For publishing, the Agent creates the `.torrent` (BitTorrent v1, the piece size libtorrent would choose, private flag and `source` as asked) and loads it pointing at the data; rTorrent checks it and seeds.
- **The `.torrent` export.** The Agent reads it from rTorrent's session folder and removes the keys rTorrent adds (`rtorrent`, `libtorrent_resume`), which gives back the original bytes and info hash.

States come from several fields: `d.hashing` set is CHECKING; a `d.message` from anything but a tracker (a storage error, a failed hash check) is ERROR; stopped or paused is PAUSED, except a torrent the Agent's queue stopped, which is QUEUED when incomplete and SEEDING when complete; started is SEEDING when complete and DOWNLOADING otherwise. A tracker's answer is the tracker verdict, not the torrent's state, as with qBittorrent. rTorrent has no verdict on its own inbound connectability: the Node reports none (unknown, ADR-034) and counts incoming peers from the peer list.

Two version differences the adapter handles: rTorrent 0.16 renamed `network.port_range`/`network.port_random` to `network.listen.port.range`/`network.listen.port.random` (the adapter tries the new name, then the old), and it tells XML-RPC from JSON-RPC by the content type, which the adapter always sends. rTorrent answers a load with 0 even when it drops the `.torrent` (0.9.8 drops one without a tracker), so the adapter confirms each `.torrent` it loads.
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
