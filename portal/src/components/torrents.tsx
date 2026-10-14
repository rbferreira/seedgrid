import type { ReactNode } from "react";
import {
  ApiError,
  downloadTorrentContent,
  prioritizeTorrent,
  removeTorrent,
  unprioritizeTorrent,
  type Torrent,
} from "../api";
import { startAction, toast, useRunningId } from "../actions";
import { href } from "../router";
import { fmtSpeed, shortId, TRANSFER } from "./ui";
import { useT } from "../i18n";

/**
 * isDone answers "is this torrent already in the collection?" — today, seeding.
 *
 * It is the predicate that splits the Portal's two lists: the **Downloaded** screen shows
 * `isDone`, the **Dashboard** shows `!isDone` (queue, downloading, paused, recheck,
 * errors). It exists as a single function, and the Dashboard uses the COMPLEMENT instead of
 * an allow-list of statuses, to guarantee the property that matters: every torrent
 * appears in exactly one of the two screens. With an allow-list, a new Agent status
 * (`STALLED`, `MOVING` from a future flow) wouldn't fall into any and the torrent would become
 * invisible in the Portal — a silent failure, the worst kind here.
 */
export function isDone(t: Torrent): boolean {
  return t.status === "SEEDING";
}

/**
 * TorrentLink: torrent name as a link to the detail view. Marks `⚡` + amber when
 * prioritized in the queue (F3) and always carries the full name in `title` — the text is
 * truncated by width, so without the tooltip a long release becomes unreadable.
 * `clamp={false}` for narrow tables that prefer wrapping over truncating.
 */
export function TorrentLink({ t, clamp = true }: { t: Torrent; clamp?: boolean }) {
  const tt = useT();
  const displayName = t.name ?? shortId(t.hash);
  return (
    <a
      href={href(`/torrents/${t.id}`)}
      title={t.prioritized_at ? tt("torrents.torrents.link.titlePrioritized", { name: t.name ?? t.hash ?? "" }) : (t.name ?? t.hash)}
      className={`${clamp ? "block max-w-[180px] truncate sm:max-w-[220px] lg:max-w-[260px] 2xl:max-w-[520px]" : ""} ${
        t.prioritized_at ? "text-amber-300 hover:text-amber-200" : "text-white hover:text-emerald-300"
      }`}
    >
      {t.prioritized_at ? "⚡ " : ""}
      {displayName}
    </a>
  );
}

// ProgressBar: thin bar + percentage, for the Progress column.
export function ProgressBar({ value }: { value?: number }) {
  const pct = Math.round((value ?? 0) * 100);
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 w-16 overflow-hidden rounded-full bg-slate-800">
        <div className="h-full rounded-full bg-emerald-500" style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs text-slate-400">{pct}%</span>
    </div>
  );
}

// SpeedPair: instant download/upload rates side by side, in the Portal's color convention
// (`TRANSFER`): blue = inbound, green = outbound.
export function SpeedPair({ dl, up }: { dl?: number; up?: number }) {
  const t = useT();
  return (
    <>
      <span className={TRANSFER.down.text} title={t("torrents.torrents.speed.download")}>↓ {fmtSpeed(dl)}</span>
      <span className="mx-1 text-slate-600">·</span>
      <span className={TRANSFER.up.text} title={t("torrents.torrents.speed.upload")}>↑ {fmtSpeed(up)}</span>
    </>
  );
}

/**
 * SeedsPeers: swarm (tracker scrape) and, with `showConnected`, the peers currently
 * connected to the Engine in parentheses. The scrape might come 0/stale while the
 * torrent is downloading from a connected peer — showing both avoids the misreading of
 * "0 seeds but it's downloading".
 *
 * Colors by `TRANSFER`: seed is who SENDS (green), leecher is who DOWNLOADS (blue) — the same
 * reading as `SpeedPair`, which usually sits in the adjacent column.
 */
