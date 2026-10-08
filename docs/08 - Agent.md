# **SeedGrid**
# **Agent Specification**
Version: 1.0
Status: Draft
Document: 08 - Agent.md
# **1. Objective**
The SeedGrid Agent is the component responsible for representing a Node to the Cluster.
Every Node participating in SeedGrid must have exactly one running instance of the Agent.
The Agent is responsible for abstracting the operating system, the hardware, and the Torrent Engine, providing the Control Plane with a single, standardized interface.
# **2. Philosophy**
The Agent represents the Node.
The Control Plane must never directly access:
- Operating System
- Docker
- Engine
- Disks
- Network interfaces
- SMART
- Files
All interaction must occur through the Agent.
# **3. Responsibilities**
The Agent must be responsible for:
Registering the Node.
Authenticating the Node.
Executing commands.
Collecting inventory.
Collecting metrics.
Translating Control Plane commands.
Translating Torrent Engine information.
Monitoring local health.
Executing administrative operations.
Managing local configuration.
Publishing events.
The Agent must NOT be responsible for:
Making Scheduler decisions.
Persisting Cluster data.
Performing user authentication.
Sending notifications.
Managing other Nodes.
# **4. Architecture**
Control Plane
│
──────────────── HTTPS ─────────────────
SeedGrid Agent
┌───────────────────────────────┐
│                               │
│ Registration Manager          │
│ Inventory Manager             │
│ Torrent Manager               │
│ Storage Manager               │
│ Metrics Collector             │
│ Health Manager                │
│ Docker Manager                │
│ Configuration Manager         │
│ Command Executor              │
│ Event Publisher               │
└───────────────────────────────┘
│
Torrent Engine
│
Operating System
# **5. Components**
The Agent is composed of the following modules.
# **Registration Manager**
Responsible for Node registration.
Functions
Register new Node.
Renew registration.
Receive identity.
Store credentials.
Send heartbeat.
# **Inventory Manager**
Responsible for inventory.
Hardware.
Operating System.
CPU.
Memory.
Disks.
USB.
Network.
Docker.
Containers.
Versions.
# **Torrent Manager**
Responsible for communication with the Engine.
Add Torrent.
Remove Torrent.
Pause.
Resume.
Recheck.
Query state.
Query statistics.
Query files.
# **Storage Manager**
Responsible for disks.
Inventory.
SMART.
Mount Points.
Filesystem.
Space.
Permissions.
# **Metrics Collector**
Responsible for collecting:
CPU.
RAM.
Disk.
Network.
Engine.
Docker.
Temperature.
# **Health Manager**
Responsible for calculating local health.
Detecting problems.
Publishing events.
Monitoring services.
# **Docker Manager**
Responsible for knowing the Docker environment.
Containers.
Images.
Volumes.
Networks.
State.
# **Configuration Manager**
Responsible for:
Loading configuration.
Persisting local configuration.
Receiving configuration from the Control Plane.
Validating configuration.
# **Command Executor**
Responsible for executing commands sent by the Control Plane.
All commands must pass through this module.
# **Event Publisher**
Responsible for generating local events.
Examples
Engine started.
Disk disconnected.
SMART failed.
High temperature.
Container restarted.
# **6. Lifecycle**
Upon startup, the Agent must execute:
1
Load configuration.
↓
2
Validate environment.
↓
3
Discover hardware.
↓
4
Initialize modules.
↓
5
Connect to the Engine.
↓
6
Register with the Control Plane.
↓
7
Send inventory.
↓
8
Become operational.
# **7. Initialization**
If any module fails during initialization:
The Agent must log the error.
Continue initializing independent modules.
Enter degraded mode when possible.
Avoid complete shutdown whenever possible.
# **8. Shutdown**
Upon shutdown:
Cancel pending operations.
Terminate communication with the Engine.
Send final event.
Persist configuration.
Terminate internal services.
# **9. Configuration**
The Agent will have only one configuration file.
Responsibilities:
Identity.
Control Plane.
Credentials.
Local parameters.
Logging.
Cache.
No operational configuration will be distributed across multiple files.
# **10. Update**
The Agent must be updatable independently of the Engine.
The update must not alter local configurations.
The update must preserve the Node's identity.
# **11. Architectural Goals**
Low memory consumption.
Low CPU consumption.
Fast startup.
Low disk usage.
Low network traffic.
Ease of update.
Low operating system dependency.
# **12. Dependencies**
The Agent will depend only on:
Operating System.
Docker.
Torrent Engine.
Control Plane.
No other SeedGrid component will be required for its local execution.
# **13. Fundamental Principle**
The Agent fully represents the Node.
All access to the Node must occur through it.
The Agent is the only local authority recognized by the Control Plane.
This separation must be preserved throughout the project's evolution.
The Control Plane decides.
The Agent executes.
The Agent never alters Cluster policies.
The Agent never chooses Nodes.
The Agent never makes business decisions.
# **14. Communication**
The Agent will be the single point of communication between the Node and the Control Plane.
No other service must receive commands directly from the Control Plane.
Every received command must pass through the Agent.
# **15. Communication with the Control Plane**
The Agent must maintain secure communication using HTTPS.
All communication will be initiated by the Agent.
The Control Plane will never open connections directly to the Node.
This approach allows operation even in environments where Nodes are behind NAT or restrictive firewalls, provided they can establish outbound connections.
# **16. Registration**
Upon starting for the first time, the Agent must:
- generate its local identity;
- register with the Control Plane;
- receive registration confirmation;
- persist its identity.
Once registered, the same Node must reuse its identity in future restarts.
# **17. Heartbeat**
The Heartbeat represents the confirmation that the Agent remains operational.
The Heartbeat must contain only summarized information.
Examples:
- Agent version;
- Engine version;
- uptime;
- current state;
- Health Score;
- inventory version.
The Heartbeat must not transport full inventories or torrent lists.

