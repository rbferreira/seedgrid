# **SeedGrid**
# **Alerting Specification**
Version: 1.0
Status: Draft
Document: 16 - Alerting.md
# **1. Objective**
The SeedGrid Alert system is responsible for identifying situations that require the operator's attention.
Its goal is to transform operational information into actionable notifications, reducing the time required to identify relevant issues.
# **2. Philosophy**
Not every event generates an alert.
Not every metric generates an alert.
An alert represents a condition that deserves the operator's attention.
This distinction must be preserved across the entire platform.
# **3. Responsibilities**
The Alert system must:
- evaluate conditions;
- create alerts;
- update existing alerts;
- close alerts automatically when applicable;
- make information available to the Portal.
The system must NOT:
- execute corrective actions;
- modify settings;
- change Node states.
These responsibilities belong to the Control Plane.
# **4. Architecture**
Metrics
Events
↓
Alert Service
↓
Database
↓
Portal
↓
Operator
# **5. Sources**
Alerts can originate from:
Metrics.
Events.
Health Score.
Scheduler.
API.
Control Plane.
SeedGrid Agent.
Torrent Engine.
# **6. Lifecycle**
Every alert will follow the same lifecycle.
Detected
↓
Open
↓
Acknowledged
↓
Resolved
↓
Closed
Not all alerts will need to pass through the "Acknowledged" state.
# **7. Severity**
Version 1.0 will use four levels.
# **Informational**
Relevant information.
Do not require action.
# **Warning**
Situations that require monitoring.
Do not yet represent an operational failure.
# **Critical**
Issues that require immediate attention.
May compromise part of the Cluster.
# **Emergency**
Issues that significantly compromise the platform's operation.
Require priority intervention.
# **8. States**
Every alert must have a state.
Expected states:
Open.
Acknowledged.
Resolved.
Closed.
# **9. Grouping**
Whenever possible, related alerts must be grouped.
Example
Loss of communication from a Node must generate only one active alert.
New related events will update the same alert.
# **10. Automatic Alerts**
The system must open alerts automatically when previously defined conditions are met.
Examples:
Node Offline.
Disk out of space.
Critical SMART.
Engine unavailable.
Control Plane unavailable.
# **11. Closure**
Whenever possible, alerts must be closed automatically.
Example
Node is responding again.
↓
Alert closed.
The operator must not need to manually close alerts resolved by the system itself.
Alerts that the system cannot confirm as resolved — reachability alerts, which depend on an inbound connection that an idle Node never receives — **expire** after a configurable validity period, without notifying and without reopening the same episode (see §46).
# **12. History**
Every alert must maintain a history.
Examples:
Opening date.
Severity changes.
Acknowledgment.
Resolution.
Closure.
# **13. Relationship with Events**
An alert can be related to multiple events.
Events represent facts.
The alert represents the operational condition derived from these facts.
# **14. Objectives**
The alert system must:
reduce noise;
prioritize important issues;
facilitate diagnosis;
improve response time.
# **15. Fundamental Principle**
Alerts represent issues.
Events represent occurrences.
Logs represent technical details.
These three categories must remain clearly separated throughout the architecture.
# **16. Alert Rules**
Every alert rule must have objective criteria.
Each rule must define:
- activation condition;
- closure condition;
- severity;
- category;
- description;
- monitored resource.
The rules must be deterministic.
# **17. Categories**
Alerts will be organized by category.
Expected categories:
Infrastructure.
Storage.
Network.
Control Plane.
Nodes.
Torrent Engine.
Scheduler.
API.
Security.
Configuration.
This organization will facilitate filters and dashboards.
# **18. Control Plane Alerts**
Examples:
API unavailable.
Database unavailable.
Scheduler stopped.
Excessive CPU usage.
Excessive memory usage.
Insufficient space.
Recurring failures.
# **19. Node Alerts**
Examples:
Node Offline.
Expired heartbeat.
Critical Health Score.
High temperature.
Excessive CPU.
Insufficient memory.
Abnormal uptime.
# **20. Storage Alerts**
Examples:
Space below threshold.
Critical SMART.
Disk removed.
Read-only file system.
Mount failure.
Read errors.
Write errors.
# **21. Torrent Engine Alerts**
Examples:
Engine unavailable.
Queue stopped.
Authentication error.
Communication failure.
Excessive number of errors.
# **22. Scheduler Alerts**
Examples:
No eligible Nodes.
Excessive decision time.
Consecutive failures.
Abnormal distribution.
All Nodes unavailable.
# **23. API Alerts**
Examples:
High response time.
Excessive amount of errors.
Authentication failures.
Requests above expectations.
# **24. Grouping Rules**
Equivalent alerts must be consolidated.
Examples:
Expired heartbeat.
Node Offline.
Communication failure.
These conditions can represent a single incident.
# **25. Suppression**
The system must avoid repetitive generation of alerts.
Examples:
A Node offline for an hour will continue to be represented by a single active alert.
Subsequent events will only update its history.
# **26. Dependencies**
When possible, dependencies must be considered.
Example:
If the Control Plane is unavailable, individual alerts must not be generated for all expired heartbeats.
The root cause must be prioritized.
# **27. Acknowledgment**
The operator may acknowledge an alert.
The acknowledgment indicates that the problem is known.
It does not alter the monitored condition.
While the condition persists, the alert will remain active.
# **28. Resolution**
An alert will be considered resolved when its condition ceases to exist.
The resolution may occur:
automatically;
or through administrative intervention, when applicable.
# **29. Retention**
The alert history must remain available for audit.
The retention policy will be configurable.
Closed alerts may be archived without impacting operational queries.
# **30. Query**
The Portal must allow queries using filters.
Examples:
Period.
Category.
Severity.
Node.
State.
Free text.
Operator responsible for the acknowledgment.
# **31. Principles**
The alert system must produce few, yet highly relevant notifications.
The absence of alerts must mean that the platform is operating normally.
An excess of alerts must be treated as a platform configuration issue, and not as expected behavior.
# **32. Integration**
The alert system must integrate naturally with the other platform components.
Simplified flow:
Metrics
Events
↓
Alert Service
↓
Database
↓
Portal
The Alert Service must not depend directly on the Agents.
# **33. Alerts Dashboard**
The Portal must provide a dashboard dedicated to active alerts.
Minimum information:
- count by severity;
- count by category;
- recent alerts;
- acknowledged alerts;
- critical alerts;
- recent history.
The goal is to allow quick identification of priority issues.
# **34. Correlation**
Whenever possible, related alerts must share context.
Examples:
Same Node.
Same disk.
Same Torrent Engine.
Same period.
This correlation will facilitate diagnosis by the operator, without altering the individuality of the recorded Events.
# **35. Audit**
The following operations must generate audit records:
Acknowledgment.
Manual closure.
Severity change.
Configuration change.
These records will remain separate from the alerts themselves.
# **36. Configuration**
The criteria used by the Alert Service must be configurable.
Examples:
Limits.
Timings.
Retention.
Categories.
Policies.
The configuration will remain centralized in the Control Plane.
# **37. Availability**
The temporary unavailability of the Alert Service must not interrupt:
- Node operation;
- Scheduler;
- Control Plane;
- Torrent Engine.
The impact will be limited to the generation and updating of alerts.
# **38. Performance**
Version 1.0 objectives:
Low CPU consumption.
Low memory consumption.
Low latency.
Continuous evaluation of rules without noticeable impact on the rest of the platform.
# **39. Scalability**
The Alert Service must support the gradual growth of the Cluster.
An increase in the number of Nodes, Torrents, or Events must not require changes to the component's architecture.
New alert categories may be added without changing the existing contracts.
# **40. Testability**
Alert rules must be testable in isolation.
Each condition must always produce the same result when subjected to the same set of information.
Automated tests must validate:
- activation;
- termination;
- grouping;
- suppression;
- severity.
# **41. Architecture Summary**
Prometheus
│
│
Metrics
│
▼
┌─────────────────┐
│  Alert Service  │
└─────────────────┘
▲
│
Events
│
Control Plane
│
▼
PostgreSQL
│
▼
Portal
│
▼
Operator
The Alert Service consolidates operational information and identifies conditions that require the operator's attention.
# **42. Best Practices**
The system must follow the following principles:
- prioritize quality over quantity of alerts;
- avoid repetitive notifications;
- favor the root cause when identifiable;
- maintain a complete history;
- allow auditing;
- facilitate investigation.
# **43. Final Statement**
The SeedGrid Alert system transforms metrics and Events into actionable information.
Its role is to highlight only conditions that deserve the operator's attention, reducing operational noise and enabling quick responses to real issues.
By remaining decoupled from the collection, execution, and monitoring mechanisms, the Alert Service preserves the modularity of the architecture and reinforces the separation of responsibilities established across the entire platform.

