# **SeedGrid**
# **Control Plane Specification**
Version: 1.0
Status: Draft
Document: 09 - Control Plane.md
# **1. Objective**
The Control Plane is the core of SeedGrid.
It is the component responsible for coordinating all Cluster operations, maintaining the platform's persistent state, and orchestrating communication between the Nodes and the Portal.
No operational decision must be made outside the Control Plane.
# **2. Philosophy**
The Control Plane does not execute torrents.
The Control Plane does not store torrent files.
The Control Plane does not directly access the Nodes.
The Control Plane coordinates.
The Nodes execute.
# **3. Responsibilities**
The Control Plane is responsible for:
- maintaining the Cluster state;
- registering Nodes;
- authenticating Nodes;
- managing users;
- persisting information;
- receiving heartbeats;
- receiving inventories;
- receiving events;
- distributing commands;
- recording audit logs;
- providing APIs to the Portal;
- triggering the Scheduler;
- coordinating administrative operations.
The Control Plane must not:
- execute local operations on the Nodes;
- directly access the Torrent Engine;
- access Node disks;
- depend on specific characteristics of the Nodes' operating system.
# **4. Architecture**
Portal
│
HTTPS / REST
│
┌─────────────────────┐
│   API Gateway        │
└─────────────────────┘
│
┌──────────────────────────────┐
│                              │
│ Cluster Services             │
│                              │
└──────────────────────────────┘
│
┌────────────┼────────────┐
│            │            │
Database     Scheduler     Event Bus
│
└────────────┬────────────┘
│
HTTPS / REST
│
SeedGrid Agents
# **5. Internal Organization**
The Control Plane will be composed of specialized services.
These services represent logical modules of the application.
Initially, they may be executed in the same process.
The architecture must allow their future separation without significant changes.
# **6. Services**
# **API Gateway**
Responsible for:
- authentication;
- authorization;
- request validation;
- internal routing;
- API documentation.
# **Cluster Service**
Responsible for:
- Cluster management;
- global configuration;
- overall platform state.
# **Node Service**
Responsible for:
- Node registration;
- Node state;
- inventory;
- capabilities;
- labels;
- tags.
# **Torrent Service**
Responsible for:
- torrent registration;
- querying;
- updating;
- history;
- administrative operations.
# **Scheduler Service**
Responsible for deciding where new torrents will be executed.
The algorithms used will be defined in a separate document.
# **Monitoring Service**
Responsible for:
- Health Scores;
- heartbeats;
- metrics integration;
- Cluster health consolidation.
# **Event Service**
Responsible for:
- reception;
- storage;
- distribution;
- event querying.
# **Alert Service**
Responsible for:
- generation;
- management;
- closure;
- alert querying.
# **Authentication Service**
Responsible for:
- users;
- sessions;
- permissions;
- authentication.
# **Configuration Service**
Responsible for:
- global configurations;
- Feature Flags;
- system parameters.
# **7. Internal Communication**
Internal services must communicate using well-defined interfaces.
No service must directly access the internal structures of another service.
All communication must occur through internal contracts.
# **8. Persistence**
All persistence will be performed exclusively by the Control Plane.
Agents will never write directly to the database.
The Portal will never write directly to the database.
# **9. Events**
Any service may generate events.
Examples:
Node registered.
Torrent created.
Configuration changed.
User authenticated.
Alert created.
All events must be recorded by the Event Service.
# **10. General Flow**
Portal
↓
API Gateway
↓
Responsible service
↓
Database
↓
Event Service
↓
Response
# **11. Node Flow**
Agent
↓
API Gateway
↓
Node Service
↓
Database
↓
Event Service
↓
Response to the Agent
# **12. Availability**
The Control Plane represents a central point of the platform.
Its unavailability does not interrupt the operation of the BitTorrent protocol on the Nodes.
During its unavailability:
- the Nodes continue seeding;
- the Engines remain operational;
- inventories temporarily stop being synchronized;
- administrative commands become unavailable.
Upon return, Agents must automatically synchronize their information.
# **13. Architectural Goals**
The Control Plane must be:
- modular;
- scalable;
- decoupled;
- observable;
- easily updatable;
- independent of the Nodes' infrastructure.
# **14. Fundamental Principle**
The Control Plane represents the highest authority of the Cluster.
Every administrative, operational, and orchestration decision must originate from it.
The other components exist to execute, present, or support these decisions.
# **15. Node Registration**
The registration of a new Node must follow the flow below.
Node
↓
Agent
↓
API Gateway
↓
Authentication Service
↓
Node Service
↓
Database
↓
Event Service
↓
Response to the Agent
After registration:
- the Node will receive its identity;
- its initial inventory will be created;
- the first heartbeat will be recorded;
- a registration event will be generated.
# **16. Receiving heartbeats**
The Monitoring Service will be responsible for receiving and validating heartbeats sent by Agents.
Upon receiving a heartbeat, the system must:
- validate the Node's identity;
- update the last contact time;
- update the Health Score;
- check for relevant changes;
- record events when necessary.
Heartbeats must not generate audit logs, as they represent routine operations.
# **17. Inventory Synchronization**
The Agent's Inventory Manager will send information only when necessary.
The Node Service must:
- compare inventory versions;
- update persistent records;
- log relevant changes;
- preserve history when applicable.
# **18. Torrent Lifecycle**
Simplified flow:
Portal
↓
API Gateway
↓
Torrent Service
↓
Scheduler Service
↓
Node Service
↓
Agent
↓
Torrent Engine
↓
Response
↓
Database
↓
Event Service
The Portal must never know the selected Node before the Scheduler's decision.
# **19. Administrative Operations**
Every administrative operation will follow the same pattern.
Example:
Portal
↓
API Gateway
↓
Responsible service
↓
Node Service
↓
Agent
↓
Result
↓
Database
↓
Event
Examples of operations:
- Pause torrent.
- Resume torrent.
- Run recheck.
- Update inventory.
- Restart Engine.
- Update configuration.
# **20. Information Query**
Read operations must never query the Nodes directly.
Whenever possible, the Portal will use information persisted in the database.
Direct communication with the Agent will be used only when real-time information is explicitly required.
# **21. Cluster State**
The Control Plane must maintain a consolidated view of the Cluster.
Examples:
- number of Nodes;
- online Nodes;
- total capacity;
- available space;
- number of torrents;
- aggregated upload;
- active alerts.
This view must be used by the main Dashboard.
# **22. Coordination**
The Control Plane acts as the coordinator.
It never executes local operations.
When an action requires execution on a Node:
- validates the request;
- records the intent;
- sends the command to the Agent;
- waits for a response;
- updates the persistent state.
# **23. Error Handling**
Every failure must be handled at three levels.
Level 1
Local service error.
Example:
Validation failure.
Level 2
Communication error.
Example:
Unavailable Node.
Level 3
Operational error.
Example:
Engine refused an operation.
Each level must produce messages appropriate for the operator and events compatible with the severity of the failure.
# **24. Audit**
Operations initiated by users must generate audit records.
Examples:
- configuration changes;
- torrent removal;
- user registration;
- permission changes;
- Label changes.
Automatic system operations will generate only operational events.
# **25. Concurrency**
The Control Plane must support multiple simultaneous operations.
Independent operations must be able to run in parallel.
Whenever necessary, synchronization mechanisms must prevent conflicts over the same resource.
Examples:
- two users trying to change the same Node;
- multiple operations on the same torrent;
- simultaneous inventory updates.
# **26. Scalability**
The architecture must allow horizontal scaling.
New services can be added without changing existing contracts.
Increasing the number of Nodes must not require structural changes to the Control Plane.
# **27. Data Provisioning**
The Control Plane will be the official source of truth for:
- Portal;
- CLI;
- future integrations;
- SDK.
None of these components may access the Agents directly.
# **28. Internal Services**
All internal services must be independent.
Example:
The Alert Service must not know the details of the Scheduler.
The Scheduler must not know the details of the Portal.
The Torrent Service must not know the details of the Inventory.
Communication will occur through well-defined internal interfaces.
# **29. Updates**
The Control Plane must allow updates without losing persistent state.
Whenever possible, updates must preserve active sessions and not require Node reconfiguration.
# **30. Resilience**
Failures in an internal service must not completely compromise the others.
Example:
If the Alert Service is unavailable:
- the Scheduler continues to function;
- the Torrent Service continues to function;
- the Node Service continues to function.
Unavailability must be limited to the affected domain.
# **31. Principles**
The Control Plane coordinates.
Services execute specific responsibilities.
Agents execute local actions.
Nodes execute torrents.
This separation must remain clear throughout the platform's evolution.
# **32. Security**
The Control Plane will be responsible for centralizing all platform security.
Its responsibilities include:
- user authentication;
- Node authentication;
- authorization;
- session management;
- credential issuance and validation;
- operation auditing.
No authorization decision may be delegated to the Nodes.
# **33. Observability**
The Control Plane must provide sufficient information for platform diagnosis.
Examples:
- state of internal services;
- number of connected Nodes;
- received Heartbeats;
- ongoing operations;
- active alerts;
- recent events;
- resource utilization of the Control Plane itself.
This information must be used by the monitoring system.
# **34. Logging**
All services must generate structured logs.
Each record must contain, when applicable:
- date and time;
- responsible service;
- severity level;
- operation identifier;
- related user or Node;
- descriptive message.
Logs must facilitate auditing and diagnosis, avoiding redundant information.
# **35. Configuration**
All Control Plane configuration must be centralized.
Examples:
- Cluster parameters;
- authentication;
- integrations;
- Scheduler policies;
- monitoring parameters;
- retention;
- Feature Flags.
Local configurations scattered across services must be avoided.

