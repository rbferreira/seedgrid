# SeedGrid

## Storage Specification

Version: 1.0

Status: Draft

Document: 06 - Storage.md

---

# 1. Objective

This document defines the storage architecture of SeedGrid.

Its goal is to establish how the platform represents, monitors, and manages storage resources used by Seed Nodes.

---

# 2. Philosophy

SeedGrid manages storage resources, not physical devices.

Every platform decision must be based on the characteristics of the available resource, regardless of the technology used to implement it.

---

# 3. Definition

A **Storage** represents a storage resource made available by a Seed Node.

This resource may correspond to:

* a physical disk;
* a partition;
* a logical volume;
* a dataset;
* a network share;
* any other resource capable of storing persistent data.

The platform does not assume any specific technology.

---

# 4. Responsibilities

The Storage layer must:

* represent storage resources;
* track capacity;
* track availability;
* provide information to the Scheduler;
* provide information to the Portal;
* integrate with the monitoring system.

The Storage layer does not perform download or torrent management operations.

---

# 5. Identity

Each Storage must have its own identity within the Cluster.

Minimum information:

* unique identifier;
* owning Seed Node;
* name;
* type;
* capacity;
* operational state.

The identity must remain stable as long as the resource exists.

---

# 6. Capacity

Every Storage must report:

* total capacity;
* used capacity;
* available capacity;
* utilization percentage.

This information must remain up to date to support Scheduler decisions.

---

# 7. State

Each Storage must have an operational state.

Defined states:

* Available;
* Degraded;
* Full;
* Read Only;
* Offline;
* Maintenance.

The state must reflect the current condition of the resource.

---

# 8. Health

In addition to the operational state, each Storage must have a health indicator.

The Health may consider:

* read errors;
* write errors;
* availability;
* integrity;
* information provided by the operating system.

The calculation method remains the responsibility of the implementation.

---

# 9. Inventory

All Storages must be part of the official Cluster inventory.

The inventory must allow identifying:

* location;
* capacity;
* utilization;
* state;
* association with the Seed Node.

---

# 10. Scheduler

The Scheduler uses Storage information when making decisions.

Examples:

* available space;
* operational state;
* administrative policies;
* remaining capacity.

The Scheduler must not depend on specific details of the storage technology.

---

# 11. Monitoring

All Storages must provide information to the monitoring system.

Examples:

* utilization;
* growth;
* availability;
* errors;
* temperature, when available.

This information may generate metrics, events, and alerts.

---

# 12. Scope

This document defines architectural concepts.

It does not establish specific storage technologies.

The implementation may use any technology compatible with the principles defined by the platform.

---

# 13. Objectives

The Storage architecture must:

* abstract physical technologies;
* facilitate Scheduler decisions;
* simplify expansion;
* reduce coupling.

---

# 14. Fundamental Principle

SeedGrid manages storage resources.

The physical implementation of these resources is the responsibility of the environment where the platform is deployed.



---

# 15. Configuration

Each Seed Node must have one or more directories configured for data storage.

Defining these directories is the responsibility of the operator.

SeedGrid only uses the paths made available by the Node.

---

# 16. Usage

The storage resources belong exclusively to the Seed Node.

The Control Plane does not perform direct file system management.

Local operations remain the responsibility of the Agent and the Torrent Engine.

---

# 17. Available Capacity

The Agent must periodically report consolidated information about the Seed Node's storage to the Control Plane.

Examples:

* total capacity;
* used capacity;
* available capacity;
* utilization percentage.

This information supports Scheduler decisions at the Node level.

---

# 18. Growth

Increasing the storage capacity of a Seed Node must occur through the expansion of the local environment.

Examples:

* disk replacement;
* volume expansion;
* addition of new devices.

After the inventory update, the Control Plane automatically recognizes the new available capacity.

---

# 19. Removal

If a storage resource is removed from the Seed Node, the Agent must update the corresponding inventory.

The platform must record events related to the change and provide information to the operator.

---

# 20. Discovery

The storage resources used by SeedGrid must be automatically identified by the Agent.

The inventory must represent only the resources explicitly made available for use by the platform.

---

# 21. Monitoring

Each Seed Node must provide consolidated information about its storage.

Examples:

* utilization;
* available space;
* consumption growth;
* availability;
* operational state.

This information will feed the monitoring system.

---

# 22. Alerts

The storage layer may generate alerts.

Examples:

* insufficient space;
* accelerated consumption growth;
* storage unavailability;
* unexpected switch to read-only mode.

The criteria will remain configurable.

---

# 23. Portal

The Portal must present consolidated information about the storage of each Seed Node.

Recommended information:

