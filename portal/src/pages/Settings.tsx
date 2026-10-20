import { useEffect, useState, useCallback } from "react";
import {
  getSettings,
  updateSetting,
  resetSetting,
  notifyTest,
  ApiError,
  type SettingGroup,
  type SettingItem,
} from "../api";
import { TrackersPanel } from "../components/TrackersPanel";
import { NodeOverridesPanel } from "../components/NodeOverridesPanel";
import { chanMatch, isChannelCustom, isChannelRule } from "../lib/notify-keys";
import { useT, rich, type Key } from "../i18n";
import { categoryName, choiceLabel, settingText } from "../lib/setting-text";
import { askConfirm } from "../components/ConfirmDialog";

// Settings panel (ADR-023): exposes the behavior tunables of the Control
// Plane, editable at runtime (hot-reload — applies without redeploy). Each field shows the
// catalog descriptor; "modified" indicates a database override (source === "db").
//
// Two categories get a SUB-DIVISION by service/channel (ADR-028/029): instead of a
// flat list, Telemetry becomes Telegram/Discord/Webhook cards (+ Send rules) and
// Integrations becomes TMDB/IGDB/imgbb/Google Books — each credential card with an
// enabled/
// disabled badge, and whatever doesn't match any card falls into "Other" to never disappear from the screen.
// The structure mirrors how the operator thinks about settings, not the catalog order.

type Sub = {
  titleKey: Key;
  blurbKey: Key;
  match: (key: string) => boolean;
  credential?: boolean;
  channel?: string; // if set, the card gets a "Test" button for that channel (ADR-028)
};

const SUBSECTIONS: Record<string, Sub[]> = {
  notify: [
    {
      titleKey: "settings.settings.subsections.notify.telegram.title",
      blurbKey: "settings.settings.subsections.notify.telegram.blurb",
      match: chanMatch("telegram"),
      credential: true,
      channel: "telegram",
    },
    {
      titleKey: "settings.settings.subsections.notify.discord.title",
      blurbKey: "settings.settings.subsections.notify.discord.blurb",
      match: chanMatch("discord"),
      credential: true,
      channel: "discord",
    },
    {
      titleKey: "settings.settings.subsections.notify.webhook.title",
      blurbKey: "settings.settings.subsections.notify.webhook.blurb",
      match: chanMatch("webhook"),
      credential: true,
      channel: "webhook",
    },
    {
      titleKey: "settings.settings.subsections.notify.rules.title",
      blurbKey: "settings.settings.subsections.notify.rules.blurb",
      match: (k) =>
        k.startsWith("notify.alerts_") ||
        k === "notify.events" ||
        k === "notify.activities" ||
        k === "notify.rate_limit_per_min",
    },
  ],
  integrations: [
    {
      titleKey: "settings.settings.subsections.integrations.tmdb.title",
      blurbKey: "settings.settings.subsections.integrations.tmdb.blurb",
      match: (k) => k.startsWith("integrations.tmdb_"),
      credential: true,
    },
    {
      titleKey: "settings.settings.subsections.integrations.igdb.title",
      blurbKey: "settings.settings.subsections.integrations.igdb.blurb",
      match: (k) => k.startsWith("integrations.igdb_"),
      credential: true,
    },
    {
      // ADR-030 replaced ptpimg (went offline) with imgbb, but this card kept
      // matching "integrations.ptpimg_" — which no longer exists in the catalog. Result: the
      // imgbb key became INVISIBLE in the Portal, without anyone noticing.
      titleKey: "settings.settings.subsections.integrations.imgbb.title",
      blurbKey: "settings.settings.subsections.integrations.imgbb.blurb",
      match: (k) => k.startsWith("integrations.imgbb_"),
      credential: true,
    },
    {
      titleKey: "settings.settings.subsections.integrations.googleBooks.title",
      blurbKey: "settings.settings.subsections.integrations.googleBooks.blurb",
      match: (k) => k.startsWith("integrations.google_books_"),
      credential: true,
    },
  ],
};

const CATEGORY_BLURB_KEY: Record<string, Key> = {
  admission: "settings.settings.categories.admission",
  agent: "settings.settings.categories.agent",
  alerting: "settings.settings.categories.alerting",
  notify: "settings.settings.categories.notify",
  integrations: "settings.settings.categories.integrations",
  workers: "settings.settings.categories.workers",
  publish: "settings.settings.categories.publish",
};

