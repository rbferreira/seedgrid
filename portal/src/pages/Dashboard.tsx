import { useEffect, useState, useCallback } from "react";
import {
  getNodesAndMode,
  setClusterMode,
  setNodeMode,
  getTorrents,
  getAlerts,
  pauseTorrent,
  resumeTorrent,
  recheckTorrent,
  getAgentAvailable,
  updateAgent,
  usingDefaultCredentials,
  ApiError,
  type Node,
  type Torrent,
  type Alert,
  type ClusterMode,
  type NodeMode,
  type AgentRelease,
} from "../api";
import { Badge, Panel, fmtTime, fmtBytes, fmtRatio, shortId, byTorrentStatus, TRANSFER } from "../components/ui";
import { startAction, Spinner, toast } from "../actions";
import { href } from "../router";
import {
  ActionBtn,
  ProgressBar,
  SeedsPeers,
  SpeedPair,
  TorrentLink,
  isDone,
  useTorrentActions,
} from "../components/torrents";
import { MigratePicker, useMigrateTarget } from "../components/MigratePicker";
import { ratioOf, topBy, TOP_STEPS } from "../lib/ranking";
import { hasNickname, nodeLabel, nodeLabelById, nodeLabelFull } from "../lib/node-label";
import { ClusterThroughput } from "../components/ClusterThroughput";
import { getLang, rich, useT, type Key } from "../i18n";

const REFRESH_MS = 10_000;
// Window for the Agent to restart on the new binary and resume reporting the version in the heartbeat
// (ADR-027). Intentionally generous: heartbeat is every ~5s, but systemd might take a while to
// restart on a busy Node. Timing out here turns into "timed out" in the toast — which is the
// right outcome: it means the Agent did NOT come back with the new version.
const AGENT_RESTART_TIMEOUT_MS = 120_000;

function Kpi({ labelKey, value, accent }: { labelKey: Key; value: string | number; accent?: string }) {
  const t = useT();
  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-3 sm:p-5">
      <p className="text-[11px] uppercase tracking-wide text-slate-400 sm:text-xs">{t(labelKey)}</p>
      <p className={`mt-1 text-2xl font-semibold sm:mt-2 sm:text-3xl ${accent ?? "text-white"}`}>{value}</p>
    </div>
  );
}

// AgentCell shows the Node's Agent version and, when a newer one is published, the update
// button (ADR-027 — manual rollout: nothing moves unless the operator clicks).
//
// Badge only shows when you can ACT: `available` null (agent.artifacts_url not
// configured) or Node architecture has no published binary ⇒ just the version, no promise
// the button wouldn't fulfill.
//
// `canUpdate` (nodes:write) decides if the badge is a BUTTON. Read-only still sees there is
// a new version — the data — but in an inert badge: clicking would get a 403.
export function AgentCell({
  node,
  available,
  busy,
  canUpdate,
  onUpdate,
}: {
  node: Node;
  available: AgentRelease | null;
  busy: boolean;
  canUpdate: boolean;
  onUpdate: () => void;
}) {
  const t = useT();
  const current = node.agent_version ?? "";
  const hasBinary = !!node.architecture && !!available?.archs.includes(node.architecture);
  const outdated = !!available && !!current && current !== available.version && hasBinary;

  if (!outdated) {
    return <span>{current || "—"}</span>;
  }
  const badge = "rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-400 ring-1 ring-amber-500/30";
  return (
    <div className="flex items-center gap-2">
      <span className="font-mono text-xs">{current}</span>
      {canUpdate ? (
        <button
          onClick={onUpdate}
          disabled={busy}
          title={t("dashboard.dashboard.agent.updateTitle", { version: available!.version, arch: node.architecture ?? "—" })}
          className={`${badge} transition hover:bg-amber-500/25 disabled:opacity-50`}
        >
          {busy ? t("dashboard.dashboard.agent.updating") : `↑ ${available!.version}`}
        </button>
      ) : (
        <span title={t("dashboard.dashboard.agent.versionAvailable", { version: available!.version })} className={badge}>
          ↑ {available!.version}
        </span>
      )}
    </div>
  );
}

// RankList: a compact list of torrents sorted by a volume metric.
/**
 * RankList: a "Top N" of the library.
 *
 * The N is the operator's, not ours: with nearly a thousand torrents, ten lines don't answer "where
 * do I get space" or "what is yielding". The metric might return `null` — ratio of those who
 * never downloaded isn't zero, it's absent (see `lib/ranking.ts`).
 */
