// Control Plane API client. The Portal only talks to the API (Vision §5.5);
// it never accesses the database or Nodes directly.

import { getLang, translate, type Key, type Params } from "./i18n";
import { messageText } from "./lib/messages";

import { filenameFromDisposition } from "./lib/profile-exchange";

export type Node = {
  id: string;
  uuid: string;
  // hostname is the technical data (the OS one, untouched). display_name is the nickname the
  // operator gave the Node within SeedGrid (#145): what the UI displays when it exists — use
  // `nodeLabel` (lib/node-label), not either of the two fields directly.
  hostname: string;
  display_name?: string | null;
  status: string;
  health_score?: number;
  storage_ok?: boolean | null; // Node storage health; null/undefined = unknown (old agent)
  storage_detail?: string | null; // reason when storage_ok === false (e.g., "not a mountpoint")
  agent_version?: string;
  engine?: string;
  last_seen_at?: string;
  registered_at?: string;
  os_name?: string;
  os_version?: string;
  kernel_version?: string;
  architecture?: string;
  inventory_updated_at?: string;
  disk_total_bytes?: number;
  disk_free_bytes?: number;
  mem_total_bytes?: number;
  mem_avail_bytes?: number;
  load1?: number;
  cpu_cores?: number;
  disk_io_util_pct?: number;
  cpu_util_pct?: number; // CPU utilization (%) from the last heartbeat (#151)
  bt_port?: number;
  last_ip?: string;
  // INBOUND Connectability (ADR-034). public_ipv6 is the address to open in the firewall;
  // engine_connectable is the Engine's verdict ("connected" = already received an outside connection,
  // "firewalled" = never received one). Absent = unknown (old agent), never "closed".
  public_ipv6?: string | null;
  engine_connectable?: string | null;
  connectable_since?: string | null;
  // Troubleshooting signals (last heartbeat).
  uptime_seconds?: number; // Agent uptime (service)
  host_uptime_seconds?: number; // uptime do host
  engine_reachable?: boolean; // Torrent Engine responded to the probe
  engine_started_at?: string; // Engine container startup (best-effort)
  engine_version?: string;
  operation_mode?: NodeMode; // per-Node mode (ADR-024); only takes effect under the Flex gate
};

// Operation mode (ADR-024). Cluster-wide gate + per-Node mode.
export type ClusterMode = "full_auto" | "flex";
export type NodeMode = "sg" | "manual";

export type SystemHealth = {
  api: string;
  database: { status: string };
  // status: "running" (healthy) | "stale" (delayed) | "starting" (no 1st cycle).
  workers: { name: string; status: string; last_run?: string | null; interval_seconds?: number }[];
  version: string;
  uptime_seconds: number;
};

export const getSystemHealth = () => req<SystemHealth>("/api/v1/system/health");

// EnrollToken: one-time join token (ADR-017) for zero-touch onboarding of a Node.
// display_name is the nickname the Node will have in SeedGrid (#145), already normalized by the CP.
export type EnrollToken = { token: string; expires_at: string; display_name?: string };
// mintEnrollToken issues the join token. The nickname is optional and travels in the token: the CP applies it
// when the Node registers, without the bootstrap needing to know it. 409 display_name_taken
// when the name already belongs to another Node (or another open token); 400 invalid_display_name
// when it fails validation.
export const mintEnrollToken = (display_name?: string) =>
  req<EnrollToken>("/api/v1/enroll-tokens", {
    method: "POST",
    ...(display_name ? { body: JSON.stringify({ display_name }) } : {}),
  });

export type Torrent = {
  id: string;
  uuid: string;
  node_id: string;
  hash?: string;
  name?: string;
  status: string;
  progress?: number;
  created_at?: string;
  num_seeds?: number; // scrape do swarm (tracker)
  num_leechers?: number; // scrape do swarm (tracker)
  num_seeds_connected?: number; // seeds actually connected to the Engine right now
  num_leechers_connected?: number; // leechers actually connected to the Engine right now
  dl_speed?: number;
  up_speed?: number;
  size_bytes?: number;
  ratio?: number;
  eta_seconds?: number;
  downloaded_bytes?: number;
  uploaded_bytes?: number;
  // Cumulative totals (#14): survive Engine session resets — the basis for the
  // aggregated ratio per Node/Cluster (the dl/uploaded_bytes snapshot resets on re-add).
  downloaded_total?: number;
  uploaded_total?: number;
  // Manual queue prioritization (F3): when present, the operator jumped the queue for this
  // torrent (the Portal shows it in yellow).
  prioritized_at?: string;
};

export type Alert = {
  id: string;
  code: string;
  severity: string;
  status: string;
  node_id?: string;
  message?: string;
  // The alert data: the phrase is built from it in the reader's language (lib/messages).
  payload?: Record<string, unknown>;
  opened_at: string;
  resolved_at?: string;
  // Closed due to VALIDITY (#211), not because the condition ceased to exist — reachability alerts
  // expire because an idle Node never receives the connection that would resolve them.
  expired?: boolean;
};

export type Event = {
  id: string;
  code: string;
  severity: string;
  node_id?: string;
  message?: string;
  // The event data: the phrase is built from it in the reader's language (lib/messages).
  payload?: Record<string, unknown>;
  created_at: string;
};

// Activity: orchestration trail (ADR-022) — "under the hood" feed, separate
// from Alerts and Events. kind: admission_hold | admission_resume | scheduled |
// downloading | seeding.
export type Activity = {
  id: string;
  kind: string;
  torrent_id?: string;
  torrent_name?: string;
  node_id?: string;
  node_hostname?: string;
  node_display_name?: string; // Node alias (#145); missing = display the hostname
  message: string;
  detail?: Record<string, unknown>;
  created_at: string;
};

const TOKEN_KEY = "seedgrid_token";
const DEFAULT_CREDS_KEY = "seedgrid_default_creds";
const USER_KEY = "seedgrid_user";
export const getToken = () => localStorage.getItem(TOKEN_KEY);
export const setToken = (t: string) => localStorage.setItem(TOKEN_KEY, t);
export const getUsername = () => localStorage.getItem(USER_KEY) ?? "";
export const clearToken = () => {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(DEFAULT_CREDS_KEY);
  localStorage.removeItem(USER_KEY);
};
// usingDefaultCredentials indicates that the operator entered the default password (admin/admin).
export const usingDefaultCredentials = () => localStorage.getItem(DEFAULT_CREDS_KEY) === "1";
export const clearDefaultCreds = () => localStorage.removeItem(DEFAULT_CREDS_KEY);

// portalMsg: the messages the Portal itself generates (without a useful server response), in the active language.
// The message that ARRIVES from the API (error.message) goes as is — its code is #34.
const portalMsg = (key: Key, params?: Params) => translate(getLang(), key, params);

