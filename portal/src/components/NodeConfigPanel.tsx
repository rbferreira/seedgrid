import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  getNodeSettings,
  resetNodeSetting,
  setNodeSetting,
  type NodeSettingItem,
  type NodeSettings,
} from "../api";
import { toast } from "../actions";
import { Panel, fmtTime } from "./ui";
import { checkConfigValue, configStatusMeta, fmtConfigValue } from "../lib/node-config";
import { rich, useT } from "../i18n";
import { settingText } from "../lib/setting-text";

const REFRESH_MS = 10_000;

// Who edits a Node's value: the SAME permission the backend requires on PUT/DELETE
// /nodes/{id}/settings/{key}. Dedicated function so the test can check which permission is requested.
export const canWriteNodeConfig = (can: (perm: string) => boolean): boolean => can("nodes:write");

const inputCls =
  "rounded-lg border border-slate-700 bg-slate-950/60 px-2.5 py-1 text-sm text-slate-100 outline-none transition focus:border-emerald-500 disabled:opacity-50";

const TONE: Record<"ok" | "warn" | "muted", string> = {
  ok: "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30",
  warn: "bg-amber-500/15 text-amber-300 ring-amber-500/30",
  muted: "bg-slate-500/15 text-slate-400 ring-slate-500/30",
};

