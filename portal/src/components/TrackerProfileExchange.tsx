import { useCallback, useRef, useState, type ChangeEvent } from "react";
import {
  ApiError,
  downloadTrackerProfile,
  importTrackerProfile,
  previewProfileImport,
  type ImportDescription,
  type ImportImageHost,
  type ImportPreview,
  type ImportResult,
  type Tracker,
} from "../api";
import { AXES, KIND_LABEL_KEY } from "../lib/taxonomy";
import { KIND_TITLE_KEY, describeBlocks } from "../lib/description";
import {
  DESCRIPTION_KEY,
  IMAGE_HOST_KEY,
  checkImportFile,
  describeChanges,
  describeResult,
  effectiveCounts,
  hasChanges,
  overwriteKey,
  protectedText,
  siteLabel,
} from "../lib/profile-exchange";
import { useT, rich } from "../i18n";

// Share the site profile (ADR-037, #197): export what this tracker knows about the SITE and
// import what someone else exported — without needing an account on their site.
//
// Three things the screen must keep honest:
//  1. The file NEVER carries token, passkey, or announce: it is the site profile (ids and names) and
//     its identification. The screen states this, and states what the file carries (the name the operator gave
//     to the tracker and the host), to check before sharing.
//  2. Importing is THIRD-PARTY content. The preview comes first, saves nothing, and repeats that
//     the ids and names were not checked against this operator's site.
//  3. What the operator adjusted by hand — and what their site confirmed on discovery — is not
//     overwritten without them asking, and asking is term by term.

const btnCls =
  "rounded-lg border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-200 transition hover:border-sky-600 hover:text-sky-200 disabled:opacity-40";

