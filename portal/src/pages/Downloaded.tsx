import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ApiError,
  getNodes,
  getTorrents,
  pauseTorrent,
  recheckTorrent,
  resumeTorrent,
  type Node,
  type Torrent,
} from "../api";
import { Badge, Panel, fmtBytes, fmtRatio, fmtTime, shortId, TRANSFER } from "../components/ui";
import { Spinner } from "../actions";
import {
  ActionBtn,
  SeedsPeers,
  SpeedPair,
  TorrentLink,
  isDone,
  useTorrentActions,
} from "../components/torrents";
import { MigratePicker, useMigrateTarget } from "../components/MigratePicker";
import { nodeLabelById } from "../lib/node-label";
import {
  filterTorrents,
  fmtAdded,
  sortTorrents,
  type SortDir,
  type SortKey,
} from "../lib/downloaded";
import { getLang, useT, type Key } from "../i18n";

const REFRESH_MS = 10_000;

// Filter size ranges — the same as Explore, plus one for the large archive.
const SIZE_STEPS: { labelKey: Key; min: number }[] = [
  { labelKey: "torrents.downloaded.filters.size.any", min: 0 },
  { labelKey: "torrents.downloaded.filters.size.gt100mb", min: 100 * 1024 ** 2 },
  { labelKey: "torrents.downloaded.filters.size.gt1gb", min: 1024 ** 3 },
  { labelKey: "torrents.downloaded.filters.size.gt5gb", min: 5 * 1024 ** 3 },
  { labelKey: "torrents.downloaded.filters.size.gt20gb", min: 20 * 1024 ** 3 },
];

// Periods of the "Added" filter. Rolling windows (the last N hours), not calendar days:
// "today" at 00:10 would hide what finished downloading at 23:50.
const DAY_MS = 24 * 60 * 60 * 1000;
const ADDED_STEPS: { labelKey: Key; ms: number }[] = [
  { labelKey: "torrents.downloaded.filters.added.any", ms: 0 },
  { labelKey: "torrents.downloaded.filters.added.last24h", ms: DAY_MS },
  { labelKey: "torrents.downloaded.filters.added.last7d", ms: 7 * DAY_MS },
  { labelKey: "torrents.downloaded.filters.added.last30d", ms: 30 * DAY_MS },
];

// Text columns start A→Z; numeric ones start from the largest, which is what you
// look for when clicking "Size" or "Uploaded".
const TEXT_KEYS: SortKey[] = ["name", "status", "node"];


/**
 * Downloaded: the archive — torrents that have completed and are seeding (`isDone`). The
 * Dashboard handles the rest (queue, downloads, recheck, errors, paused).
 *
 * Filters and sorting are client-side on `GET /torrents`, which already returns the entire
 * list each cycle: there's no paginated/filterable endpoint, and at the scale of this
 * cluster (hundreds of torrents) the cost is negligible compared to changing the contract.
 */