// Stamp "is the Agent up to date with today's config?" (ADR-039).
function StatusPill({ data }: { data: NodeSettings }) {
  const t = useT();
  const meta = configStatusMeta(t, data.status);
  return (
    <span
      title={meta.hint}
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${TONE[meta.tone]}`}
    >
      {meta.label}
    </span>
  );
}

// Origin of the effective value. "Node" (own value) is what deserves attention — it's the deliberate
// difference from the cluster —, so it is highlighted and shows what the cluster would say.
function SourceCell({ item }: { item: NodeSettingItem }) {
  const t = useT();
  if (item.source === "node") {
    return (
      <div className="flex flex-col items-start gap-0.5">
        <span className="inline-flex rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-300 ring-1 ring-inset ring-amber-500/30">
          {t("nodes.nodeConfigPanel.sourceNode")}
        </span>
        <span className="text-[11px] text-slate-500">{t("nodes.nodeConfigPanel.sourceClusterValue", { value: fmtConfigValue(t, item, item.cluster_value) })}</span>
      </div>
    );
  }
  return (
    <span className="inline-flex rounded-full bg-slate-500/15 px-2 py-0.5 text-[11px] font-medium text-slate-300 ring-1 ring-inset ring-slate-500/30">
      {t("nodes.nodeConfigPanel.sourceCluster")}
    </span>
  );
}

// ValueInput: control by type — select for bool and enum, text/number for the rest.
function ValueInput({
  item,
  value,
  onChange,
  disabled,
}: {
  item: NodeSettingItem;
  value: string;
  onChange: (v: string) => void;
  disabled: boolean;
}) {
  const t = useT();
  if (item.type === "bool") {
    return (
      <select value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled} className={`w-32 ${inputCls}`}>
        <option value="true">{t("nodes.nodeConfigPanel.boolOn")}</option>
        <option value="false">{t("nodes.nodeConfigPanel.boolOff")}</option>
      </select>
    );
  }
  if (item.options && item.options.length > 0) {
    return (
      <select value={value} onChange={(e) => onChange(e.target.value)} disabled={disabled} className={`w-32 ${inputCls}`}>
        {item.options.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </select>
    );
  }
  const numeric = item.type === "int" || item.type === "int64" || item.type === "float";
  return (
    <input
      type={numeric ? "number" : "text"}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
      placeholder={item.type === "duration" ? t("nodes.nodeConfigPanel.durationPlaceholder") : ""}
      className={`w-28 font-mono ${inputCls}`}
    />
  );
}

// Single line. Editing is local (draft state); the container talks to the API, via
// onSave/onReset, which return an error message (or "" if it succeeded).
function ConfigRow({
  item,
  canWrite,
  onSave,
  onReset,
}: {
  item: NodeSettingItem;
  canWrite: boolean;
  onSave: (key: string, value: string) => Promise<string>;
  onReset: (key: string) => Promise<string>;
}) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(item.value);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const start = () => {
    setDraft(item.value);
    setErr("");
    setEditing(true);
  };
  const save = async () => {
    const problem = checkConfigValue(t, item, draft);
    if (problem) {
      setErr(problem);
      return;
    }
    setBusy(true);
    const msg = await onSave(item.key, draft.trim());
    setBusy(false);
    if (msg) setErr(msg);
    else setEditing(false);
  };
  const reset = async () => {
    setBusy(true);
    const msg = await onReset(item.key);
    setBusy(false);
    if (msg) setErr(msg);
  };

  return (
    <tr className="border-t border-slate-800/60 align-top">
      <td className="px-5 py-2.5">
        <p className="text-sm font-medium text-slate-200" title={settingText(item.key, "description", item.description)}>
          {settingText(item.key, "label", item.label)}
        </p>
        <p className="font-mono text-[11px] text-slate-500">{item.key}</p>
      </td>
      <td className="whitespace-nowrap px-5 py-2.5">
        {editing ? (
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
              <ValueInput item={item} value={draft} onChange={setDraft} disabled={busy} />
              {item.unit && <span className="text-xs text-slate-500">{item.unit}</span>}
            </div>
            {err && <span className="max-w-xs whitespace-normal text-xs text-red-400">{err}</span>}
          </div>
        ) : (
          <>
            <span className="font-mono text-sm text-white">{fmtConfigValue(t, item, item.value)}</span>
            {err && <p className="max-w-xs whitespace-normal text-xs text-red-400">{err}</p>}
          </>
        )}
      </td>
      <td className="px-5 py-2.5">
        <SourceCell item={item} />
      </td>
      {/* Button gate (docs/11 - Portal.md §13.1): without nodes:write the button disappears; the data stays. */}
      {canWrite && (
        <td className="whitespace-nowrap px-5 py-2.5 text-right">
          {editing ? (
            <div className="flex justify-end gap-1">
              <button
                onClick={save}
                disabled={busy}
                className="rounded-lg bg-emerald-600 px-2.5 py-1 text-xs font-medium text-white transition hover:bg-emerald-500 disabled:opacity-40"
              >
                {t("common.save")}
              </button>
              <button
                onClick={() => setEditing(false)}
                disabled={busy}
                className="rounded-lg border border-slate-700 px-2.5 py-1 text-xs text-slate-300 transition hover:bg-slate-800 disabled:opacity-40"
              >
                {t("common.cancel")}
              </button>
            </div>
          ) : (
            <div className="flex justify-end gap-1">
              <button
                onClick={start}
                disabled={busy}
                title={
                  item.source === "node"
                    ? t("nodes.nodeConfigPanel.editTitleNode")
                    : t("nodes.nodeConfigPanel.editTitleCluster")
                }
                className="rounded-lg border border-slate-700 px-2.5 py-1 text-xs text-slate-300 transition hover:bg-slate-800 disabled:opacity-40"
              >
                {item.source === "node" ? t("nodes.nodeConfigPanel.actionEdit") : t("nodes.nodeConfigPanel.actionOverride")}
              </button>
              {item.source === "node" && (
                <button
                  onClick={reset}
                  disabled={busy}
                  title={t("nodes.nodeConfigPanel.resetTitle")}
                  className="rounded-lg border border-slate-700 px-2.5 py-1 text-xs text-amber-400 transition hover:bg-slate-800 disabled:opacity-40"
                >
                  {t("nodes.nodeConfigPanel.actionReset")}
                </button>
              )}
            </div>
          )}
        </td>
      )}
    </tr>
  );
}

/**
 * NodeConfigView: "Configuration of this Node" (ADR-039) — the EFFECTIVE value the Agent receives, where
 * it comes from (this Node's value or the cluster's) and whether the Agent has applied it. Pure: receives data and
 * actions via props, to render and test without network.
 *
 * `canWrite` is the nodes:write permission (canWriteNodeConfig). Without it, the set/remove
 * buttons disappear (and the actions column);
 * the data remains on screen, like in the rest of the Portal (gate by button, not by data).
 */
export function NodeConfigView({
  data,
  canWrite,
  onSave,
  onReset,
}: {
  data: NodeSettings;
  canWrite: boolean;
  onSave: (key: string, value: string) => Promise<string>;
  onReset: (key: string) => Promise<string>;
}) {
  const t = useT();
  const overridden = data.items.filter((i) => i.source === "node").length;
  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          {t("nodes.nodeConfigPanel.title")}
          <StatusPill data={data} />
        </span>
      }
      right={
        data.applied_at && data.status !== "unsupported" ? (
          <span className="text-[11px] text-slate-500">{t("nodes.nodeConfigPanel.appliedAt", { time: fmtTime(data.applied_at) })}</span>
        ) : undefined
      }
    >
      <p className="px-5 pt-3 text-xs leading-relaxed text-slate-500">
        {rich(t("nodes.nodeConfigPanel.description", {
          overridden: overridden > 0 ? ` ${t("nodes.nodeConfigPanel.overriddenCount", { count: overridden })}` : "",
        }), {
          cluster: <b className="text-slate-400">{t("nodes.nodeConfigPanel.termCluster")}</b>,
          nodeValue: <b className="text-slate-400">{t("nodes.nodeConfigPanel.termNodeValue")}</b>,
        })}
      </p>
      <table className="mt-2 w-full text-left text-sm">
        <thead className="text-xs uppercase text-slate-500">
          <tr>
            <th className="px-5 py-2 font-medium">{t("nodes.nodeConfigPanel.colParam")}</th>
            <th className="px-5 py-2 font-medium">{t("nodes.nodeConfigPanel.colValue")}</th>
            <th className="px-5 py-2 font-medium">{t("nodes.nodeConfigPanel.colSource")}</th>
            {canWrite && <th className="px-5 py-2 text-right font-medium">{t("nodes.nodeConfigPanel.colActions")}</th>}
          </tr>
        </thead>
        <tbody>
          {data.items.map((it) => (
            <ConfigRow key={it.key} item={it} canWrite={canWrite} onSave={onSave} onReset={onReset} />
          ))}
        </tbody>
      </table>
    </Panel>
  );
}

/**
 * NodeConfigPanel: the container — fetches, updates every 10 s, and talks to the API. It doesn't decide
 * permissions on its own: it receives `can` and passes only `nodes:write` to the view.
 */
export function NodeConfigPanel({
  nodeId,
  can,
  onLogout,
}: {
  nodeId: string;
  can: (perm: string) => boolean;
  onLogout: () => void;
}) {
  const t = useT();
  const [data, setData] = useState<NodeSettings | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setData(await getNodeSettings(nodeId));
      setError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return onLogout();
      setError(t("nodes.nodeConfigPanel.errorLoad"));
    }
  }, [nodeId, onLogout, t]);

  useEffect(() => {
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [load]);

  // They return the error message ("" = succeeded) for the row to display next to the field.
  const run = useCallback(
    async (action: () => Promise<unknown>, okMsg: string): Promise<string> => {
      try {
        await action();
        toast(okMsg);
        await load();
        return "";
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          onLogout();
          return "";
        }
        return err instanceof ApiError ? err.message : t("nodes.nodeConfigPanel.errorSave");
      }
    },
    [load, onLogout, t],
  );

  const onSave = (key: string, value: string) =>
    run(() => setNodeSetting(nodeId, key, value), t("nodes.nodeConfigPanel.toastSaved"));
  const onReset = (key: string) =>
    run(() => resetNodeSetting(nodeId, key), t("nodes.nodeConfigPanel.toastReset"));

  if (error && !data) {
    return (
      <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">{error}</div>
    );
  }
  if (!data) return null;
  return <NodeConfigView data={data} canWrite={canWriteNodeConfig(can)} onSave={onSave} onReset={onReset} />;
}
