import { useEffect, useState, type ReactNode } from "react";
import { getUsername, hasPermission, type Me } from "../api";
import { useRoute, href } from "../router";
import { Logo } from "./ui";
import { ThemeSwitch } from "./ThemeSwitch";
import { applyTheme, readThemePref, saveThemePref, watchSystemTheme, type ThemePref } from "../lib/theme";
import { useT, useLang, type Key, type Lang } from "../i18n";

// match: extra routes that highlight the item. The Node detail (/nodes/{id}) still
// exists after the LIST of Nodes became part of Health — without this it wouldn't highlight
// any menu item.
type NavItem = { path: string; labelKey: Key; icon: string; perm?: string; match?: string[] };

const NAV: NavItem[] = [
  { path: "/dashboard", labelKey: "nav.dashboard", icon: "▦" },
  { path: "/downloaded", labelKey: "nav.downloaded", icon: "📥", perm: "torrents:read" },
  { path: "/analytics", labelKey: "nav.analytics", icon: "📊", perm: "torrents:read" },
  { path: "/add", labelKey: "nav.add", icon: "＋", perm: "torrents:create" },
  { path: "/publish", labelKey: "nav.publish", icon: "📤", perm: "torrents:publish" },
  { path: "/explore", labelKey: "nav.explore", icon: "🧭", perm: "torrents:read" },
  { path: "/feed", labelKey: "nav.feed", icon: "✨", perm: "activities:read" },
  { path: "/alerts", labelKey: "nav.alerts", icon: "🔔", perm: "alerts:read" },
  { path: "/health", labelKey: "nav.health", icon: "❤", perm: "metrics:read", match: ["/nodes"] },
  { path: "/audit", labelKey: "nav.audit", icon: "📜", perm: "audit:read" },
  { path: "/users", labelKey: "nav.users", icon: "👤", perm: "users:read" },
  { path: "/settings", labelKey: "nav.settings", icon: "⚙", perm: "settings:write" },
];

// isActive: highlights the item whose route matches (or is an ancestor, for detail pages), as well
// as the routes declared in `match`.
function isActive(route: string, path: string, match: string[] = []): boolean {
  const hit = (p: string) => route === p || route.startsWith(p + "/");
  return hit(path) || match.some(hit);
}

