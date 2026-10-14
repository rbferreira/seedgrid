import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError, resetDescriptionConfig, saveDescriptionConfig, type DescriptionConfig, type Tracker } from "../api";
import {
  blockHint,
  CONTENT_LANGUAGES,
  KIND_TITLE_KEY,
  MEDIAINFO_TAGS,
  contentLanguage,
  blockLabel,
  describeConfig,
  effectiveConfig,
  fromDraft,
  moveBlock,
  sameConfig,
  toDraft,
  toggleBlock,
  type DescriptionDraft,
  type DescriptionKind,
} from "../lib/description";
import { rich, useT } from "../i18n";

// Upload description composition (ADR-037, #192): which blocks the publish panel assembles, in
// what order, and the tag wrapping MediaInfo. It is the `description` section of the site profile — per
// tracker, because two sites require different markup — and the text itself is composed by the Control Plane
// (POST /api/v1/publish/description). The operator keeps editing the field before publishing.
//
// The gate is by BUTTON (docs/11 - Portal.md §13.1): whoever can read the tracker SEES the composition; only
// those who can write (trackers:write, the permission for PUT/DELETE routes) see the edit controls.

const LANGUAGE_NAME_KEY = {
  en: "trackers.trackerDescriptionEditor.languages.en",
  "pt-BR": "trackers.trackerDescriptionEditor.languages.ptBR",
} as const;

const btnCls =
  "rounded-full border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-200 transition hover:border-sky-600 hover:text-sky-200 disabled:opacity-40";
const stepCls =
  "rounded px-1.5 py-0.5 text-xs text-slate-400 transition hover:bg-slate-800 hover:text-sky-300 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-slate-400";

