// Tracker profile taxonomy (ADR-037, #196). Pure functions: the publishing panel and the
// profile editor read the site profile options — previously they were hardcoded in
// TrackerUploadPanel.tsx (UNIT3D_CATEGORIES/TYPES/RESOLUTIONS), with the IDs of one site
// as if they were for every UNIT3D site.

import type {
  ApplyItem,
  ProposalItem,
  TaxonomyAxis,
  TaxonomyTerm,
  TermKind,
  TrackerProfile,
} from "../api";
import type { Key } from "../i18n";

export const AXES: { axis: TaxonomyAxis; labelKey: Key }[] = [
  { axis: "category", labelKey: "publish.taxonomy.axes.category" },
  { axis: "type", labelKey: "publish.taxonomy.axes.type" },
  { axis: "resolution", labelKey: "publish.taxonomy.axes.resolution" },
];

export const KIND_LABEL_KEY: Record<TermKind, Key> = {
  movie: "publish.taxonomy.kinds.movie",
  tv: "publish.taxonomy.kinds.tv",
  game: "publish.taxonomy.kinds.game",
};

/** Terms of an axis, in profile order (by id). Missing profile = empty list. */
export function termsOf(profile: TrackerProfile | undefined, axis: TaxonomyAxis): TaxonomyTerm[] {
  return profile?.taxonomy?.[axis] ?? [];
}

/** Options for a <select>: the site id and the label. This is what the panel renders. */
export function termOptions(profile: TrackerProfile | undefined, axis: TaxonomyAxis): { id: number; label: string }[] {
  return termsOf(profile, axis).map((t) => ({ id: t.id, label: t.name }));
}

/**
 * Does the profile have the minimum required to publish? Category and type are mandatory in UNIT3D
 * (resolution is optional), so without them the panel has nothing to offer.
 */
export function canPublish(profile: TrackerProfile | undefined): boolean {
  return termsOf(profile, "category").length > 0 && termsOf(profile, "type").length > 0;
}

/** The role of the chosen category (tv, game...), or undefined if it has no special role. */
export function kindOfCategory(profile: TrackerProfile | undefined, categoryId: number): TermKind | undefined {
  return termsOf(profile, "category").find((t) => t.id === categoryId)?.kind;
}

/** Normalizes a label for comparison: lowercase, no accents, collapsed spaces. */
export function normalizeLabel(s: string): string {
  return s
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * Finds the term whose name is the given label (ignoring case and accents). Only returns when the label
 * is UNEQUIVOCAL: if more than one term in the axis has this name (two ids for "Series"), there is no way
 * to know which one the operator wants, and picking the first would let a term — for example one imported
 * from a file — obscure the correct one without anyone deciding. Without pre-filling, the operator
 * chooses.
 *
 * The server uses the same normalization (`NormalizeLabel` in trackerprofile) to reject duplicate names
 * in an imported file and to warn when a new term would start sharing the name
 * with another — change both together.
 */
export function findByLabel(terms: TaxonomyTerm[], label: string): TaxonomyTerm | undefined {
  const want = normalizeLabel(label);
  if (!want) return undefined;
  const hits = terms.filter((t) => normalizeLabel(t.name) === want);
  return hits.length === 1 ? hits[0] : undefined;
}

/** The category the panel picks initially: the first "movie"; otherwise the first available. */
export function defaultCategory(profile: TrackerProfile | undefined): number {
  const terms = termsOf(profile, "category");
  return (terms.find((t) => t.kind === "movie") ?? terms[0])?.id ?? 0;
}

export type Prefill = { category_id: number; type_id: number; resolution_id: number };

/**
 * Translates what the Control Plane parser inferred from the release name to the IDs of THIS site.
 *
 * The parser returns only labels, never numbers: each site numbers things its own way, and using the number
 * from one site on another would silently choose a valid but wrong option ("1080p" is 3 on one
 * site and 2 on another). Therefore, the bridge is the LABEL ("1080p", "WEB-DL") and the ROLE of the category
 * (series or movie), which apply to any site. 0 = could not decide; the operator chooses.
 */
export function prefillFromParsed(
  profile: TrackerProfile | undefined,
  parsed: { is_tv: boolean; type: string; resolution: string },
): Prefill {
  const cats = termsOf(profile, "category");
  const wantKind: TermKind = parsed.is_tv ? "tv" : "movie";
  return {
    category_id: cats.find((t) => t.kind === wantKind)?.id ?? 0,
    type_id: findByLabel(termsOf(profile, "type"), parsed.type)?.id ?? 0,
    resolution_id: findByLabel(termsOf(profile, "resolution"), parsed.resolution)?.id ?? 0,
  };
}

// ---- Discovery proposal -------------------------------------------------

/** Stable key for a proposal row. */
export const proposalKey = (axis: TaxonomyAxis, id: number) => `${axis}:${id}`;

/**
 * What is checked by default: only what is NEW. Renaming is opt-in — changing the label the
 * operator sees is their decision, and what was adjusted manually (protected) cannot even be checked.
 */
export function defaultSelection(proposal: Record<TaxonomyAxis, ProposalItem[]>): Set<string> {
  const sel = new Set<string>();
  for (const { axis } of AXES) {
    for (const it of proposal[axis] ?? []) if (it.status === "new") sel.add(proposalKey(axis, it.id));
  }
  return sel;
}

/** Can a proposal row be applied? "Already in profile" and "protected" would change nothing. */
export function isApplicable(it: ProposalItem): boolean {
  return it.status === "new" || (it.status === "renamed" && !it.protected);
}

/**
 * Builds the apply items from what the operator checked. If they EDITED the name in the
 * proposal, the item goes as "manual" — it's their name, not the site's — and becomes
 * protected from future discoveries. Without editing, it goes as "discovered".
 */
export function buildApplyItems(
  proposal: Record<TaxonomyAxis, ProposalItem[]>,
  selected: Set<string>,
  edited: Record<string, string>,
): ApplyItem[] {
  const out: ApplyItem[] = [];
  for (const { axis } of AXES) {
    for (const it of proposal[axis] ?? []) {
      const key = proposalKey(axis, it.id);
      if (!selected.has(key) || !isApplicable(it)) continue;
      const name = (edited[key] ?? it.name).trim();
      out.push({ axis, id: it.id, name, origin: name !== it.name ? "manual" : "discovered" });
    }
  }
  return out;
}

export function proposalCounts(proposal: Record<TaxonomyAxis, ProposalItem[]>): {
  new: number;
  same: number;
  renamed: number;
} {
  const c = { new: 0, same: 0, renamed: 0 };
  for (const { axis } of AXES) for (const it of proposal[axis] ?? []) c[it.status]++;
  return c;
}

export const STOP_REASON_KEY: Record<string, Key> = {
  end_of_catalog: "publish.taxonomy.stopReasons.endOfCatalog",
  max_pages: "publish.taxonomy.stopReasons.maxPages",
  rate_limited: "publish.taxonomy.stopReasons.rateLimited",
  time_budget: "publish.taxonomy.stopReasons.timeBudget",
  error: "publish.taxonomy.stopReasons.error",
};