export function Layout({
  me,
  meReady = true,
  onLogout,
  children,
}: {
  me: Me | null;
  meReady?: boolean;
  onLogout: () => void;
  children: ReactNode;
}) {
  const t = useT();
  const [lang, setLang] = useLang();
  const route = useRoute();
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);

  // Theme (backlog #10, #17): light, dark or follow the operating system. The inline script in
  // index.html already applied it before the first paint; here it is re-applied on change and,
  // under "system", whenever the operating system switches.
  const [themePref, setThemePref] = useState<ThemePref>(() => readThemePref());
  useEffect(() => {
    applyTheme(themePref);
    if (themePref !== "system") return;
    return watchSystemTheme(() => applyTheme("system"));
  }, [themePref]);
  const chooseTheme = (pref: ThemePref) => {
    saveThemePref(pref);
    setThemePref(pref);
  };

  // Language selector (ADR-041): a list, not a switch — more languages are expected. Same row
  // shape as the theme: label on the left, control on the right; the list takes what is left of
  // the row and truncates instead of pushing past the sidebar edge. Language names are kept in
  // their own language (do not translate "Português" or "English").
  const langSelect = (
    <label className="flex items-center justify-between gap-2 px-1">
      <span className="shrink-0 text-xs uppercase tracking-wide text-slate-500">{t("lang.label")}</span>
      <select
        value={lang}
        onChange={(e) => setLang(e.target.value as Lang)}
        className="sg-field-sm sg-select min-w-0"
      >
        <option value="pt-BR">Português</option>
        <option value="en">English</option>
      </select>
    </label>
  );

  const nav = (
    <nav className="flex flex-col gap-1">
      {NAV.filter((n) => !n.perm || !meReady || hasPermission(me, n.perm)).map((n) => {
        const active = isActive(route, n.path, n.match);
        return (
          <a
            key={n.path}
            href={href(n.path)}
            onClick={close}
            className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
              active
                ? "bg-emerald-600/15 font-medium text-emerald-300 ring-1 ring-inset ring-emerald-600/30"
                : "text-slate-400 hover:bg-slate-800/60 hover:text-slate-200"
            }`}
          >
            <span aria-hidden className="w-5 text-center text-base">{n.icon}</span>
            {t(n.labelKey)}
          </a>
        );
      })}
    </nav>
  );

  const sidebar = (
    <div className="flex h-full flex-col gap-6 p-4">
      <a href={href("/dashboard")} onClick={close} className="flex items-center gap-3 px-1">
        <Logo className="h-9 w-9" />
        <span className="text-lg font-semibold text-white">SeedGrid</span>
      </a>
      {nav}
      <div className="mt-auto border-t border-slate-800 pt-4">
        <div className="mb-3 space-y-2.5">
          <div className="flex items-center justify-between gap-2 px-1">
            <span className="text-xs uppercase tracking-wide text-slate-500">{t("theme.label")}</span>
            <ThemeSwitch value={themePref} onChange={chooseTheme} />
          </div>
          {langSelect}
        </div>
        <a
          href={href("/profile")}
          onClick={close}
          className={`flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition ${
            isActive(route, "/profile")
              ? "bg-slate-800 text-white"
              : "text-slate-300 hover:bg-slate-800/60"
          }`}
        >
          <span aria-hidden className="grid h-6 w-6 place-items-center rounded-full bg-slate-700 text-xs">
            {(getUsername() || "?").slice(0, 1).toUpperCase()}
          </span>
          <span className="truncate">{getUsername() || t("profile.placeholder")}</span>
        </a>
        <button
          onClick={onLogout}
          className="mt-1 flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm text-slate-400 transition hover:bg-slate-800/60 hover:text-red-400"
        >
          <span aria-hidden className="w-6 text-center">⏻</span>
          {t("common.logout")}
        </button>
      </div>
    </div>
  );

  return (
    // overflow-x-clip: mobile safeguard — prevents any wide child
    // (e.g., 8-column table) from extending the PAGE beyond the viewport and making the browser
    // shrink everything to fit (content squished into the left half). Panels with tables
    // scroll internally via overflow-x-auto. `clip` (not `hidden`) preserves the `sticky`
    // mobile header.
    <div className="min-h-screen overflow-x-clip bg-slate-950 lg:flex">
      {/* Desktop sidebar. w-48 (192px) instead of w-52 (208px): frees up 16px for the main
          content — which allows wider tables (e.g., Nodes with Size column, #90)
          to fit without a horizontal scrollbar on laptops. Menu labels still fit
          comfortably (the longest is "Alerts & Events"). */}
      <aside className="hidden w-48 shrink-0 border-r border-slate-800 bg-slate-900/40 lg:block">
        <div className="sticky top-0 h-screen">{sidebar}</div>
      </aside>

      {/* Sidebar mobile (drawer) */}
      {open && (
        <div className="fixed inset-0 z-40 lg:hidden" role="dialog" aria-modal="true">
          <div className="absolute inset-0 bg-black/60" onClick={close} />
          <aside className="absolute inset-y-0 left-0 w-64 border-r border-slate-800 bg-slate-900">
            {sidebar}
          </aside>
        </div>
      )}

      {/* Content. min-w-0: without this the flex item won't shrink below the width of the
          content, so a wide table (overflow-x-auto) pushes the entire PAGE and
          creates a horizontal scrollbar on laptops. With min-w-0 the scroll stays in the panel. */}
      <div className="flex min-h-screen min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 flex items-center gap-3 border-b border-slate-800 bg-slate-950/80 px-4 py-3 backdrop-blur lg:hidden">
          <button
            onClick={() => setOpen(true)}
            aria-label={t("nav.openMenu")}
            className="rounded-full border border-slate-700 px-3 py-1.5 text-slate-300"
          >
            ☰
          </button>
          <span className="font-semibold text-white">SeedGrid</span>
          <div className="ml-auto">
            <ThemeSwitch value={themePref} onChange={chooseTheme} />
          </div>
        </header>
        {/* max-w-[2000px]: takes advantage of ultrawide screens (used to cap at 1536px and end up
            squished in the middle) without stretching too much to the point of hindering readability. */}
        <main className="mx-auto w-full min-w-0 max-w-[2000px] flex-1 space-y-6 px-3 py-6 sm:px-4 lg:px-5 2xl:px-8">
          {children}
        </main>
      </div>
    </div>
  );
}
