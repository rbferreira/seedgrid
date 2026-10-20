import { useEffect, useState, useCallback } from "react";
import {
  deleteNode,
  getNodes,
  getMetric,
  getSystemHealth,
  getTorrents,
  restartEngine,
  ApiError,
  type Node,
  type MetricSeries,
  type SystemHealth,
  type Torrent,
} from "../api";
import { Chart } from "../components/Chart";
import { fmtUptime } from "../components/ui";
import { startAction, toast, useRunningId } from "../actions";
import { AddNodeModal } from "../components/AddNodeModal";
import { MigratePicker, useMigrateTarget } from "../components/MigratePicker";
import { NodeCard } from "../components/NodeCard";
import { RenameNodeModal } from "../components/RenameNodeModal";
import { nodeLabel, nodeLabelFull, relabelByNode } from "../lib/node-label";
import { useT, type TFn } from "../i18n";
import { askConfirm } from "../components/ConfirmDialog";
import { restartEnginePrompt } from "../lib/confirm-prompts";

const REFRESH_MS = 15_000;
const RANGE = "3h";
const CP_INSTANCE = "seedgrid-controller";

// fmtAgo formats "how long ago" from an ISO timestamp. null = never ran.
function fmtAgo(iso: string | null | undefined, t: TFn): string {
  if (!iso) return t("health.healthStatus.workers.never");
  const secs = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (secs < 60) return t("health.healthStatus.workers.agoSeconds", { count: secs });
  if (secs < 3600) return t("health.healthStatus.workers.agoMinutes", { count: Math.floor(secs / 60) });
  return t("health.healthStatus.workers.agoHours", { count: Math.floor(secs / 3600) });
}

// WorkerCard shows the last run of an internal CP Worker. status:
// running (healthy) · stale (late, hasn't run for > 3 intervals) · starting (no 1st cycle).
function WorkerCard({ w }: { w: SystemHealth["workers"][number] }) {
  const t = useT();
  const tone =
    w.status === "running" ? "text-emerald-400" : w.status === "stale" ? "text-red-400" : "text-amber-400";
  const dot =
    w.status === "running" ? "bg-emerald-400" : w.status === "stale" ? "bg-red-400" : "bg-amber-400";
  const label = w.status === "running" ? "OK" : w.status === "stale" ? t("health.healthStatus.workers.stale") : t("health.healthStatus.workers.starting");
  return (
    <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-3">
      <div className="flex items-center justify-between">
        <span className="text-sm text-slate-300">{w.name}</span>
        <span className={`flex items-center gap-1.5 text-xs ${tone}`}>
          <span className={`inline-block h-2 w-2 rounded-full ${dot}`} />
          {label}
        </span>
      </div>
      <p className="mt-1 text-xs text-slate-500">
        {t("health.healthStatus.workers.lastRun")} <span className="text-slate-400">{fmtAgo(w.last_run, t)}</span>
        {w.interval_seconds ? ` · ${t("health.healthStatus.workers.interval", { seconds: w.interval_seconds })}` : ""}
      </p>
    </div>
  );
}

function ServiceCard({ label, ok, detail }: { label: string; ok: boolean; detail?: string }) {
  const t = useT();
  return (
    <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-3">
      <div className="flex items-center justify-between">
        <span className="text-sm text-slate-300">{label}</span>
        <span className={`flex items-center gap-1.5 text-xs ${ok ? "text-emerald-400" : "text-red-400"}`}>
          <span className={`inline-block h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-red-400"}`} />
          {ok ? "OK" : t("health.healthStatus.services.down")}
        </span>
      </div>
      {detail && <p className="mt-1 text-xs text-slate-500">{detail}</p>}
    </div>
  );
}

