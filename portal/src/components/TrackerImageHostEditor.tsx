import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  clearImageHostKey,
  resetImageHostSpec,
  saveImageHostSpec,
  setImageHostKey,
  type ImageHostSpec,
  type Tracker,
} from "../api";
import { rich, useT } from "../i18n";

// Example values for technical fields (the format of a ShareX host): these are what you type,
// the same in any language — not interface text.
const EXAMPLE = {
  url: "https://img.tracker.example/api/v1/sharex/upload",
  fileField: "file",
  headerName: "Authorization",
  headerPrefix: "Bearer ",
  urlField: "url",
  thumbField: "thumbnail_url",
};

// Tracker image host (#193, ADR-040): the site's own host for hosting screenshots.
// The profile's `image_host` section is layer 2 — the operator's key is layer 3 and comes in via its
// own route, attached to the upload_url host. Button gate: those who can read the tracker SEE the section;
// only those who can write (trackers:write) see the edit controls.

const btnCls =
  "rounded-full border border-slate-700 bg-slate-900 px-3 py-1.5 text-xs font-medium text-slate-200 transition hover:border-sky-600 hover:text-sky-200 disabled:opacity-40";
// Form-control base classes (#22).
const inputCls = "sg-field w-full";

export function TrackerImageHostEditor({
  tracker,
  can,
  onChanged,
  onLogout,
  onDirtyChange,
}: {
  tracker: Tracker;
  can: (perm: string) => boolean;
  onChanged: () => void | Promise<void>;
  onLogout: () => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const t = useT();
  const canWrite = can("trackers:write");
  const current = tracker.profile.image_host;
  const has = !!current;

  const [draft, setDraft] = useState<ImageHostSpec>(() => current ?? emptySpec());
  const [keyDraft, setKeyDraft] = useState("");
  const [busy, setBusy] = useState<"" | "save" | "reset" | "key" | "clearKey">("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const sig = JSON.stringify(current);
  useEffect(() => {
    setDraft(current ?? emptySpec());
    setKeyDraft("");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sig]);

  const dirty = canWrite && (has || !sameSpec(draft, emptySpec())) && !sameSpec(draft, current ?? emptySpec());
  const hasKeyDraft = keyDraft.trim().length > 0;
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
      await saveImageHostSpec(tracker.id, draft);
      setNotice(t("trackers.trackerImageHostEditor.noticeSaved"));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerImageHostEditor.errorSave"));
    } finally {
      setBusy("");
    }
  }

  async function reset() {
    setError("");
    setNotice("");
    setBusy("reset");
    try {
      await resetImageHostSpec(tracker.id);
      setNotice(t("trackers.trackerImageHostEditor.noticeReset"));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerImageHostEditor.errorReset"));
    } finally {
      setBusy("");
    }
  }

  async function saveKey() {
    setError("");
    setNotice("");
    setBusy("key");
    try {
      await setImageHostKey(tracker.id, keyDraft);
      setKeyDraft("");
      setNotice(t("trackers.trackerImageHostEditor.noticeKeyRegistered"));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerImageHostEditor.errorRegisterKey"));
    } finally {
      setBusy("");
    }
  }

  async function clearKey() {
    setError("");
    setNotice("");
    setBusy("clearKey");
    try {
      await clearImageHostKey(tracker.id);
      setNotice(t("trackers.trackerImageHostEditor.noticeKeyRemoved"));
      await onChanged();
    } catch (err) {
      fail(err, t("trackers.trackerImageHostEditor.errorRemoveKey"));
    } finally {
      setBusy("");
    }
  }

  // DNS host of the SAVED upload_url — this is where the server sends the key, not to the draft.
  const savedHost = current ? hostOf(current.upload_url) : "";
  const keyMismatch = keyDiverges(tracker, savedHost);

  return (
    <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-950/60 p-3">
      <p className="text-xs font-medium text-slate-300">
        {t("trackers.trackerImageHostEditor.title")}{" "}
        <span
          className={`ml-1 rounded-full px-2 py-0.5 text-[11px] font-normal ring-1 ring-inset ${
            has ? "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30" : "bg-slate-500/15 text-slate-300 ring-slate-500/30"
          }`}
        >
          {has ? t("trackers.trackerImageHostEditor.statusConfigured") : t("trackers.trackerImageHostEditor.statusImgbbOnly")}
        </span>
      </p>
      <p className="text-xs text-slate-400">
        {t("trackers.trackerImageHostEditor.description")}
      </p>

      {canWrite ? (
        <>
          <div className="grid gap-2 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-xs text-slate-400">
              {t("trackers.trackerImageHostEditor.labelName")}
              <input
                value={draft.name}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                placeholder={t("trackers.trackerImageHostEditor.placeholderName")}
                aria-label={t("trackers.trackerImageHostEditor.ariaName")}
                className={inputCls}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-slate-400">
              {t("trackers.trackerImageHostEditor.labelUploadUrl")}
              <input
                value={draft.upload_url}
                onChange={(e) => setDraft({ ...draft, upload_url: e.target.value })}
                placeholder={EXAMPLE.url}
                aria-label={t("trackers.trackerImageHostEditor.ariaUploadUrl")}
                className={inputCls}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-slate-400">
              {t("trackers.trackerImageHostEditor.labelFileField")}
              <input
                value={draft.file_field}
                onChange={(e) => setDraft({ ...draft, file_field: e.target.value })}
                placeholder={EXAMPLE.fileField}
                aria-label={t("trackers.trackerImageHostEditor.ariaFileField")}
                className={inputCls}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-slate-400">
              {t("trackers.trackerImageHostEditor.labelAuthHeader")}
              <input
                value={draft.auth_header}
                onChange={(e) => setDraft({ ...draft, auth_header: e.target.value })}
                placeholder={EXAMPLE.headerName}
                aria-label={t("trackers.trackerImageHostEditor.ariaAuthHeader")}
                className={inputCls}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-slate-400">
              {t("trackers.trackerImageHostEditor.labelAuthPrefix")}
              <input
                value={draft.auth_prefix}
                onChange={(e) => setDraft({ ...draft, auth_prefix: e.target.value })}
                placeholder={EXAMPLE.headerPrefix}
                aria-label={t("trackers.trackerImageHostEditor.ariaAuthPrefix")}
                className={inputCls}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-slate-400">
              {t("trackers.trackerImageHostEditor.labelUrlPath")}
              <input
                value={draft.url_path}
                onChange={(e) => setDraft({ ...draft, url_path: e.target.value })}
                placeholder={EXAMPLE.urlField}
                aria-label={t("trackers.trackerImageHostEditor.ariaUrlPath")}
                className={inputCls}
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-slate-400">
              {t("trackers.trackerImageHostEditor.labelThumbnailPath")}
              <input
                value={draft.thumbnail_path ?? ""}
                onChange={(e) => setDraft({ ...draft, thumbnail_path: e.target.value })}
                placeholder={EXAMPLE.thumbField}
                aria-label={t("trackers.trackerImageHostEditor.ariaThumbnailPath")}
                className={inputCls}
              />
            </label>
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <button onClick={save} disabled={busy !== "" || !dirty} className={btnCls}>
              {busy === "save" ? t("common.saving") : t("trackers.trackerImageHostEditor.btnSaveHost")}
            </button>
            {has && (
              <button onClick={reset} disabled={busy !== ""} className={btnCls} title={t("trackers.trackerImageHostEditor.titleReset")}>
                {busy === "reset" ? t("trackers.trackerImageHostEditor.btnRemoving") : t("trackers.trackerImageHostEditor.btnRemoveHost")}
              </button>
            )}
            {dirty && <span className="text-xs text-amber-400">{t("trackers.trackerImageHostEditor.unsavedChanges")}</span>}
            {notice && <span className="text-xs text-emerald-400">{notice}</span>}
          </div>

          {/* The key section appears whenever there is something to manage: an image_host section OR an
              orphan key (registered for a host that the profile no longer describes). Button gate:
              those who read see the seal and the warning; only those who write see register/delete (#193, ADR-040). */}
          {(has || tracker.has_image_host_key) && (
            <div className="space-y-2 rounded-lg border border-slate-700 bg-slate-900/40 p-2">
              <p className="text-xs font-medium text-slate-300">{t("trackers.trackerImageHostEditor.keySectionTitle")}</p>
              <p className="text-xs text-slate-400">
                {rich(t("trackers.trackerImageHostEditor.keyDescription"), {
                  host: <span className="font-mono text-slate-200">{savedHost || t("trackers.trackerImageHostEditor.noHostConfigured")}</span>,
                })}
              </p>
              {tracker.has_image_host_key && (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11px] text-emerald-300 ring-1 ring-inset ring-emerald-500/30">
                    {t("trackers.trackerImageHostEditor.badgeKeyRegistered")}
                  </span>
                  {tracker.image_host_key_host && (
                    <span className="text-xs text-slate-500">
                      {rich(t("trackers.trackerImageHostEditor.keyForHost"), {
                        host: <span className="font-mono">{tracker.image_host_key_host}</span>,
                      })}
                    </span>
                  )}
                  {keyMismatch && (
                    <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-[11px] text-amber-300 ring-1 ring-inset ring-amber-500/30">
                      {t("trackers.trackerImageHostEditor.warningKeyMismatch")}
                    </span>
                  )}
                  <button onClick={clearKey} disabled={busy !== ""} className={btnCls}>
                    {busy === "clearKey" ? t("trackers.trackerImageHostEditor.btnRemoving") : t("trackers.trackerImageHostEditor.btnDeleteKey")}
                  </button>
                </div>
              )}
              {/* Registering a new key only makes sense with the image_host section: without it, the API
                  refuses (409 profile_without_image_host). The orphan key above can only be deleted. */}
              {has && (
                <div className="flex items-center gap-2">
                  <input
                    type="password"
                    autoComplete="new-password"
                    value={keyDraft}
                    onChange={(e) => setKeyDraft(e.target.value)}
                    placeholder={t("trackers.trackerImageHostEditor.placeholderNewKey")}
                    aria-label={t("trackers.trackerImageHostEditor.ariaImageHostKey")}
                    className={`${inputCls} max-w-xs`}
                  />
                  <button onClick={saveKey} disabled={busy !== "" || !hasKeyDraft} className={btnCls}>
                    {busy === "key" ? t("trackers.trackerImageHostEditor.btnRegistering") : t("trackers.trackerImageHostEditor.btnRegisterKey")}
                  </button>
                </div>
              )}
            </div>
          )}
        </>
      ) : (
        <ReadOnly spec={current} tracker={tracker} />
      )}
      {error && <p className="text-sm text-red-400">{error}</p>}
    </div>
  );
}