export class ApiError extends Error {
  status: number;
  code?: string; // API error code (e.g., "duplicate", "no_eligible_node")
  // details: the whole `error` object of the response, for the codes that carry data beyond the
  // message (e.g., the versions of "other_versions", #37).
  details?: Record<string, unknown>;
  constructor(status: number, message: string, code?: string, details?: Record<string, unknown>) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

// TorrentVersion: a version of a work — one already in the cluster, or the one being added (no
// Node). Comes from the "other_versions" 409 and from the catalog browse (#37).
export type TorrentVersion = {
  name: string;
  resolution: string;
  codec: string;
  size_bytes: number;
  node: string;
};

// OtherVersions: what the "other_versions" 409 says — the versions of the same work the cluster
// already has and the one being added. null for any other error.
export type OtherVersions = { versions: TorrentVersion[]; adding: TorrentVersion | null };
export function otherVersionsOf(e: unknown): OtherVersions | null {
  if (!(e instanceof ApiError) || e.status !== 409 || e.code !== "other_versions") return null;
  const versions = Array.isArray(e.details?.versions) ? (e.details!.versions as TorrentVersion[]) : [];
  const adding = (e.details?.adding as TorrentVersion | undefined) ?? null;
  return { versions, adding };
}

// serverError: the error that arrives from the API. When the Portal knows the code, the phrase comes from the
// catalog in the reader's language, with error.params (ADR-041 §4); otherwise error.message applies,
// which is the fallback and the detail ({detail} in the phrases that cite it).
type ErrorBody = {
  error?: { code?: string; message?: string; params?: Record<string, unknown> } & Record<string, unknown>;
} | null;
function serverError(status: number, body: ErrorBody): ApiError {
  const e = body?.error;
  const reserve = e?.message ?? portalMsg("api.httpError", { status });
  const text = e?.code ? messageText("error", e.code, e.params, reserve, { detail: e.message ?? "" }) : reserve;
  return new ApiError(status, text, e?.code, e);
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken();
  const res = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers ?? {}),
    },
  });
  if (res.status === 401) {
    clearToken();
    throw new ApiError(401, portalMsg("api.unauthorized"));
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw serverError(res.status, body);
  }
  return (await res.json()) as T;
}

export async function login(username: string, password: string): Promise<void> {
  const res = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) throw new ApiError(res.status, portalMsg("api.invalidCredentials"));
  const data = (await res.json()) as { token: string; default_credentials?: boolean };
  setToken(data.token);
  localStorage.setItem(USER_KEY, username);
  if (data.default_credentials) localStorage.setItem(DEFAULT_CREDS_KEY, "1");
  else localStorage.removeItem(DEFAULT_CREDS_KEY);
}

export const changePassword = (current_password: string, new_password: string) =>
  req<unknown>("/api/v1/auth/password", {
    method: "POST",
    body: JSON.stringify({ current_password, new_password }),
  });

// Time series coming from Prometheus, served by the CP (ADR-016).
export type MetricPoint = { t: number; v: number };
export type MetricSeries = { labels: Record<string, string>; points: MetricPoint[] };

export const getMetric = (metric: string, range = "6h") =>
  req<{ metric: string; data: MetricSeries[] }>(
    `/api/v1/metrics/query?metric=${encodeURIComponent(metric)}&range=${encodeURIComponent(range)}`,
  ).then((r) => r.data);

// getClusterThroughput (F2): CUMULATIVE cluster throughput (cumulative download/upload)
// over time, with server-side downsampling. `range` in Go duration (e.g., "168h" = 1
// week, "8760h" = 1 year). Returns two series (Downloaded/Uploaded) in Chart format.
export const getClusterThroughput = (range: string) =>
  req<{ data: MetricSeries[] }>(
    `/api/v1/metrics/cluster-throughput?range=${encodeURIComponent(range)}`,
  ).then((r) => r.data);

export const getNodes = () => req<{ data: Node[] }>("/api/v1/nodes").then((r) => r.data);

// getNodesAndMode fetches the Nodes + the global gate (ADR-024) in a single call — the Dashboard
// shows the Full auto/Flex switch and the "Mode" column without an extra request.
export const getNodesAndMode = () =>
  req<{ data: Node[]; cluster_operation_mode: ClusterMode }>("/api/v1/nodes");

// setClusterMode toggles the global gate (Full auto/Flex). Reverting to full_auto resets the
// per-Node modes to SG (full-auto is full-auto — ADR-024).
export const setClusterMode = (mode: ClusterMode) =>
  req<{ operation_mode: ClusterMode }>("/api/v1/cluster/operation-mode", {
    method: "PUT",
    body: JSON.stringify({ mode }),
  });

// setNodeMode sets a Node's mode (SG/Manual). Only allowed under the Flex gate.
export const setNodeMode = (id: string, mode: NodeMode) =>
  req<{ operation_mode: NodeMode }>(`/api/v1/nodes/${id}/operation-mode`, {
    method: "PUT",
    body: JSON.stringify({ mode }),
  });
// deleteNode removes a Seed Node. force=true also removes the Node's torrents;
// without force, the CP returns 409 (node_has_torrents) if there are still torrents.
export const deleteNode = (id: string, force = false) =>
  req<unknown>(`/api/v1/nodes/${id}${force ? "?force=true" : ""}`, { method: "DELETE" });

// renameNode sets (or, with "", removes) the Node's nickname (#145). It only lives in SeedGrid:
// nothing is sent to the Node and the OS hostname doesn't change. 409 display_name_taken when the name
// already belongs to another Node; 400 invalid_display_name when it fails validation.
export const renameNode = (id: string, display_name: string) =>
  req<{ display_name: string }>(`/api/v1/nodes/${id}/display-name`, {
    method: "PUT",
    body: JSON.stringify({ display_name }),
  });
export const getTorrents = () => req<{ data: Torrent[] }>("/api/v1/torrents").then((r) => r.data);

// Time series samples for a torrent (bytes/speed history).
export type TorrentSample = {
  t: number;
  downloaded_bytes: number;
  uploaded_bytes: number;
  dl_speed: number;
  up_speed: number;
};
export const getTorrentSamples = (id: string, range = "6h") =>
  req<{ data: TorrentSample[] }>(
    `/api/v1/torrents/${id}/samples?range=${encodeURIComponent(range)}`,
  ).then((r) => r.data);
// allowOtherVersion: the operator saw the versions of the same work the cluster already has and
// wants to add anyway (#37). An identical file (same hash) has no confirmation: it's the
// "duplicate" 409 and that's it — the Portal no longer offers a 2nd copy.
export const addTorrentMagnet = (magnet: string, name?: string, allowOtherVersion = false) =>
  req<Torrent>("/api/v1/torrents", {
    method: "POST",
    body: JSON.stringify({ magnet, name, allow_other_version: allowOtherVersion }),
  });

