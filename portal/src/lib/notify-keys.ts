/**
 * Matchers for the notification keys (ADR-028).
 *
 * They live off-screen because they were the source of a bug that went unnoticed (#163):
 * the keys for a channel use TWO separators, and matching only one of them caused 18 tunables
 * to fall into the "Others" card, which became a single list and deformed the page.
 *
 *   credential -> notify.telegram_bot_token   (underscore, literal catalog)
 *   rule       -> notify.telegram.custom      (dot, GENERATED at runtime per channel)
 */

/** Notification channels with their own card on the Settings screen. */
export const NOTIFY_CHANNELS = ["telegram", "discord", "webhook"] as const;

/** chanMatch: matches ALL keys of a channel — credentials and rules. */
export const chanMatch = (name: string) => (k: string) =>
  k.startsWith(`notify.${name}_`) || k.startsWith(`notify.${name}.`);

/** Rule PER CHANNEL (dot separator). The `custom` is the switch, not a rule. */
export const isChannelRule = (k: string) => /^notify\.[a-z]+\./.test(k) && !k.endsWith(".custom");

/** The switch that decides if the channel's own rules apply (notify.filtersFor). */
export const isChannelCustom = (k: string) => /^notify\.[a-z]+\.custom$/.test(k);

/** GLOBAL sending rules — those that apply to every channel without `custom` enabled. */
export const isGlobalRule = (k: string) =>
  k.startsWith("notify.alerts_") ||
  k === "notify.events" ||
  k === "notify.activities" ||
  k === "notify.rate_limit_per_min" ||
  k === "notify.language";

/**
 * notifyLeftover returns the keys that don't match any card — what would fall into
 * "Others". It exists to be asserted in tests.
 *
 * ⚠️ Scope: the test passes a list of keys WRITTEN BY HAND, mirroring the catalog.
 * It catches matcher regressions (breaking chanMatch fails the test), but DOES NOT catch
 * alone a new notify.* key created in the Control Plane catalog — it wouldn't be
 * in the list. Covering this would require reading the real API keys, which is an integration
 * test. Keeping the list up to date is the responsibility of whoever tweaks the catalog.
 */
export function notifyLeftover(keys: string[]): string[] {
  const owners = [...NOTIFY_CHANNELS.map(chanMatch), isGlobalRule];
  return keys.filter((k) => !owners.some((m) => m(k)));
}