// "Filled" for badge/state purposes: secret → has_value; text → not empty.
function filled(it: SettingItem): boolean {
  return it.secret ? !!it.has_value : it.value.trim() !== "";
}


// SettingField — an editable row. Control by type: select for bool and enum, password
// for secret (value never comes from the API), numeric/text input for the rest.
function SettingField({
  item,
  onSaved,
  onLogout,
}: {
  item: SettingItem;
  onSaved: (updated: SettingItem) => void;
  onLogout: () => void;
}) {
  const t = useT();
  const [value, setValue] = useState(item.value);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);

  useEffect(() => {
    setValue(item.value);
  }, [item.value]);

  const multi = !!item.multi && !!item.choices?.length;
  // Selected set of a multi field (CSV). "*" = all; expands to compare.
  const allVals = item.choices?.map((c) => c.value) ?? [];
  const selectedSet = (csv: string): Set<string> => {
    const tVal = csv.trim();
    if (tVal === "*") return new Set(allVals);
    return new Set(tVal.split(",").map((s) => s.trim()).filter(Boolean));
  };
  const sel = selectedSet(value);
  const toggle = (code: string) => {
    const next = new Set(sel);
    next.has(code) ? next.delete(code) : next.add(code);
    // Always persists explicitly (never rewrites "*"), in catalog order.
    setValue(allVals.filter((v) => next.has(v)).join(","));
  };
  const sameSel = (a: string, b: string) => {
    const sa = selectedSet(a);
    const sb = selectedSet(b);
    return sa.size === sb.size && [...sa].every((x) => sb.has(x));
  };

  // Secret: field starts empty (value doesn't come), and "save empty = keep" — so it
  // gets dirty with any text; the others compare with the current value.
  const dirty = multi
    ? !sameSel(value, item.value)
    : item.secret
      ? value !== ""
      : value.trim() !== item.value;
  const modified = item.source === "db";
  const numeric = item.type === "int" || item.type === "int64" || item.type === "float";

  const run = useCallback(
    async (action: () => Promise<SettingItem>) => {
      setBusy(true);
      setMsg(null);
      try {
        onSaved(await action());
        setMsg({ kind: "ok", text: t("settings.settings.messages.applied") });
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return onLogout();
        setMsg({ kind: "err", text: err instanceof ApiError ? err.message : t("settings.settings.messages.saveFailed") });
      } finally {
        setBusy(false);
      }
    },
    [onSaved, onLogout, t],
  );

  // Form-control base classes (#22): sg-field, plus sg-select on the lists.
  const inputCls = "sg-field";

  // CLUSTER value of an Agent/Engine tunable (ADR-039) reaches ALL Nodes without their
  // own value on the next heartbeat — it's the broadest change on this screen, and the one #144
  // exists to make conscious. Asks for confirmation before saving or restoring.
  const confirmCluster = async (what: string, confirmLabel: string) =>
    !item.per_node ||
    askConfirm({ title: what, message: t("settings.settings.confirm.clusterScope"), confirmLabel });

  return (
    <div className="flex flex-col gap-1.5 py-3">
      <div className="flex items-baseline justify-between gap-3">
        <label className="text-sm font-medium text-slate-200">{settingText(item.key, "label", item.label)}</label>
        <span className="flex shrink-0 items-center gap-1.5">
          {item.per_node && (
            <span
              title={t("settings.settings.field.clusterBadgeTitle")}
              className="rounded bg-sky-600/15 px-1.5 py-0.5 text-[10px] font-medium text-sky-300 ring-1 ring-inset ring-sky-600/30"
            >
              {t("settings.settings.field.clusterBadge")}
            </span>
          )}
          {modified && (
            <span className="rounded bg-emerald-600/15 px-1.5 py-0.5 text-[10px] font-medium text-emerald-300 ring-1 ring-inset ring-emerald-600/30">
              {t("settings.settings.field.modified")}
            </span>
          )}
        </span>
      </div>
      <p className="text-xs leading-relaxed text-slate-500">{settingText(item.key, "description", item.description)}</p>

      {multi ? (
        <div className="mt-1 space-y-2">
          <div className="grid gap-x-4 gap-y-1.5 sm:grid-cols-2">
            {item.choices!.map((c) => (
              <label
                key={c.value}
                className="flex cursor-pointer items-center gap-2 text-sm text-slate-300"
              >
                <input
                  type="checkbox"
                  checked={sel.has(c.value)}
                  onChange={() => toggle(c.value)}
                  disabled={busy}
                  className="sg-check shrink-0"
                />
                {choiceLabel(item.key, c.value, c.label)}
              </label>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => run(() => updateSetting(item.key, value))}
              disabled={busy || !dirty}
              className="rounded-full bg-emerald-600 px-3 py-1.5 text-sm font-medium text-[#fff] transition hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {t("common.save")}
            </button>
            {sel.size > 0 && (
              <button
                onClick={() => setValue("")}
                disabled={busy}
                className="rounded-full border border-slate-700 px-3 py-1.5 text-sm text-slate-300 transition hover:bg-slate-800/60 disabled:opacity-40"
              >
                {t("settings.settings.actions.clear")}
              </button>
            )}
            <span className="text-xs text-slate-500">
              {sel.size === 0 ? t("settings.settings.field.nothingSelected") : t("settings.settings.field.selectedCount", { count: sel.size })}
            </span>
            {msg && (
              <span className={`text-xs ${msg.kind === "ok" ? "text-emerald-400" : "text-red-400"}`}>
                {msg.text}
              </span>
            )}
          </div>
        </div>
      ) : (
      <div className="mt-1 flex flex-wrap items-center gap-2">
        {item.type === "bool" ? (
          <select value={value} onChange={(e) => setValue(e.target.value)} disabled={busy} className={`w-40 ${inputCls} sg-select`}>
            <option value="true">{t("settings.settings.field.optionOn")}</option>
            <option value="false">{t("settings.settings.field.optionOff")}</option>
          </select>
        ) : item.options && item.options.length > 0 ? (
          <select value={value} onChange={(e) => setValue(e.target.value)} disabled={busy} className={`w-40 ${inputCls} sg-select`}>
            {item.options.map((o) => (
              <option key={o} value={o}>
                {o}
              </option>
            ))}
          </select>
        ) : (
          <input
            type={item.secret ? "password" : numeric ? "number" : "text"}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            disabled={busy}
            autoComplete={item.secret ? "new-password" : undefined}
            placeholder={
              item.secret
                ? item.has_value
                  ? t("settings.settings.field.placeholderSecretChange")
                  : t("settings.settings.field.placeholderNotConfigured")
                : item.type === "duration"
                  ? t("settings.settings.field.placeholderDuration")
                  : ""
            }
            className={`${item.secret ? "w-72" : numeric ? "w-40" : "w-72"} ${inputCls} font-mono`}
          />
        )}
        {item.unit && <span className="text-xs text-slate-500">{item.unit}</span>}
        <button
          onClick={async () => {
            const next = item.secret ? value : value.trim();
            const shown = item.type === "bool" ? (next === "true" ? t("settings.settings.field.optionOn") : t("settings.settings.field.optionOff")) : next;
            if (await confirmCluster(t("settings.settings.confirm.changeCluster", { label: settingText(item.key, "label", item.label), value: shown, unit: item.unit ? ` ${item.unit}` : "" }), t("common.save"))) {
              run(() => updateSetting(item.key, next));
            }
          }}
          disabled={busy || !dirty}
          className="rounded-full bg-emerald-600 px-3 py-1.5 text-sm font-medium text-[#fff] transition hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {t("common.save")}
        </button>
        {modified && (
          <button
            onClick={async () => {
              if (await confirmCluster(t("settings.settings.confirm.resetCluster", { label: settingText(item.key, "label", item.label), defaultValue: item.default || t("settings.settings.field.emptyValue") }), t("settings.settings.confirm.restore"))) {
                run(() => resetSetting(item.key));
              }
            }}
            disabled={busy}
            className="rounded-full border border-slate-700 px-3 py-1.5 text-sm text-slate-300 transition hover:bg-slate-800/60 disabled:opacity-40"
            title={t("settings.settings.actions.resetDefaultTitle")}
          >
            {t("settings.settings.actions.resetDefault")}
          </button>
        )}
        {msg && (
          <span className={`text-xs ${msg.kind === "ok" ? "text-emerald-400" : "text-red-400"}`}>{msg.text}</span>
        )}
      </div>
      )}
    </div>
  );
}

// Service/channel card: title, a one-liner of what it is, enabled/disabled badge (only for
// credentials) and the fields, separated by subtle dividers.
function ServiceCard({
  sub,
  items,
  onSaved,
  onLogout,
}: {
  sub: Sub;
  items: SettingItem[];
  onSaved: (u: SettingItem) => void;
  onLogout: () => void;
}) {
  const t = useT();
  // The badge looks ONLY at credentials. Including per-channel rules would break it: they are born
  // empty (notify.<ch>.events = ""), and a Telegram with token and chat_id filled
  // would appear as "disabled".
  const creds = items.filter((it) => !isChannelRule(it.key) && !isChannelCustom(it.key));
  const on = creds.length > 0 && creds.every(filled);

  // Per-channel rules only apply when `custom` is enabled — which is what the Control Plane
  // does in notify.filtersFor(). Showing them disabled would offer control that the
  // backend ignores, and it's what stretched the card. With custom off, everything disappears except the
  // switch itself.
  const custom = items.find((it) => isChannelCustom(it.key));
  const useOwn = custom ? custom.value === "true" : true;
  const shown = useOwn ? items : items.filter((it) => !isChannelRule(it.key));

  return (
    <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-slate-100">{t(sub.titleKey)}</h3>
        {sub.credential && (
          <span
            className={`rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset ${
              on
                ? "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30"
                : "bg-slate-500/15 text-slate-400 ring-slate-500/30"
            }`}
          >
            {on ? t("settings.settings.status.on") : t("settings.settings.status.off")}
          </span>
        )}
      </div>
      <p className="mt-0.5 text-xs text-slate-500">{t(sub.blurbKey)}</p>
      <div className="mt-1 divide-y divide-slate-800/70">
        {shown.map((it) => (
          <SettingField key={it.key} item={it} onSaved={onSaved} onLogout={onLogout} />
        ))}
      </div>
      {sub.channel && <ChannelTestButton channel={sub.channel} enabled={on} onLogout={onLogout} />}
    </div>
  );
}

// "Test" button for ONE channel (ADR-028). It proves that channel's token is correct,
// and says which one failed — without this, the operator only finds out at the first incident. Disabled
// when the channel is disabled (no credential).
function ChannelTestButton({
  channel,
  enabled,
  onLogout,
}: {
  channel: string;
  enabled: boolean;
  onLogout: () => void;
}) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ kind: "ok" | "err"; text: string } | null>(null);
  const send = async () => {
    setBusy(true);
    setMsg(null);
    try {
      await notifyTest(channel);
      setMsg({ kind: "ok", text: t("settings.settings.messages.testSent") });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return onLogout();
      setMsg({ kind: "err", text: err instanceof ApiError ? err.message : t("settings.settings.messages.sendFailed") });
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="mt-3 flex items-center gap-2 border-t border-slate-800/70 pt-3">
      <button
        onClick={send}
        disabled={busy || !enabled}
        className="rounded-full border border-emerald-600/40 bg-emerald-600/10 px-3 py-1.5 text-sm font-medium text-emerald-300 transition hover:bg-emerald-600/20 disabled:cursor-not-allowed disabled:opacity-40"
        title={enabled ? t("settings.settings.actions.testEnabledTitle") : t("settings.settings.actions.testDisabledTitle")}
      >
        {busy ? t("settings.settings.actions.sending") : t("settings.settings.actions.test")}
      </button>
      {msg && (
        <span className={`text-xs ${msg.kind === "ok" ? "text-emerald-400" : "text-red-400"}`}>{msg.text}</span>
      )}
    </div>
  );
}