// addTorrentFile sends the .torrent as multipart (the browser sets the Content-Type).
export async function addTorrentFile(file: File, allowOtherVersion = false): Promise<Torrent> {
  const token = getToken();
  const fd = new FormData();
  fd.append("torrent", file);
  if (allowOtherVersion) fd.append("allow_other_version", "true");
  const res = await fetch("/api/v1/torrents", {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    body: fd,
  });
  if (res.status === 401) {
    clearToken();
    throw new ApiError(401, portalMsg("api.unauthorized"));
  }
  if (!res.ok) {
    const b = await res.json().catch(() => null);
    throw serverError(res.status, b);
  }
  return (await res.json()) as Torrent;
}

// Torrent actions: each queues a command on the Node and returns the command_id,
// so the Portal can track progress via getCommand (GET /commands/{id}).
type ActionResponse = { status: string; command_id: string };
export const pauseTorrent = (id: string) =>
  req<ActionResponse>(`/api/v1/torrents/${id}/pause`, { method: "POST" }).then((r) => r.command_id);
export const resumeTorrent = (id: string) =>
  req<ActionResponse>(`/api/v1/torrents/${id}/resume`, { method: "POST" }).then((r) => r.command_id);
// recheckTorrent forces data re-verification in the Engine (recovers a torrent in
// ERROR, e.g., after the disk reconnects — ADR-018/maintenance).
export const recheckTorrent = (id: string) =>
  req<ActionResponse>(`/api/v1/torrents/${id}/recheck`, { method: "POST" }).then((r) => r.command_id);
export const removeTorrent = (id: string) =>
  req<ActionResponse>(`/api/v1/torrents/${id}`, { method: "DELETE" }).then((r) => r.command_id);
// Manual queue prioritization (F3): doesn't queue a command on the Node — it only sets/unsets the
// state in the Control Plane, which the admission controller consumes on the next pass.
export const prioritizeTorrent = (id: string) =>
  req<{ status: string }>(`/api/v1/torrents/${id}/prioritize`, { method: "POST" }).then(() => {});
export const unprioritizeTorrent = (id: string) =>
  req<{ status: string }>(`/api/v1/torrents/${id}/prioritize`, { method: "DELETE" }).then(() => {});
// restartEngine restarts the Node's Torrent Engine (the container comes back up via
// restart policy). Returns the command_id to track progress.
export const restartEngine = (nodeId: string) =>
  req<ActionResponse>(`/api/v1/nodes/${nodeId}/engine/restart`, { method: "POST" }).then(
    (r) => r.command_id,
  );

// Agent version published by the CP (ADR-027) — compared with each Node's agent_version
// to show "update available". 503 = `agent.artifacts_url` not configured (feature
// disabled); we return null so the UI simply doesn't show anything.
export type AgentRelease = { version: string; archs: string[] };
export const getAgentAvailable = async (): Promise<AgentRelease | null> => {
  try {
    return await req<AgentRelease>("/api/v1/agent/available");
  } catch {
    return null;
  }
};

// updateAgent queues the Agent binary replacement for that Node (ADR-027). The CP resolves the
// binary for the Node's architecture; returns the command_id to track progress.
export const updateAgent = (nodeId: string) =>
  req<ActionResponse>(`/api/v1/nodes/${nodeId}/update-agent`, { method: "POST" }).then(
    (r) => r.command_id,
  );

// State of a queued command (to display the action's progress in the Portal).
export type CommandStatus = {
  id: string;
  type: string;
  status: string; // PENDING | DISPATCHED | DONE | FAILED
  error?: string;
  created_at: string;
  dispatched_at?: string;
  finished_at?: string;
};
export const getCommand = (id: string) => req<CommandStatus>(`/api/v1/commands/${id}`);

// Torrent migration between Nodes (ADR-018).
export type Migration = {
  id: string;
  torrent_id: string;
  source_node_id: string;
  dest_node_id: string;
  status: string; // QUEUED | PENDING | COMPLETED | FAILED
  error?: string;
  // FAILED terminated by the operator (POST /migrations/{id}/cancel) — not a migration failure.
  cancelled?: boolean;
  created_at: string;
  completed_at?: string;
};
export const migrateTorrent = (id: string, dest_node_id: string) =>
  req<Migration>(`/api/v1/torrents/${id}/migrate`, {
    method: "POST",
    body: JSON.stringify({ dest_node_id }),
  });
export const drainNode = (id: string, dest_node_id: string) =>
  req<{ data: Migration[]; count: number }>(`/api/v1/nodes/${id}/drain`, {
    method: "POST",
    body: JSON.stringify({ dest_node_id }),
  });
export const getMigrations = (limit = 50) =>
  req<{ data: Migration[] }>(`/api/v1/migrations?limit=${limit}`).then((r) => r.data);
// Cancels an in-flight migration (requires torrents:migrate): source resumed, destination copy
// discarded. 409 if already finished (migration_finished) or already canceled.
export const cancelMigration = (id: string) =>
  req<Migration>(`/api/v1/migrations/${id}/cancel`, { method: "POST" });

// Operator trackers (ADR-019/ADR-020): announce credentials to publish to a
// private tracker. The API never returns the announce/passkey nor the api_token — only
// name + host + source/api_url + has_api_token (boolean). source/api_url/api_token
// enable the UNIT3D integration (automatic site registration, ADR-020).
export type Tracker = {
  id: string;
  name: string;
  host: string;
  source: string; // .torrent tag (e.g.: TRACKER); "" = common tracker
  api_url: string; // UNIT3D site base; "" = no integration
  has_api_token: boolean; // API token present (never returned)
  has_image_host_key: boolean; // registered image host key (#193)
  image_host_key_host: string; // host for which the key was registered (#193)
  is_default: boolean;
  created_at: string;
  profile: TrackerProfile; // SITE profile (taxonomy): this is where the publish panel reads the options from
  // The description composition that applies when the profile does NOT have the `description` section (#192) — and,
  // by listing all blocks in their original order, the catalog of what the editor offers. It comes from
  // the server so the Portal doesn't duplicate the default.
  description_default: DescriptionConfig;
};

// Tracker site profile (ADR-037, layer 2): the knowledge that belongs to the site and not the
// operator — the category/type/resolution IDs, which vary from site to site. No
// credentials by design: announce and api_token are layer 3 and never come here.
export type TaxonomyAxis = "category" | "type" | "resolution";
// Origin of a term: seed (migration), discovered (discovery in the site catalog), manual (the
// operator typed/renamed it) or imported (came from a profile file, #197). Discovered and imported
// never overwrite manual — the origin is LOCAL state and doesn't travel in the file.
export type TermOrigin = "seed" | "discovered" | "manual" | "imported";
// Semantic role of a category (only on the category axis): what the panel needs to know
// beyond the number — show season/episode (tv) and the IGDB game search (game).
export type TermKind = "movie" | "tv" | "game";
export type TaxonomyTerm = { id: number; name: string; kind?: TermKind; origin: TermOrigin };
export type Taxonomy = Record<TaxonomyAxis, TaxonomyTerm[]>;
export type TrackerProfile = {
  family: string;
  schema_version: number;
  taxonomy: Taxonomy;
  // Upload description composition (#192). Absent = the default (`description_default` of the tracker).
  description?: DescriptionConfig;
  // Tracker image host (#193, ADR-040): the site's own host for hosting screenshots.
  // Absent = only the global imgbb. The operator's key is tied to the upload_url host.
  image_host?: ImageHostSpec;
};
// Upload description composition (#192): which blocks go in and in what order (omitting from the list =
// disable; `[]` = none), and the tag that wraps the MediaInfo. The ids are a closed set from the
// server; tag: mediainfo | code | pre | spoiler.
// language (#74): the content language of the site — description headings and the TMDB synopsis.
// Missing means English.
export type DescriptionConfig = { media_blocks: string[]; game_blocks: string[]; media_info_tag: string; language?: string };

