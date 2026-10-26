import { useState, useEffect, useRef } from "react";
import { getToken, clearToken, getActivities, getMe, hasPermission, type Me } from "./api";
import { useRoute } from "./router";
import { Login } from "./pages/Login";
import { Dashboard } from "./pages/Dashboard";
import { Analytics } from "./pages/Analytics";
import { Downloaded } from "./pages/Downloaded";
import { NodeDetail } from "./pages/NodeDetail";
import { TorrentDetail } from "./pages/TorrentDetail";
import { AlertsEvents } from "./pages/AlertsEvents";
import { Feed } from "./pages/Feed";
import { HealthStatus } from "./pages/HealthStatus";
import { AddTorrent, Publish } from "./pages/AddTorrent";
import { Explore } from "./pages/Explore";
import { Profile } from "./pages/Profile";
import { Settings } from "./pages/Settings";
import { AuditPage } from "./pages/AuditPage";
import { Users } from "./pages/Users";
import { Layout } from "./components/Layout";
import { ToastViewport, toast } from "./actions";
import { ConfirmHost } from "./components/ConfirmDialog";
import { useT } from "./i18n";

// useSeedWatcher triggers a toast when a torrent starts SEEDING, on ANY page
// (M1). Reuses the `seeding` activity trail (ADR-022), whose transition detection
// DOWNLOADING→SEEDING is already done on the server (SyncStates) — more reliable than differentiating
// status on the client. The 1st pass only "primes" existing ones (doesn't notify history);
// from then on, every new `seeding` becomes a toast. Best-effort: network errors are silent.
function useSeedWatcher(authed: boolean) {
  const t = useT();
  const seen = useRef<Set<string>>(new Set());
  const primed = useRef(false);
  useEffect(() => {
    if (!authed) {
      seen.current = new Set();
      primed.current = false;
      return;
    }
    let stopped = false;
    const tick = async () => {
      try {
        const acts = await getActivities(50);
        const seeds = acts.filter((a) => a.kind === "seeding");
        if (!primed.current) {
          seeds.forEach((a) => seen.current.add(a.id)); // logs the history without notifying
          primed.current = true;
          return;
        }
        // Notifies the new ones in chronological order (the API returns the most recent first).
        for (const a of [...seeds].reverse()) {
          if (seen.current.has(a.id)) continue;
          seen.current.add(a.id);
          toast(t("app.app.seedWatcher.started", { name: a.torrent_name || "Torrent" }), "success");
        }
      } catch {
        /* watcher best-effort: doesn't pollute the screen with transient network errors */
      }
    };
    tick();
    const id = setInterval(() => !stopped && tick(), 8000);
    return () => {
      stopped = true;
      clearInterval(id);
    };
  }, [authed, t]);
}

/**
 * Redirect: replaces the current route in place, without stacking history. Uses
 * `location.replace` on purpose: the router's `navigate()` stacks, and an operator who
 * bookmarked #/nodes would get stuck — going back would hit the redirect again.
 * (`history.replaceState` doesn't work: it doesn't trigger `hashchange`, and the UI would freeze.)
 */
function Redirect({ to }: { to: string }) {
  useEffect(() => {
    window.location.replace(`#${to}`);
  }, [to]);
  return null;
}

