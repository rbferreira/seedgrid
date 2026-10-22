import { useEffect, useState, useCallback, type ReactNode } from "react";
import {
  getNodes,
  getTorrents,
  pauseTorrent,
  resumeTorrent,
  recheckTorrent,
  restartEngine,
  ApiError,
  type Node,
  type Torrent,
} from "../api";
import {
  Badge,
  BackLink,
  Meter,
  Panel,
  fmtTime,
  fmtBytes,
  fmtRatio,
  fmtUptime,
  fmtSince,
  shortId,
  usedPct,
  byTorrentStatus,
  TRANSFER,
} from "../components/ui";
import { startAction, Spinner } from "../actions";
import { ActionBtn, SeedsPeers, SpeedPair, TorrentLink, useTorrentActions } from "../components/torrents";
import { RenameNodeModal } from "../components/RenameNodeModal";
import { NodeConfigPanel } from "../components/NodeConfigPanel";
import { hasNickname, nodeLabel, nodeLabelFull } from "../lib/node-label";
import { useT } from "../i18n";
import { askConfirm } from "../components/ConfirmDialog";
import { restartEnginePrompt } from "../lib/confirm-prompts";

const REFRESH_MS = 10_000;

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4 border-b border-slate-800/60 py-1.5 text-sm last:border-0">
      <dt className="text-slate-400">{label}</dt>
      <dd className="text-right text-slate-200">{value}</dd>
    </div>
  );
}