// Renders a category: sub-divided into cards (notify/integrations) or a simple list.
function CategorySection({
  group,
  onSaved,
  onLogout,
}: {
  group: SettingGroup;
  onSaved: (u: SettingItem) => void;
  onLogout: () => void;
}) {
  const t = useT();
  const subs = SUBSECTIONS[group.category];
  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-800 pb-2">
        <div>
          <h2 className="text-base font-semibold text-white">{categoryName(group.category, group.label)}</h2>
          {CATEGORY_BLURB_KEY[group.category] && (
            <p className="text-xs text-slate-500">{t(CATEGORY_BLURB_KEY[group.category])}</p>
          )}
        </div>
      </div>

      {subs ? renderSubs(subs, group, onSaved, onLogout) : (
        <div className="grid gap-x-6 rounded-xl border border-slate-800 bg-slate-900/40 px-4 lg:grid-cols-2">
          {group.items.map((it) => (
            <SettingField key={it.key} item={it} onSaved={onSaved} onLogout={onLogout} />
          ))}
        </div>
      )}

      {/* Agent / Engine (ADR-039): next to the cluster value, WHO has their own value — which
          answers "if I change this, who isn't affected?". */}
      {group.items.some((it) => it.per_node) && <NodeOverridesPanel onLogout={onLogout} />}
    </section>
  );
}

