# **SeedGrid**
**Scheduler Specification**
Version: 1.0
Status: Draft
Document: 10 - Scheduler.md
# **1. Objective**
The Scheduler is responsible for determining on which Node a given operation must be executed.
In version 1.0, its main responsibility will be selecting the most suitable Node to receive a new Torrent.
The Scheduler does not execute operations.
The Scheduler makes decisions.
# **2. Philosophy**
The operator must never need to manually decide where a Torrent will be stored.
The Cluster must behave as a single logical environment.
The physical location of Torrents is an implementation detail.
# **3. Responsibilities**
The Scheduler must:
- select Nodes for new Torrents;
- respect Cluster constraints;
- consider the health of the Nodes;
- distribute load evenly;
- record the reasons for each decision.
The Scheduler must NOT:
- communicate directly with the Agent;
- execute commands;
- move files;
- change Node configurations.
# **4. Flow**
Portal
↓
Control Plane
↓
Scheduler
↓
Chosen Node
↓
Control Plane
↓
Agent
↓
Torrent Engine
# **5. Decision Process**
Every decision will follow the steps below.
1
Get list of eligible Nodes.
↓
2
Apply constraints.
↓
3
Calculate Score.
↓
4
Sort candidates.
↓
5
Select best candidate.
↓
6
Record decision.
↓
7
Return chosen Node.
# **6. Eligibility**
A Node may only participate in the selection if:
it is ONLINE;
it has an operational Engine;
it has available space;
it is not in MAINTENANCE;
it is not in DRAIN;
it is not DISABLED.
# **7. Criteria**
Each Node will receive a score based on several factors.
The initial criteria will be:
Available space.
Health Score.
Number of Torrents.
System load.
Disk utilization.
CPU utilization.
Memory utilization.
All criteria must have a configurable weight.
# **8. Score**
The Scheduler will calculate a single Score for each Node.
A higher Score represents a better candidate.
The algorithm must be deterministic.
For the same set of information, the result must always be the same.
# **9. Elimination Criteria**
Before calculating the Score, Nodes may be eliminated.
Examples
Node OFFLINE.
Critical SMART.
Disk out of space.
Engine unavailable.
Health Score below the minimum.
Eliminated Nodes will not participate in the scoring step.
# **10. Balancing**
The Scheduler must seek a balanced distribution.
It must not concentrate Torrents on a single Node when equivalent alternatives exist.
The distribution must consider:
space;
number of Torrents;
current load;
Node capacity.
# **11. Affinity**
In version 1.0, affinity will be based solely on Labels.
Examples
tracker=movies
country=br
priority=gold
storage=ssd
If a policy requires a specific Label, only compatible Nodes will participate in the selection.
# **12. Capabilities**
The Scheduler may use Capabilities as eligibility criteria.
Examples
SMART available.
SSD.
Docker.
USB3.
ARM64.
x86.
Compatible Torrent Engine.
Capabilities are automatically detected by the Agent.
Never by the operator.
# **13. Health Score**
The Health Score represents the overall condition of the Node.
The Scheduler must consider this value before selection.
Degraded Nodes may remain eligible, but will receive a lower score.
# **14. Space**
Available space must have a high weight.
The Scheduler must avoid Nodes close to the utilization limit.
# **15. Transparency**
Every decision must be recorded.
The operator must be able to answer later:
Why was this Node chosen?
Which Nodes participated?
What Score did each one receive?
Which criteria eliminated other candidates?
Every decision must be auditable.
# **16. Evaluation Process**
After selecting the eligible Nodes, the Scheduler will start the evaluation process.
Each Node will be analyzed individually.
The evaluation must use exclusively information persisted by the Control Plane.
The Scheduler will never directly query the Agents during a decision.
# **17. Evaluation Criteria**
The criteria below make up the initial evaluation for version 1.0.
**Available space**
Objective
Avoid Nodes close to maximum capacity.
The more free space, the greater its contribution to the score.
**Health Score**
Objective
Prioritize Nodes considered healthy.
Degraded Nodes may continue to participate, but with lower priority.
**Number of Torrents**
Objective
Avoid excessive concentration of torrents on a single Node.
This criterion considers only the number of torrents managed by SeedGrid.
**CPU Utilization**
Objective
Avoid sending new torrents to overloaded Nodes.
Momentary utilization must not completely prevent selection, except when it exceeds critical limits defined by the Cluster.
**Memory Utilization**
Objective
Distribute load evenly.
The Scheduler must avoid Nodes operating close to the limit of available memory.
**Storage Utilization**
Objective
Distribute storage growth among the Nodes.
There must not be a permanent preference for a single disk.
# **18. Tie-breaking Criteria**
If two or more Nodes obtain an equivalent score, the Scheduler must use additional criteria.
Suggested order:
Most free space.
Highest Health Score.
Fewest Torrents.
Node that has gone the longest without receiving new Torrents.
Lowest average load.
If a tie persists, any deterministic criterion may be used.
# **19. Temporary Exclusion**
A Node may be temporarily excluded from decisions.
Examples:
Maintenance.
Critical Health Score.
Insufficient space.
Recurring communication error.
This exclusion does not alter the Node's record.
It only prevents its selection while the condition persists.
# **20. Constraints**
The Scheduler must respect all constraints defined by the operator.
Examples:
Required labels.
Required capabilities.
Specific storage types.
These constraints will always take priority over the Score calculation.
# **21. Decision Record**
Each decision must generate a record containing, at a minimum:
Date and time.
Torrent.
Selected Node.
Candidate list.
Final score.
Disqualifying criteria.
Algorithm version.
This information will allow auditing and future analysis.
# **22. Failure Handling**
If no Node is eligible, the Scheduler must return an error condition to the Control Plane.
The Control Plane will be responsible for notifying the operator.
The Scheduler must not attempt to automatically correct the situation.
# **23. Reproducibility**
Given the same set of information, the Scheduler must produce the same result.
Changes to the algorithm must be versioned.
The history must record which version of the algorithm was used in each decision.
# **24. Independence**
The Scheduler must not know the details of the Torrent Engine.
It also must not depend on specific operating system characteristics of the Nodes.
All necessary information must be available through the SeedGrid data model.
# **25. Performance**
Decisions must occur quickly.
Version 1.0 goals:
Node selection under 100 ms.
Even with Cluster growth, the decision time must remain proportional to the number of eligible Nodes.
# **26. Extensibility**
New criteria may be incorporated in the future without the need to change the Scheduler architecture.
These criteria must integrate into the existing evaluation process.
# **27. Responsibilities**
The Scheduler only answers the question:
**"Which is the best Node to execute this operation?"**
It does not execute the operation.
It does not validate permissions.
It does not communicate with the Agent.
It does not change Cluster states.
# **28. Principles**
Decisions must be:
Consistent.
Auditable.
Deterministic.
Reproducible.
Transparent.
These principles must be preserved throughout the entire evolution of the Scheduler.
# **29. Operational Scenarios**
The examples below illustrate the expected behavior of the Scheduler.
The values presented are for illustrative purposes only.
# **Scenario 1**
Healthy Cluster.
Node 01
Status: ONLINE
Space: 82%
Health: 96
CPU: 18%
Torrents: 320
- ---------------------------
Node 02
Status: ONLINE
Space: 77%
Health: 94
CPU: 14%
Torrents: 305
- ---------------------------
Node 03
Status: ONLINE
Space: 91%
Health: 97
CPU: 22%
Torrents: 318
Expected result
All Nodes are eligible.
The Scheduler calculates the Score of each candidate.
The Node with the highest score will be selected.
# **Scenario 2**
Node in maintenance.
Node 01
ONLINE
- ---------------
Node 02
MAINTENANCE
- ---------------
Node 03
ONLINE
Expected result
Node 02 will be removed from the candidate list before the Score calculation.
# **Scenario 3**
Insufficient space.
Node 01
Free Space
4%
- ---------------
Node 02
42%
- ---------------
Node 03
51%
Expected result
Node 01 must not participate in the selection.
# **Scenario 4**
Degraded Health.
Node 01
Health
98
- ---------------
Node 02
Health
56
- ---------------
Node 03
Health
94
Expected result
Node 02 may remain eligible, but will receive a penalty in its score.
If the Health is below the configured minimum threshold, it must be eliminated.
# **Scenario 5**
No candidates available.
Expected result
The Scheduler will return a failure to the Control Plane.
The Control Plane will decide how to handle the situation.
The Scheduler must not create special rules for this scenario.
# **30. Decision Record**
After each decision, the Scheduler must record:
Decision identifier.
Date and time.
Torrent.
Chosen Node.
Algorithm version.
Criteria used.
Final score.
List of evaluated candidates.
Time required for the decision.
These records will be used for auditing and behavior analysis.
# **31. Transparency**
Every decision must be explainable.
The operator must be able to view:
Why a Node was chosen.
Which criteria had the greatest influence.
Which Nodes were discarded.
What the final ranking was.
The goal is to eliminate decisions considered "black box".
# **32. Algorithm Evolution**
The algorithm used by the Scheduler may evolve over time.
However, every change must preserve:
- determinism;
- auditability;
- compatibility with the Control Plane contracts;
- recording of the version used.
Algorithm changes must not require changes to the Portal or the Agents.
# **33. Configuration**
The Scheduler parameters must be centralized in the Control Plane.
Examples:
Weight of available space.
Weight of the Health Score.
Weight of CPU utilization.
Weight of memory.
Minimum free space limit.
Minimum Health Score limit.
These settings must be persisted in the database and made available through the administrative API.
# **34. Monitoring**
The Scheduler must expose sufficient information for operational monitoring.
Examples:
Number of decisions made.
Average decision time.
Number of failures.
Number of eligible Nodes.
Average number of candidates per decision.
Version of the algorithm in use.
This information will assist in diagnosing and evolving the algorithm.
# **35. Testability**
The Scheduler must be implemented so that its criteria can be tested in isolation.
Each criterion must produce predictable results when given the same data set.
Automated tests must validate:
- elimination criteria;
- Score calculation;
- tie-breakers;
- failure behavior.
# **36. Summary**
The Scheduler transforms a set of information from the Cluster into an objective decision.
Its operation is based on three main steps:
Select
↓
Evaluate
↓
Classify
↓
Choose
The rest of the platform depends on this result.
# **37. Final Statement**
The Scheduler represents the decision-making mechanism of SeedGrid.
Its responsibility is to select, in a consistent, auditable, and predictable manner, the most suitable Node to execute each operation.
The Scheduler does not execute commands, does not interact directly with Nodes, and does not have specific knowledge about the Torrent Engine.
Its role is to provide reliable decisions so that the Control Plane coordinates the Cluster as a single distributed platform.
