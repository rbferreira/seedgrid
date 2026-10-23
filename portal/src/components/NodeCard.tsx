import type { Node, Torrent } from "../api";
import { Spinner } from "../actions";
import { href } from "../router";
import { hasNickname, nodeLabel } from "../lib/node-label";
import { useT } from "../i18n";
import {
  Badge,
  HealthBadge,
  Meter,
  Panel,
  Stat,
  fmtBytes,
  fmtRatio,
  fmtSince,
  fmtTime,
  fmtUptime,
  usedPct,
  TRANSFER,
} from "./ui";

/**
 * NodeCard: the console for a Seed Node — heartbeat features, aggregated data from its
 * torrents and Node actions. It originated from merging two cards that showed the same Node
 * in different ways: the one from the (defunct) Nodes screen, with aggregates and actions, and the one from
 * the Health screen, which was the only one to show disk I/O and "how much space is left" on disk.
 *
 * The stacked wide panel format (rather than the 380px card grid from Health) is what
 * accommodates the header row: error/recheck pills, status, storage, health, and the
 * four controls.
 *
 * It doesn't decide permissions nor confirm anything: it receives `can` and callbacks. Gating happens here,
 * because the destructive button must disappear — but the DATA (disk, memory, health) stays
 * visible to those who only have read access to metrics.
 */
