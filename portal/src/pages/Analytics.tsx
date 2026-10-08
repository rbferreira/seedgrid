import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError, getAnalytics, type AnalyticsReport, type AnalyticsTorrent } from "../api";
import { Badge, Panel, fmtBytes, fmtRatio, shortId, TRANSFER } from "../components/ui";
import { byWorstRatio } from "../lib/ranking";
import { TorrentLink } from "../components/torrents";
import { Spinner } from "../actions";
import { nodeLabel } from "../lib/node-label";
import { getLang, rich, useT, type Key } from "../i18n";

/**
 * Analytics: the collection questions the Portal couldn't answer (#160).
 *
 * Two rules for this screen, both learned the hard way:
 *
 * 1. **Counter without a list is a dead end** (#169). Each card is a table FILTER —
 *    seeing "157 with dead swarm" without reaching the 157 is worse than not showing the number.
 * 2. **Use the project's visual language, don't invent another**: `Panel`, `Stat`,
 *    `Badge` and the transfer axis color convention (#130 — blue=inbound,
 *    green=outbound) apply here just like on any other screen.
 */

const REFRESH_MS = 60_000;

type FilterKind = "todos" | "tracker_dead" | "duplicate_extra" | "never_uploaded" | "dead_swarm";
type Order = "size" | "age" | "ratio" | "seeds" | "name";

// Motif color map, following the same semantics as STATUS_STYLES: red for what's
// broken, amber for what requires a decision, slate for what's just inert.
// The key is the motif code (the same filter id); the text comes from the catalog.
type Reason = Exclude<FilterKind, "todos">;
const REASON_STYLES: Record<Reason, string> = {
  tracker_dead: "bg-red-500/15 text-red-400 ring-red-500/30",
  dead_swarm: "bg-red-500/15 text-red-400 ring-red-500/30",
  duplicate_extra: "bg-amber-500/15 text-amber-400 ring-amber-500/30",
  never_uploaded: "bg-slate-500/15 text-slate-300 ring-slate-500/30",
};
const REASON_KEY: Record<Reason, Key> = {
  tracker_dead: "analytics.analytics.reasons.trackerDead",
  dead_swarm: "analytics.analytics.reasons.deadSwarm",
  duplicate_extra: "analytics.analytics.reasons.duplicateExtra",
  never_uploaded: "analytics.analytics.reasons.neverUploaded",
};

const CARDS: { id: Reason; labelKey: Key; hintKey: Key }[] = [
  { id: "tracker_dead", labelKey: "analytics.analytics.cards.trackerDead", hintKey: "analytics.analytics.cards.trackerDeadHint" },
  { id: "duplicate_extra", labelKey: "analytics.analytics.cards.duplicateExtra", hintKey: "analytics.analytics.cards.duplicateExtraHint" },
  { id: "never_uploaded", labelKey: "analytics.analytics.cards.neverUploaded", hintKey: "analytics.analytics.cards.neverUploadedHint" },
  { id: "dead_swarm", labelKey: "analytics.analytics.cards.deadSwarm", hintKey: "analytics.analytics.cards.deadSwarmHint" },
];

const SIZE_STEPS: { labelKey: Key; min: number }[] = [
  { labelKey: "analytics.analytics.size.any", min: 0 },
  { labelKey: "analytics.analytics.size.gt1", min: 1024 ** 3 },
  { labelKey: "analytics.analytics.size.gt5", min: 5 * 1024 ** 3 },
  { labelKey: "analytics.analytics.size.gt20", min: 20 * 1024 ** 3 },
];

function has(t: AnalyticsTorrent, f: FilterKind): boolean {
  switch (f) {
    case "todos":
      return true;
    case "tracker_dead":
      return !!t.tracker_dead;
    case "duplicate_extra":
      return !!t.duplicate_extra;
    case "never_uploaded":
      return !!t.never_uploaded;
    case "dead_swarm":
      return !!t.dead_swarm;
  }
}

function reasonsOf(t: AnalyticsTorrent): Reason[] {
  const m: Reason[] = [];
  if (t.tracker_dead) m.push("tracker_dead");
  if (t.duplicate_extra) m.push("duplicate_extra");
  // "dead swarm" already implies "never sent"; showing both would be noise.
  if (t.dead_swarm) m.push("dead_swarm");
  else if (t.never_uploaded) m.push("never_uploaded");
  return m;
}