---

# **44. Webhook contract — versioned `data` block (#147)**

The **webhook** channel (ADR-028) `POST`s the entire `notify.Message` in JSON. In addition to the
fields that text channels render, alerts that exist to **trigger automation**
carry a structured `data` block, versioned by `schema`.

## Why `data` and not `fields`

`fields` is a `{label, value}` list **for humans to read** — it is what Telegram and Discord
turn into text, and changes whenever someone improves a phrase. Automation that depended
on it would break with every copy adjustment. `data` is the machine contract and does not change for
aesthetics.

## Versioning

`schema` is an integer and increments **only on breaking changes**:

- new field inside `data` → **does not** increment;
- removing a field, or changing the meaning of an existing one → **increments**.

The consumer must check the number and **fail loudly** if it does not recognize it, instead of processing
it incorrectly in silence. A message without a structured block omits `schema` and `data` — those who already
consume the webhook today see no difference.

## `NODE_IPV6_CHANGED` — `schema: 1`

```json
{
  "kind": "alert",
  "type": "NODE_IPV6_CHANGED",
  "severity": "WARNING",
  "title": "IPv6 do Node mudou — as regras de firewall que liberavam … não valem mais",
  "node": "seedgrid-node-01",
  "fields": [
    { "label": "Anterior", "value": "2001:db8:601:d300:be24:11ff:fe12:dee1" },
    { "label": "Atual",    "value": "2001:db8:60b:c000:be24:11ff:fe12:dee1" },
    { "label": "Porta BT", "value": "6881" }
  ],
  "at": "2026-09-07T04:12:33Z",

  "schema": 1,
  "data": {
    "event": "node_ipv6_changed",
    "node": {
      "id": "b4f312bb-cd99-4463-9fd3-276e626b17ff",
      "hostname": "seedgrid-node-01",
      "bt_port": 6881
    },
    "ipv6": {
      "previous":        "2001:db8:601:d300:be24:11ff:fe12:dee1",
      "current":         "2001:db8:60b:c000:be24:11ff:fe12:dee1",
      "prefix_previous": "2001:db8:601:d300::/64",
      "prefix_current":  "2001:db8:60b:c000::/64"
    }
  }
}
```

