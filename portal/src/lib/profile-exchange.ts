// Export/import of the tracker profile (ADR-037, #197). Pure functions: the component only
// renders and calls the API; the decision of "what the operator will see and what they can choose"
// stays here, where it can be tested.

import type { ImportCounts, ImportItem, ImportPlan, TaxonomyAxis } from "../api";
import type { TFn } from "../i18n";

/** File ceiling, the same as the server (MaxDocumentBytes): rejects before reading and sending. */
export const MAX_IMPORT_BYTES = 512 * 1024;

const FALLBACK_FILENAME = "seedgrid-profile.json";

/**
 * The file name from Content-Disposition. The server only emits [a-z0-9-] and the Portal
 * checks it again — the name goes to `a.download`, and an unexpected value must not become a path.
 */
export function filenameFromDisposition(header: string | null): string {
  const m = /filename="([A-Za-z0-9._-]{1,120})"/.exec(header ?? "");
  return m && !m[1].startsWith(".") ? m[1] : FALLBACK_FILENAME;
}

/** Checks the chosen file before reading. "" = can proceed; otherwise, the reason to show. */
export function checkImportFile(t: TFn, file: { name: string; size: number }): string {
  if (file.size === 0) return t("trackers.profileExchange.errors.emptyFile");
  if (file.size > MAX_IMPORT_BYTES) {
    return t("trackers.profileExchange.errors.fileTooLarge", {
      sizeKib: Math.ceil(file.size / 1024),
      maxKib: MAX_IMPORT_BYTES / 1024,
    });
  }
  return "";
}

/** Stable key of a term (the same one the server reads in ?overwrite=axis:id). */
export const overwriteKey = (axis: TaxonomyAxis, id: number) => `${axis}:${id}`;

/** The request to overwrite the description composition (#192): `?overwrite=description`. */
export const DESCRIPTION_KEY = "description";

/** The request to overwrite the image host (#193): `?overwrite=image_host`. */
export const IMAGE_HOST_KEY = "image_host";

/** The terms the import will keep because they belong to the operator or their site — the only ones the operator can request to overwrite. */
export function protectedItems(plan: ImportPlan, axes: TaxonomyAxis[]): ImportItem[] {
  return axes.flatMap((axis) => (plan.items[axis] ?? []).filter((it) => it.status === "protected"));
}

/**
 * The numbers of what will happen when applying, counting what the operator marked to
 * overwrite: each request takes a term out of "kept" and puts it into "renamed".
 */
export function effectiveCounts(plan: ImportPlan, overwrite: Set<string>): ImportCounts {
  // The request to overwrite the description and image host are not about terms:
  // they don't count towards the terms.
  const asked = Math.min(
    [...overwrite].filter((k) => k !== DESCRIPTION_KEY && k !== IMAGE_HOST_KEY).length,
    plan.counts.protected,
  );
  return {
    ...plan.counts,
    renamed: plan.counts.renamed + asked,
    protected: plan.counts.protected - asked,
  };
}

/**
 * Would the file description composition be applied? new and replaced yes; protected only if the
 * operator requested to overwrite; same, or a file without the section, no.
 */
export function descriptionApplies(plan: ImportPlan, overwrite: Set<string>): boolean {
  const d = plan.description;
  if (!d) return false;
  return d.status === "new" || d.status === "replaced" || (d.status === "protected" && overwrite.has(DESCRIPTION_KEY));
}

/**
 * Would the file image host be applied? new and replaced yes; protected only if the operator
 * requested to overwrite; same, or a file without the section, no.
 */
export function imageHostApplies(plan: ImportPlan, overwrite: Set<string>): boolean {
  const h = plan.image_host;
  if (!h) return false;
  return h.status === "new" || h.status === "replaced" || (h.status === "protected" && overwrite.has(IMAGE_HOST_KEY));
}

/** Would applying change anything? Without this the button stays disabled: there is nothing to confirm. */
export function hasChanges(plan: ImportPlan, overwrite: Set<string>): boolean {
  const c = effectiveCounts(plan, overwrite);
  return c.new + c.renamed + c.kind_set > 0 || descriptionApplies(plan, overwrite) || imageHostApplies(plan, overwrite);
}

/** "4 new, 1 renamed, 2 roles filled" — only what is not zero. */
export function describeChanges(t: TFn, c: ImportCounts): string {
  const parts: string[] = [];
  if (c.new > 0) parts.push(t("trackers.profileExchange.changes.new", { count: c.new }));
  if (c.renamed > 0) parts.push(t("trackers.profileExchange.changes.renamed", { count: c.renamed }));
  if (c.kind_set > 0) parts.push(t("trackers.profileExchange.changes.kindSet", { count: c.kind_set }));
  return parts.length > 0 ? parts.join(", ") : t("trackers.profileExchange.changes.none");
}

/** Why the import keeps this term: whose name it protects. */
export function protectedText(t: TFn, it: Pick<ImportItem, "current_name" | "current_origin">): string {
  const name = it.current_name ?? "";
  if (it.current_origin === "manual") return t("trackers.profileExchange.protected.manual", { name });
  if (it.current_origin === "discovered") return t("trackers.profileExchange.protected.discovered", { name });
  return t("trackers.profileExchange.protected.profile", { name });
}

/**
 * What the Portal says after applying (the plan is what the server EXECUTED). The composition of
 * the description and the image host enter the sentence when the file brought them: applied, swapped
 * on request, or kept.
 */
export function describeResult(t: TFn, plan: ImportPlan): string {
  const c = plan.counts;
  const desc = plan.description;
  const host = plan.image_host;
  const descApplied = desc?.status === "new" || desc?.status === "replaced";
  const hostApplied = host?.status === "new" || host?.status === "replaced";
  const termChanges = c.new + c.renamed + c.kind_set > 0;

  const extras: string[] = [];
  if (descApplied) extras.push(t("trackers.profileExchange.result.extras.description"));
  if (hostApplied) extras.push(t("trackers.profileExchange.result.extras.imageHost"));

  let base: string;
  if (termChanges) {
    const changesText = describeChanges(t, c);
    if (extras.length === 2) {
      base = t("trackers.profileExchange.result.importedTermsAndTwoExtras", {
        changes: changesText,
        extra1: extras[0],
        extra2: extras[1],
      });
    } else if (extras.length === 1) {
      base = t("trackers.profileExchange.result.importedTermsAndOneExtra", {
        changes: changesText,
        extra: extras[0],
      });
    } else {
      base = t("trackers.profileExchange.result.importedTermsOnly", { changes: changesText });
    }
  } else if (extras.length === 2) {
    base = t("trackers.profileExchange.result.importedTwoExtrasOnly", {
      extra1: extras[0],
      extra2: extras[1],
    });
  } else if (extras.length === 1) {
    base = t("trackers.profileExchange.result.importedOneExtraOnly", { extra: extras[0] });
  } else {
    base = t("trackers.profileExchange.result.noChanges");
  }

  let kept = "";
  if (c.protected > 0) {
    kept = " " + t("trackers.profileExchange.result.keptTerms", { count: c.protected });
  }

  const keptDesc = desc?.status === "protected" ? " " + t("trackers.profileExchange.result.keptDescription") : "";
  const keptHost = host?.status === "protected" ? " " + t("trackers.profileExchange.result.keptImageHost") : "";

  return base + kept + keptDesc + keptHost;
}

/** The file site label, for the preview header. */
export function siteLabel(site: { name: string; host?: string }): string {
  return site.host ? `${site.name} (${site.host})` : site.name;
}
