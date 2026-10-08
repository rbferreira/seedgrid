# **SeedGrid**
**Product Roadmap**
Version: 1.0
Status: Draft
Document: 22 - Roadmap.md
# **1. Objective**
This document defines the evolution strategy for SeedGrid.
Its goal is to record the capabilities planned for future versions of the platform, maintaining alignment with the architecture and avoiding uncontrolled growth.
# **2. Philosophy**
The Roadmap represents a strategic direction.
It does not constitute an implementation commitment.
Priorities may be reviewed as the project evolves.
# **3. Principles**
The evolution of the platform must respect the following principles:
- simplicity;
- low coupling;
- predictability;
- observability;
- modularity;
- architectural compatibility.
No evolution must compromise these principles.
# **4. Version 1.0**
Main objective:
Provide a stable platform for centralized management of Seed Nodes.
Capabilities:
- Control Plane;
- Portal;
- API;
- Scheduler;
- Agent;
- Monitoring;
- Alerts;
- Platform backup;
- Disaster Recovery;
- Observability;
- Complete documentation.
Version 1.0 prioritizes stability and solid architecture.
# **5. Evolution Guidelines**
Every new feature must positively answer the following questions:
- does it solve a real problem?
- does it maintain simplicity?
- does it respect the architecture?
- does it reduce operational overhead?
- does it favor maintenance?
Otherwise, its inclusion must be reevaluated.
# **6. Vision for V2**
Main objective:
Transform SeedGrid from an administration platform into an intelligent operation platform.
V2 must prioritize:
- automation;
- operator assistance;
- explainability;
- operational planning;
- reduction of cognitive load.
# **7. Strategic Theme**
The evolution of V2 focuses on a central concept:
**Assist the operator in making better decisions, without taking away their control over the platform.**
This principle must guide all future features.
# **8. Evolution Axes**
The planned features are organized into five axes:
Operation.
Observability.
Operator Experience.
Architecture.
Engineering Tools.
This organization facilitates prioritization and incremental evolution.
# **9. Compatibility**
Whenever possible, new features must preserve compatibility with the V1 architecture.
Incompatible changes must be preceded by new ADRs.
# **10. Process**
Every relevant feature must follow the platform's official flow:
Need
↓
Discussion
↓
ADR (when applicable)
↓
Implementation
↓
Testing
↓
Documentation
# **11. Objectives**
The Roadmap must guide the evolution of the platform without compromising its architectural consistency.
# **12. Fundamental Principle**
The evolution of SeedGrid must prioritize quality, coherence, and sustainability over the quantity of features.
# **13. Capability: Reduce Operational Latency**
# **Objective**
Reduce the time between an operator action and its execution in the Cluster.
# **Motivation**
Improve platform responsiveness during administrative operations.
# **Possible Implementations**
- Persistent channel between Agent and Control Plane using WebSocket.
- Other equivalent mechanisms that preserve the platform's architectural principles.
# **Expected Benefits**
- lower latency;
- reduction of polling;
- lower resource consumption;
- better operational experience.
# **Priority**
Medium.
# **Status**
Idea.
# **14. Capability: Improve Understanding of Decisions**
# **Objective**
Allow the operator to understand why the platform made a given decision.
# **Motivation**
Increase transparency and trust in Cluster operation.
# **Possible Implementations**
- Explain Mode.
- Display of the criteria used by the Scheduler.
- Detailed justifications for Alerts.
- Exposure of the Health Score calculation.
# **Expected Benefits**
- reduction of cognitive load;
- greater predictability;
- faster diagnostics.
# **Priority**
High.
# **Status**
Planned.
# **15. Capability: Facilitate Operational Diagnostics**
# **Objective**
Reduce the time required to identify the origin of problems.
# **Motivation**
Facilitate troubleshooting and operational auditing.
# **Possible Implementations**
- Node Timeline.
- Root Cause Analysis.
- Automatic correlation between Events and Alerts.
- Contextual navigation between resources.
# **Expected Benefits**
- faster diagnostics;
- less downtime;
- better traceability.
# **Priority**
High.
# **Status**
Planned.
# **16. Capability: Operational Assistance**
# **Objective**
Guide the operator during recurring activities and recovery procedures.
# **Motivation**
Reduce operational errors and facilitate task execution.
# **Possible Implementations**
- Recovery Assistant.
- Guided runbooks.
- Contextual assistants.
- Automated checklists.
# **Expected Benefits**
- shorter learning curve;
- consistent procedures;
- reduction of human errors.
# **Priority**
Medium.
# **Status**
Idea.
# **17. Capability: Capacity Planning**
# **Objective**
Support decisions related to Cluster growth.
# **Motivation**
Allow proactive infrastructure planning.
# **Possible Implementations**
- Capacity Planner.
- Utilization projection.
- Growth trends.
- Storage exhaustion estimation.
# **Expected Benefits**
- better planning;
- reduction of downtime;
- infrastructure optimization.
# **Priority**
Medium.
# **Status**
Planned.
# **18. Capability: Engineering Evolution**
# **Objective**
Continuously improve the quality of platform development.
# **Motivation**
Automate architectural checks and reduce inconsistencies.
# **Possible Implementations**
- Architecture Linter.
- Automatic validations in CI.
- ADR compliance verification.
- Automatic observability rules.
# **Expected Benefits**
- higher code quality;
- lower technical debt;
- more consistent architecture.
# **Priority**
Low.
# **Status**
Idea.
# **19. Capability: Platform Administration**
# **Objective**
Simplify administrative and operational activities.
# **Motivation**
Reduce repetitive tasks and facilitate maintenance.
# **Possible Implementations**
- Official administration tool (seedgridctl).
- Validation automation.
- Upgrade automation.
- Diagnostic automation.
# **Expected Benefits**
- lower operational effort;
- greater standardization;
- error reduction.
# **Priority**
Medium.
# **Status**
Idea.
# **20. Principles**
The features presented in this document represent strategic directions.
The suggested implementations are not mandatory.
New solutions may replace the current proposals, provided they meet the same architectural and operational objectives defined for the platform.
# **21. Prioritization**
The capabilities foreseen in the Roadmap must be prioritized considering:
- impact on the operator;
- architectural compliance;
- reduction of operational complexity;
- implementation cost;
- technical risks;
- platform maturity.
The priority may be reviewed throughout the project's evolution.
# **22. Evolution Process**
The inclusion of a new capability must follow a structured process.
Recommended flow:
Need
↓
Discussion
↓
Architectural Analysis
↓
ADR (when applicable)
↓
Inclusion in the Roadmap
↓
Implementation
↓
Documentation
Not every idea must enter the Roadmap immediately.
# **23. Capability Status**
Each capability must have a clearly identified state.
Official states:
- Idea
- Under evaluation
- Planned
- In development
- Completed
- Deferred
- Canceled
The state change must be recorded to preserve the history of the platform's evolution.
# **24. Reviews**
The Roadmap must be reviewed periodically.
During each review, the following must be evaluated:
- new needs;
- completed capabilities;
- obsolete capabilities;
- technological changes;
- adherence to architectural principles.
# **25. Relationship with ADRs**
The Roadmap describes **where** the platform intends to evolve.
ADRs describe **how and why** architectural decisions were made.
Whenever a capability requires a significant architectural change, a new ADR must be created before its implementation.
# **26. Relationship with Other Documents**
The Roadmap complements the official SeedGrid documentation.
It does not replace:
- Vision.md;
- Requirements.md;
- Architecture.md;
- ADR.md;
- Development Guide.md.
Its role is to guide the strategic evolution of the platform.
# **27. Summary**
The evolution of SeedGrid must be driven by capabilities, not by specific technologies.
Each capability must have:
- a clear objective;
- motivation;
- expected benefits;
- possible implementations;
- priority;
- status.
This approach preserves technological flexibility and maintains focus on the problems the platform intends to solve.