export function Downloaded({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [torrents, setTorrents] = useState<Torrent[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  // `loaded` separates "still loading" from "loaded and empty" — without it the screen
  // flashes the wrong empty state on every refresh.
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState("");

  const [q, setQ] = useState("");
  const [fNode, setFNode] = useState("");
  const [fStatus, setFStatus] = useState("");
  const [fMinSize, setFMinSize] = useState(0);
  const [fAdded, setFAdded] = useState(0);
  // Opens newest first: whoever lands here is usually looking for what just finished.
  const [sort, setSort] = useState<{ key: SortKey; dir: SortDir }>({ key: "added", dir: "desc" });
  // A single "now" per load: the filter cutoff and the column's "N min ago" come from the
  // same instant, and Date.now() stays out of render.
  const [now, setNow] = useState(() => Date.now());

  const load = useCallback(async () => {
    try {
      // getNodes requires `nodes:read`, which this page (gated by `torrents:read`) does
      // not guarantee: without the catch, a role without this permission would crash the whole screen over
      // a single column. It degrades to the Node's short id.
      const [ts, ns] = await Promise.all([getTorrents(), getNodes().catch(() => [] as Node[])]);
      setTorrents(ts.filter(isDone));
      setNodes(ns);
      setError("");
      setNow(Date.now());
      setUpdatedAt(new Date().toLocaleTimeString(getLang()));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("torrents.downloaded.error.loadFailed"));
    } finally {
      setLoaded(true);
    }
  }, [onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const { running, act, onRemove, onDownload } = useTorrentActions({ reload: load, onLogout });
  const { target, setTarget, confirm: confirmMigrate } = useMigrateTarget({ nodes, reload: load });

  const nodeName = useCallback(
    (id: string) => nodeLabelById(nodes, id) || shortId(id),
    [nodes],
  );

  // Node filter options derived from TORRENTS, not the Nodes list: a torrent
  // whose Node was removed remains in the list and needs an option to reach it.
  const nodeOptions = useMemo(() => {
    const ids = [...new Set(torrents.map((t) => t.node_id))];
    return ids.map((id) => ({ id, label: nodeName(id) })).sort((a, b) => a.label.localeCompare(b.label, getLang()));
  }, [torrents, nodeName]);

  // Actually present statuses. Today "Downloaded" is just SEEDING, so the select stays hidden
  // — a single-option control is noise. If the definition of "downloaded" grows, it appears.
  const statusOptions = useMemo(() => [...new Set(torrents.map((t) => t.status))].sort(), [torrents]);

  // Filter and sorting live in lib/downloaded — pure, tested without DOM (#98).
  const visible = useMemo(
    () =>
      sortTorrents(
        filterTorrents(torrents, {
          q,
          node: fNode,
          status: fStatus,
          minSize: fMinSize,
          addedSince: fAdded ? now - fAdded : 0,
        }),
        sort,
        nodeName,
      ),
    [torrents, q, fNode, fStatus, fMinSize, fAdded, now, sort, nodeName],
  );

  const hasFilter = !!q || !!fNode || !!fStatus || fMinSize > 0 || fAdded > 0;
  const clearFilters = () => {
    setQ("");
    setFNode("");
    setFStatus("");
    setFMinSize(0);
    setFAdded(0);
  };

  const toggleSort = (key: SortKey) =>
    setSort((s) =>
      s.key === key
        ? { key, dir: s.dir === "asc" ? "desc" : "asc" }
        : { key, dir: TEXT_KEYS.includes(key) ? "asc" : "desc" },
    );

  // Th: sortable header. The inner button (instead of onClick on the <th>) keeps the column
  // keyboard accessible, and aria-sort informs the screen reader.
  const Th = ({ label, k, align = "left" }: { label: string; k: SortKey; align?: "left" | "right" | "center" }) => {
    const active = sort.key === k;
    return (
      <th
        className={`px-3 py-2 font-medium ${align === "right" ? "text-right" : align === "center" ? "text-center" : ""}`}
        aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}
      >
        <button
          onClick={() => toggleSort(k)}
          title={t("torrents.downloaded.table.sortBy", { label: label.toLowerCase() })}
          className={`inline-flex items-center gap-1 uppercase transition hover:text-slate-300 ${
            active ? "text-emerald-400" : ""
          }`}
        >
          {label}
          <span aria-hidden className={active ? "" : "opacity-0"}>
            {active && sort.dir === "desc" ? "▼" : "▲"}
          </span>
        </button>
      </th>
    );
  };

  const canWrite = can("torrents:write");

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold text-white">{t("torrents.downloaded.title")}</h1>
          <p className="text-xs text-slate-500">
            {t("torrents.downloaded.subtitle")}
          </p>
        </div>
        <span className="text-xs text-slate-500">{updatedAt ? t("torrents.downloaded.updatedAt", { time: updatedAt }) : t("common.loading")}</span>
      </div>

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}

      <MigratePicker target={target} nodes={nodes} onPick={confirmMigrate} onClose={() => setTarget(null)} />

      {/* Filter bar. Instant search (the filter is local — no network throttling). */}
      <div className="flex flex-wrap items-center gap-2 rounded-2xl border border-slate-800 bg-slate-900/60 p-3">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("torrents.downloaded.filters.searchPlaceholder")}
          className="min-w-[240px] flex-1 rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        />
        <select
          value={fNode}
          onChange={(e) => setFNode(e.target.value)}
          title="Node"
          className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        >
          <option value="">{t("torrents.downloaded.filters.allNodes")}</option>
          {nodeOptions.map((n) => (
            <option key={n.id} value={n.id}>
              {n.label}
            </option>
          ))}
        </select>
        {statusOptions.length > 1 && (
          <select
            value={fStatus}
            onChange={(e) => setFStatus(e.target.value)}
            title={t("torrents.downloaded.filters.statusTitle")}
            className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
          >
            <option value="">{t("torrents.downloaded.filters.allStatuses")}</option>
            {statusOptions.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        )}
        <select
          value={fMinSize}
          onChange={(e) => setFMinSize(Number(e.target.value))}
          title={t("torrents.downloaded.filters.minSizeTitle")}
          className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        >
          {SIZE_STEPS.map((s) => (
            <option key={s.min} value={s.min}>
              {t(s.labelKey)}
            </option>
          ))}
        </select>
        <select
          value={fAdded}
          onChange={(e) => setFAdded(Number(e.target.value))}
          title={t("torrents.downloaded.filters.addedTitle")}
          className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        >
          {ADDED_STEPS.map((s) => (
            <option key={s.ms} value={s.ms}>
              {t(s.labelKey)}
            </option>
          ))}
        </select>
        {hasFilter && (
          <button onClick={clearFilters} className="text-sm text-slate-400 underline hover:text-slate-200">
            {t("torrents.downloaded.filters.clear")}
          </button>
        )}
        <span className="ml-auto text-sm text-slate-500">
          {t("torrents.downloaded.filters.seedingCount", { visible: visible.length, total: torrents.length })}
        </span>
      </div>

      <Panel title={t("torrents.downloaded.panelTitle", { count: visible.length })}>
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase text-slate-500">
            <tr>
              <Th label={t("torrents.downloaded.table.colName")} k="name" />
              <Th label={t("torrents.downloaded.table.colStatus")} k="status" />
              <Th label={t("torrents.downloaded.table.colSize")} k="size" align="right" />
              <Th label={t("torrents.downloaded.table.colNode")} k="node" />
              <Th label={t("torrents.downloaded.table.colAdded")} k="added" />
              <Th label={t("torrents.downloaded.table.colSpeed")} k="up" />
              <Th label={t("torrents.downloaded.table.colSeedsPeers")} k="seeds" align="center" />
              <Th label={t("torrents.downloaded.table.colTransferred")} k="uploaded" />
              <Th label={t("torrents.downloaded.table.colRatio")} k="ratio" />
              <th className="px-3 py-2 text-right font-medium">{t("torrents.downloaded.table.colActions")}</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((item) => (
              <tr key={item.id} className="border-t border-slate-800/60">
                <td className="px-3 py-2.5 font-medium text-white">
                  <TorrentLink t={item} />
                </td>
                <td className="px-3 py-2.5">
                  <Badge label={item.status} />
                </td>
                <td className="whitespace-nowrap px-3 py-2.5 text-right text-slate-300">
                  {item.size_bytes != null ? fmtBytes(item.size_bytes) : "—"}
                </td>
                <td className="px-3 py-2.5 text-slate-300">{nodeName(item.node_id)}</td>
                <td className="whitespace-nowrap px-3 py-2.5 text-slate-300" title={fmtTime(item.created_at)}>
                  {fmtAdded(t, item, now)}
                </td>
                <td className="whitespace-nowrap px-3 py-2.5 text-xs">
                  <SpeedPair dl={item.dl_speed} up={item.up_speed} />
                </td>
                <td className="whitespace-nowrap px-3 py-2.5 text-center text-slate-300">
                  <SeedsPeers t={item} />
                </td>
                <td className="px-3 py-2.5 text-xs text-slate-400">
                  <span className={TRANSFER.down.text}>{fmtBytes(item.downloaded_total)}</span>
                  {" / "}
                  <span className={TRANSFER.up.text}>{fmtBytes(item.uploaded_total)}</span>
                </td>
                <td className="px-3 py-2.5 text-slate-300">{fmtRatio(item.uploaded_total, item.downloaded_total)}</td>
                <td className="px-3 py-2.5">
                  <div className="flex items-center justify-end gap-1">
                    {running.has(item.id) ? (
                      <Spinner className="mr-1" />
                    ) : (
                      <>
                        {can("torrents:download") && (
                          <ActionBtn
                            tone="blue"
                            title={t("torrents.downloaded.actions.downloadContent")}
                            onClick={() => onDownload(item)}
                            disabled={running.size > 0}
                          >
                            ⬇
                          </ActionBtn>
                        )}
                        {canWrite &&
                          (item.status === "PAUSED" ? (
                            <ActionBtn
                              tone="emerald"
                              title={t("torrents.downloaded.actions.resume")}
                              onClick={() => act(item, t("torrents.downloaded.actions.resume"), resumeTorrent)}
                              disabled={running.size > 0}
                            >
                              ▶
                            </ActionBtn>
                          ) : (
                            <ActionBtn
                              tone="amber"
                              title={t("torrents.downloaded.actions.pause")}
                              onClick={() => act(item, t("torrents.downloaded.actions.pause"), pauseTorrent)}
                              disabled={running.size > 0}
                            >
                              ⏸
                            </ActionBtn>
                          ))}
                        {canWrite && (
                          <ActionBtn
                            tone={item.status === "ERROR" ? "red" : "neutral"}
                            title={t("torrents.downloaded.actions.recheck")}
                            onClick={() => act(item, t("torrents.downloaded.actions.recheck"), recheckTorrent)}
                            disabled={running.size > 0}
                          >
                            ↻
                          </ActionBtn>
                        )}
                        {can("torrents:migrate") && (
                          <ActionBtn
                            tone="blue"
                            title={t("torrents.downloaded.actions.migrate")}
                            onClick={() =>
                              setTarget({
                                kind: "torrent",
                                id: item.id,
                                name: item.name ?? shortId(item.hash),
                                sourceNodeId: item.node_id,
                                label: t("torrents.downloaded.actions.migrateConfirmLabel", { name: item.name ?? shortId(item.hash) }),
                              })
                            }
                            disabled={running.size > 0}
                          >
                            ⇄
                          </ActionBtn>
                        )}
                        {can("torrents:delete") && (
                          <ActionBtn
                            tone="red"
                            title={t("torrents.downloaded.actions.delete")}
                            onClick={() => onRemove(item)}
                            disabled={running.size > 0}
                          >
                            🗑
                          </ActionBtn>
                        )}
                      </>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {visible.length === 0 && (
              <tr>
                <td colSpan={10} className="px-3 py-8 text-center text-slate-500">
                  {!loaded ? (
                    t("common.loading")
                  ) : torrents.length === 0 ? (
                    <>
                      {t("torrents.downloaded.empty.noTorrents")}{" "}
                      <a href="#/add" className="text-emerald-400 underline hover:text-emerald-300">
                        {t("torrents.downloaded.empty.addLink")}
                      </a>
                    </>
                  ) : (
                    <>
                      {t("torrents.downloaded.empty.noMatches")}{" "}
                      <button onClick={clearFilters} className="text-emerald-400 underline hover:text-emerald-300">
                        {t("torrents.downloaded.filters.clear")}
                      </button>
                    </>
                  )}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Panel>
    </>
  );
}
