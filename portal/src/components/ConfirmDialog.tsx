import { useEffect, useRef, useSyncExternalStore, type ReactNode } from "react";
import { useT } from "../i18n";

/**
 * Action confirmation in SeedGrid's look, in place of `window.confirm` (#45).
 *
 * `askConfirm` returns a Promise, so a caller that did `if (!window.confirm(…)) return` moves to
 * `if (!(await askConfirm(…))) return` without changing its flow. The state is a module store, like
 * `toast()`'s: `<ConfirmHost/>` is mounted once in the App and any screen asks without passing a
 * dialog through props.
 *
 * Cancel is the initial focus, the Esc key and the click outside: the path that changes nothing is the
 * one taken without thinking (the same as OtherVersionsDialog). In the browser pop-up, OK had the focus.
 */
export type ConfirmOptions = {
  title: string;
  /** Detail of the consequence. Text with `\n` breaks the line, as in `window.confirm`. */
  message?: ReactNode;
  /** The action's verb ("Delete", "Restart") — never "OK". */
  confirmLabel: string;
  /** Label of the button that does nothing. Change it when the action itself is a "cancel". */
  cancelLabel?: string;
  /** `danger` for what deletes data or removes access: a red button. */
  tone?: "default" | "danger";
};

export type PendingConfirm = ConfirmOptions & { key: number; resolve: (ok: boolean) => void };

// A queue, not a slot: two questions in a row (the 2nd born from the 1st's answer, or from another
// screen) can't overwrite each other and leave a Promise unanswered forever.
let queue: PendingConfirm[] = [];
const listeners = new Set<() => void>();
let seq = 0;

function emit() {
  for (const l of listeners) l();
}
function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

export function askConfirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    queue = [...queue, { ...opts, key: ++seq, resolve }];
    emit();
  });
}

/** The current question, or null. */
export function currentConfirm(): PendingConfirm | null {
  return queue[0] ?? null;
}

/** Answers question `key` and moves on to the next in the queue. A stale key does nothing. */
export function answerConfirm(key: number, ok: boolean) {
  const p = queue.find((x) => x.key === key);
  if (!p) return;
  queue = queue.filter((x) => x.key !== key);
  emit();
  p.resolve(ok);
}

/** Mounted once in the App: shows the current question. */
export function ConfirmHost() {
  const p = useSyncExternalStore(subscribe, currentConfirm, currentConfirm);
  if (!p) return null;
  // key: a new question remounts the dialog, and the focus goes back to Cancel.
  return (
    <ConfirmDialog
      key={p.key}
      title={p.title}
      message={p.message}
      confirmLabel={p.confirmLabel}
      cancelLabel={p.cancelLabel}
      tone={p.tone}
      onConfirm={() => answerConfirm(p.key, true)}
      onCancel={() => answerConfirm(p.key, false)}
    />
  );
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel,
  cancelLabel,
  tone = "default",
  onConfirm,
  onCancel,
}: ConfirmOptions & { onConfirm: () => void; onCancel: () => void }) {
  const t = useT();
  const cancelRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    cancelRef.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onCancel();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onCancel]);

  const danger = tone === "danger";
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
      onClick={onCancel}
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
        aria-describedby={message ? "confirm-message" : undefined}
        className="w-full max-w-md rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3">
          <span
            aria-hidden="true"
            className={
              "flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-base ring-1 ring-inset " +
              (danger ? "bg-red-500/15 text-red-300 ring-red-500/30" : "bg-sky-500/15 text-sky-300 ring-sky-500/30")
            }
          >
            {danger ? "!" : "?"}
          </span>
          <div className="min-w-0">
            <h2 id="confirm-title" className="break-words text-sm font-semibold text-white">
              {title}
            </h2>
            {message && (
              <div id="confirm-message" className="mt-1 whitespace-pre-line break-words text-xs leading-relaxed text-slate-400">
                {message}
              </div>
            )}
          </div>
        </div>

        <div className="mt-5 flex gap-2">
          <button
            ref={cancelRef}
            type="button"
            onClick={onCancel}
            className="flex-1 rounded-full px-3 py-2 text-xs text-slate-300 transition hover:bg-slate-800"
          >
            {cancelLabel ?? t("common.cancel")}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className={
              "flex-1 rounded-full px-3 py-2 text-sm font-medium text-[#fff] transition " +
              (danger ? "bg-red-600 hover:bg-red-500" : "bg-emerald-600 hover:bg-emerald-500")
            }
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
