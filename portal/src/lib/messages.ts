import { shortId, fmtBytes } from "../components/ui";
import { getLang, isPlural, lookup, render, type Params } from "../i18n";
import { settingText } from "./setting-text";

// The phrase for Audit alerts, events, activities and actions, built in the reader's language from the code
// and the payload that the Control Plane records (ADR-041 §3). The recorded `message` is the fallback: it applies
// to a code that the Portal doesn't know yet and to an old record whose payload doesn't
// have the data the phrase requests. The contract — codes, payload keys and variants — is
// in control-plane/internal/messages and in shared/messages.json; messages-contract.test.ts
// checks the catalogs against it.

export type MessageKind = "alert" | "event" | "activity" | "error" | "audit";

// Where the phrase for each trail lives in the catalog.
export function catalogPath(kind: MessageKind, code: string): string {
  return kind === "error" ? `api.errors.${code}` : `alerts.messages.${kind}.${code}`;
}
export type Payload = Record<string, unknown> | null | undefined;

// VARIANT: the payload key whose value selects the phrase, when the same code describes
// different situations. Mirror of the record; the contract test fails on divergence.
export const VARIANT: Record<string, string> = {
  "event/NODE_ADOPTED_MANUAL": "gate_moved",
  "event/NODE_REGISTERED": "name_dropped",
  "event/STORAGE_RECOVERY": "step",
  "event/TORRENT_CREATED": "origin",
  "event/TORRENT_MIGRATION_FAILED": "cancelled",
  "event/TORRENT_REMOVED": "reason",
  "activity/admission_hold": "reason_code",
  "activity/scheduled": "reason_code",
  "activity/screenshot_host_failed": "reason_code",
  "audit/cluster_mode_changed": "mode",
  "audit/node_mode_changed": "mode",
  "audit/node_override_cleared": "had_before",
  "audit/node_override_set": "had_before",
  "audit/node_renamed": "change",
  "audit/setting_changed": "value_kind",
};

// DERIVED: parameters that the phrases use and are not a raw payload key, with the
// keys they come from (just one of them in the payload is enough). The contract test accepts these names.
export const DERIVED: Record<string, string[]> = {
  torrent: ["name", "hash"], // the name, or the short hash when there is no name
  shortfall: ["min_free_bytes", "projected_free_bytes"],
  need: ["need_bytes"],
  min_free: ["min_free_bytes"],
  best_free: ["best_free_bytes"],
  setting: ["key"], // the setting label in the reader's language (Audit records it in PT)
  sections: ["description", "image_host"], // what the profile import did with the optional sections
};

// Parameters that can be empty without the phrase losing meaning: the phrase won't fall back
// because of them.
const OPTIONAL = new Set(["sections"]);

// importSections: the part of the profile import phrase with the result of each optional section
// that the file brought in — empty when it brought none.
function importSections(payload: Record<string, unknown>): string {
  const lang = getLang();
  return (["description", "image_host"] as const)
    .map((section) => {
      const status = payload[section];
      const leaf = typeof status === "string" && status ? lookup(lang, `audit.importSections.${section}.${status}`) : undefined;
      return typeof leaf === "string" ? leaf : "";
    })
    .join("");
}

// What the phrase receives: the simple payload values, formatted where needed, plus the
// derived ones and what comes from outside it (the torrent name of an activity).
function paramsOf(payload: Record<string, unknown>, extra: Params): Params {
  const p: Params = {};
  for (const [k, v] of Object.entries(payload)) {
    if (typeof v === "string" || typeof v === "number") p[k] = v;
    else if (typeof v === "boolean") p[k] = String(v);
    else if (Array.isArray(v)) p[k] = v.join(", ");
    else if (v && typeof v === "object") {
      p[k] = Object.entries(v as Record<string, unknown>)
        .map(([kk, vv]) => `${kk}=${String(vv)}`)
        .join(", ");
    }
  }
  if (typeof payload.util_pct === "number") p.util_pct = Math.round(payload.util_pct);
  const name = typeof payload.name === "string" ? payload.name : "";
  const hash = typeof payload.hash === "string" ? payload.hash : "";
  if (name || hash) p.torrent = name || shortId(hash);
  if (typeof payload.min_free_bytes === "number" && typeof payload.projected_free_bytes === "number") {
    p.shortfall = fmtBytes(payload.min_free_bytes - payload.projected_free_bytes);
  }
  if (typeof payload.need_bytes === "number") p.need = fmtBytes(payload.need_bytes);
  if (typeof payload.min_free_bytes === "number") p.min_free = fmtBytes(payload.min_free_bytes);
  if (typeof payload.best_free_bytes === "number") p.best_free = fmtBytes(payload.best_free_bytes);
  if (typeof payload.key === "string" && payload.key) {
    const label = typeof payload.label === "string" && payload.label ? payload.label : payload.key;
    p.setting = settingText(payload.key, "label", label);
  }
  if ("description" in payload || "image_host" in payload) p.sections = importSections(payload);
  return { ...p, ...extra };
}

function paramNames(text: string): string[] {
  return [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]);
}

/**
 * messageText returns the phrase for the code in the active language. `fallback` is the `message` recorded by the
 * Control Plane; `extra` brings data that isn't in the payload (the torrent name of the activity).
 */
export function messageText(
  kind: MessageKind,
  code: string,
  payload: Payload,
  fallback: string | undefined,
  extra: Params = {},
): string {
  const reserve = fallback || code;
  const lang = getLang();
  const data = payload ?? {};
  let leaf = lookup(lang, catalogPath(kind, code));
  if (leaf && typeof leaf === "object" && !isPlural(leaf)) {
    const key = VARIANT[`${kind}/${code}`];
    leaf = key === undefined ? undefined : (leaf as Record<string, unknown>)[String(data[key])];
  }
  if (typeof leaf !== "string" && !isPlural(leaf)) return reserve;
  const params = paramsOf(data, extra);
  const texts = typeof leaf === "string" ? [leaf] : Object.values(leaf);
  // Old record without the data the phrase requests: the fallback, not a phrase with a {hole}.
  if (texts.some((s) => paramNames(s).some((n) => !(n in params) || (params[n] === "" && !OPTIONAL.has(n))))) {
    return reserve;
  }
  return render(lang, leaf, params);
}