// Tracker image host (#193, ADR-040): closed-form description of the site's own host
// for hosting screenshots. The operator's key is layer 3 — comes in via its own route and is
// tied to the upload_url host (tracker.has_image_host_key / tracker.image_host_key_host).
export type ImageHostSpec = {
  name: string;
  upload_url: string;
  file_field: string;
  auth_header: string;
  auth_prefix: string;
  url_path: string;
  thumbnail_path?: string; // absent = no thumbnail (the server omits the empty one)
};

// ImportImageHost: the `image_host` section of the file versus the profile's (#193). Same section-unit
// rule as `description`: new/same/protected/replaced.
export type ImportImageHostStatus = "new" | "same" | "protected" | "replaced";
export type ImportImageHost = {
  status: ImportImageHostStatus;
  config: ImageHostSpec;
  current?: ImageHostSpec;
  overwrites_protected?: boolean;
};

// TermInput: term as the operator sends it during editing. The origin is decided by the server.
export type TermInput = { id: number; name: string; kind?: TermKind };

// Taxonomy discovery (#196): proposed from the tracker's own catalog. NEVER
// applied directly — the operator checks, edits, and tells it to apply whatever they want.
export type ProposalStatus = "new" | "same" | "renamed";
export type ProposalItem = {
  id: number;
  name: string; // the name the site uses
  seen: number; // how many times it appeared in what was scanned
  status: ProposalStatus;
  current_name?: string; // when renamed: what the profile has today
  current_origin?: TermOrigin;
  protected?: boolean; // renamed + manual origin: applying would do nothing
};
export type StopReason = "max_pages" | "end_of_catalog" | "rate_limited" | "time_budget" | "error";
export type DiscoveryResult = {
  family: string;
  pages_requested: number;
  pages_scanned: number;
  items_scanned: number;
  stopped_reason: StopReason;
  proposal: Record<TaxonomyAxis, ProposalItem[]>;
  rejected: number; // catalog pairs discarded by validation (third-party content)
  warnings: string[];
  notice: string; // the warning that the discovery is incomplete by definition
};
export type ApplyItem = { axis: TaxonomyAxis; id: number; name: string; origin?: "discovered" | "manual" };
export type ApplyResult = {
  profile: TrackerProfile;
  applied: { axis: TaxonomyAxis; id: number; name: string; action: "added" | "renamed" }[];
  skipped: { axis: TaxonomyAxis; id: number; name: string; reason: "same" | "protected" }[];
};
// TrackerInput: write fields. During editing, an empty api_token keeps the current token.
export type TrackerInput = {
  name: string;
  announce: string;
  source?: string;
  api_url?: string;
  api_token?: string;
  is_default?: boolean;
};
export const getTrackers = () => req<{ data: Tracker[] }>("/api/v1/trackers").then((r) => r.data);
export const createTracker = (input: TrackerInput) =>
  req<Tracker>("/api/v1/trackers", {
    method: "POST",
    body: JSON.stringify(input),
  });
