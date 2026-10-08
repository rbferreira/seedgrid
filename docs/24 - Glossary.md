# **SeedGrid**
# **Glossary**
Version: 1.0
Status: Draft
Document: 24 - Glossary.md
# **1. Objective**
This document defines the official terminology used by SeedGrid.
Its objective is to ensure consistency across the platform's documentation, implementation, and operation.
Every term defined in this glossary must have a single official meaning.
# **2. Philosophy**
Each concept must have a clear and unambiguous definition.
Different terms must not be used to represent the same concept.
Likewise, a single term must not represent distinct concepts.
# **3. Cluster**
# **Definition**
The complete set of components that form a SeedGrid installation.
Includes:
- Control Plane;
- Seed Nodes;
- Portal;
- API;
- Scheduler;
- Database;
- observability components.
# **Do not use for**
- a single Seed Node;
- a single group of torrents.
# **4. Control Plane**
# **Definition**
Central component responsible for Cluster administration.
Responsibilities:
- management;
- persistence of the Cluster State;
- coordination;
- authentication;
- integration between components.
# **Do not use for**
Portal.
API.
Scheduler.
Although these components are part of the Control Plane, they do not represent the Control Plane in isolation.
# **5. Seed Node**
# **Definition**
Equipment responsible for running the Torrent Engine and storing torrent data.
Each Seed Node represents an independent operational unit.
# **Do not use for**
Agent.
Torrent Engine.
Disk.
The Seed Node encompasses all these elements.
# **6. Agent**
# **Definition**
Service running on each Seed Node responsible for communication with the Control Plane.
Functions:
- Heartbeats;
- inventory;
- command execution;
- local information collection.
# **Do not use for**
Torrent Engine.
Scheduler.
# **7. Scheduler**
# **Definition**
Component responsible for deciding where new workloads must be executed.
The decision uses global Cluster information.
# **Do not use for**
Load balancer.
Torrent Engine.
# **8. Portal**
# **Definition**
Official graphical interface of the platform.
Allows administration and visualization of the Cluster.
All communication occurs through the API.
# **Do not use for**
API.
Control Plane.
# **9. API**
# **Definition**
Official communication layer between clients and the Control Plane.
All administrative interaction must occur through the API.
# **Do not use for**
Database.
Portal.
Agent.
# **10. Torrent Engine**
# **Definition**
Application responsible for executing the BitTorrent protocol on a Seed Node.
Its responsibility is limited to torrent-related operations.
# **Do not use for**
Agent.
Scheduler.
Control Plane.
# **11. Cluster State**
# **Definition**
Set of persistent information that describes the administrative state of the Cluster.
Examples:
- Nodes;
- users;
- permissions;
- settings;
- events;
- alerts;
- audit.
The Cluster State represents the primary information protected by the platform.
# **12. Heartbeat**
# **Definition**
Periodic message sent by the Agent to the Control Plane to indicate availability.
Its absence may indicate a communication failure or Seed Node unavailability.
# **Do not use for**
Event.
Alert.
# **13. Health Score**
# **Definition**
Calculated indicator that represents the operational condition of a Seed Node or the Cluster.
It is used to support decision-making by the Scheduler and the operator.
It does not represent a failure condition in isolation.
# **14. Principle**
The terms defined in this document represent the official SeedGrid nomenclature.
All future documentation must use these definitions.
# **15. Log**
# **Definition**
Detailed technical record produced by a component during its execution.
Logs are primarily intended for diagnostics, debugging, and troubleshooting.
# **Characteristics**
- high frequency;
- low level of abstraction;
- oriented towards developers and operators.
# **Do not use for**
- Alerts;
- Domain events;
- Audit.
# **16. Event**
# **Definition**
Structured representation of a relevant fact that occurred on the platform.
Events describe important occurrences, regardless of whether they represent issues.
# **Examples**
- Node registered.
- Torrent added.
- Backup completed.
- Update started.
# **Characteristics**
- structured;
- traceable;
- used for automation and operational history.
# **Do not use for**
Logs.
Alerts.
# **17. Alert**
# **Definition**
Notification generated when an operational condition requires the operator's attention.
Alerts are derived from the analysis of metrics, events, and known states of the platform.
# **Examples**
- Disk nearly full.
- Node Offline.
- Critical Health Score.
# **Characteristics**
- low frequency;
- high relevance;
- actionable.
# **Do not use for**
Logs.
Events.
# **18. Incident**
# **Definition**
Operational situation that impacts or may impact the availability, integrity, or functioning of the platform.
An incident may be associated with one or more alerts.
# **Examples**
- Loss of a Seed Node.
- Database corruption.
- Control Plane failure.
# **Characteristics**
- represents an operational issue;
- may trigger Disaster Recovery procedures;
- typically has a start, handling, and resolution.
# **19. Audit**
# **Definition**
Permanent record of administrative actions executed by operators or authorized processes.
Its objective is to ensure traceability and accountability.
# **Examples**
- Configuration change.
- User deletion.
- Permission change.
- Manual backup execution.
# **Characteristics**
- immutable;
- persistent;
- governance-oriented.
# **Do not use for**
Technical logs.
Operational events.
# **20. Metric**
# **Definition**
Numeric value used to measure platform behavior over time.
Metrics support monitoring, alert generation, and performance analysis.
# **Examples**
- CPU usage.
- Memory usage.
- API latency.
- Active torrents.
- Free disk space.
# **Characteristics**
- time series;
- collected periodically;
- used for quantitative analysis.
# **21. Relationship between Concepts**
The main observability concepts relate as follows:
Logs
│
▼
Events
│
▼
Alerts
│
▼
Incidents
Audit remains parallel to this flow, recording administrative actions.
Metrics feed Events, Alerts, and monitoring dashboards, but do not represent events in themselves.
# **22. Operator**
# **Definition**
Person responsible for the administration, operation, and maintenance of the Cluster.
The operator uses the Portal and other official platform tools to monitor, configure, and recover the environment.
# **23. Capability**
# **Definition**
Strategic platform evolution goal described in the Roadmap.
A capability represents a problem to be solved, regardless of the technology used to do so.
A capability may have multiple possible implementations over the course of the project's evolution.
# **24. Principle**
Logs describe technical details.
Events describe facts.
Alerts describe conditions that require attention.
Incidents describe operational problems.
Audit describes administrative actions.
Metrics describe the quantitative behavior of the platform.
These concepts represent distinct categories and must never be used as synonyms.
# **25. Node Capacity**
# **Definition**
Set of resources available on a Seed Node for workload execution.
It may consider:
- available storage;
- CPU;
- memory;
- bandwidth;
- administrative limits.
Capacity is used as one of the inputs for the Scheduler.
# **26. Inventory**
# **Definition**
Set of information known to the Control Plane about the existing resources in the Cluster.
It includes, among others:
- Seed Nodes;
- disks;
- network interfaces;
- software versions;
- Torrent Engines;
- capabilities.
The inventory is updated periodically by the Agents.
# **27. Maintenance Mode**
# **Definition**
Administrative state in which a component remains operational but unavailable for new workloads.
During this period, the component may continue to be monitored and managed.
Its purpose is to allow planned interventions without interrupting the rest of the Cluster.
# **28. Scheduler Decision**
# **Definition**
Result produced by the Scheduler indicating where a given workload must be executed.
A decision may consider:
- Health Score;
- available capacity;
- Cluster policies;
- administrative constraints;
- operational state of the Nodes.
Scheduler decisions must be traceable.
# **29. Policy**
# **Definition**
Set of administrative rules that guide platform behavior.
Examples:
- distribution policies;
- backup policies;
- retention policies;
- maintenance policies.
Policies represent high-level configurations defined by the operator.
# **30. Configuration**
# **Definition**
Set of parameters used to control platform behavior.
Configurations can exist at different levels:
- Cluster;
- component;
- Node.
Configurations do not represent permanent architectural decisions.
# **31. Capability**
# **Definition**
Strategic goal registered in the Roadmap.
A Capability describes **the problem to be solved**, regardless of the technology used.
Each Capability may have one or more possible implementations.
# **32. Component**
# **Definition**
Logical unit of the SeedGrid architecture with a clearly defined responsibility.
Examples:
- Portal;
- API;
- Scheduler;
- Agent;
- Alert Service;
- Monitoring Stack.
Each component must have a single responsibility and well-defined interfaces.
# **33. Service**
# **Definition**
Executable implementation of a Component.
A Component represents an architectural concept.
A Service represents its operational implementation.
Although there is often a one-to-one relationship, these concepts are not equivalent.
# **34. Terminological Conventions**
All official documentation must exclusively use the terms defined in this Glossary.
Synonyms must be avoided when they can generate ambiguities.
New terms must be added to this document before being officially adopted by the platform.
# **35. Evolution**
The Glossary must evolve along with the architecture.
Whenever new structural concepts are introduced, their official definitions must be recorded before their use in documentation or implementation.
# **36. Relationship with Other Documents**
This Glossary complements all official SeedGrid documentation.
Whenever there is doubt about the meaning of a technical term, the definition established in this document prevails.
Other documents must reference these concepts, avoiding local redefinitions.
# **37. Summary**
The Glossary establishes the official language of SeedGrid.
Its purpose is to ensure that architecture, documentation, implementation, and operations use the same concepts, preserving clarity, consistency, and traceability throughout the platform's evolution.
# **38. Final Statement**
The official SeedGrid terminology is an integral part of its architecture.
By precisely defining the concepts used by the platform, this Glossary reduces ambiguities, facilitates communication between teams, and ensures that any future evolution preserves a common and consistent language.
