import { useState, useEffect, useCallback } from "react";
import { TrackerProfileEditor } from "./TrackerProfileEditor";
import { canPublish } from "../lib/taxonomy";
import {
  getTrackers,
  createTracker,
  updateTracker,
  deleteTracker,
  setDefaultTracker,
  ApiError,
  type Tracker,
} from "../api";
import { rich, useT } from "../i18n";

// TrackersPanel manages the operator's announce trackers (ADR-019/ADR-020). The
// passkey (in announce) and api_token are never returned by the API — the list shows only name +
// host + source + if it has API. Editing allows attaching the UNIT3D integration (source +
// API URL + token) to an existing tracker without retyping the passkey. It lives in
// Settings (publication tracker registration).
// Example values for technical fields: they are what you type, identical in any language.
const EXAMPLE = { source: "TRACKER", apiUrl: "https://tracker.example" };

const emptyForm = { name: "", announce: "", source: "", apiUrl: "", apiToken: "", isDefault: false };

export function TrackersPanel({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [trackers, setTrackers] = useState<Tracker[]>([]);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState(emptyForm);
  const [showIntegration, setShowIntegration] = useState(false);
  const [profileOpen, setProfileOpen] = useState<string | null>(null); // tracker with the open profile
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const set = (k: keyof typeof emptyForm) => (v: string | boolean) => setForm((f) => ({ ...f, [k]: v }));
  const editing = editingId !== null;

  const load = useCallback(async () => {
    try {
      setTrackers(await getTrackers());
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) onLogout();
    }
  }, [onLogout]);

  useEffect(() => {
    load();
  }, [load]);

  function resetForm() {
    setEditingId(null);
    setForm(emptyForm);
    setShowIntegration(false);
    setError("");
  }

  function startEdit(tracker: Tracker) {
    setEditingId(tracker.id);
    // announce and api_token are never returned by the API → blank = keep current ones.
    setForm({ name: tracker.name, announce: "", source: tracker.source, apiUrl: tracker.api_url, apiToken: "", isDefault: tracker.is_default });
    setShowIntegration(!!tracker.source || !!tracker.api_url || tracker.has_api_token);
    setError("");
  }

  async function submit() {
    setError("");
    if (!form.name.trim()) {
      setError(t("trackers.trackersPanel.errors.nameRequired"));
      return;
    }
    if (!editing && !form.announce.trim()) {
      setError(t("trackers.trackersPanel.errors.announceRequired"));
      return;
    }
    setBusy(true);
    const payload = {
      name: form.name.trim(),
      announce: form.announce.trim(),
      source: form.source.trim(),
      api_url: form.apiUrl.trim(),
      api_token: form.apiToken.trim(),
      is_default: form.isDefault,
    };
    try {
      if (editing) await updateTracker(editingId!, payload);
      else await createTracker(payload);
      resetForm();
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(err instanceof ApiError ? err.message : t("trackers.trackersPanel.errors.saveFailed"));
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: string, nm: string) {
    if (!window.confirm(t("trackers.trackersPanel.confirmRemove", { name: nm }))) return;
    try {
      await deleteTracker(id);
      if (editingId === id) resetForm();
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) onLogout();
    }
  }

  async function makeDefault(id: string) {
    try {
      await setDefaultTracker(id);
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) onLogout();
    }
  }

  // Form-control base classes (#22).
  const inputCls = "sg-field w-full";

  return (
    <div className="max-w-2xl space-y-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-6">
      <div>
        <h2 className="text-sm font-semibold text-white">{t("trackers.trackersPanel.title")}</h2>
        <p className="mt-1 text-xs text-slate-400">
          {rich(t("trackers.trackersPanel.description"))}
        </p>
      </div>

      {trackers.length > 0 && (
        <ul className="divide-y divide-slate-800 rounded-xl border border-slate-800">
          {trackers.map((tracker) => (
            <li key={tracker.id}>
              <div className="flex items-center justify-between gap-3 px-4 py-2.5">
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-2 text-sm text-white">
                    <span className="truncate">{tracker.name}</span>
                    {tracker.is_default && (
                      <span className="shrink-0 rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs text-emerald-400 ring-1 ring-inset ring-emerald-500/30">
                        {t("trackers.trackersPanel.badges.default")}
                      </span>
                    )}
                    {tracker.source && (
                      <span className="shrink-0 rounded-full bg-sky-500/15 px-2 py-0.5 text-xs text-sky-300 ring-1 ring-inset ring-sky-500/30">
                        {tracker.source}
                      </span>
                    )}
                    {tracker.has_api_token && (
                      <span className="shrink-0 rounded-full bg-violet-500/15 px-2 py-0.5 text-xs text-violet-300 ring-1 ring-inset ring-violet-500/30">
                        {t("trackers.trackersPanel.badges.api")}
                      </span>
                    )}
                    {tracker.api_url && !canPublish(tracker.profile) && (
                      <span
                        title={t("trackers.trackersPanel.tooltips.emptyProfile")}
                        className="shrink-0 rounded-full bg-amber-500/15 px-2 py-0.5 text-xs text-amber-300 ring-1 ring-inset ring-amber-500/30"
                      >
                        {t("trackers.trackersPanel.badges.emptyProfile")}
                      </span>
                    )}
                  </p>
                  <p className="truncate font-mono text-xs text-slate-500">{tracker.host}</p>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {tracker.api_url && (
                    <button
                      onClick={() => setProfileOpen(profileOpen === tracker.id ? null : tracker.id)}
                      title={t("trackers.trackersPanel.tooltips.profileButton")}
                      className="rounded-full px-2 py-1 text-xs text-slate-400 transition hover:bg-slate-800 hover:text-sky-300"
                    >
                      {profileOpen === tracker.id ? t("trackers.trackersPanel.actions.profileOpen") : t("trackers.trackersPanel.actions.profileClosed")}
                    </button>
                  )}
                  <button
                    onClick={() => startEdit(tracker)}
                    className="rounded-full px-2 py-1 text-xs text-slate-400 transition hover:bg-slate-800 hover:text-sky-300"
                  >
                    {t("trackers.trackersPanel.actions.edit")}
                  </button>
                  {!tracker.is_default && (
                    <button
                      onClick={() => makeDefault(tracker.id)}
                      className="rounded-full px-2 py-1 text-xs text-slate-400 transition hover:bg-slate-800 hover:text-emerald-400"
                    >
                      {t("trackers.trackersPanel.actions.makeDefault")}
                    </button>
                  )}
                  <button
                    onClick={() => remove(tracker.id, tracker.name)}
                    title={t("trackers.trackersPanel.actions.removeTitle")}
                    className="rounded-full px-2 py-1 text-xs text-slate-400 transition hover:bg-slate-800 hover:text-red-400"
                  >
                    🗑
                  </button>
                </div>
              </div>
              {profileOpen === tracker.id && <TrackerProfileEditor tracker={tracker} can={can} onChanged={load} onLogout={onLogout} />}
            </li>
          ))}
        </ul>
      )}

      <div className="space-y-3 rounded-xl border border-slate-800 bg-slate-950/40 p-4">
        <div className="flex items-center justify-between">
          <p className="text-xs font-medium uppercase tracking-wide text-slate-500">
            {editing ? t("trackers.trackersPanel.form.editTitle") : t("trackers.trackersPanel.form.newTitle")}
          </p>
          {editing && (
            <button onClick={resetForm} className="text-xs text-slate-400 hover:text-white">
              {t("common.cancel")}
            </button>
          )}
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <input
            value={form.name}
            onChange={(e) => set("name")(e.target.value)}
            placeholder={t("trackers.trackersPanel.placeholders.name")}
            className={inputCls}
          />
          <input
            value={form.announce}
            onChange={(e) => set("announce")(e.target.value)}
            placeholder={editing ? t("trackers.trackersPanel.placeholders.announceKeep") : t("trackers.trackersPanel.placeholders.announceNew")}
            className={`${inputCls} font-mono text-xs`}
          />
        </div>

        <button
          type="button"
          onClick={() => setShowIntegration((v) => !v)}
          className="flex items-center gap-1 text-xs font-medium text-sky-300 hover:text-sky-200"
        >
          <span>{showIntegration ? "▾" : "▸"}</span> {t("trackers.trackersPanel.integration.toggle")}
        </button>
        {showIntegration && (
          <div className="space-y-3 rounded-lg border border-slate-800 bg-slate-950/60 p-3">
            <p className="text-xs text-slate-400">
              {t("trackers.trackersPanel.integration.help")}
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block">
                <span className="mb-1 block text-xs text-slate-400">
                  {t("trackers.trackersPanel.fields.sourceLabel")}
                </span>
                <input
                  value={form.source}
                  onChange={(e) => set("source")(e.target.value)}
                  placeholder={EXAMPLE.source}
                  className={`${inputCls} font-mono`}
                />
              </label>
              <label className="block">
                <span className="mb-1 block text-xs text-slate-400">{t("trackers.trackersPanel.fields.apiUrlLabel")}</span>
                <input
                  value={form.apiUrl}
                  onChange={(e) => set("apiUrl")(e.target.value)}
                  placeholder={EXAMPLE.apiUrl}
                  className={`${inputCls} font-mono text-xs`}
                />
              </label>
            </div>
            <label className="block">
              <span className="mb-1 block text-xs text-slate-400">{t("trackers.trackersPanel.fields.apiTokenLabel")}</span>
              <input
                type="password"
                value={form.apiToken}
                onChange={(e) => set("apiToken")(e.target.value)}
                placeholder={editing ? t("trackers.trackersPanel.placeholders.tokenKeep") : t("trackers.trackersPanel.placeholders.tokenNew")}
                className={`${inputCls} font-mono text-xs`}
              />
            </label>
          </div>
        )}

        {!editing && (
          <label className="flex items-center gap-2 text-xs text-slate-400">
            <input
              type="checkbox"
              checked={form.isDefault}
              onChange={(e) => set("isDefault")(e.target.checked)}
              className="sg-check"
            />
            {t("trackers.trackersPanel.form.setDefault")}
          </label>
        )}
        {error && <p className="text-sm text-red-400">{error}</p>}
        <button
          onClick={submit}
          disabled={busy}
          className="rounded-full bg-emerald-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-50"
        >
          {busy ? t("common.saving") : editing ? t("trackers.trackersPanel.actions.saveChanges") : t("trackers.trackersPanel.actions.addTracker")}
        </button>
      </div>
    </div>
  );
}
