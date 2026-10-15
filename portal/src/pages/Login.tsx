import { useState, type FormEvent } from "react";
import { login } from "../api";
import { Logo } from "../components/ui";
import { useT } from "../i18n";

export function Login({ onLogin }: { onLogin: () => void }) {
  const t = useT();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      await login(username, password);
      onLogin();
    } catch {
      setError(t("login.invalidCredentials"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <form
        onSubmit={submit}
        className="w-full max-w-sm rounded-2xl border border-slate-800 bg-slate-900/60 p-8 shadow-2xl shadow-emerald-950/30"
      >
        <div className="mb-6 flex items-center gap-3">
          <Logo className="h-10 w-10" />
          <div>
            <h1 className="text-lg font-semibold text-white">SeedGrid</h1>
            <p className="text-xs text-slate-400">{t("login.subtitle")}</p>
          </div>
        </div>

        <label className="mb-1 block text-sm text-slate-300">{t("login.username")}</label>
        <input
          className="sg-field mb-4 w-full"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoFocus
        />

        <label className="mb-1 block text-sm text-slate-300">{t("login.password")}</label>
        <input
          type="password"
          className="sg-field mb-4 w-full"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />

        {error && <p className="mb-4 text-sm text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={loading || !username || !password}
          className="w-full rounded-full bg-emerald-600 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-50"
        >
          {loading ? t("login.submitting") : t("login.submit")}
        </button>
      </form>
    </div>
  );
}