# **28. Capability: Evolve Storage Management**
Objective

Allow the platform to evolve from a Seed Node-based storage view to a view based on independent storage resources.

Motivation

V1 considers storage as an attribute of the Seed Node, simplifying the architecture and reducing operational complexity.

In larger environments, this abstraction may limit more advanced storage allocation and utilization policies.

Possible Implementations
Introduction of the Storage concept as a first-class architectural entity.
Storage selection by the Scheduler during workload distribution.
Storage Classes.
Storage labels and metadata.
Affinity and eligibility policies.
Utilization balancing among Storages within the same Node.
Expected Benefits
greater flexibility in workload distribution;
support for multiple storage resources per Seed Node;
advanced allocation policies;
better utilization of available capacity;
preparation for heterogeneous environments.
Priority

Low.

Status

Idea.

Notes

This capability represents an architectural evolution and must only be considered after the stabilization of the V1 architecture. Its implementation will require a review of the ADRs, the domain model, the Scheduler, the API, and components related to inventory and monitoring.

# **29. Capability: Blocking Release Groups Banned by Tracker**
Objective

Prevent SeedGrid from publishing a release to a tracker if its group is on that tracker's banned groups list, blocking the upload and informing the operator of the reason — instead of letting the tracker refuse (`postpone`/ban) after the upload.

