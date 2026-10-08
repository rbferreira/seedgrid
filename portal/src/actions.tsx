// Action toasts (bottom right corner). Each action (pause/resume/recheck/delete/
// migrate/drain/Engine restart/Agent update) queues a command on the Node and returns a
// command_id; here we track the progress (PENDING → DISPATCHED → DONE/FAILED) via
// GET /commands/{id} and display it in a notification that:
//  - confirms completion ("Done") or failure, and
//  - shows progress while running (useful for migration: Queued → Running → done).
// There is also a simple toast (toast()) for confirmations without a command (e.g., saved something).
//
// Module store (singleton) signed by useSyncExternalStore — any page stacks
// toasts in the same viewport, without a Provider at the top of the tree.
import { useEffect, useState, useSyncExternalStore } from "react";
import { getCommand, getMigrations, type CommandStatus } from "./api";
import { getLang, translate, useT, type TFn } from "./i18n";

// The action store is global, outside of components: the status text is translated into the active language
// at the time of the event. Components in this file have their own `t` (useT).
const t: TFn = (key, params) => translate(getLang(), key, params);

export type ToastKind = "action" | "success" | "error" | "info";

export type Toast = {
  key: number; // toast identity in the stack
  id: string; // target (torrent/node) — used by the right button's spinner; "" for simple toasts
  label: string; // friendly description ("Pause «Movie»")
  status: string; // humanized text ("Queued", "Migrating… 2/5", "Done")
  error?: string;
  terminal: boolean;
  failed?: boolean; // failure outcome (the status is text, can't infer from it)
  kind: ToastKind;
  startedAt: number;
};

let toasts: Toast[] = [];
const listeners = new Set<() => void>();
let seq = 0;

function emit() {
  for (const l of listeners) l();
}
function upsert(t: Toast) {
  const i = toasts.findIndex((x) => x.key === t.key);
  toasts = i >= 0 ? toasts.map((x) => (x.key === t.key ? t : x)) : [...toasts, t];
  emit();
}
function remove(key: number) {
  toasts = toasts.filter((t) => t.key !== key);
  emit();
}
function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

// useToasts returns the current stack and re-renders on every change.
export function useToasts(): Toast[] {
  return useSyncExternalStore(subscribe, () => toasts);
}

// useRunningId returns the set of targets with a NON-terminal action in progress — to
// disable/spin the right button. Set (not string) because now multiple actions, on different
// targets, can run at the same time.
export function useRunningId(): Set<string> {
  const ts = useToasts();
  const s = new Set<string>();
  for (const t of ts) if (t.kind === "action" && !t.terminal && t.id) s.add(t.id);
  return s;
}

const POLL_MS = 800;
const COMMAND_TIMEOUT_MS = 90_000;
const MIGRATION_TIMEOUT_MS = 45 * 60_000; // migration moves data between Nodes (+ ADR-031 queue) — can take a while; on timeout, NEUTRAL toast (not a failure)
const CLEAR_OK_MS = 4_000; // success disappears by itself
const CLEAR_ERR_MS = 9_000; // failures stay longer (the operator needs to see it)

type Base = { key: number; id: string; label: string; kind: "action" };
// Polling result, already humanized — decouples the toast from the status vocabulary
// (command: PENDING/DISPATCHED/DONE; migration: PENDING/COMPLETED/FAILED).
export type Probe = { statusText: string; terminal: boolean; failed: boolean; error?: string };

function scheduleClear(key: number, ms: number) {
  setTimeout(() => {
    const t = toasts.find((x) => x.key === key);
    if (t && t.terminal) remove(key); // don't overwrite a toast that turned into something else
  }, ms);
}

function fail(key: number, base: Base, error: string) {
  upsert({ ...base, status: t("actions.actions.failed"), error, terminal: true, failed: true, startedAt: Date.now() });
  scheduleClear(key, CLEAR_ERR_MS);
}

// toast fires a simple notification (no command), for immediate confirmations.
export function toast(message: string, kind: Exclude<ToastKind, "action"> = "success") {
  const key = ++seq;
  upsert({ key, id: "", label: message, status: "", error: undefined, terminal: true, kind, startedAt: Date.now() });
  scheduleClear(key, kind === "error" ? CLEAR_ERR_MS : CLEAR_OK_MS);
}