export const updateTracker = (id: string, input: TrackerInput) =>
  req<Tracker>(`/api/v1/trackers/${id}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
export const deleteTracker = (id: string) =>
  req<unknown>(`/api/v1/trackers/${id}`, { method: "DELETE" });
export const setDefaultTracker = (id: string) =>
  req<unknown>(`/api/v1/trackers/${id}/default`, { method: "POST" });
// Manual taxonomy editing: the sent axes REPLACE the current ones; the absent ones stay.
export const saveTrackerProfile = (id: string, taxonomy: Partial<Record<TaxonomyAxis, TermInput[]>>) =>
  req<TrackerProfile>(`/api/v1/trackers/${id}/profile`, {
    method: "PUT",
    body: JSON.stringify({ taxonomy }),
  });
// Description composition (#192): the profile's `description` section. PUT saves it (the server validates
// everything and rejects with 400 invalid_profile and the field path); DELETE reverts to the default.
export const saveDescriptionConfig = (id: string, cfg: DescriptionConfig) =>
  req<TrackerProfile>(`/api/v1/trackers/${id}/profile/description`, {
    method: "PUT",
    body: JSON.stringify(cfg),
  });
export const resetDescriptionConfig = (id: string) =>
  req<TrackerProfile>(`/api/v1/trackers/${id}/profile/description`, { method: "DELETE" });

// Tracker image host (#193, ADR-040): the profile's `image_host` section. PUT saves the flat
// Spec (the server validates everything); DELETE deletes it. The operator's key comes in via its own route.
export const saveImageHostSpec = (id: string, spec: ImageHostSpec) =>
  req<TrackerProfile>(`/api/v1/trackers/${id}/profile/image-host`, {
    method: "PUT",
    body: JSON.stringify(spec),
  });
export const resetImageHostSpec = (id: string) =>
  req<TrackerProfile>(`/api/v1/trackers/${id}/profile/image-host`, { method: "DELETE" });

// Operator key for the image host (#193, ADR-040, layer 3). Registered via its own
// route — tied to the current upload_url host. Empty key is 400; profile without image_host
// is 409. The key never comes back in the API; the tracker only brings has_image_host_key and the host.
export const setImageHostKey = (id: string, key: string) =>
  req<{ status: string }>(`/api/v1/trackers/${id}/image-host-key`, {
    method: "PUT",
    body: JSON.stringify({ key }),
  });
export const clearImageHostKey = (id: string) =>
  req<{ status: string }>(`/api/v1/trackers/${id}/image-host-key`, { method: "DELETE" });
// Discover only PROPOSES (nothing is saved). `pages` absent = the server default.
export const discoverTaxonomy = (id: string, pages?: number) =>
  req<DiscoveryResult>(`/api/v1/trackers/${id}/discover-taxonomy`, {
    method: "POST",
    body: JSON.stringify(pages ? { pages } : {}),
  });
// Apply runs the coexistence rule ON THE SERVER: completes what's missing and never overwrites
// what the operator adjusted by hand.
export const applyTaxonomy = (id: string, items: ApplyItem[]) =>
  req<ApplyResult>(`/api/v1/trackers/${id}/apply-taxonomy`, {
    method: "POST",
    body: JSON.stringify({ items }),
  });

// Profile export/import (#197). The file is layer 2 (site profile) with an identification
// envelope: it never carries a token, passkey, or announce. The import is third-party content — the
// server validates everything, and the Portal only sends the file TEXT, without reinterpreting it.
export type ProfileSite = { name: string; host?: string };
// new/same/renamed as in the discovery proposal; protected = the term belongs to the operator (manual) or
// was confirmed by their site (discovered) and the import keeps it — unless the operator asks,
// term by term, to overwrite.
export type ImportStatus = "new" | "same" | "renamed" | "protected";
export type ImportItem = {
  axis: TaxonomyAxis;
  id: number;
  name: string; // the name the file brings
  kind?: TermKind;
  status: ImportStatus;
  current_name?: string; // renamed/protected: what the profile has today
  current_origin?: TermOrigin;
  sets_kind?: boolean; // the profile has no role for this term and the file brings one
  overwrites_protected?: boolean; // the operator asked to overwrite a protected name
  warnings?: string[]; // collisions of this term with what the profile already has (repeated name, role that changes the default)
};
export type ImportCounts = {
  new: number;
  renamed: number;
  same: number;
  protected: number;
  kind_set: number;
  local_only: number; // profile terms that the file doesn't bring (they stay as they are)
};
// The file's `description` section versus the profile's (#192): new = the profile had no config
// of its own; same; protected = the operator's is kept, unless they ask (overwrite=description);
// replaced = they asked and it will be swapped.
export type ImportDescriptionStatus = "new" | "same" | "protected" | "replaced";
export type ImportDescription = {
  status: ImportDescriptionStatus;
  config: DescriptionConfig; // what the file brings
  current?: DescriptionConfig; // protected/replaced: what the profile has today
  overwrites_protected?: boolean;
};
export type ImportPlan = {
  site: ProfileSite;
  family: string;
  schema_version: number;
  items: Record<TaxonomyAxis, ImportItem[]>;
  counts: ImportCounts;
  description?: ImportDescription; // only when the file brings the section
  image_host?: ImportImageHost; // only when the file brings the section
  warnings: string[];
};
export type ImportPreview = { plan: ImportPlan; notice: string };
export type ImportResult = { profile: TrackerProfile; plan: ImportPlan };

// Overwrite requests go in the query (`axis:id`, or `description` for the description
// composition), so the body is the file, byte by byte.
const overwriteQuery = (keys: string[]) =>
  keys.length ? "?" + keys.map((k) => `overwrite=${encodeURIComponent(k)}`).join("&") : "";

// Preview NEVER saves. `fileText` is the file content, as is.
export const previewProfileImport = (id: string, fileText: string, overwrite: string[] = []) =>
  req<ImportPreview>(`/api/v1/trackers/${id}/profile/import/preview${overwriteQuery(overwrite)}`, {
    method: "POST",
    body: fileText,
  });
// Apply is the second explicit call. The rule that protects what the operator adjusted by hand runs
// on the server, over the locked state.
export const importTrackerProfile = (id: string, fileText: string, overwrite: string[] = []) =>
  req<ImportResult>(`/api/v1/trackers/${id}/profile/import${overwriteQuery(overwrite)}`, {
    method: "POST",
    body: fileText,
  });

// Downloads the exchange file. The route requires the Authorization header, so you can't browse
// directly (like in torrent download, which uses a ticket): fetches the body and delivers it to the browser as
// a file. Returns the name it was saved with.
export async function downloadTrackerProfile(id: string): Promise<string> {
  const token = getToken();
  const res = await fetch(`/api/v1/trackers/${id}/profile/export`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (res.status === 401) {
    clearToken();
    throw new ApiError(401, portalMsg("api.unauthorized"));
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw serverError(res.status, body);
  }
  const filename = filenameFromDisposition(res.headers.get("Content-Disposition"));
  const url = URL.createObjectURL(await res.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
  return filename;
}

// PendingPublish: torrent created with a tracker but not yet published to it
// (ADR-020, backlog #8) — the Publish tab queue to reopen publishing without
// redoing the upload.
export type PendingPublish = {
  id: string;
  name?: string;
  status: string;
  created_at?: string;
  has_file: boolean; // .torrent persisted (prerequisite for publishing)
  tracker_id: string;
  tracker_name: string;
  tracker_source?: string;
};
export const getPendingPublish = () =>
  req<{ pending: PendingPublish[] }>("/api/v1/torrents/pending-tracker").then((r) => r.pending);

// Content download via Portal (backlog #1): issues a one-time ticket (protected
// route) and triggers browser navigation to the download URL (the CP proxies
// the Node's stream). Single file comes raw (real name); folder comes as .tar.
export const createDownloadTicket = (id: string) =>
  req<{ ticket: string; url: string }>(`/api/v1/torrents/${id}/download-ticket`, { method: "POST" });

export async function downloadTorrentContent(id: string): Promise<void> {
  const { url } = await createDownloadTicket(id);
  const a = document.createElement("a");
  a.href = url; // relative to the Portal's origin; nginx proxies to the Control Plane
  a.rel = "noreferrer";
  document.body.appendChild(a);
  a.click();
  a.remove();
}

// Browse tracker catalog (backlog #12): lists UNIT3D tracker torrents and
// marks those already in the cluster (in_cluster, heuristic cross-referencing by name — UNIT3D
// doesn't expose info_hash). "Download & Seed" downloads the .torrent and adds it to the cluster.
// "Enriched" fields (title_pt, poster_url, etc.) come from the CP querying TMDB/IGDB/
// OpenLibrary by tmdb_id or heuristics; they may be empty if the provider is missing a key
// or found no match — the UI handles all with a local fallback.
export type MediaType = "movie" | "tv" | "game" | "book" | "audiobook" | "other" | "";
export type TrackerBrowseItem = {
  id: string;
  name: string;
  category: string;
  type: string;
  size: number;
  seeders: number;
  leechers: number;
  times_completed: number;
  tmdb_id: number;
  imdb_id: string;
  year: number;
  details_link: string;
  in_cluster: boolean; // the SAME file is already in the cluster (size + name, #37)
  other_versions?: TorrentVersion[]; // other versions of the same work the cluster already has (#37)
  // Enrichment (#12) — all optional.
  media_type?: MediaType;
  title_pt?: string; // translated title; for books, the OpenLibrary Title
  title_original?: string; // original title; for books, the author (we reuse the slot)
  poster_url?: string;
  overview?: string;
  genres?: string[];
  original_language?: string; // "en", "ja", "pt", …
  resolution?: string; // "1080p", "2160p" …
  audio_langs?: string[]; // ["PT-BR DUB", "Original"] via name parsing
  subtitle_langs?: string[]; // ["PT-BR LEG"] via name parsing
};
export const browseTracker = (trackerId: string, cursor = "", q = "") => {
  const p = new URLSearchParams();
  if (cursor) p.set("cursor", cursor);
  if (q) p.set("q", q); // server-side search (UNIT3D param name)
  const qs = p.toString();
  return req<{ items: TrackerBrowseItem[]; next_cursor: string }>(
    `/api/v1/trackers/${trackerId}/browse${qs ? `?${qs}` : ""}`,
  );
};
// DupeCandidate: tracker catalog item that might be a duplicate of the release being
// published (#186). It's evidence for human review, not a verdict.
export type DupeCandidate = TrackerBrowseItem & { same_year?: boolean };

// trackerDupes looks for duplicates by the title inferred from the name. It never fails "upwards":
// a down tracker returns 200 with an empty list and `note` — the search is an aid, and aid
// failure must not block publishing.
export const trackerDupes = (trackerId: string, name: string) =>
  req<{ query: string; candidates: DupeCandidate[]; note?: string }>(
    `/api/v1/trackers/${trackerId}/dupes?name=${encodeURIComponent(name)}`,
  );

export const seedFromTracker = (trackerId: string, torrentId: string, allowOtherVersion = false) =>
  req<Torrent>(`/api/v1/trackers/${trackerId}/seed`, {
    method: "POST",
    body: JSON.stringify({ torrent_id: torrentId, allow_other_version: allowOtherVersion }),
  });

// publishContent sends local files to the Cluster (ADR-019): the chosen Node
// creates the .torrent and seeds it right away. Uses XHR to report upload progress.
// trackerId: "public" (DHT, no tracker) or the ID of a registered tracker;
// empty forceNodeId = the Scheduler picks the Node.
// PublishResult = created torrent + mediainfo (flattened in the CP response, ADR-020).
export type PublishResult = Torrent & { media_info?: string };

// InferResult: metadata inferred from a release name (ADR-020) — parsing + TMDB.
export type InferResult = {
  parsed: {
    title: string;
    year: number;
    season: number;
    episode: number;
    is_tv: boolean;
    resolution: string;
    type: string;
    group: string;
    imdb: string;
    source: string;
    encoding: string;
    subsource: string;
    audio: string;
    channels: string;
    flags: string[] | null;
    standard_name: string; // name in the release naming convention
  };
  tmdb: {
    tmdb_id: number;
    imdb: string;
    tvdb: number;
    title: string;
    year: number;
    overview: string;
    poster_url: string;
    is_tv: boolean;
  } | null;
  tmdb_note?: string;
};
// inferPublish infers publishing fields from the name; when passing mediainfo,
// the standardized name is enriched with resolution/codec/audio (ADR-020).
// trackerId (#74): the synopsis comes in that tracker's content language.
export const inferPublish = (name: string, tv?: boolean, mediainfo?: string, trackerId?: string) =>
  req<InferResult>("/api/v1/publish/infer", {
    method: "POST",
    body: JSON.stringify({ name, tv, mediainfo: mediainfo ?? "", tracker_id: trackerId ?? "" }),
  });

// BBCode description composition (#192): the Control Plane builds the text — per tracker, from its
// profile's `description` section — and the Portal just delivers it to the field, which the operator keeps
// editing. Failure does NOT clear the field: the caller shows the error and leaves the text as it was.
export type ComposeMediaInput = {
  poster: string;
  overview: string;
  keywords: string;
  screenshots: string; // pre-built BBCode block from the CP (ADR-026)
  mediainfo: string;
};
export const composeMediaDescription = (trackerId: string, media: ComposeMediaInput) =>
  req<{ description: string }>("/api/v1/publish/description", {
    method: "POST",
    body: JSON.stringify({ tracker_id: trackerId, kind: "media", media }),
  }).then((r) => r.description);

// composeGameDescription builds a game description from the IGDB candidate (#17).
export const composeGameDescription = (trackerId: string, g: GameCandidate) =>
  req<{ description: string }>("/api/v1/publish/description", {
    method: "POST",
    body: JSON.stringify({
      tracker_id: trackerId,
      kind: "game",
      game: {
        cover_url: g.cover_url ?? "",
        name: g.name,
        year: g.year ?? 0,
        summary: g.summary ?? "",
        genres: g.genres ?? [],
        platforms: g.platforms ?? [],
      },
    }),
  }).then((r) => r.description);

// PublishTrackerPayload: metadata reviewed by the operator for UNIT3D registration.
export type PublishTrackerPayload = {
  tracker_id: string;
  name: string;
  description: string;
  category_id: number;
  type_id: number;
  resolution_id: number;
  tmdb: number;
  imdb: string;
  tvdb: number;
  mal: number;
  igdb: number; // games (#17)
  season_number: number;
  episode_number: number;
  keywords: string;
  mediainfo: string;
  anonymous: boolean;
};
export const publishToTracker = (
  torrentId: string,
  p: PublishTrackerPayload & { preview_token?: string },
) =>
  req<{ status: string; download_url: string; message: string }>(
    `/api/v1/torrents/${torrentId}/publish-tracker`,
    { method: "POST", body: JSON.stringify(p) },
  );

// PublishPreview: the EXACT payload that submit would send (#188). UNIT3D has no
// dry-run — without this, the only way to know what will be posted is to post it.
export type PublishPreview = {
  endpoint: string;
  tracker: string;
  fields: Record<string, string>;
  torrent_name: string;
  torrent_size: number;
  missing: string[];
  token: string; // payload printout; returned on submit and locks the modified record (409)
};

export const previewPublish = (torrentId: string, p: PublishTrackerPayload) =>
  req<PublishPreview>(`/api/v1/torrents/${torrentId}/publish-tracker/preview`, {
    method: "POST",
    body: JSON.stringify(p),
  });

// generateScreenshots (ADR-026): asks the CP to extract screenshots from the torrent
// (via Agent) and upload them to an image host, returning the BBCode ready to paste into the
// description — the operator reviews/edits before publishing (human review).
// optional tracker_id (#193): if present, the tracker's host joins the upload chain.
export type ScreenshotFailure = { host: string; reason: string };
export type ScreenshotResult = {
  bbcode: string;
  host?: string; // hosting host (DNS name of the upload_url or "imgbb")
  failures?: ScreenshotFailure[]; // failures of previous hosts in the chain
};
export const generateScreenshots = (torrentId: string, trackerId?: string) =>
  req<ScreenshotResult>(`/api/v1/torrents/${torrentId}/screenshots`, {
    method: "POST",
    body: JSON.stringify(trackerId ? { tracker_id: trackerId } : {}),
  });

// Step-based flow (F1): extractScreenshots queues the extraction on the Node and returns the
// command_id (the Portal tracks the "Extracting on Node" step via getCommand); then
// renderScreenshots reads the frames from that command, uploads them to the host chain, and returns the BBCode.
export const extractScreenshots = (torrentId: string) =>
  req<{ command_id: string }>(`/api/v1/torrents/${torrentId}/screenshots/extract`, {
    method: "POST",
  });
// optional tracker_id (#193): if present, the tracker host enters the upload chain.
export const renderScreenshots = (commandId: string, trackerId?: string) =>
  req<ScreenshotResult>(`/api/v1/screenshots/render`, {
    method: "POST",
    body: JSON.stringify({ command_id: commandId, ...(trackerId ? { tracker_id: trackerId } : {}) }),
  });

// GameCandidate: a result from the IGDB game search (#17).
export type GameCandidate = {
  igdb_id: number;
  name: string;
  year: number;
  summary: string;
  cover_url: string;
  genres: string[] | null;
  platforms: string[] | null;
};
export const searchGames = (q: string) =>
  req<{ games: GameCandidate[] }>(`/api/v1/publish/games/search?q=${encodeURIComponent(q)}`).then(
    (r) => r.games,
  );

// extOf returns the extension (with dot) of a filename, or "".
function extOf(n: string): string {
  const i = n.lastIndexOf(".");
  return i > 0 ? n.slice(i) : "";
}

export function publishContent(
  files: File[],
  opts: {
    name: string;
    trackerId: string;
    forceNodeId?: string;
    // standardizeFile: for a single file, renames the FILE inside the .torrent to
    // the standard name (opts.name + extension) — not just the folder (ADR-020, UNIT3D tracker).
    standardizeFile?: boolean;
    onProgress?: (pct: number) => void;
  },
): Promise<PublishResult> {
  return new Promise((resolve, reject) => {
    const token = getToken();
    const fd = new FormData();
    // Manifest: name+size per file. For a single file on a UNIT3D tracker,
    // the file is renamed to the standard (the CP uses the manifest name in the tar).
    const manifest = files.map((f) => ({ name: f.name, size: f.size }));
    if (opts.standardizeFile && files.length === 1 && opts.name.trim()) {
      manifest[0].name = opts.name.trim() + extOf(files[0].name);
    }
    // Order matters: text fields before files (the CP reads via streaming).
    fd.append("name", opts.name);
    fd.append("tracker_id", opts.trackerId);
    fd.append("force_node_id", opts.forceNodeId ?? "");
    fd.append("manifest", JSON.stringify(manifest));
    for (const f of files) fd.append("files", f);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/v1/torrents/create");
    if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && opts.onProgress) opts.onProgress(Math.round((e.loaded / e.total) * 100));
    };
    xhr.onload = () => {
      if (xhr.status === 401) {
        clearToken();
        reject(new ApiError(401, portalMsg("api.unauthorized")));
        return;
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(JSON.parse(xhr.responseText) as PublishResult);
        return;
      }
      let body: ErrorBody = null;
      try {
        body = JSON.parse(xhr.responseText) as ErrorBody;
      } catch {
        /* non-JSON response: "error {status}" applies */
      }
      reject(serverError(xhr.status, body));
    };
    xhr.onerror = () => reject(new ApiError(0, portalMsg("api.networkError")));
    xhr.send(fd);
  });
}

export const getAlerts = (openOnly = false) =>
  req<{ data: Alert[] }>(`/api/v1/alerts${openOnly ? "?status=open" : ""}`).then((r) => r.data);
export const getEvents = (limit = 50) =>
  req<{ data: Event[] }>(`/api/v1/events?limit=${limit}`).then((r) => r.data);
export const getActivities = (limit = 100, torrentId?: string) =>
  req<{ data: Activity[] }>(
    `/api/v1/activities?limit=${limit}${torrentId ? `&torrent_id=${encodeURIComponent(torrentId)}` : ""}`,
  ).then((r) => r.data);

// Audit trail (ADR-025): operator actions (who did what, when, from where).
export type AuditEntry = {
  id: string;
  actor: string;
  action: string; // the Portuguese phrase recorded by the Control Plane: the fallback
  // Where the Portal builds the phrase in the reader's language (#57). Missing in older records.
  action_code?: string;
  action_params?: Record<string, unknown> | null;
  method?: string;
  path?: string;
  status_code?: number;
  source_ip?: string;
  created_at: string;
};
export const getAudit = (limit = 150, actor = "") =>
  req<{ data: AuditEntry[] }>(
    `/api/v1/audit?limit=${limit}${actor ? `&actor=${encodeURIComponent(actor)}` : ""}`,
  ).then((r) => r.data);

// Dynamic tunables (ADR-023): Settings panel with hot-reload. value/default are
// strings (duration "5m", number "51200"); source indicates if there's a DB override.
export type SettingItem = {
  key: string;
  type: "duration" | "int" | "int64" | "float" | "string" | "bool";
  label: string;
  description: string;
  unit: string;
  value: string; // always "" when secret — the value never returns from the API (ADR-028)
  default: string;
  source: "db" | "default";
  min: number | null;
  max: number | null;
  options?: string[]; // enum → select
  multi?: boolean; // multi-select (CSV) → checkboxes
  choices?: { value: string; label: string }[]; // multi: code → friendly label
  secret?: boolean; // user credential: password input, empty = keep (ADR-028)
  has_value?: boolean; // secret: whether it's filled (without revealing what)
  per_node?: boolean; // CLUSTER value of an Agent/Engine tunable: each Node can have its own (ADR-039)
};
export type SettingGroup = { category: string; label: string; items: SettingItem[] };
export const getSettings = () => req<{ groups: SettingGroup[] }>("/api/v1/settings").then((r) => r.groups);
export const updateSetting = (key: string, value: string) =>
  req<SettingItem>(`/api/v1/settings/${encodeURIComponent(key)}`, {
    method: "PUT",
    body: JSON.stringify({ value }),
  });
export const resetSetting = (key: string) =>
  req<SettingItem>(`/api/v1/settings/${encodeURIComponent(key)}`, { method: "DELETE" });

// ── Agent/Engine behavior configuration per Node (ADR-039) ─────────────────────
// The CLUSTER value is the catalog's (updateSetting/resetSetting, settings:write permission);
// here lives the PER NODE value: the effective one, where it comes from, and the override (nodes:write permission).
export type NodeSettingItem = {
  key: string;
  type: SettingItem["type"];
  label: string;
  description: string;
  unit: string;
  min: number | null;
  max: number | null;
  options?: string[];
  value: string; // effective: what the Agent receives
  source: "node" | "cluster"; // where the effective value comes from
  cluster_value: string; // what the Node would have without an override
  cluster_source: "db" | "default"; // the operator set the cluster value, or it's the catalog default
  override: string | null; // the Node's own value; null = none
};
// in_sync: the Agent applied today's version · pending: changed and the Agent hasn't confirmed yet ·
// unsupported: the Agent never reported it (pre-ADR-039; still in the local .env).
export type NodeConfigStatus = "in_sync" | "pending" | "unsupported";
export type NodeSettings = {
  node_id: string;
  status: NodeConfigStatus;
  effective_version: string;
  applied_version: string | null;
  applied_at: string | null;
  seeded_at: string | null;
  items: NodeSettingItem[];
};
export const getNodeSettings = (nodeId: string) =>
  req<NodeSettings>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/settings`);
