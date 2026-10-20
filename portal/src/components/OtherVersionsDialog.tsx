import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import type { OtherVersions, TorrentVersion } from "../api";
import { fmtBytes } from "./ui";
import { rich, useT } from "../i18n";

/**
 * OtherVersionsDialog: the cluster already has another version of the same work (another
 * resolution, another codec, another release) and the operator decides whether to add this one too
 * (#37).
 *
 * It shows side by side what already exists — name, resolution, codec, size and Node — and what is
 * being added, because the decision depends on the comparison ("I have it in 1080p x264; this one is
 * 2160p x265"). An identical file doesn't come through here: it's "in cluster", and that's it.
 *
 * Cancel is the initial focus and the Esc key: the path that changes nothing is the one taken without
 * thinking.
 */
export function OtherVersionsDialog({
  title,
  info,
  onConfirm,
  onCancel,
}: {
  title: string;
  info: OtherVersions;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const t = useT();
  const cancelRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    cancelRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onCancel();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onCancel]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
      onClick={onCancel}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="other-versions-title"
        className="w-full max-w-xl rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3">
          <span
            aria-hidden="true"
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-amber-500/15 text-base text-amber-300 ring-1 ring-inset ring-amber-500/30"
          >
            {"⧉"}
          </span>
          <div className="min-w-0">
            <h2 id="other-versions-title" className="text-sm font-semibold text-white">
              {t("ui.otherVersions.title")}
            </h2>
            <p className="mt-0.5 text-xs text-slate-400">
              {rich(t("ui.otherVersions.summary", { count: info.versions.length }), {
                title: <span className="font-medium text-slate-200">«{title}»</span>,
              })}
            </p>
          </div>
        </div>

        <p className="mt-4 text-[10px] font-semibold uppercase tracking-wider text-slate-500">
          {t("ui.otherVersions.inCluster")}
        </p>
        <ul className="mt-1.5 max-h-60 space-y-1.5 overflow-y-auto">
          {info.versions.map((v, i) => (
            <li key={v.name + v.node + i}>
              <VersionRow v={v} />
            </li>
          ))}
        </ul>

        {info.adding && (
          <>
            <p className="mt-4 text-[10px] font-semibold uppercase tracking-wider text-emerald-400/80">
              {t("ui.otherVersions.adding")}
            </p>
            <div className="mt-1.5">
              <VersionRow v={info.adding} adding />
            </div>
          </>
        )}

        <div className="mt-5 flex gap-2">
          <button
            ref={cancelRef}
            type="button"
            onClick={onCancel}
            className="flex-1 rounded-full px-3 py-2 text-xs text-slate-300 transition hover:bg-slate-800"
          >
            {t("ui.otherVersions.cancel")}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className="flex-1 rounded-full bg-emerald-600 px-3 py-2 text-sm font-medium text-[#fff] transition hover:bg-emerald-500"
          >
            {t("ui.otherVersions.confirm")}
          </button>
        </div>
      </div>
    </div>
  );
}

// VersionRow: one version on one line — the full name in the tooltip, the data that decides in chips.
function VersionRow({ v, adding = false }: { v: TorrentVersion; adding?: boolean }) {
  const t = useT();
  return (
    <div
      className={
        "rounded-xl border px-3 py-2 " +
        (adding ? "border-emerald-500/30 bg-emerald-500/5" : "border-slate-800 bg-slate-950/40")
      }
    >
      <p className="truncate font-mono text-xs text-slate-200" title={v.name}>
        {v.name}
      </p>
      <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11px]">
        <Tag tone="bg-slate-800 text-slate-200 ring-slate-700">{v.resolution || t("ui.otherVersions.unknownResolution")}</Tag>
        <Tag tone="bg-indigo-500/10 text-indigo-300 ring-indigo-500/30">{v.codec || t("ui.otherVersions.unknownCodec")}</Tag>
        {v.size_bytes > 0 && <Tag tone="bg-slate-800 text-slate-300 ring-slate-700">{fmtBytes(v.size_bytes)}</Tag>}
        {v.node && (
          <span className="ml-auto text-slate-500">
            {rich(t("ui.otherVersions.onNode"), { node: <span className="text-slate-300">{v.node}</span> })}
          </span>
        )}
      </div>
    </div>
  );
}

function Tag({ tone, children }: { tone: string; children: ReactNode }) {
  return <span className={"rounded-full px-2 py-0.5 font-medium ring-1 ring-inset " + tone}>{children}</span>;
}

type Pending = { title: string; info: OtherVersions; resolve: (ok: boolean) => void };

/**
 * useOtherVersionsPrompt: asks the operator, with the dialog above, and returns the answer as a
 * Promise — the flow that calls it (a seed, or a batch of adds in sequence) waits for the decision in
 * the middle, as it waited for window.confirm, without the browser pop-up.
 */
export function useOtherVersionsPrompt(): [(title: string, info: OtherVersions) => Promise<boolean>, ReactNode] {
  const [pending, setPending] = useState<Pending | null>(null);
  const ask = useCallback(
    (title: string, info: OtherVersions) => new Promise<boolean>((resolve) => setPending({ title, info, resolve })),
    [],
  );
  const close = useCallback(
    (ok: boolean) => {
      pending?.resolve(ok);
      setPending(null);
    },
    [pending],
  );
  const onConfirm = useCallback(() => close(true), [close]);
  const onCancel = useCallback(() => close(false), [close]);
  const dialog = pending ? (
    <OtherVersionsDialog title={pending.title} info={pending.info} onConfirm={onConfirm} onCancel={onCancel} />
  ) : null;
  return [ask, dialog];
}
