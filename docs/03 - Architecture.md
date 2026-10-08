# **SeedGrid**
# **Software Architecture**
Version: 1.0
Status: Draft
Document: 03 - Architecture.md
# **1. Objective**
This document defines the logical architecture of SeedGrid.
All implementations must respect this architecture.
No component may communicate directly with another if this violates the rules established here.
# **2. Architectural Philosophy**
SeedGrid is a distributed, service-oriented platform.
The system is divided into independent, decoupled, and specialized components.
Each component has a single responsibility.
Each component can evolve independently.
# **3. Principles**
# **Single Responsibility**
Each component has a single responsibility.
Examples:
Portal → Interface
Control Plane → Orchestration
Agent → Local Execution
Engine → BitTorrent
Scheduler → Decision
# **API First**
All communication happens via API.
No component accesses another component's database.
No component accesses another component's internal files.
# **Event Driven**
Every important change generates an Event.
Events must never be silently discarded.
Events must be persisted.
# **Stateless Services**
Whenever possible, components must be stateless.
State must remain in the database.
# **Loose Coupling**
No component knows the internal details of another.
Each component knows only the contracts.
# **Replaceable Components**
Any component must be replaceable.
Examples:
qBittorrent
↓
Deluge
↓
Transmission
↓
Another Engine
Without changing the rest of the architecture.
# **4. General Architecture**
User
│
Web Browser
│
────────────────────────────────────────
Portal
│
──────────────── REST API ──────────────
Control Plane
┌────────────┬────────────┐
│            │            │
│            │            │
Scheduler     Database     Event Bus
│
────────┼────────────────────────────────────────
Agent        Agent        Agent
│            │            │
Torrent Engine Torrent Engine Torrent Engine
│            │            │
Local Storage Local Storage Local Storage
# **5. Layers**
The system is divided into five layers.
# **Layer 1**
Presentation Layer
Responsible for human interaction.
Component:
Portal
# **Layer 2**
Business Layer
Responsible for business rules.
Components:
Control Plane
Scheduler
Authentication
Notifications
# **Layer 3**
Service Layer
Internal services.
Database
Cache
Event Bus
Logs
# **Layer 4**
Execution Layer
Nodes.
Agent.
# **Layer 5**
Infrastructure Layer
Hardware.
Operating System.
Disks.
Network.
Docker.
# **6. Components**
# **Portal**
Responsibility:
Graphical interface.
Never makes decisions.
Never accesses Nodes.
Never accesses the database directly.
Always uses the API.
# **Control Plane**
Responsibility:
Intelligence center.
Receives commands.
Executes rules.
Queries the Scheduler.
Updates the database.
Emits events.
Notifies users.
Never accesses Node disks.
Never runs BitTorrent.
# **Scheduler**
Responsibility:
Choose the best Node.
Never communicates directly with the Portal.
Never communicates directly with the Engine.
Always responds to the Control Plane.
# **Agent**
Responsibility:
Represent the Node.
Execute commands.
Collect metrics.
Collect inventory.
Translate Control Plane calls.
Execute local operations.
# **Torrent Engine**
Responsibility:
Execute the BitTorrent protocol.
Nothing else.
# **7. Communication**
All communication must occur via HTTPS.
Never via shared operating system access.
Allowed flow:
Portal
↓
Control Plane
↓
Agent
↓
Engine
Prohibited flows:
Portal → Agent
Portal → Engine
Portal → Database
Scheduler → Agent
Scheduler → Engine
Engine → Database
Engine → Portal
Agent → Database
# **8. Responsibilities**
# **Portal**
Visualization.
Search.
Configuration.
Authentication.
# **Control Plane**
Orchestration.
Persistence.
Events.
Audit.
# **Scheduler**
Decision making.
Balancing.
Planning.
# **Agent**
Execution.
Inventory.
Monitoring.
Local control.
# **Engine**
BitTorrent.
# **9. Node Model**
Every Node must have:
Agent
Engine
Storage
Network
Health
Capabilities
Labels
State
# **10. Node States**
ONLINE
Receives new torrents.
DRAIN
Does not receive new torrents.
Continues operating.
MAINTENANCE
Executes only administrative operations.
OFFLINE
No communication.
FAILED
Severe failure.
Intervention required.
UNKNOWN
Unknown state.
DISABLED
Administratively disabled.
Does not participate in Scheduler selection.
Note
The canonical list of Node states is the one defined in Database.md §11 (ONLINE, OFFLINE, DRAIN, MAINTENANCE, FAILED, UNKNOWN, DISABLED).
ONLINE, DRAIN, MAINTENANCE, and DISABLED are defined by the operator (Requirements RF-003).
FAILED and UNKNOWN are assigned by the system itself; never chosen manually.
# **11. Heartbeat**
Every Agent must send a heartbeat.
Minimum information:
Timestamp
Version
CPU
RAM
Disk
Temperature
Engine
Status
Uptime
Load
Health Score
# **12. Inventory**
The Agent must send inventory.
Examples:
CPU
RAM
Operating System
Kernel
Docker
Disks
Interfaces
USB
Filesystem
Containers
Versions
# **13. Capabilities**
Each Node has Capabilities.
Examples:
Torrent Engine
SMART
Docker
SSD
UPS
USB3
NVMe
ARM64
x86
GPU
The Scheduler uses this information.
# **14. Labels**
Labels are information defined by the operator.
Examples:
country=br
rack=01
tracker=anime
priority=gold
energy=low
Labels are never changed automatically.
# **15. Health Score**
Every Node has a Health Score.
Value:
0 to 100.
Calculated continuously.
Factors:
CPU
RAM
Temperature
SMART
Network
Heartbeat
Availability
Storage
# **16. Database**
The database belongs exclusively to the Control Plane.
No other component has direct access.
# **17. Event Bus**
Every change generates an event.
Examples:
Torrent created.
Torrent removed.
Node offline.
SMART failed.
Update started.
Events are consumed by:
Alerts.
Dashboard.
Logs.
Audit.
Reports.
# **18. Security**
Every component must have its own identity.
All communication must be authenticated.
No secrets may be stored in code.
# **19. Scalability**
The system must allow:
Add Nodes.
Remove Nodes.
Replace Hardware.
Replace Engine.
Replace Operating System.
Without architectural changes.
# **20. Dependencies**
The Portal depends only on the API.
The Control Plane depends on the database.
The Agent depends only on the Engine.
The Scheduler depends only on the database.
The Engine depends only on the BitTorrent protocol.
# **21. Anti-Patterns**
The following are prohibited:
SSH access for normal operations.
Manual execution of commands.
Direct modification of the database.
Local-only configurations.
Dependence on the Engine WebUI.
Direct communication between Nodes.
Disk sharing between Nodes.
# **22. Evolutionary Architecture**
The architecture must allow future implementations of:
Multiple Control Planes.
High availability.
Plugins.
Multiple Engines.
SDK.
CLI.
Mobile App.
Cloud Nodes.
Federated Clusters.
# **23. Fundamental Principle**
Components must not know the internal details of each other.
All interactions occur through well-defined contracts.
Nodes provide capacity.
The Control Plane provides intelligence.
The Portal provides the experience.
Together, they form a single logical Cluster.
# **24. Tracker Profile (ADR-037)**
SeedGrid publishes to a UNIT3D tracker. Site-specific knowledge is **not** code
and is **not** operator configuration: it is a third kind of thing, and the architecture separates it into
three layers with an explicit boundary.
Layer 1 — family
API shape, authentication, and multipart for a family of sites (currently only `unit3d`).
It lives in code (`internal/unit3d`): it is identical across every site in the family.
Layer 2 — site profile
Taxonomy (category/type/resolution IDs) and, in upcoming issues, name format, rules, and
image host policy. It lives in **data** (`tracker_profiles`), exportable: it is what someone with
an account on the site discovers, tweaks, and shares. A new tracker is a **new row, not a new release**.
Layer 3 — operator config
`api_token`, announce with passkey, `anon`, `modq`. It lives in `trackers`. It is **never exported** and
never leaves via the API.
Principles
- The boundary between layers 2 and 3 is **structural**: separate tables, and no profile type has a credential field. An export that leaks the token due to a forgotten filter would first need a field that does not exist.
- The Portal only talks to the API (§6): it **does not know** the taxonomy of any site — it renders what the tracker profile declares. Previously, the samaritano ID lists were hardcoded in the frontend, as if they applied to every UNIT3D site.
- Only the Control Plane accesses the database and only the Control Plane talks to the tracker site (discovery and publishing, using the operator's token). The Agent does not participate: it continues executing and reporting.
- The profile is **untrusted content** — a third-party site catalog, the edit, and the imported file. Everything goes through type and range validation on input, nothing is executable, and no free-text field goes directly into a request.
- **Discovery proposes, the operator applies.** Discovery reads the tracker's own catalog and builds a proposal; applying is a separate, audited action, and the rule that protects what was manually adjusted runs on the server. It only sees what has already been posted — the discovered profile **is not complete by definition**, and nothing in the interface can suggest otherwise.
- **Export/import.** The exchange file is the `Profile` plus an identification envelope, generated by serializing a type that **embeds** the profile and has no credential field — a new profile section is exported without changing the export. The origin of the terms is local and does not travel; the import assigns its own (`imported`). Preview (which never writes) and apply are two calls; only the second is audited.
Data model: `docs/12 - Database.md` §52-A. Contract: `docs/13 - API.md` (profile, taxonomy discovery, export, and import). Decision: ADR-037.
