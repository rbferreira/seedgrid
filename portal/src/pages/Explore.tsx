import { useEffect, useState, useCallback, type ReactNode } from "react";
import {
  getTrackers,
  browseTracker,
  seedFromTracker,
  ApiError,
  type Tracker,
  type TrackerBrowseItem,
  type MediaType,
} from "../api";
import { fmtBytes, TRANSFER } from "../components/ui";
import { Spinner, toast } from "../actions";
import { useT, type TFn } from "../i18n";

// Explore (#12/#13, catalog redesign): the Control Plane browses the UNIT3D tracker + enriches
// each item with TMDB (movie/series), IGDB (game) or Open Library (book/audiobook) —
// poster, PT title, genres, original language. Audio/subtitle languages and resolution
// come from parsing the release name (UNIT3D doesn't return them). Layout: row card with poster
// on the left (140×200), featured PT title on the right, original title, chips and action.
function dedupById(arr: TrackerBrowseItem[]): TrackerBrowseItem[] {
  const seen = new Set<string>();
  return arr.filter((x) => (seen.has(x.id) ? false : (seen.add(x.id), true)));
}

// Type emoji — becomes the missing poster placeholder AND the chip icon. No inventing
// icon libraries: emojis have universal support and match the screen's lighter tone.
const MEDIA_ICONS: Record<string, string> = {
  movie: "🎬",
  tv: "📺",
  game: "🎮",
  book: "📚",
  audiobook: "🎧",
  other: "📦",
  "": "📦",
};

// Type chip color — each media type has its own tone to catch the eye and classify.
const MEDIA_CHIP: Record<string, string> = {
  movie: "bg-indigo-500/15 text-indigo-300 ring-indigo-500/30",
  tv: "bg-purple-500/15 text-purple-300 ring-purple-500/30",
  game: "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30",
  book: "bg-amber-500/15 text-amber-300 ring-amber-500/30",
  audiobook: "bg-pink-500/15 text-pink-300 ring-pink-500/30",
  other: "bg-slate-500/15 text-slate-300 ring-slate-500/30",
  "": "bg-slate-500/15 text-slate-300 ring-slate-500/30",
};

function mediaLabel(t: TFn, mt?: MediaType) {
  const map: Record<string, string> = {
    movie: t("explore.explore.media.movie"),
    tv: t("explore.explore.media.tv"),
    game: t("explore.explore.media.game"),
    book: t("explore.explore.media.book"),
    audiobook: t("explore.explore.media.audiobook"),
    other: t("explore.explore.media.other"),
    "": t("explore.explore.media.item"),
  };
  return map[mt ?? ""] ?? t("explore.explore.media.item");
}
function mediaIcon(t?: MediaType) {
  return MEDIA_ICONS[t ?? ""] ?? "📦";
}
function mediaChip(t?: MediaType) {
  return MEDIA_CHIP[t ?? ""] ?? MEDIA_CHIP[""];
}

// parseSeasonEpisode: extracts "S05" / "S01E12" / "Season 3" from the release name. Only used
// for series — shows "Season X" (or "Season X · Ep. Y" when the release is a single
// episode). Returns null when nothing is found. Frontend parses it (same heuristic
// as release/langs.go) to avoid needing a new backend field.
function parseSeasonEpisode(name: string): { season: number; episode?: number } | null {
  const m = name.match(/\bS(\d{1,2})(?:[\s._-]?E(\d{1,3}))?\b/i);
  if (m) {
    const season = parseInt(m[1], 10);
    const episode = m[2] ? parseInt(m[2], 10) : undefined;
    if (season >= 1 && season <= 99) return { season, episode };
  }
  const s = name.match(/\bSeason[\s._-]?(\d{1,2})\b/i);
  if (s) {
    const season = parseInt(s[1], 10);
    if (season >= 1 && season <= 99) return { season };
  }
  return null;
}

// Generic chip. `title` is optional for tooltip when the label might be truncated.
function Chip({ children, tone, title }: { children: ReactNode; tone?: string; title?: string }) {
  return (
    <span
      title={title}
      className={
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2.5 py-1 text-xs font-medium ring-1 ring-inset " +
        (tone ?? "bg-slate-800 text-slate-300 ring-slate-700")
      }
    >
      {children}
    </span>
  );
}

