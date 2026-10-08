import { useEffect, useState, type ReactNode } from "react";
import { getLang, useT } from "../i18n";

export const STATUS_STYLES: Record<string, string> = {
  ONLINE: "bg-emerald-500/15 text-emerald-400 ring-emerald-500/30",
  SEEDING: "bg-emerald-500/15 text-emerald-400 ring-emerald-500/30",
  OFFLINE: "bg-red-500/15 text-red-400 ring-red-500/30",
  ERROR: "bg-red-500/15 text-red-400 ring-red-500/30",
  DOWNLOADING: "bg-blue-500/15 text-blue-400 ring-blue-500/30",
  QUEUED: "bg-amber-500/15 text-amber-400 ring-amber-500/30",
  PAUSED: "bg-slate-500/15 text-slate-300 ring-slate-500/30",
  RESOLVED: "bg-slate-500/15 text-slate-300 ring-slate-500/30",
  OPEN: "bg-amber-500/15 text-amber-400 ring-amber-500/30",
};

export const SEVERITY_STYLES: Record<string, string> = {
  CRITICAL: "bg-red-500/15 text-red-400 ring-red-500/30",
  WARNING: "bg-amber-500/15 text-amber-400 ring-amber-500/30",
  INFO: "bg-blue-500/15 text-blue-400 ring-blue-500/30",
};

/**
 * TRANSFER: the color convention for the Portal's transfer axis (#130).
 *
 *   BLUE  = IN  → download, downloaded, leechers/peers, "arriving"
 *   GREEN = OUT    → upload, uploaded, seeds, seeding, "going out"
 *
 * It exists as a single constant because the rule was being repeated by hand in ~15 places
 * (tables, KPIs, Node cards, torrent detail) and the `Chart` was breaking it: it colored
 * the series by POSITION in the categorical palette, and since the Control Plane emits "Downloaded" before
 * "Uploaded", the chart came out with green download and blue upload — the inverse of the entire
 * rest of the screen. One place to change, and one place to check.
 *
 * `TRANSFER` provides the Tailwind classes (`strong` = large highlighted number), which swap
 * shades on their own via the vars in `index.css`. `TRANSFER_HEX` provides the SAME convention already
 * resolved in hex, for what doesn't go through CSS — the Recharts lines, which turn into
 * SVG attributes. That's why the hex needs the light variant by hand: on the white card the
 * green and blue from the dark theme lack contrast.
 */
export const TRANSFER = {
  down: { text: "text-blue-400", strong: "text-blue-300" },
  up: { text: "text-emerald-400", strong: "text-emerald-300" },
} as const;

export const TRANSFER_HEX = {
  dark: { down: "#3b82f6", up: "#22c55e" },
  light: { down: "#2563eb", up: "#047857" },
} as const;

/**
 * useTheme returns the current theme by reading the `data-theme` of `<html>` — the same attribute
 * that the `Layout` toggles and persists in `localStorage`.
 *
 * Only what isn't CSS needs this. The entire theme is made of CSS variables
 * (`index.css`), which swap on their own; but Recharts writes `stroke`/`fill` as
 * SVG ATTRIBUTES, and attributes don't resolve `var(--…)`. So the chart needs the color already
 * resolved in hex, and needs to re-render when the operator switches the theme.
 *
 * It observes via `MutationObserver` instead of a global context/state because the Layout
 * changes the attribute imperatively: this way any component can react to the change
 * without the Layout needing to know about it.
 */
export function useTheme(): "dark" | "light" {
  const [theme, setTheme] = useState<"dark" | "light">(() =>
    typeof document !== "undefined" && document.documentElement.dataset.theme === "light" ? "light" : "dark",
  );
  useEffect(() => {
    const el = document.documentElement;
    const sync = () => setTheme(el.dataset.theme === "light" ? "light" : "dark");
    const obs = new MutationObserver(sync);
    obs.observe(el, { attributes: true, attributeFilter: ["data-theme"] });
    sync(); // the theme may have been restored from localStorage before this mount
    return () => obs.disconnect();
  }, []);
  return theme;
}

// Torrent status attention order: what needs action first
// (error/verifying), then in progress, and lastly the stable ones. It serves to
// group the lists (Dashboard/Nodes) by status without scanning everything.
const TORRENT_STATUS_ORDER: Record<string, number> = {
  ERROR: 0,
  CHECKING: 1,
  MOVING: 2,
  DOWNLOADING: 3,
  QUEUED: 4,
  SEEDING: 5,
  PAUSED: 6,
};