Motivation

Some UNIT3D trackers (e.g., samaritano.cc) explicitly prohibit uploads from certain release groups due to practices recognized as problematic: retagging (renaming files to appear as higher quality), false classification (labeling simple encodes as REMUX/WEB-DL), inserted ads (subtitles/mediainfo/video), tampered files (watermarks, artificial upscaling, non-original authored BDs), and spam/external links. Today, SeedGrid does not validate this — it only finds out when the tracker manually refuses the upload.

Possible Implementations
- New section in Settings (Portal) to manage the list of banned groups, per tracker (same scope as `trackers`, ADR-020 — each tracker can have its own list/rule).
- Check in the publishing flow (before `POST /api/v1/torrents/{id}/publish-tracker`): compares the `Group` field already extracted by the release parser (`control-plane/internal/release/release.go`, `Info.Group`) against the target tracker's list, case-insensitive comparison.
- Explicit block with a clear message to the operator (e.g., "Group «YIFY» is not allowed on samaritano.cc — upload blocked") — never fails silently nor lets it pass only to fail later on the tracker.
- Initial seed of the list for samaritano.cc with the groups below (gathered from the tracker's wiki); the list must be editable by the operator (trackers change the list over time).

Initial list of banned groups (samaritano.cc)

3LTON, 4yEo, ADE, AFG, AniHLS, AniURL, AnimeRG, AROMA, aXXo, B3LLUM, BLUDV, CaNNIBal, CHD, CM8, Comando, CrEwSaDe, D3US, d3g, DeadFish, DragsterPS, DNL, DOZITOS, eSc, ELiTE, Emmid, FaNGDiNG0, FGT, Flights, FRDS, FUM, HAiKU, HD2DVD, HDS, HDTime, Hi10, Hiro360, ION10, iPlanet, JIVE, KiNGDOM, Lama, Lapumia, Leffe, LEGi0N, LOAD, MACCAULAY, MeGusta, MezRips, mHD, mRS, mSD, MSD, NeXus, nHD, NhaNc3, nikt0, NOIVTC, nSD, OFT, Oj, PiRaTeS, PlaySD, playXD, PRODJi, Radarr, RAPiDCOWS, RARBG, RCDiVX, RDN, REsuRRecTioN, RetroPeeps, RMTeam, S74Ll10n, SANTi, SicFoI, SILVEIRATeam, SPASM, SPDVD, STARCKFILMES, STUTTERSHIT, Telly, TGx, TM, TRiToN, UPiNSMOKE, URANiME, VXT, WAF, Will1869, x0r, xRed, XS, YIFY, YTS, ZKBL, ZMNT, ZmN

Expected Benefits
- avoids `postpone`/ban due to a banned group, without relying on the operator remembering each tracker's list;
- immediate feedback in the Portal, before upload, instead of being discovered later by the tracker staff;
- list remains centralized and editable, tracking policy changes of the tracker over time.

Priority

High — motivated by a real case (upload rejection on samaritano.cc due to standardization; the list of banned groups is an explicit rule of the same tracker).

Status

Idea.

Notes

Complements ADR-020 (tracker publishing) and ADR-026 (screenshots) — one more check in the same human review stage before the `POST` to the tracker. It will require a new migration (list of groups per tracker) if not resolved as a simple text tunable; to be evaluated in the design of the corresponding ADR before implementation (contract/schema change).

# **30. Final Statement**
The SeedGrid Roadmap represents the strategic vision of the platform's evolution.
By prioritizing capabilities instead of specific implementations, the project maintains the freedom to adopt the most suitable technical solutions at the appropriate time, preserving its architecture, principles, and identity over time.
