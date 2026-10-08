import { useEffect, useState, useCallback } from "react";
import { getActivities, ApiError, type Activity } from "../api";
import { fmtTime } from "../components/ui";
import { href } from "../router";
import { hasNickname, nodeLabel } from "../lib/node-label";
import { messageText } from "../lib/messages";
import { getLang, useT, type Key, type TFn } from "../i18n";

// Live Feed (ADR-022): "under the hood" of the orchestration. Short refresh for a
// real-time feel (no SSE in V1; polling like the rest of the Portal).
const REFRESH_MS = 4_000;

// Color by activity type. "Downloading" and "Seeding" follow the Portal's transfer
// convention (blue = in, green = out) and match the status badges in `ui.tsx` — previously
// "Downloading" was `sky` here and `blue` in the badge, two blues for the same state (#130).
// "Scheduled" dropped blue precisely to avoid competing with download readability: it's a
// Scheduler decision, not a transfer.
type KindMeta = { icon: string; labelKey: Key; ring: string; text: string };
const KIND: Record<string, KindMeta> = {
  admission_hold: { icon: "⏸", labelKey: "feed.feed.kind.admissionHold", ring: "ring-amber-500/40", text: "text-amber-300" },
  admission_resume: { icon: "▶", labelKey: "feed.feed.kind.admissionResume", ring: "ring-emerald-500/40", text: "text-emerald-300" },
  scheduled: { icon: "🎯", labelKey: "feed.feed.kind.scheduled", ring: "ring-violet-500/40", text: "text-violet-300" },
  downloading: { icon: "⬇", labelKey: "feed.feed.kind.downloading", ring: "ring-blue-500/40", text: "text-blue-300" },
  seeding: { icon: "🌱", labelKey: "feed.feed.kind.seeding", ring: "ring-emerald-500/40", text: "text-emerald-300" },
  // screenshot_host_failed (#193, ADR-040): the tracker's image host failed and the fallback
  // (imgbb) was used. Not a critical error — upload succeeded — but the operator must know.
  screenshot_host_failed: { icon: "🖼", labelKey: "feed.feed.kind.screenshotHostFailed", ring: "ring-amber-500/40", text: "text-amber-300" },
};
const FALLBACK: KindMeta = { icon: "•", labelKey: "feed.feed.kind.fallback", ring: "ring-slate-500/40", text: "text-slate-300" };

const FILTERS: { key: string; labelKey: Key }[] = [
  { key: "ALL", labelKey: "feed.feed.filter.all" },
  { key: "scheduled", labelKey: "feed.feed.kind.scheduled" },
  { key: "downloading", labelKey: "feed.feed.kind.downloading" },
  { key: "seeding", labelKey: "feed.feed.kind.seeding" },
  { key: "admission_hold", labelKey: "feed.feed.kind.admissionHold" },
  { key: "admission_resume", labelKey: "feed.feed.kind.admissionResume" },
  { key: "screenshot_host_failed", labelKey: "feed.feed.kind.screenshotHostFailed" },
];

function fmtAgo(t: TFn, iso: string): string {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return t("feed.feed.time.secondsAgo", { count: s });
  if (s < 3600) return t("feed.feed.time.minutesAgo", { count: Math.floor(s / 60) });
  if (s < 86400) return t("feed.feed.time.hoursAgo", { count: Math.floor(s / 3600) });
  return t("feed.feed.time.daysAgo", { count: Math.floor(s / 86400) });
}

// reasonText: the Scheduler score, if any. The reason itself is in the sentence (ADR-041 §3:
// each reason's sentence comes from the catalog, by reason_code).
function reasonText(a: Activity, t: TFn): string {
  const score = a.detail?.score;
  return typeof score === "number" ? t("feed.feed.score", { score }) : "";
}

export function Feed({ onLogout }: { onLogout: () => void }) {
  const t = useT();
  const [acts, setActs] = useState<Activity[]>([]);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState("");
  const [filter, setFilter] = useState<(typeof FILTERS)[number]["key"]>("ALL");

  const load = useCallback(async () => {
    try {
      const a = await getActivities(150);
      setActs(a);
      setError("");
      setUpdatedAt(new Date().toLocaleTimeString(getLang()));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("feed.feed.error.loadFailed"));
    }
  }, [onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const items = filter === "ALL" ? acts : acts.filter((a) => a.kind === filter);

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-semibold text-white">{t("feed.feed.title")}</h1>
          <span className="flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs text-emerald-400 ring-1 ring-inset ring-emerald-500/30">
            <span className="relative flex h-2 w-2">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
            </span>
            {t("feed.feed.live")}
          </span>
        </div>
        <span className="text-xs text-slate-500">{updatedAt ? t("feed.feed.updatedAt", { time: updatedAt }) : t("common.loading")}</span>
      </div>

      <p className="text-xs text-slate-500">
        {t("feed.feed.description")}
      </p>

      <div className="flex flex-wrap items-center gap-1 rounded-lg border border-slate-800 bg-slate-900/60 p-1">
        {FILTERS.map((f) => (
          <button
            key={f.key}
            onClick={() => setFilter(f.key)}
            className={`rounded-md px-3 py-1 text-xs transition ${
              filter === f.key ? "bg-slate-800 text-white" : "text-slate-400 hover:text-slate-200"
            }`}
          >
            {t(f.labelKey)}
          </button>
        ))}
      </div>

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}

      <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-5">
        <ol className="space-y-0">
          {items.map((a, i) => {
            const k = KIND[a.kind] ?? FALLBACK;
            const reason = reasonText(a, t);
            return (
              <li key={a.id} className="relative flex gap-3 pb-5 last:pb-0">
                <div className="flex flex-col items-center">
                  <span
                    className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-950 text-sm ring-1 ${k.ring}`}
                  >
                    {k.icon}
                  </span>
                  {i < items.length - 1 && <span className="mt-1 w-px flex-1 bg-slate-800" />}
                </div>
                <div className="min-w-0 flex-1 pt-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={`text-xs font-semibold uppercase tracking-wide ${k.text}`}>{t(k.labelKey)}</span>
                    {a.torrent_id ? (
                      <a
                        href={href(`/torrents/${a.torrent_id}`)}
                        className="min-w-0 truncate text-sm font-medium text-white hover:text-emerald-300"
                      >
                        {a.torrent_name ?? "torrent"}
                      </a>
                    ) : a.torrent_name ? (
                      <span className="truncate text-sm font-medium text-white">{a.torrent_name}</span>
                    ) : null}
                    {a.node_hostname && (
                      <span
                        className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[11px] text-slate-300"
                        title={hasNickname({ hostname: a.node_hostname, display_name: a.node_display_name }) ? a.node_hostname : undefined}
                      >
                        {nodeLabel({ hostname: a.node_hostname, display_name: a.node_display_name })}
                      </span>
                    )}
                    <span
                      className="ml-auto whitespace-nowrap text-xs text-slate-500"
                      title={fmtTime(a.created_at)}
                    >
                      {fmtAgo(t, a.created_at)}
                    </span>
                  </div>
                  <p className="mt-0.5 text-xs text-slate-400">
                    {messageText("activity", a.kind, a.detail, a.message, { torrent_name: a.torrent_name ?? "" })}
                    {reason && <span className="text-slate-500"> · {reason}</span>}
                  </p>
                </div>
              </li>
            );
          })}
          {items.length === 0 && (
            <li className="py-10 text-center text-sm text-slate-500">
              {t("feed.feed.empty")}
            </li>
          )}
        </ol>
      </div>
    </>
  );
}