**The Heartbeat is also the configuration channel (ADR-039).** The response brings `agent_config` — the
effective behavior values for this Node, with precedence already resolved by the Control Plane
(Node override > cluster value) — and the next heartbeat returns `config_version`: the version
that the Agent applied **successfully**. The Control Plane sends the entire desired state with each
heartbeat; losing one costs nothing. New fields in the body are only sent after the Control
Plane proves, in a previous response, that it understands them (see `13 - API.md` §8.1).
# **18. Inventory**
The inventory must be sent:
- during the first registration;
- when requested by the Control Plane;
- when the Agent detects relevant changes.
Examples of changes:
- new disk;
- disk removal;
- memory change;
- operating system update;
- network interface change;
- Engine update.
# **19. Local Configuration**
The Agent must keep only the configurations necessary for its operation.
Examples:
Control Plane
Credentials
Logging
Cache
Local directories
Log levels
Collection intervals
All operational Cluster configuration belongs to the Control Plane.

**Implemented by ADR-039 (#144).** What is **identity/machine** remains only in the `.env` (Control
Plane URL, token, Engine credential, BT port, `save_path`, Engine container name, migration
port, data directory). What is **behavior** — heartbeat interval, command
polling interval, log level, and Engine queue (`max_active_*`, anti-stall rotation) — comes from the
Control Plane, and its env is a **fallback** (only valid when the CP sends nothing: CP prior to ADR-039,
CP down, or boot before the first heartbeat). Precedence: Node override > cluster value >
env > code default.