export function HealthStatus({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [sys, setSys] = useState<SystemHealth | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [cpu, setCpu] = useState<MetricSeries[]>([]);
  const [mem, setMem] = useState<MetricSeries[]>([]);
  const [netRx, setNetRx] = useState<MetricSeries[]>([]);
  const [netTx, setNetTx] = useState<MetricSeries[]>([]);
  const [diskRd, setDiskRd] = useState<MetricSeries[]>([]);
  const [diskWr, setDiskWr] = useState<MetricSeries[]>([]);
  const [diskUtil, setDiskUtil] = useState<MetricSeries[]>([]);
  const [torrents, setTorrents] = useState<Torrent[]>([]);
  const [error, setError] = useState("");
  // busy: Node in operation WITHOUT command_id (removal) — the rest use the action tracker.
  const [busy, setBusy] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [renaming, setRenaming] = useState<Node | null>(null);
  const running = useRunningId();

  const load = useCallback(async () => {
    try {
      // The per-call `.catch` are deliberate: this page is gated by `metrics:read`,
      // but /nodes requires `nodes:read` and /torrents requires `torrents:read`. Without them, a role
      // without these permissions would lose the ENTIRE page instead of losing a block.
      const [s, n, ts, c, m, rx, tx, drd, dwr, dutil] = await Promise.all([
        getSystemHealth().catch(() => null),
        getNodes().catch(() => [] as Node[]),
        getTorrents().catch(() => [] as Torrent[]),
        getMetric("host_cpu_pct", RANGE),
        getMetric("host_mem_used_bytes", RANGE),
        getMetric("host_net_rx_bytes", RANGE),
        getMetric("host_net_tx_bytes", RANGE),
        getMetric("host_disk_read_bytes", RANGE),
        getMetric("host_disk_write_bytes", RANGE),
        getMetric("host_disk_util_pct", RANGE),
      ]);
      setSys(s);
      setNodes(n);
      setTorrents(ts);
      setCpu(c);
      setMem(m);
      setNetRx(rx);
      setNetTx(tx);
      setDiskRd(drd);
      setDiskWr(dwr);
      setDiskUtil(dutil);
      setError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("health.healthStatus.errors.loadFailed"));
    }
  }, [onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const { target, setTarget, confirm: confirmMigrate } = useMigrateTarget({ nodes, reload: load });

  const onRestartEngine = async (n: Node) => {
    // Destructive confirmation: shows nickname AND hostname — the actual machine that will undergo the action.
    if (!(await askConfirm(restartEnginePrompt(t, nodeLabelFull(n))))) return;
    startAction({
      id: n.id,
      label: t("health.healthStatus.actions.restartLabel", { name: nodeLabel(n) }),
      run: () => restartEngine(n.id),
      onSettled: load,
    });
  };

  // onDeleteNode: removes the Node. If the CP refuses due to having torrents (409), offers to force.
  const onDeleteNode = async (n: Node) => {
    const ok = await askConfirm({
      title: t("health.healthStatus.actions.deleteConfirm.title", { name: nodeLabelFull(n) }),
      confirmLabel: t("health.healthStatus.actions.deleteConfirm.confirm"),
      tone: "danger",
    });
    if (!ok) return;
    setBusy(n.id);
    try {
      await deleteNode(n.id);
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        const force = await askConfirm({
          title: t("health.healthStatus.actions.deleteWithTorrentsConfirm.title", { name: nodeLabelFull(n) }),
          message: t("health.healthStatus.actions.deleteWithTorrentsConfirm.message"),
          confirmLabel: t("health.healthStatus.actions.deleteWithTorrentsConfirm.confirm"),
          tone: "danger",
        });
        if (force) {
          try {
            await deleteNode(n.id, true);
            await load();
          } catch (e2) {
            toast(e2 instanceof ApiError ? e2.message : t("health.healthStatus.errors.deleteFailed"), "error");
          }
        }
      } else if (err instanceof ApiError && err.status === 401) {
        onLogout();
      } else {
        toast(err instanceof ApiError ? err.message : t("health.healthStatus.errors.deleteFailed"), "error");
      }
    } finally {
      setBusy("");
    }
  };

  const cpCpu = cpu.filter((s) => s.labels.instance === CP_INSTANCE);
  const cpMem = mem.filter((s) => s.labels.instance === CP_INSTANCE);
  // Prometheus series include the hostname in `instance`; the legend shows the name the
  // operator gave to the Node (#145).
  const perNode = (s: MetricSeries[]) => relabelByNode(s.filter((x) => x.labels.instance !== CP_INSTANCE), nodes);
  const nodeCpu = perNode(cpu);
  const nodeMem = perNode(mem);
  const nodeRx = perNode(netRx);
  const nodeTx = perNode(netTx);
  const nodeDiskRd = perNode(diskRd);
  const nodeDiskWr = perNode(diskWr);
  const nodeDiskUtil = perNode(diskUtil);
  const toGB = (v: number) => Math.round((v / 1024 ** 3) * 100) / 100;
  const toMBs = (v: number) => Math.round((v / 1024 ** 2) * 100) / 100;

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold text-white">{t("health.healthStatus.title")}</h1>
        {can("enroll:write") && (
          <button
            onClick={() => setAddOpen(true)}
            className="rounded-full border border-emerald-600/40 bg-emerald-600/15 px-3 py-1.5 text-sm font-medium text-emerald-300 transition hover:bg-emerald-600/25"
          >
            {t("health.healthStatus.actions.addNode")}
          </button>
        )}
      </div>

      {addOpen && <AddNodeModal onClose={() => setAddOpen(false)} onLogout={onLogout} />}
      {renaming && (
        <RenameNodeModal node={renaming} onClose={() => setRenaming(null)} onSaved={load} onLogout={onLogout} />
      )}
      <MigratePicker target={target} nodes={nodes} onPick={confirmMigrate} onClose={() => setTarget(null)} />
      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}

      {/* ===== Control Plane ===== */}
      <section className="rounded-2xl border border-emerald-900/50 bg-emerald-950/10 p-4">
        <div className="mb-3 flex items-center gap-2">
          <span className="rounded bg-emerald-600 px-2 py-0.5 text-xs font-semibold text-white">Control Plane</span>
          {sys && (
            <span className="text-xs text-slate-500">
              {t("health.healthStatus.cpVersion", {
                // The build version may already carry the "v" (git describe): never show "vv1.2.0".
                version: sys.version.replace(/^v/, ""),
                uptime: fmtUptime(sys.uptime_seconds),
              })}
            </span>
          )}
        </div>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <ServiceCard label="API" ok={sys?.api === "ok"} />
          <ServiceCard label={t("health.healthStatus.services.database")} ok={sys?.database.status === "ok"} detail="PostgreSQL" />
          {(sys?.workers ?? []).map((w) => (
            <WorkerCard key={w.name} w={w} />
          ))}
          {!sys && <p className="text-sm text-slate-500">{t("health.healthStatus.cp.loading")}</p>}
        </div>
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <Chart title={t("health.healthStatus.charts.cpCpu")} series={cpCpu} unit="%" />
          <Chart title={t("health.healthStatus.charts.cpMem")} series={cpMem} transform={toGB} unit=" GB" />
        </div>
      </section>

      {/* ===== Seed Nodes =====
           The cards came from the /nodes screen, which no longer exists: here they merged with
           the summary that already existed on this page (disk I/O and "how much is left" of disk were
           exclusive to here). Wide stacked panel, not a grid: it's the format that accommodates the
           pills + the four controls in the header. */}
      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <span className="rounded bg-slate-700 px-2 py-0.5 text-xs font-semibold text-white">Seed Nodes</span>
          <span className="text-xs text-slate-500">{t("health.healthStatus.nodes.subtitle")}</span>
        </div>
        {nodes.map((n) => (
          <NodeCard
            key={n.id}
            node={n}
            torrents={torrents.filter((t) => t.node_id === n.id)}
            can={can}
            busy={busy === n.id}
            running={running}
            onDrain={() =>
              setTarget({
                kind: "node",
                id: n.id,
                name: nodeLabel(n),
                sourceNodeId: n.id,
                label: t("health.healthStatus.actions.migrateLabel", { name: nodeLabelFull(n) }),
              })
            }
            onRestartEngine={() => onRestartEngine(n)}
            onDelete={() => onDeleteNode(n)}
            onRename={() => setRenaming(n)}
          />
        ))}
        {nodes.length === 0 && (
          <p className="rounded-2xl border border-slate-800 bg-slate-900/60 px-5 py-8 text-center text-slate-500">
            {t("health.healthStatus.nodes.empty")}
          </p>
        )}
      </section>

      {/* ===== Trends per Node ===== */}
      <section className="rounded-2xl border border-slate-800 bg-slate-900/60 p-4">
        <div className="mb-3">
          <span className="rounded bg-slate-700 px-2 py-0.5 text-xs font-semibold text-white">
            {t("health.healthStatus.trends.title")}
          </span>
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <Chart title={t("health.healthStatus.charts.nodeCpu")} series={nodeCpu} unit="%" />
          <Chart title={t("health.healthStatus.charts.nodeMem")} series={nodeMem} transform={toGB} unit=" GB" />
          <Chart title={t("health.healthStatus.charts.nodeNetRx")} series={nodeRx} transform={toMBs} unit=" MB/s" />
          <Chart title={t("health.healthStatus.charts.nodeNetTx")} series={nodeTx} transform={toMBs} unit=" MB/s" />
          <Chart title={t("health.healthStatus.charts.nodeDiskRd")} series={nodeDiskRd} transform={toMBs} unit=" MB/s" />
          <Chart title={t("health.healthStatus.charts.nodeDiskWr")} series={nodeDiskWr} transform={toMBs} unit=" MB/s" />
          <Chart title={t("health.healthStatus.charts.nodeDiskUtil")} series={nodeDiskUtil} unit="%" />
        </div>
        <p className="mt-2 text-[11px] text-slate-500">
          {t("health.healthStatus.trends.diskUtilNote")}
        </p>
      </section>

      <p className="text-xs text-slate-500">
        {t("health.healthStatus.footer", { range: RANGE })}
      </p>
    </>
  );
}
