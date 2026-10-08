# **SeedGrid**
**Development Guide**
Version: 1.0
Status: Draft
Document: 20 - Development Guide.md
# **1. Objective**
This document defines the organization of the SeedGrid development environment.
Its objective is to standardize how new developers set up the environment, understand the project structure, and contribute to its evolution.
# **2. Philosophy**
The development environment must be:
- simple;
- reproducible;
- documented;
- predictable.
A new developer must be able to start their contribution using only the official documentation.
# **3. Target Audience**
This document is intended for:
- developers;
- maintainers;
- reviewers;
- external contributors.
# **4. General Organization**
The project will be divided into independent components.
Suggested structure:
seedgrid/
├── control-plane/
├── portal/
├── agent/
├── scheduler/
├── api/
├── shared/
├── infrastructure/
├── deployment/
├── docs/
├── scripts/
└── tests/
Each component must have a clearly defined responsibility.
# **5. Component Organization**
Each service must follow a similar structure.
Example:
component/
├── src/
├── tests/
├── docs/
├── config/
├── migrations/
├── Dockerfile
└── README.md
This standardization facilitates navigation and maintenance.
# **6. Documentation**
Each component must have its own documentation.
Minimum items:
- objective;
- responsibilities;
- dependencies;
- configuration;
- execution instructions;
- available tests.
Specific documentation complements the architectural documentation.
# **7. Local Environment**
The development environment must allow local execution of the main components.
Objectives:
- facilitate debugging;
- accelerate development;
- reduce dependency on external infrastructure.
Whenever possible, auxiliary services must be executed in containers.
# **8. Dependencies**
All dependencies must be explicitly declared.
Implicit dependencies or undocumented manual configurations will not be accepted.
# **9. Configuration**
Configurations must remain external to the source code.
Examples:
- environment variables;
- .env files;
- YAML files;
- JSON files.
No environment-specific configuration must be embedded in the code.
# **10. Database**
The local environment must provide a PostgreSQL instance compatible with production.
Migrations must be used for all structural changes.
Manual changes to the database will not be considered an official part of the project.
# **11. Auxiliary Services**
During development, auxiliary services may be used.
Examples:
- PostgreSQL;
- Prometheus;
- Grafana;
- Loki;
- Reverse Proxy.
These components must be initialized in a reproducible manner.
# **12. Tests**
Each component must have tests compatible with its responsibility.
Expected types:
- unit tests;
- integration tests;
- functional tests.
The architecture must favor automated test execution.
# **13. Scripts**
Repetitive tasks must be automated.
Examples:
- start environment;
- stop environment;
- run tests;
- apply migrations;
- clean environment;
- generate documentation.
Scripts must remain versioned along with the project.
# **14. Objectives**
The development environment must reduce the time required for a new developer to understand the architecture and start contributing.
# **15. Fundamental Principle**
The development of SeedGrid must prioritize simplicity, consistency, and standardization.
Every contribution must respect the architecture defined in the official documents and the decisions recorded in the ADRs.
# **16. Development Flow**
Every new feature must follow a standardized flow.
Recommended flow:
Requirement
↓
ADR (when necessary)
↓
Implementation
↓
Tests
↓
Code Review
↓
Documentation
↓
Merge
No feature will be considered complete without documentation and compatible tests.
# **17. Organization of Changes**
Each change must have a single, clearly defined objective.
Whenever possible, distinct changes must be implemented in independent Pull Requests.
This practice facilitates review, testing, and traceability.
# **18. Migrations**
Structural changes to the database must use versioned migrations.
Principles:
- reversible, whenever possible;
- idempotent;
- documented;
- tested.
Manual changes to the database are not part of the official process.
# **19. APIs**
Every new API must follow the standards defined in API.md.
Minimum requirements:
- authentication;
- consistent error handling;
- versioning;
- documentation;
- automated tests.
No endpoint must access internal components directly without going through the layers defined in the architecture.
# **20. New Services**
The creation of a new service must be preceded by an architectural evaluation.
Each new component must have:
- single responsibility;
- its own documentation;
- monitoring strategy;
- logging strategy;
- tests;
- observability integration.
When it represents a significant architectural change, a new ADR must be created.
# **21. Dependency Handling**
External dependencies must be reduced to the minimum necessary.
When adding a new dependency, the developer must consider:
- project maturity;
- license;
- active maintenance;
- security impact;
- operational impact.
# **22. Test Environment**
The development environment must allow complete execution of the platform using test data.
Objectives:
- validate new features;
- reproduce failures;
- run automated tests;
- simulate multiple Seed Nodes.
No test must depend on production infrastructure.
# **23. Development Data**
Data used for development must be fictitious.
The following must never be used:
- real credentials;
- production certificates;
- personal information;
- operational data from the production environment.
# **24. Mock Components**
Whenever necessary, mock components may be used.
Examples:
- Mock Agent;
- Mock Torrent Engine;
- Mock API;
- mock events.
These components facilitate testing and reduce dependencies between teams.
# **25. Code Review**
Every contribution must undergo review.
The review must evaluate, at a minimum:
- adherence to the architecture;
- compliance with ADRs;
- code quality;
- test coverage;
- documentation updates.
# **26. Versioning**
All components must follow the same versioning strategy as the platform.
The published version must clearly identify the compatibility between:
- Control Plane;
- Agent;
- Portal;
- API;
- Scheduler.
# **27. Principles**
The development process must prioritize:
- simplicity;
- readability;
- predictability;
- documentation;
- testing;
- incremental evolution.
Every contribution must strengthen the consistency of the platform as a whole.
# **28. Definition of Done (DoD)**
A feature will only be considered complete when it meets the criteria defined in this section.
The goal of the Definition of Done is to ensure consistency, quality, and adherence to the platform's architecture.
# **Mandatory Criteria**
Whenever applicable, a delivery must meet the following requirements:
- implementation completed;
- automated tests executed successfully;
- documentation updated;
- adherence to current ADRs;
- APIs documented;
- migrations created and tested;
- logs added when necessary;
- metrics added when relevant;
- alert rules reviewed when applicable;
- monitoring updated when necessary;
- code review approved.
# **Architectural Criteria**
The implementation must respect the principles defined in the official documentation.
In particular:
- well-defined responsibilities;
- low coupling;
- communication through the expected components;
- absence of implicit dependencies;
- compliance with the service architecture.
# **Operational Criteria**
Whenever a change impacts the platform's operation, the following must be evaluated:
- monitoring;
- alerts;
- backup;
- Disaster Recovery;
- operational documentation.
# **Security Criteria**
Whenever applicable:
- authentication validated;
- authorization reviewed;
- proper error handling;
- protection of sensitive information;
- input validation.
# **Observability Criteria**
Every relevant feature must consider:
- log generation;
- operational metrics;
- events;
- traceability;
- audit, when applicable.
# **Evolution Criteria**
Whenever a change modifies the platform's architecture:
- an ADR must be created or updated;
- impacts must be documented;
- compatibility must be evaluated.
# **29. Continuous Integration**
The project must be compatible with Continuous Integration (CI) pipelines.
Objectives:
- automatic test execution;
- code quality validation;
- documentation verification;
- migration validation;
- artifact generation.
The architecture must favor automation from the beginning of development.
The CI includes the **Policies (Go)** job, which runs project policy checkers (the `tools/` module): the commit message format (`commitmsgcheck`), the architecture boundaries (`architecturepolicy`) and the code language (`languagepolicy`). The code is written in English — comments, log and error messages, test names and identifiers; the product speaks other languages only through its catalogs (ADR-041). A legitimate exception (a Brazilian site's category name the code must recognize, a real title cited as an example) goes in `tools/language-allow.txt` with its reason.

## **Commit Messages**

Commit messages follow the Conventional Commits format and are automatically verified in the CI by `commitmsgcheck`.

Rules:
- **Subject** (first line, no trailing spaces) in the conventional format:
  `^(feat|fix|chore|docs|refactor|test|perf|ci|build|style|revert)(\([A-Za-z0-9._/#-]+\))?!?: \S`
  Valid examples: `feat: x`, `fix(deploy): x`, `feat(api)!: x`, `chore: x (#8)` (GitHub squash).
- **Exempt**: messages generated by git itself — subject starting with `Merge ` or `Revert "`.
- **No co-authorship trailer**: any line matching `(?i)^\s*co-authored-by\s*:` fails.
- Empty message (or only comments) fails.
- In hook mode (`-file`), lines starting with `#` are git comments and are discarded before checking.

There are no language or size rules.

**Run manually:**

```bash
cd tools && go run ./cmd/commitmsgcheck -range origin/main..HEAD
```

**Install as a `commit-msg` hook:**

Create `.git/hooks/commit-msg` with the following content:

```bash
#!/bin/sh
# Git passes the file relative to the root (.git/COMMIT_EDITMSG): resolve it before cd.
case "$1" in /*) f="$1" ;; *) f="$PWD/$1" ;; esac
cd tools && exec go run ./cmd/commitmsgcheck -file "$f"
```

## **Architecture Boundaries**

Hard architectural boundaries are automatically verified in the CI by `architecturepolicy`.

Rules:
- **agent-no-database** — no `.go` under `agent/` imports `database/sql`, `github.com/jackc/`, or `github.com/lib/pq`. Protects: only the Control Plane accesses the database.
- **agent-no-control-plane** — no `.go` under `agent/` imports `github.com/rbferreira/seedgrid/control-plane` (or subpackages). Protects: Agent and Control Plane only communicate via the API.
- **control-plane-no-agent** — no `.go` under `control-plane/` imports `github.com/rbferreira/seedgrid/agent` (or subpackages). Protects: the Control Plane does not communicate directly with the Engine.
- **agent-no-worker** — no `.go` under `agent/` declares an identifier (package, type, function, method, parameter, variable, constant, struct field, interface method) containing `worker`, ignoring case. Comments and strings do not count. Protects: Worker is an internal Control Plane component and does not run on the Seed Node.
- **portal-network-only-via-api** — in `portal/src/**/*.ts` and `*.tsx`, except `portal/src/api.ts` and test files, no line uses `fetch(`, `new XMLHttpRequest`, `new WebSocket`, `new EventSource`, or `sendBeacon(`. Protects: all Portal network traffic goes through `api.ts`, which only communicates with the API.

**Run manually:**

```bash
cd tools && go run ./cmd/architecturepolicy -root ..
```
# **30. Evolution**
The development process must evolve alongside the platform.
New practices may be incorporated provided they respect the principles defined in this document and the ADRs.
# **31. Relationship with Other Documents**
The Development Guide complements the other project documents.
While the architectural documents define **what** must be built and **why**, this document establishes **how** the team must develop, validate, and maintain the platform over time.
# **32. Summary**
The official SeedGrid development process is based on:
- documented architecture;
- decisions recorded in ADRs;
- continuous documentation;
- automated testing;
- code review;
- observability;
- incremental evolution.
These principles must guide all contributions to the project.
# **33. Final Statement**
The Development Guide establishes the official engineering practices for SeedGrid.
Its goal is to ensure that the platform's evolution occurs consistently, predictably, and aligned with the architecture defined by the other documents, preserving technical quality, traceability, and maintainability throughout the project's entire lifecycle.