export function TrackerDescriptionEditor({
  tracker,
  can,
  onChanged,
  onLogout,
  onDirtyChange,
}: {
  tracker: Tracker;
  /** Gate by button: edit controls only appear for those with the route permission. */
  can: (perm: string) => boolean;
  onChanged: () => void | Promise<void>;
  onLogout: () => void;
  /** Warns the profile editor: there are unsaved changes here (blocks export and import, which would alter things underneath). */
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const t = useT();
  const canWrite = can("trackers:write");
  const catalog = tracker.description_default;
  const custom = !!tracker.profile.description;
  const effective = useMemo(() => effectiveConfig(tracker.profile, catalog), [tracker.profile, catalog]);

  const [draft, setDraft] = useState<DescriptionDraft>(() => toDraft(effective, catalog));
  const [busy, setBusy] = useState<"" | "save" | "reset">("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  // The profile changed on the server (saved, restored, imported): the screen reverts to reflecting the saved state. The
  // key is the CONTENT, not the object's identity — reloading the list for another reason must not
  // discard what the operator is assembling.
  const sig = JSON.stringify(effective);
  useEffect(() => {
    setDraft(toDraft(effective, catalog));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sig]);

  const dirty = canWrite && !sameConfig(fromDraft(draft), effective);
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);

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

  async function save() {
    setError("");
    setNotice("");
    setBusy("save");
    try {
      await saveDescriptionConfig(tracker.id, fromDraft(draft));
      setNotice(t("trackers.trackerDescriptionEditor.noticeSaved"));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerDescriptionEditor.errorSave"));
    } finally {
      setBusy("");
    }
  }

  async function restore() {
    setError("");
    setNotice("");
    setBusy("reset");
    try {
      await resetDescriptionConfig(tracker.id);
      setNotice(t("trackers.trackerDescriptionEditor.noticeRestored"));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerDescriptionEditor.errorRestore"));
    } finally {
      setBusy("");
    }
  }

  return (
    <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-950/60 p-3">
      <p className="text-xs font-medium text-slate-300">
        {t("trackers.trackerDescriptionEditor.title")}{" "}
        <span
          className={`ml-1 rounded-full px-2 py-0.5 text-[11px] font-normal ring-1 ring-inset ${
            custom ? "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30" : "bg-slate-500/15 text-slate-300 ring-slate-500/30"
          }`}
        >
          {custom ? t("trackers.trackerDescriptionEditor.badgeCustom") : t("trackers.trackerDescriptionEditor.badgeDefault")}
        </span>
      </p>
      <p className="text-xs text-slate-400">
        {rich(t("trackers.trackerDescriptionEditor.helpText"))}
      </p>

      {canWrite ? (
        <>
          <BlockList
            kind="media"
            list={draft.media}
            onToggle={(i) => setDraft((d) => ({ ...d, media: toggleBlock(d.media, i) }))}
            onMove={(i, delta) => setDraft((d) => ({ ...d, media: moveBlock(d.media, i, delta) }))}
          />
          <label className="flex items-center gap-2 text-xs text-slate-400">
            {t("trackers.trackerDescriptionEditor.mediaInfoTagLabel")}
            <select
              value={draft.tag}
              onChange={(e) => setDraft((d) => ({ ...d, tag: e.target.value }))}
              aria-label={t("trackers.trackerDescriptionEditor.mediaInfoTagAria")}
              className="sg-field-sm sg-select"
            >
              {MEDIAINFO_TAGS.map((tag) => (
                <option key={tag} value={tag}>
                  [{tag}]
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2 text-xs text-slate-400">
            {t("trackers.trackerDescriptionEditor.languageLabel")}
            <select
              value={draft.lang}
              onChange={(e) => setDraft((d) => ({ ...d, lang: e.target.value }))}
              aria-label={t("trackers.trackerDescriptionEditor.languageLabel")}
              className="sg-field-sm sg-select"
            >
              {CONTENT_LANGUAGES.map((lang) => (
                <option key={lang} value={lang}>
                  {t(LANGUAGE_NAME_KEY[lang])}
                </option>
              ))}
            </select>
            <span className="text-slate-500">{t("trackers.trackerDescriptionEditor.languageHint")}</span>
          </label>
          <BlockList
            kind="game"
            list={draft.game}
            onToggle={(i) => setDraft((d) => ({ ...d, game: toggleBlock(d.game, i) }))}
            onMove={(i, delta) => setDraft((d) => ({ ...d, game: moveBlock(d.game, i, delta) }))}
          />
          <div className="flex flex-wrap items-center gap-3">
            <button onClick={save} disabled={busy !== "" || !dirty} className={btnCls}>
              {busy === "save" ? t("common.saving") : t("trackers.trackerDescriptionEditor.saveComposition")}
            </button>
            {custom && (
              <button onClick={restore} disabled={busy !== ""} className={btnCls} title={t("trackers.trackerDescriptionEditor.restoreTitle")}>
                {busy === "reset" ? t("trackers.trackerDescriptionEditor.restoring") : t("trackers.trackerDescriptionEditor.restoreDefault")}
              </button>
            )}
            {dirty && <span className="text-xs text-amber-400">{t("trackers.trackerDescriptionEditor.unsavedChanges")}</span>}
            {notice && <span className="text-xs text-emerald-400">{notice}</span>}
          </div>
        </>
      ) : (
        <ReadOnly cfg={effective} />
      )}
      {error && <p className="text-sm text-red-400">{error}</p>}
    </div>
  );
}

/** The composition as text, for those who can view but not edit. */
function ReadOnly({ cfg }: { cfg: DescriptionConfig }) {
  const t = useT();
  return (
    <div className="space-y-1 text-xs text-slate-300">
      {describeConfig(t, cfg).map((row) => (
        <p key={row.kind}>
          <span className="text-slate-400">{t(KIND_TITLE_KEY[row.kind])}:</span> {row.text}
        </p>
      ))}
      <p>
        <span className="text-slate-400">{t("trackers.trackerDescriptionEditor.readOnlyMediaInfoTag")}:</span> [{cfg.media_info_tag}]
      </p>
      <p>
        <span className="text-slate-400">{t("trackers.trackerDescriptionEditor.languageLabel")}:</span>{" "}
        {t(LANGUAGE_NAME_KEY[contentLanguage(cfg) as (typeof CONTENT_LANGUAGES)[number]] ?? LANGUAGE_NAME_KEY.en)}
      </p>
    </div>
  );
}

function BlockList({
  kind,
  list,
  onToggle,
  onMove,
}: {
  kind: DescriptionKind;
  list: { id: string; on: boolean }[];
  onToggle: (index: number) => void;
  onMove: (index: number, delta: -1 | 1) => void;
}) {
  const t = useT();
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-slate-300">{t(KIND_TITLE_KEY[kind])}</p>
      <ul className="space-y-0.5">
        {list.map((b, i) => (
          <li key={b.id} className="flex items-center gap-2 text-xs">
            <input
              type="checkbox"
              checked={b.on}
              onChange={() => onToggle(i)}
              aria-label={t("trackers.trackerDescriptionEditor.includeBlockAria", { label: blockLabel(t, b.id), kind: t(KIND_TITLE_KEY[kind]) })}
              className="sg-check"
            />
            <span className={`w-24 shrink-0 ${b.on ? "text-slate-200" : "text-slate-500 line-through"}`}>{blockLabel(t, b.id)}</span>
            <span className="min-w-0 flex-1 truncate text-slate-500" title={blockHint(t, b.id)}>
              {blockHint(t, b.id)}
            </span>
            <button
              onClick={() => onMove(i, -1)}
              disabled={i === 0}
              aria-label={t("trackers.trackerDescriptionEditor.moveUpAria", { label: blockLabel(t, b.id), kind: t(KIND_TITLE_KEY[kind]) })}
              className={stepCls}
            >
              ↑
            </button>
            <button
              onClick={() => onMove(i, 1)}
              disabled={i === list.length - 1}
              aria-label={t("trackers.trackerDescriptionEditor.moveDownAria", { label: blockLabel(t, b.id), kind: t(KIND_TITLE_KEY[kind]) })}
              className={stepCls}
            >
              ↓
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