export function Analytics({ onLogout }: { onLogout: () => void }) {
  const t = useT();
  const [rep, setRep] = useState<AnalyticsReport | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [activeFilter, setFilter] = useState<FilterKind>("todos");
  const [q, setQ] = useState("");
  const [fNode, setFNode] = useState("");
  const [fMinSize, setFMinSize] = useState(0);
  const [order, setOrder] = useState<Order>("size");

  const load = useCallback(async () => {
    try {
      setRep(await getAnalytics());
      setError("");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        onLogout();
        return;
      }
      setError(e instanceof Error ? e.message : t("analytics.analytics.loadError"));
    } finally {
      setLoading(false);
    }
  }, [onLogout, t]);

  useEffect(() => {
    void load();
    const id = setInterval(() => void load(), REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const flagged = useMemo(() => rep?.flagged ?? [], [rep]);

  // Counters derived from the SAME list the table shows — that's what ensures the card
  // and the click result match (#169).
  const counts = useMemo(() => {
    const c: Record<string, { n: number; bytes: number }> = {};
    for (const card of CARDS) c[card.id] = { n: 0, bytes: 0 };
    for (const t of flagged) {
      for (const card of CARDS) {
        if (has(t, card.id)) {
          c[card.id].n += 1;
          c[card.id].bytes += t.size_bytes;
        }
      }
    }
    return c;
  }, [flagged]);

  const nodeOptions = useMemo(() => {
    const m = new Map<string, string>();
    for (const t of flagged) m.set(t.node_id, nodeLabel(t) || shortId(t.node_id));
    return [...m].map(([id, label]) => ({ id, label })).sort((a, b) => a.label.localeCompare(b.label, getLang()));
  }, [flagged]);

  const visible = useMemo(() => {
    const term = q.trim().toLowerCase();
    const filteredList = flagged.filter((t) => {
      if (!has(t, activeFilter)) return false;
      if (term && !(t.name ?? "").toLowerCase().includes(term)) return false;
      if (fNode && t.node_id !== fNode) return false;
      if (fMinSize && t.size_bytes < fMinSize) return false;
      return true;
    });
    const cmp: Record<Order, (a: AnalyticsTorrent, b: AnalyticsTorrent) => number> = {
      size: (a, b) => b.size_bytes - a.size_bytes,
      age: (a, b) => b.age_days - a.age_days,
      // No download doesn't mean zero ratio: `byWorstRatio` sends those with no ratio to the
      // bottom, otherwise the stopped ones top "worst ratio" and hide the truly bad ones.
      ratio: byWorstRatio,
      seeds: (a, b) => a.num_seeds - b.num_seeds,
      name: (a, b) => (a.name ?? "").localeCompare(b.name ?? "", getLang(), { numeric: true }),
    };
    return [...filteredList].sort(cmp[order]);
  }, [flagged, activeFilter, q, fNode, fMinSize, order]);

  if (loading && !rep) {
    return (
      <div className="p-4 text-sm text-slate-400">
        <Spinner /> {t("analytics.analytics.loadingCollection")}
      </div>
    );
  }
  if (error && !rep) {
    return (
      <div className="m-4 rounded-2xl border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-400">{error}</div>
    );
  }
  if (!rep) return null;

  const dups = rep.duplicates ?? [];
  const identicalDups = dups.filter((d) => d.identical);
  const bytesVisible = visible.reduce((a, t) => a + t.size_bytes, 0);
  const hasFilter = activeFilter !== "todos" || !!q || !!fNode || fMinSize > 0;
  const clear = () => {
    setFilter("todos");
    setQ("");
    setFNode("");
    setFMinSize(0);
  };

  return (
    <div className="space-y-4 p-4">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold text-white">{t("analytics.analytics.title")}</h1>
          <p className="text-sm text-slate-500">
            {t("analytics.analytics.summary", { total: rep.total, bytes: fmtBytes(rep.total_bytes) })}
            {rep.tracker_unknown > 0 && ` · ${t("analytics.analytics.unknownVerdict", { count: rep.tracker_unknown })}`}
          </p>
        </div>
      </div>

      {/* Cards = filters. Same Stat look; the emerald border marks the active one. */}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {CARDS.map((c) => {
          const isActive = activeFilter === c.id;
          const { n, bytes } = counts[c.id];
          return (
            <button
              key={c.id}
              type="button"
              onClick={() => setFilter(isActive ? "todos" : c.id)}
              title={t("analytics.analytics.filterTitle", { label: t(c.labelKey) })}
              className={`rounded-xl border px-3 py-2 text-left transition ${
                isActive
                  ? "border-emerald-500 bg-emerald-500/10"
                  : "border-slate-800 bg-slate-950/40 hover:border-slate-700"
              }`}
            >
              <p className="text-[10px] uppercase tracking-wide text-slate-500">{t(c.labelKey)}</p>
              <p className={`mt-0.5 text-sm font-semibold ${n ? "text-amber-400" : "text-white"}`}>
                {n} · {fmtBytes(bytes)}
              </p>
              <p className="mt-0.5 text-xs text-slate-500">{t(c.hintKey)}</p>
            </button>
          );
        })}
      </div>

      {/* Filter bar following the same pattern as the Downloaded screen. */}
      <div className="flex flex-wrap items-center gap-2 rounded-2xl border border-slate-800 bg-slate-900/60 p-3">
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("analytics.analytics.searchPlaceholder")}
          className="min-w-[240px] flex-1 rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        />
        <select
          value={fNode}
          onChange={(e) => setFNode(e.target.value)}
          title={t("analytics.analytics.nodeFilterTitle")}
          className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        >
          <option value="">{t("analytics.analytics.allNodes")}</option>
          {nodeOptions.map((n) => (
            <option key={n.id} value={n.id}>
              {n.label}
            </option>
          ))}
        </select>
        <select
          value={fMinSize}
          onChange={(e) => setFMinSize(Number(e.target.value))}
          title={t("analytics.analytics.minSizeTitle")}
          className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        >
          {SIZE_STEPS.map((s) => (
            <option key={s.min} value={s.min}>
              {t(s.labelKey)}
            </option>
          ))}
        </select>
        <select
          value={order}
          onChange={(e) => setOrder(e.target.value as Order)}
          title={t("analytics.analytics.sortTitle")}
          className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        >
          <option value="size">{t("analytics.analytics.sort.size")}</option>
          <option value="age">{t("analytics.analytics.sort.age")}</option>
          <option value="ratio">{t("analytics.analytics.sort.ratio")}</option>
          <option value="seeds">{t("analytics.analytics.sort.seeds")}</option>
          <option value="name">{t("analytics.analytics.sort.name")}</option>
        </select>
        {hasFilter && (
          <button onClick={clear} className="text-sm text-slate-400 underline hover:text-slate-200">
            {t("analytics.analytics.clearFilters")}
          </button>
        )}
        <span className="ml-auto text-sm text-slate-500">
          {t("analytics.analytics.visibleOf", { visible: visible.length, total: flagged.length, bytes: fmtBytes(bytesVisible) })}
        </span>
      </div>

      <Panel
        title={`${activeFilter === "todos" ? t("analytics.analytics.panel.flagged") : t(CARDS.find((c) => c.id === activeFilter)!.labelKey)} (${visible.length})`}
        right={<span className="text-xs text-slate-500">{t("analytics.analytics.onDisk", { size: fmtBytes(bytesVisible) })}</span>}
      >
        {visible.length === 0 ? (
          <p className="px-4 py-8 text-center text-sm text-slate-500">
            {flagged.length === 0 ? t("analytics.analytics.empty.clean") : t("analytics.analytics.empty.filtered")}
          </p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase text-slate-500">
              <tr>
                <th className="px-3 py-2 font-medium">{t("analytics.analytics.table.name")}</th>
                <th className="px-3 py-2 font-medium">{t("analytics.analytics.table.reason")}</th>
                <th className="px-3 py-2 font-medium">{t("analytics.analytics.table.node")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.table.size")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.table.age")}</th>
                <th className="px-3 py-2 text-center font-medium">{t("analytics.analytics.table.seeds")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.table.ratio")}</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((tItem) => (
                <tr key={tItem.id} className="border-t border-slate-800/60">
                  <td className="px-3 py-2.5 font-medium text-white">
                    <TorrentLink t={tItem as never} />
                    {tItem.duplicate_of && (
                      <p className="text-xs font-normal text-slate-500">{t("analytics.analytics.copyOf", { name: tItem.duplicate_of })}</p>
                    )}
                    {tItem.tracker_message && (
                      <p className="text-xs font-normal text-slate-500">{tItem.tracker_message}</p>
                    )}
                  </td>
                  <td className="px-3 py-2.5">
                    <div className="flex flex-wrap gap-1">
                      {reasonsOf(tItem).map((m) => (
                        <Badge key={m} label={m} map={REASON_STYLES} text={t(REASON_KEY[m])} />
                      ))}
                    </div>
                  </td>
                  <td className="px-3 py-2.5 text-slate-400">{nodeLabel(tItem) || shortId(tItem.node_id)}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-slate-300">{fmtBytes(tItem.size_bytes)}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-slate-400">{Math.round(tItem.age_days)}d</td>
                  {/* Seeds is OUTBOUND on the transfer axis (#130): green. */}
                  <td className={`px-3 py-2.5 text-center tabular-nums ${tItem.num_seeds ? TRANSFER.up.text : "text-slate-500"}`}>
                    {tItem.num_seeds}
                  </td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-amber-400">
                    {fmtRatio(tItem.uploaded_total, tItem.downloaded_total)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      <div className="grid gap-4 xl:grid-cols-2">
        <Panel
          title={`${t("analytics.analytics.panel.repeatedTitles")} (${dups.length})`}
          right={
            identicalDups.length > 0 ? (
              <span className="text-xs text-red-400">{t("analytics.analytics.identicalCount", { count: identicalDups.length })}</span>
            ) : undefined
          }
        >
          {dups.length === 0 ? (
            <p className="px-4 py-8 text-center text-sm text-slate-500">{t("analytics.analytics.dups.empty")}</p>
          ) : (
            <>
              {/* Without this caveat the screen becomes a "delete this" list: only the identical
                  group is pure waste. Different resolution or release the tracker
                  accepts, and both yield ratio — the decision is the operator's. */}
              <p className="border-b border-slate-800 px-4 py-2.5 text-xs text-slate-500">
                {rich(t("analytics.analytics.dups.disclaimer"), { redMarker: <span className="text-red-400">{t("analytics.analytics.dups.redMarker")}</span> })}
              </p>
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase text-slate-500">
                <tr>
                  <th className="px-3 py-2 font-medium">{t("analytics.analytics.dups.table.title")}</th>
                  <th className="px-3 py-2 font-medium">{t("analytics.analytics.dups.table.copies")}</th>
                  <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.dups.table.reclaim")}</th>
                </tr>
              </thead>
              <tbody>
                {dups.map((d, i) => (
                  <tr key={`${d.title}-${i}`} className="border-t border-slate-800/60 align-top">
                    <td className="px-3 py-2.5">
                      <p className="font-medium text-white">
                        {d.title}
                        {d.year ? ` (${d.year})` : ""} {d.se}
                        {d.part ? ` — ${d.part}` : ""}
                      </p>
                      {d.identical && (
                        <p className="mt-0.5 text-xs text-red-400">
                          {t("analytics.analytics.dups.identicalWarn")}
                        </p>
                      )}
                    </td>
                    <td className="px-3 py-2.5">
                      {d.copies.map((c) => (
                        <p key={c.id} className="text-xs text-slate-400">
                          <span className="text-slate-500">{c.resolution || "?"}</span> · {fmtBytes(c.size_bytes)} ·{" "}
                          {nodeLabel(c) || shortId(c.node_id)} ·{" "}
                          <span className={c.num_seeds ? TRANSFER.up.text : "text-slate-500"}>
                            {c.num_seeds} seeds
                          </span>
                        </p>
                      ))}
                    </td>
                    <td className="px-3 py-2.5 text-right tabular-nums text-amber-400">
                      {fmtBytes(d.reclaim_bytes)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            </>
          )}
        </Panel>

        <Panel title={t("analytics.analytics.panel.spaceByNode")}>
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase text-slate-500">
              <tr>
                <th className="px-3 py-2 font-medium">{t("analytics.analytics.table.node")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.spaceTable.torrents")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.spaceTable.occupied")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.spaceTable.idle")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("analytics.analytics.spaceTable.duplicated")}</th>
              </tr>
            </thead>
            <tbody>
              {(rep.nodes ?? []).map((n) => (
                <tr key={n.node_id} className="border-t border-slate-800/60">
                  <td className="px-3 py-2.5 font-medium text-white">{nodeLabel(n) || shortId(n.node_id)}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-slate-400">{n.torrents}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-slate-300">{fmtBytes(n.bytes)}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-amber-400">{fmtBytes(n.idle_bytes)}</td>
                  <td className="px-3 py-2.5 text-right tabular-nums text-amber-400">
                    {fmtBytes(n.reclaim_bytes)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </Panel>
      </div>
    </div>
  );
}