// The key only goes to the host recorded with it (strict binding, ADR-040): with a saved host and
// registered key, any difference — including a key without a recorded host — is a mismatch.
function keyDiverges(tracker: Tracker, savedHost: string): boolean {
  return tracker.has_image_host_key && savedHost !== "" && tracker.image_host_key_host !== savedHost;
}

/**
 * The section as text, for those who can view but not edit. The data remains visible (button
 * gate, §13.1): this includes the key seal and the mismatch warning — those who read also
 * need to know that the key will not be sent (#193, ADR-040).
 */
function ReadOnly({ spec, tracker }: { spec?: ImageHostSpec; tracker: Tracker }) {
  const t = useT();
  const keyMismatch = keyDiverges(tracker, spec ? hostOf(spec.upload_url) : "");
  return (
    <div className="space-y-1 text-xs text-slate-300">
      {spec ? (
        <>
          <p>
            <span className="text-slate-400">{t("trackers.trackerImageHostEditor.readonlyLabelName")}</span> {spec.name}
          </p>
          <p>
            <span className="text-slate-400">{t("trackers.trackerImageHostEditor.readonlyLabelUpload")}</span> <span className="font-mono">{spec.upload_url}</span>
          </p>
          <p>
            <span className="text-slate-400">{t("trackers.trackerImageHostEditor.readonlyLabelField")}</span> <span className="font-mono">{spec.file_field}</span>
            <span className="text-slate-400"> · {t("trackers.trackerImageHostEditor.readonlyLabelHeader")}</span> <span className="font-mono">{spec.auth_header}: {spec.auth_prefix}&lt;chave&gt;</span>
          </p>
          <p>
            <span className="text-slate-400">{t("trackers.trackerImageHostEditor.readonlyLabelUrl")}</span> <span className="font-mono">{spec.url_path}</span>
            {spec.thumbnail_path && (
              <>
                <span className="text-slate-400"> · {t("trackers.trackerImageHostEditor.readonlyLabelThumbnail")}</span> <span className="font-mono">{spec.thumbnail_path}</span>
              </>
            )}
          </p>
        </>
      ) : (
        <p className="text-slate-400">{t("trackers.trackerImageHostEditor.readonlyNoHost")}</p>
      )}
      {tracker.has_image_host_key ? (
        <p className="text-emerald-400">
          {rich(t("trackers.trackerImageHostEditor.readonlyKeyRegisteredFor"), {
            host: <span className="font-mono">{tracker.image_host_key_host}</span>,
          })}
        </p>
      ) : (
        spec && <p className="text-slate-500">{t("trackers.trackerImageHostEditor.readonlyNoKey")}</p>
      )}
      {keyMismatch && (
        <p className="text-amber-400">{t("trackers.trackerImageHostEditor.warningKeyMismatch")}</p>
      )}
    </div>
  );
}

function emptySpec(): ImageHostSpec {
  return { name: "", upload_url: "", file_field: "", auth_header: "", auth_prefix: "", url_path: "", thumbnail_path: "" };
}

function sameSpec(a: ImageHostSpec, b: ImageHostSpec): boolean {
  return (
    a.name === b.name &&
    a.upload_url === b.upload_url &&
    a.file_field === b.file_field &&
    a.auth_header === b.auth_header &&
    a.auth_prefix === b.auth_prefix &&
    a.url_path === b.url_path &&
    (a.thumbnail_path ?? "") === (b.thumbnail_path ?? "")
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return "";
  }
}
