// Upload description composition (#192). Pure functions: the component only renders and calls the
// API; the decision of "what the editor shows, what changes, and what is considered a change" stays here,
// where it can be tested.
//
// The COMPOSITION itself — building the BBCode — is not in the Portal: it belongs to the Control Plane
// (POST /api/v1/publish/description), per tracker. Here only lives what the screen needs to edit
// the configuration of this composition: enabling, disabling and reordering blocks, and choosing the MediaInfo tag.

import type { DescriptionConfig, TrackerProfile } from "../api";
import type { Key, TFn } from "../i18n";

export type DescriptionKind = "media" | "game";

export const KIND_TITLE_KEY: Record<DescriptionKind, Key> = {
  media: "trackers.description.kindTitle.media",
  game: "trackers.description.kindTitle.game",
};

/** Label for each block. The set of ids is CLOSED on the server; a new id falls back to its own id. */
export const BLOCK_LABEL_KEY: Record<string, Key> = {
  poster: "trackers.description.blockLabel.poster",
  synopsis: "trackers.description.blockLabel.synopsis",
  tags: "trackers.description.blockLabel.tags",
  screenshots: "trackers.description.blockLabel.screenshots",
  mediainfo: "trackers.description.blockLabel.mediainfo",
  cover: "trackers.description.blockLabel.cover",
  title: "trackers.description.blockLabel.title",
  about: "trackers.description.blockLabel.about",
  genres: "trackers.description.blockLabel.genres",
  platforms: "trackers.description.blockLabel.platforms",
};

/** What each block puts in the description — the editor's supporting line. */
export const BLOCK_HINT_KEY: Record<string, Key> = {
  poster: "trackers.description.blockHint.poster",
  synopsis: "trackers.description.blockHint.synopsis",
  tags: "trackers.description.blockHint.tags",
  screenshots: "trackers.description.blockHint.screenshots",
  mediainfo: "trackers.description.blockHint.mediainfo",
  cover: "trackers.description.blockHint.cover",
  title: "trackers.description.blockHint.title",
  about: "trackers.description.blockHint.about",
  genres: "trackers.description.blockHint.genres",
  platforms: "trackers.description.blockHint.platforms",
};

/** The tags the server accepts to wrap the MediaInfo (closed set). */
export const MEDIAINFO_TAGS = ["mediainfo", "code", "pre", "spoiler"] as const;

/** Content languages a site can ask for (#74), the default first. */
export const CONTENT_LANGUAGES = ["en", "pt-BR"] as const;
export const contentLanguage = (cfg: DescriptionConfig) => cfg.language || "en";

export const blockLabel = (t: TFn, id: string) => {
  const key = BLOCK_LABEL_KEY[id];
  return key ? t(key) : id;
};

/** The supporting line of a block; an id without known support gets no line. */
export const blockHint = (t: TFn, id: string) => {
  const key = BLOCK_HINT_KEY[id];
  return key ? t(key) : "";
};

/** A block in the editor: enabled (included in the description) or not. The position in the list is the order. */
export type DraftBlock = { id: string; on: boolean };
export type DescriptionDraft = { media: DraftBlock[]; game: DraftBlock[]; tag: string; lang: string };

/**
 * The configuration that applies to the tracker: the profile's, or the default when the profile lacks the
 * section. A list omitted by the server falls back to the default (the server always sends everything, but the
 * Portal doesn't assume).
 */
export function effectiveConfig(profile: TrackerProfile | undefined, fallback: DescriptionConfig): DescriptionConfig {
  const d = profile?.description;
  if (!d) return fallback;
  return {
    media_blocks: d.media_blocks ?? fallback.media_blocks,
    game_blocks: d.game_blocks ?? fallback.game_blocks,
    media_info_tag: d.media_info_tag || fallback.media_info_tag,
    ...(d.language || fallback.language ? { language: d.language || fallback.language } : {}),
  };
}

/** The enabled blocks, in config order, followed by the disabled ones, in catalog order. */
function draftList(active: string[], universe: string[]): DraftBlock[] {
  const seen = new Set<string>();
  const on: DraftBlock[] = [];
  for (const id of active) {
    // id outside the catalog or duplicated: the server wouldn't return it, and the editor doesn't invent it.
    if (!universe.includes(id) || seen.has(id)) continue;
    seen.add(id);
    on.push({ id, on: true });
  }
  return [...on, ...universe.filter((id) => !seen.has(id)).map((id) => ({ id, on: false }))];
}

/** The editor draft from the effective config; the catalog (all blocks) comes from the server. */
export function toDraft(cfg: DescriptionConfig, catalog: DescriptionConfig): DescriptionDraft {
  return {
    media: draftList(cfg.media_blocks, catalog.media_blocks),
    game: draftList(cfg.game_blocks, catalog.game_blocks),
    tag: cfg.media_info_tag,
    lang: contentLanguage(cfg),
  };
}

/** The config that the draft represents: only the enabled blocks, in list order. */
export function fromDraft(d: DescriptionDraft): DescriptionConfig {
  return {
    media_blocks: d.media.filter((b) => b.on).map((b) => b.id),
    game_blocks: d.game.filter((b) => b.on).map((b) => b.id),
    media_info_tag: d.tag,
    ...(d.lang && d.lang !== "en" ? { language: d.lang } : {}), // English is the default: not written
  };
}

/** Enables or disables the block at position `index`, without changing the order. */
export function toggleBlock(list: DraftBlock[], index: number): DraftBlock[] {
  return list.map((b, i) => (i === index ? { ...b, on: !b.on } : b));
}

/** Swaps the block at position `index` with its neighbor (delta -1 = up, +1 = down). Out of bounds, it changes nothing. */
export function moveBlock(list: DraftBlock[], index: number, delta: -1 | 1): DraftBlock[] {
  const to = index + delta;
  if (index < 0 || index >= list.length || to < 0 || to >= list.length) return list;
  const out = [...list];
  [out[index], out[to]] = [out[to], out[index]];
  return out;
}

const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((x, i) => x === b[i]);

/** Do two configs compose the same way? The order of enabled blocks matters; the order of disabled ones does not. */
export function sameConfig(a: DescriptionConfig, b: DescriptionConfig): boolean {
  return (
    sameList(a.media_blocks, b.media_blocks) &&
    sameList(a.game_blocks, b.game_blocks) &&
    a.media_info_tag === b.media_info_tag &&
    contentLanguage(a) === contentLanguage(b)
  );
}

/** "Poster → Synopsis → Tags", or "no blocks" when the list is empty. */
export function describeBlocks(t: TFn, ids: string[]): string {
  return ids.length === 0 ? t("trackers.description.noBlocks") : ids.map((id) => blockLabel(t, id)).join(" → ");
}

/** The composition in one sentence per type, for the import preview and read-only viewing. */
export function describeConfig(t: TFn, cfg: DescriptionConfig): { kind: DescriptionKind; text: string }[] {
  return [
    { kind: "media", text: describeBlocks(t, cfg.media_blocks) },
    { kind: "game", text: describeBlocks(t, cfg.game_blocks) },
  ];
}
