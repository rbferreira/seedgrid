# **SeedGrid**
**Network Specification**
Version: 1.0
Status: Draft
Document: 05 - Network.md
# **1. Objective**
This document defines the SeedGrid network architecture.
Its goal is to standardize communication between all platform components, ensuring simplicity, security, and operational predictability.
# **2. Philosophy**
The network architecture must be simple.
All components must communicate using widely supported protocols.
Whenever possible, it must be possible to deploy SeedGrid on any conventional TCP/IP network without the need for adaptations.
# **3. Topology**
The logical topology will consist of:
Control Plane.
Seed Nodes.
Operators.
All connected through the same IP infrastructure.
Example
Operator
│
──────────────────────────────
Control Plane
│
──────────────────────────────
Seed Node 01
Seed Node 02
Seed Node 03
# **4. Communication Model**
All communication must be initiated by the clients.
Portal → Control Plane
Agent → Control Plane
CLI → Control Plane
Integrations → Control Plane
The Control Plane will never initiate connections directly to the Nodes.
# **5. Protocols**
Protocols planned for version 1.0
HTTPS
REST
WebSocket (when necessary)
TCP/IP
No proprietary protocols will be used.
# **6. Communication between Nodes**
Seed Nodes must not communicate directly with each other.
All coordination will occur through the Control Plane.
This decision reduces dependencies and simplifies network management.
# **7. Discovery**
Automatic Node discovery will not be used.
Every Node must know the Control Plane address in advance.
This address will be defined during the initial Agent configuration.
# **8. DNS**
It is recommended that the Control Plane have a stable DNS name.
Example
seedgrid.local
or
seedgrid.lab
If there is no internal DNS, an IP address can be used.
# **9. Addressing**
It is recommended that all Nodes have a stable IP address.
The recommended options are:
DHCP reservation.
Static IP address.
Avoid dynamic addresses without reservation.
# **10. Ports**
The ports used must be kept to the minimum possible number.
Categories
Portal.
API.
Database.
Monitoring.
Torrent Engine.
The specific ports will be defined in the Deployment.md document.
# **11. Secure Communication**
All communication between the Agent and the Control Plane must use TLS.
Credentials must never be transmitted in plain text.
Self-signed certificates may be used in home environments.
In corporate environments, it is recommended to use certificates issued by a trusted certificate authority.
# **12. Firewall**
The Control Plane must expose only the necessary services.
Nodes must accept only connections essential for the operation of the Torrent Engine and local administration.
Everything else must remain blocked.
# **13. General Flow**
Portal
↓
HTTPS
↓
API
↓
Control Plane
↓
HTTPS
↓
Agent
↓
Engine
All administrative traffic will follow this flow.
# **14. Segmentation**
When possible, it is recommended to keep Nodes on a dedicated network or specific VLAN.
This separation improves organization and security, but is not mandatory for the platform to function.
# **15. Objectives**
The network architecture must prioritize:
Simplicity.
Low latency.
Low overhead.
Ease of deployment.
Compatibility with home and corporate networks.
# **16. Fundamental Principle**
All SeedGrid communication must occur through the Control Plane.
Nodes will never depend on direct communication with each other to execute their responsibilities.
# **17. Communication Flows**
All platform flows must have clearly defined origin, destination, and responsibility.
No component must communicate directly with another outside the flows established in this document.
# **18. Registration Flow**
Objective
Register a new Node in the Cluster.
SeedGrid Agent
↓
HTTPS
↓
API Gateway
↓
Authentication Service
↓
Node Service
↓
Database
↓
Response to the Agent
This flow will occur only during the initial registration or when a new Node is added to the Cluster.
# **19. Heartbeat Flow**
Objective
Inform that the Node remains operational.
Agent
↓
HTTPS
↓
API Gateway
↓
Monitoring Service
↓
Database
This flow must be lightweight and frequent.
Heartbeats must never carry large volumes of data.
# **20. Inventory Flow**
Objective
Update the Node's known configuration.
Agent
↓
HTTPS
↓
Node Service
↓
Database
The inventory will be sent:
- on the first registration;
- when requested;
- when changes are detected.
# **21. Event Flow**
Objective
Record relevant events.
Agent
↓
Event Service
↓
Database
↓
Portal
Examples:
Engine started.
Critical SMART.
Disk removed.
Communication error.
# **22. Operations Flow**
Objective
Execute administrative actions.
Portal
↓
API
↓
Control Plane
↓
Agent
↓
Engine
Examples:
Add torrent.
Pause.
Resume.
Recheck.
Update configuration.
# **23. Query Flow**
Whenever possible:
Portal
↓
API
↓
Database
↓
Response
Queries must not access Agents directly, except when real-time information is indispensable.
# **24. Monitoring Flow**
Monitoring will follow a pull architecture.
Prometheus
↓
Collection
↓
Control Plane
↓
Grafana
When necessary, Prometheus may collect metrics directly from the Node Exporters.
# **25. Alert Flow**
Event
↓
Alert Service
↓
Database
↓
Portal
↓
Operator
Alerts represent active issues.
Not every event generates an alert.
# **26. Authentication Flow**
Portal
↓
Authentication Service
↓
Database
↓
Session
↓
Response
Agents will use their own authentication mechanism.
Users will never share credentials with Nodes.
# **27. Configuration Flow**
Portal
↓
Configuration Service
↓
Database
↓
Response
Whenever a configuration impacts the Nodes, the Control Plane will be responsible for coordinating its distribution.
# **28. Timeouts**
All communication must have a configurable timeout.
Objectives:
Avoid permanent blocks.
Detect unavailability quickly.
Allow automatic recovery.
Specific values will be defined during implementation.
# **29. Reconnection**
If communication between the Agent and the Control Plane is interrupted:
The Agent must:
- continue operating;
- attempt to reconnect periodically;
- synchronize pending information after reconnection.
This behavior must be transparent to the operator whenever possible.
# **30. Compression**
Small messages must not use compression.
Larger messages, such as complete inventories, may use compression mechanisms when this provides an operational benefit.
This functionality must remain transparent to API consumers.
# **31. Network Objectives**
The network must provide:
Low latency.
High predictability.
Low bandwidth consumption.
Automatic recovery.
Continuous operation.
# **32. Principles**
Communication flows must remain simple.
Each piece of information must have a single official path within the architecture.
This approach reduces coupling, facilitates monitoring, and simplifies future platform evolutions.
# **33. Availability**
The network must be considered a shared resource among all Cluster components.
Temporary network unavailability must not interrupt the local operation of Seed Nodes.
During communication failures:
- the Torrent Engine will continue operating;
- torrents will continue to be distributed normally;
- the Agent will continue monitoring the Node;
- administrative operations will be temporarily unavailable.
# **34. Recovery**
After communication with the Control Plane is restored, the Agent must:
- re-establish authentication;
- send a new Heartbeat;
- synchronize pending Events;
- update inventory when necessary;
- resume normal operation.
The entire process must occur automatically.
# **35. Fault Tolerance**
SeedGrid must support temporary network failures.
Examples:
Momentary loss of connectivity.
Switch reboot.
IP address renewal.
Control Plane restart.
Whenever possible, recovery must occur without operator intervention.
# **36. Latency Requirements**
The platform was designed for local networks.
Communication must remain efficient even on home networks.
Latency must not be a determining factor for the Cluster's operation.
However, it is recommended that all Nodes be connected via Ethernet whenever possible.
# **37. Bandwidth**
SeedGrid must generate low administrative traffic.
The highest bandwidth consumption will continue to be the BitTorrent protocol.
Communications between the Agent and the Control Plane must represent only a small fraction of the total infrastructure traffic.
# **38. Time Synchronization**
All equipment must keep their clocks synchronized.
The use of NTP is recommended.
Proper synchronization is essential for:
- audit;
- Events;
- monitoring;
- log correlation.
# **39. Security**
Recommended best practices:
- use HTTPS for all administrative communication;
- restrict access to the Portal;
- protect Agent credentials;
- keep systems updated;
- use a firewall on the Control Plane;
- limit service exposure to the network.
Whenever possible, only the Portal and API must be accessible to operators.
# **40. Environments**
The architecture must support different scenarios.
# **Homelab**
Control Plane on a virtual machine.
Seed Nodes connected to the LAN.
Optional DNS.
Self-signed certificates.
# **Small Corporate Environment**
Dedicated Control Plane.
Internal DNS.
Valid certificates.
Centralized monitoring.
Automated backups.
# **Distributed Environment**
The architecture allows the geographic distribution of Nodes, provided there is IP connectivity with the Control Plane.
Latency and availability issues must be evaluated according to the environment.
# **41. Scalability**
Network growth must occur only by adding new Nodes.
No structural changes will be required in the communication protocol.
All new Nodes will follow exactly the same registration and operation flow.
# **42. Network Monitoring**
The Control Plane must provide sufficient information to identify communication issues.
Examples:
- last Heartbeat received;
- disconnected Nodes;
- time since last contact;
- authentication failures;
- communication errors.
This information must be integrated into the platform's monitoring system.
# **43. Summary**
The SeedGrid network architecture is based on a simple and centralized model.
Operator
│
▼
Portal
│
▼
API Gateway
│
▼
Control Plane
│
┌───┼───────────────┐
▼   ▼               ▼
Agent 01       Agent 02       Agent 03
│               │               │
qBittorrent    qBittorrent    qBittorrent
All administrative traffic must pass through the Control Plane.
Nodes remain independent and do not establish direct communication with each other.
# **44. Final Statement**
The SeedGrid network architecture was designed to prioritize simplicity, predictability, and loose coupling.
Centralized communication through the Control Plane reduces operational complexity, facilitates auditing and monitoring, and allows new Nodes to be integrated into the Cluster without changes to the existing infrastructure.
This approach establishes a solid foundation for the platform's evolution, preserving compatibility across different deployment environments and different generations of hardware.