// pollLoop polls until the outcome and updates the toast at each step. probe() already returns
// humanized text; the loop just handles cadence, timeout, and cleanup. Shared by command and
// migration — the only difference between them is the probe.
//
// finalize=false: reached the end SUCCESSFULLY but the toast does NOT close — there is a 2nd phase
// ahead (see `confirm` in startAction). Failure closes in any case.
// onTimeout: outcome when the deadline expires. Default = failure "timed out". Migration
// passes a NEUTRAL outcome: the client deadline isn't a migration failure — it is durable
// on the server and can finish later (see startMigration).
// Returns true when it has finished successfully.
async function pollLoop(
  key: number,
  base: Base,
  probe: () => Promise<Probe>,
  timeoutMs: number,
  finalize = true,
  onTimeout?: () => void,
): Promise<boolean> {
  const timedOut = () => {
    if (onTimeout) onTimeout();
    else fail(key, base, t("actions.actions.timeout"));
    return false;
  };
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    let p: Probe;
    try {
      p = await probe();
    } catch {
      if (Date.now() > deadline) return timedOut();
      await sleep(POLL_MS);
      continue;
    }
    // In an intermediate phase the toast stays "alive": terminal only when there is no 2nd phase.
    const terminal = p.terminal && (finalize || p.failed);
    upsert({ ...base, status: p.statusText, error: p.error, terminal, failed: p.failed, startedAt: Date.now() });
    if (p.terminal) {
      if (terminal) scheduleClear(key, p.failed ? CLEAR_ERR_MS : CLEAR_OK_MS);
      return !p.failed;
    }
    if (Date.now() > deadline) return timedOut();
    await sleep(POLL_MS);
  }
}

function probeCommand(st: CommandStatus): Probe {
  const failed = st.status === "FAILED";
  const map: Record<string, string> = {
    PENDING: t("actions.actions.statusQueued"),
    DISPATCHED: t("actions.actions.statusDispatched"),
    DONE: t("actions.actions.statusDone"),
    FAILED: t("actions.actions.failed"),
  };
  return { statusText: map[st.status] ?? st.status, terminal: failed || st.status === "DONE", failed, error: st.error ?? undefined };
}

// startAction triggers the action, tracks the COMMAND to its outcome in a toast, and calls
// onSettled (the page reloads its data). Locks by TARGET: ignores a new trigger for the
// same id while the previous one runs (prevents double-click); different targets run together.
//
// `confirm` (optional) adds a 2nd PHASE: the command returning DONE doesn't always mean
// the EFFECT happened. In the Agent update (ADR-027) `DONE` is returned when the binary is
// swapped — the process still needs to restart. Saying "Done" there is a lie: the operator saw
// the old version on the screen and clicked again. With `confirm`, the toast only closes (and the button only
// unlocks) when the effect is actually observed.
export async function startAction(opts: {
  id: string;
  label: string;
  run: () => Promise<string>; // returns the command_id
  confirm?: { probe: () => Promise<Probe>; timeoutMs: number };
  onSettled?: () => void | Promise<void>;
}): Promise<void> {
  if (busyFor(opts.id)) return;
  const key = ++seq;
  const base: Base = { key, id: opts.id, label: opts.label, kind: "action" };
  upsert({ ...base, status: t("actions.actions.statusQueued"), terminal: false, startedAt: Date.now() });
  let cmdID: string;
  try {
    cmdID = await opts.run();
  } catch (err) {
    fail(key, base, err instanceof Error ? err.message : t("actions.actions.sendFailed"));
    await opts.onSettled?.();
    return;
  }
  const ok = await pollLoop(
    key,
    base,
    async () => probeCommand(await getCommand(cmdID)),
    COMMAND_TIMEOUT_MS,
    !opts.confirm,
  );
  if (ok && opts.confirm) {
    await pollLoop(key, base, opts.confirm.probe, opts.confirm.timeoutMs);
  }
  await opts.onSettled?.();
}

// startMigration tracks a MIGRATION (ADR-018) — which is not a command: run() returns the
// ids of the created migrations (1 on torrent migrate; N on drain), and we poll GET
// /migrations until they all finish. The toast shows the progress (e.g., "Migrating… 2/5").
export async function startMigration(opts: {
  id: string;
  label: string;
  run: () => Promise<string[]>; // returns the migration ids to track
  onSettled?: () => void | Promise<void>;
}): Promise<void> {
  if (busyFor(opts.id)) return;
  const key = ++seq;
  const base: Base = { key, id: opts.id, label: opts.label, kind: "action" };
  upsert({ ...base, status: t("actions.actions.starting"), terminal: false, startedAt: Date.now() });
  let ids: string[];
  try {
    ids = await opts.run();
  } catch (err) {
    fail(key, base, err instanceof Error ? err.message : t("actions.actions.migrationStartFailed"));
    await opts.onSettled?.();
    return;
  }
  if (ids.length === 0) {
    upsert({ ...base, status: t("actions.actions.nothingToMigrate"), terminal: true, startedAt: Date.now() });
    scheduleClear(key, CLEAR_OK_MS);
    await opts.onSettled?.();
    return;
  }
  const want = new Set(ids);
  await pollLoop(
    key,
    base,
    async () => probeMigrations(await getMigrations(80), want),
    MIGRATION_TIMEOUT_MS,
    true,
    // Client timeout ≠ migration failure: it continues on the server (queue + large
    // transfer can exceed the deadline) and the reconciler completes it. NEUTRAL toast, not red,
    // pointing to where to see the real outcome. (Bug: migration finished but the toast said
    // "Failed / timed out".)
    () => {
      upsert({
        ...base,
        status: t("actions.actions.stillInProgress"),
        terminal: true,
        failed: false,
        startedAt: Date.now(),
      });
      scheduleClear(key, CLEAR_ERR_MS); // stays a bit longer for the operator to read
    },
  );
  await opts.onSettled?.();
}