### `prefix_*` is calculated by the Control Plane, on purpose

What the firewall rule needs is the **`/64` network object**, not the full address —
and it was exactly the **literal** address that caused the incident on 05-06/09/2026: the rules
survived the prefix rotation by pointing to addresses that no longer existed. Letting the
consumer derive the prefix with string manipulation would plant the same bug on the other side
of the integration.

`prefix_previous`/`prefix_current` come as `""` when the address could not be safely reduced with
security. **A wrong prefix is worse than no prefix**: a rule written with an incorrect `/64`
silently opens what it shouldn't, or closes what it should open. In this case the
consumer still has the full address and decides what to do.

### For consumers (n8n)

1. filter by `type == "NODE_IPV6_CHANGED"`;
2. **check `schema == 1`** and abort with a visible error if it is another;
3. use `data.ipv6.prefix_current` to rewrite the gateway rule, and
   `data.node.bt_port` for the port;
4. do not parse `title`, `message` or `fields` — they change.

> **With the deterministic suffix (ADR-038) the new address is arithmetic.** A Node that applied the convention has the address `<prefixo /64>::5eed:<N>`, where `N = bt_port − 6880` (decimal in the last group: Node 12 is `::5eed:12`). After a prefix rotation only **one** unknown remains — the `prefix_current` — and the address of each Node is `prefix_current` + `::5eed:` + `N`; without the convention, the suffix came from the MAC and each address had to be discovered, Node by Node. The token only fixes the suffix: the prefix rotation **continues** to invalidate the rules, and this alert continues to be the way to find out.

