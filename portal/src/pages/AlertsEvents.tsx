import { useEffect, useState, useCallback, type ReactNode } from "react";
import { PillSwitch } from "../components/PillSwitch";
import {
  getNodes,
  getAlerts,
  getEvents,
  getMigrations,
  cancelMigration,
  ApiError,
  type Node,
  type Alert,
  type Event,
  type Migration,
} from "../api";
import { fmtTime, shortId } from "../components/ui";
import { currentBtEndpoint } from "../lib/alert-address";
import { nodeLabelById } from "../lib/node-label";
import { toast } from "../actions";
import { canCancelMigration, cancelMigrationPrompt, migrationStatusLabel } from "../lib/migrations";
import { messageText } from "../lib/messages";
import { getLang, useT, type Key, type TFn } from "../i18n";
import { askConfirm } from "../components/ConfirmDialog";

const REFRESH_MS = 10_000;
const SEVERITIES = ["ALL", "CRITICAL", "WARNING", "INFO"] as const;

// Meta by severity — same visual pattern as Feed/Audit (circle + colored ring).
const SEV: Record<string, { icon: string; ring: string }> = {
  CRITICAL: { icon: "⛔", ring: "ring-red-500/40" },
  WARNING: { icon: "⚠", ring: "ring-amber-500/40" },
  INFO: { icon: "ℹ", ring: "ring-sky-500/40" },
};
const SEV_FALLBACK = { icon: "•", ring: "ring-slate-600" };
const sevMeta = (s?: string) => (s && SEV[s]) || SEV_FALLBACK;

function fmtAgo(t: TFn, iso: string): string {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return t("alerts.alertsEvents.timeAgo.seconds", { count: s });
  if (s < 3600) return t("alerts.alertsEvents.timeAgo.minutes", { count: Math.floor(s / 60) });
  if (s < 86400) return t("alerts.alertsEvents.timeAgo.hours", { count: Math.floor(s / 3600) });
  return t("alerts.alertsEvents.timeAgo.days", { count: Math.floor(s / 86400) });
}

// TimelineItem: the default item for tracks (Feed/Audit/Alerts/Events).
function TimelineItem({
  icon,
  ring,
  primary,
  tags,
  when,
  meta,
  last,
}: {
  icon: string;
  ring: string;
  primary: ReactNode;
  tags?: ReactNode;
  when: string;
  meta?: string;
  last: boolean;
}) {
  const t = useT();
  return (
    <li className="relative flex gap-3 pb-5 last:pb-0">
      <div className="flex flex-col items-center">
        <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-950 text-sm ring-1 ${ring}`}>
          {icon}
        </span>
        {!last && <span className="mt-1 w-px flex-1 bg-slate-800" />}
      </div>
      <div className="min-w-0 flex-1 pt-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm text-slate-200">{primary}</span>
          {tags}
          <span className="ml-auto whitespace-nowrap text-xs text-slate-500" title={fmtTime(when)}>
            {fmtAgo(t, when)}
          </span>
        </div>
        {meta && <p className="mt-0.5 truncate font-mono text-[11px] text-slate-600">{meta}</p>}
      </div>
    </li>
  );
}

// CancelMigrationButton: the button for an in-flight migration. Exported for the button gate test
// (docs/11 - Portal.md §13.1): only shows with `torrents:migrate` — the permission of the route
// it calls — and only for migrations that can still be canceled. The migration DATA (route,
// state, reason) doesn't pass here: the line shows it to readers, with or without the button.
export function CancelMigrationButton({
  migration,
  canMigrate,
  busy,
  onCancel,
}: {
  migration: Migration;
  canMigrate: boolean;
  busy: boolean;
  onCancel: () => void;
}) {
  const t = useT();
  if (!canMigrate || !canCancelMigration(migration)) return null;
  return (
    <button
      type="button"
      onClick={onCancel}
      disabled={busy}
      title={t("alerts.alertsEvents.cancelMigration.title")}
      className="rounded-full border border-red-500/40 px-2 py-0.5 text-[11px] text-red-300 transition hover:bg-red-500/10 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {busy ? t("alerts.alertsEvents.cancelMigration.cancelling") : t("alerts.alertsEvents.cancelMigration.label")}
    </button>
  );
}

function TrailPanel({ titleKey, count, children }: { titleKey: Key; count: number; children: ReactNode }) {
  const t = useT();
  return (
    <div>
      <h2 className="mb-2 text-sm font-semibold text-white">
        {t(titleKey)} <span className="text-slate-500">({count})</span>
      </h2>
      <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-5">
        <ol className="space-y-0">{children}</ol>
      </div>
    </div>
  );
}

const chip = (text: string, cls: string) => (
  <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset ${cls}`}>{text}</span>
);