function probeMigrations(all: { id: string; status: string; error?: string }[], want: Set<string>): Probe {
  const total = want.size;
  const mine = all.filter((m) => want.has(m.id));
  const failed = mine.filter((m) => m.status === "FAILED");
  const done = mine.filter((m) => m.status === "COMPLETED").length;
  if (failed.length > 0) {
    return { statusText: total > 1 ? t("actions.actions.migrationFailedCount", { count: failed.length, total }) : t("actions.actions.failed"), terminal: true, failed: true, error: failed[0].error };
  }
  if (mine.length === total && done === total) {
    return { statusText: total > 1 ? t("actions.actions.migrationsCompleted", { count: total }) : t("actions.actions.migrationCompleted"), terminal: true, failed: false };
  }
  // Migrations now wait for a slot under migration.max_concurrent (ADR-031). Saying
  // "Migrating…" while the migration is only in the QUEUE is lying to the operator — they
  // would be waiting for bytes that haven't started moving yet.
  const running = mine.filter((m) => m.status === "PENDING").length;
  if (running === 0 && mine.some((m) => m.status === "QUEUED")) {
    return { statusText: total > 1 ? t("actions.actions.migrationQueuedCount", { done, total }) : t("actions.actions.migrationQueued"), terminal: false, failed: false };
  }
  return { statusText: total > 1 ? t("actions.actions.migratingCount", { done, total }) : t("actions.actions.migrating"), terminal: false, failed: false };
}

function busyFor(id: string): boolean {
  return toasts.some((t) => t.kind === "action" && t.id === id && !t.terminal);
}

function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms));
}

// Inline spinner (no dependencies): spinning ring.
export function Spinner({ className = "" }: { className?: string }) {
  const t = useT();
  return (
    <span
      role="status"
      aria-label={t("actions.actions.processing")}
      className={`inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-slate-600 border-t-emerald-400 ${className}`}
    />
  );
}

// ToastCard: a notification. Slides in on mount; icon by state; dismissible.
function ToastCard({ t }: { t: Toast }) {
  const tr = useT();
  const [shown, setShown] = useState(false);
  useEffect(() => {
    const r = requestAnimationFrame(() => setShown(true));
    return () => cancelAnimationFrame(r);
  }, []);

  const failed = !!t.failed || t.kind === "error";
  const ok = t.terminal && !failed;
  const accent = failed
    ? "border-red-600/40 bg-[var(--sg-toast-error-bg)]"
    : ok
      ? "border-emerald-600/40 bg-[var(--sg-toast-ok-bg)]"
      : "border-slate-700 bg-slate-900/90";

  // Subtitle: already humanized status of actions; nothing on simple toasts.
  const sub = t.kind === "action" ? t.status : "";

  return (
    <div
      role="status"
      className={`pointer-events-auto flex w-96 max-w-[calc(100vw-2rem)] items-start gap-3 rounded-xl border ${accent} px-4 py-3 shadow-lg shadow-black/30 backdrop-blur transition-all duration-200 ${
        shown ? "translate-x-0 opacity-100" : "translate-x-4 opacity-0"
      }`}
    >
      <span className="mt-0.5 shrink-0">
        {!t.terminal ? (
          <Spinner className="h-4 w-4" />
        ) : failed ? (
          <span className="text-red-400">✕</span>
        ) : (
          <span className="text-emerald-400">✓</span>
        )}
      </span>
      <div className="min-w-0 flex-1">
        {/* label is just a short TITLE on action toasts (the detail goes on the line below);
            on simple toasts (toast()) it is the entire message — cannot truncate, otherwise it cuts off
            the error reason (#bug: unreadable error toast). */}
        <p className={`text-sm font-medium text-white ${t.kind === "action" ? "truncate" : ""}`}>{t.label}</p>
        {(sub || t.error) && (
          <p className={`mt-0.5 text-xs ${failed ? "text-red-400" : "text-slate-400"}`}>
            {sub}
            {t.error ? `${sub ? " — " : ""}${t.error}` : ""}
          </p>
        )}
      </div>
      <button
        onClick={() => remove(t.key)}
        aria-label={tr("common.close")}
        className="shrink-0 text-slate-500 transition hover:text-slate-300"
      >
        ✕
      </button>
    </div>
  );
}

// ToastViewport: fixed stack in the bottom right corner. Mount ONCE, at the top of the app.
// Shows the most recent at the bottom; limits to 4 visible so it doesn't cover the screen.
export function ToastViewport() {
  const ts = useToasts();
  const visible = ts.slice(-4);
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex flex-col gap-2">
      {visible.map((t) => (
        <ToastCard key={t.key} t={t} />
      ))}
    </div>
  );
}
