# **SeedGrid**
**Monitoring Specification**
Version: 1.0
Status: Draft
Document: 15 - Monitoring.md
# **1. Objective**
The SeedGrid monitoring system is responsible for collecting, consolidating, and presenting operational information about the entire platform.
Its goal is to provide continuous visibility into the health of the Cluster, enabling the identification of issues, trends, and optimization opportunities.
# **2. Philosophy**
The monitoring system must answer the following questions:
Is the Cluster healthy?
Are all Nodes operational?
Is there a risk of unavailability?
Is there a risk of running out of space?
Is the Scheduler working correctly?
Are the services responding properly?
# **3. Responsibilities**
The monitoring system must:
- collect metrics;
- consolidate information;
- calculate indicators;
- provide dashboards;
- support alert generation.
The monitoring system must NOT:
- execute corrective actions;
- modify configurations;
- make administrative decisions.
These responsibilities belong to other platform components.
# **4. Architecture**
Seed Nodes
│
Exporters
│
Prometheus
│
Grafana
│
Operator
The Control Plane will also provide metrics to Prometheus.
# **5. Components**
The monitoring solution will consist of:
Prometheus.
Grafana.
Exporters.
Control Plane metrics.
Agent metrics.
# **6. Data Sources**
The main sources of information will be:
Control Plane.
SeedGrid Agent.
Node Exporter.
SMART Exporter.
Torrent Engine.
Operating System.
# **7. Prometheus**
Responsible for:
collecting metrics;
storing time series;
providing queries;
supplying data to Grafana.
Prometheus will be considered the official source of metrics for the platform.
# **8. Visualization (Portal) — ADR-016**
> **Updated by ADR-016:** Grafana has been removed from the stack. The visualization of
> metrics (graphs, panels, dashboards) is now the responsibility of the **Portal**
> (product). To comply with ADR-005 (Portal only talks to the API), the **Control Plane**
> queries Prometheus (PromQL) on the server and exposes the series via `/api/v1/metrics/...`;
> the Portal only plots. Flow: Portal → Control Plane API → Prometheus.
> The domain state/health of the Cluster continues to come from Postgres via the API.
> Threshold-based metric alerts are handled by Alertmanager (webhook → Control Plane).
# **9. Exporters**
Each Node must provide metrics through appropriate Exporters.
Examples:
Node Exporter.
SMART Exporter.
SeedGrid Agent metrics.
Torrent Engine metrics.
# **10. Cluster Metrics**
Suggested indicators:
Online Nodes.
Offline Nodes.
Nodes in maintenance.
Total capacity.
Free space.
Number of Torrents.
Aggregated upload.
Aggregated download.
Average health.
Active alerts.
# **11. Node Metrics**
Each Node must provide, at a minimum:
CPU.
Memory.
Disk.
Network.
Temperature.
Uptime.
Agent version.
Engine version.
Health Score.
# **12. Torrent Engine Metrics**
Suggested indicators:
Number of Torrents.
Active Torrents.
Seeds.
Peers.
Upload.
Download.
Connections.
Disk usage.
# **13. Dashboards**
The system must have specific dashboards for:
Cluster.
Nodes.
Torrents.
Scheduler.
Infrastructure.
Control Plane.
# **14. Updates**
Metrics must be updated periodically.
The frequency must balance:
accuracy;
resource consumption;
network traffic.
Specific intervals will be defined during implementation.
# **15. Objectives**
The monitoring system must enable rapid platform diagnosis without the need for direct access to the Nodes.
# **16. Fundamental Principle**
Every operational decision must be based on reliable and up-to-date information.
The monitoring system represents the operational view of the platform and must faithfully reflect the state of the Cluster.
# **17. Metrics Organization**
Metrics must be organized by domain.
Expected domains:
Cluster.
Control Plane.
Nodes.
Torrent Engine.
Storage.
Network.
Scheduler.
API.
Portal.
This organization will facilitate dashboards, queries, and future integrations.
# **18. Cluster Metrics**
Consolidated indicators.
Examples:
Number of Nodes.
Online Nodes.
Offline Nodes.
Nodes in maintenance.
Total capacity.
Used space.
Available space.
Total number of Torrents.
Aggregated upload.
Aggregated download.
Average health.
Active alerts.
Events per hour.
# **19. Control Plane Metrics**
Suggested indicators:
Uptime.
CPU utilization.
Memory utilization.
Number of requests.
Average response time.
API errors.
Active sessions.
Database connections.
Average query time.
# **20. Node Metrics**
Each Node must provide:
Average load.
CPU.
Memory.
Swap.
Disk.
IOPS.
Temperature.
Network interfaces.
Available space.
Uptime.
Health Score.
Last Heartbeat.
Agent version.
# **21. Storage Metrics**
Per disk:
Capacity.
Used space.
Free space.
Temperature.
SMART.
Known errors.
File system.
Uptime.
# **22. Torrent Engine Metrics**
Suggested indicators:
Active Torrents.
Paused Torrents.
Downloading Torrents.
Seeding Torrents.
Seeds.
Peers.
Upload speed.
Download speed.
Disk usage.
Open connections.
# **23. Scheduler Metrics**
Operational indicators.
Examples:
Number of decisions.
Average decision time.
Maximum time.
Failures.
Average number of candidates.
Most selected Node.
Torrent distribution.
Algorithm version.
# **24. API Metrics**
Examples:
Requests per minute.
Latency.
Errors.
Authentications.
Sessions.
Average time per endpoint.
# **25. Dashboards**
Grafana must provide specialized dashboards.
Cluster Dashboard
Overview.
Nodes Dashboard
Detailed state of each Node.
Storage Dashboard
Disk health.
SMART.
Space.
Torrents Dashboard
Torrent Engine indicators.
API Dashboard
Control Plane health.
Scheduler Dashboard
Distribution of decisions.
# **26. Update**
Metrics must be collected at appropriate intervals.
Critical information may be collected more frequently.
Inventories must not be treated as metrics.
# **27. Retention**
The metrics retention period must be configurable.
The retention policy must consider:
Storage capacity.
Operational need.
Historical analysis goals.
# **28. Queries**
The system must allow historical queries.
Examples:
CPU usage over the last 24 hours.
Available space over the last week.
Health Score over the last month.
Aggregated upload over the last hours.
These queries will be performed directly on Prometheus.
# **29. Export**
Metrics must be available for integration with Prometheus-compatible tools.
The architecture must not prevent future integrations with other observability systems.
# **30. Principles**
Monitoring must provide sufficient information for:
Diagnosis.
Planning.
Capacity.
Performance analysis.
Failure investigation.
Without introducing unnecessary complexity or excessive resource consumption.
# **31. Health Score**
The Health Score represents a consolidated indicator of a Node's operational health.
Its goal is to provide a quick assessment of the overall condition of the equipment.
The Health Score calculation will be performed by the Control Plane using information received from Agents.
# **Objectives**
The Health Score must assist:
- the operator;
- the Scheduler;
- platform monitoring.
The value does not replace individual metrics, it only summarizes the overall condition of the Node.
# **32. History**
Metrics must allow historical analysis.
Examples:
- storage growth;
- upload evolution;
- Node availability;
- CPU utilization;
- memory utilization.
The history will be maintained exclusively by the monitoring system.
# **33. Recommended Dashboards**
# **Executive Dashboard**
Cluster overview.
Consolidated indicators.
Active alerts.
Available capacity.
Overall health.
# **Operational Dashboard**
Detailed state of the Nodes.
Engine.
Disks.
Network.
Recent events.
# **Capacity Dashboard**
Available space.
Historical utilization.
Growth.
Torrent distribution.
# **Platform Dashboard**
Control Plane.
API.
Portal.
Database.
Scheduler.
Response time.
# **34. Diagnosis**
Monitoring must allow quickly answering questions such as:
Which Node is unavailable?
Which disk is nearing maximum capacity?
Which service has stopped responding?
Which Node has the worst Health Score?
Which component has the highest resource consumption?
# **35. Correlation**
Whenever possible, dashboards must allow navigation between related information.
Examples:
Node → Torrents.
Node → Events.
Node → Alerts.
Torrent → Node.
Event → Related resource.
This navigation reduces the time required for diagnosis.
# **36. Resource Consumption**
The monitoring system itself must have low impact on the platform.
Objectives:
Low CPU consumption.
Low memory consumption.
Low network traffic.
Low storage consumption.
Metric collection must never compromise the normal operation of the Nodes.
# **37. Availability**
The temporary unavailability of the monitoring system must not affect:
- the operation of the Nodes;
- the Torrent Engine;
- the Scheduler;
- the Control Plane.
Monitoring represents an observability feature and does not directly participate in the platform's operation.
# **38. Integration**
The metrics produced by SeedGrid must remain compatible with the Prometheus ecosystem.
Whenever possible, conventions widely adopted by the community will be used.
This will facilitate future integrations and the reuse of existing tools.
# **39. Evolution**
The architecture must allow the inclusion of new metrics without the need for significant changes to existing components.
New collectors may be added respecting the contracts established by the Agent and the Control Plane.
# **40. Architecture Summary**
Seed Nodes
│
┌──────────┴──────────┐
│                     │
SeedGrid Agent      Exporters
│                     │
└──────────┬──────────┘
│
Prometheus
│
Grafana
│
Operator
Monitoring complements the platform by providing continuous visibility into the Cluster's operation.
# **41. Final Statement**
The SeedGrid monitoring system is designed to provide a clear, consolidated, and reliable view of the platform.
It supports the operator in identifying issues, capacity planning, and analyzing Cluster behavior, preserving the decoupling between monitoring, decision, and execution established by SeedGrid's overall architecture.
