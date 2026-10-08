import { useEffect, useState, useCallback, type ReactNode } from "react";
import {
  getTorrents,
  getNodes,
  getTorrentSamples,
  pauseTorrent,
  resumeTorrent,
  recheckTorrent,
  removeTorrent,
  downloadTorrentContent,
  ApiError,
  type Torrent,
  type Node,
  type TorrentSample,
  type MetricSeries,
} from "../api";
import { Badge, BackLink, Panel, fmtTime, fmtBytes, fmtRatio, shortId, TRANSFER } from "../components/ui";
import { startAction, useRunningId, Spinner, toast } from "../actions";
import { Chart, type TransferKind } from "../components/Chart";
import { href, navigate } from "../router";
import { nodeLabelById } from "../lib/node-label";
import { useT } from "../i18n";

const toGB = (v: number) => Math.round((v / 1024 ** 3) * 100) / 100;
const toMBs = (v: number) => Math.round((v / 1024 ** 2) * 100) / 100;

// series builds a MetricSeries (Chart format) from the torrent samples.
function series(
  samples: TorrentSample[],
  label: string,
  pick: (s: TorrentSample) => number,
  transfer?: TransferKind,
): MetricSeries {
  const labels: Record<string, string> = transfer ? { instance: label, transfer } : { instance: label };
  return { labels, points: samples.map((s) => ({ t: s.t, v: pick(s) })) };
}

const REFRESH_MS = 10_000;