* total capacity;
* utilization;
* available space;
* growth trend;
* operational state;
* related alerts.

---

# 24. Expansion

The expansion of the Cluster capacity will occur naturally through:

* expanding the capacity of existing Seed Nodes; or
* adding new Seed Nodes.

None of these operations must require structural changes to the platform architecture.

---

# 25. Principles

The Storage architecture must:

* abstract file system details;
* maintain independence from storage technology;
* provide consistent information to the operator;
* support monitoring and capacity planning;
* remain decoupled from the Scheduler's architectural decisions.


---

# 26. Backup

The storage used by the Seed Nodes may follow independent backup policies.

SeedGrid must record and present the configured policy for each Seed Node, without assuming direct responsibility for protecting the stored data.

Backup strategies are defined in **Backup.md**.

---

# 27. Disaster Recovery

In recovery scenarios, storage must be treated as a resource belonging to the Seed Node.

The replacement or recovery of a Node must follow the procedures described in **Disaster Recovery.md**.

The Control Plane must keep the Cluster State independent of the physical storage implementation.

---

# 28. Audit

Administrative changes related to storage must generate audit records when applicable.

Examples:

* changing storage paths;
* enabling or disabling storage areas;
* entering or exiting maintenance mode;
* configuration changes.

The audit must record:

* date and time;
* operator;
* affected Seed Node;
* executed operation;
* result.

---

# 29. Observability

The storage layer must integrate with the platform's observability model.

Whenever applicable, it must produce:

* metrics;
* events;
* alerts;
* operational logs.

Each mechanism must fulfill its specific purpose, as defined in **Glossary.md**.

---

# 30. Security

Access to storage must respect the security principles defined by the platform.

SeedGrid must not require privileges beyond those necessary for its operation.

Whenever possible, it must adopt the principle of least privilege.

---

# 31. Evolution

New features related to storage must preserve the simplicity of the V1 architecture.

Evolutions that modify the storage architectural model must be planned for future versions and preceded by a specific ADR.

---

# 32. Relationship with Other Documents

This document complements:

* Architecture.md;
* Infrastructure.md;
* Scheduler.md;
* Monitoring.md;
* Alerting.md;
* Backup.md;
* Disaster Recovery.md;
* Glossary.md.

Whenever there are doubts regarding storage-related responsibilities, this document must be considered the official reference.

---

# 33. Summary

The SeedGrid storage architecture is based on the following principles:

* storage belongs to the Seed Node;
* the Control Plane manages information, not devices;
* the Agent collects and reports storage information;
* the Scheduler uses consolidated information at the Node level;
* the platform remains independent of the storage technology used.

These principles ensure simplicity, low coupling, and controlled evolution of the platform.

---

# 34. Final Statement

The SeedGrid storage architecture was designed to abstract implementation details without losing operational visibility.

By treating storage as a resource managed by the Seed Node, the platform maintains a simple, consistent architecture prepared for future evolutions, preserving the principles established for version 1.0.

---

# 35. Seed Node Protection against Missing, Late, or Remounted Data Disk

The Engine writes torrent data to `/data`. When `/data` **is not** the data disk — because it has not yet been mounted, was mounted after the Engine came up, or was unmounted and remounted — the default behavior is the worst possible case, and **silent**: the Engine keeps writing, but to the **system** disk. The Node remains ONLINE, the Engine remains reachable, and the measured free space becomes that of the root, so it appears to have room and keeps accepting torrents until the system disk fills up.

This is only a defect for those who **have** a data disk. A single-disk Node — a VM, a mini PC, a NAS — uses the system disk by choice, and for it "`/data` is a folder" is the correct and permanent state. The two cases are **indistinguishable** by looking at the current `/data` (an unmounted disk looks like a local folder), so SeedGrid does not try to distinguish them at runtime: the operator **declares** during installation which case applies to this Node, and the Agent obeys (§35.3). **The chain below protects the Node that declared a dedicated disk**; a single disk is a supported configuration, with the guarantees described in §35.3.

Protection is not a single mechanism. It is a chain of independent layers, and each one covers a failure that the previous one does not see:

| # | Layer | Who | Failure it covers |
|---|---|---|---|
| 1 | **UUID-based mounting, absence-tolerant** | `bootstrap-node.sh` (`/etc/fstab`) | Disk present at boot: mounts by filesystem UUID (`/dev/sdX` changes between boots) with `nofail` and `x-systemd.device-timeout`, so a missing disk **does not prevent the OS from booting**. |
| 2 | **`/data` health check** | Agent (`StorageHealth`, every heartbeat) | `/data` that is not a *mountpoint* (same `st_dev` as parent directory) or that does not accept writes. The reason (`não é mountpoint`, `somente leitura`, …) goes into the heartbeat as `storage_ok=false` + `storage_detail`. **The mountpoint requirement only applies with a declared dedicated disk** (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`, the default — §35.3); the write requirement always applies. |
| 3 | **`NODE_STORAGE` Alert** (CRITICAL) | Control Plane | The failure **sustained** for `alerting.storage_grace` (1m by default) becomes an alert and notification. The grace period exists so an I/O hiccup does not scream; resolution has no grace period. |
| 4 | **Scheduler Exclusion** | Control Plane | Node with `storage_ok = false` **receives no work**, from the very first bad read (there is no grace period here: it is protection, not an indicator). `NULL` — Agent that does not report the field — remains eligible. |
| 5 | **udev Reattachment** | `seedgrid-storage-reattach` (udev rule + oneshot unit, installed by bootstrap when `/data` has a UUID) | Disk that **drops and comes back** (re-enumerates with a different `/dev/sdX`): remounts `/data` if needed, **restarts the Engine** and rechecks torrents in error. |
| 6 | **Mount propagation to the Engine** | `docker-compose.yml` (`propagation: rslave`) | Disk mounted on the host **after** the Engine starts: the new mount propagates into the container without recreating it. |
| 7 | **Attach later** | `bootstrap-node.sh --storage-only` | Node that was born without a disk: mounts by UUID, installs layer 5, reconnects the Engine to the new `/data` and **declares the dedicated disk** (`SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=true`, §35.3). Idempotent; discovers the Control Plane via the Agent env. |

Layers 1, 5, 6 and 7 **prevent or repair** the problem on the Node itself; layers 2, 3 and 4 exist because the others can fail and the operator needs to know, and because no new work must go to a disk that is not there.

On a **single-disk** Node there is no disk that can be missing, so layers 1, 5, 6 and 7 do not apply (there is nothing to mount, reattach or propagate), and the *mountpoint* requirement of layer 2 does not exist. What remains is what still makes sense: the write probe (a read-only `/data` is a defect in any topology, and triggers layers 3 and 4) and free space, monitored by `NODE_DISK_FULL` and the Scheduler's space filter.

## 35.1 Mount propagation (layer 6)

By default, Docker mounts a bind with `rprivate` propagation: what the host mounts **later** on `/data` does not propagate to the container's mount namespace. The Engine continues to see the old directory — the system disk — even with the data disk already mounted on the host. The Seed Node compose uses the long syntax to choose `rslave`:

```yaml
- type: bind
  source: /data
  target: /data
  bind:
    propagation: ${SEEDGRID_DATA_PROPAGATION:-rslave}
```

The behavior was **measured**, not assumed:

| Scenario | Result |
|---|---|
| Container starts with `/data` still being the system directory; the disk is mounted on the host afterwards | `rprivate`: the container continues to see the old directory. **`rslave`: the container sees the disk.** |
| (`rslave`) The disk is unmounted and mounted again, without the Engine having an open file on it | The unmount and the new mount propagate: the container goes back to seeing the system directory and then the disk. |
| (`rslave`) The Engine **has an open file** on the old disk and the host remounts | The container **remains the owner of the old mount**: the new mount does not replace it, and the handles point to the device that disappeared. |
| `docker restart` after any of the above cases (`rprivate` or `rslave`) | The container starts seeing the mount that the host has at that moment. |

Two operational conclusions:

1. **Propagation solves "the disk arrived later", not "the disk dropped and came back".** In the second case, the Engine continues with handles to the old device, and only a restart drops them — which is why layer 5 continues to restart the Engine. It is also not necessary to *recreate* the container: the restart is enough, because the container starts with the mount that the host has at that moment.
2. **When `/data` is a bind of a host directory** (Node in a system container, where the hypervisor mounts the disk and delivers it via bind), the hypervisor mount does not propagate into the live Node — the bind is resolved when the system container starts. In this topology, the recovery of a disk that drops and comes back is on the hypervisor side (restarting the system container after remounting), and layer 6 only applies to disks mounted *inside* the Node.

### Prerequisite: the mount containing `/data` cannot be `private`

`rslave` requires the mount containing the source directory to be shared or *slave*. If it is `private`, Docker **refuses to create the container** (`path /data is mounted on / but it is not a shared or slave mount`) — **it is not a no-op, it is a failure**, and the Engine does not start. On a systemd host, the root is already shared; only an init that does not make this adjustment runs into the problem. To avoid turning a hardening into a defect, the bootstrap checks beforehand:

```sh
findmnt -no PROPAGATION -T /data    # shared | slave | shared,slave  →  rslave works
                                    # private                        →  the bootstrap writes rprivate
```

And, in the second case, it writes `SEEDGRID_DATA_PROPAGATION=rprivate` to the compose `.env` (the previous behavior, with a warning). The same value can be set manually. To check what a running Engine is using:

```sh
docker inspect seedgrid-engine -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Propagation}}{{end}}{{end}}'
# rslave
```

## 35.2 Propagation and Existing Nodes

The compose file is read at container creation: an Engine created before this change remains with `rprivate` until it is recreated from the new compose file (`docker compose up -d`). The procedure is in `docs/19 - Deployment.md` §50.4.

## 35.3 Dedicated disk or single disk: the `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT` declaration

What determines whether `/data` needs to be a *mountpoint* is a **per-machine** variable in the Node's `/etc/seedgrid-agent.env`, written by `bootstrap-node.sh` during installation:

| Value | Meaning | The Agent checks | When the bootstrap writes it |
|---|---|---|---|
| `true` | `/data` is a **dedicated disk** | mountpoint **and** write access | it mounted a disk at `/data`, or `/data` was already mounted; and always on `--storage-only` |
| `false` | `/data` is a folder on the **system disk** (single disk) | write access only | there was no disk to mount: `/data` ended up on the system disk |
| *(missing)* | — | as `true` | — (this is what every Node prior to the variable has) |

**Why declared, and not inferred.** The natural inference — "if `/data` is a folder and not a mountpoint, I do not require a mountpoint" — fails precisely in the case the rule exists to catch: a dedicated disk that **did not mount** is, from the Node's perspective, a local folder. Any runtime detection would turn the silent failure that §35 combats into a "healthy" state. Therefore, the expectation is written **once, during installation**, when the operator (and the script) know what was done, and the Agent treats it as a given — it never re-evaluates it. **Missing is `true`**: a Node that never had the variable remains exactly as it was, and any misconfiguration fails safely (Node excluded, never an unmounted disk silently accepted). A non-boolean value (`flase`, `nao`) also evaluates to `true`.

**What the bootstrap does in each path**

- **Mounted, or found, a dedicated disk** → writes `true`.
- **Did not find a disk** (`/data` ended up on the system disk) → writes `false` and **warns in the terminal**: the Node will store torrents on the system disk, the Agent will not require a *mountpoint*, free space is tracked via `NODE_DISK_FULL`, and how to attach a disk later (`sh /opt/seedgrid-node/bootstrap-node.sh --storage-only`).
- **`--storage-only`** (attach the disk later) → writes `true` and restarts the Agent, which only re-reads the env on startup. Idempotent.
- **A second run never downgrades a Node that has already declared a disk.** If the bootstrap runs again without finding the disk, but the previous env already said `true` (or `fstab` already mounts `/data`), the value remains `true`: the disk **disappeared**, the Node did not become "single disk". Only an explicit declaration downgrades it.
- **Operator overrides**, for things the script has no way of knowing: `STORAGE_REQUIRE_MOUNT=true` declares a dedicated disk even without a disk *right now* — the case of a disk that arrives a few minutes after installation, where the Node must stay out of the Scheduler until it mounts, instead of accepting torrents on the system disk —; `STORAGE_REQUIRE_MOUNT=false` deliberately reverts a Node to the system disk. Any other value aborts the script before changing anything.

**What the single-disk Node gets and what it does not get**

- It keeps `storage_ok = true` as long as `/data` accepts writes, is eligible in the Scheduler, and **does not open `NODE_STORAGE`**.
- The measured disk (`disk_total_bytes`, `disk_free_bytes`) becomes the **system** disk, so it is the one that feeds the Scheduler's space filter (ADR-032) and `NODE_DISK_FULL`. A margin (`admission.disk_min_free`) that also covers what the operating system needs is advisable.
- **There is no protection against "the disk did not mount"** — there is no disk. If a disk is attached outside of the bootstrap (manually mounted at `/data`) on a Node declared as `false`, the requirement **remains disabled**: the Agent does not infer. Use `sh /opt/seedgrid-node/bootstrap-node.sh --storage-only`, which mounts, sets up reattachment, and changes the Node to `true`.

**Changing the declaration on an existing Node**: single disk → dedicated disk is done via `--storage-only` (above). Dedicated disk → single disk (the disk was removed for good) is done by editing `SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT=false` in the env and restarting the Agent — or by running the bootstrap with `STORAGE_REQUIRE_MOUNT=false`. Nodes prior to the variable require no action: missing is already `true`. See `docs/19 - Deployment.md` §50.4.