// byTorrentStatus is a stable comparator (Array.prototype.sort is stable in
// ES2019+) that sorts torrents by the attention order above; unknown statuses
// go to the end. It preserves the original order within each group.
export function byTorrentStatus(a: { status: string }, b: { status: string }): number {
  const rank = (s: string) => TORRENT_STATUS_ORDER[s] ?? 99;
  return rank(a.status) - rank(b.status);
}

/**
 * SeedGrid logo (central CP connected to four Seed Nodes). It reuses the favicon FILE
 * (`public/favicon.svg`) instead of duplicating the inline SVG: single source — tweaking the tab
 * icon changes the UI logo along with it, with no chance of them diverging.
 *
 * `alt=""` + aria-hidden because in every use the name "SeedGrid" is written next to it — announcing
 * "SeedGrid" again in the screen reader would be noise. The shadow is `drop-shadow` (follows the alpha,
 * follows the SVG's own rounded corner) and not `shadow-*`, which would draw a rectangle.
 */
export function Logo({ className = "h-9 w-9" }: { className?: string }) {
  return (
    <img
      src="/favicon.svg"
      alt=""
      aria-hidden="true"
      className={`${className} shrink-0 drop-shadow-[0_4px_10px_rgba(6,78,59,0.45)]`}
    />
  );
}

// `label` picks the color in the map; `text`, when provided, is what is displayed (a translated label
// for a code that isn't interface text).
export function Badge({ label, map, text }: { label: string; map?: Record<string, string>; text?: string }) {
  const cls = (map ?? STATUS_STYLES)[label] ?? "bg-slate-500/15 text-slate-300 ring-slate-500/30";
  return (
    <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${cls}`}>
      {text ?? label}
    </span>
  );
}

// HealthBadge displays the Health Score (0-100) as a pill, following the same pattern as the
// status/storage Badge. Color by score: green ≥80, amber 50-79, red <50
// (failed storage zeroes dimensions → overall plunges to red). null = unknown.
export function HealthBadge({ score }: { score?: number | null }) {
  const t = useT();
  const cls =
    score == null
      ? "bg-slate-500/15 text-slate-300 ring-slate-500/30"
      : score >= 80
        ? "bg-emerald-500/15 text-emerald-400 ring-emerald-500/30"
        : score >= 50
          ? "bg-amber-500/15 text-amber-400 ring-amber-500/30"
          : "bg-red-500/15 text-red-400 ring-red-500/30";
  return (
    <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${cls}`}>
      {t("ui.ui.healthBadge.label")} {score ?? "—"}
    </span>
  );
}

/**
 * BackLink: the "back" button for detail pages. It uses the browser history, because a
 * torrent (or Node) is reachable from several places — Dashboard table, Downloaded screen,
 * rankings, Feed, Node detail, Explore — and a fixed link sent the operator to a
 * screen that wasn't where they came from.
 *
 * `fallback` covers the case with no history (tab opened directly on a deep link, or first
 * session navigation): then there's nowhere to go back to and the button becomes a normal link. It's
 * a real `<a>`, not a `<button>`, so the fallback opens in a new tab if the operator
 * wants — and the click is only intercepted when there is history to consume.
 */
export function BackLink({ fallback, label }: { fallback: string; label?: string }) {
  const t = useT();
  const hasHistory = typeof window !== "undefined" && window.history.length > 1;
  return (
    <a
      href={`#${fallback}`}
      onClick={(e) => {
        if (!hasHistory) return;
        e.preventDefault();
        window.history.back();
      }}
      className="text-sm text-emerald-400 hover:text-emerald-300"
    >
      ← {label ?? t("common.back")}
    </a>
  );
}

export function Panel({ title, right, children }: { title: ReactNode; right?: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-2xl border border-slate-800 bg-slate-900/60">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 px-4 py-3">
        <h2 className="text-sm font-semibold text-white">{title}</h2>
        {right}
      </div>
      <div className="overflow-x-auto">{children}</div>
    </section>
  );
}

// usedPct converts "how much is FREE" (what the heartbeat reports: `disk_free_bytes`,
// `mem_avail_bytes`) into the USED percentage, which is what the UI shows. 0 when the total didn't
// come through — an old Agent or incomplete heartbeat shouldn't paint a full red bar.
export function usedPct(free?: number, total?: number): number {
  if (!total || total <= 0) return 0;
  return Math.round(((total - (free ?? 0)) / total) * 100);
}