export const setNodeSetting = (nodeId: string, key: string, value: string) =>
  req<NodeSettingItem>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/settings/${encodeURIComponent(key)}`, {
    method: "PUT",
    body: JSON.stringify({ value }),
  });
export const resetNodeSetting = (nodeId: string, key: string) =>
  req<NodeSettingItem>(`/api/v1/nodes/${encodeURIComponent(nodeId)}/settings/${encodeURIComponent(key)}`, {
    method: "DELETE",
  });
// Nodes with their own value — on the Settings screen, it answers "if I change the
// cluster value, who is NOT affected?".
export type SettingOverride = {
  node_id: string;
  node_label: string;
  key: string;
  label: string;
  unit: string;
  value: string;
  cluster_value: string;
  updated_at: string;
  updated_by?: string;
};
export const getSettingOverrides = () =>
  req<{ data: SettingOverride[] }>("/api/v1/settings/overrides").then((r) => r.data);

// Sends a test message through the configured channels (ADR-028). Synchronous: the channel
// error (invalid token, wrong chat_id) returns to the screen — configuring notifications without
// feedback is pure guessing.
// notifyTest sends a test message. With channel, it tests only that channel
// (telegram|discord|webhook); without, all configured ones.
export const notifyTest = (channel?: string) =>
  req<{ status: string }>(
    `/api/v1/notify/test${channel ? `?channel=${encodeURIComponent(channel)}` : ""}`,
    { method: "POST" },
  );

// ── Users and RBAC (ADR-035) ────────────────────────────────────────────────

// Me: profile of the authenticated operator, with roles and permissions (ADR-035).
export type Me = { id: string; username: string; roles: string[]; permissions: string[] };
export const getMe = () => req<Me>("/api/v1/auth/me");

// System roles (ADR-035). The Portal filters actions and pages by permission.
export const ROLES = ["Administrator", "Publisher", "Downloader", "Viewer"] as const;
export type Role = (typeof ROLES)[number];

// hasPermission mirrors the backend logic (ADR-035): Administrator resolves as
// full access; wildcard `*:*`/`resource:*`/`*:action` on the user side covers the
// requested permission. Wildcard on the requested permission side DOES NOT grant anything by itself.
export function hasPermission(me: Me | null | undefined, perm: string): boolean {
  if (!me) return false;
  if (me.roles.includes("Administrator")) return true;
  const [wantR, wantA] = perm.split(":");
  for (const p of me.permissions) {
    const [gotR, gotA] = p.split(":");
    // Route requesting wildcard (poorly declared) only matches if the user also has
    // wildcard in that position — `torrents:read` does not cover `*:*`.
    if (wantR === "*" && gotR !== "*") continue;
    if (wantA === "*" && gotA !== "*") continue;
    const rMatch = gotR === "*" || wantR === "*" || gotR === wantR;
    const aMatch = gotA === "*" || wantA === "*" || gotA === wantA;
    if (rMatch && aMatch) return true;
  }
  return false;
}

// User: management view of an operator (without credentials).
export type User = {
  id: string;
  username: string;
  display_name: string;
  email: string;
  status: string;
  roles: string[];
  last_login_at: string | null;
  created_at: string;
  updated_at: string;
};

export type UserInput = {
  username: string;
  display_name?: string;
  email?: string;
  password: string;
  roles: string[];
};

export const listUsers = () => req<{ data: User[] }>("/api/v1/users").then((r) => r.data);
export const getUser = (id: string) => req<User>(`/api/v1/users/${encodeURIComponent(id)}`);
export const createUser = (input: UserInput) =>
  req<User>("/api/v1/users", { method: "POST", body: JSON.stringify(input) });
export const updateUser = (
  id: string,
  input: { display_name?: string; email?: string; status?: string; roles?: string[] },
) => req<User>(`/api/v1/users/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(input) });
export const deleteUser = (id: string) =>
  req<{ status: string }>(`/api/v1/users/${encodeURIComponent(id)}`, { method: "DELETE" });
