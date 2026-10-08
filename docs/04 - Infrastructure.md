# **SeedGrid**
**Infrastructure Specification**
Version: 1.0
Status: Draft
Document: 04 - Infrastructure.md
# **1. Objective**
This document defines the infrastructure required for the deployment of SeedGrid.
Its objective is to standardize the environment organization, define the required components, and establish the responsibility of each piece of equipment participating in the Cluster.
# **2. Philosophy**
The infrastructure must be simple.
Each piece of equipment must have only the necessary responsibilities.
All intelligence will remain concentrated in the Control Plane.
Nodes must be easily replaceable.
# **3. Physical Architecture**
The minimum architecture for version 1.0 will consist of:
One Control Plane.
One or more Nodes.
One local network.
Local storage on each Node.
Example
Internet
│
Firewall / Gateway
│
Gigabit Switch
───────────────┼────────────────
│                 │
Control Plane VM     Node 01
│
USB HD
│                 │
Node 02          Node 03
│                 │
USB HD            USB HD
# **4. Control Plane**
Responsible for:
Portal.
API.
Scheduler.
Database.
Events.
Authentication.
Alerts.
Configuration.
Minimum requirements
4 vCPU
8 GB RAM
100 GB SSD
Gigabit Network
Linux Operating System
Docker
Docker Compose
Recommended requirements
8 vCPU
16 GB RAM
250 GB SSD
Daily backup
Periodic snapshot
# **5. Nodes**
Each Node represents an executor of the Cluster.
In the initial version, a Node will consist of:
Raspberry Pi.
Power supply.
Local storage.
Ethernet network.
Minimum requirements
Raspberry Pi 3B+
2 GB RAM
Ethernet network
Dedicated disk
Docker
SeedGrid Agent
Torrent Engine
Recommended requirements
Raspberry Pi 4 or higher.
SSD for the operating system.
Dedicated HDD for Torrent storage.
Official power supply.
Shared UPS.
# **6. Operating System**
Control Plane
Ubuntu Server LTS.
Nodes
Debian.
Ubuntu Server.
Raspberry Pi OS Lite.
Distributions with a graphical interface are not recommended.
# **7. Docker**
All SeedGrid software must be executed in containers.
Exceptions
Operating System.
Docker.
SeedGrid Agent (to be defined in the implementation; may run as a system service or in a container).
# **8. Control Plane Containers**
Initially, the following containers are planned:
API Gateway
Portal
PostgreSQL
Redis (if adopted in the implementation)
Scheduler
Monitoring Service
Alert Service
Event Service
Authentication Service
Configuration Service
Reverse Proxy
The container organization does not necessarily imply independent processes.
Some services may share the same application during version 1.0.
# **9. Node Containers**
Each Node must have only the necessary components.
Planned containers
Torrent Engine
SeedGrid Agent (if containerized)
Node Exporter
SMART Exporter
Watchtower (optional)
The objective is to minimize resource consumption.
# **10. Network**
All components must remain on the same logical network.
Nodes must have a fixed IP address or DHCP reservation.
The Control Plane must have a permanent address.
Communication between components will occur exclusively over the IP network.
# **11. Storage**
Each Node will have independent storage.
The Cluster will not use shared storage.
Each Torrent will physically exist only on the Node to which it was assigned.
# **12. Control Plane Directories**
Suggested structure
/opt/seedgrid/
├── compose/
├── config/
├── backups/
├── logs/
├── database/
└── monitoring/
# **13. Node Directories**
Suggested structure
/opt/seedgrid/
├── config/
├── cache/
├── logs/
└── runtime/
Torrent Storage
/data/
├── torrents/
├── incomplete/
├── completed/
└── lost+found
The Agent must never store permanent Torrent data.
These belong exclusively to the Engine.
# **14. Hardware**
SeedGrid will not depend on a specific manufacturer.
Compatible examples
Raspberry Pi.
Intel NUC.
Mini PCs.
Servers.
Virtual Machines.
VPS.
NAS.
All will be treated as Nodes.
# **15. Scalability**
Adding a new Node must only require:
Operating system preparation.
Docker installation.
Agent installation.
Registration in the Control Plane.
After this process, the new Node must immediately become eligible for use by the Scheduler.
# **16. Objectives**
The infrastructure must prioritize:
Low power consumption.
Ease of expansion.
Low cost.
Operational simplicity.
Ease of hardware replacement.
High availability of configuration data.
Independence between Nodes.
# **17. Control Plane Architecture**
The Control Plane will be deployed on a single virtual machine.
The entire platform will be executed via Docker Compose.
Logical structure:
Ubuntu Server
│
Docker Engine
│
Docker Compose
│
├── Traefik
├── Portal
├── Control Plane
├── PostgreSQL
├── Prometheus
├── Grafana
├── Loki
└── Redis (optional)
All containers will share a Docker network dedicated to SeedGrid.
# **18. Node Structure**
Each Node will consist of the following elements:
Operating System
│
Docker Engine
│
├── Torrent Engine
├── Node Exporter
├── SMART Exporter
└── (Watchtower)
│
SeedGrid Agent
│
Local Disks
The Agent will be responsible for coordinating all local components.
# **19. File System Organization**
# **Operating System**
The operating system must remain isolated from torrent data.
Example:
/ (SSD or SD card)
Operating System
Docker
SeedGrid Agent
Logs
# **Storage**
Torrents must remain on a dedicated disk: it isolates the operating system from the data and is what the protection layers in `docs/06 - Storage.md` §35 monitor. A **single-disk** Node (`/data` on the system disk) is a supported configuration, declared during installation (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false`, §35.3).
Example:
/data
├── torrents
├── incomplete
├── completed
├── recycle
└── backup
The Engine will be configured to exclusively use this volume.
# **20. Docker Organization**
Each service must have:
Dedicated volume.
Healthcheck.
Automatic restart.
Independent logs.
Standardized name.
Example:
seedgrid-api
seedgrid-portal
seedgrid-postgres
seedgrid-grafana
seedgrid-prometheus
seedgrid-loki
seedgrid-traefik
# **21. Docker Networks**
Dedicated networks will be used.
Example:
seedgrid-core
Internal communication.
- ---------------------------
seedgrid-public
Portal
Traefik
API
- ---------------------------
seedgrid-monitoring
Grafana
Prometheus
Loki
This separation facilitates organization and future expansions.
# **22. Persistent Volumes**
All persistent data must use dedicated volumes.
Examples:
Database.
Grafana.
Prometheus.
Loki.
Portal (when necessary).
Settings.
Backups.
No container must depend on the internal file system of the image.
# **23. Hostname Conventions**
Examples:
Control Plane
seedgrid-controller
Nodes
seedgrid-node-01
seedgrid-node-02
seedgrid-node-03
This convention facilitates identification in logs and monitoring.
# **24. Directory Conventions**
Control Plane
/opt/seedgrid
/config
/data
/logs
/backups
/docker
Nodes
/opt/seedgrid
/config
/cache
/logs
/runtime
# **25. Startup Flow**
# **Control Plane**
1. 1.
Operating System.
↓
1. 2.
Docker.
↓
1. 3.
Database.
↓
1. 4.
Internal services.
↓
1. 5.
Portal.
↓
1. 6.
Traefik.
The Portal must only start after the API is available.
# **Node**
1. 1.
Operating System.
↓
1. 2.
Docker.
↓
1. 3.
Torrent Engine.
↓
1. 4.
SeedGrid Agent.
↓
1. 5.
Registration.
↓
1. 6.
Heartbeat.
↓
1. 7.
Operation.
# **26. Updates**
Containers must be updated individually.
Updating a service must not require updating the entire environment.
Whenever possible, updates must preserve settings and persistent volumes.
# **27. Backups**
The Control Plane must have its own backup routine.
Minimum items:
Database.
Settings.
Persistent volumes.
Documentation.
The Nodes will remain responsible only for the local data of the Torrent Engine.
# **28. Hardware Replacement**
Replacing a Node must follow a simple workflow.
Prepare new equipment.
Install operating system.
Install Docker.
Install SeedGrid Agent.
Connect storage.
Register in the Cluster.
No additional manual configuration must be required.
# **29. Scalability**
The infrastructure must allow gradual growth.
New Nodes can be added without interrupting existing ones.
Increasing the Cluster capacity must occur by adding new Nodes, preserving the independence between them.
# **30. Principles**
The infrastructure must remain:
Modular.
Reproducible.
Containerized.
Standardized.
Easily expandable.
Each Node must be considered disposable.
The Control Plane must preserve the Cluster's identity.
# **31. Recommended Topology**
The minimum recommended infrastructure for version 1.0 consists of a single Control Plane and multiple independent Nodes.
Internet
│
Firewall / Router
│
Gigabit Switch
│
┌─────────────────┼─────────────────┐
│                 │                 │
│                 │                 │
Control Plane       Seed Node 01     Seed Node 02
│                 │                 │
│                 │                 │
PostgreSQL         HD USB 1 TB      HD USB 1 TB
│
│
Grafana
Prometheus
Loki
│
│
Seed Node 03
│
HD USB 500 GB
This topology fully meets the requirements of version 1.0 and can be expanded with new Nodes without structural changes.
# **32. Equipment Responsibility**
# **Control Plane**
Responsible for:
- Web Portal;
- API;
- Database;
- Scheduler;
- Events;
- Monitoring;
- Cluster Configuration;
- Audit.
# **Seed Nodes**
Responsible for:
- running the Torrent Engine;
- storing the torrents;
- collecting local metrics;
- executing received commands;
- providing inventory.
No persistent Cluster information must depend exclusively on a single Node.
# **33. Operational Flow**
Simplified flow of a new torrent.
Operator
↓
Portal
↓
API
↓
Control Plane
↓
Scheduler
↓
Chosen Node
↓
Agent
↓
qBittorrent
↓
Download
↓
Seeding
The operator never directly chooses the Node.
# **34. Monitoring Flow**
Node
↓
Agent
↓
Control Plane
↓
Prometheus
↓
Grafana
The Agent is responsible for collection.
The Control Plane centralizes.
Prometheus stores metrics.
Grafana presents information to the operator.
# **35. Event Flow**
Operating System
↓
Agent
↓
Control Plane
↓
Database
↓
Portal
All events follow the same path.
# **36. Availability**
The unavailability of a Node must not affect the others.
The unavailability of the Control Plane must not interrupt the BitTorrent protocol.
Torrents will continue to be distributed normally by their respective Engines while the Control Plane is unavailable.
# **37. Recovery**
After the Control Plane returns:
- the Agents must reconnect automatically;
- Heartbeats must be resumed;
- pending events must be sent;
- inventories must be synchronized when necessary.
No manual intervention must be required.
# **38. Expansion**
Cluster expansion must occur exclusively by adding new Nodes.
Recommended flow:
1. Prepare the equipment.
2. Install the operating system.
3. Install Docker.
4. Install the SeedGrid Agent.
5. Configure the Control Plane address.
6. Register the Node.
After registration, the Scheduler can automatically use it.
# **39. Maintenance**
Nodes can be placed in maintenance mode.
While in this state:
- they will not receive new torrents;
- they will continue seeding existing torrents;
- they will remain visible to the operator.
This mechanism allows interventions without impacting Cluster availability.
# **40. Network Requirements**
All Nodes must have connectivity to the Control Plane.
Nodes are not required to communicate directly with each other.
This decision reduces infrastructure coupling and simplifies deployment across different environments.
# **41. Storage Requirements**
Each Node is solely responsible for its own local storage.
SeedGrid will not use:
- NFS;
- SMB;
- Ceph;
- GlusterFS;
- distributed storage.
This decision reduces complexity, increases predictability, and keeps the architecture compatible with low-cost hardware.
# **42. Summary**
The version 1.0 infrastructure was designed to prioritize operational simplicity.
All components have well-defined responsibilities.
Nodes are independent.
The Control Plane centralizes intelligence and persistence.
Scaling is achieved solely by adding new Nodes.
# **43. Final Statement**
The SeedGrid infrastructure was designed to be reproducible, modular, and low-cost.
The use of independent components, centralized communication, and local storage makes it possible to build a distributed Cluster using commodity hardware, while preserving deployment simplicity and ease of maintenance.
This architecture constitutes the operational foundation of SeedGrid version 1.0.