How the Agent applies it (`internal/remotecfg`), **hot and without restarting**:
- **Engine queue** → `setPreferences` only for queue keys (the BT port and `save_path` belong to the machine and are only set on the process's first application);
- **Heartbeat interval** and **command polling interval** → recreate the ticker;
- **Log level** → `slog.LevelVar`.

Agent rules: (1) **validates what it receives** — an interval ≤ 0 would crash a ticker, a download limit ≤ 0 is not "unlimited" in qBittorrent — and rejects the bad value, keeping the previous one, without declaring the version as applied; an unknown key (newer CP) is ignored; (2) **repeats the application to the Engine on every heartbeat** until it succeeds (the Engine might be down); (3) **on boot, holds the application** while the Control Plane requests the env report (seeding overrides, ADR-039 §7) — the cluster value never reaches a Node that had its own value — and only applies the `.env` if the Control Plane does not respond within three heartbeats; (4) **never sends a new field to a CP that does not understand it** (the heartbeat body is decoded with `DisallowUnknownFields`). What requires a restart is what remained in the `.env`.
# **20. Local Cache**
The Agent may maintain a local cache to reduce calls to the Engine.
The cache must contain only transient information.
Examples:
latest inventory;
latest statistics;
known state of torrents;
latest events.
Loss of the cache must never prevent Node operation.
# **21. Torrent Manager**
The Torrent Manager is responsible for all interactions with the Torrent Engine.
Its responsibilities include:
Add torrents.
Remove torrents.
Pause torrents.
Resume torrents.
Run recheck.
Query files.
Query statistics.
Query trackers.
Query priorities.
Query progress.
The rest of the Agent must never communicate directly with the Engine.
# **22. Storage Manager**
Responsible for local storage.
Functions:
Discover disks.
Discover partitions.
Identify mount points.
Query SMART.
Query available space.
Query file system.
Validate permissions.
Detect physical changes.
Detect unexpected removals.
# **23. Inventory Manager**
Responsible for the full Node inventory.
Inventory:
Hardware.
CPU.
Memory.
Interfaces.
Disks.
Docker.
Containers.
Operating System.
Kernel.
Versions.
USB.
Architecture.
The goal is to faithfully represent the environment where the Agent is running.
# **24. Metrics Collector**
Responsible for collecting instantaneous metrics.
Examples:
CPU.
RAM.
Swap.
Disk.
Network.
Temperature.
Engine.
Docker.
The collected metrics will be used by the Health Manager and exported to the monitoring system.
The Agent must not keep a history of metrics.
# **25. Health Manager**
Responsible for continuously evaluating the Node's health.
Evaluated criteria:
Engine availability.
Available space.
Temperature.
SMART.
System load.
Essential services.
Connectivity.
Heartbeat.
If problems are identified, the Health Manager must generate local events for the Event Publisher.
# **26. Docker Manager**
Responsible for understanding the Node's Docker environment.
Functions:
List containers.
Query state.
Query images.
Query volumes.
Query networks.
Check container health.
The Docker Manager must never modify containers that do not belong to SeedGrid without an explicit request from the operator.
# **27. Command Executor**
Every command received from the Control Plane must be executed exclusively by this module.
Examples:
Add torrent.
Pause torrent.
Resume torrent.
Update inventory.
Execute backup.
Update configuration.
Execute recheck.
Update Agent.
Extract media information (`mediainfo`, ADR-020).
Extract video screenshots (`extract_screenshots`, via `ffmpeg`/`ffprobe`, ADR-026).
No other module will execute commands directly.
# **28. Event Publisher**
Responsible for transforming local occurrences into Cluster events.
Examples:
Engine started.
Engine stopped.
Disk connected.
Disk removed.
Critical SMART.
High temperature.
Communication error.
Update completed.
All events will be sent to the Control Plane.
# **29. Fault Handling**
The Agent must be resilient.
The temporary unavailability of one module must not interrupt the others.
Examples:
If Docker is unavailable, the rest of the Agent continues to function.
If SMART fails, the inventory remains available.
If the Engine is stopped, the Node remains registered.
Whenever possible, faults must degrade functionality, never completely interrupt the Agent.
# **30. Logging**
All modules must generate structured logs.
Each entry must identify:
- module;
- severity;
- time;
- operation;
- result.
The log level must be configurable.
# **31. Principles**
The Agent never makes business decisions.
The Agent never chooses Nodes.
The Agent never changes Cluster policies.
The Agent executes.
The Control Plane decides.
This separation must be preserved throughout the entire evolution of the project.
# **32. Relationship with the Torrent Engine**
The Torrent Engine is considered an external component to the Agent.
The Agent will be responsible for adapting the Engine's interface to the SeedGrid data and operations model.
The rest of the Cluster must never depend directly on Engine-specific APIs.
# **33. Adapters (Engine Adapters)**
The Agent must have an adaptation layer for the Engine.
Each supported Engine must have its own adapter.
Future examples:
- qBittorrent Adapter
- Transmission Adapter
- Deluge Adapter
All must expose the same internal interface to the rest of the Agent.
# **34. Operating System Independence**
The Agent must minimize operating system-specific dependencies.
Whenever possible, use mechanisms compatible across Linux distributions.
If a feature depends on a distribution-specific resource, this dependency must be isolated in its own module.
# **35. Administrative Operations**
The Agent will be responsible only for the execution of operations.
Examples:
- inventory update;
- local configuration update;
- Engine restart;
- system information collection;
- execution of local checks.
The Agent must not execute destructive operations without an explicit command from the Control Plane.
# **36. Consistency**
Whenever an administrative operation is completed, the Agent must update the Control Plane with the execution result.
If the operation fails, the Agent must report:
- executed operation;
- failure reason;
- date and time;
- known impact.
# **37. Reconnection**
If communication with the Control Plane is interrupted:
The Agent must:
- continue running normally;
- keep the Engine operational;
- continue collecting inventory when necessary;
- continue monitoring local health;
- maintain the **last applied configuration value** (ADR-039): a down Control Plane does not undo what is already in effect — only an Agent **restart** starts from `.env`.
When communication is restored:
- resend pending information;
- synchronize state;
- update Heartbeat.
The Node must not restart automatically solely due to loss of connectivity.
# **38. Offline Operation**
During temporary unavailability of the Control Plane:
The Node will continue:
- seeding torrents;
- responding to the BitTorrent protocol;
- running local monitoring.
Administrative operations will depend on the availability of the Control Plane.
# **39. Resource Consumption**
The Agent must be designed for reduced resource consumption.
V1 goals:
Idle CPU below 2%.
Memory below 150 MB.
Minimal disk usage.
Low number of processes.
Low network consumption.
These values represent design goals and may be revised as the platform evolves.
# **40. Security**
The Agent must:
- store only the strictly necessary credentials;
- use encrypted communication;
- validate the Control Plane's identity;
- protect sensitive information in local storage.
No end-user credentials must be stored by the Agent.
# **41. Observability**
The Agent must provide sufficient information for remote diagnostics.
Examples:
- installed version;
- module state;
- runtime;
- last Heartbeat sent;
- inventory version;
- Engine in use;
- Engine state.
This information must be available to the Control Plane.
# **42. Updates**
The Agent update must preserve:
- Node identity;
- local configuration;
- non-critical cache (when applicable).
After a successful update, the Agent must:
- register its new version;
- resend Heartbeat;
- report success to the Control Plane.
# **43. Scalability**
The Agent architecture must allow the inclusion of new modules without significant changes to existing modules.
New modules must integrate using the same configuration, logging, events, and communication mechanisms already established.
# **44. External Dependencies**
The Agent must depend only on components essential to the Node's operation.
External dependencies must be kept to a minimum.
The objective is to simplify installation, updates, and maintenance.
Examples of external binaries required by specific publishing features (optional, best-effort — their absence must not prevent the rest of the Agent from functioning):
- `mediainfo`: extracts technical metadata from the torrent's main media file, used for publishing to UNIT3D trackers (ADR-020).
- `ffmpeg`/`ffprobe`: extract frames (screenshots) from the main media file, also for publishing to UNIT3D trackers (ADR-026). Required on all Nodes, including ARM64 architecture (RPi 3B+).
# **45. Testability**
The architecture must allow independent testing for each module.
The failure of one module must not prevent the validation of the others.
Whenever possible, modules must be decoupled from the Engine through the adaptation layer.
# **46. Evolution**
The Agent architecture must allow future expansions without the need for restructuring.
Examples of possible evolutions:
- support for new Engines;
- new inventory collectors;
- new monitoring mechanisms;
- new administrative operations.
These evolutions must respect the principles defined in this document.
# **47. Architecture Summary**
Control Plane
│
HTTPS / API
│
SeedGrid Agent
│
┌───────────────────────────────────────┐
│ Registration Manager                  │
│ Inventory Manager                     │
│ Torrent Manager                       │
│ Storage Manager                       │
│ Metrics Collector                     │
│ Health Manager                        │
│ Docker Manager                        │
│ Configuration Manager                 │
│ Command Executor                      │
│ Event Publisher                       │
└───────────────────────────────────────┘
│
Engine Adapter
│
Torrent Engine
│
Operating System
# **48. Final Statement**
The Agent is the official representative of the Node to the Cluster.
Its function is to abstract the local environment, standardize communication with the Control Plane, and provide a consistent view of the Node, regardless of the hardware, operating system, or Torrent Engine used.
All interaction between the Cluster and the Node must occur exclusively through the Agent, preserving the decoupling between components and allowing independent evolution of each layer of the architecture.