function RankList({
  titleKey,
  torrents,
  metric,
  nodeName,
  accent,
  format = fmtBytes,
}: {
  titleKey: Key;
  torrents: Torrent[];
  metric: (t: Torrent) => number | null;
  nodeName: (id: string) => string;
  accent: string;
  format?: (v: number) => string;
}) {
  const t = useT();
  const [limit, setLimit] = useState<number>(TOP_STEPS[0]);
  const top = topBy(torrents, metric, limit);
  const title = t(titleKey);
  return (
    <Panel
      title={title}
      right={
        <select
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
          title={t("dashboard.dashboard.rank.howMany")}
          aria-label={t("dashboard.dashboard.rank.itemsIn", { title })}
          className="rounded-lg border border-slate-700 bg-slate-950 px-2 py-1 text-xs text-slate-300 outline-none focus:border-emerald-500"
        >
          {TOP_STEPS.map((n) => (
            <option key={n} value={n}>
              Top {n}
            </option>
          ))}
        </select>
      }
    >
      <ol className="divide-y divide-slate-800/60">
        {top.map((tItem, i) => (
          <li key={tItem.id} className="flex items-center justify-between gap-3 px-4 py-2.5">
            <div className="flex min-w-0 items-center gap-3">
              <span className="w-5 shrink-0 text-right text-xs font-semibold text-slate-500">
                {i + 1}
              </span>
              {/* The name leads to the torrent detail, like in the table — before it was plain
                  text, so the ranking showed the upload champion with no path to it
                  (and no tooltip, a truncated name became unreadable). */}
              <div className="min-w-0 text-sm">
                <TorrentLink t={tItem} />
                <p className="text-xs text-slate-500">{nodeName(tItem.node_id)}</p>
              </div>
            </div>
            <span className={`shrink-0 font-mono text-sm ${accent}`}>{format(metric(tItem) as number)}</span>
          </li>
        ))}
        {top.length === 0 && (
          <li className="px-3 py-6 text-center text-slate-500">{t("dashboard.dashboard.rank.noData")}</li>
        )}
      </ol>
    </Panel>
  );
}

