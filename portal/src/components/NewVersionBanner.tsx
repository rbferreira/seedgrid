import { useEffect, useState } from "react";
import { bundleOf, isStale, runningBundle } from "../lib/portal-version";
import { getPublishedIndex } from "../api";
import { useT } from "../i18n";

const CHECK_MS = 60_000;

/**
 * NewVersionBanner: says a new Portal version is published and offers to reload (#39).
 *
 * It checks every minute and when the tab regains focus — the typical case is the tab left open
 * during the deploy that the operator comes back to. It doesn't reload by itself: the operator may be
 * in the middle of a form or an upload.
 */
export function NewVersionBanner() {
  const [stale, setStale] = useState(false);

  useEffect(() => {
    const running = runningBundle();
    if (!running) return; // dev: no hashed bundle, nothing to compare
    let alive = true;
    const check = async () => {
      const published = bundleOf(await getPublishedIndex());
      if (alive && isStale(running, published)) setStale(true);
    };
    const onVisible = () => document.visibilityState === "visible" && check();
    const id = window.setInterval(check, CHECK_MS);
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("focus", check);
    return () => {
      alive = false;
      window.clearInterval(id);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("focus", check);
    };
  }, []);

  if (!stale) return null;
  return <NewVersionBar onReload={() => window.location.reload()} />;
}

// NewVersionBar: the banner itself, apart from the check so the test can render it without network.
export function NewVersionBar({ onReload }: { onReload: () => void }) {
  const t = useT();
  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-3 rounded-2xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3"
    >
      <span
        aria-hidden="true"
        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-emerald-500/20 text-sm text-emerald-300"
      >
        {"↻"}
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-emerald-200">{t("ui.newVersion.title")}</p>
        <p className="text-xs text-emerald-300/70">{t("ui.newVersion.body")}</p>
      </div>
      <button
        type="button"
        onClick={onReload}
        className="shrink-0 rounded-full bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white transition hover:bg-emerald-500"
      >
        {t("ui.newVersion.reload")}
      </button>
    </div>
  );
}