// Meter: horizontal usage bar (0-100%) with traffic light at 75/90 — the same scale for
// disk, memory, and I/O, so the operator doesn't have to relearn the color per metric.
// The `detail` (e.g., "free 1.2 TB out of 3.6 TB") comes from the caller; the `· N%` is added
// here so the number never disappears when the detail is long.
export function Meter({ label, used, detail }: { label: string; used: number; detail: string }) {
  const color = used >= 90 ? "bg-red-500" : used >= 75 ? "bg-amber-500" : "bg-emerald-500";
  return (
    <div>
      <div className="flex items-center justify-between text-xs text-slate-400">
        <span>{label}</span>
        <span className="text-slate-300">
          {detail} · {used}%
        </span>
      </div>
      <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-slate-800">
        <div className={`h-full rounded-full ${color}`} style={{ width: `${used}%` }} />
      </div>
    </div>
  );
}

// Stat: compact aggregated number card (used in Node grids). Label in small
// caps, highlighted value; `accent` colors the value by semantics (blue=download,
// emerald=upload/seeding, amber=ratio).
export function Stat({ label, value, accent }: { label: string; value: string; accent?: string }) {
  return (
    <div className="rounded-xl border border-slate-800 bg-slate-950/40 px-3 py-2">
      <p className="text-[10px] uppercase tracking-wide text-slate-500">{label}</p>
      <p className={`mt-0.5 text-sm font-semibold ${accent ?? "text-white"}`}>{value}</p>
    </div>
  );
}

export function fmtTime(s?: string): string {
  if (!s) return "—";
  return new Date(s).toLocaleString(getLang(), { dateStyle: "short", timeStyle: "medium" });
}

/**
 * fmtBytes formats a size in bytes into the largest unit that doesn't make the number absurd.
 *
 * It had a ceiling in GB, and in a multi-TB cluster this produced "5677.52 GB" instead of
 * "5.54 TB" — a number the operator has to divide in their head to understand. It showed up in the
 * Analytics panel, but applied to the entire Portal (library, Node cards, migration).
 *
 * PB exists for the same reason: the previous ceiling was only noticed when the library exceeded it.
 */
export function fmtBytes(n?: number): string {
  const b = n ?? 0;
  if (b >= 1024 ** 5) return `${(b / 1024 ** 5).toFixed(2)} PB`;
  if (b >= 1024 ** 4) return `${(b / 1024 ** 4).toFixed(2)} TB`;
  if (b >= 1024 ** 3) return `${(b / 1024 ** 3).toFixed(2)} GB`;
  if (b >= 1024 ** 2) return `${(b / 1024 ** 2).toFixed(0)} MB`;
  if (b >= 1024) return `${(b / 1024).toFixed(0)} KB`;
  return `${b} B`;
}

export function shortId(id?: string): string {
  return id ? id.slice(0, 8) : "—";
}

// fmtSpeed formats an instantaneous rate in bytes/s (e.g., "1.2 MB/s", "340 KB/s").
// "—" when there is no traffic (0), to avoid polluting with "0 B/s".
export function fmtSpeed(n?: number): string {
  const b = n ?? 0;
  if (b <= 0) return "—";
  if (b >= 1024 ** 2) return `${(b / 1024 ** 2).toFixed(1)} MB/s`;
  if (b >= 1024) return `${(b / 1024).toFixed(0)} KB/s`;
  return `${b} B/s`;
}

// fmtRatio formats an uploaded/downloaded ratio (#14). No download: "∞" when there was
// upload (pure seed), "—" when there was no traffic. This is the LOCAL ratio accumulated
// by the Cluster; the official ratio is the one from the tracker site.
export function fmtRatio(uploaded?: number, downloaded?: number): string {
  const up = uploaded ?? 0;
  const dl = downloaded ?? 0;
  if (dl <= 0) return up > 0 ? "∞" : "—";
  return (up / dl).toFixed(2);
}

// fmtUptime formats a duration in seconds as "3d 4h", "5h 12m" or "8m".
export function fmtUptime(seconds?: number): string {
  if (!seconds || seconds <= 0) return "—";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${Math.floor(seconds)}s`;
}

// fmtSince formats the elapsed time since an ISO instant (e.g., container start).
export function fmtSince(iso?: string): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  return fmtUptime((Date.now() - t) / 1000);
}