export const resetUserPassword = (id: string, new_password: string) =>
  req<{ status: string }>(`/api/v1/users/${encodeURIComponent(id)}/reset-password`, {
    method: "POST",
    body: JSON.stringify({ new_password }),
  });

// Collection analytics (#160): announce health, duplicate titles, stalled collection and
// occupancy per Node. A single route because the panels overlap — see the handler.
export type AnalyticsTorrent = {
  id: string;
  hash: string;
  name: string;
  node_id: string;
  hostname: string;
  display_name?: string; // Node alias (#145)
  size_bytes: number;
  num_seeds: number;
  uploaded_total: number;
  downloaded_total: number;
  ratio: number;
  age_days: number;
  tracker_status?: number;
  tracker_message?: string;
  resolution?: string;
  // Flags that classify the torrent. They exist so every counter on the screen has
  // a corresponding list (#169) — without them, the number was a dead end.
  never_uploaded?: boolean;
  dead_swarm?: boolean;
  tracker_dead?: boolean;
  duplicate_extra?: boolean;
  duplicate_of?: string;
};

export type DuplicateSet = {
  title: string;
  year?: number;
  se?: string;
  // part separates divided works ("Phase 1" x "Phase 2"), which the release parser collapses
  // into the same title when applying the tracker's alternative title rule (#172).
  part?: string;
  identical: boolean;
  copies: AnalyticsTorrent[];
  reclaim_bytes: number;
};