// `can` gates each button by the SAME permission the backend requires on the route it calls
// (button gate — docs/11 - Portal.md §13.1). The Dashboard is the FALLBACK page for those who
// have no permissions at all, so what's on it is exposed to all roles: the data
// (KPIs, operation mode, queue) remains visible, only the button disappears.
export function Dashboard({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [torrents, setTorrents] = useState<Torrent[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState<string>("");
  // Global operation gate (ADR-024). modeBusy disables the toggles during the switch.
  const [gate, setGate] = useState<ClusterMode>("full_auto");
  const [agentRel, setAgentRel] = useState<AgentRelease | null>(null);
  const [modeBusy, setModeBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      // agentRel comes null when orchestrated update is not configured
      // (empty agent.artifacts_url, ADR-027) — the Agent column only shows the version, no badge.
      const [n, tData, a, rel] = await Promise.all([
        getNodesAndMode(),
        getTorrents(),
        getAlerts(false),
        getAgentAvailable(),
      ]);
      setNodes(n.data);
      setGate(n.cluster_operation_mode);
      setTorrents(tData);
      setAlerts(a);
      setAgentRel(rel);
      setError("");
      setUpdatedAt(new Date().toLocaleTimeString(getLang()));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("dashboard.dashboard.loadError"));
    }
  }, [onLogout, t]);

  // Switch the global gate (Full auto/Flex) and the per-Node mode (SG/Manual) — ADR-024.
  const changeGate = async (mode: ClusterMode) => {
    setModeBusy(true);
    try {
      await setClusterMode(mode);
      await load();
    } catch (err) {
      toast(err instanceof ApiError ? err.message : t("dashboard.dashboard.changeClusterModeError"), "error");
    } finally {
      setModeBusy(false);
    }
  };
  const changeNodeMode = async (id: string, mode: NodeMode) => {
    setModeBusy(true);
    try {
      await setNodeMode(id, mode);
      await load();
    } catch (err) {
      toast(err instanceof ApiError ? err.message : t("dashboard.dashboard.changeNodeModeError"), "error");
    } finally {
      setModeBusy(false);
    }
  };

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const nodeName = (id: string) => nodeLabelById(nodes, id) || shortId(id);
  const canMode = can("nodes:operation-mode");
  const canNode = can("nodes:write");
  const canWrite = can("torrents:write");
  const canDownload = can("torrents:download");
  const canMigrate = can("torrents:migrate");
  const canDelete = can("torrents:delete");
  // With NO torrent permission the Actions column would be empty: it vanishes entirely (data stays).
  const hasActions = canWrite || canDownload || canMigrate || canDelete;

  const { running, act, onRemove, onTogglePriority, onDownload } = useTorrentActions({
    reload: load,
    onLogout,
  });
  const { target, setTarget, confirm: confirmMigrate } = useMigrateTarget({ nodes, reload: load });

  // Agent update (ADR-027). Confirms why the Agent restarts: the Node's seeding
  // goes unsupervised for a few seconds (the Engine keeps running; nothing is lost).
  const onUpdateAgent = (n: Node) => {
    if (
      !window.confirm(
        t("dashboard.dashboard.agent.confirmUpdate", {
          node: nodeLabelFull(n),
          version: agentRel?.version ?? "",
        }),
      )
    ) {
      return;
    }
    const targetVersion = agentRel?.version;
    startAction({
      id: n.id,
      label: t("dashboard.dashboard.agent.actionLabel", { node: nodeLabel(n), version: targetVersion ?? "" }),
      run: () => updateAgent(n.id),
      // The command returns DONE when the binary is SWAPPED — the process will still restart, and
      // until the next heartbeat the Node reports the OLD version. Completing there made the toast
      // say "Done" with the "↑ vX" button still on screen, and the operator would click again.
      // The 2nd phase waits for the Node to report the target version: then it's truly finished.
      confirm: {
        timeoutMs: AGENT_RESTART_TIMEOUT_MS,
        probe: async () => {
          const currentVersion = (await getNodesAndMode()).data.find((x) => x.id === n.id)?.agent_version ?? "";
          if (targetVersion && currentVersion === targetVersion) {
            return { statusText: t("dashboard.dashboard.agent.updatedStatus", { version: targetVersion }), terminal: true, failed: false };
          }
          return { statusText: t("dashboard.dashboard.agent.restarting"), terminal: false, failed: false };
        },
      },
      onSettled: load,
    });
  };
  const onlineNodes = nodes.filter((n) => n.status === "ONLINE").length;
  const seeding = torrents.filter((tItem) => tItem.status === "SEEDING").length;
  const downloading = torrents.filter((tItem) => tItem.status === "DOWNLOADING").length;
  const checking = torrents.filter((tItem) => tItem.status === "CHECKING").length;
  const errored = torrents.filter((tItem) => tItem.status === "ERROR").length;
  const openAlerts = alerts.filter((a) => a.status === "OPEN");
  // Cluster Ratio (#14): aggregates the cumulative totals of all torrents (stable
  // across Engine session resets). It's the LOCAL ratio; the official one is from the tracker site.
  const clusterUp = torrents.reduce((s, tItem) => s + (tItem.uploaded_total ?? 0), 0);
  const clusterDown = torrents.reduce((s, tItem) => s + (tItem.downloaded_total ?? 0), 0);
  // Aggregate speed of the entire SG right now (snapshot, all Nodes).
  const clusterDlNow = torrents.reduce((s, tItem) => s + (tItem.dl_speed ?? 0), 0);
  const clusterUpNow = torrents.reduce((s, tItem) => s + (tItem.up_speed ?? 0), 0);
  // Aggregate disk of the SG (sum of Nodes). Note: Nodes in containers on the same host may
  // reflect the same physical disk — the sum is the logical view per Node.
  const clusterDiskTotal = nodes.reduce((s, n) => s + (n.disk_total_bytes ?? 0), 0);
  const clusterDiskFree = nodes.reduce((s, n) => s + (n.disk_free_bytes ?? 0), 0);
  const clusterDiskUsed = clusterDiskTotal - clusterDiskFree;
  const clusterDiskPct = clusterDiskTotal > 0 ? Math.round((clusterDiskUsed / clusterDiskTotal) * 100) : 0;
  // THE QUEUE: everything not yet in the library — downloading, waiting, paused, in
  // recheck, with error. The library (seeding) lives in the Downloaded screen. It's the COMPLEMENT of
  // `isDone`, not a list of allowed statuses: this way a new Agent status shows up
  // here instead of disappearing from both screens. Grouped by urgency (error/checking
  // first) — see byTorrentStatus.
  //
  // Note: KPIs, aggregate cards, and rankings still operate on the ENTIRE `torrents` — they are
  // a cluster view, not a queue view.
  const queue = torrents.filter((tItem) => !isDone(tItem)).sort(byTorrentStatus);

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold text-white">{t("dashboard.dashboard.title")}</h1>
        <div className="flex flex-wrap items-center gap-3">
          {/* Global admission gate (ADR-024): Full auto = SG controls everything; Flex
              unlocks the per-Node mode ("Mode" column below). */}
          <div className="flex items-center gap-2" title={t("dashboard.dashboard.admission.title")}>
            <span className="text-xs uppercase tracking-wide text-slate-500">{t("dashboard.dashboard.admission.label")}</span>
            {canMode ? (
              <div className="inline-flex overflow-hidden rounded-lg border border-slate-700 text-xs">
                {(["full_auto", "flex"] as ClusterMode[]).map((m) => (
                  <button
                    key={m}
                    onClick={() => changeGate(m)}
                    disabled={modeBusy || gate === m}
                    className={`px-3 py-1.5 font-medium transition disabled:cursor-default ${
                      gate === m
                        ? "bg-blue-600 text-white"
                        : "bg-slate-900 text-slate-400 hover:text-slate-200"
                    }`}
                  >
                    {m === "full_auto" ? "Full auto" : "Flex"}
                  </button>
                ))}
              </div>
            ) : (
              // Without nodes:operation-mode the operator still needs to KNOW the mode — just doesn't change it.
              <span className="rounded-lg border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-300">
                {gate === "flex" ? "Flex" : "Full auto"}
              </span>
            )}
          </div>
          <span className="text-xs text-slate-500">
            {updatedAt ? t("dashboard.dashboard.updatedAt", { time: updatedAt }) : t("common.loading")}
          </span>
        </div>
      </div>
      {usingDefaultCredentials() && (
        <div className="flex items-start gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-300">
          <span aria-hidden className="mt-0.5 text-base">⚠️</span>
          <div>
            <p className="font-medium text-amber-200">{t("dashboard.dashboard.defaultCreds.title")}</p>
            <p className="text-amber-300/90">
              {t("dashboard.dashboard.defaultCreds.body")}
            </p>
          </div>
        </div>
      )}
      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}


      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4 xl:grid-cols-8">
        <Kpi labelKey="dashboard.dashboard.kpi.nodesOnline" value={`${onlineNodes}/${nodes.length}`} accent="text-emerald-400" />
        <Kpi labelKey="dashboard.dashboard.kpi.torrents" value={torrents.length} />
        <Kpi labelKey="dashboard.dashboard.kpi.seeding" value={seeding} accent="text-emerald-400" />
        <Kpi labelKey="dashboard.dashboard.kpi.downloading" value={downloading} accent={downloading ? TRANSFER.down.text : "text-white"} />
        <Kpi labelKey="dashboard.dashboard.kpi.recheck" value={checking} accent={checking ? "text-amber-400" : "text-white"} />
        <Kpi labelKey="dashboard.dashboard.kpi.errors" value={errored} accent={errored ? "text-red-400" : "text-white"} />
        <Kpi
          labelKey="dashboard.dashboard.kpi.openAlerts"
          value={openAlerts.length}
          accent={openAlerts.length ? "text-red-400" : "text-white"}
        />
        <Kpi labelKey="dashboard.dashboard.kpi.ratioLocal" value={fmtRatio(clusterUp, clusterDown)} accent="text-amber-300" />
      </div>

      {/* Aggregate transfer of the entire SeedGrid (all Nodes). Cumulative totals
          (#14): stable across Engine session resets. */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div className="rounded-2xl border border-slate-700/60 bg-gradient-to-br from-slate-800/40 to-slate-900/60 p-5">
          <div className="flex items-center justify-between">
            <p className="text-xs uppercase tracking-wide text-slate-400">{t("dashboard.dashboard.transfer.totalDownloaded")}</p>
            <span aria-hidden className={`text-lg ${TRANSFER.down.text}`}>↓</span>
          </div>
          <p className={`mt-1 text-4xl font-semibold ${TRANSFER.down.strong}`}>{fmtBytes(clusterDown)}</p>
          <p className="mt-1 text-xs text-slate-500">
            {rich(
              t("dashboard.dashboard.transfer.nowLine", {
                torrents: torrents.length,
                nodes: t("dashboard.dashboard.nodeCount", { count: nodes.length }),
              }),
              { speed: <span className={TRANSFER.down.text}>{fmtBytes(clusterDlNow)}/s</span> },
            )}
          </p>
        </div>
        <div className="rounded-2xl border border-slate-700/60 bg-gradient-to-br from-slate-800/40 to-slate-900/60 p-5">
          <div className="flex items-center justify-between">
            <p className="text-xs uppercase tracking-wide text-slate-400">{t("dashboard.dashboard.transfer.totalUploaded")}</p>
            <span aria-hidden className={`text-lg ${TRANSFER.up.text}`}>↑</span>
          </div>
          <p className={`mt-1 text-4xl font-semibold ${TRANSFER.up.strong}`}>{fmtBytes(clusterUp)}</p>
          <p className="mt-1 text-xs text-slate-500">
            {rich(t("dashboard.dashboard.transfer.uploadedNowLine"), {
              speed: <span className={TRANSFER.up.text}>{fmtBytes(clusterUpNow)}/s</span>,
              ratio: <span className="text-amber-300">{fmtRatio(clusterUp, clusterDown)}</span>,
            })}
          </p>
        </div>
        <div className="rounded-2xl border border-slate-700/60 bg-gradient-to-br from-slate-800/40 to-slate-900/60 p-5">
          <div className="flex items-center justify-between">
            <p className="text-xs uppercase tracking-wide text-slate-400">{t("dashboard.dashboard.disk.used")}</p>
            <span aria-hidden className="text-lg text-slate-400">💾</span>
          </div>
          <p className="mt-1 text-4xl font-semibold text-white">{fmtBytes(clusterDiskUsed)}</p>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-800">
            <div
              className={`h-full rounded-full ${
                clusterDiskPct >= 90 ? "bg-red-500" : clusterDiskPct >= 75 ? "bg-amber-500" : "bg-emerald-500"
              }`}
              style={{ width: `${clusterDiskPct}%` }}
            />
          </div>
          <p className="mt-1 text-xs text-slate-500">
            {t("dashboard.dashboard.disk.percentOfTotal", { pct: clusterDiskPct, total: fmtBytes(clusterDiskTotal) })}
          </p>
        </div>
      </div>

      {/* Cumulative cluster throughput (F2). Came from the Health screen: the reading of "how
          the cluster has been behaving" is a general overview question, and here it made room with
          the library moving to the Downloaded screen. */}
      <ClusterThroughput refreshMs={REFRESH_MS} />

      <MigratePicker target={target} nodes={nodes} onPick={confirmMigrate} onClose={() => setTarget(null)} />

      <Panel title={t("dashboard.dashboard.seedNodes.title")}>
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase text-slate-500">
            <tr>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colNode")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colStatus")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colMode")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colHealth")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colEngine")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colEndpoint")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colAgent")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.seedNodes.colLastHeartbeat")}</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => (
              <tr key={n.id} className="border-t border-slate-800/60">
                <td className="px-3 py-2.5 font-medium text-white" title={hasNickname(n) ? n.hostname : undefined}>
                  {nodeLabel(n)}
                </td>
                <td className="px-3 py-2.5">
                  <Badge label={n.status} />
                </td>
                <td className="px-3 py-2.5">
                  {gate === "flex" && canMode ? (
                    <div className="inline-flex overflow-hidden rounded-md border border-slate-700 text-xs">
                      {(["sg", "manual"] as NodeMode[]).map((m) => {
                        const active = (n.operation_mode ?? "sg") === m;
                        return (
                          <button
                            key={m}
                            onClick={() => changeNodeMode(n.id, m)}
                            disabled={modeBusy || active}
                            className={`px-2 py-1 font-medium transition disabled:cursor-default ${
                              active
                                ? m === "manual"
                                  ? "bg-amber-600 text-white"
                                  : "bg-blue-600 text-white"
                                : "bg-slate-900 text-slate-400 hover:text-slate-200"
                            }`}
                          >
                            {m === "sg" ? "SG" : "Manual"}
                          </button>
                        );
                      })}
                    </div>
                  ) : gate === "flex" ? (
                    // Flex without nodes:operation-mode: each Node's mode remains in view, without the toggle.
                    <span
                      className={`rounded-md px-2 py-1 text-xs ${
                        n.operation_mode === "manual" ? "bg-amber-600/20 text-amber-300" : "bg-slate-800 text-slate-400"
                      }`}
                      title={t("dashboard.dashboard.seedNodes.modeReadOnly")}
                    >
                      {n.operation_mode === "manual" ? "Manual" : "SG"}
                    </span>
                  ) : (
                    <span
                      className="rounded-md bg-slate-800 px-2 py-1 text-xs text-slate-400"
                      title={t("dashboard.dashboard.seedNodes.fullAutoHint")}
                    >
                      SG
                    </span>
                  )}
                </td>
                <td className="px-3 py-2.5 text-slate-300">{n.health_score ?? "—"}</td>
                <td className="px-3 py-2.5 text-slate-300">{n.engine ?? "—"}</td>
                <td className="px-3 py-2.5 font-mono text-xs text-slate-300">
                  {n.last_ip && n.bt_port ? `${n.last_ip}:${n.bt_port}` : "—"}
                </td>
                <td className="px-3 py-2.5 text-slate-300">
                  <AgentCell
                    node={n}
                    available={agentRel}
                    busy={running.has(n.id)}
                    canUpdate={canNode}
                    onUpdate={() => onUpdateAgent(n)}
                  />
                </td>
                <td className="px-3 py-2.5 text-slate-400">{fmtTime(n.last_seen_at)}</td>
              </tr>
            ))}
            {nodes.length === 0 && (
              <tr>
                <td colSpan={8} className="px-3 py-6 text-center text-slate-500">
                  {t("dashboard.dashboard.seedNodes.empty")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Panel>

      <Panel title={t("dashboard.dashboard.queue.title", { count: queue.length })}>
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase text-slate-500">
            <tr>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.queue.colName")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.queue.colStatus")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.queue.colProgress")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.queue.colSpeed")}</th>
              <th className="px-3 py-2 text-center font-medium">{t("dashboard.dashboard.queue.colSeedsPeers")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.queue.colRatio")}</th>
              <th className="px-3 py-2 font-medium">{t("dashboard.dashboard.queue.colNode")}</th>
              {hasActions && <th className="px-3 py-2 font-medium text-right">{t("dashboard.dashboard.queue.colActions")}</th>}
            </tr>
          </thead>
          <tbody>
            {queue.map((tItem) => (
              <tr key={tItem.id} className="border-t border-slate-800/60">
                <td className="px-3 py-2.5 font-medium text-white">
                  <TorrentLink t={tItem} />
                </td>
                <td className="px-3 py-2.5">
                  <Badge label={tItem.status} />
                </td>
                <td className="px-3 py-2.5">
                  <ProgressBar value={tItem.progress} />
                </td>
                <td className="whitespace-nowrap px-3 py-2.5 text-xs">
                  <SpeedPair dl={tItem.dl_speed} up={tItem.up_speed} />
                </td>
                <td className="whitespace-nowrap px-3 py-2.5 text-center text-slate-300">
                  <SeedsPeers t={tItem} />
                </td>
                <td className="px-3 py-2.5 text-slate-300">{fmtRatio(tItem.uploaded_total, tItem.downloaded_total)}</td>
                <td className="px-3 py-2.5 text-slate-300">{nodeName(tItem.node_id)}</td>
                {hasActions && (
                  <td className="px-3 py-2.5">
                    <div className="flex items-center justify-end gap-1">
                      {running.has(tItem.id) ? (
                        <Spinner className="mr-1" />
                      ) : (
                        <>
                          {/* ⚡ only makes sense for incomplete torrents — it's skipping the admission
                              queue. The condition lost the SEEDING check: here no
                              torrent is seeding, and `progress < 1` is already the criterion. */}
                          {canWrite && (tItem.progress ?? 0) < 1 && (
                            <ActionBtn
                              tone={tItem.prioritized_at ? "gold" : "slate"}
                              title={tItem.prioritized_at ? t("dashboard.dashboard.actions.removePriority") : t("dashboard.dashboard.actions.prioritize")}
                              onClick={() => onTogglePriority(tItem)}
                              disabled={running.size > 0}
                            >
                              ⚡
                            </ActionBtn>
                          )}
                          {/* Complete but not seeding (CHECKING, MOVING, paused at the end of the
                              download): the content is on disk, so downloading already works. */}
                          {canDownload && (tItem.progress ?? 0) >= 1 && (
                            <ActionBtn
                              tone="blue"
                              title={t("dashboard.dashboard.actions.downloadContent")}
                              onClick={() => onDownload(tItem)}
                              disabled={running.size > 0}
                            >
                              ⬇
                            </ActionBtn>
                          )}
                          {canWrite &&
                            (tItem.status === "PAUSED" ? (
                              <ActionBtn
                                tone="emerald"
                                title={t("dashboard.dashboard.actions.resume")}
                                onClick={() => act(tItem, t("dashboard.dashboard.actions.resume"), resumeTorrent)}
                                disabled={running.size > 0}
                              >
                                ▶
                              </ActionBtn>
                            ) : (
                              <ActionBtn
                                tone="amber"
                                title={t("dashboard.dashboard.actions.pause")}
                                onClick={() => act(tItem, t("dashboard.dashboard.actions.pause"), pauseTorrent)}
                                disabled={running.size > 0}
                              >
                                ⏸
                              </ActionBtn>
                            ))}
                          {/* ↻ is mandatory here: torrents in ERROR live in this table and
                              recheck is what recovers them (e.g., after the disk comes back).
                              Before it only existed in the Nodes screen, which ceases to exist. */}
                          {canWrite && (
                            <ActionBtn
                              tone={tItem.status === "ERROR" ? "red" : "neutral"}
                              title={t("dashboard.dashboard.actions.recheck")}
                              onClick={() => act(tItem, t("dashboard.dashboard.actions.recheck"), recheckTorrent)}
                              disabled={running.size > 0}
                            >
                              ↻
                            </ActionBtn>
                          )}
                          {/* ⇄ also used to come only from the Nodes screen. Without it, a download on a
                              Node with a full disk would have no way to get out of there. */}
                          {canMigrate && (
                            <ActionBtn
                              tone="blue"
                              title={t("dashboard.dashboard.actions.migrate")}
                              onClick={() =>
                                setTarget({
                                  kind: "torrent",
                                  id: tItem.id,
                                  name: tItem.name ?? shortId(tItem.hash),
                                  sourceNodeId: tItem.node_id,
                                  label: t("dashboard.dashboard.actions.migrateLabel", { name: tItem.name ?? shortId(tItem.hash) }),
                                })
                              }
                              disabled={running.size > 0}
                            >
                              ⇄
                            </ActionBtn>
                          )}
                          {canDelete && (
                            <ActionBtn
                              tone="red"
                              title={t("dashboard.dashboard.actions.delete")}
                              onClick={() => onRemove(tItem)}
                              disabled={running.size > 0}
                            >
                              🗑
                            </ActionBtn>
                          )}
                        </>
                      )}
                    </div>
                  </td>
                )}
              </tr>
            ))}
            {/* Tests the QUEUE, not `torrents`: with everything seeding the list is empty and without
                this empty state the table would render zero lines without explanation. */}
            {queue.length === 0 && (
              <tr>
                <td colSpan={hasActions ? 8 : 7} className="px-3 py-6 text-center text-slate-500">
                  {torrents.length === 0 ? (
                    t("dashboard.dashboard.queue.emptyNoTorrents")
                  ) : (
                    <>
                      {t("dashboard.dashboard.queue.emptyAllSeeding")}{" "}
                      <a href={href("/downloaded")} className="text-emerald-400 underline hover:text-emerald-300">
                        {t("dashboard.dashboard.queue.goToDownloaded")}
                      </a>
                    </>
                  )}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Panel>

      <div className="grid gap-6 lg:grid-cols-2 xl:grid-cols-3">
        <RankList
          titleKey="dashboard.dashboard.rank.topUploaded"
          torrents={torrents}
          metric={(tItem) => tItem.uploaded_total ?? 0}
          nodeName={nodeName}
          accent={TRANSFER.up.text}
        />
        <RankList
          titleKey="dashboard.dashboard.rank.topDownloaded"
          torrents={torrents}
          metric={(tItem) => tItem.downloaded_total ?? 0}
          nodeName={nodeName}
          accent={TRANSFER.down.text}
        />
        {/* Ratio is not a transfer axis (#130): it's a derived measure, amber like in the
            Analytics Ratio column. Came from Analytics, where it was a loose list far
            from the other rankings and ranked over an already truncated list (#174). */}
        <RankList
          titleKey="dashboard.dashboard.rank.topRatio"
          torrents={torrents}
          metric={ratioOf}
          nodeName={nodeName}
          accent="text-amber-400"
          format={(v) => v.toFixed(2)}
        />
      </div>
    </>
  );
}
