import { useEffect, useState, useCallback, useMemo } from "react";
import { getAudit, ApiError, type AuditEntry } from "../api";
import { fmtTime } from "../components/ui";
import { getLang, useT, type TFn } from "../i18n";
import { messageText } from "../lib/messages";

// Audit trail (ADR-025): the operator's actions in friendly language — who did
// what, when, from where. Separated from Alerts (issues), Events (domain), and Feed
// (orchestration). The phrase comes from code and action data, in the reader's language (#57); the
// `action` recorded in Portuguese is the fallback. Method/path/IP go in the footer.
const REFRESH_MS = 8_000;

function fmtAgo(t: TFn, iso: string): string {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return t("audit.log.time.seconds", { count: s });
  if (s < 3600) return t("audit.log.time.minutes", { count: Math.floor(s / 60) });
  if (s < 86400) return t("audit.log.time.hours", { count: Math.floor(s / 3600) });
  return t("audit.log.time.days", { count: Math.floor(s / 86400) });
}

// icon: an icon based on the action's nature, route, and method — not by the action text,
// which is a phrase in a language (the Control Plane records it in Portuguese; ADR-041).
function icon(e: AuditEntry): string {
  const p = (e.path ?? "").replace(/^\/api\/v1\//, "");
  if (p.startsWith("auth/")) return "🔑";
  if (p.startsWith("settings") || p.endsWith("operation-mode")) return "⚙";
  if (e.method === "DELETE") return "🗑";
  if (p.endsWith("publish-tracker") || p.startsWith("trackers") || p.startsWith("publish")) return "📤";
  if (p.startsWith("torrents") || p.startsWith("migrations")) return "📦";
  if (p.startsWith("nodes") || p.startsWith("enroll-tokens")) return "🖥";
  return "•";
}

const failed = (code?: number) => code != null && code >= 400;

export function AuditPage({ onLogout }: { onLogout: () => void }) {
  const t = useT();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState("");
  const [actor, setActor] = useState("");

  const load = useCallback(async () => {
    try {
      const e = await getAudit(200);
      setEntries(e);
      setError("");
      setUpdatedAt(new Date().toLocaleTimeString(getLang()));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(t("audit.log.error.load"));
    }
  }, [onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  const actors = useMemo(() => Array.from(new Set(entries.map((e) => e.actor))).sort(), [entries]);
  const items = actor ? entries.filter((e) => e.actor === actor) : entries;

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold text-white">{t("audit.log.title")}</h1>
        <span className="text-xs text-slate-500">
          {updatedAt ? t("audit.log.updatedAt", { time: updatedAt }) : t("common.loading")}
        </span>
      </div>

      <p className="text-xs text-slate-500">{t("audit.log.description")}</p>

      {actors.length > 1 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <button type="button" onClick={() => setActor("")} aria-pressed={actor === ""} className="sg-chip">
            {t("audit.log.filter.all")}
          </button>
          {actors.map((a) => (
            <button key={a} type="button" onClick={() => setActor(a)} aria-pressed={actor === a} className="sg-chip">
              {a}
            </button>
          ))}
        </div>
      )}

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {error}
        </div>
      )}

      <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-5">
        <ol className="space-y-0">
          {items.map((e, i) => (
            <li key={e.id} className="relative flex gap-3 pb-5 last:pb-0">
              <div className="flex flex-col items-center">
                <span
                  className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-950 text-sm ring-1 ${
                    failed(e.status_code) ? "ring-red-500/40" : "ring-slate-700"
                  }`}
                >
                  {icon(e)}
                </span>
                {i < items.length - 1 && <span className="mt-1 w-px flex-1 bg-slate-800" />}
              </div>
              <div className="min-w-0 flex-1 pt-1">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="text-sm text-white">
                    <span className="font-semibold text-emerald-300">{e.actor}</span>{" "}
                    <span className="text-slate-200">
                      {e.action_code ? messageText("audit", e.action_code, e.action_params, e.action) : e.action}
                    </span>
                  </span>
                  {failed(e.status_code) && (
                    <span className="rounded-full bg-red-500/15 px-2 py-0.5 text-[11px] font-medium text-red-400 ring-1 ring-inset ring-red-500/30">
                      {e.status_code
                        ? t("audit.log.status.failedWithCode", { code: e.status_code })
                        : t("audit.log.status.failed")}
                    </span>
                  )}
                  <span className="ml-auto whitespace-nowrap text-xs text-slate-500" title={fmtTime(e.created_at)}>
                    {fmtAgo(t, e.created_at)}
                  </span>
                </div>
                <p className="mt-0.5 truncate font-mono text-[11px] text-slate-600">
                  {e.method ? `${e.method} ` : ""}
                  {e.path ?? ""}
                  {e.source_ip ? ` · ${e.source_ip}` : ""}
                </p>
              </div>
            </li>
          ))}
          {items.length === 0 && (
            <li className="py-10 text-center text-sm text-slate-500">
              {t("audit.log.empty")}
            </li>
          )}
        </ol>
      </div>
    </>
  );
}
