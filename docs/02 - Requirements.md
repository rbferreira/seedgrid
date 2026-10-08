# **SeedGrid**
# **Requirements Specification**
Version: 1.0
Status: Draft
Document: 02 - Requirements.md
# **1. Objective**
This document defines all functional and non-functional requirements for SeedGrid.
No implementation must introduce features not described in this document without a formal update to the specification.
# **2. Scope**
SeedGrid must allow centralized administration of multiple BitTorrent Nodes, completely abstracting their physical existence.
The operator must see only a single Cluster.
# **3. Definitions**
# **Cluster**
A set of Nodes administered by the same Control Plane.
# **Node**
Physical or virtual machine participating in the Cluster.
Examples:
- Raspberry Pi
- Mini PC
- VM
- Server
- VPS
- NAS
# **Agent**
Daemon installed on each Node.
Responsible for communication between the Control Plane and the Engine.
# **Engine**
Software responsible for BitTorrent communication.
Initial version:
- qBittorrent
# **Control Plane**
Component responsible for all Cluster decisions.
# **Portal**
Graphical interface for the operator.
# **4. Functional Requirements**
# **RF-001**
The system must allow registering new Nodes.
# **RF-002**
The system must allow removing Nodes.
# **RF-003**
The system must allow placing Nodes in mode:
- Online
- Drain
- Maintenance
- Disabled
# **RF-004**
The system must automatically detect unavailable Nodes.
# **RF-005**
The system must maintain a complete inventory of all Nodes.
# **RF-006**
The system must allow adding torrents via:
- .torrent file upload
- Magnet Link
# **RF-007**
The operator must never manually choose a Node as the default behavior.
Selection must be performed automatically by the Scheduler.
The Portal may offer an advanced option to force a specific Node when authorized.
# **RF-008**
Every torrent must have an internal unique identifier.
# **RF-009**
Every torrent must have a complete history.
# **RF-010**
Every torrent must have a state.
Allowed states:
- Queued
- Downloading
- Seeding
- Paused
- Checking
- Error
- Archived
Note
This list is an operator-oriented subset.
The canonical list of Torrent states is the one defined in Database.md §36 (QUEUED, DOWNLOADING, SEEDING, PAUSED, CHECKING, MOVING, ERROR, ARCHIVED, DELETED, UNKNOWN).
# **RF-011**
The system must allow searching for torrents using:
Name
Hash
Tracker
Status
Node
Tag
Category
Date
Ratio
# **RF-012**
The system must allow moving torrents between Nodes.
# **RF-013**
The system must allow pausing torrents.
# **RF-014**
The system must allow resuming torrents.
# **RF-015**
The system must allow removing torrents.
# **RF-016**
The system must allow optionally removing physical data.
# **RF-017**
The system must allow executing Recheck.
# **RF-018**
The system must store upload history.
# **RF-019**
The system must store download history.
# **RF-020**
The system must store ratio history.
# **5. Node Management**
Each Node must have:
UUID
Hostname
Friendly name
Description
Operating System
Architecture
CPU
RAM
Disks
Agent version
Engine version
IP address
Registration date
Last Heartbeat
State
Tags
Labels
# **6. Disk Management**
Each disk must have:
UUID
Serial
Model
Capacity
Free space
Temperature
Power On Hours
SMART
Filesystem
Mount point
State
# **7. Tracker Management**
Each Tracker must have:
Name
URL
Status
Number of Torrents
Upload
Download
Ratio
Average response time
Error count
Last announce
# **8. User Management**
The system must support multiple users.
Initial roles:
Administrator
Operator
Read Only
# **9. Dashboard**
The Dashboard must display:
Cluster health
Node count
Online Nodes
Offline Nodes
Aggregated upload
Aggregated download
Torrent count
Peer count
Seed count
Used space
Available space
Active Alerts
Recent Events
# **10. Node Dashboard**
Each Node must have its own page containing:
Hardware
Operating System
Temperature
CPU
RAM
Network
Disks
SMART
Containers
Engine
Speed
Peers
Seeds
Torrents
History
Events
# **11. Torrent Dashboard**
Each Torrent must have a page containing:
Name
Hash
Node
Tracker
Tags
Category
Priority
Status
Upload
Download
Ratio
Peers
Seeds
Active time
History
Logs
# **12. Tracker Dashboard**
Each Tracker must display:
Availability
Response time
Torrent count
Peer count
Seed count
Ratio
Upload
Download
History
# **13. Events**
Every relevant operation must generate an Event.
Examples:
Node registered
Node removed
Node offline
Node online
Torrent created
Torrent migrated
Torrent removed
Backup executed
Update executed
SMART error
Tracker unavailable
# **14. Alerts**
The system must generate Alerts for:
Low disk space
Critical SMART
High temperature
Node offline
Lost heartbeat
Container stopped
API error
Tracker unavailable
announce error
High CPU usage
High memory usage
# **15. Logs**
All components must generate structured logs.
Logs must contain:
Timestamp
Node
Service
Severity
Event
Description
Correlation ID
# **16. API**
All features must be available via API.
No feature must exist only in the graphical interface.
# **17. Scheduler**
The Scheduler must automatically decide where new torrents will run.
The criteria will be defined in a separate document.
# **18. Backup**
The system must allow automatic backup of:
Database
Settings
Metadata
Torrent state
.torrent files
FastResume files
Agent configuration
# **19. Restore**
The system must automatically restore a replaced Node.
# **20. Updates**
The Agent must support remote updates.
The Control Plane must support independent updates.
The Portal must support independent updates.
# **21. Security**
All communication must use TLS.
All users must be authenticated.
All APIs must require authentication.
All operations must be audited.
# **22. Audit**
All changes must record:
User
Timestamp
Origin
Modified object
Previous value
New value
# **23. Non-Functional Requirements**
# **RNF-001**
Continuous availability.
# **RNF-002**
Horizontal scalability.
# **RNF-003**
Low memory consumption.
# **RNF-004**
Low CPU consumption.
# **RNF-005**
Updates without configuration loss.
# **RNF-006**
Full observability.
# **RNF-007**
Reproducible infrastructure.
# **RNF-008**
Initial compatibility:
- Raspberry Pi 3B+
- Raspberry Pi 4
- Raspberry Pi 5
- Ubuntu Server
- Debian
- Raspberry Pi OS Lite
# **RNF-009**
All configuration must be exportable.
# **RNF-010**
No component must depend directly on a graphical interface.
# **24. Acceptance Criteria**
The system will be considered functional when it allows:
- Registering new Nodes.
- Adding torrents without manually choosing a Node.
- Viewing the Cluster as a single entity.
- Replacing a Node without operational loss.
- Fully monitoring all components.
- Operating exclusively through the Portal or the API.
