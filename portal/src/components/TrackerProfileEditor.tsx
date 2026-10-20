import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ApiError,
  applyTaxonomy,
  discoverTaxonomy,
  saveTrackerProfile,
  type ApplyResult,
  type DiscoveryResult,
  type ProposalItem,
  type TaxonomyAxis,
  type TaxonomyTerm,
  type TermInput,
  type TermKind,
  type TermOrigin,
  type Tracker,
} from "../api";
import { rich, useT, type Key, type TFn } from "../i18n";
import { TrackerDescriptionEditor } from "./TrackerDescriptionEditor";
import { TrackerImageHostEditor } from "./TrackerImageHostEditor";
import { TrackerProfileExchange } from "./TrackerProfileExchange";
import {
  AXES,
  KIND_LABEL_KEY,
  STOP_REASON_KEY,
  buildApplyItems,
  defaultSelection,
  isApplicable,
  proposalCounts,
  proposalKey,
  termsOf,
} from "../lib/taxonomy";

// Site profile editor for a tracker (ADR-037, #196): the taxonomy (category IDs,
// type and resolution) offered by the publishing panel, and discovery from the tracker's
// own catalog.
//
// Two ideas the screen must keep honest:
//  1. Discovery only PROPOSES. Nothing is saved until the operator reviews (and edits) and applies it.
//  2. Discovery only sees what has already been posted: an unused category does not appear.
//     Discovered profile is NOT complete by definition — the screen states this, rather than suggesting
//     that "discovering" solves everything.

type DraftTerm = {
  key: string; // row identity on the screen (the id is editable on new rows)
  id: string; // input text; existing rows have a fixed id
  name: string;
  kind: "" | TermKind;
  origin?: TermOrigin; // absent = new row, not yet saved
  existing: boolean;
};
type Draft = Record<TaxonomyAxis, DraftTerm[]>;

let rowSeq = 0;
const newKey = () => `row-${++rowSeq}`;

function toDraft(tracker: Tracker): Draft {
  const mk = (t: TaxonomyTerm): DraftTerm => ({
    key: newKey(),
    id: String(t.id),
    name: t.name,
    kind: t.kind ?? "",
    origin: t.origin,
    existing: true,
  });
  return {
    category: termsOf(tracker.profile, "category").map(mk),
    type: termsOf(tracker.profile, "type").map(mk),
    resolution: termsOf(tracker.profile, "resolution").map(mk),
  };
}

function toInputs(rows: DraftTerm[], axis: TaxonomyAxis, t: TFn): { terms: TermInput[]; error: string } {
  const seen = new Set<number>();
  const terms: TermInput[] = [];
  for (const r of rows) {
    const id = Number(r.id);
    if (!Number.isInteger(id) || id < 1) return { terms, error: t("trackers.trackerProfileEditor.errors.invalidId", { axis, id: r.id }) };
    if (seen.has(id)) return { terms, error: t("trackers.trackerProfileEditor.errors.duplicateId", { axis, id }) };
    seen.add(id);
    const name = r.name.trim();
    if (!name) return { terms, error: t("trackers.trackerProfileEditor.errors.missingName", { axis, id }) };
    terms.push(axis === "category" && r.kind ? { id, name, kind: r.kind } : { id, name });
  }
  return { terms, error: "" };
}

type OriginBadgeDef = { labelKey: Key; cls: string; titleKey: Key };

const ORIGIN_BADGE: Record<TermOrigin, OriginBadgeDef> = {
  seed: {
    labelKey: "trackers.trackerProfileEditor.origins.seed.label",
    cls: "bg-slate-500/15 text-slate-300 ring-slate-500/30",
    titleKey: "trackers.trackerProfileEditor.origins.seed.title",
  },
  discovered: {
    labelKey: "trackers.trackerProfileEditor.origins.discovered.label",
    cls: "bg-sky-500/15 text-sky-300 ring-sky-500/30",
    titleKey: "trackers.trackerProfileEditor.origins.discovered.title",
  },
  manual: {
    labelKey: "trackers.trackerProfileEditor.origins.manual.label",
    cls: "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30",
    titleKey: "trackers.trackerProfileEditor.origins.manual.title",
  },
  imported: {
    labelKey: "trackers.trackerProfileEditor.origins.imported.label",
    cls: "bg-violet-500/15 text-violet-300 ring-violet-500/30",
    titleKey: "trackers.trackerProfileEditor.origins.imported.title",
  },
};