// renderSubs draws the sub-section cards. When there's a NON-credential sub (the "Send
// rules" of Telemetry), it uses two columns: channels stacked on the left, rules alone on the
// right. Without this (Integrations — credentials only), it falls into a 2xN grid.
function renderSubs(
  subs: Sub[],
  group: SettingGroup,
  onSaved: (u: SettingItem) => void,
  onLogout: () => void,
) {
  const card = (sub: Sub) => {
    const items = group.items.filter((it) => sub.match(it.key));
    if (items.length === 0) return null;
    return <ServiceCard key={sub.titleKey} sub={sub} items={items} onSaved={onSaved} onLogout={onLogout} />;
  };

  // Safety net: a tunable that doesn't match ANY card falls here instead of
  // disappearing from the screen. Without this, the card list (which lives in the Portal) had to be
  // updated along with the catalog (which lives in the Control Plane) — and when it wasn't, the
  // key became unreachable without anyone noticing: it happened with imgbb (ADR-030) and
  // with the Google Books one.
  const leftover = group.items.filter((it) => !subs.some((sub) => sub.match(it.key)));
  const cardLeftover = leftover.length > 0 && (
    <ServiceCard
      key="__sobra"
      sub={{
        titleKey: "settings.settings.subsections.other.title",
        blurbKey: "settings.settings.subsections.other.blurb",
        match: () => true,
        credential: subs.every((x) => x.credential),
      }}
      items={leftover}
      onSaved={onSaved}
      onLogout={onLogout}
     
    />
  );
  const rules = subs.filter((s) => !s.credential);
  if (rules.length === 0) {
    return (
      <div className="grid gap-3 lg:grid-cols-2">
        {subs.map(card)}
        {cardLeftover}
      </div>
    );
  }
  const channels = subs.filter((s) => s.credential);
  return (
    <div className="grid items-start gap-3 lg:grid-cols-2">
      <div className="space-y-3">{channels.map(card)}</div>
      <div className="space-y-3">
        {rules.map(card)}
        {cardLeftover}
      </div>
    </div>
  );
}