export function AlertsEvents({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [nodes, setNodes] = useState<Node[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [events, setEvents] = useState<Event[]>([]);
  const [migrations, setMigrations] = useState<Migration[]>([]);
  const [cancelling, setCancelling] = useState<string | null>(null); // id of the migration being canceled
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState("");
  const [openOnly, setOpenOnly] = useState(false);
  const [sev, setSev] = useState<(typeof SEVERITIES)[number]>("ALL");

  const load = useCallback(async () => {
    try {
      const [n, a, e, m] = await Promise.all([getNodes(), getAlerts(false), getEvents(100), getMigrations(20)]);
      setNodes(n);
      setAlerts(a);
      setEvents(e);
      setMigrations(m);
      setError("");
      setUpdatedAt(new Date().toLocaleTimeString(getLang()));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("alerts.alertsEvents.error.load"));
    }
  }, [onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const nodeName = (id?: string) => (id ? nodeLabelById(nodes, id) || shortId(id) : "");
  const bySev = <T extends { severity: string }>(rows: T[]) =>
    sev === "ALL" ? rows : rows.filter((r) => r.severity === sev);
  const shownAlerts = bySev(alerts).filter((a) => (openOnly ? a.status === "OPEN" : true));
  const shownEvents = bySev(events);

  // Cancel discards the destination copy, so it asks for confirmation. The button only shows with
  // torrents:migrate (the same permission to migrate) — the migration DATA remains visible to
  // everyone; the backend blocks it anyway (button gate, #97).
  const cancelMig = async (mg: Migration) => {
    if (!(await askConfirm(cancelMigrationPrompt(t, nodeName(mg.source_node_id), nodeName(mg.dest_node_id), mg.status)))) return;
    setCancelling(mg.id);
    try {
      await cancelMigration(mg.id);
      toast(t("alerts.alertsEvents.toast.migrationCancelled"));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      toast(err instanceof ApiError ? err.message : t("alerts.alertsEvents.error.cancelMigration"), "error");
    } finally {
      setCancelling(null);
      await load();
    }
  };

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold text-white">{t("alerts.alertsEvents.pageTitle")}</h1>
        <span className="text-xs text-slate-500">{updatedAt ? t("alerts.alertsEvents.updatedAt", { time: updatedAt }) : t("common.loading")}</span>
      </div>

      <p className="text-xs text-slate-500">
        {t("alerts.alertsEvents.description")}
      </p>

      <div className="flex flex-wrap items-center gap-2">
        <PillSwitch
          label={t("alerts.alertsEvents.filter.severity")}
          value={sev}
          onChange={setSev}
          options={SEVERITIES.map((s) => ({ value: s, content: s === "ALL" ? t("alerts.alertsEvents.filter.all") : s }))}
        />
        <label className="flex cursor-pointer items-center gap-2 rounded-full border border-slate-700 bg-slate-950 px-3 py-1 text-xs text-slate-300">
          <input
            type="checkbox"
            checked={openOnly}
            onChange={(e) => setOpenOnly(e.target.checked)}
            className="sg-check"
          />
          {t("alerts.alertsEvents.filter.openOnly")}
        </label>
      </div>

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <TrailPanel titleKey="alerts.alertsEvents.panel.alerts" count={shownAlerts.length}>
          {shownAlerts.map((a, i) => {
            const m = sevMeta(a.severity);
            const open = a.status === "OPEN";
            // The alert text doesn't include the address (#203: it was getting stale); the current one comes from the Node.
            const endpoint = currentBtEndpoint(a, nodes);
            return (
              <TimelineItem
                key={a.id}
                icon={m.icon}
                ring={open ? m.ring : "ring-slate-700"}
                primary={messageText("alert", a.code, a.payload, a.message)}
                tags={
                  <>
                    {chip(
                      open ? t("alerts.alertsEvents.status.open") : a.expired ? t("alerts.alertsEvents.status.expired") : t("alerts.alertsEvents.status.resolved"),
                      open
                        ? "bg-red-500/15 text-red-400 ring-red-500/30"
                        : "bg-slate-800 text-slate-400 ring-slate-700",
                    )}
                    {a.node_id && chip(nodeName(a.node_id), "bg-slate-800 text-slate-300 ring-slate-700")}
                    {endpoint && (
                      <span
                        title={t("alerts.alertsEvents.endpoint.title")}
                        className="rounded-full bg-slate-800 px-2 py-0.5 font-mono text-[11px] text-slate-300 ring-1 ring-inset ring-slate-700"
                      >
                        {endpoint}
                      </span>
                    )}
                  </>
                }
                when={a.opened_at}
                meta={a.code}
                last={i === shownAlerts.length - 1}
              />
            );
          })}
          {shownAlerts.length === 0 && <li className="py-8 text-center text-sm text-slate-500">{t("alerts.alertsEvents.empty.alerts")}</li>}
        </TrailPanel>

        <TrailPanel titleKey="alerts.alertsEvents.panel.events" count={shownEvents.length}>
          {shownEvents.map((e, i) => {
            const m = sevMeta(e.severity);
            return (
              <TimelineItem
                key={e.id}
                icon={m.icon}
                ring={m.ring}
                primary={messageText("event", e.code, e.payload, e.message)}
                tags={e.node_id ? chip(nodeName(e.node_id), "bg-slate-800 text-slate-300 ring-slate-700") : undefined}
                when={e.created_at}
                meta={e.code}
                last={i === shownEvents.length - 1}
              />
            );
          })}
          {shownEvents.length === 0 && <li className="py-8 text-center text-sm text-slate-500">{t("alerts.alertsEvents.empty.events")}</li>}
        </TrailPanel>
      </div>

      <TrailPanel titleKey="alerts.alertsEvents.panel.migrations" count={migrations.length}>
        {migrations.map((mg, i) => (
          <TimelineItem
            key={mg.id}
            icon="🚚"
            ring={mg.cancelled ? "ring-slate-600" : mg.error ? "ring-red-500/40" : "ring-emerald-500/40"}
            primary={
              <span>
                {nodeName(mg.source_node_id)} <span className="text-slate-500">→</span> {nodeName(mg.dest_node_id)}
                {mg.error && (
                  <span className={`ml-2 ${mg.cancelled ? "text-slate-500" : "text-red-400"}`}>· {mg.error}</span>
                )}
              </span>
            }
            tags={
              <>
                {chip(migrationStatusLabel(t, mg), "bg-slate-800 text-slate-300 ring-slate-700")}
                <CancelMigrationButton
                  migration={mg}
                  canMigrate={can("torrents:migrate")}
                  busy={cancelling === mg.id}
                  onCancel={() => cancelMig(mg)}
                />
              </>
            }
            when={mg.created_at}
            last={i === migrations.length - 1}
          />
        ))}
        {migrations.length === 0 && <li className="py-8 text-center text-sm text-slate-500">{t("alerts.alertsEvents.empty.migrations")}</li>}
      </TrailPanel>
    </>
  );
}
