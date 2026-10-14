// UNIT3D tracker upload review panel (ADR-020) and the step bar
// of the full form (F1). Extracted from pages/AddTorrent.tsx (#118) without
// behavior changes — the publish screen just mounts it.

import { useState, useEffect, useRef } from "react";
import {
  publishToTracker,
  generateScreenshots,
  extractScreenshots,
  renderScreenshots,
  getCommand,
  searchGames,
  inferPublish,
  composeMediaDescription,
  composeGameDescription,
  trackerDupes,
  previewPublish,
  ApiError,
  type Tracker,
  type InferResult,
  type GameCandidate,
  type DupeCandidate,
  type PublishPreview,
  type ScreenshotFailure,
} from "../../api";
import { audioLanguagesFromMediaInfo, addDualToName } from "./bbcode";
import { TaxonomySelects } from "./TaxonomySelects";
import { canPublish, defaultCategory, kindOfCategory, prefillFromParsed } from "../../lib/taxonomy";
import { rich, useT, type Key } from "../../i18n";

// Category/type/resolution options NO LONGER live here: they come from the tracker
// profile (`tracker.profile`, ADR-037/#196) — IDs are from the site, not the product, and vary from one
// UNIT3D site to another.

// TrackerUploadPanel is the review step (ADR-020): after uploading to a UNIT3D
// tracker, it pre-fills fields (name parse + TMDB) and the mediainfo extracted on the
// Node; the operator reviews/adjusts and triggers registration on the site (explicit gate).
export function TrackerUploadPanel({
  torrentId,
  name,
  mediaInfo,
  tracker,
  onLogout,
  onPublished,
  onClose,
}: {
  torrentId: string;
  name: string;
  mediaInfo: string;
  tracker: Tracker;
  onLogout: () => void;
  onPublished?: () => void;
  onClose: () => void;
}) {
  const t = useT();
  const [form, setForm] = useState({
    name,
    description: "",
    // No type/resolution default: a default no one reviewed is like publishing
    // "Encode 1080p" by mistake. Pre-filling (name → profile label) chooses what
    // can be decided; the operator chooses the rest.
    category_id: defaultCategory(tracker.profile),
    type_id: 0,
    resolution_id: 0,
    tmdb: 0,
    imdb: "",
    tvdb: 0,
    mal: 0,
    igdb: 0,
    season_number: 0,
    episode_number: 0,
    keywords: "",
    mediainfo: mediaInfo,
    anonymous: false,
  });
  // IGDB game search (#17): only appears when the category is Games.
  const [gameQ, setGameQ] = useState("");
  const [games, setGames] = useState<GameCandidate[] | null>(null);
  const [gameBusy, setGameBusy] = useState(false);
  const [gameErr, setGameErr] = useState("");
  const [infer, setInfer] = useState<InferResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<{ download_url: string; message: string } | null>(null);
  // Tracker duplicate evidence (#186). `dupeAck` is the "publish anyway":
  // the search informs, the operator decides — same pattern as cluster allow_duplicate (#11).
  const [dupes, setDupes] = useState<{ query: string; candidates: DupeCandidate[]; note?: string } | null>(null);
  const [dupeAck, setDupeAck] = useState(false);
  // Frozen preview (#188): null = not yet reviewed; the button changes action.
  const [preview, setPreview] = useState<PublishPreview | null>(null);
  // Screenshots (ADR-026): extraction via Agent (ffmpeg) + upload to the host chain (ADR-040),
  // the returned BBCode is inserted at the beginning of the description — the operator reviews/edits.
  // The tracker_id is sent (#193) so the tracker host joins the upload chain.
  const [genLoading, setGenLoading] = useState(false);
  const [genError, setGenError] = useState<string | null>(null);
  // Screenshot upload result (#193): where the images ended up and the failures.
  const [shotResult, setShotResult] = useState<{ host: string; failures: ScreenshotFailure[] } | null>(null);
  // Full form step bar (F1): each step with its status. null = hidden.
  const [formSteps, setFormSteps] = useState<FormStep[] | null>(null);

  // Any form edit discards the preview: what the operator reviewed is no longer
  // what would be sent. The backend also rejects (409 preview_stale) — this is just
  // so the screen doesn't offer a button that will fail.
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) => {
    setPreview(null);
    setForm((f) => ({ ...f, [k]: v }));
  };

  // payload builds the form only once — preview and submit must send the SAME
  // body, otherwise the fingerprint won't match and the operator gets a 409 without touching anything.
  const payload = () => ({
    tracker_id: tracker.id,
    name: form.name.trim(),
    description: form.description.trim(),
    category_id: form.category_id,
    type_id: form.type_id,
    resolution_id: form.resolution_id,
    tmdb: form.tmdb,
    imdb: form.imdb,
    tvdb: form.tvdb,
    mal: form.mal,
    igdb: form.igdb,
    season_number: form.season_number,
    episode_number: form.episode_number,
    keywords: form.keywords,
    mediainfo: form.mediainfo,
    anonymous: form.anonymous,
  });

  // category and type are required in UNIT3D; the select's 0 is "not yet chosen".
  function taxonomyError(): string {
    if (!canPublish(tracker.profile)) {
      return t("publish.trackerUploadPanel.errors.noTaxonomy", { name: tracker.name });
    }
    if (!form.category_id || !form.type_id) return t("publish.trackerUploadPanel.errors.missingTaxonomy");
    return "";
  }

  async function openPreview() {
    setError("");
    const te = taxonomyError();
    if (te) {
      setError(te);
      return;
    }
    setBusy(true);
    try {
      setPreview(await previewPublish(torrentId, payload()));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(err instanceof ApiError ? err.message : t("publish.trackerUploadPanel.errors.previewFailed"));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    (async () => {
      try {
        // Passes the mediainfo → the standardized name comes enriched (resolution/codec/audio).
        const inf = await inferPublish(name, undefined, mediaInfo, tracker.id);
        setInfer(inf);
        // The parser returns labels; here what matters are the SITE IDs. The bridge is
        // the label ("1080p", "WEB-DL") and the category role (series/movie) — see prefillFromParsed.
        const pre = prefillFromParsed(tracker.profile, inf.parsed);
        setForm((f) => ({
          ...f,
          // Name in the naming convention — editable; falls back to title if empty.
          name: inf.parsed.standard_name || f.name || inf.parsed.title,
          category_id: pre.category_id || f.category_id,
          type_id: pre.type_id || f.type_id,
          resolution_id: pre.resolution_id || f.resolution_id,
          season_number: inf.parsed.season,
          episode_number: inf.parsed.episode,
          tmdb: inf.tmdb?.tmdb_id ?? 0,
          // IMDB: prefers the one from TMDB; falls back to the embedded one in the name (Radarr/Plex) if TMDB didn't bring it.
          imdb: inf.tmdb?.imdb || inf.parsed.imdb || "",
          tvdb: inf.tmdb?.tvdb ?? 0,
          // Initial description: TMDB synopsis (the operator enriches/BBCode later).
          description: inf.tmdb?.overview ?? "",
        }));
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) onLogout();
      } finally {
        setLoading(false);
      }
    })();
    // runs once per upload
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Searches for duplicates in the tracker catalog (#186), in parallel with infer. Failure here is
  // silent on purpose: evidence is assistance and cannot become a screen error nor
  // block the upload — without a response, the panel proceeds as if there were no candidate.
  useEffect(() => {
    (async () => {
      try {
        setDupes(await trackerDupes(tracker.id, name));
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) onLogout();
      }
    })();
    // runs once per upload
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function publish() {
    setError("");
    if (!form.name.trim() || !form.description.trim()) {
      setError(t("publish.trackerUploadPanel.errors.requiredFields"));
      return;
    }
    const te = taxonomyError();
    if (te) {
      setError(te);
      return;
    }
    setBusy(true);
    try {
      // preview_token: the backend rejects if the form changed since the review (#188).
      const res = await publishToTracker(torrentId, { ...payload(), preview_token: preview?.token });
      setResult(res);
      onPublished?.(); // left the pending queue (backlog #8)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setError(err instanceof ApiError ? err.message : t("publish.trackerUploadPanel.errors.publishFailed"));
    } finally {
      setBusy(false);
    }
  }

  // handleGenerateScreenshots (ADR-026): asks the CP to extract screenshots from the torrent
  // already in the Engine and inserts the returned BBCode at the start of the description, without overwriting
  // what's already there (e.g., TMDB synopsis) — the operator still reviews/edits before publishing.
  // Sends the tracker_id (#193) so the tracker host joins the upload chain.
  async function handleGenerateScreenshots() {
    setGenError(null);
    setShotResult(null);
    setGenLoading(true);
    try {
      const res = await generateScreenshots(torrentId, tracker.id);
      set("description", res.bbcode + (form.description.trim() ? "\n\n" + form.description : ""));
      if (res.host) setShotResult({ host: res.host, failures: res.failures ?? [] });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setGenError(err instanceof Error ? err.message : t("publish.trackerUploadPanel.errors.screenshotsFailed"));
    } finally {
      setGenLoading(false);
    }
  }

  // handleGenerateCard (F1): builds the FULL FORM in one click, showing a STEP
  // BAR with the real progress of each stage: extract frames on the Node (command with its
  // own cycle, tracked by getCommand), host on the host chain (ADR-040), and build the
  // BBCode. Screenshots are best-effort: if a stage fails (incomplete torrent, missing
  // ffmpeg, no imgbb key), the form is still built without them — screenshots don't block
  // publishing. Sends the tracker_id (#193) so the tracker host joins the upload chain.
  async function handleGenerateCard() {
    setGenError(null);
    setShotResult(null);
    setGenLoading(true);
    const steps: FormStep[] = [
      { label: t("publish.trackerUploadPanel.sheetSteps.extract"), state: "active" },
      { label: t("publish.trackerUploadPanel.sheetSteps.host"), state: "pending" },
      { label: t("publish.trackerUploadPanel.sheetSteps.compose"), state: "pending" },
    ];
    const sync = () => setFormSteps([...steps]);
    const upd = (i: number, patch: Partial<FormStep>) => {
      steps[i] = { ...steps[i], ...patch };
      sync();
    };
    sync();

    let screenshots = "";
    let host = "";
    let failures: ScreenshotFailure[] = [];
    try {
      // Step 1: extracts on the Node — tracks the command cycle (Queued → Running).
      const { command_id } = await extractScreenshots(torrentId);
      for (;;) {
        const st = await getCommand(command_id);
        if (st.status === "DONE") break;
        if (st.status === "FAILED") throw new Error(st.error || t("publish.trackerUploadPanel.errors.extractionFailed"));
        upd(0, { detail: CMD_STAGE_KEY[st.status] ? t(CMD_STAGE_KEY[st.status]) : st.status });
        await new Promise((r) => setTimeout(r, 800));
      }
      upd(0, { state: "done", detail: undefined });
      // Step 2: hosts on the host chain + builds the BBCode block (server-side).
      upd(1, { state: "active" });
      const res = await renderScreenshots(command_id, tracker.id);
      screenshots = res.bbcode;
      host = res.host ?? "";
      failures = res.failures ?? [];
      upd(1, { state: "done" });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      const msg = err instanceof Error ? err.message : t("publish.trackerUploadPanel.errors.screenshotsFailed");
      const active = steps.findIndex((s) => s.state === "active");
      if (active >= 0) upd(active, { state: "error", detail: msg });
      [0, 1].forEach((j) => steps[j].state === "pending" && upd(j, { state: "skipped" }));
      setGenError(t("publish.trackerUploadPanel.errors.sheetWithoutScreenshots", { error: msg }));
    }

    // Step 3: builds the form (always — screenshots are optional). The Control Plane composes it,
    // based on the PROFILE composition of this tracker (#192); the Portal just delivers the text to the field. If the
    // call fails, the error appears and the description field remains EXACTLY as it was.
    upd(2, { state: "active" });
    try {
      const text = await composeMediaDescription(tracker.id, {
        poster: infer?.tmdb?.poster_url ?? "",
        overview: infer?.tmdb?.overview || form.description,
        keywords: form.keywords,
        screenshots,
        mediainfo: form.mediainfo,
      });
      set("description", text);
      upd(2, { state: "done" });
      if (host) setShotResult({ host, failures });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      const msg = err instanceof Error ? err.message : t("publish.trackerUploadPanel.errors.composeFailed");
      upd(2, { state: "error", detail: msg });
      setGenError(t("publish.trackerUploadPanel.errors.composeFailedFull", { msg }));
    }
    setGenLoading(false);
    setTimeout(() => setFormSteps(null), 5000); // disappears on its own after the operator sees it
  }

  const inputCls =
    "w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500";
  const num = (v: string) => (v === "" ? 0 : Number(v) || 0);
  // The category role comes from the profile (kind), not the number: the "Games" category ID is 5
  // on one site and another on another. Category without role = no special fields.
  const kind = kindOfCategory(tracker.profile, form.category_id);
  const isTV = kind === "tv";
  const isGame = kind === "game"; // Games (#17)
  // DUAL from the mediainfo (≥2 audio tracks in different languages) — avoids having to
  // open the file. Suggests marking DUAL in the name when not yet marked.
  const audioLangs = audioLanguagesFromMediaInfo(form.mediainfo);
  const suggestDual = !isGame && audioLangs.length >= 2 && !/\bdual\b/i.test(form.name);

  // cleanGameTerm reduces a ROM release name to a searchable title on IGDB:
  // removes extension, bracketed tags ([Xbox][NTSCU][ByAlvRo]…), separators and leftovers.
  function cleanGameTerm(raw: string): string {
    return raw
      .replace(/\.[a-z0-9]{2,4}$/i, "") // extension (.zip/.iso…)
      .replace(/\[[^\]]*\]/g, " ") // bracketed tags
      .replace(/\([^)]*\)/g, " ") // parentheses (region etc.)
      .replace(/[._]/g, " ")
      .replace(/\s*[-–]\s*$/g, "") // trailing loose dash
      .replace(/\s+/g, " ")
      .trim();
  }

  // IGDB search (#17): triggers on magnifying glass/Enter (not on every key). Suggests the term by name.
  async function runGameSearch() {
    const term = cleanGameTerm(gameQ.trim() || form.name.trim());
    if (!term) return;
    setGameBusy(true);
    setGameErr("");
    try {
      setGames(await searchGames(term));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setGameErr(
        err instanceof ApiError && err.code === "igdb_disabled"
          ? t("publish.trackerUploadPanel.errors.igdbDisabled")
          : err instanceof ApiError
            ? err.message
            : t("publish.trackerUploadPanel.errors.igdbSearchFailed"),
      );
    } finally {
      setGameBusy(false);
    }
  }

  // pickGame fills the IGDB ID on the spot and asks the Control Plane for the description (cover + title + about
  // + genres/platforms), composed by this tracker's profile configuration (#192). If the call
  // fails, the error appears next to the search and the description field is NOT touched. Two choices in
  // sequence: only the last one counts (a delayed response doesn't overwrite the one for the game chosen later).
  const pickSeq = useRef(0);
  async function pickGame(g: GameCandidate) {
    const mine = ++pickSeq.current;
    setPreview(null); // changes igdb+description: the reviewed preview is no longer valid (#188)
    setForm((f) => ({ ...f, igdb: g.igdb_id }));
    setGames(null);
    setGameErr("");
    try {
      const text = await composeGameDescription(tracker.id, g);
      if (mine !== pickSeq.current) return;
      setPreview(null);
      setForm((f) => ({ ...f, description: text }));
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      if (mine !== pickSeq.current) return;
      setGameErr(
        t("publish.trackerUploadPanel.errors.gameDescFailed", {
          msg: err instanceof Error ? err.message : t("publish.trackerUploadPanel.errors.unknownFailure"),
        }),
      );
    }
  }

  // When changing the category to Games, it automatically searches for the game by the torrent name
  // (avoids the operator having to notice/click) — only on the 1st time, if one hasn't been chosen yet.
  useEffect(() => {
    if (isGame && games === null && !gameBusy && form.igdb === 0) {
      runGameSearch();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isGame]);

  if (result) {
    return (
      <div className="mt-4 max-w-2xl space-y-3 rounded-2xl border border-emerald-700/50 bg-emerald-500/5 p-6">
        <h2 className="text-sm font-semibold text-emerald-300">
          ✓ {t("publish.trackerUploadPanel.result.registered", { tracker: tracker.source || tracker.name })}
        </h2>
        <p className="text-sm text-slate-300">{result.message || t("publish.trackerUploadPanel.result.success")}</p>
        {result.download_url && (
          <p className="break-all font-mono text-xs text-slate-400">
            {rich(t("publish.trackerUploadPanel.result.torrentLink"), {
              link: (
                <a href={result.download_url} target="_blank" rel="noreferrer" className="text-emerald-400 hover:underline">
                  {result.download_url}
                </a>
              ),
            })}
          </p>
        )}
        <button onClick={onClose} className="rounded-full bg-slate-700 px-4 py-2 text-sm text-white hover:bg-slate-600">
          {t("common.close")}
        </button>
      </div>
    );
  }

  return (
    <div className="mt-4 max-w-2xl space-y-4 rounded-2xl border border-sky-800/50 bg-slate-900/60 p-6">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-white">
            {t("publish.trackerUploadPanel.header.title", { tracker: tracker.source || tracker.name })}
          </h2>
          <p className="mt-1 text-xs text-slate-400">
            {t("publish.trackerUploadPanel.header.subtitle")}
          </p>
        </div>
        {infer?.tmdb?.poster_url && (
          <img src={infer.tmdb.poster_url} alt={t("publish.trackerUploadPanel.posterAlt")} className="h-24 w-auto shrink-0 rounded-md" />
        )}
      </div>

      {loading ? (
        <p className="text-sm text-slate-400">{t("publish.trackerUploadPanel.loading")}</p>
      ) : (
        <>
          {infer?.tmdb_note && !infer.tmdb && <p className="text-xs text-amber-400">⚠ {infer.tmdb_note}</p>}

          <div>
            <label className="mb-1 block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.name")}</label>
            <input value={form.name} onChange={(e) => set("name", e.target.value)} className={inputCls} />
            {suggestDual && (
              <button
                type="button"
                onClick={() => set("name", addDualToName(form.name))}
                title={t("publish.trackerUploadPanel.fields.dualTitle")}
                className="mt-1 rounded-full border border-amber-600/50 bg-amber-500/10 px-2 py-0.5 text-[11px] text-amber-300 transition hover:bg-amber-500/20"
              >
                🎧 {t("publish.trackerUploadPanel.fields.dualSuggest", { count: audioLangs.length, langs: audioLangs.join(", ") })}
              </button>
            )}
          </div>

          <TaxonomySelects
            profile={tracker.profile}
            values={form}
            onChange={(axis, id) => set(`${axis}_id`, id)}
            inputCls={inputCls}
          />
          {!canPublish(tracker.profile) && (
            <p className="text-xs text-amber-400">
              ⚠ {t("publish.trackerUploadPanel.warnings.noTaxonomy", { name: tracker.name })}
            </p>
          )}

          {/* Games (#17): searches IGDB instead of TMDB/IMDB. The operator chooses the match
              (IGDB search sometimes prioritizes mods/hacks) → fills the igdb_id + description. */}
          {isGame && (
            <div className="space-y-2 rounded-xl border border-fuchsia-800/40 bg-fuchsia-500/5 p-3">
              <label className="block text-xs font-medium text-fuchsia-300">🎮 {t("publish.trackerUploadPanel.game.searchLabel")}</label>
              <p className="text-[11px] leading-snug text-slate-400">
                {rich(t("publish.trackerUploadPanel.game.patternHint"))}
              </p>
              <div className="flex items-center gap-2">
                <input
                  value={gameQ}
                  onChange={(e) => setGameQ(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), runGameSearch())}
                  placeholder={form.name || t("publish.trackerUploadPanel.game.placeholder")}
                  className={inputCls}
                />
                <button
                  type="button"
                  onClick={runGameSearch}
                  disabled={gameBusy}
                  className="shrink-0 rounded-full bg-fuchsia-600 px-3 py-2 text-sm text-white transition hover:bg-fuchsia-500 disabled:opacity-40"
                >
                  {gameBusy ? "…" : t("publish.trackerUploadPanel.game.searchBtn")}
                </button>
              </div>
              {gameErr && <p className="text-xs text-red-400">{gameErr}</p>}
              {form.igdb > 0 && (
                <p className="text-xs text-emerald-400">✓ {t("publish.trackerUploadPanel.game.selected", { id: form.igdb })}</p>
              )}
              {games && games.length === 0 && !gameBusy && (
                <p className="text-xs text-slate-400">{t("publish.trackerUploadPanel.game.noResults")}</p>
              )}
              {games && games.length > 0 && (
                <ul className="max-h-64 space-y-1 overflow-y-auto">
                  {games.map((g) => (
                    <li key={g.igdb_id}>
                      <button
                        type="button"
                        onClick={() => pickGame(g)}
                        className="flex w-full items-center gap-3 rounded-lg border border-slate-800 px-2 py-1.5 text-left transition hover:border-fuchsia-600 hover:bg-slate-800/50"
                      >
                        {g.cover_url ? (
                          <img src={g.cover_url} alt="" className="h-12 w-auto shrink-0 rounded" />
                        ) : (
                          <span className="flex h-12 w-9 shrink-0 items-center justify-center rounded bg-slate-800 text-slate-600">🎮</span>
                        )}
                        <span className="min-w-0">
                          <span className="block truncate text-sm text-slate-200">
                            {g.name} {g.year ? <span className="text-slate-500">({g.year})</span> : null}
                          </span>
                          <span className="block truncate text-xs text-slate-500">
                            {(g.platforms ?? []).slice(0, 3).join(", ") || "—"}
                          </span>
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}

          {!isGame && (
            <div className="grid gap-3 sm:grid-cols-3">
              <div>
                <label className="mb-1 block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.tmdb")}</label>
                <input value={form.tmdb || ""} onChange={(e) => set("tmdb", num(e.target.value))} className={`${inputCls} font-mono`} />
              </div>
              <div>
                <label className="mb-1 block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.imdb")}</label>
                <input value={form.imdb} onChange={(e) => set("imdb", e.target.value)} className={`${inputCls} font-mono`} />
              </div>
              {isTV && (
                <div>
                  <label className="mb-1 block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.tvdb")}</label>
                  <input value={form.tvdb || ""} onChange={(e) => set("tvdb", num(e.target.value))} className={`${inputCls} font-mono`} />
                </div>
              )}
            </div>
          )}

          {isTV && (
            <div className="grid gap-3 sm:grid-cols-3">
              <div>
                <label className="mb-1 block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.season")}</label>
                <input value={form.season_number || ""} onChange={(e) => set("season_number", num(e.target.value))} className={`${inputCls} font-mono`} />
              </div>
              <div>
                <label className="mb-1 block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.episode")}</label>
                <input value={form.episode_number || ""} onChange={(e) => set("episode_number", num(e.target.value))} className={`${inputCls} font-mono`} />
              </div>
            </div>
          )}

          <div>
            <div className="mb-1 flex items-center justify-between">
              <label className="block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.description")}</label>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={handleGenerateScreenshots}
                  disabled={genLoading}
                  title={t("publish.trackerUploadPanel.actions.genScreenshotsTitle")}
                  className="rounded-full border border-slate-700 px-2 py-0.5 text-xs text-sky-300 transition hover:bg-slate-800 disabled:opacity-40"
                >
                  {genLoading ? t("publish.trackerUploadPanel.actions.genScreenshotsLoading") : t("publish.trackerUploadPanel.actions.genScreenshots")}
                </button>
                {!isGame && (
                  <button
                    type="button"
                    onClick={handleGenerateCard}
                    disabled={genLoading}
                    title={t("publish.trackerUploadPanel.actions.genSheetTitle")}
                    className="rounded-full border border-slate-700 px-2 py-0.5 text-xs text-sky-300 transition hover:bg-slate-800 disabled:opacity-50"
                  >
                    {genLoading ? t("publish.trackerUploadPanel.actions.genSheetLoading") : t("publish.trackerUploadPanel.actions.genSheet")}
                  </button>
                )}
              </div>
            </div>
            {formSteps && <FormStepBar steps={formSteps} />}
            {genError && <p className="mb-1 text-xs text-amber-400">{genError}</p>}
            {shotResult && (
              <div className="mb-1 space-y-1 rounded-lg border border-emerald-700/40 bg-emerald-500/5 px-3 py-2 text-xs">
                <p className="text-emerald-300">
                  {rich(t("publish.trackerUploadPanel.shots.hostedAt"), { host: <span className="font-mono">{shotResult.host}</span> })}
                </p>
                {shotResult.failures.length > 0 && (
                  <ul className="space-y-0.5">
                    {shotResult.failures.map((f, i) => (
                      <li key={i} className="text-amber-300">
                        ⚠ <span className="font-mono">{f.host}</span>: {f.reason}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
            <textarea value={form.description} onChange={(e) => set("description", e.target.value)} rows={6} className={`${inputCls} resize-y font-mono text-xs`} />
          </div>

          {!isGame && (
            <div>
              <label className="mb-1 block text-xs text-slate-400">
                {mediaInfo ? t("publish.trackerUploadPanel.fields.mediainfoAvailable") : t("publish.trackerUploadPanel.fields.mediainfoMissing")}
              </label>
              <textarea value={form.mediainfo} onChange={(e) => set("mediainfo", e.target.value)} rows={5} className={`${inputCls} resize-y font-mono text-xs`} />
            </div>
          )}

          <div className="flex flex-wrap items-center gap-4">
            <div className="flex-1">
              <label className="mb-1 block text-xs text-slate-400">{t("publish.trackerUploadPanel.fields.keywords")}</label>
              <input value={form.keywords} onChange={(e) => set("keywords", e.target.value)} className={inputCls} />
            </div>
            <label className="mt-4 flex items-center gap-2 text-xs text-slate-300">
              <input type="checkbox" checked={form.anonymous} onChange={(e) => set("anonymous", e.target.checked)} className="accent-emerald-500" />
              {t("publish.trackerUploadPanel.fields.anonymous")}
            </label>
          </div>

          {/* Duplicate evidence (#186): tracker already has something similar? Show and let
              the operator judge — never block on its own. */}
          {dupes && dupes.candidates.length > 0 && (
            <div className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3">
              <p className="text-sm font-medium text-amber-300">
                {t("publish.trackerUploadPanel.dupes.found", {
                  count: dupes.candidates.length,
                  tracker: tracker.source || tracker.name,
                  query: dupes.query,
                })}
              </p>
              <ul className="mt-2 space-y-1">
                {dupes.candidates.map((c) => (
                  <li key={c.id} className="flex flex-wrap items-center gap-2 text-xs text-slate-300">
                    {c.details_link ? (
                      <a href={c.details_link} target="_blank" rel="noreferrer" className="text-sky-400 hover:underline">
                        {c.name}
                      </a>
                    ) : (
                      <span>{c.name}</span>
                    )}
                    {c.same_year && <span className="rounded bg-amber-500/20 px-1.5 py-0.5 text-amber-300">{t("publish.trackerUploadPanel.dupes.sameYear")}</span>}
                    {c.resolution && <span className="text-slate-500">{c.resolution}</span>}
                    {c.type && <span className="text-slate-500">{c.type}</span>}
                    <span className="text-slate-500">↑{c.seeders}</span>
                  </li>
                ))}
              </ul>
              <label className="mt-3 flex items-center gap-2 text-xs text-amber-200">
                <input type="checkbox" checked={dupeAck} onChange={(e) => setDupeAck(e.target.checked)} className="accent-amber-500" />
                {t("publish.trackerUploadPanel.dupes.ack")}
              </label>
            </div>
          )}
          {dupes?.note && <p className="text-xs text-slate-500">{dupes.note}</p>}

          {/* Frozen preview (#188): UNIT3D doesn't have dry-run, so the only way for
              the operator to see what will be posted is here — before, not after. */}
          {preview && (
            <div className="rounded-lg border border-slate-700 bg-slate-900/60 p-3">
              <p className="text-sm font-medium text-slate-200">
                {rich(t("publish.trackerUploadPanel.preview.endpoint"), { endpoint: <span className="font-mono text-xs text-slate-400">{preview.endpoint}</span> })}
              </p>
              {preview.missing.length > 0 && (
                <p className="mt-1 text-xs text-amber-400">{t("publish.trackerUploadPanel.preview.missing", { fields: preview.missing.join(", ") })}</p>
              )}
              <dl className="mt-2 grid grid-cols-[max-content_1fr] gap-x-3 gap-y-1 font-mono text-xs">
                {Object.entries(preview.fields)
                  .sort(([a], [b]) => a.localeCompare(b))
                  .map(([k, v]) => (
                    <div key={k} className="contents">
                      <dt className="text-slate-500">{k}</dt>
                      <dd className="truncate text-slate-300" title={v}>
                        {v === "" ? <span className="text-slate-600">{t("publish.trackerUploadPanel.preview.empty")}</span> : v}
                      </dd>
                    </div>
                  ))}
                {/* multipart field name, like the other lines (k): not UI text */}
                <dt className="text-slate-500">{"torrent"}</dt>
                <dd className="text-slate-300">
                  {preview.torrent_name} ({(preview.torrent_size / 1024).toFixed(1)} KB)
                </dd>
              </dl>
              <p className="mt-2 text-xs text-slate-500">
                {t("publish.trackerUploadPanel.preview.warning")}
              </p>
            </div>
          )}

          {error && <p className="text-sm text-red-400">{error}</p>}

          <div className="flex items-center gap-2">
            <button
              onClick={preview ? publish : openPreview}
              disabled={busy || (!!dupes && dupes.candidates.length > 0 && !dupeAck)}
              className="rounded-full bg-sky-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-sky-500 disabled:opacity-50"
            >
              {busy
                ? preview
                  ? t("publish.trackerUploadPanel.actions.submitting")
                  : t("publish.trackerUploadPanel.actions.previewing")
                : preview
                  ? t("publish.trackerUploadPanel.actions.publish", { tracker: tracker.source || tracker.name })
                  : t("publish.trackerUploadPanel.actions.check")}
            </button>
            <button onClick={onClose} disabled={busy} className="rounded-full px-4 py-2 text-sm text-slate-400 hover:text-white disabled:opacity-50">
              {t("publish.trackerUploadPanel.actions.notNow")}
            </button>
          </div>
        </>
      )}
    </div>
  );
}

// FormStep is a step in the full sheet's step bar (F1).
type FormStepState = "pending" | "active" | "done" | "error" | "skipped";
type FormStep = { label: string; state: FormStepState; detail?: string };

// CMD_STAGE_KEY humanizes the extraction command status for the bar (same vocabulary
// as the action toasts).
const CMD_STAGE_KEY: Record<string, Key> = {
  PENDING: "publish.trackerUploadPanel.cmdStage.pending",
  DISPATCHED: "publish.trackerUploadPanel.cmdStage.dispatched",
};

// FormStepBar shows the progress of the full sheet steps. Each step has an icon
// per state; the active step spins, the completed one turns green, the failed one turns amber (not
// red: screenshot is optional and the sheet goes through anyway), the skipped one is dimmed.

function FormStepBar({ steps }: { steps: FormStep[] }) {
  const icon: Record<FormStepState, string> = {
    pending: "○",
    active: "◐",
    done: "✓",
    error: "!",
    skipped: "–",
  };
  const color: Record<FormStepState, string> = {
    pending: "text-slate-500",
    active: "text-sky-300",
    done: "text-emerald-400",
    error: "text-amber-400",
    skipped: "text-slate-600",
  };
  return (
    <div className="mb-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
      {steps.map((s, i) => (
        <span key={i} className={`flex items-center gap-1 ${color[s.state]}`}>
          <span className={s.state === "active" ? "animate-spin" : ""}>{icon[s.state]}</span>
          <span>{s.label}</span>
          {s.detail && <span className="text-slate-500">· {s.detail}</span>}
          {i < steps.length - 1 && <span className="text-slate-700">→</span>}
        </span>
      ))}
    </div>
  );
}
