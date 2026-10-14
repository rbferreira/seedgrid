import { useState, type FormEvent } from "react";
import { ApiError, mintEnrollToken, type EnrollToken } from "../api";
import { rich, useT } from "../i18n";
import { NODE_NAME_MAX } from "../lib/node-label";
import { fmtTime } from "./ui";

// AddNodeModal generates a one-time join token (ADR-017) and builds the onboarding command
// zero-touch ready to copy. The operator runs the command on the new Node; it does the
// enroll (the CP assigns the BT port), starts the Engine and registers. Only the
// port-forward on the firewall is left (operator's task).
//
// The name is optional (#145) and is the first thing the modal asks, because the join token
// carries it: the CP saves it as the Node nickname when it registers. The command itself doesn't
// change — the bootstrap doesn't need to know the name, and the OS hostname is not touched.
export function AddNodeModal({ onClose, onLogout }: { onClose: () => void; onLogout: () => void }) {
  const t = useT();
  const [name, setName] = useState("");
  const [tok, setTok] = useState<EnrollToken | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState("");

  const generate = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      setTok(await mintEnrollToken(name.trim()));
    } catch (e2) {
      if (e2 instanceof ApiError && e2.status === 401) onLogout();
      else if (e2 instanceof ApiError && (e2.status === 400 || e2.status === 409)) setErr(e2.message);
      else setErr(t("health.addNodeModal.errorGenerate"));
    } finally {
      setBusy(false);
    }
  };

  const cp = window.location.origin;
  const cmd = tok ? `curl -fsSL ${cp}/node/bootstrap-node.sh | CP_URL=${cp} JOIN_TOKEN=${tok.token} sh` : "";

  const copy = (text: string, what: string) => {
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(what);
      setTimeout(() => setCopied(""), 1500);
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <div
        className="w-full max-w-2xl rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-sm font-semibold text-white">{t("health.addNodeModal.title")}</h2>

        {!tok && (
          <form onSubmit={generate} className="mt-1">
            <p className="text-xs text-slate-400">
              {t("health.addNodeModal.nameDescription")}
            </p>
            <label className="mt-4 block text-[10px] uppercase tracking-wide text-slate-500" htmlFor="node-name">
              {t("health.addNodeModal.nameLabel")}
            </label>
            <input
              id="node-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={NODE_NAME_MAX}
              placeholder={t("health.addNodeModal.namePlaceholder")}
              autoFocus
              autoComplete="off"
              className="sg-field mt-1 w-full"
            />
            {err && <p className="mt-3 rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-400">{err}</p>}
            <button
              type="submit"
              disabled={busy}
              className="mt-4 w-full rounded-full bg-emerald-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-50"
            >
              {busy ? t("health.addNodeModal.generating") : t("health.addNodeModal.generateBtn")}
            </button>
          </form>
        )}

        {tok && (
          <>
            <p className="mt-1 text-xs text-slate-400">
              {t("health.addNodeModal.commandInstructions")}
            </p>

            {tok.display_name && (
              <p className="mt-3 text-xs text-slate-400">
                {rich(t("health.addNodeModal.displayNamePreview"), {
                  name: <span className="font-medium text-white">{tok.display_name}</span>,
                })}
              </p>
            )}

            <div className="mt-4">
              <p className="text-[10px] uppercase tracking-wide text-slate-500">{t("health.addNodeModal.onboardingCommandLabel")}</p>
              <div className="mt-1 flex items-start gap-2">
                <code className="flex-1 overflow-x-auto whitespace-pre rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2 font-mono text-xs text-emerald-300">
                  {cmd}
                </code>
                <button
                  onClick={() => copy(cmd, "cmd")}
                  className="shrink-0 rounded-full border border-slate-700 px-2.5 py-2 text-xs text-white transition hover:bg-slate-800"
                >
                  {copied === "cmd" ? t("health.addNodeModal.copiedCheck") : t("health.addNodeModal.copy")}
                </button>
              </div>
            </div>

            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <p className="text-[10px] uppercase tracking-wide text-slate-500">{t("health.addNodeModal.joinTokenLabel")}</p>
                <div className="mt-1 flex items-center gap-2">
                  <code className="flex-1 overflow-x-auto rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2 font-mono text-xs text-slate-300">
                    {tok.token}
                  </code>
                  <button
                    onClick={() => copy(tok.token, "token")}
                    className="shrink-0 rounded-full border border-slate-700 px-2.5 py-2 text-xs text-white transition hover:bg-slate-800"
                  >
                    {copied === "token" ? "✓" : t("health.addNodeModal.copy")}
                  </button>
                </div>
              </div>
              <div>
                <p className="text-[10px] uppercase tracking-wide text-slate-500">{t("health.addNodeModal.expiresLabel")}</p>
                <p className="mt-1 rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2 text-xs text-slate-300">
                  {fmtTime(tok.expires_at)}
                </p>
              </div>
            </div>

            <p className="mt-3 text-[11px] text-slate-500">
              {rich(t("health.addNodeModal.firewallNote"), { var: <span className="font-mono">{"CP_URL"}</span> })}
            </p>
          </>
        )}

        <button
          onClick={onClose}
          className="mt-4 w-full rounded-full px-3 py-2 text-xs text-slate-400 transition hover:bg-slate-800"
        >
          {t("common.close")}
        </button>
      </div>
    </div>
  );
}
