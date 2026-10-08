import { useEffect, useState } from "react";
import { ApiError, getSettingOverrides, type SettingOverride } from "../api";
import { href } from "../router";
import { useT } from "../i18n";
import { settingText } from "../lib/setting-text";

/**
 * NodeOverridesList: the Nodes with their own value (ADR-039). It sits on the Settings screen,
 * right below the Agent/Engine cluster values, because it answers the question of who will
 * touch them: "if I change the cluster value, WHO is not affected?". Without this, a changed
 * cluster value seems to apply to the entire fleet — the error that #144 exists to prevent.
 *
 * Pure: receives the list via props. Each row leads to the Node detail, where its own value is
 * edited (nodes:write).
 */
export function NodeOverridesList({ rows }: { rows: SettingOverride[] }) {
  const t = useT();
  if (rows.length === 0) {
    return (
      <p className="px-4 py-3 text-xs text-slate-500">
        {t("nodes.nodeOverridesPanel.empty")}
      </p>
    );
  }
  return (
    <table className="w-full text-left text-sm">
      <thead className="text-xs uppercase text-slate-500">
        <tr>
          <th className="px-4 py-2 font-medium">{t("nodes.nodeOverridesPanel.headers.node")}</th>
          <th className="px-4 py-2 font-medium">{t("nodes.nodeOverridesPanel.headers.parameter")}</th>
          <th className="px-4 py-2 font-medium">{t("nodes.nodeOverridesPanel.headers.nodeValue")}</th>
          <th className="px-4 py-2 font-medium">{t("nodes.nodeOverridesPanel.headers.cluster")}</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={`${r.node_id}:${r.key}`} className="border-t border-slate-800/60">
            <td className="px-4 py-2">
              <a href={href(`/nodes/${r.node_id}`)} className="text-emerald-400 hover:text-emerald-300">
                {r.node_label}
              </a>
            </td>
            <td className="px-4 py-2 text-slate-300">{settingText(r.key, "label", r.label)}</td>
            <td className="px-4 py-2 font-mono text-amber-300">
              {r.value}
              {r.unit ? ` ${r.unit}` : ""}
            </td>
            <td className="px-4 py-2 font-mono text-slate-500">
              {r.cluster_value}
              {r.unit ? ` ${r.unit}` : ""}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function NodeOverridesPanel({ onLogout }: { onLogout: () => void }) {
  const t = useT();
  const [rows, setRows] = useState<SettingOverride[] | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let alive = true;
    getSettingOverrides()
      .then((r) => alive && setRows(r))
      .catch((err) => {
        if (!alive) return;
        if (err instanceof ApiError && err.status === 401) return onLogout();
        setError(t("nodes.nodeOverridesPanel.loadError"));
      });
    return () => {
      alive = false;
    };
  }, [onLogout, t]);

  return (
    <div className="rounded-xl border border-slate-800 bg-slate-900/40">
      <div className="border-b border-slate-800 px-4 py-3">
        <h3 className="text-sm font-semibold text-slate-100">{t("nodes.nodeOverridesPanel.title")}</h3>
        <p className="mt-0.5 text-xs text-slate-500">
          {t("nodes.nodeOverridesPanel.description")}
        </p>
      </div>
      {error && <p className="px-4 py-3 text-xs text-red-400">{error}</p>}
      {rows && <NodeOverridesList rows={rows} />}
      {!rows && !error && <p className="px-4 py-3 text-xs text-slate-500">{t("common.loading")}</p>}
    </div>
  );
}
