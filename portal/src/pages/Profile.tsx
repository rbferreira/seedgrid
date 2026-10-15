import { useState } from "react";
import { changePassword, clearDefaultCreds, getUsername, ApiError } from "../api";
import { useT } from "../i18n";

export function Profile({ onLogout }: { onLogout: () => void }) {
  const t = useT();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [ok, setOk] = useState(false);

  async function submit() {
    setError("");
    setOk(false);
    if (next.length < 6) {
      setError(t("profile.changePassword.tooShort"));
      return;
    }
    if (next !== confirm) {
      setError(t("profile.changePassword.mismatch"));
      return;
    }
    setLoading(true);
    try {
      await changePassword(current, next);
      clearDefaultCreds(); // hide the admin/admin warning
      setOk(true);
      setCurrent("");
      setNext("");
      setConfirm("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      if (err instanceof ApiError && err.status === 400) {
        setError(t("profile.changePassword.badCurrent"));
      } else {
        setError(t("profile.changePassword.failed"));
      }
    } finally {
      setLoading(false);
    }
  }

  const field = (label: string, val: string, set: (v: string) => void) => (
    <div>
      <label className="mb-1 block text-sm text-slate-300">{label}</label>
      <input
        type="password"
        value={val}
        onChange={(e) => set(e.target.value)}
        className="sg-field w-full"
      />
    </div>
  );

  return (
    <>
      <h1 className="text-lg font-semibold text-white">{t("profile.title")}</h1>
      <div className="max-w-md space-y-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-6">
        <div>
          <p className="text-xs uppercase tracking-wide text-slate-500">{t("profile.operator")}</p>
          <p className="text-lg font-medium text-white">{getUsername() || "—"}</p>
        </div>
        <hr className="border-slate-800" />
        <h2 className="text-sm font-semibold text-white">{t("profile.changePassword.title")}</h2>
        {field(t("profile.changePassword.current"), current, setCurrent)}
        {field(t("profile.changePassword.next"), next, setNext)}
        {field(t("profile.changePassword.confirm"), confirm, setConfirm)}
        {error && <p className="text-sm text-red-400">{error}</p>}
        {ok && <p className="text-sm text-emerald-400">{t("profile.changePassword.success")}</p>}
        <button
          onClick={submit}
          disabled={loading || !current || !next}
          className="rounded-full bg-emerald-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-50"
        >
          {loading ? t("profile.changePassword.submitting") : t("profile.changePassword.submit")}
        </button>
      </div>
    </>
  );
}