**Settings Store (ADR-023).** The **behavior** tunables (Worker intervals,
admission/alert thresholds, retention) are kept in a code catalog (single source of metadata)
with overrides in the `settings` table, served by a thread-safe `Store`: lock-free reads
in the loops, writes validate and apply at runtime (hot-reload). Precedence: **the database rules; the
env only seeds** on 1st boot. Editable in the Portal **Settings** panel via `/api/v1/settings`.
Infra and secrets (DB URL, tokens, admin password, API keys, prom URL, ports) remain in env,
immutable at runtime. Full catalog in `25 - Business Rules.md`.

**Agent and Engine Configuration per Node (ADR-039).** The catalog also covers the behavior
of the **Seed Nodes** (`agent.*`, `engine.*`, marked `PerNode`): the cluster value is the one from the `Store`
above and the `internal/agentconfig` package adds the **per-Node override** (table `node_settings`),
versions the set (hash), records the version each Agent applied and **delivers the effective
values in the heartbeat response**. The Agent applies it hot. Precedence: Node override >
cluster value > Node env (Agent fallback) > code default. The migration of existing Nodes
is supported by a **single seeding** of the overrides from the `.env` that the Agent
reports (`NODE_CONFIG_SEEDED`). See `13 - API.md` §8.1 and `19 - Deployment.md` §50.5.