export function NodeCard({
  node: n,
  torrents,
  can,
  busy,
  running,
  onDrain,
  onRestartEngine,
  onDelete,
  onRename,
}: {
  node: Node;
  torrents: Torrent[];
  can: (perm: string) => boolean;
  busy: boolean;
  running: Set<string>;
  onDrain: () => void;
  onRestartEngine: () => void;
  onDelete: () => void;
  onRename: () => void;
}) {
  const t = useT();
  const seeding = torrents.filter((t) => t.status === "SEEDING").length;
  const nodeErrors = torrents.filter((t) => t.status === "ERROR").length;
  const nodeChecking = torrents.filter((t) => t.status === "CHECKING").length;
  // Cumulative totals (#14): they survive Engine session resets, so the Node's ratio is
  // stable (the snapshot was resetting to zero on every re-add).
  const totalUp = torrents.reduce((s, t) => s + (t.uploaded_total ?? 0), 0);
  const totalDown = torrents.reduce((s, t) => s + (t.downloaded_total ?? 0), 0);
  const dlNow = torrents.reduce((s, t) => s + (t.dl_speed ?? 0), 0);
  const upNow = torrents.reduce((s, t) => s + (t.up_speed ?? 0), 0);
  const canOperate = can("nodes:write");

  return (
    <Panel
      // The title is the name the operator gave the Node (#145); the OS hostname sits next to it,
      // as a technical detail — it's what appears in the terminal and in firewall rules.
      title={
        <>
          {nodeLabel(n)}
          {hasNickname(n) && (
            <span className="ml-2 font-mono text-xs font-normal text-slate-500" title={t("health.nodeCard.title.hostname")}>
              {n.hostname}
            </span>
          )}
        </>
      }
      right={
        <div className="flex flex-wrap items-center justify-end gap-2">
          {nodeErrors > 0 && (
            <span className="rounded-full bg-red-500/15 px-2 py-0.5 text-xs font-medium text-red-400 ring-1 ring-inset ring-red-500/30">
              {t("health.nodeCard.badges.errors", { count: nodeErrors })}
            </span>
          )}
          {nodeChecking > 0 && (
            <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-400 ring-1 ring-inset ring-amber-500/30">
              {t("health.nodeCard.badges.recheck", { count: nodeChecking })}
            </span>
          )}
          <Badge label={n.status} />
          {n.storage_ok === true && (
            <span className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-400 ring-1 ring-inset ring-emerald-500/30">
              {t("health.nodeCard.badges.storageOk")}
            </span>
          )}
          {n.storage_ok === false && (
            <span
              title={n.storage_detail ?? undefined}
              className="rounded-full bg-red-500/15 px-2 py-0.5 text-xs font-medium text-red-400 ring-1 ring-inset ring-red-500/30"
            >
              {t("health.nodeCard.badges.storageFailed")}
            </span>
          )}
          {/* Inbound connectability: a Node can be ONLINE, with a reachable Engine
              and high health, yet no one from the outside can reach it — this was
              invisible for weeks when the provider rotated the IPv6 prefix
              and firewall rules started pointing to dead addresses. Absent =
              unknown: it renders no badge, to avoid asserting what isn't known. */}
          {n.engine_connectable === "firewalled" && (
            <span
              title={
                n.public_ipv6 && n.bt_port
                  ? t("health.nodeCard.badges.firewalledTitleWithAddr", { addr: `[${n.public_ipv6}]:${n.bt_port}` })
                  : t("health.nodeCard.badges.firewalledTitleGeneric")
              }
              className="rounded-full bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-400 ring-1 ring-inset ring-amber-500/30"
            >
              {t("health.nodeCard.badges.noInbound")}
            </span>
          )}
          {n.engine_connectable === "connected" && (
            <span
              title={t("health.nodeCard.badges.reachableTitle")}
              className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-400 ring-1 ring-inset ring-emerald-500/30"
            >
              {t("health.nodeCard.badges.reachable")}
            </span>
          )}
          <HealthBadge score={n.health_score} />
          <a
            href={href(`/nodes/${n.id}`)}
            className="rounded-lg border border-slate-700 px-2.5 py-1 text-xs text-emerald-400 transition hover:bg-slate-800"
          >
            {t("health.nodeCard.actions.details")}
          </a>
          {canOperate && (
            <>
              <button
                onClick={onRename}
                disabled={busy}
                title={t("health.nodeCard.actions.renameTitle")}
                className="rounded-full border border-slate-700 px-2.5 py-1 text-xs text-slate-300 transition hover:bg-slate-800 disabled:opacity-40"
              >
                {t("health.nodeCard.actions.rename")}
              </button>
              <button
                onClick={onDrain}
                disabled={busy || torrents.length === 0}
                title={t("health.nodeCard.actions.drainTitle")}
                className="rounded-full border border-slate-700 px-2.5 py-1 text-xs text-blue-400 transition hover:bg-slate-800 disabled:opacity-40"
              >
                {t("health.nodeCard.actions.drain")}
              </button>
              <button
                onClick={onRestartEngine}
                disabled={busy || running.size > 0}
                title={t("health.nodeCard.actions.restartEngineTitle")}
                className="inline-flex items-center gap-1.5 rounded-full border border-slate-700 px-2.5 py-1 text-xs text-amber-400 transition hover:bg-slate-800 disabled:opacity-40"
              >
                {running.has(n.id) ? <Spinner /> : "⟳"} {t("health.nodeCard.actions.restartEngine")}
              </button>
              <button
                onClick={onDelete}
                disabled={busy}
                title={t("health.nodeCard.actions.removeTitle")}
                className="rounded-full border border-slate-700 px-2.5 py-1 text-xs text-red-400 transition hover:bg-slate-800 disabled:opacity-40"
              >
                {t("health.nodeCard.actions.remove")}
              </button>
            </>
          )}
        </div>
      }
    >
      <div className="space-y-4 p-5">
        {/* Storage failure reason in full text: in the old card it only lived in the tooltip
            of the pill, and it's the information that tells you what to do (unmounted disk, permissions,
            unpowered drive). */}
        {n.storage_ok === false && n.storage_detail && (
          <p className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400">
            {n.storage_detail}
          </p>
        )}

        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-7">
          <Stat label={t("health.nodeCard.stats.downloadNow")} value={`${fmtBytes(dlNow)}/s`} accent={TRANSFER.down.text} />
          <Stat label={t("health.nodeCard.stats.uploadNow")} value={`${fmtBytes(upNow)}/s`} accent={TRANSFER.up.text} />
          <Stat label={t("health.nodeCard.stats.torrents")} value={String(torrents.length)} />
          <Stat label={t("health.nodeCard.stats.seeding")} value={String(seeding)} accent="text-emerald-400" />
          <Stat label={t("health.nodeCard.stats.totalDownloaded")} value={fmtBytes(totalDown)} accent={TRANSFER.down.text} />
          <Stat label={t("health.nodeCard.stats.totalUploaded")} value={fmtBytes(totalUp)} accent={TRANSFER.up.text} />
          <Stat label={t("health.nodeCard.stats.ratioLocal")} value={fmtRatio(totalUp, totalDown)} accent="text-amber-300" />
        </div>

        {/* Resource meters. The disk/memory detail shows FREE, not used: whoever
            manages storage asks "how much is left" — the used percentage already comes
            from the Meter itself. */}
        <div className="grid gap-4 sm:grid-cols-3">
          {n.disk_total_bytes ? (
            <Meter
              label={t("health.nodeCard.meters.disk")}
              used={usedPct(n.disk_free_bytes, n.disk_total_bytes)}
              detail={t("health.nodeCard.meters.freeDetail", { free: fmtBytes(n.disk_free_bytes), total: fmtBytes(n.disk_total_bytes) })}
            />
          ) : null}
          {n.mem_total_bytes ? (
            <Meter
              label={t("health.nodeCard.meters.memory")}
              used={usedPct(n.mem_avail_bytes, n.mem_total_bytes)}
              detail={t("health.nodeCard.meters.freeDetail", { free: fmtBytes(n.mem_avail_bytes), total: fmtBytes(n.mem_total_bytes) })}
            />
          ) : null}
          {/* Disk I/O: it was exclusive to the Health screen and is the signal that explains slow
              torrents with idle CPU — sustained ~100% = saturated disk. */}
          {n.disk_io_util_pct != null ? (
            <Meter label={t("health.nodeCard.meters.diskIo")} used={Math.round(n.disk_io_util_pct)} detail={t("health.nodeCard.meters.utilization")} />
          ) : null}
          {/* CPU: measured by the Agent via /proc/stat, NOT derived from load — load counts
              processes waiting for I/O, so on a Node with a saturated disk it goes up without
              the CPU being busy, and the bar would lie exactly where it matters most.
              The detail shows cores and load because the same % weighs differently on 2 and 4
              cores, and the distance between the two numbers exposes I/O wait. */}
          {n.cpu_util_pct != null ? (
            <Meter
              label={t("health.nodeCard.meters.cpu")}
              used={Math.round(n.cpu_util_pct)}
              detail={
                (n.cpu_cores ? t("health.nodeCard.meters.cores", { count: n.cpu_cores }) : t("health.nodeCard.meters.utilization")) +
                (n.load1 != null ? ` · load ${n.load1.toFixed(2)}` : "")
              }
            />
          ) : null}
        </div>

        <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-slate-400">
          <span>
            {t("health.nodeCard.info.endpointBt")}:{" "}
            <span
              className="font-mono text-slate-300"
              title={t("health.nodeCard.info.endpointBtTitle")}
            >
              {n.last_ip && n.bt_port ? `${n.last_ip}:${n.bt_port}` : "—"}
            </span>
          </span>
          {/* The address the operator needs to allow. Under CGNAT IPv4 port-forwarding is
              harmless and the inbound only exists over IPv6 (ADR-034) — without this here, finding
              the address requires logging into the Node, and realizing it CHANGED won't happen. */}
          {n.public_ipv6 && n.bt_port ? (
            <span>
              {t("health.nodeCard.info.inboundIpv6")}:{" "}
              <span
                className="font-mono text-slate-300"
                title={t("health.nodeCard.info.inboundIpv6Title")}
              >
                [{n.public_ipv6}]:{n.bt_port}
              </span>
            </span>
          ) : null}
          <span>
            {t("health.nodeCard.info.engine")}:{" "}
            <span className={n.engine_reachable === false ? "text-red-400" : "text-slate-300"}>
              {n.engine ?? "—"}
              {n.engine_version ? ` ${n.engine_version}` : ""}
              {n.engine_reachable === false ? ` (${t("health.nodeCard.info.unreachable")})` : ""}
            </span>
          </span>
          <span>
            {t("health.nodeCard.info.agent")}: <span className="text-slate-300">{n.agent_version ?? "—"}</span>
          </span>
          <span>
            {t("health.nodeCard.meters.cpu")}:{" "}
            <span className="text-slate-300">
              {n.cpu_cores ? t("health.nodeCard.meters.cores", { count: n.cpu_cores }) : "—"}
              {n.load1 != null ? ` · load ${n.load1.toFixed(2)}` : ""}
            </span>
          </span>
          <span>
            {t("health.nodeCard.info.uptimeHost")}: <span className="text-slate-300">{fmtUptime(n.host_uptime_seconds)}</span>
          </span>
          <span>
            {t("health.nodeCard.info.uptimeService")}: <span className="text-slate-300">{fmtUptime(n.uptime_seconds)}</span>
          </span>
          <span>
            {t("health.nodeCard.info.uptimeEngine")}: <span className="text-slate-300">{fmtSince(n.engine_started_at)}</span>
          </span>
          <span>
            {t("health.nodeCard.info.heartbeat")}: <span className="text-slate-300">{fmtTime(n.last_seen_at)}</span>
          </span>
        </div>
      </div>
    </Panel>
  );
}