export function SeedsPeers({ t, showConnected = true }: { t: Torrent; showConnected?: boolean }) {
  const tt = useT();
  return (
    <span
      title={
        showConnected
          ? tt("torrents.torrents.seedsPeers.titleConnected")
          : tt("torrents.torrents.seedsPeers.title")
      }
    >
      <span className={TRANSFER.up.text}>{t.num_seeds ?? 0}</span>
      {" / "}
      <span className={TRANSFER.down.text}>{t.num_leechers ?? 0}</span>
      {showConnected && (
        <span className="ml-1 text-xs text-slate-500">
          ({t.num_seeds_connected ?? 0}/{t.num_leechers_connected ?? 0})
        </span>
      )}
    </span>
  );
}

// Colors by action semantics — the same across every Portal torrent table, so
// the operator can recognize the button by color before reading the icon.
const TONES: Record<string, string> = {
  blue: TRANSFER.down.text, // download content — blue, like every Portal download
  emerald: "text-emerald-400", // resume
  amber: "text-amber-400", // pause
  red: "text-red-400", // delete / recheck torrent in ERROR
  neutral: "text-slate-400", // recheck out of ERROR
  slate: "text-slate-500", // priority off
  gold: "text-amber-300", // priority on
};

export function ActionBtn({
  tone,
  title,
  onClick,
  disabled,
  children,
}: {
  tone: keyof typeof TONES | string;
  title: string;
  onClick: () => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`rounded-full px-2 py-1 text-xs hover:bg-slate-800 disabled:opacity-40 ${TONES[tone] ?? TONES.slate}`}
    >
      {children}
    </button>
  );
}

/**
 * useTorrentActions centralizes the torrent actions that were literally copied in
 * Dashboard/Nodes/NodeDetail — including the confirmation messages, which need to be
 * the same across every screen (the operator confirms a `window.confirm`, not a page).
 *
 * - `act` wraps the action in a tracked command (`startAction`): inline spinner,
 *   progress toast, and reload on finish.
 * - `onTogglePriority` DOES NOT use `act` on purpose: prioritizing is Control Plane state,
 *   it doesn't queue a command on the Node, so there is no command_id to track.
 * - `onDownload` also doesn't generate a command: it's a one-time ticket + browser download.
 */
export function useTorrentActions({ reload, onLogout }: { reload: () => void; onLogout?: () => void }) {
  const t = useT();
  const running = useRunningId();

  const act = (tr: Torrent, verb: string, fn: (id: string) => Promise<string>) =>
    startAction({
      id: tr.id,
      label: t("torrents.torrents.actions.label", { verb, name: tr.name ?? shortId(tr.hash) }),
      run: () => fn(tr.id),
      onSettled: reload,
    });

  const onRemove = (tr: Torrent) => {
    if (window.confirm(t("torrents.torrents.actions.confirmRemove", { name: tr.name ?? tr.hash ?? "" }))) {
      act(tr, t("torrents.torrents.actions.verbRemove"), removeTorrent);
    }
  };

  const onTogglePriority = async (tr: Torrent) => {
    const on = !tr.prioritized_at;
    const label = tr.name ?? shortId(tr.hash);
    try {
      await (on ? prioritizeTorrent : unprioritizeTorrent)(tr.id);
      toast(on ? t("torrents.torrents.actions.toastPrioritized", { name: label }) : t("torrents.torrents.actions.toastUnprioritized", { name: label }));
      reload();
    } catch (err) {
      toast(err instanceof Error ? err.message : t("torrents.torrents.actions.errorPriority"), "error");
    }
  };

  const onDownload = async (tr: Torrent) => {
    try {
      await downloadTorrentContent(tr.id);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) onLogout?.();
      else toast(err instanceof ApiError ? err.message : t("torrents.torrents.actions.errorDownload"), "error");
    }
  };

  return { running, act, onRemove, onTogglePriority, onDownload };
}
