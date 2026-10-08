# SeedGrid

## Workers Specification

Version: 1.0

Status: Draft

Document: 07 - Workers.md

---

# 1. Objective

This document defines the architecture of the SeedGrid Workers.

Its objective is to establish the official organization of the platform's asynchronous processing, defining responsibilities, boundaries, and principles that must be preserved by any implementation.

---

# 2. Philosophy

Workers represent architectural components responsible for executing asynchronous activities of the Control Plane.

Their purpose is to decouple long-running, periodic, or non-interactive tasks from operator-facing services.

Workers represent architectural responsibilities, not specific implementation technologies or mechanisms.

---

# 3. Definition

A Worker is a logical component responsible for executing a specific set of asynchronous activities.

Each Worker has a single, well-defined responsibility.

Workers do not represent:

* processes;
* threads;
* containers;
* executables;
* operating system services.

These details belong exclusively to the implementation.

---

# 4. Scope

All Workers belong to the Control Plane.

No Worker must execute directly on the Seed Nodes.

All interaction with the Seed Nodes must occur exclusively through the Agent and the official communication channels defined by the architecture.

---

# 5. V1 Architecture

Version 1.0 of SeedGrid defines five official Workers.

These Workers represent the official architecture of the platform.

They are:

* Inventory Worker;
* Monitoring Worker;
* Alert Worker;
* Scheduler Worker;
* Maintenance Worker.

New Workers are not part of the V1 architecture.

---

# 6. Inventory Worker

Responsible for updating the Cluster inventory.

Its responsibilities include:

* processing information sent by Agents;
* updating the Seed Nodes inventory;
* updating known capabilities;
* synchronizing the Cluster State related to the inventory.

The Inventory Worker does not execute administrative decisions.

---

# 7. Monitoring Worker

Responsible for processing the platform's operational information.

Examples:

* metrics consolidation;
* updating operational states;
* calculating indicators;
* providing information for observability.

The Monitoring Worker does not generate Alerts directly.

---

# 8. Alert Worker

Responsible for evaluating the conditions that trigger Alerts.

Its responsibilities include:

* metrics analysis;
* Event analysis;
* Cluster State analysis;
* opening Alerts;
* closing Alerts.

The Alert Worker does not send notifications directly to operators.

---

# 9. Scheduler Worker

Responsible for the operational execution of the decisions produced by the Scheduler.

Examples:

* processing distribution requests;
* applying Scheduler decisions;
* updating the Cluster State after the execution of operations.

Decision rules remain the responsibility of the Scheduler.

---

# 10. Maintenance Worker

Responsible for the periodic administrative activities of the platform.

Examples:

* clearing temporary data;
* data reconciliation;
* periodic checks;
* scheduled administrative routines.

All tasks must be idempotent whenever technically possible.

---

# 11. Communication

Workers must communicate only through the components defined by the architecture.

Examples:

* Database;
* Scheduler;
* internal API;
* internal Control Plane services.

Workers must never communicate directly with the Portal.

---

# 12. State

Workers must not maintain their own persistent State.

All permanent information must remain in the official Cluster State.

Workers may only use temporary information necessary for the execution of ongoing tasks.

---

# 13. Objectives

The Worker architecture must:

* organize asynchronous processing;
* reduce coupling;
* facilitate maintenance;
* promote controlled evolution;
* keep responsibilities clearly defined.

---

# 14. Fundamental Principle

The architecture defines which Workers exist and which responsibilities belong to each of them.

The implementation is free to define how these Workers will be executed, provided it fully preserves the architecture established in this document.



---

# 15. Execution Model

Workers must execute their activities independently of the platform's interactive services.

A task being executed by a Worker must not block:

* Portal;
* API;
* Scheduler;
* other Workers.

The architecture must favor asynchronous processing whenever appropriate.

---

# 16. Scheduling

Each Worker may execute tasks:

* on demand;
* periodically;
* in response to internal Events.

The scheduling mechanism belongs to the implementation and must remain transparent to the architecture.

---

# 17. Work Queues

When a task cannot be executed immediately, it may be placed in an internal queue.

Queues represent an implementation mechanism and are not part of the official architecture.

Regardless of the technology used, the platform must preserve the logical order and integrity of the tasks.

---

# 18. Idempotency