export type AnalyticsReport = {
  total: number;
  total_bytes: number;
  tracker_unknown: number;
  duplicates: DuplicateSet[] | null;
  idle: {
    never_uploaded: number;
    never_uploaded_bytes: number;
    dead_swarm: number;
    dead_swarm_bytes: number;
  };
  nodes:
    | {
        node_id: string;
        hostname: string;
        display_name?: string; // Node alias (#145)
        torrents: number;
        bytes: number;
        idle_bytes: number;
        reclaim_bytes: number;
      }[]
    | null;
  // All flagged torrents, WITHOUT truncating — this is what the screen filters by card.
  flagged: AnalyticsTorrent[] | null;
};

export const getAnalytics = () => req<AnalyticsReport>("/api/v1/analytics");

// getPublishedIndex reads the index.html the server publishes now, so the Portal can tell whether the
// bundle it runs is still the published one (#39). cache: "no-store" on top of nginx's no-cache, so no
// intermediate cache hands back the index from before the deploy. null on any failure: a network error
// must not be read as "new version".
export async function getPublishedIndex(): Promise<string | null> {
  try {
    const res = await fetch("/", { cache: "no-store", headers: { Accept: "text/html" } });
    return res.ok ? await res.text() : null;
  } catch {
    return null;
  }
}
