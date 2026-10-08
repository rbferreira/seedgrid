import { getLang, lookup, render } from "../i18n";

// Label, description, choices and category of the settings in the reader's language (ADR-041 §4). The
// Control Plane sends this text in Portuguese; the Portal catalog has the translation by the setting
// key (the contract is shared/settings.json, checked by settings-contract.test.ts). Without
// an entry in the catalog — a new setting in a Control Plane newer than the Portal — the
// text that came from the API is used.

// The settings generated per channel (notify.<channel>.<suffix>) use a single template, with {channel}.
const CHANNELS: Record<string, string> = { telegram: "Telegram", discord: "Discord", webhook: "Webhook" };
const CHANNEL_KEY = /^notify\.([a-z]+)\.([a-z_]+)$/;

function text(path: string, fallback: string, params?: Record<string, string>): string {
  const lang = getLang();
  const leaf = lookup(lang, path);
  return typeof leaf === "string" ? render(lang, leaf, params) : fallback;
}

export function settingText(key: string, field: "label" | "description", fallback: string): string {
  const m = CHANNEL_KEY.exec(key);
  if (m && CHANNELS[m[1]]) {
    return text(`settings.channelSpecs.${m[2]}.${field}`, fallback, { channel: CHANNELS[m[1]] });
  }
  return text(`settings.specs.${key}.${field}`, fallback);
}

// choiceLabel: the label of an option in the tracking settings (events or activities to notify).
export function choiceLabel(settingKey: string, value: string, fallback: string): string {
  const kind = settingKey.endsWith(".events") ? "event" : settingKey.endsWith(".activities") ? "activity" : "";
  return kind ? text(`settings.choices.${kind}.${value}`, fallback) : fallback;
}

export function categoryName(category: string, fallback: string): string {
  return text(`settings.categoryNames.${category}`, fallback);
}