// Form-control base classes (#22).
const inputCls = "sg-field w-full";

export function TrackerProfileEditor({
  tracker,
  can,
  onChanged,
  onLogout,
}: {
  tracker: Tracker;
  /** Button gate: export/import only appear for those with route permission. */
  can: (perm: string) => boolean;
  onChanged: () => void | Promise<void>;
  onLogout: () => void;
}) {
  const t = useT();
  const [draft, setDraft] = useState<Draft>(() => toDraft(tracker));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  // Description composition (#192) has its own save; we only need to know if there is something to
  // save there, since exporting and importing the profile would mess with it underneath.
  const [descDirty, setDescDirty] = useState(false);
  // The image host (#193) also has its own save — same logic as the description.
  const [imageHostDirty, setImageHostDirty] = useState(false);

  // Discovery.
  const [pages, setPages] = useState(8);
  const [discovering, setDiscovering] = useState(false);
  const [disc, setDisc] = useState<DiscoveryResult | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [edited, setEdited] = useState<Record<string, string>>({});
  const [applying, setApplying] = useState(false);
  const [applied, setApplied] = useState<ApplyResult | null>(null);

  // Profile changed on the server (saved, applied): screen reflects the saved state. The key is the
  // profile CONTENT, not the object's identity — reloading the list for another reason
  // (making another tracker default, for example) must not throw away what the operator typed.
  const profileSig = JSON.stringify(tracker.profile);
  useEffect(() => {
    setDraft(toDraft(tracker));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profileSig]);

  const hasAPI = !!tracker.api_url && tracker.has_api_token;

  const dirty = useMemo(() => {
    const strip = (d: Draft) =>
      JSON.stringify(
        AXES.map(({ axis }) => d[axis].map((r) => [r.id.trim(), r.name.trim(), r.kind])),
      );
    return strip(draft) !== strip(toDraft(tracker));
  }, [draft, tracker]);

  const fail = useCallback(
    (err: unknown, fallback: string) => {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(err instanceof ApiError ? err.message : fallback);
    },
    [onLogout],
  );

  function patchRow(axis: TaxonomyAxis, key: string, patch: Partial<DraftTerm>) {
    setDraft((d) => ({ ...d, [axis]: d[axis].map((r) => (r.key === key ? { ...r, ...patch } : r)) }));
  }
  function removeRow(axis: TaxonomyAxis, key: string) {
    setDraft((d) => ({ ...d, [axis]: d[axis].filter((r) => r.key !== key) }));
  }
  function addRow(axis: TaxonomyAxis) {
    setDraft((d) => ({
      ...d,
      [axis]: [...d[axis], { key: newKey(), id: "", name: "", kind: "", existing: false }],
    }));
  }

  async function save() {
    setError("");
    setNotice("");
    const body: Partial<Record<TaxonomyAxis, TermInput[]>> = {};
    for (const { axis } of AXES) {
      const { terms, error: e } = toInputs(draft[axis], axis, t);
      if (e) {
        setError(e);
        return;
      }
      body[axis] = terms;
    }
    setSaving(true);
    try {
      await saveTrackerProfile(tracker.id, body);
      setNotice(t("trackers.trackerProfileEditor.notices.saved"));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerProfileEditor.errors.saveFailed"));
    } finally {
      setSaving(false);
    }
  }

  async function discover() {
    setError("");
    setNotice("");
    setApplied(null);
    setDiscovering(true);
    try {
      const res = await discoverTaxonomy(tracker.id, pages);
      setDisc(res);
      setSelected(defaultSelection(res.proposal));
      setEdited({});
    } catch (err) {
      setDisc(null);
      fail(err, t("trackers.trackerProfileEditor.errors.discoverFailed"));
    } finally {
      setDiscovering(false);
    }
  }

  const items = useMemo(
    () => (disc ? buildApplyItems(disc.proposal, selected, edited) : []),
    [disc, selected, edited],
  );

  async function apply() {
    if (!disc || items.length === 0) return;
    setError("");
    setApplying(true);
    try {
      const res = await applyTaxonomy(tracker.id, items);
      setApplied(res);
      setDisc(null);
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerProfileEditor.errors.applyFailed"));
    } finally {
      setApplying(false);
    }
  }

  function toggle(key: string) {
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(key)) n.delete(key);
      else n.add(key);
      return n;
    });
  }

  return (
    <div className="space-y-4 border-t border-slate-800 bg-slate-950/40 px-4 py-4">
      <div>
        <p className="text-xs font-medium uppercase tracking-wide text-slate-500">
          {t("trackers.trackerProfileEditor.header.title")}{" "}
          <span className="ml-1 rounded-full bg-slate-800 px-2 py-0.5 normal-case text-slate-400">
            {tracker.profile.family} · formato v{tracker.profile.schema_version}
          </span>
        </p>
        <p className="mt-1 text-xs text-slate-400">
          {rich(t("trackers.trackerProfileEditor.header.description"))}
        </p>
      </div>

      {AXES.map(({ axis, labelKey }) => (
        <div key={axis} className="space-y-1.5">
          <p className="text-xs font-medium text-slate-300">{t(labelKey)}</p>
          {draft[axis].length === 0 && <p className="text-xs text-slate-500">{t("trackers.trackerProfileEditor.rows.empty")}</p>}
          <div className="space-y-1">
            {draft[axis].map((r) => (
              <div key={r.key} className="flex items-center gap-2">
                <input
                  value={r.id}
                  onChange={(e) => patchRow(axis, r.key, { id: e.target.value })}
                  readOnly={r.existing}
                  inputMode="numeric"
                  placeholder={t("trackers.trackerProfileEditor.rows.idPlaceholder")}
                  aria-label={t("trackers.trackerProfileEditor.rows.idAria", { label: t(labelKey) })}
                  className={`${inputCls} w-16 shrink-0 font-mono text-xs ${r.existing ? "opacity-60" : ""}`}
                />
                <input
                  value={r.name}
                  onChange={(e) => patchRow(axis, r.key, { name: e.target.value })}
                  placeholder={t("trackers.trackerProfileEditor.rows.namePlaceholder")}
                  aria-label={t("trackers.trackerProfileEditor.rows.nameAria", { label: t(labelKey) })}
                  maxLength={64}
                  className={inputCls}
                />
                {axis === "category" && (
                  <select
                    value={r.kind}
                    onChange={(e) => patchRow(axis, r.key, { kind: e.target.value as DraftTerm["kind"] })}
                    aria-label={t("trackers.trackerProfileEditor.rows.kindAria")}
                    title={t("trackers.trackerProfileEditor.rows.kindTitle")}
                    className={`${inputCls} sg-select w-24 shrink-0 text-xs`}
                  >
                    <option value="">—</option>
                    {(Object.keys(KIND_LABEL_KEY) as TermKind[]).map((k) => (
                      <option key={k} value={k}>
                        {t(KIND_LABEL_KEY[k])}
                      </option>
                    ))}
                  </select>
                )}
                {r.origin ? (
                  <span
                    title={t(ORIGIN_BADGE[r.origin].titleKey)}
                    className={`w-20 shrink-0 rounded-full px-2 py-0.5 text-center text-[11px] ring-1 ring-inset ${ORIGIN_BADGE[r.origin].cls}`}
                  >
                    {t(ORIGIN_BADGE[r.origin].labelKey)}
                  </span>
                ) : (
                  <span className="w-20 shrink-0 text-center text-[11px] text-slate-500">{t("trackers.trackerProfileEditor.rows.newBadge")}</span>
                )}
                <button
                  onClick={() => removeRow(axis, r.key)}
                  title={t("trackers.trackerProfileEditor.rows.removeTitle")}
                  className="shrink-0 rounded-full px-1.5 py-1 text-xs text-slate-500 hover:bg-slate-800 hover:text-red-400"
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
          <button onClick={() => addRow(axis)} className="text-xs text-sky-300 hover:text-sky-200">
            {t("trackers.trackerProfileEditor.rows.add", { label: t(labelKey).toLowerCase() })}
          </button>
        </div>
      ))}

      <div className="flex flex-wrap items-center gap-3">
        <button
          onClick={save}
          disabled={saving || !dirty}
          className="rounded-full bg-emerald-600 px-4 py-1.5 text-sm font-medium text-[#fff] transition hover:bg-emerald-500 disabled:opacity-40"
        >
          {saving ? t("common.saving") : t("trackers.trackerProfileEditor.actions.save")}
        </button>
        {dirty && <span className="text-xs text-amber-400">{t("trackers.trackerProfileEditor.status.unsaved")}</span>}
        {notice && <span className="text-xs text-emerald-400">{notice}</span>}
      </div>

      <TrackerDescriptionEditor
        tracker={tracker}
        can={can}
        onChanged={onChanged}
        onLogout={onLogout}
        onDirtyChange={setDescDirty}
      />

      <TrackerImageHostEditor
        tracker={tracker}
        can={can}
        onChanged={onChanged}
        onLogout={onLogout}
        onDirtyChange={setImageHostDirty}
      />

      <TrackerProfileExchange
        tracker={tracker}
        can={can}
        blocked={dirty || descDirty || imageHostDirty}
        onChanged={onChanged}
        onLogout={onLogout}
      />

      <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-950/60 p-3">
        <p className="text-xs font-medium text-slate-300">{t("trackers.trackerProfileEditor.discovery.title")}</p>
        <p className="text-xs text-slate-400">
          {rich(t("trackers.trackerProfileEditor.discovery.description"))}
        </p>
        <p className="rounded-lg border border-amber-700/40 bg-amber-500/5 px-3 py-2 text-xs text-amber-300">
          {rich(t("trackers.trackerProfileEditor.discovery.warning"))}
        </p>
        {hasAPI ? (
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2 text-xs text-slate-400">
              {t("trackers.trackerProfileEditor.discovery.pagesLabel")}
              <input
                type="number"
                min={1}
                max={20}
                value={pages}
                onChange={(e) => setPages(Math.max(1, Math.min(20, Number(e.target.value) || 1)))}
                className={`${inputCls} w-16`}
              />
            </label>
            <button
              onClick={discover}
              disabled={discovering || dirty}
              title={dirty ? t("trackers.trackerProfileEditor.discovery.dirtyTitle") : undefined}
              className="rounded-full bg-sky-700 px-4 py-1.5 text-sm font-medium text-[#fff] transition hover:bg-sky-600 disabled:opacity-40"
            >
              {discovering ? t("trackers.trackerProfileEditor.discovery.scanning") : t("trackers.trackerProfileEditor.discovery.button")}
            </button>
          </div>
        ) : (
          <p className="text-xs text-slate-500">
            {t("trackers.trackerProfileEditor.discovery.noApi")}
          </p>
        )}
      </div>

      {error && <p className="text-sm text-red-400">{error}</p>}

      {disc && (
        <Proposal
          disc={disc}
          selected={selected}
          edited={edited}
          count={items.length}
          applying={applying}
          onToggle={toggle}
          onEdit={(key, v) => setEdited((e) => ({ ...e, [key]: v }))}
          onApply={apply}
          onDiscard={() => setDisc(null)}
        />
      )}

      {applied && (
        <div className="rounded-xl border border-emerald-700/40 bg-emerald-500/5 px-3 py-2 text-xs text-emerald-300">
          {t("trackers.trackerProfileEditor.applied.summary", {
            added: applied.applied.filter((a) => a.action === "added").length,
            renamed: applied.applied.filter((a) => a.action === "renamed").length,
          })}
          {applied.skipped.some((s) => s.reason === "protected") &&
            ` ${t("trackers.trackerProfileEditor.applied.skipped", { count: applied.skipped.filter((s) => s.reason === "protected").length })}`}
        </div>
      )}
    </div>
  );
}

function Proposal({
  disc,
  selected,
  edited,
  count,
  applying,
  onToggle,
  onEdit,
  onApply,
  onDiscard,
}: {
  disc: DiscoveryResult;
  selected: Set<string>;
  edited: Record<string, string>;
  count: number;
  applying: boolean;
  onToggle: (key: string) => void;
  onEdit: (key: string, name: string) => void;
  onApply: () => void;
  onDiscard: () => void;
}) {
  const t = useT();
  const c = proposalCounts(disc.proposal);
  return (
    <div className="space-y-3 rounded-xl border border-sky-800/50 bg-sky-500/5 p-3">
      <div>
        <p className="text-sm font-medium text-white">{t("trackers.trackerProfileEditor.proposal.title")}</p>
        <p className="mt-0.5 text-xs text-slate-400">
          {rich(
            t("trackers.trackerProfileEditor.proposal.stats", {
              requested: disc.pages_requested,
              torrents: disc.items_scanned,
              reason: STOP_REASON_KEY[disc.stopped_reason] ? t(STOP_REASON_KEY[disc.stopped_reason]) : disc.stopped_reason,
              new: c.new,
              same: c.same,
              renamed: c.renamed,
            }),
            { scanned: <strong>{disc.pages_scanned}</strong> },
          )}
        </p>
        <p className="mt-1 text-xs text-amber-300">
          {t("trackers.trackerProfileEditor.proposal.reminder")}
        </p>
      </div>

      {disc.rejected > 0 && (
        <p className="text-xs text-amber-400">
          {t("trackers.trackerProfileEditor.proposal.rejected", { count: disc.rejected })}
        </p>
      )}
      {disc.warnings.map((w) => (
        <p key={w} className="text-xs text-amber-400">
          ⚠ {w}
        </p>
      ))}

      {AXES.map(({ axis, labelKey }) => {
        const rows: ProposalItem[] = disc.proposal[axis] ?? [];
        return (
          <div key={axis} className="space-y-1">
            <p className="text-xs font-medium text-slate-300">{t(labelKey)}</p>
            {rows.length === 0 && <p className="text-xs text-slate-500">{t("trackers.trackerProfileEditor.proposal.empty")}</p>}
            {rows.map((it) => {
              const key = proposalKey(axis, it.id);
              const ok = isApplicable(it);
              return (
                <div key={key} className="flex items-center gap-2 text-xs">
                  <input
                    type="checkbox"
                    checked={ok && selected.has(key)}
                    disabled={!ok}
                    onChange={() => onToggle(key)}
                    aria-label={t("trackers.trackerProfileEditor.proposal.applyAria", { label: t(labelKey), id: it.id })}
                    className="sg-check"
                  />
                  <span className="w-12 shrink-0 font-mono text-slate-400">{it.id}</span>
                  <input
                    value={edited[key] ?? it.name}
                    onChange={(e) => onEdit(key, e.target.value)}
                    disabled={!ok}
                    maxLength={64}
                    aria-label={t("trackers.trackerProfileEditor.proposal.nameAria", { label: t(labelKey), id: it.id })}
                    className={`${inputCls} min-w-0`}
                  />
                  <span className="w-44 shrink-0 text-slate-500">
                    {it.status === "new" && <span className="text-sky-300">{t("trackers.trackerProfileEditor.proposal.statusNew", { count: it.seen })}</span>}
                    {it.status === "same" && t("trackers.trackerProfileEditor.proposal.statusSame")}
                    {it.status === "renamed" && (
                      <span className={it.protected ? "text-emerald-400" : "text-amber-300"}>
                        {it.protected
                          ? t("trackers.trackerProfileEditor.proposal.statusProtected", { name: it.current_name ?? "" })
                          : t("trackers.trackerProfileEditor.proposal.statusRenamed", { name: it.current_name ?? "" })}
                      </span>
                    )}
                  </span>
                </div>
              );
            })}
          </div>
        );
      })}

      <div className="flex flex-wrap items-center gap-3">
        <button
          onClick={onApply}
          disabled={applying || count === 0}
          className="rounded-full bg-emerald-600 px-4 py-1.5 text-sm font-medium text-[#fff] transition hover:bg-emerald-500 disabled:opacity-40"
        >
          {applying ? t("trackers.trackerProfileEditor.proposal.applying") : t("trackers.trackerProfileEditor.proposal.applyButton", { count })}
        </button>
        <button onClick={onDiscard} className="text-xs text-slate-400 hover:text-white">
          {t("trackers.trackerProfileEditor.proposal.discard")}
        </button>
        <span className="text-xs text-slate-500">
          {rich(t("trackers.trackerProfileEditor.proposal.editHint"))}
        </span>
      </div>
    </div>
  );
}