// `can` gates the "Download & Seed" button by the permission of the route it calls (trackers:seed) —
// button-level gate, docs/11 - Portal.md §13.1. The catalog is open via torrents:read, which Viewer
// and Downloader have; the data (showcase, "already in the cluster") remains visible, only the button disappears.
export function Explore({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [trackers, setTrackers] = useState<Tracker[]>([]);
  const [trackerId, setTrackerId] = useState("");
  const [items, setItems] = useState<TrackerBrowseItem[]>([]);
  const [cursor, setCursor] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const [q, setQ] = useState(""); // search field text
  const [activeQ, setActiveQ] = useState(""); // term actually sent to the tracker (#13)
  // Client-side filters (refine what has already been loaded from the server).
  const [fType, setFType] = useState<MediaType | "">("");
  const [fSize, setFSize] = useState(0);
  const [fSeeds, setFSeeds] = useState(0);
  const [fStatus, setFStatus] = useState<"all" | "avail" | "have">("all");

  // The NAME search is server-side (#13); here we only refine by type/size/seeds/status.
  const filtered = items.filter(
    (it) =>
      (!fType || (it.media_type ?? "") === fType) &&
      it.size >= fSize &&
      it.seeders >= fSeeds &&
      (fStatus === "all" || (fStatus === "have" ? it.in_cluster : !it.in_cluster)),
  );

  const integ = trackers.filter((t) => t.source && t.has_api_token);

  useEffect(() => {
    (async () => {
      try {
        const ts = await getTrackers();
        setTrackers(ts);
        const first = ts.find((t) => t.source && t.has_api_token);
        if (first) setTrackerId(first.id);
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) onLogout();
      }
    })();
  }, [onLogout]);

  // load: cur="" restarts; append accumulates (with dedup by id). query = server-side search.
  const load = useCallback(
    async (id: string, cur: string, append: boolean, query: string) => {
      if (!id) return;
      setLoading(true);
      setError("");
      try {
        const res = await browseTracker(id, cur, query);
        setItems((prev) => (append ? dedupById([...prev, ...res.items]) : res.items));
        setCursor(res.next_cursor || "");
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          onLogout();
          return;
        }
        setError(err instanceof ApiError ? err.message : t("explore.explore.error.listFailed"));
      } finally {
        setLoading(false);
      }
    },
    [onLogout, t],
  );

  // (Re)loads when changing tracker.
  useEffect(() => {
    if (!trackerId) return;
    setQ("");
    setActiveQ("");
    setItems([]);
    setCursor("");
    load(trackerId, "", false, "");
  }, [trackerId, load]);

  // doSearch: triggers the server-side search (#13). Enter/magnifier — not on every keystroke (the tracker
  // has aggressive throttling). Empty term goes back to listing the entire catalog.
  function doSearch() {
    const term = q.trim();
    setActiveQ(term);
    setItems([]);
    setCursor("");
    load(trackerId, "", false, term);
  }

  const hasFilter = !!activeQ || !!fType || fSize > 0 || fSeeds > 0 || fStatus !== "all";
  function clearFilters() {
    setQ("");
    setActiveQ("");
    setFType("");
    setFSize(0);
    setFSeeds(0);
    setFStatus("all");
    setItems([]);
    setCursor("");
    load(trackerId, "", false, ""); // clearing also resets the server-side search
  }

  const markInCluster = (id: string) =>
    setItems((prev) => prev.map((x) => (x.id === id ? { ...x, in_cluster: true } : x)));

  async function seed(it: TrackerBrowseItem) {
    setBusyId(it.id);
    setError("");
    try {
      try {
        await seedFromTracker(trackerId, it.id, false);
      } catch (e) {
        // #11: already exists in the cluster → confirms the 2nd copy.
        if (e instanceof ApiError && e.status === 409 && e.code === "duplicate") {
          if (!window.confirm(`${e.message}\n\n${t("explore.explore.confirmDuplicate")}`)) {
            return;
          }
          await seedFromTracker(trackerId, it.id, true);
        } else {
          throw e;
        }
      }
      markInCluster(it.id);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      toast(err instanceof ApiError ? err.message : t("explore.explore.error.seedFailed"), "error");
    } finally {
      setBusyId("");
    }
  }

  return (
    <>
      <h1 className="text-lg font-semibold text-white">{t("explore.explore.title")}</h1>
      <p className="text-sm text-slate-400">
        {t("explore.explore.description")}
      </p>

      {integ.length === 0 ? (
        <div className="max-w-2xl rounded-2xl border border-slate-800 bg-slate-900/60 p-6 text-sm text-slate-400">
          {t("explore.explore.noIntegration")}{" "}
          <a href="#/settings" className="text-emerald-400 hover:underline">
            {t("explore.explore.trackersLink")}
          </a>
          .
        </div>
      ) : (
        <>
          {integ.length > 1 && (
            <div className="max-w-xs">
              <label className="mb-1 block text-sm text-slate-300">{t("explore.explore.filters.tracker")}</label>
              <select
                value={trackerId}
                onChange={(e) => setTrackerId(e.target.value)}
                className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
              >
                {integ.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.source || t.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          {error && (
            <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
              {error}
            </div>
          )}

          {/* Search bar + filters (server-side by name, client-side for the rest). */}
          <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex min-w-[240px] flex-1 items-center gap-1">
                <input
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && doSearch()}
                  placeholder={t("explore.explore.search.placeholder")}
                  className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-base text-white outline-none focus:border-emerald-500"
                />
                <button
                  onClick={doSearch}
                  disabled={loading}
                  title={t("explore.explore.search.buttonTitle")}
                  className="shrink-0 rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-40"
                >
                  {t("explore.explore.search.button")}
                </button>
              </div>
              <select
                value={fType}
                onChange={(e) => setFType(e.target.value as MediaType | "")}
                title={t("explore.explore.filters.typeTitle")}
                className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
              >
                <option value="">{t("explore.explore.filters.allTypes")}</option>
                <option value="movie">🎬 {t("explore.explore.media.moviePlural")}</option>
                <option value="tv">📺 {t("explore.explore.media.tvPlural")}</option>
                <option value="game">🎮 {t("explore.explore.media.gamePlural")}</option>
                <option value="book">📚 {t("explore.explore.media.bookPlural")}</option>
                <option value="audiobook">🎧 {t("explore.explore.media.audiobookPlural")}</option>
                <option value="other">📦 {t("explore.explore.media.other")}</option>
              </select>
              <select
                value={fSize}
                onChange={(e) => setFSize(Number(e.target.value))}
                title={t("explore.explore.filters.sizeTitle")}
                className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
              >
                <option value={0}>{t("explore.explore.filters.anySize")}</option>
                <option value={104857600}>{"> 100 MB"}</option>
                <option value={1073741824}>{"> 1 GB"}</option>
                <option value={5368709120}>{"> 5 GB"}</option>
              </select>
              <input
                type="number"
                min={0}
                value={fSeeds || ""}
                onChange={(e) => setFSeeds(Number(e.target.value) || 0)}
                placeholder={t("explore.explore.filters.minSeedsPlaceholder")}
                title={t("explore.explore.filters.minSeedsTitle")}
                className="w-28 rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
              />
              <select
                value={fStatus}
                onChange={(e) => setFStatus(e.target.value as "all" | "avail" | "have")}
                title={t("explore.explore.filters.statusTitle")}
                className="rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-sm text-white outline-none focus:border-emerald-500"
              >
                <option value="all">{t("explore.explore.filters.statusAll")}</option>
                <option value="avail">{t("explore.explore.filters.statusAvail")}</option>
                <option value="have">{t("explore.explore.filters.statusHave")}</option>
              </select>
              {hasFilter && (
                <button
                  onClick={clearFilters}
                  className="text-sm text-slate-400 underline hover:text-slate-200"
                >
                  {t("explore.explore.filters.clear")}
                </button>
              )}
              <span className="ml-auto text-sm text-slate-500">
                {activeQ ? `${t("explore.explore.stats.searchPrefix", { query: activeQ })}: ` : ""}
                {t("explore.explore.stats.count", { shown: filtered.length, total: items.length })}
              </span>
            </div>
          </div>

          {/* List cards. Each card is a <li> with a 140×200 poster on the left + details. */}
          <ul className="grid gap-3">
            {filtered.map((it) => (
              <CatalogCard
                key={it.id}
                item={it}
                busy={busyId === it.id}
                canSeed={can("trackers:seed")}
                onSeed={() => seed(it)}
              />
            ))}
            {filtered.length === 0 && !loading && (
              <li className="rounded-2xl border border-slate-800 bg-slate-900/60 px-4 py-10 text-center text-sm text-slate-500">
                {items.length === 0 ? t("explore.explore.empty.catalog") : t("explore.explore.empty.filters")}
              </li>
            )}
            {loading && items.length === 0 && (
              // Skeleton for the first 4 cards while the 1st page hasn't returned.
              <>
                {[0, 1, 2, 3].map((i) => (
                  <SkeletonCard key={i} />
                ))}
              </>
            )}
          </ul>

          <div className="flex items-center gap-3">
            {cursor && (
              <button
                onClick={() => load(trackerId, cursor, true, activeQ)}
                disabled={loading}
                className="rounded-lg border border-slate-700 px-4 py-2 text-sm text-slate-300 transition hover:bg-slate-800 disabled:opacity-40"
              >
                {loading ? t("common.loading") : t("explore.explore.loadMore")}
              </button>
            )}
          </div>
        </>
      )}
    </>
  );
}

// CatalogCard: catalog row. Poster on the left (140×200; themed placeholder
// when enrichment didn't find one); details on the right (featured type badge at the
// top, PT title, meta line with season+year, chips, synopsis, footer with size/
// seeds/button). Item marked as "already in the cluster" gets a greenish strip.
function CatalogCard({
  item,
  busy,
  canSeed,
  onSeed,
}: {
  item: TrackerBrowseItem;
  busy: boolean;
  canSeed: boolean;
  onSeed: () => void;
}) {
  const t = useT();
  const mt = item.media_type ?? "";
  const isBook = mt === "book" || mt === "audiobook";
  const isTV = mt === "tv";
  const displayTitle = item.title_pt || item.title_original || item.name;
  // "Original Title" for a book is the author (the Control Plane reuses the original_name slot).
  const originalLine = isBook
    ? item.title_original // = author
    : item.title_original && item.title_original !== displayTitle
      ? item.title_original
      : "";

  // Series: extracts season/episode from the release name to show in the meta line
  // ("Season 5 · 2012") — the CP doesn't have this separate field, it's a local heuristic.
  const se = isTV ? parseSeasonEpisode(item.name) : null;
  const seasonLabel = se
    ? se.episode != null
      ? t("explore.explore.card.seasonEpisode", { season: se.season, episode: se.episode })
      : t("explore.explore.card.season", { season: se.season })
    : "";

  return (
    <li
      className={
        "flex gap-4 overflow-hidden rounded-2xl border bg-slate-900/60 p-4 transition hover:border-slate-700 " +
        (item.in_cluster ? "border-emerald-500/20" : "border-slate-800")
      }
    >
      {/* Fixed poster at 140×200: consistent across rows even when TMDB returns
          posters with slightly different proportions (object-cover crops the excess). */}
      <div className="relative shrink-0">
        <div className="h-[200px] w-[140px] overflow-hidden rounded-lg bg-slate-950 ring-1 ring-slate-800">
          {item.poster_url ? (
            <img
              src={item.poster_url}
              alt=""
              loading="lazy"
              className="h-full w-full object-cover"
              onError={(e) => {
                // Silent fallback: if the provider URL breaks (404, CORS), it hides
                // the image and shows the placeholder underneath.
                (e.currentTarget as HTMLImageElement).style.display = "none";
              }}
            />
          ) : null}
          {!item.poster_url && (
            <div className="flex h-full w-full flex-col items-center justify-center gap-1 text-slate-600">
              <span className="text-5xl" aria-hidden>
                {mediaIcon(item.media_type)}
              </span>
              <span className="text-[11px] uppercase tracking-widest">{t("explore.explore.card.noPoster")}</span>
            </div>
          )}
        </div>
      </div>

      {/* Details. min-w-0 is essential: without it, long titles push the card and break
          the layout (flexbox uses intrinsic width by default). truncate + line-clamp
          ensure nothing overflows. */}
      <div className="flex min-w-0 flex-1 flex-col gap-2.5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            {/* Type badge above the title — uppercase + colored, in the style of
                Netflix/Plex ("SERIES" before the episode name). It's the first
                element the eye encounters. */}
            <div
              className={
                "mb-1 inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[11px] font-bold uppercase tracking-wider ring-1 ring-inset " +
                mediaChip(item.media_type)
              }
            >
              <span aria-hidden>{mediaIcon(item.media_type)}</span>
              {mediaLabel(t, item.media_type)}
            </div>
            <div className="flex items-center gap-2">
              <h3 className="truncate text-lg font-semibold text-white" title={displayTitle}>
                {displayTitle}
              </h3>
              {item.details_link && (
                <a
                  href={item.details_link}
                  target="_blank"
                  rel="noreferrer"
                  title={t("explore.explore.card.openTracker")}
                  className="shrink-0 text-base text-slate-500 hover:text-emerald-400"
                >
                  ↗
                </a>
              )}
            </div>
            {/* Meta line: for books, "by Author · Year"; for series, "Season X · Year";
                for movies, "Original Title · Year". If there's only a year, it shows it alone. */}
            {(originalLine || seasonLabel || item.year > 0) && (
              <div className="truncate text-sm text-slate-500" title={originalLine || seasonLabel}>
                {isTV ? seasonLabel : isBook && originalLine ? t("explore.explore.card.byAuthor", { author: originalLine }) : originalLine}
                {(isTV ? seasonLabel : originalLine) && item.year > 0 && " · "}
                {item.year > 0 && item.year}
              </div>
            )}
          </div>

          {/* "in the cluster" status vs action button. Reserved on the right so the eye goes
              straight to "can download / already have it". */}
          <div className="shrink-0">
            {item.in_cluster ? (
              <span className="inline-flex rounded-full bg-emerald-500/15 px-3 py-1.5 text-sm font-medium text-emerald-400 ring-1 ring-inset ring-emerald-500/30">
                ✓ {t("explore.explore.card.inCluster")}
              </span>
            ) : (
              canSeed && (
                <button
                  onClick={onSeed}
                  disabled={busy}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-emerald-500 disabled:opacity-40"
                >
                  {busy ? <Spinner /> : "⬇"} {t("explore.explore.card.seedButton")}
                </button>
              )
            )}
          </div>
        </div>

        {/* Secondary chips: resolution + audio + subtitles + genres (up to 3). The type
            already went up as a badge at the top — no need to repeat it here. */}
        <div className="flex flex-wrap items-center gap-1.5">
          {item.resolution && (
            <Chip tone="bg-slate-800 text-slate-200 ring-slate-700">{item.resolution.toUpperCase()}</Chip>
          )}
          {(item.audio_langs ?? []).map((a) => (
            <Chip key={"a-" + a} tone="bg-blue-500/10 text-blue-300 ring-blue-500/30">
              🔊 {a}
            </Chip>
          ))}
          {(item.subtitle_langs ?? []).map((s) => (
            <Chip key={"s-" + s} tone="bg-cyan-500/10 text-cyan-300 ring-cyan-500/30">
              💬 {s}
            </Chip>
          ))}
          {(item.genres ?? []).slice(0, 3).map((g) => (
            <Chip key={"g-" + g} title={g}>
              {g}
            </Chip>
          ))}
          {(item.genres?.length ?? 0) > 3 && (
            <Chip title={item.genres!.slice(3).join(", ")}>+{item.genres!.length - 3}</Chip>
          )}
        </div>

        {/* Synopsis: 2 lines, the rest disappears via line-clamp — avoids uneven height
            cards when a movie has a huge synopsis and another just one sentence. */}
        {item.overview && (
          <p
            className="text-sm leading-relaxed text-slate-400 [display:-webkit-box] [-webkit-box-orient:vertical] [-webkit-line-clamp:2] overflow-hidden"
            title={item.overview}
          >
            {item.overview}
          </p>
        )}

        {/* Footer: torrent technical data (size, seeders, release name).
            Kept in mono to visually separate it from the "curation" part. */}
        <div className="mt-auto flex flex-wrap items-center gap-x-4 gap-y-1 pt-1 text-xs text-slate-500">
          <span className="font-medium text-slate-300">{fmtBytes(item.size)}</span>
          <span className={`font-medium ${TRANSFER.up.text}`}>↑ {item.seeders} seeds</span>
          {item.leechers > 0 && (
            <span className={TRANSFER.down.text}>↓ {item.leechers} peers</span>
          )}
          <span className="min-w-0 flex-1 truncate font-mono text-slate-600" title={item.name}>
            {item.name}
          </span>
        </div>
      </div>
    </li>
  );
}