// `can` gates each button by the SAME permission that the backend requires on the route it calls
// (button gate — docs/11 - Portal.md §13.1). This page is reachable by any role,
// and the data (resources, inventory, Node torrents) remains visible to everyone: only the button disappears.
export function NodeDetail({
  nodeId,
  can,
  onLogout,
}: {
  nodeId: string;
  can: (perm: string) => boolean;
  onLogout: () => void;
}) {
  const t = useT();
  const [node, setNode] = useState<Node | null>(null);
  const [renaming, setRenaming] = useState(false);
  const [torrents, setTorrents] = useState<Torrent[]>([]);
  const [error, setError] = useState("");
  const [notFound, setNotFound] = useState(false);

  const load = useCallback(async () => {
    try {
      const [nodes, ts] = await Promise.all([getNodes(), getTorrents()]);
      const n = nodes.find((x) => x.id === nodeId) ?? null;
      setNode(n);
      setNotFound(!n);
      setTorrents(ts.filter((t) => t.node_id === nodeId));
      setError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("nodes.nodeDetail.loadError"));
    }
  }, [nodeId, onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const { running, act, onRemove } = useTorrentActions({ reload: load, onLogout });

  const onRestartEngine = async () => {
    if (!node) return;
    if (!(await askConfirm(restartEnginePrompt(t, nodeLabelFull(node))))) return;
    startAction({
      id: node.id,
      label: t("nodes.nodeDetail.actionRestartEngine", { name: nodeLabel(node) }),
      run: () => restartEngine(node.id),
      onSettled: load,
    });
  };

  // Fallback to Health, which is where the Nodes listing now lives.
  const back = <BackLink fallback="/health" />;

  if (notFound) {
    return (
      <>
        {back}
        <p className="rounded-2xl border border-slate-800 bg-slate-900/60 px-5 py-8 text-center text-slate-500">
          {t("nodes.nodeDetail.notFound")}
        </p>
      </>
    );
  }

  const canOperate = can("nodes:write");
  const canWrite = can("torrents:write");
  const canDelete = can("torrents:delete");
  // With no torrent permissions the Actions column would be empty: it vanishes entirely (the data stays).
  const hasActions = canWrite || canDelete;

  const seeding = torrents.filter((t) => t.status === "SEEDING").length;
  const downloading = torrents.filter((t) => t.status === "DOWNLOADING").length;
  // List grouped by status (error/checking first) — see byTorrentStatus.
  const sortedTorrents = [...torrents].sort(byTorrentStatus);
  // Cumulative totals (#14): stable across Engine session resets → reliable ratio.
  const totalUp = torrents.reduce((s, t) => s + (t.uploaded_total ?? 0), 0);
  const totalDown = torrents.reduce((s, t) => s + (t.downloaded_total ?? 0), 0);
  const dlNow = torrents.reduce((s, t) => s + (t.dl_speed ?? 0), 0);
  const upNow = torrents.reduce((s, t) => s + (t.up_speed ?? 0), 0);

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-3">
          {back}
          <div>
            <h1 className="text-lg font-semibold text-white">{nodeLabel(node) || shortId(nodeId)}</h1>
            {/* The OS hostname is technical data: it goes right below the name the operator gave. */}
            {hasNickname(node) && (
              <p className="font-mono text-xs text-slate-500" title={t("nodes.nodeDetail.hostnameTitle")}>
                {node?.hostname}
              </p>
            )}
          </div>
          {node && <Badge label={node.status} />}
          {node?.engine_adopted && (
            <span
              title={t("nodes.nodeDetail.adoptedTitle")}
              className="rounded-full bg-sky-500/15 px-2 py-0.5 text-xs font-medium text-sky-400 ring-1 ring-inset ring-sky-500/30"
            >
              {t("nodes.nodeDetail.adopted")}
            </span>
          )}
        </div>
        <div className="flex items-center gap-3">
          {node?.health_score != null && (
            <span className="text-xs text-slate-500">{t("nodes.migratePicker.healthScore", { score: node.health_score ?? "—" })}</span>
          )}
          {node && canOperate && (
            <button
              onClick={() => setRenaming(true)}
              title={t("nodes.nodeDetail.renameTitle")}
              className="inline-flex items-center gap-1.5 rounded-full border border-slate-700 px-2.5 py-1 text-xs text-slate-300 transition hover:bg-slate-800"
            >
              ✎ {t("nodes.nodeDetail.rename")}
            </button>
          )}
          {node && canOperate && !node.engine_adopted && (
            <button
              onClick={onRestartEngine}
              disabled={running.size > 0}
              title={t("nodes.nodeDetail.restartEngineTitle")}
              className="inline-flex items-center gap-1.5 rounded-full border border-slate-700 px-2.5 py-1 text-xs text-amber-400 transition hover:bg-slate-800 disabled:opacity-40"
            >
              {running.has(node.id) ? <Spinner /> : "⟳"} {t("nodes.nodeDetail.restartEngine")}
            </button>
          )}
        </div>
      </div>

      {renaming && node && (
        <RenameNodeModal node={node} onClose={() => setRenaming(false)} onSaved={load} onLogout={onLogout} />
      )}

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}


      {/* Live metrics + aggregates */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-7">
        {([
          { l: t("nodes.nodeDetail.metrics.downloadNow"), v: `${fmtBytes(dlNow)}/s`, a: TRANSFER.down.text },
          { l: t("nodes.nodeDetail.metrics.uploadNow"), v: `${fmtBytes(upNow)}/s`, a: TRANSFER.up.text },
          { l: t("nodes.nodeDetail.metrics.torrents"), v: String(torrents.length) },
          {
            l: t("nodes.nodeDetail.metrics.seedingDownloading"),
            // Pair with TWO semantics in a single card: each half gets its own color
            // (`TRANSFER`) instead of green for both — seeding is output, downloading is
            // input, and the card was painting the download green.
            v: (
              <>
                <span className={TRANSFER.up.text}>{seeding}</span>
                {" / "}
                <span className={TRANSFER.down.text}>{downloading}</span>
              </>
            ),
          },
          { l: t("nodes.nodeDetail.metrics.totalDownloaded"), v: fmtBytes(totalDown), a: TRANSFER.down.text },
          { l: t("nodes.nodeDetail.metrics.totalUploaded"), v: fmtBytes(totalUp), a: TRANSFER.up.text },
          { l: t("nodes.nodeDetail.metrics.ratioLocal"), v: fmtRatio(totalUp, totalDown), a: "text-amber-300" },
        ] as { l: string; v: ReactNode; a?: string }[]).map((s) => (
          <div key={s.l} className="rounded-2xl border border-slate-800 bg-slate-900/60 p-4">
            <p className="text-[10px] uppercase tracking-wide text-slate-500">{s.l}</p>
            <p className={`mt-1 text-lg font-semibold ${s.a ?? "text-white"}`}>{s.v}</p>
          </div>
        ))}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Inventory */}
        <Panel title={t("nodes.nodeDetail.inventory.title")}>
          <dl className="px-5 py-3">
            <Field label={t("nodes.nodeDetail.inventory.hostname")} value={node?.hostname ?? "—"} />
            <Field label={t("nodes.nodeDetail.inventory.os")} value={[node?.os_name, node?.os_version].filter(Boolean).join(" ") || "—"} />
            <Field label={t("nodes.nodeDetail.inventory.kernel")} value={node?.kernel_version ?? "—"} />
            <Field label={t("nodes.nodeDetail.inventory.architecture")} value={node?.architecture ?? "—"} />
            <Field
              label={t("nodes.nodeDetail.inventory.engine")}
              value={
                node?.engine_reachable === false
                  ? t("nodes.nodeDetail.inventory.engineUnreachable", { engine: node?.engine ?? "—" })
                  : [node?.engine, node?.engine_version].filter(Boolean).join(" ") || "—"
              }
            />
            <Field label={t("nodes.nodeDetail.inventory.agentVersion")} value={node?.agent_version ?? "—"} />
            <Field label={t("nodes.nodeDetail.inventory.cpu")} value={node?.cpu_cores ? t("nodes.nodeDetail.inventory.cpuCores", { count: node.cpu_cores }) : "—"} />
            <Field label={t("nodes.nodeDetail.inventory.load1m")} value={node?.load1?.toFixed(2) ?? "—"} />
            <Field
              label={t("nodes.nodeDetail.inventory.btEndpoint")}
              value={node?.last_ip && node?.bt_port ? `${node.last_ip}:${node.bt_port}` : "—"}
            />
            <Field label={t("nodes.nodeDetail.inventory.hostUptime")} value={fmtUptime(node?.host_uptime_seconds)} />
            <Field label={t("nodes.nodeDetail.inventory.agentUptime")} value={fmtUptime(node?.uptime_seconds)} />
            <Field label={t("nodes.nodeDetail.inventory.engineUptime")} value={fmtSince(node?.engine_started_at)} />
            <Field label={t("nodes.nodeDetail.inventory.registeredAt")} value={fmtTime(node?.registered_at)} />
            <Field label={t("nodes.nodeDetail.inventory.lastHeartbeat")} value={fmtTime(node?.last_seen_at)} />
          </dl>
        </Panel>

        {/* Resources */}
        <Panel title={t("nodes.nodeDetail.resources.title")}>
          <div className="space-y-4 p-5">
            {node?.disk_total_bytes ? (
              <Meter
                label={t("nodes.nodeDetail.resources.disk")}
                used={usedPct(node.disk_free_bytes, node.disk_total_bytes)}
                detail={`${fmtBytes((node.disk_total_bytes ?? 0) - (node.disk_free_bytes ?? 0))} / ${fmtBytes(node.disk_total_bytes)}`}
              />
            ) : (
              <p className="text-sm text-slate-500">{t("nodes.nodeDetail.resources.noDiskData")}</p>
            )}
            {node?.mem_total_bytes ? (
              <Meter
                label={t("nodes.nodeDetail.resources.memory")}
                used={usedPct(node.mem_avail_bytes, node.mem_total_bytes)}
                detail={`${fmtBytes((node.mem_total_bytes ?? 0) - (node.mem_avail_bytes ?? 0))} / ${fmtBytes(node.mem_total_bytes)}`}
              />
            ) : (
              <p className="text-sm text-slate-500">{t("nodes.nodeDetail.resources.noMemoryData")}</p>
            )}
            <p className="text-[11px] text-slate-500">
              {t("nodes.nodeDetail.resources.firewallHint", { port: node?.bt_port ?? "—", ip: node?.last_ip ?? "IP" })}
            </p>
          </div>
        </Panel>
      </div>

      {/* Behavior configuration for this Node's Agent/Engine (ADR-039): effective value,
          where it comes from (Node or cluster) and override. The button gate is on the panel itself. */}
      {node && <NodeConfigPanel nodeId={nodeId} can={can} onLogout={onLogout} engineAdopted={node.engine_adopted === true} />}

      {/* Node Torrents */}
      <Panel title={t("nodes.nodeDetail.torrents.title", { count: torrents.length })}>
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase text-slate-500">
            <tr>
              <th className="px-5 py-2 font-medium">{t("nodes.nodeDetail.torrents.colName")}</th>
              <th className="px-5 py-2 font-medium">{t("nodes.nodeDetail.torrents.colStatus")}</th>
              <th className="px-5 py-2 font-medium">{t("nodes.nodeDetail.torrents.colProgress")}</th>
              <th className="px-5 py-2 font-medium">{t("nodes.nodeDetail.torrents.colSpeed")}</th>
              {/* Size next to Downloaded/Uploaded — makes cleanup for full disk easier (#90). */}
              <th className="px-5 py-2 text-right font-medium">{t("nodes.nodeDetail.torrents.colSize")}</th>
              <th className="px-5 py-2 text-center font-medium">{t("nodes.nodeDetail.torrents.colSeedsPeers")}</th>
              <th className="px-5 py-2 font-medium">{t("nodes.nodeDetail.torrents.colTransferred")}</th>
              <th className="px-5 py-2 font-medium">{t("nodes.nodeDetail.torrents.colRatio")}</th>
              {hasActions && <th className="px-5 py-2 font-medium text-right">{t("nodes.nodeDetail.torrents.colActions")}</th>}
            </tr>
          </thead>
          <tbody>
            {sortedTorrents.map((tItem) => (
              <tr key={tItem.id} className="border-t border-slate-800/60">
                <td className="px-5 py-2.5 font-medium text-white">
                  <TorrentLink t={tItem} clamp={false} />
                </td>
                <td className="px-5 py-2.5">
                  <Badge label={tItem.status} />
                </td>
                <td className="px-5 py-2.5 text-slate-400">{Math.round((tItem.progress ?? 0) * 100)}%</td>
                <td className="whitespace-nowrap px-5 py-2.5 text-xs">
                  <SpeedPair dl={tItem.dl_speed} up={tItem.up_speed} />
                </td>
                <td className="whitespace-nowrap px-5 py-2.5 text-right text-slate-300">
                  {fmtBytes(tItem.size_bytes)}
                </td>
                <td className="whitespace-nowrap px-5 py-2.5 text-center">
                  <SeedsPeers t={tItem} showConnected={false} />
                </td>
                <td className="px-5 py-2.5 text-xs text-slate-400">
                  <span className={TRANSFER.down.text}>{fmtBytes(tItem.downloaded_total)}</span>
                  {" / "}
                  <span className={TRANSFER.up.text}>{fmtBytes(tItem.uploaded_total)}</span>
                </td>
                <td className="px-5 py-2.5 text-slate-300">{fmtRatio(tItem.uploaded_total, tItem.downloaded_total)}</td>
                {hasActions && (
                  <td className="px-5 py-2.5">
                    <div className="flex items-center justify-end gap-1">
                      {running.has(tItem.id) ? (
                        <Spinner className="mr-1" />
                      ) : (
                        <>
                          {canWrite &&
                            (tItem.status === "PAUSED" ? (
                              <ActionBtn
                                tone="emerald"
                                title={t("nodes.nodeDetail.torrents.resumeTitle")}
                                onClick={() => act(tItem, t("nodes.nodeDetail.torrents.resumeLabel"), resumeTorrent)}
                                disabled={running.size > 0}
                              >
                                ▶
                              </ActionBtn>
                            ) : (
                              <ActionBtn
                                tone="amber"
                                title={t("nodes.nodeDetail.torrents.pauseTitle")}
                                onClick={() => act(tItem, t("nodes.nodeDetail.torrents.pauseLabel"), pauseTorrent)}
                                disabled={running.size > 0}
                              >
                                ⏸
                              </ActionBtn>
                            ))}
                          {canWrite && (
                            <ActionBtn
                              tone={tItem.status === "ERROR" ? "red" : "neutral"}
                              title={t("nodes.nodeDetail.torrents.recheckTitle")}
                              onClick={() => act(tItem, t("nodes.nodeDetail.torrents.recheckLabel"), recheckTorrent)}
                              disabled={running.size > 0}
                            >
                              ↻
                            </ActionBtn>
                          )}
                          {canDelete && (
                            <ActionBtn
                              tone="red"
                              title={t("nodes.nodeDetail.torrents.deleteTitle")}
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
            {torrents.length === 0 && (
              <tr>
                <td colSpan={hasActions ? 9 : 8} className="px-5 py-6 text-center text-slate-500">
                  {t("nodes.nodeDetail.torrents.empty")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Panel>
    </>
  );
}