export function Settings({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [groups, setGroups] = useState<SettingGroup[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      setGroups(await getSettings());
      setError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return onLogout();
      setError(t("settings.settings.messages.loadFailed"));
    } finally {
      setLoading(false);
    }
  }, [onLogout, t]);

  useEffect(() => {
    load();
  }, [load]);

  // When saving/restoring an item, replaces it in the list in place (without reloading everything).
  const onSaved = useCallback((updated: SettingItem) => {
    setGroups((prev) =>
      prev.map((g) => ({ ...g, items: g.items.map((it) => (it.key === updated.key ? updated : it)) })),
    );
  }, []);

  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <div>
        <h1 className="text-xl font-semibold text-white">{t("settings.settings.page.title")}</h1>
        <p className="mt-1 text-sm text-slate-400">
          {rich(t("settings.settings.page.intro"), {
            m5: <span className="font-mono text-slate-300">5m</span>,
            s90: <span className="font-mono text-slate-300">90s</span>,
            h168: <span className="font-mono text-slate-300">168h</span>,
          })}
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          {error}
        </div>
      )}
      {loading && !groups.length && <p className="text-sm text-slate-500">{t("common.loading")}</p>}

      {groups.map((g) => (
        <CategorySection key={g.category} group={g} onSaved={onSaved} onLogout={onLogout} />
      ))}

      {/* Announce tracker registration (ADR-019/020) — part of Announce. */}
      <section className="space-y-3">
        <div className="border-b border-slate-800 pb-2">
          <h2 className="text-base font-semibold text-white">{t("settings.settings.trackers.title")}</h2>
          <p className="text-xs text-slate-500">{t("settings.settings.trackers.blurb")}</p>
        </div>
        <TrackersPanel can={can} onLogout={onLogout} />
      </section>
    </div>
  );
}
