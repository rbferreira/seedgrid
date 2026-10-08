# **SeedGrid**
# **Database Specification**
Version: 1.0
Status: Draft
Document: 12 - Database.md
# **1. Objective**
This document defines the official data model for SeedGrid.
All information persisted by the system must obey the structures defined here.
No component may create its own tables outside this specification.
All structural changes must occur through versioned migrations.
# **2. Philosophy**
The database represents the state of the Cluster.
It **is not** a real-time metrics store.
It **is not** a logging system.
It **is not** a cache.
Its responsibility is to store the persistent state of the platform.
# **3. Technologies**
# **Primary Database**
PostgreSQL 17
Responsible for:
- Configuration
- Cluster State
- Inventory
- Users
- Torrents
- Nodes
- Events
- Audit
# **Mandatory Extensions**
uuid-ossp
pgcrypto
citext
btree_gin
pg_trgm
# **Data Not Stored**
The following data DOES NOT belong in PostgreSQL:
Metrics → Prometheus
Logs → Loki
Dashboards → Grafana
.torrent files → Node Storage
FastResume → Node Storage
# **4. Conventions**
Every table must use:
id UUID PRIMARY KEY
Never use SERIAL.
Every table must have:
created_at
updated_at
When applicable:
deleted_at
No table will use physical deletion.
Always Soft Delete.
All dates:
TIMESTAMP WITH TIME ZONE
UTC
Strings:
TEXT
Avoid VARCHAR.
Booleans:
BOOLEAN
Public identifiers:
UUID v7 (when available)
If unavailable:
UUID v4.
# **5. Naming Conventions**
Tables
Always plural.
Example
nodes
torrents
users
trackers
labels
Columns
snake_case
Foreign keys
node_id
torrent_id
tracker_id
user_id
Indexes
idx_nodes_hostname
idx_torrents_hash
Unique
uk_nodes_hostname
Foreign Keys
fk_torrents_node
# **6. Conceptual Model**
The database will be divided into Domains.
Each domain represents a responsibility.
Cluster
├── Identity
├── Nodes
├── Storage
├── Torrent
├── Users
├── Monitoring
├── Inventory
├── Scheduler
├── Events
├── Audit
└── Configuration
# **7. Main Entities**
The system will initially have the following entities.
Identity
users
roles
permissions
api_tokens
sessions
Cluster
clusters
cluster_settings
cluster_variables
Nodes
nodes
node_labels
node_capabilities
node_tags
node_states
node_versions
node_interfaces
Storage
disks
partitions
filesystems
mountpoints
storage_pools
Torrent
torrents
torrent_states
torrent_categories
torrent_tags
torrent_trackers
torrent_files
torrent_history
Inventory
hardware
cpus
memory_modules
usb_devices
network_interfaces
docker_info
containers
Monitoring
health_scores
heartbeats
alerts
notifications
Events
events
event_types
event_queue
Audit
audit_logs
audit_changes
Configuration
settings
feature_flags
system_variables
# **8. General Relationships**
Cluster
│
├── Nodes
│      │
│      ├── Disks
│      ├── Inventory
│      ├── Heartbeats
│      ├── Torrents
│      └── Events
│
├── Users
│
├── Trackers
│
└── Settings
# **9. clusters Table**
Objective
Represent a logical Cluster.
Even if only one exists initially, the structure already supports multiple Clusters.
Fields
id
uuid
name
description
timezone
created_at
updated_at
deleted_at
Relationships
1:N Nodes
1:N Users
1:N Settings
# **10. nodes Table**
Objective
Represent any equipment participating in the Cluster.
Never store hardware-specific information in this table.
This information belongs to the Inventory domain.
Fields
id
cluster_id
uuid
hostname
display_name
description
fqdn
management_ip
management_port
agent_version
engine
engine_version
os_name
os_version
kernel_version
architecture
timezone
status
health_score
registered_at
last_seen_at
agent_config_version (ADR-039: version of the behavior config that the Agent applied; NULL = never reported)
agent_config_applied_at
agent_config_seeded_at (ADR-039 §7: the seeding of overrides is a one-time event per Node)
created_at
updated_at
deleted_at
Constraints
Unique Hostname per Cluster.
Unique Management IP.
Globally unique UUID.
Indexes
hostname
management_ip
status
health_score
last_seen_at
# **11. Node States**
ONLINE
OFFLINE
DRAIN
MAINTENANCE
FAILED
UNKNOWN
DISABLED
Note
The state must not be calculated solely by the heartbeat.
The Control Plane may change the state manually.
# **12. node_labels Table**
Objective
Allow logical classification of Nodes.
Labels are created exclusively by the operator.
Never automatically.
Fields
id
node_id
key
value
description
created_at
updated_at
Examples
country=br
energy=low
rack=01
tracker=movies
priority=gold
# **13. node_capabilities Table**
Objective
Record capabilities automatically detected by the Agent.
Never edited manually.
Fields
id
node_id
capability
value
source
detected_at
Examples
docker
true
smart
true
nvme
false
ups
true
usb3
true
arm64
true
qbittorrent
true
This information will be used in the future by the Scheduler for decision making.
# **14. node_tags Table**
Difference between Labels and Tags
Labels
Defined by the operator.
Used in policies.
Persistent.
Tags
Generated automatically.
Transient.
Informative.
Examples
hot
high-upload
low-space
overloaded
new-node
cooling
Tags may be changed automatically by the Control Plane.
# **15. Architectural Decisions**
The database will never store continuous metrics.
Examples:
CPU per second
RAM per second
Temperature per second
Instantaneous Upload
Instantaneous Download
This information belongs to Prometheus.
PostgreSQL will only store relevant states, configurations, inventory, and operational history.
# **16. Inventory Domain**
# **Objective**
The Inventory domain represents all physical and logical characteristics of a Node.
Its purpose is to allow the Control Plane to have complete knowledge about each device in the Cluster.
The inventory is collected exclusively by the Agent.
No information may be changed manually by the operator, except when explicitly provided for in another document.
# **17. hardware table**
Objective
Store general information about the device.
Relationship
1:1 with nodes.
Fields
id
node_id
manufacturer
model
board_model
board_revision
serial_number
cpu_serial
firmware_version
bios_version
bootloader_version
boot_mode
power_supply
created_at
updated_at
Examples
Raspberry Pi 3B+
Raspberry Pi 4
Dell OptiPlex
Intel NUC
KVM VM
# **18. cpus table**
Objective
Represent installed processors.
Relationship
1:N with nodes.
Fields
id
node_id
socket
vendor
model
architecture
cores
threads
base_frequency
max_frequency
cache_l1
cache_l2
cache_l3
virtualization_supported
aes_supported
neon_supported
current_governor
created_at
updated_at
Note
The Agent must update the current frequency only through metrics.
The database stores only the CPU characteristics.
# **19. memory_modules table**
Objective
Represent memory modules.
Relationship
1:N with nodes.
Fields
id
node_id
slot
manufacturer
capacity
type
speed
serial
part_number
created_at
updated_at
For Raspberry Pi, where this information may not exist, the Agent must populate only the available fields.
# **20. network_interfaces table**
Objective
Represent network interfaces.
Relationship
1:N with nodes.
Fields
id
node_id
name
type
mac_address
driver
speed
duplex
mtu
ipv4
ipv6
gateway
dns
dhcp
link_state
created_at
updated_at
Examples
eth0
wlan0
usb0
bond0
# **21. usb_devices table**
Objective
Inventory connected USB devices.
Relationship
1:N with nodes.
Fields
id
node_id
vendor
product
manufacturer
serial
device_class
usb_version
bus
port
speed
power_required
created_at
updated_at
Examples
External HDD
USB SSD
Flash Drive
USB Hub
UPS
Zigbee Dongle
# **22. disks table**
Objective
Represent physical storage devices.
Relationship
1:N with nodes.
Fields
id
node_id
device_name
manufacturer
model
serial
type
connection
capacity
sector_size
firmware
rotation
trim_supported
smart_supported
created_at
updated_at
Supported types
HDD
SSD
NVMe
USB Flash
RAM Disk
Loop Device
Supported connections
USB
SATA
NVMe
MMC
SD Card
Virtual
# **23. partitions table**
Objective
Represent disk partitions.
Relationship
1:N with disks.
Fields
id
disk_id
device
filesystem
label
uuid
size
mountpoint
bootable
created_at
updated_at
# **24. filesystems table**
Objective
Represent known file systems.
Relationship
1:N with partitions.
Fields
id
partition_id
type
block_size
compression
encryption
case_sensitive
created_at
updated_at
Expected types
ext4
xfs
btrfs
zfs
ntfs
fat32
exfat
# **25. mountpoints table**
Objective
Represent active mount points.
Relationship
1:N with partitions.
Fields
id
partition_id
path
read_only
mounted
options
available_space
used_space
reserved_space
created_at
updated_at
Example
/data
/downloads
/backup
/var/lib/docker
# **26. docker_info table**
Objective
Represent the Docker installation on a Node.
Relationship
1:1 with nodes.
Fields
id
node_id
version
compose_version
storage_driver
cgroup_version
root_directory
logging_driver
swarm_enabled
created_at
updated_at
# **27. containers table**
Objective
Inventory running containers.
Relationship
1:N with nodes.
Fields
id
node_id
container_name
image
tag
container_id
status
restart_policy
network
ports
created_at
updated_at
Note
SeedGrid must know all existing containers on the Node, not just its own.
This will allow the detection of conflicts and resource usage, and facilitate diagnostics.
# **28. services table**
Objective
Represent operating system services.
Relationship
1:N with nodes.
Fields
id
node_id
service_name
status
enabled
version
created_at
updated_at
Examples
docker
ssh
seedgrid-agent
cron
systemd-resolved
# **29. Partial Inventory**
If any information cannot be obtained by the Agent, the record must remain valid.
Unknown fields must be stored as NULL.
The Agent must never invent values.
# **30. Inventory Update**
The complete inventory must not be sent at every heartbeat.
The Agent must:
- send a complete inventory on the first record;
- resend only when changes are detected;
- allow manual synchronization requested by the Control Plane.
This strategy reduces traffic, database writes, and processing.
# **31. Versioning**
Each inventory update will increment a version number.
Additional fields:
inventory_version
inventory_updated_at
These attributes belong to the nodes table and allow the Control Plane to quickly identify if the local inventory is synchronized with the latest known inventory.
# **32. General Rules**
The inventory represents the Node configuration.
It does not represent its momentary state.
Usage changes (CPU, memory, temperature, network) belong to the metrics domain and must not be persisted in this set of tables.
The goal is to enable faithful reconstruction of each Node's configuration, auditing of hardware changes, and support for the Scheduler's scheduling algorithm.
# **33. Torrent Domain**
# **Objective**
The Torrent domain represents all objects related to the BitTorrent operation of the Cluster.
The Control Plane must never depend directly on the Engine's data structure.
The Agent will be responsible for the translation between the Engine data and the SeedGrid data model.
# **34. Concepts**
A Torrent represents a logical object of the Cluster.
It always belongs to a single Node.
A Torrent can have:
- multiple files;
- multiple trackers;
- multiple tags;
- multiple events;
- multiple histories;
- multiple statistics.
# **35. torrents table**
Objective
Represent a Torrent known to the Cluster.
Relationship
N:1 with Nodes
Fields
id
uuid
node_id
hash
name
size
piece_size
piece_count
private
engine_id
engine_version
status
priority
category_id
created_by
created_at
updated_at
deleted_at
Notes
The hash field represents the universal identifier of the torrent.
The engine_id field represents the internal identifier used by the Engine.
The Control Plane will never use engine_id as a business key.
# **36. Torrent States**
The official states of SeedGrid will be:
QUEUED
DOWNLOADING
SEEDING
PAUSED
CHECKING
MOVING
ERROR
ARCHIVED
DELETED
UNKNOWN
The Agent will be responsible for mapping Engine-specific states to these states.
# **37. torrent_categories table**
Objective
Group torrents.
Fields
id
name
description
color
icon
created_at
updated_at
Examples
Movies
Series
Linux
Games
Backup
ISO
Freeleech
Temp
Note
Categories belong to SeedGrid.
They do not necessarily reflect existing categories in the Engine.
# **38. torrent_tags table**
Objective
Allow flexible classification.
Fields
id
name
color
description
system
created_at
updated_at
Examples
4K
HDR
Anime
PT-BR
Seeder
Rare
Pinned
Important
Tags can be:
Created manually.
Created automatically.
# **39. torrent_tag_relations table**
N:N Relationship
Fields
torrent_id
tag_id
created_at
# **40. torrent_trackers table**
Objective
Represent trackers associated with the Torrent.
Fields
id
torrent_id
tracker_url
tier
status
last_announce
last_scrape
last_error
message
created_at
updated_at
Note
SeedGrid can consolidate information from multiple Engines.
# **41. torrent_files table**
Objective
Represent files belonging to the Torrent.
Fields
id
torrent_id
path
filename
extension
size
priority
selected
completed
checksum
created_at
updated_at
Note
This information will enable future features such as:
content preview;
individual file selection;
partial migration.
# **42. torrent_statistics table**
Objective
Store persistent statistics.
Fields
id
torrent_id
uploaded_bytes
downloaded_bytes
ratio
completed_bytes
availability
connected_peers
connected_seeds
share_time
active_time
last_activity
created_at
updated_at
Important
The table will NOT store temporal history.
It only represents the current known state.
# **43. torrent_history table**
Objective
Record important changes.
Fields
id
torrent_id
event
old_value
new_value
performed_by
created_at
Examples
Torrent created
Torrent paused
Torrent resumed
Category change
Priority change
Migration
Recheck
Error
# **44. torrent_locations table**
Objective
Represent physical location.
Fields
id
torrent_id
disk_id
partition_id
mountpoint
relative_path
absolute_path
created_at
updated_at
Importance
Never depend on the path configured in the Engine.
Always know exactly where the data is.
# **45. torrent_scheduler table**
Objective
Record Scheduler decisions.
Fields
id
torrent_id
selected_node
decision_score
decision_reason
algorithm_version
created_at
Example
Selected Node
seedgrid-node-02
Score
94
Reason
Most available space
This information will be extremely useful for auditing.
# **46. torrent_operations table**
Objective
Record administrative operations.
Fields
id
torrent_id
operation
requested_by
requested_at
started_at
finished_at
status
error_message
Examples
Pause
Resume
Delete
Move
Recheck
Relocate
Export
Import
# **47. torrent_queue table**
Objective
Represent the internal operation queue.
Fields
id
torrent_id
operation
priority
status
retry_count
scheduled_at
started_at
finished_at
Note
The Control Plane will never execute operations directly.
Every action will go through the queue.
# **48. Integrity Rules**
There must not be a Torrent without a Node.
There must not be a file without a Torrent.
There must not be a Tracker without a Torrent.
There must not be statistics without a Torrent.
There must not be an operation without a Torrent.
# **49. Deletion**
When removing a Torrent from the Cluster:
The main record will be marked with deleted_at.
The history will be preserved.
Operations will remain recorded.
Statistics may be archived.
No records will be physically removed by default.
# **50. Synchronization**
The Agent will send to the Control Plane:
Current state.
Changes.
New Torrents.
Removals.
Tracker changes.
Location changes.
Statistics changes.
The Control Plane will always be the official source of persistent state.
# **51. Conceptual Model**
Node
│
├── Torrents
│      │
│      ├── Files
│      ├── Trackers
│      ├── Tags
│      ├── Statistics
│      ├── Operations
│      ├── Queue
│      ├── History
│      └── Locations
│
└── Scheduler Decisions
# **52. Principles**
SeedGrid represents Torrents as Cluster objects.
The Engine represents Torrents as local objects.
These two models must never be directly coupled.
All translation between them will be the exclusive responsibility of the Agent.
# **52-A. Tracker Profile (tracker_profiles)**
Objective
Store the knowledge that belongs **to the site** of a tracker and not to the operator (ADR-037, migration 00043).
Today it is the taxonomy — the IDs for `category`, `type`, and `resolution`, which each admin configures on their
site (`Encode` is 3 on samaritano and 12 on Blutopia), the **description composition** of the upload (#192: which blocks
go in, in what order, and the MediaInfo tag) and the site's **image host** (#193, ADR-040: the closed-form Spec
that describes the tracker's own host for the screenshot upload chain). Name format and
rules come in as new sections of the same document (#191), without remodeling the table.
The three layers of ADR-037 and where each one lives
- Family (API shape, auth, multipart): Go code (`internal/unit3d`). It is not in the database.
- Site profile: **this table**. Exportable in JSON (#197, delivered) — anyone with an account on the site can discover, adjust, and share it.
- Operator config (`api_token`, announce with passkey, `image_host_key`/`image_host_key_host` — migration 00045, #193 / ADR-040 —, and in the future `anon`/`modq`): `trackers` table. **Never exported.** `image_host_key` is the operator's key for the tracker's own image host (secret, never returned in the API); `image_host_key_host` is the DNS host for which the key was registered — if the profile's `upload_url` changes host, the old key will no longer work.
Why it is a separate table from `trackers`
The export for #197 reads only from this table (and, from `trackers`, the name and host — never announce, `api_url`, token, or `source`). `tracker_profiles` **has no credential or config
column for the operator**, on purpose: layer 3 is kept out by design, not by a serialization filter
that someone needs to remember. The same principle that already applies to `trackers.announce` and `trackers.api_token`,
which never leave the API. Tests guard this: one iterates through the table columns and the type tree
of the profile looking for credential names; another creates a tracker with passkey and token and checks that nothing
of this appears in any profile read; and, in the export, a third iterates through the type tree of the
file (credential field, opaque field, custom `MarshalJSON`) and a fourth exports a tracker with everything
filled and looks for the sensitive **values** in the file text.
Fields
id (UUID)
tracker_id (FK trackers — one **live** profile per tracker, partial unique index `WHERE deleted_at IS NULL`)
family (TEXT, default `unit3d`; validated in Go — new family means new code)
schema_version (INTEGER ≥ 1, the version of the profile **format**: what the export stamps and the import checks)
taxonomy (JSONB, always an object — `CHECK jsonb_typeof(taxonomy) = 'object'`; it is the **entire document** of the profile: the axes at the root and, alongside them, a reserved key per section — today `description` and `image_host`)
created_at
updated_at
deleted_at
Shape of `taxonomy`
`{"category": [{"id": 1, "name": "Filmes", "kind": "movie", "origin": "seed"}], "type": […], "resolution": […]}`
- `origin`: `seed` (migration) | `discovered` (discovered in the site catalog) | `manual` (the operator typed or renamed it) | `imported` (came from a profile file, #197). Decides who can change the **name**: discovery and import never overwrite `manual` — nor does the import overwrite `discovered` without a request; an unknown origin is treated as protected (fail closed). It is **local** state — it is not part of the exchange file; the import assigns `imported` to what it writes. No `CHECK` in the database (it lives inside the JSONB): the value is validated in Go, and `imported` **did not require a migration**.
- `description` (#192): `{"media_blocks": ["poster", "synopsis", "tags", "screenshots", "mediainfo"], "game_blocks": ["cover", "title", "about", "genres", "platforms"], "media_info_tag": "mediainfo"}` — what is written is the **default**. Block IDs from a closed set, per type, without repetition; `media_blocks: []` means "no blocks" (not the default); the tag is `mediainfo`, `code`, `pre` or `spoiler`. **Missing** key = the default. **No migration and no new column:** it is one more key in the existing document, and the row prior to #192 is, byte for byte, a document without it. An axis is never named `description` (the set of axes is closed), so the key does not collide. Reading is strict (wrong type is an error, not "empty"); writing validates everything in `internal/trackerprofile` and rewrites it **together** with the taxonomy under the same `FOR UPDATE`. Trade-off: a Control Plane prior to #192 will not read the profile of a tracker with the custom composition. Optional `language` (#74): `en` or `pt-BR`, omitted when English.
- `image_host` (#193 / ADR-040): `{"name": "Host do site", "upload_url": "https://img.tracker.exemplo.org/api/v1/sharex/upload", "file_field": "file", "auth_header": "Authorization", "auth_prefix": "Bearer ", "url_path": "url", "thumbnail_path": "thumbnail_url"}` — the **closed-form** Spec of the tracker's own image host. `name` with the name rules (1 to 64 characters, no control or invisible formatting); absolute `upload_url`, `https`, DNS host (no literal IP, no `localhost`, no port, no userinfo, no fragment), up to 2048 characters; `file_field` `[A-Za-z0-9_.-]{1,64}`; `auth_header` header token `[A-Za-z0-9-]{1,64}`, excluding `Host`, `Content-Type`, `Content-Length`, `Transfer-Encoding`, `Connection`, `Cookie`, `Upgrade`, `TE`, `Trailer`, and `Proxy-*` (case-insensitive); `auth_prefix` 0 to 32 printable ASCII characters; `url_path` 1 to 5 segments `[A-Za-z0-9_]{1,64}` separated by dots; `thumbnail_path` optional, same rules. **Missing** key = no own host (only the global imgbb in the screenshot chain). **No migration and no new column:** it is one more key in the existing document, and the row prior to #193 is, byte for byte, a document without it. An axis is never named `image_host` (the set of axes is closed), so the key does not collide. Reading is strict (wrong type is an error, not "empty"); writing validates everything in `internal/trackerprofile` and rewrites it **together** with the taxonomy under the same `FOR UPDATE`. The **operator's key** for the host is layer 3 (`trackers` table, #193 / ADR-040) and never lives here. Trade-off: a Control Plane prior to #193 will not read the profile of a tracker with a configured image host.
- `kind`: `movie` | `tv` | `game`, only in `category`. The semantic role that the Portal used to deduce from the number (`isTV = id 2 ou 3`, `isGame = id 5`) — which is only true for one site.
- Per axis: `id` in `1..1000000` and unique; name with 1 to 64 characters without control or invisible formatting; at most 256 terms; sorted by `id`.
Why JSONB and not one table per term
- The unit of export, import, and validation is the **document**: one column per section is the format of the contributed file, without assembling and disassembling rows.
- New axis (audio, language, tag, flag) and new section (rules, image host) come in without a table migration — only with `schema_version`.
- The taxonomy is never queried by content ("which trackers have category 5?"): it is read entirely, there are a few dozen terms, and the Portal edits it as a whole.
- What the relational model would give for free (unique id per axis, range, size) lives in `internal/trackerprofile`, at **every** write point, and the write is serialized by `SELECT … FOR UPDATE` on the profile row — lockless read-modify-write would lose concurrent applications (there is a test with 8 simultaneous applications).
Coexistence rule between discovery and manual editing
- Discovery only **proposes**; applying is another operation, audited (ADR-025).
- Missing ID in the profile → adds (`discovered`). Same name → nothing. Different name → renames **only if the origin is not `manual`**. What the catalog did not show remains untouched: absence is not evidence, the catalog only shows what someone posted.
- Editing by hand (`PUT`) marks as `manual` whatever is created or renamed; changing only the `kind` does not change the origin, because discovery never touches `kind`.
Samaritano migration (without the operator reconfiguring anything)
00043 creates the table empty: no tracker gets a profile from a migration. An operator whose site already has a profile in the repository imports the file (`profiles/<family>/<site>.json`, ADR-037); the others run "Discover taxonomy". The migration is tested against a disposable database: it upgrades to 00041 with a tracker registered, applies the rest, and checks that no profile was created, that a tracker has at most one live profile, and that the down leaves nothing behind.
Notes
- The row is created on demand (first edit or discovery); a tracker without a row is read as an empty profile.
- Soft delete: removing the tracker soft deletes the profile (best-effort — every read checks ownership and existence of the tracker, so an orphaned profile is inert).
- Profile is **untrusted content** (third-party catalog, editing, and imported file): type and range validation on input, nothing executable, no free-form field going into the request.
- **Export/import (#197):** the file is the `Document` (envelope + `Profile`), with no credential field by design; the import writes to the same row, under the same `FOR UPDATE`, and the coexistence rule (`manual` is never overwritten without a request; the import never removes) runs on the locked row. Format and validation: ADR-037 and `docs/13 - API.md`. `schema_version` is the version of the **format** — it goes up when a field changes shape or a new **mandatory** section is added; an optional and additive section, which the export omits by default (the `description`, #192), does not go up (the column keeps the row's version; the file stamps the one emitted by the code).
# **53. Identity Domain**
# **Objective**
The Identity domain is responsible for authentication, authorization, and identity management for SeedGrid operators.
This domain has no responsibility for authentication between Nodes and the Control Plane.
This communication will be documented in **Authentication.md**.
# **54. users table**
Objective
Represent platform users.
Fields
id
uuid
username
display_name
email
password_hash
status
last_login_at
created_at
updated_at
deleted_at
Status
ACTIVE
DISABLED
LOCKED
PENDING
Notes
Username must be unique.
Email must be unique.
Passwords must never be stored in plain text.
# **55. roles table**
Objective
Represent access roles.
Fields
id
name
description
system
created_at
updated_at
Default roles
Administrator
Operator
ReadOnly
# **56. permissions table**
Objective
Represent individual permissions.
Fields
id
resource
action
description
created_at
Examples
nodes.read
nodes.write
nodes.delete
torrents.read
torrents.write
scheduler.execute
alerts.read
cluster.admin
# **57. role_permissions table**
Relationship
N:N
Fields
role_id
permission_id
created_at
# **58. user_roles table**
Relationship
N:N
Fields
user_id
role_id
created_at
# **59. sessions table**
Objective
Represent authenticated sessions.
Fields
id
user_id
session_token
ip_address
user_agent
expires_at
last_activity
created_at
Note
Expired sessions may be automatically removed.
# **60. Monitoring Domain**
# **61. heartbeats table**
Objective
Record the last received heartbeat.
Important
This table has NO history.
It represents only the last known state.
Fields
id
node_id
received_at
agent_version
engine_version
status
uptime
health_score
created_at
updated_at
# **62. health_scores table**
Objective
Represent the current health of a Node.
Fields
id
node_id
overall_score
cpu_score
memory_score
storage_score
network_score
temperature_score
engine_score
calculated_at
Values
0
to
100
# **63. Events Domain**
Objective
Represent relevant events.
Every component must generate events.
# **64. event_types table**
Objective
Catalog event types.
Fields
id
code
name
severity
description
Examples
NODE_ONLINE
NODE_OFFLINE
TORRENT_CREATED
TORRENT_DELETED
SMART_WARNING
SMART_FAILURE
BACKUP_STARTED
BACKUP_FINISHED
# **65. events table**
Objective
Record events.
Fields
id
event_type_id
node_id
torrent_id
user_id
message
payload
severity
created_at
Notes
Payload will be stored in JSONB.
It will allow evolution without migrations.
# **66. event_queue table**
Objective
Internal processing queue.
Fields
id
event_id
status
attempts
last_attempt
processed_at
created_at
Status
PENDING
PROCESSING
DONE
FAILED
# **67. Alerting Domain**
# **68. alerts table**
Objective
Represent active alerts.
Fields
id
node_id
torrent_id
severity
title
description
status
created_at
resolved_at
Status
OPEN
ACKNOWLEDGED
RESOLVED
CLOSED
Severities
INFO
WARNING
ERROR
CRITICAL
# **69. notifications table**
Objective
Record sent notifications.
Fields
id
alert_id
channel
destination
status
sent_at
response
Examples
Telegram
Discord
Google Chat
Webhook
Email
# **70. Configuration Domain**
# **71. settings table**
Objective
Store the overrides for behavior tunables (ADR-023), overriding the defaults from the
code catalog. Pure KV: the metadata (type, default, description, unit, limits, category)
lives in the catalog (`internal/settings/catalog.go`), NOT in the database.
Fields
key (PK, namespaced — e.g.: admission.dead_grace)
value (string; parsed by catalog type)
updated_at
updated_by (operator; 'env-seed' on boot seed)
Semantics
- Override of the default; removing the row = reset to default.
- Precedence: the database rules; the env var only seeds on the 1st boot (Store.Load).
- Applied at runtime by the Store (hot-reload). See `25 - Business Rules.md`.
Examples
admission.dead_grace = 3m
alerting.disk_saturated_pct = 90
workers.reconciler_interval = 15s
# **71.1 node_settings table**
Objective
PER NODE override of the Agent/Engine behavior tunables (ADR-039, #144), over the cluster
value from the `settings` table. Only the `PerNode` keys from the catalog (`agent.heartbeat_interval`,
`agent.command_poll_interval`, `agent.log_level`, `engine.*`) go here.
Fields
id (UUID PK)
node_id (FK → nodes, ON DELETE CASCADE)
key (namespaced, same as `settings`)
value (string; parsed by catalog type)
created_at
updated_at
updated_by (operator; 'seeding (Node env)' when it comes from seeding)
Constraints
UNIQUE (node_id, key)
Semantics
- Effective value of a Node = Node override > cluster value (`settings`/catalog default).
  The Node env and the code default are the Agent's fallback, only when the CP sends nothing.
- Validated by the SAME catalog `Spec` (type, range, options) — there is no second standard.
- Removing the row returns the Node to the cluster value. No soft delete, like `settings`:
  the history (before and after) is kept in Audit (ADR-025).
- Delivered to the Agent in the heartbeat response (`agent_config`). See `13 - API.md` and `25 - Business Rules.md`.
Examples
(node-04, Atom) engine.max_active_downloads = 3 — the cluster value is 10
# **72. feature_flags table**
Objective
Control experimental features.
Fields
id
name
enabled
description
created_at
updated_at
Examples
scheduler_v2
agent_auto_update
experimental_api
# **73. system_variables table**
Objective
Store internal variables.
Fields
id
key
value
updated_at
Examples
schema_version
cluster_uuid
last_backup
database_version
# **74. General Rules**
Users never belong to Nodes.
Events must never be automatically deleted.
Resolved Alerts must remain available for audit.
Settings must always be global (cluster value). The only exception is the per-Node override of the
Agent/Engine behavior tunables (`node_settings`, ADR-039), which never replaces the
cluster value: it only overrides it for that Node, and reverts to it when removed.
Feature Flags must never replace settings.
System variables must never be edited by the operator.
# **75. Conceptual Model**
Users
│
├── Roles
│
├── Sessions
│
└── Permissions
Nodes
│
├── Heartbeats
├── Health Scores
├── Alerts
└── Events
Cluster
│
├── Settings
├── Feature Flags
└── System Variables
# **76. Principles**
Events represent occurred facts.
Alerts represent active issues.
Settings represent expected behavior.
Variables represent internal state.
These four concepts must never be confused.
The data model must preserve this separation throughout the entire evolution of the project.
# **77. Audit**
# **Objective**
Ensure complete traceability of operations performed on the Cluster.
Every relevant change must be audited.
The audit must allow reconstructing who performed a given action, when it occurred, and what its impact was.
# **78. audit_logs Table**
Objective
Log all administrative actions.
Fields
id
user_id
node_id
torrent_id
resource
resource_id
action
ip_address
user_agent
request_id
status
created_at
Examples
CREATE_NODE
UPDATE_NODE
DELETE_NODE
MOVE_TORRENT
RECHECK_TORRENT
UPDATE_SETTINGS
LOGIN
LOGOUT
# **79. audit_changes Table**
Objective
Log data changes.
Relationship
N:1 with audit_logs.
Fields
id
audit_log_id
field_name
old_value
new_value
created_at
Note
Complex values must be stored in JSONB.
# **80. Referential Integrity**
All Foreign Keys must have explicit constraints.
Never use ON DELETE CASCADE for critical entities.
Examples of critical entities:
nodes
torrents
users
events
alerts
Deletions must be handled by the application.
# **81. Soft Delete**
All persistent entities must use Soft Delete.
Mandatory fields
deleted_at
deleted_by
Eligible entities
Nodes
Torrents
Users
Trackers
Tracker Profiles
Categories
Tags
Settings
Ineligible entities
Heartbeats
Health Scores
Sessions
Event Queue
Feature Flags
System Variables
# **82. Indexes**
All fields used in search must have an index.
Mandatory indexes
nodes.hostname
nodes.status
nodes.last_seen_at
torrents.hash
torrents.status
torrents.name
alerts.status
alerts.severity
events.created_at
users.username
users.email
JSONB fields used in filters must have GIN indexes.
# **83. Search**
The database must support efficient text search.
Eligible fields
torrent.name
node.display_name
tracker.name
event.message
alert.title
alert.description
The implementation must use PostgreSQL Full Text Search or pg_trgm indexes, as applicable.
# **84. Schema Versioning**
Every structural change must occur exclusively through migrations.
Each migration must have:
unique identifier
description
timestamp
rollback when applicable
Never change tables manually in production.
# **85. Retention**
The system must allow configurable retention policies.
Recommended default values
Audit Logs: 365 days
Events: 180 days
Notifications: 90 days
Expired Sessions: 30 days
Heartbeats: keep only the last record
Health Scores: keep only the last record
These values must be configurable.
# **86. Backup**
The database must support consistent backup without service interruption.
The backup must include:
Schema
Data
Indexes
Constraints
Extensions
Must not be stored in the database:
.torrent files
FastResume
Logs
Metrics
# **87. Restore**
The restore must allow complete reconstruction of the Control Plane state.
After restoration, the Agents must automatically synchronize their states with the database.
No manual intervention must be required for inventory reimport or Node identification.
# **88. Performance**
Minimum objectives
Torrent search by hash: \< 50 ms
Search by name: \< 200 ms
Node listing: \< 100 ms
Main dashboard: \< 500 ms
Event registration: \< 20 ms
These values represent design goals and may be revised in future versions.
# **89. Scalability**
The data model must support, at a minimum:
100 Nodes
100.000 Torrents
10 million events
50 users
1 million audit records
Without the need for structural changes.
# **90. Consistency**
PostgreSQL will be considered the single Source of Truth for:
Configuration
Inventory
Cluster State
Torrent State
Users
Events
Alerts
Audit
No component may maintain parallel persistent state for these entities.
# **91. General Conventions**
All times must be stored in UTC.
All identifiers must use UUID.
All critical operations must occur within transactions.
No business rule must depend exclusively on database triggers.
Business logic belongs to the Control Plane.
# **92. Evolution**
New entities must follow the same standards established in this document.
It is forbidden to create "temporary" tables for permanent features.
Every new entity must:
- have a clearly defined responsibility;
- respect naming conventions;
- use audit when applicable;
- have a backup and retention strategy.
# **93. Dependencies**
The documents below depend directly on this specification:
04 - Infrastructure.md
05 - Network.md
06 - Storage.md
07 - Workers.md
08 - Agent.md
09 - Control Plane.md
10 - Scheduler.md
11 - Portal.md
13 - API.md
14 - Authentication.md
15 - Monitoring.md
16 - Alerting.md
17 - Backup.md
18 - Disaster Recovery.md
Engine.md
# **94. Model Summary**
Implemented domains
✓ Identity
✓ Cluster
✓ Nodes
✓ Inventory
✓ Storage
✓ Torrent
✓ Tracker Profile (ADR-037)
✓ Monitoring
✓ Events
✓ Alerting
✓ Configuration
✓ Audit
# **95. Final Statement**
The SeedGrid database represents the persistent state of the Cluster.
It is not a mechanism for execution, real-time monitoring, or command processing.
Its responsibility is to ensure the consistency, traceability, and integrity of the information necessary for the Control Plane to operate in a predictable, auditable, and scalable manner.
Any future evolution of SeedGrid must preserve the principles established in this document.