export function App() {
  const [authed, setAuthed] = useState(() => !!getToken());
  const [me, setMe] = useState<Me | null>(null);
  // meReady: the profile hasn't resolved yet (or failed and will be retried). While
  // it doesn't resolve, the permission gate isn't applied — otherwise the sidebar disappears and
  // every route falls to the Dashboard until getMe finishes (ADR-035 review, bug #5).
  const [meReady, setMeReady] = useState(false);
  const route = useRoute();
  useSeedWatcher(authed);

  // Operator profile (ADR-035): roles/permissions to filter the UI. With retry
  // — transient network failure shouldn't hide the menu forever.
  useEffect(() => {
    if (!authed) {
      setMe(null);
      setMeReady(true);
      return;
    }
    let stopped = false;
    let attempt = 0;
    const load = () => {
      setMeReady(false);
      getMe()
        .then((m) => {
          if (stopped) return;
          setMe(m);
          setMeReady(true);
        })
        .catch(() => {
          if (stopped) return;
          attempt += 1;
          if (attempt >= 3) {
            // Retries exhausted: releases the UI without a profile (backend blocks
            // actions without permission; better than a blank screen forever).
            setMeReady(true);
            return;
          }
          setTimeout(load, 1000 * attempt);
        });
    };
    load();
    return () => {
      stopped = true;
    };
  }, [authed]);

  if (!authed) return <Login onLogin={() => setAuthed(true)} />;

  const logout = () => {
    clearToken();
    setMe(null);
    setMeReady(false);
    setAuthed(false);
  };

  // While the profile doesn't resolve, it shows the Dashboard (loading) without applying
  // the gate — the Layout renders the full nav, and protected routes fall to the
  // backend if the operator lacks the permission.
  const can = (perm: string) => (meReady ? hasPermission(me, perm) : true);

  // The Dashboard is the fallback page for every denied route, so any role reaches it:
  // it receives `can` to hide buttons the role can't use (button-level gate).
  const dashboard = <Dashboard can={can} onLogout={logout} />;

  const nodeDetail = route.match(/^\/nodes\/(.+)$/);
  const torrentDetail = route.match(/^\/torrents\/(.+)$/);
  let page;
  if (nodeDetail) page = <NodeDetail nodeId={nodeDetail[1]} can={can} onLogout={logout} />;
  else if (torrentDetail) page = <TorrentDetail torrentId={torrentDetail[1]} can={can} onLogout={logout} />;
  // /nodes (list) was absorbed by the Health screen. Redirects instead of becoming an alias:
  // keeping the old URL rendering another page would make the address lie and the sidebar
  // highlight point to the wrong item. `/nodes/` with a slash is tested separately because the
  // detail regex requires at least one character and wouldn't match — it would fall to the Dashboard.
  else if (route === "/nodes" || route === "/nodes/") page = <Redirect to="/health" />;
  else if (route === "/downloaded")
    page = can("torrents:read") ? <Downloaded can={can} onLogout={logout} /> : dashboard;
  else if (route === "/analytics")
    page = can("torrents:read") ? <Analytics onLogout={logout} /> : dashboard;
  else if (route === "/add") page = can("torrents:create") ? <AddTorrent onLogout={logout} /> : dashboard;
  else if (route === "/publish") page = can("torrents:publish") ? <Publish can={can} onLogout={logout} /> : dashboard;
  else if (route === "/explore") page = can("torrents:read") ? <Explore can={can} onLogout={logout} /> : dashboard;
  else if (route === "/alerts") page = can("alerts:read") ? <AlertsEvents can={can} onLogout={logout} /> : dashboard;
  else if (route === "/feed") page = can("activities:read") ? <Feed onLogout={logout} /> : dashboard;
  else if (route === "/health")
    page = can("metrics:read") ? <HealthStatus can={can} onLogout={logout} /> : dashboard;
  else if (route === "/settings") page = can("settings:write") ? <Settings can={can} onLogout={logout} /> : dashboard;
  else if (route === "/audit") page = can("audit:read") ? <AuditPage onLogout={logout} /> : dashboard;
  else if (route === "/users") page = can("users:read") ? <Users /> : dashboard;
  else if (route === "/profile") page = <Profile onLogout={logout} />;
  else page = dashboard;

  // A global toast viewport (bottom right corner) for every action/confirmation, and the
  // dialog for askConfirm's questions (#45).
  return (
    <>
      <Layout me={me} meReady={meReady} onLogout={logout}>
        {page}
      </Layout>
      <ToastViewport />
      <ConfirmHost />
    </>
  );
}