export function TrackerProfileExchange({
  tracker,
  can,
  blocked,
  onChanged,
  onLogout,
}: {
  tracker: Tracker;
  /** Gate by button (docs/11 - Portal.md §13.1): hides the button, never the data. */
  can: (perm: string) => boolean;
  /** There are unsaved changes in the editor: the file would carry the SAVED profile, and the import would replace it underneath. */
  blocked: boolean;
  onChanged: () => void | Promise<void>;
  onLogout: () => void;
}) {
  const t = useT();
  const canExport = can("trackers:read");
  const canImport = can("trackers:write");

  const fileInput = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState<"" | "export" | "preview" | "apply">("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [file, setFile] = useState<{ name: string; text: string } | null>(null);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [overwrite, setOverwrite] = useState<Set<string>>(new Set());

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

  const reset = () => {
    setFile(null);
    setPreview(null);
    setOverwrite(new Set());
  };

  async function doExport() {
    setError("");
    setNotice("");
    setBusy("export");
    try {
      const name = await downloadTrackerProfile(tracker.id);
      setNotice(t("trackers.trackerProfileExchange.exportSuccess", { name }));
    } catch (err) {
      fail(err, t("trackers.trackerProfileExchange.exportFail"));
    } finally {
      setBusy("");
    }
  }

  async function onPick(e: ChangeEvent<HTMLInputElement>) {
    const picked = e.target.files?.[0];
    e.target.value = ""; // choosing the same file again must trigger onChange
    if (!picked) return;
    setError("");
    setNotice("");
    reset();
    const problem = checkImportFile(t, picked);
    if (problem) {
      setError(problem);
      return;
    }
    setBusy("preview");
    try {
      const text = await picked.text();
      const res = await previewProfileImport(tracker.id, text);
      setFile({ name: picked.name, text });
      setPreview(res);
    } catch (err) {
      fail(err, t("trackers.trackerProfileExchange.readFail"));
    } finally {
      setBusy("");
    }
  }

  function toggle(key: string) {
    setOverwrite((s) => {
      const n = new Set(s);
      if (n.has(key)) n.delete(key);
      else n.add(key);
      return n;
    });
  }

  async function apply() {
    if (!file || !preview) return;
    setError("");
    setBusy("apply");
    try {
      const res: ImportResult = await importTrackerProfile(tracker.id, file.text, [...overwrite]);
      reset();
      setNotice(describeResult(t, res.plan));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerProfileExchange.importFail"));
    } finally {
      setBusy("");
    }
  }

  if (!canExport && !canImport) return null;

  const why = blocked ? t("trackers.trackerProfileExchange.blockedHint") : undefined;
  return (
    <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-950/60 p-3">
      <p className="text-xs font-medium text-slate-300">{t("trackers.trackerProfileExchange.title")}</p>
      <p className="text-xs text-slate-400">
        {rich(t("trackers.trackerProfileExchange.description"))}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {canExport && (
          <button onClick={doExport} disabled={busy !== "" || blocked} title={why} className={btnCls}>
            {busy === "export" ? t("trackers.trackerProfileExchange.exporting") : t("trackers.trackerProfileExchange.export")}
          </button>
        )}
        {canImport && (
          <>
            <button
              onClick={() => fileInput.current?.click()}
              disabled={busy !== "" || blocked}
              title={why}
              className={btnCls}
            >
              {busy === "preview" ? t("trackers.trackerProfileExchange.reading") : t("trackers.trackerProfileExchange.import")}
            </button>
            <input
              ref={fileInput}
              type="file"
              accept=".json,application/json"
              onChange={onPick}
              aria-label={t("trackers.trackerProfileExchange.fileAria")}
              className="sg-field hidden"
            />
          </>
        )}
      </div>
      {notice && <p className="text-xs text-emerald-400">{notice}</p>}
      {error && <p className="text-sm text-red-400">{error}</p>}
      {preview && file && (
        <ImportPreviewView
          preview={preview}
          fileName={file.name}
          overwrite={overwrite}
          applying={busy === "apply"}
          onToggle={toggle}
          onApply={apply}
          onCancel={reset}
        />
      )}
    </div>
  );
}

/**
 * The import preview: what the file is, what would change, and what the operator can
 * decide. Pure in props — the state (file, flags) belongs to the parent component.
 */
export function ImportPreviewView({
  preview,
  fileName,
  overwrite,
  applying,
  onToggle,
  onApply,
  onCancel,
}: {
  preview: ImportPreview;
  fileName: string;
  overwrite: Set<string>;
  applying: boolean;
  onToggle: (key: string) => void;
  onApply: () => void;
  onCancel: () => void;
}) {
  const t = useT();
  const { plan } = preview;
  const eff = effectiveCounts(plan, overwrite);
  const changes = hasChanges(plan, overwrite);
  return (
    <div className="space-y-3 rounded-xl border border-sky-800/50 bg-sky-500/5 p-3">
      <div>
        <p className="text-sm font-medium text-white">{t("trackers.trackerProfileExchange.previewTitle")}</p>
        <p className="mt-0.5 text-xs text-slate-400">
          {rich(t("trackers.trackerProfileExchange.previewMeta"), {
            fileName: <span className="font-mono">{fileName}</span>,
            site: <strong>{siteLabel(plan.site)}</strong>,
            family: plan.family,
            version: plan.schema_version,
          })}
        </p>
      </div>

      <p className="rounded-lg border border-amber-700/40 bg-amber-500/5 px-3 py-2 text-xs text-amber-300">
        {/* The warning is constant: the Portal has its own, in the reader's language; the API's preview.notice is for those consuming it without the Portal. */}
        ⚠ {t("trackers.trackerProfileExchange.importNotice")}
      </p>
      {plan.warnings.map((w) => (
        <p key={w} className="text-xs text-amber-400">
          ⚠ {w}
        </p>
      ))}

      <p className="text-xs text-slate-300">
        {rich(t("trackers.trackerProfileExchange.applySummaryPrefix"), {
          strongChanges: <strong>{describeChanges(t, eff)}</strong>,
        })}
        {eff.same > 0 && ` ${t("trackers.trackerProfileExchange.applySummarySame", { count: eff.same })}`}
        {eff.protected > 0 && ` ${t("trackers.trackerProfileExchange.applySummaryProtected", { count: eff.protected })}`}
        {plan.counts.local_only > 0 && ` ${t("trackers.trackerProfileExchange.applySummaryLocalOnly", { count: plan.counts.local_only })}`}
      </p>

      {AXES.map(({ axis, labelKey }) => {
        const rows = plan.items[axis] ?? [];
        if (rows.length === 0) return null;
        return (
          <div key={axis} className="space-y-1">
            <p className="text-xs font-medium text-slate-300">{t(labelKey)}</p>
            {rows.map((it) => {
              const key = overwriteKey(axis, it.id);
              return (
                <div key={key} className="space-y-0.5">
                  <div className="flex items-center gap-2 text-xs">
                    <span className="w-12 shrink-0 font-mono text-slate-400">{it.id}</span>
                    <span className="min-w-0 flex-1 truncate text-slate-200" title={it.name}>
                      {it.status === "renamed" ? (
                        <>
                          <span className="text-slate-500 line-through">{it.current_name}</span> → {it.name}
                        </>
                      ) : (
                        it.name
                      )}
                      {it.kind && <span className="ml-2 text-slate-500">({t(KIND_LABEL_KEY[it.kind])})</span>}
                    </span>
                    <span className="shrink-0 text-right">
                      {it.status === "new" && <span className="text-sky-300">{t("trackers.trackerProfileExchange.statusNew")}</span>}
                      {it.status === "same" && (
                        <span className="text-slate-500">
                          {it.sets_kind ? t("trackers.trackerProfileExchange.statusSameSetsKind") : t("trackers.trackerProfileExchange.statusSame")}
                        </span>
                      )}
                      {it.status === "renamed" && (
                        <span className={it.overwrites_protected ? "text-emerald-400" : "text-amber-300"}>
                          {it.overwrites_protected ? t("trackers.trackerProfileExchange.statusRenamedOverwrites") : t("trackers.trackerProfileExchange.statusRenamed")}
                          {it.sets_kind ? ` · ${t("trackers.trackerProfileExchange.setsKindSuffix")}` : ""}
                        </span>
                      )}
                      {it.status === "protected" && (
                        <label className="flex items-center gap-1.5 text-emerald-400">
                          <span>{protectedText(t, it)}</span>
                          <input
                            type="checkbox"
                            checked={overwrite.has(key)}
                            onChange={() => onToggle(key)}
                            aria-label={t("trackers.trackerProfileExchange.useFileItemAria", { label: t(labelKey), id: it.id })}
                            className="sg-check"
                          />
                          <span className="text-slate-400">{t("trackers.trackerProfileExchange.useFromFile")}</span>
                        </label>
                      )}
                    </span>
                  </div>
                  {(it.warnings ?? []).map((w) => (
                    <p key={w} className="pl-14 text-xs text-amber-400">
                      ⚠ {w}
                    </p>
                  ))}
                </div>
              );
            })}
          </div>
        );
      })}

      {plan.description && (
        <DescriptionImportRow
          item={plan.description}
          asked={overwrite.has(DESCRIPTION_KEY)}
          onToggle={() => onToggle(DESCRIPTION_KEY)}
        />
      )}

      {plan.image_host && (
        <ImageHostImportRow
          item={plan.image_host}
          asked={overwrite.has(IMAGE_HOST_KEY)}
          onToggle={() => onToggle(IMAGE_HOST_KEY)}
        />
      )}

      <div className="flex flex-wrap items-center gap-3">
        <button
          onClick={onApply}
          disabled={applying || !changes}
          className="rounded-full bg-emerald-600 px-4 py-1.5 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-40"
        >
          {applying ? t("trackers.trackerProfileExchange.applying") : t("trackers.trackerProfileExchange.apply")}
        </button>
        <button onClick={onCancel} disabled={applying} className="text-xs text-slate-400 hover:text-white">
          {t("common.cancel")}
        </button>
        {!changes && <span className="text-xs text-slate-500">{t("trackers.trackerProfileExchange.noChanges")}</span>}
      </div>
    </div>
  );
}

/**
 * The description composition section (#192) in the preview: what the file brings, what
 * happens to the profile's, and — when the operator's would be kept — the choice to replace it. It's ONE
 * unit (there are no terms): the rule is the manual's, applied to the entire section.
 */
function DescriptionImportRow({ item, asked, onToggle }: { item: ImportDescription; asked: boolean; onToggle: () => void }) {
  const t = useT();
  const file = item.config;
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-slate-300">{t("trackers.trackerProfileExchange.descCompositionTitle")}</p>
      <div className="space-y-0.5 text-xs text-slate-300">
        <p>
          <span className="text-slate-400">{t(KIND_TITLE_KEY.media)}:</span> {describeBlocks(t, file.media_blocks)}
        </p>
        <p>
          <span className="text-slate-400">{t(KIND_TITLE_KEY.game)}:</span> {describeBlocks(t, file.game_blocks)}
        </p>
        <p>
          <span className="text-slate-400">{t("trackers.trackerProfileExchange.descMediaInfoTag")}:</span> [{file.media_info_tag}]
        </p>
      </div>
      <div className="text-xs">
        {item.status === "new" && <span className="text-sky-300">{t("trackers.trackerProfileExchange.descStatusNew")}</span>}
        {item.status === "same" && <span className="text-slate-500">{t("trackers.trackerProfileExchange.descStatusSame")}</span>}
        {item.status === "replaced" && <span className="text-emerald-400">{t("trackers.trackerProfileExchange.descStatusReplaced")}</span>}
        {item.status === "protected" && (
          <label className="flex items-center gap-1.5 text-emerald-400">
            <span>{t("trackers.trackerProfileExchange.descStatusProtected")}</span>
            <input
              type="checkbox"
              checked={asked}
              onChange={onToggle}
              aria-label={t("trackers.trackerProfileExchange.descUseFileAria")}
              className="sg-check"
            />
            <span className="text-slate-400">{t("trackers.trackerProfileExchange.useFromFile")}</span>
          </label>
        )}
      </div>
      {item.current && (
        <p className="text-xs text-slate-500">
          {t("trackers.trackerProfileExchange.descCurrent", {
            media: describeBlocks(t, item.current.media_blocks),
            games: describeBlocks(t, item.current.game_blocks),
            tag: item.current.media_info_tag,
          })}
        </p>
      )}
    </div>
  );
}