# **36. Integrations**
The Control Plane must be the only component authorized to integrate with external systems.
Examples:
- authentication services;
- monitoring systems;
- notification platforms;
- webhooks;
- future public APIs.
Nodes must not communicate directly with external services on behalf of the Cluster, except when necessary for the Torrent Engine's own operation.

**Screenshots for Publishing (ADR-026/030/040).** Extends the UNIT3D integration from ADR-020: the
`extract_screenshots` command (command queue, ADR-014) tells the Agent to extract N JPEG frames
from the torrent via `ffmpeg`/`ffprobe` and return them in base64 in the command `result`; the CP
waits in a blocking manner (`command.Service.WaitDone`) or in stages (extract + render),
decodes the frames and hosts them via the **upload chain** (`internal/imagehost`, ADR-040):
tracker host (`image_host` section of the site profile + operator key registered and
verified) → imgbb (global key in the catalog, client `internal/imgbb`). The first host
to host all frames wins; the total budget is **25 s**, split among the
remaining hosts. Each failure becomes an entry in the Feed (`screenshot_host_failed`,
ADR-022) with the reason from the closed set of ADR-040. Exposed to the Portal via three routes
(documented in `13 - API.md`): `POST /torrents/{id}/screenshots` (one step),
`POST /torrents/{id}/screenshots/extract` (queues extraction) and
`POST /screenshots/render` (hosts + builds the BBCode). The human review before
submission to the tracker (same gate as ADR-020) is preserved. The number of screenshots is
tunable (`publish.screenshot_count`, ADR-023 catalog — see `25 - Business Rules.md`).
# **37. Scalability**
The architecture must allow gradual expansion.
Exemplos:
- increase in the number of Nodes;
- increase in the number of torrents;
- growth in the number of users;
- inclusion of new internal services.
Whenever possible, new components must reuse existing contracts.
# **38. Evolution**
The Control Plane must allow evolution without significant architectural changes.
Examples:
- new Node types;
- new Engines;
- new Scheduler algorithms;
- new user interfaces;
- new integrations.
The architecture must favor extension over replacement.
# **39. Recovery**
After a restart, the Control Plane must:
- restore the persisted state;
- make the API available;
- accept Node reconnections;
- update the Cluster state as Agents resume communication.
No manual synchronization must be required.
# **40. Limits of Responsibility**
The Control Plane must not:
- manipulate torrent files directly;
- access Node disks;
- execute Node operating system commands;
- interpret Torrent Engine internal details.
These responsibilities belong exclusively to the Agent.
# **41. Dependencies**
The Control Plane must depend on:
- PostgreSQL;
- monitoring infrastructure;
- authentication services;
- communication network with the Agents.
The unavailability of optional components must not prevent the platform's main operation.
# **42. Design Principles**
The Control Plane must remain:
- modular;
- predictable;
- decoupled;
- auditable;
- scalable;
- service-oriented.
These principles must guide future evolutions.
# **43. General Diagram**
Operator
│
Portal
│
API Gateway
│
┌───────────────────────────────────────────────┐
│               Control Plane                   │
│                                               │
│ Cluster Service                               │
│ Node Service                                  │
│ Torrent Service                               │
│ Scheduler Service                             │
│ Monitoring Service                            │
│ Event Service                                 │
│ Alert Service                                 │
│ Authentication Service                        │
│ Configuration Service                         │
└───────────────────────────────────────────────┘
│
PostgreSQL
│
HTTPS / REST
│
┌────────────┼────────────┐
│            │            │
Agent        Agent        Agent
│            │            │
Engine       Engine       Engine
│            │            │
Storage      Storage      Storage
# **44. Summary**
The Control Plane represents the central authority of the Cluster.
It maintains the consolidated view of the platform, coordinates internal services, provides APIs for operators and integrations, receives information from the Nodes, and ensures that all administrative and operational decisions are made consistently.
All the intelligence of SeedGrid resides in the Control Plane.
# **45. Final Statement**
The Control Plane architecture was designed to clearly separate decision-making, execution, and presentation.
The Portal presents.
The Control Plane decides.
The Agent executes.
The Nodes provide compute and storage capacity.
This separation forms the architectural foundation of SeedGrid and must be preserved throughout the entire evolution of the platform.
