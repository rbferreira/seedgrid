import { useState, type FormEvent } from "react";
import { ApiError, renameNode, type Node } from "../api";
import { toast } from "../actions";
import { NODE_NAME_MAX } from "../lib/node-label";
import { rich, useT } from "../i18n";

/**
 * RenameNodeModal: sets (or removes) the nickname of a Node within SeedGrid (#145).
 *
 * It only talks to the Control Plane — nothing reaches the Node and the OS hostname doesn't change, so there
 * is no restart or unattended dialog. That's why it doesn't ask for confirmation: it's instantly reversible.
 * An empty field removes the nickname and the Portal goes back to showing the hostname.
 *
 * It doesn't decide permissions: what gates it (`nodes:write`) is the button that opens the modal.
 */
export function RenameNodeModal({
  node,
  onClose,
  onSaved,
  onLogout,
}: {
  node: Node;
  onClose: () => void;
  onSaved: () => void;
  onLogout: () => void;
}) {
  const t = useT();
  const [value, setValue] = useState(node.display_name ?? "");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const r = await renameNode(node.id, value.trim());
      toast(r.display_name ? t("health.renameNodeModal.toastRenamed", { name: r.display_name }) : t("health.renameNodeModal.toastCleared", { hostname: node.hostname }));
      onSaved();
      onClose();
    } catch (e2) {
      if (e2 instanceof ApiError && e2.status === 401) onLogout();
      else if (e2 instanceof ApiError && [400, 404, 409].includes(e2.status)) setErr(e2.message);
      else setErr(t("health.renameNodeModal.errorGeneric"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <form
        onSubmit={save}
        className="w-full max-w-md rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-sm font-semibold text-white">{t("health.renameNodeModal.title")}</h2>
        <p className="mt-1 text-xs text-slate-400">
          {rich(t("health.renameNodeModal.description"), {
            hostname: <span className="font-mono text-slate-300">{node.hostname}</span>,
          })}
        </p>
        <label className="mt-4 block text-[10px] uppercase tracking-wide text-slate-500" htmlFor="rename-node">
          {t("health.renameNodeModal.label")}
        </label>
        <input
          id="rename-node"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          maxLength={NODE_NAME_MAX}
          placeholder={node.hostname}
          autoFocus
          autoComplete="off"
          className="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
        />
        {err && <p className="mt-3 rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-400">{err}</p>}
        <div className="mt-4 flex gap-2">
          <button
            type="button"
            onClick={onClose}
            className="flex-1 rounded-lg px-3 py-2 text-xs text-slate-400 transition hover:bg-slate-800"
          >
            {t("common.cancel")}
          </button>
          <button
            type="submit"
            disabled={busy}
            className="flex-1 rounded-lg bg-emerald-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-50"
          >
            {busy ? t("common.saving") : t("common.save")}
          </button>
        </div>
      </form>
    </div>
  );
}
