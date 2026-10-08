import type { NodeConfigStatus, NodeSettingItem } from "../api";
import type { TFn } from "../i18n";

// Support for the "Node Configuration" section (ADR-039): pure text and rules, without React, to
// test without rendering.

export type ConfigStatusMeta = {
  label: string;
  // Explains the state in a sentence — goes in the badge title.
  hint: string;
  tone: "ok" | "warn" | "muted";
};

// The badge shows if the Agent is up to date with today's config. "unsupported" isn't an error: it's the
// pre-ADR-039 Agent, which still relies entirely on the local .env until it is updated (ADR-027).
export function configStatusMeta(t: TFn, status: NodeConfigStatus): ConfigStatusMeta {
  switch (status) {
    case "in_sync":
      return { label: t("nodes.nodeConfig.status.inSync.label"), hint: t("nodes.nodeConfig.status.inSync.hint"), tone: "ok" };
    case "pending":
      return {
        label: t("nodes.nodeConfig.status.pending.label"),
        hint: t("nodes.nodeConfig.status.pending.hint"),
        tone: "warn",
      };
    default:
      return {
        label: t("nodes.nodeConfig.status.unsupported.label"),
        hint: t("nodes.nodeConfig.status.unsupported.hint"),
        tone: "muted",
      };
  }
}

// Display text for a value: bool becomes On/Off; the rest takes the unit from the catalog.
export function fmtConfigValue(t: TFn, item: Pick<NodeSettingItem, "type" | "unit">, value: string): string {
  if (item.type === "bool") return value === "true" ? t("nodes.nodeConfig.value.on") : t("nodes.nodeConfig.value.off");
  return item.unit ? `${value} ${item.unit}` : value;
}

// LIGHT validation in the browser, just to give feedback before the round-trip: the Control Plane is what
// decides (same cluster value Spec, with range and options). Returns the message, or "" if
// there's nothing to complain about.
export function checkConfigValue(t: TFn, item: NodeSettingItem, raw: string): string {
  const v = raw.trim();
  if (v === "") return t("nodes.nodeConfig.validation.required");
  if (item.type === "int" || item.type === "int64") {
    if (!/^-?\d+$/.test(v)) return t("nodes.nodeConfig.validation.integer");
  } else if (item.type === "float") {
    if (Number.isNaN(Number(v))) return t("nodes.nodeConfig.validation.number");
  } else if (item.type === "duration") {
    if (!/^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/.test(v)) return t("nodes.nodeConfig.validation.duration");
    return ""; // duration range is checked by the Control Plane
  } else if (item.type === "bool") {
    return v === "true" || v === "false" ? "" : t("nodes.nodeConfig.validation.bool");
  } else if (item.options && item.options.length > 0) {
    return item.options.includes(v) ? "" : t("nodes.nodeConfig.validation.options", { values: item.options.join(", ") });
  }
  if (item.type === "int" || item.type === "int64" || item.type === "float") {
    const n = Number(v);
    if (item.min != null && n < item.min) return t("nodes.nodeConfig.validation.min", { min: item.min });
    if (item.max != null && n > item.max) return t("nodes.nodeConfig.validation.max", { max: item.max });
  }
  return "";
}