The alert **resolves itself** when the Node starts receiving inbound connections again after the
change (see §Rules, `NODE_IPV6_CHANGED`), or **expires** after `alerting.reachability_ttl`
(§46), so the automation does not need to close anything. Expiring **does not** deliver a webhook: the
automation receives one call per rotation (plus one for each subsequent rotation, #209) and
none for validity closure.

### Next rotation with the alert still `OPEN` (#209)

A Node with no demand in the swarm may not receive inbound traffic for days — the alert stays `OPEN`
waiting to resolve. If the IPv6 rotates **again** in this interval, the webhook fires
again, with `data.ipv6.current` already reflecting the MOST RECENT address (not the one that opened
the alert originally). It is not a new condition: the Control Plane **updates** the alert
existing instead of opening a second line, and notifies again via the same path as
`AlertOpened` — never via the resolution one, because nothing was fixed. The automation must treat
each delivery of this webhook as "the current address now is this one", not assume that it only
receives it once per incident.


---

# **45. Resolution also notifies (#161)**

Every alert that the Alert Worker closes goes through `alert.Service`, which is what triggers the
hook `AlertResolved` from telemetry (ADR-028). Before, the five rules closed with an `UPDATE`
directly on the table: the `notify.alerts_on_resolve` tunable came **enabled by default** and
promised to "notify when resolved", but `Service.Resolve` did not have a single caller in
production. Opening notified; resolving did not — with no error in the log and nothing in the Portal
indicating this.

The `resolveVia` helper centralizes the closure: it selects the Nodes that satisfy the
resolve condition and calls `Service.Resolve` **one per Node**. It is one extra query per
closed alert, and it is what allows the Service to return severity, message and payload of
each alert — without them the notification would go out without a hostname or reason.

It applies to `NODE_OFFLINE`, `NODE_STORAGE`, `NODE_DISK_FULL`, `DISK_IO_SATURATED`,
`NODE_FIREWALLED`, `NODE_INBOUND_SILENT` and `NODE_IPV6_CHANGED`.

⚠️ **Burst:** a general outage turns into a general recovery, and the resolve of all Nodes goes out together. The
`notify.rate_limit_per_min` limit (per channel) is what holds this back; a channel with a low limit
discards the excess instead of queuing.

Exception: closure by **validity** (§46) does not notify.


---

# **46. Validity of reachability alerts (#211)**

`NODE_FIREWALLED`, `NODE_INBOUND_SILENT` and `NODE_IPV6_CHANGED` deduce that the Node is
unreachable because **no inbound connection arrived**, and only resolve when one
arrives. On a Node with no demand in the swarm it never arrives, with the port open or not, and the
alert stayed open forever. Measured on 24/09/2026: four open alerts, all
false — each Node's port checked from the outside, open; one of the Nodes with 0 leechers
across 116 torrents.

What matters in these alerts is the **outbound call** on opening (webhook, Telegram,
Discord). After `alerting.reachability_ttl` (default 24h; 0 = no expiry) the
alert **expires**:

- closes as `RESOLVED` with `expired = true`, **without notifying** — "Resolved" would be
  a lie, nothing was fixed. The Portal shows *expired*;
- the expiry counts from the **last notification** (`updated_at`): a new rotation with the
  alert open notifies again and renews it;
- the **same episode does not reopen**. Each alert records in `episode_key` the identity of the
  condition — the rotation (`public_ipv6_changed_at`), the start of the silence
  (`last_incoming_at`), the Engine verdict (`connectable_since`) — and an episode that already
  generated an alert, in any status, does not generate another. Without this the condition, which still
  holds, would reopen the alert on the next pass and notify on every cycle of the
  expiry. The same mechanism eliminates re-notification on every **reboot** (leaving
  ONLINE closes the alert; the return would reopen it);
- expiry runs even with the rule disabled, to avoid orphans.

**Discarded:** confirming the port with an external probe before closing. It is the right
measure, but it would require every operator to maintain a system talking to an endpoint of
Control Plane integration — a regular user will not do that, and the alert cannot
depend on something that most installations do not have.

---

# **47. Sustained disk saturation (#150)**

`DISK_IO_SATURATED` opened with **a single** reading above the threshold (`alerting.disk_saturated_pct`,
90). The reading is `nodes.disk_io_util_pct`, the ~10s average between two heartbeats, and a USB HDD is
sawtooth by nature — 87% in one interval, 13% five minutes later (node-04, 06/09/2026).
A seconds-long spike became an alert; measured in the production database, of the 18 alerts opened between
12/09 and 02/10/2026, **16 lasted exactly 30s** — one Alert Worker pass; the other two
lasted 9min (node-03) and 4m30 (node-02), the kind of episode the rule still catches if the
saturation sustains. The Portal chart
(Prometheus, 2-minute window) never showed these spikes, and the operator learned to ignore
an alert that looked false — the worst possible outcome for an alert.

Now the alert only opens after the disk stays **continuously** above the threshold for
`alerting.disk_saturated_for` (default 90s; 0 = opens on the first reading, as before):

- the Node must be ≥ threshold on **all** worker passes during the grace period. A
  pass below the threshold — or with the Node not ONLINE or without a recent heartbeat — **resets the
  count**;
- the granularity is that of the worker (`alerting.worker_interval`, 30s): with the defaults the opening
  fires on the 4th consecutive reading. The comparison has a slack of 1/10 of the interval, so that the millisecond
  jitter of the passes does not push the opening a full pass forward when the
  grace period is a multiple of the interval;
- **resolution has not changed**: no grace period (a disk that eased up is good news) and with the 15pp
  hysteresis. A reading between the slack and the threshold keeps the alert open but does not count as saturated,
  so reopening after resolving requires an entirely new sequence;
- the start of the sequence lives **in memory** in the Worker (`satSince`), per Node. Restarting the
  Controller resets the count — it only **delays** the alert by up to the grace period, it never creates a false one. An
  already-open alert is in the database and is not affected.

**Discarded (operator decision, 03/10/2026):** persisting `disk_io_util_pct` in `heartbeats`
(migration, so there would be a history of the series that opens the alert) and displaying that series on the Node
card. The cause of the noise was opening on a single reading, which the sustained rule resolves; the
history would be a separate feature. The in-memory state requires no migration and does not touch the hot
path of the heartbeat — the persisted timestamp (like `storage_failing_since` from #177) would only gain
the view of each 10s heartbeat, while the worker already sees the signal every 30s.

---

# **48. NODE_INBOUND_SILENT: weighted demand and text that does not go stale (#206, #203)**

`NODE_INBOUND_SILENT` infers that the Node became unreachable because no inbound connection has
arrived for `alerting.incoming_silence`, **given demand**. It covers the case that
`NODE_FIREWALLED` does not catch — the Node that *was* reachable and stopped being, without changing
address — because the Engine verdict is sticky (see `25 - Business Rules.md`). That is why it stays; what changed
is the **precision**.

The demand was "there is a leecher in any torrent on the Node, at the moment of the pass", and this
treats as the same thing two unrelated scenarios: a leecher in a starved swarm *needs* the
Node; a leecher in a swarm with 63 seeds opens connections with half a dozen of them, and not being one of the
chosen ones for hours is the expected behavior of a healthy Node. Measured on 09/09/2026,
the six torrents that were arming the alert on Nodes with the port verified from outside and open had
2 to 63 seeds per leecher. In total, of the 27 alerts opened between 08/09 and 30/09/2026 in a
cluster of four Nodes, **only the first one has a documented real problem**: 14 closed because
the Node left ONLINE, 2 expired (§46) and 11 because an inbound connection arrived.

## 48.1 Demand weighted by competition

A torrent only counts as demand when

```
num_leechers > 0   e   num_seeds <= num_leechers × alerting.incoming_demand_seeds_per_leecher
```

The default is **1**: leechers are at least as numerous as seeds — there are more mouths than
servers. `num_seeds` is the swarm total according to the tracker and may already include the Node itself,
hence 1 leecher / 1 seed **arms** (the only seed is probably us) and 1 / 2 does not.

| Swarm (leechers / seeds) | Matches the default? |
|---|---|
| 1 / 0 | **yes** — the leecher has no one to download from |
| 1 / 1 | **yes** |
| 3 / 3 | **yes** — equality counts (the rule is `<=`) |
| 1 / 2, 3 / 17, 1 / 63 | no |
| `num_leechers` or `num_seeds` NULL | no — unknown never triggers |

- Demand belongs to the **Node**: a single needy swarm among its torrents is enough. Saturated swarms and those
  without leechers are excluded from the count.
- The tunable is a decimal. `0` is the strictest reading (only a swarm **with no seeds**), not
  "disabled"; a very high value (e.g., 100000) reverts to the old behavior, where any
  leecher counts. The entire rule still disables via `alerting.incoming_silence = 0`.
- The alert records in the payload only what triggered it (`demand_torrents`, `demand_leechers`,
  `demand_seeds`) and the opening log repeats all three. There is no peer history in the database; this is
  the only trace of *why* it opened, and what will allow recalibrating the tunable with field data.
- **Known limitation, retained:** demand is evaluated at the *instant* of the pass, not accumulated
  over the window. Weighting reduces the noise from this limitation, it does not eliminate it — a leecher that just
  arrived at a needy swarm still triggers a rule whose window had already expired. Accumulating
  demand requires a new column and is deferred until the signal matures.

**Default calibration (production data, read-only).** Seed/leecher ratio of the
torrents that triggered the rule on 09/09/2026: 2, 63, 20, 5,7, 15 and 16 — all on healthy Nodes;
with `1`, **none** trigger. On 03/10/2026, out of 1.131 torrents, six had leechers, with ratios
0, 1, 4, 5,5, 5,5 and 24; with `1` the first two trigger (the swarms where the Node is actually
needed). A looser default (2 to 5) would readmit exactly the range that generated the noise.

## 48.2 Window: default from 12h to 240h

`alerting.incoming_silence` had already been patched from 12h to 48h on 09/09/2026 and remained
noisy. The window only makes sense if it is greater than the **normal** silence of a healthy Node,
so it was measured. Reconstructing, from production alerts, the silence episodes
of Nodes with no known port issue (the external check, on the dates it was performed,
found the port open) — alerts reopened after the Node went down and came back are the **same**
silence, and were chained —, the total silence until an incoming connection arrived was, in hours:

| 49 | 61 | 62 | 86 | 111 | 177 | 217 | 221 | still no incoming: over 21 days |
|---|---|---|---|---|---|---|---|---|

The values of 86, 111, and 177h are floors (the window added to the time the alert was open).
Across these nine episodes, a 48h window triggered on **all** of them; 96h, on five; 168h, on
four; **240h, on one** — the Node that remains without incoming connections. A healthy Node goes 2 to 9 days without
receiving any connection.

The cost is detecting in up to 10 days instead of 2. It is acceptable because the cause that originated the
rule family (IPv6 prefix rotation) has its own immediate alert
(`NODE_IPV6_CHANGED`); this is the generic safety net, and a slow and
precise safety net is worth more than a fast one that the operator learns to ignore. A naturally idle Node
is not distinguished from a broken Node by silence alone — for this case the rule relies on
weighted demand (§48.1) and validity (§46). The value recorded in the database (Settings
panel) prevails over the default; only those who have never adjusted it will change behavior.

## 48.3 Text that does not age

The alert text is recorded **once**, at opening, and the alert can remain open for days.
Address and duration inside it age: a Node's IPv6 address changed almost 38h after the
alert opened and the panel kept telling to release an address that no longer existed; the "12h0m22s
ago" remained frozen at the opening value.

The message is now fixed and neutral — *Node without incoming connection beyond the tolerated limit, despite having
demand — it may be unreachable: check if the Node's BT port is open for its current
address* — and what varies goes in the **payload**: `hostname`, `public_ipv6`, and `bt_port` (from the time
of opening), `last_incoming_at` (the start of the silence), and the demand.

- **Portal:** the *Alerts & Events* screen shows, next to the **open** alert, the Node's
  **current** `[ipv6]:porta` address, read from the Node itself (which the page already reloads every cycle),
  not from the payload. It disappears when the alert closes or the Node has not reported an address/port.
- **Notification** (Telegram, Discord, webhook): it is an instant, so it takes the address from the
  payload as the *Node Address* field and the start of the silence as *Last Incoming*. The versioned
  `data` block (§44) does not change: it remains only in `NODE_IPV6_CHANGED`.
- `NODE_IPV6_CHANGED` is different: the address is part of the condition, and the alert is updated on
  every rotation (#209). `NODE_FIREWALLED` still carries the address in the text — same class of
  problem, outside this delivery.

An alert already open before the deploy keeps the old text until it closes or expires (§46); the
message is not rewritten.

**Discarded (operator decision, 03/10/2026):**

- **Retire the alert.** It covers a real case — the Node that became unreachable without changing
  its address — that no other alert catches.
- **External port probe** (promoting the external check to a Control Plane signal). It depends on
  a third-party service, and the Control Plane, from inside the Node's network, does not measure from the outside.
- **IPv6 egress probe on the Agent** (#203), to choose between "check the firewall rules" and "the Node
  cannot exit via IPv6". The cause of the incident that motivated it was resolved by the deterministic IPv6
  suffix (#202); the probe would be a permanent cost (column, heartbeat, metric) for a
  case that no longer occurs.

# **49. NODE_FIREWALLED: text that does not age (#223)**

`NODE_FIREWALLED` opens for Nodes whose Engine reports `firewalled` for longer than the grace period (`alerting.firewalled_grace`): no one from the outside can open a connection to the BT port. It is the onboarding rule — the Node whose port was never opened is born `firewalled` and never leaves.

The alert text was recorded **once**, when it opened, and could remain open for days. Everything it said that changes over time aged along with it: the IPv6 address (the provider rotates the prefix and the panel keeps prompting to open an address that no longer exists) and the "Xs ago" (frozen at the value from when it opened). It is the same class of problem that §48.3 fixed in `NODE_INBOUND_SILENT` (#203), and the treatment is the same.

## 49.1 Constant text

The message is now fixed and neutral — *Node Engine reports firewalled beyond the grace period — no one from the outside can open a connection to the BT port: check if the Node's BT port is open for its current address* — and what varies goes in the **payload**: `hostname`, `public_ipv6` and `bt_port` (from the time it opened) and `connectable_since` (the start of the verdict, analogous to `last_incoming_at` in `NODE_INBOUND_SILENT`).

- **Portal:** the *Alerts & Events* screen shows, next to the **open** alert, the **current** `[ipv6]:porta` address of the Node, read from the Node itself (which the page already reloads every cycle), not from the payload. It disappears when the alert closes or if the Node has not reported an address/port.
- **Notification** (Telegram, Discord, webhook): it is an instant, so it takes the address from the payload as the *Node Address* field and the start of the verdict as *Firewalled since*. The versioned `data` block (§44) does not change: it remains only in `NODE_IPV6_CHANGED`.
- `NODE_IPV6_CHANGED` is different: the address is part of the condition, and the alert is updated on every rotation (§44, #209).

An alert already open before the deployment retains the old text until it closes or expires (§46); the message is not rewritten.

# **50. Language of alert and event text (ADR-041)**
The text of an alert, event, or activity is a **presentation** of `code` + `payload`, assembled by the display layer, in the reader's language (ADR-041). It is the generalization of what sections 48 and 49 did for the Node address: the data lives in the `payload`, not in the sentence.

- The **Portal** assembles the title and sentence from the active language catalog, based on the `code` and `payload`.
- The Control Plane continues to record the `message`, in the cluster's default language, as a **fallback**: it is what is read by those consuming the API without a catalog, and what the Portal shows for a code it does not yet know. No data can exist solely in the `message`.
- **Notifications** are sent from the Control Plane in the cluster's language (`notify.language`), using the same set of keys.
- Rule for creating a new alert or event: define the `code`, put **everything** the sentence needs in the `payload`, and add the entry to the catalogs — a contract test rejects code without an entry or a sentence that requests missing data.