// fmtEta formats seconds into compact text ("2h 5m"); "—" when absent/invalid.
function fmtEta(secs?: number): string {
  if (secs == null || secs <= 0 || secs >= 8_640_000) return "—";
  const d = Math.floor(secs / 86_400);
  const h = Math.floor((secs % 86_400) / 3_600);
  const m = Math.floor((secs % 3_600) / 60);
  const s = Math.floor(secs % 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function Field({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-slate-800/60 py-1.5 text-sm last:border-0">
      <dt className="text-slate-400">{label}</dt>
      <dd className="text-right text-slate-200">{value}</dd>
    </div>
  );
}

// `can` gates each button using the SAME permission the backend requires on the route it calls
// (button gating — docs/11 - Portal.md §13.1). This page is accessible by any role,
// and the data (progress, swarm, charts) remains visible to everyone: only the button disappears.
export function TorrentDetail({
  torrentId,
  can,
  onLogout,
}: {
  torrentId: string;
  can: (perm: string) => boolean;
  onLogout: () => void;
}) {
  const t = useT();
  const [torrent, setTorrent] = useState<Torrent | null>(null);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [samples, setSamples] = useState<TorrentSample[]>([]);
  const [error, setError] = useState("");
  const [notFound, setNotFound] = useState(false);
  const [busy, setBusy] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const running = useRunningId();

  // Content download via the Portal (#1): requests a ticket and triggers
  // browser navigation (the CP proxies the Node stream). The "downloading" only covers
  // ticket preparation — the download itself continues in the browser download manager.
  const onDownload = async () => {
    setDownloading(true);
    try {
      await downloadTorrentContent(torrentId);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      toast(err instanceof ApiError ? err.message : t("torrents.torrentDetail.downloadFailed"), "error");
    } finally {
      setDownloading(false);
    }
  };

  const load = useCallback(async () => {
    try {
      const [ts, ns, smp] = await Promise.all([
        getTorrents(),
        getNodes(),
        getTorrentSamples(torrentId, "6h").catch(() => [] as TorrentSample[]),
      ]);
      const found = ts.find((x) => x.id === torrentId) ?? null;
      setTorrent(found);
      setNotFound(!found);
      setNodes(ns);
      setSamples(smp);
      setError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("torrents.torrentDetail.loadFailed"));
    }
  }, [torrentId, onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const act = (verb: string, fn: (id: string) => Promise<string>) =>
    startAction({
      id: torrentId,
      label: t("torrents.torrentDetail.actionLabel", { verb, name: torrent?.name ?? shortId(torrent?.hash) }),
      run: () => fn(torrentId),
      onSettled: load,
    });
  const onRemove = async () => {
    if (!window.confirm(t("torrents.torrentDetail.confirmRemove", { name: torrent?.name ?? torrent?.hash ?? "" }))) {
      return;
    }
    setBusy(true);
    try {
      await removeTorrent(torrentId);
      navigate("/dashboard");
    } catch (err) {
      toast(err instanceof ApiError ? err.message : t("torrents.torrentDetail.removeFailed"), "error");
      setBusy(false);
    }
  };

  const back = <BackLink fallback="/dashboard" />;

  if (notFound) {
    return (
      <>
        {back}
        <p className="rounded-2xl border border-slate-800 bg-slate-900/60 px-5 py-8 text-center text-slate-500">
          {t("torrents.torrentDetail.notFound")}
        </p>
      </>
    );
  }

  if (!torrent) {
    return (
      <>
        {back}
        {error ? (
          <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
            {error}
          </div>
        ) : (
          <p className="text-sm text-slate-500">{t("common.loading")}</p>
        )}
      </>
    );
  }

  const tor = torrent;
  const nodeName = nodeLabelById(nodes, tor.node_id) || shortId(tor.node_id);
  const canDownload = can("torrents:download");
  const canWrite = can("torrents:write");
  const canDelete = can("torrents:delete");
  const progressPct = Math.round((tor.progress ?? 0) * 100);

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-3">
          {back}
          <h1 className="text-lg font-semibold text-white">{tor.name ?? shortId(tor.hash)}</h1>
          <Badge label={tor.status} />
        </div>
        <div className="flex items-center gap-1">
          {running.has(torrentId) ? (
            <span className="inline-flex items-center gap-2 rounded-lg border border-slate-700 px-3 py-1.5 text-xs text-slate-300">
              <Spinner /> {t("torrents.torrentDetail.processing")}
            </span>
          ) : (
            <>
              {canDownload && (
                <button
                  onClick={onDownload}
                  disabled={downloading || running.size > 0 || busy}
                  title={t("torrents.torrentDetail.downloadTitle")}
                  className={`inline-flex items-center gap-1.5 rounded-lg border border-slate-700 px-3 py-1.5 text-xs transition hover:bg-slate-800 disabled:opacity-40 ${TRANSFER.down.text}`}
                >
                  {downloading ? <Spinner /> : "⬇"} {t("torrents.torrentDetail.download")}
                </button>
              )}
              {canWrite &&
                (tor.status === "PAUSED" ? (
                  <button
                    onClick={() => act(t("torrents.torrentDetail.resumeVerb"), resumeTorrent)}
                    disabled={running.size > 0 || busy}
                    className="rounded-lg border border-slate-700 px-3 py-1.5 text-xs text-emerald-400 transition hover:bg-slate-800 disabled:opacity-40"
                  >
                    ▶ {t("torrents.torrentDetail.resume")}
                  </button>
                ) : (
                  <button
                    onClick={() => act(t("torrents.torrentDetail.pauseVerb"), pauseTorrent)}
                    disabled={running.size > 0 || busy}
                    className="rounded-lg border border-slate-700 px-3 py-1.5 text-xs text-amber-400 transition hover:bg-slate-800 disabled:opacity-40"
                  >
                    ⏸ {t("torrents.torrentDetail.pause")}
                  </button>
                ))}
              {canWrite && (
                <button
                  onClick={() => act(t("torrents.torrentDetail.recheckVerb"), recheckTorrent)}
                  disabled={running.size > 0 || busy}
                  title={t("torrents.torrentDetail.recheckTitle")}
                  className={`rounded-lg border border-slate-700 px-3 py-1.5 text-xs transition hover:bg-slate-800 disabled:opacity-40 ${
                    tor.status === "ERROR" ? "text-red-400" : "text-slate-300"
                  }`}
                >
                  ↻ {t("torrents.torrentDetail.recheck")}
                </button>
              )}
              {canDelete && (
                <button
                  onClick={onRemove}
                  disabled={running.size > 0 || busy}
                  className="rounded-lg border border-slate-700 px-3 py-1.5 text-xs text-red-400 transition hover:bg-slate-800 disabled:opacity-40"
                >
                  🗑 {t("torrents.torrentDetail.remove")}
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}


      {/* Progress */}
      <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-5">
        <div className="flex items-center gap-3">
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-slate-800">
            <div className="h-full rounded-full bg-emerald-500" style={{ width: `${progressPct}%` }} />
          </div>
          <span className="text-sm text-slate-300">{progressPct}%</span>
        </div>
      </div>

      {/* Details */}
      <Panel title={t("torrents.torrentDetail.detailsTitle")}>
        <dl className="px-5 py-3">
          <Field
            label={t("torrents.torrentDetail.fieldNode")}
            value={
              <a href={href(`/nodes/${tor.node_id}`)} className="text-emerald-400 hover:text-emerald-300">
                {nodeName}
              </a>
            }
          />
          <Field label={t("torrents.torrentDetail.fieldHash")} value={<span className="font-mono text-xs">{tor.hash ?? "—"}</span>} />
          <Field label={t("torrents.torrentDetail.fieldSize")} value={fmtBytes(tor.size_bytes)} />
          <Field
            label={t("torrents.torrentDetail.fieldSeedsPeers")}
            value={
              <span title={t("torrents.torrentDetail.swarmTooltip")}>
                <span className={TRANSFER.up.text}>{tor.num_seeds ?? 0}</span>
                {" / "}
                <span className={TRANSFER.down.text}>{tor.num_leechers ?? 0}</span>
                <span className="ml-1 text-xs text-slate-500">
                  ({tor.num_seeds_connected ?? 0}/{tor.num_leechers_connected ?? 0} {t("torrents.torrentDetail.connected")})
                </span>
              </span>
            }
          />
          <Field
            label={t("torrents.torrentDetail.fieldSpeed")}
            value={
              <span>
                <span className={TRANSFER.down.text}>{fmtBytes(tor.dl_speed)}/s</span>
                {" / "}
                <span className={TRANSFER.up.text}>{fmtBytes(tor.up_speed)}/s</span>
              </span>
            }
          />
          <Field label={t("torrents.torrentDetail.fieldRatio")} value={fmtRatio(tor.uploaded_total, tor.downloaded_total)} />
          <Field label={t("torrents.torrentDetail.fieldEta")} value={fmtEta(tor.eta_seconds)} />
          <Field
            label={t("torrents.torrentDetail.fieldTransferred")}
            value={
              <span>
                <span className={TRANSFER.down.text}>{fmtBytes(tor.downloaded_total)}</span>
                {" / "}
                <span className={TRANSFER.up.text}>{fmtBytes(tor.uploaded_total)}</span>
              </span>
            }
          />
          <Field label={t("torrents.torrentDetail.fieldCreated")} value={fmtTime(tor.created_at)} />
        </dl>
      </Panel>

      {/* Time series (6h window) */}
      <div className="grid gap-6 lg:grid-cols-2">
        <Chart
          title={t("torrents.torrentDetail.chartTransferTitle")}
          series={[
            series(samples, t("torrents.torrentDetail.seriesDownloaded"), (s) => s.downloaded_bytes, "down"),
            series(samples, t("torrents.torrentDetail.seriesUploaded"), (s) => s.uploaded_bytes, "up"),
          ]}
          transform={toGB}
          unit=" GB"
        />
        <Chart
          title={t("torrents.torrentDetail.chartSpeedTitle")}
          series={[
            series(samples, t("torrents.torrentDetail.seriesDlSpeed"), (s) => s.dl_speed),
            series(samples, t("torrents.torrentDetail.seriesUpSpeed"), (s) => s.up_speed),
          ]}
          transform={toMBs}
          unit=" MB/s"
        />
      </div>
    </>
  );
}