// SkeletonCard: 1st page placeholder. Same footprint (200px height, 140×200 on the
// poster) as the real card → no "jump" transition when data arrives.
function SkeletonCard() {
  return (
    <li className="flex gap-4 overflow-hidden rounded-2xl border border-slate-800 bg-slate-900/60 p-4">
      <div className="h-[200px] w-[140px] shrink-0 animate-pulse rounded-lg bg-slate-800/70" />
      <div className="flex min-w-0 flex-1 flex-col gap-2.5">
        <div className="h-5 w-20 animate-pulse rounded-md bg-slate-800/70" />
        <div className="h-5 w-2/3 animate-pulse rounded bg-slate-800/70" />
        <div className="h-4 w-1/3 animate-pulse rounded bg-slate-800/70" />
        <div className="flex gap-2 pt-1">
          <div className="h-6 w-16 animate-pulse rounded-full bg-slate-800/70" />
          <div className="h-6 w-16 animate-pulse rounded-full bg-slate-800/70" />
          <div className="h-6 w-20 animate-pulse rounded-full bg-slate-800/70" />
        </div>
        <div className="h-4 w-full animate-pulse rounded bg-slate-800/70" />
        <div className="h-4 w-5/6 animate-pulse rounded bg-slate-800/70" />
      </div>
    </li>
  );
}