/**
 * The image host section (#193) in the preview: what the file brings, what happens
 * to the profile's, and — when the operator's would be kept — the choice to replace it. It's ONE unit
 * (there are no terms): the rule is the manual's, applied to the entire section.
 */
function ImageHostImportRow({ item, asked, onToggle }: { item: ImportImageHost; asked: boolean; onToggle: () => void }) {
  const t = useT();
  const file = item.config;
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-slate-300">{t("trackers.trackerProfileExchange.imageHostTitle")}</p>
      <div className="space-y-0.5 text-xs text-slate-300">
        <p>
          <span className="text-slate-400">{t("trackers.trackerProfileExchange.imageHostName")}:</span> {file.name}
        </p>
        <p>
          <span className="text-slate-400">{t("trackers.trackerProfileExchange.imageHostUpload")}:</span> <span className="font-mono">{file.upload_url}</span>
        </p>
        <p>
          <span className="text-slate-400">{t("trackers.trackerProfileExchange.imageHostField")}:</span> <span className="font-mono">{file.file_field}</span>
          <span className="text-slate-400"> · {t("trackers.trackerProfileExchange.imageHostHeader")}:</span> <span className="font-mono">{file.auth_header}: {file.auth_prefix}&lt;chave&gt;</span>
        </p>
        <p>
          <span className="text-slate-400">{t("trackers.trackerProfileExchange.imageHostUrl")}:</span> <span className="font-mono">{file.url_path}</span>
          {file.thumbnail_path && (
            <>
              <span className="text-slate-400"> · {t("trackers.trackerProfileExchange.imageHostThumbnail")}:</span> <span className="font-mono">{file.thumbnail_path}</span>
            </>
          )}
        </p>
      </div>
      <div className="text-xs">
        {item.status === "new" && <span className="text-sky-300">{t("trackers.trackerProfileExchange.imageHostStatusNew")}</span>}
        {item.status === "same" && <span className="text-slate-500">{t("trackers.trackerProfileExchange.imageHostStatusSame")}</span>}
        {item.status === "replaced" && <span className="text-emerald-400">{t("trackers.trackerProfileExchange.imageHostStatusReplaced")}</span>}
        {item.status === "protected" && (
          <label className="flex items-center gap-1.5 text-emerald-400">
            <span>{t("trackers.trackerProfileExchange.imageHostStatusProtected")}</span>
            <input
              type="checkbox"
              checked={asked}
              onChange={onToggle}
              aria-label={t("trackers.trackerProfileExchange.imageHostUseFileAria")}
              className="sg-check"
            />
            <span className="text-slate-400">{t("trackers.trackerProfileExchange.useFromFile")}</span>
          </label>
        )}
      </div>
      {item.current && (
        <p className="text-xs text-slate-500">
          {t("trackers.trackerProfileExchange.imageHostCurrent", {
            name: item.current.name,
            url: item.current.upload_url,
          })}
        </p>
      )}
      {/* The operator's key is layer 3: it never leaves this server in the file (#193, ADR-040). */}
      <p className="text-xs text-slate-500">{t("trackers.trackerProfileExchange.imageHostKeyNote")}</p>
    </div>
  );
}