Whenever technically possible, the operations executed by the Workers must be idempotent.

The re-execution of a task must not produce unexpected side effects or inconsistencies in the Cluster State.

When idempotency is not possible, the limitation must be documented.

---

# 19. Failure Recovery

Failures during the execution of a task must be handled in a controlled manner.

The architecture provides for:

* error logging;
* retry capability;
* preservation of Cluster State consistency.

The retry strategy belongs to the implementation.

---

# 20. Concurrency

The architecture must allow the simultaneous execution of different Workers.

However, a single architectural responsibility must preserve the consistency of its operations.

When necessary, synchronization mechanisms may be used by the implementation.

---

# 21. Monitoring

Each Worker must provide information for observability.

Recommended minimum information:

* current state;
* number of executed tasks;
* number of failures;
* average execution time;
* last completed execution.

---

# 22. Logs

Every relevant execution must produce structured logs.

The records must allow answering, at a minimum:

* which Worker executed the task;
* when it started;
* when it finished;
* what the result was;
* how long it took.

---

# 23. Events

Workers may produce Events when relevant changes occur in the Cluster State.

Events must follow the standards defined in **Glossary.md**.

Events do not replace logs or alerts.

---

# 24. Alerts

Issues related to Workers may trigger Alerts.

Examples:

* consecutive failures;
* abnormal execution time;
* unexpected interruption;
* tasks accumulated beyond defined limits.

Alert generation rules must remain configurable.

---

# 25. Audit

When a Worker performs administrative changes to the Cluster State, these changes must generate audit records as defined in **Audit** and **Glossary.md**.

The automatic execution of a task does not eliminate the need for traceability.

---

# 26. Principles

Worker execution must preserve:

* predictability;
* isolation;
* traceability;
* consistency;
* fault tolerance;
* observability.

These principles must guide any implementation of the asynchronous processing layer.



---

# 27. Security

Workers must operate using only the privileges necessary to execute their responsibilities.

No Worker must have administrative permissions beyond those required for its role.

Whenever possible, the principle of least privilege must be adopted.

---

# 28. Configuration

The configurations used by Workers must remain external to the source code.

Examples:

* execution intervals;
* operational limits;
* processing parameters;
* retry policies.

Every configuration must have corresponding documentation.

**Dynamic tunables (ADR-023).** Worker behavior parameters — execution
intervals, admission controller thresholds (ADR-021) and alert rules (ADR-008),
sample retention — are **read per pass** from a dynamic catalog (`internal/settings`),
not captured at boot. They are editable in the Portal **Settings** panel and take effect **without
redeploy** (hot-reload); the effective value is the database override (table `settings`) over the
catalog default. The complete catalog is in `25 - Business Rules.md`.

---

# 29. Availability

The unavailability of a Worker must not immediately compromise the operation of the other platform components.

When a Worker is unavailable:

* the fact must be logged;
* the operator must be notified when applicable;
* recovery must occur in a controlled manner.

The architecture must favor controlled degradation over cascading failures.

---

# 30. Evolution

The Worker architecture may evolve in future versions.

However, changes that modify:

* responsibilities;
* the number of official Workers;
* the processing flow;
* architectural boundaries;

must be preceded by corresponding documentation and, when applicable, a new ADR.

---

# 31. Relationship with Other Documents

This document complements:

* Architecture.md;
* Scheduler.md;
* Control Plane.md;
* Monitoring.md;
* Alerting.md;
* Development Guide.md;
* Coding Standards.md;
* Glossary.md.

The responsibilities defined herein prevail over implicit interpretations present in other documents.

---

# 32. Summary

The SeedGrid Worker architecture is based on the following principles:

* well-defined architectural responsibilities;
* decoupled asynchronous processing;
* absence of local persistent State;
* full observability;
* fault tolerance;
* implementation simplicity.

The mechanisms used to implement these principles remain free, provided they respect the contracts defined in this specification.

---

# 33. Final Statement

Workers constitute the official asynchronous processing layer of SeedGrid.

By defining clear responsibilities independent of execution mechanisms, the platform preserves its architecture, facilitates maintenance, and allows different implementation strategies without compromising system consistency.

The architecture establishes **what responsibilities exist**.

The implementation defines **how these responsibilities will be executed**.

This separation represents one of the fundamental principles of SeedGrid.
