import { describe, expect, it } from "vitest";
import {
  chanMatch,
  isChannelCustom,
  isChannelRule,
  isGlobalRule,
  NOTIFY_CHANNELS,
  notifyLeftover,
} from "./notify-keys";

/**
 * The ACTUAL keys of the `notify` group, as returned by the Control Plane API:
 * 9 catalog literals (underscore separator) + 6 per channel generated at runtime
 * (dot separator), for 3 channels = 27.
 */
const LITERALS = [
  "notify.telegram_bot_token",
  "notify.telegram_chat_id",
  "notify.discord_webhook_url",
  "notify.webhook_url",
  "notify.alerts_min_severity",
  "notify.alerts_on_resolve",
  "notify.events",
  "notify.activities",
  "notify.rate_limit_per_min",
  "notify.language",
];
const BY_CHANNEL = NOTIFY_CHANNELS.flatMap((c) =>
  ["custom", "alerts_min_severity", "alerts_on_resolve", "events", "activities", "rate_limit_per_min"].map(
    (k) => `notify.${c}.${k}`,
  ),
);
const ALL_KEYS = [...LITERALS, ...BY_CHANNEL];

describe("chanMatch", () => {
  it("matches BOTH channel separators — this is bug #163", () => {
    // Matching only the underscore made the 18 generated keys fall into the "Other" card,
    // which turned into a single 18-line list and deformed the page.
    const tel = chanMatch("telegram");
    expect(tel("notify.telegram_bot_token")).toBe(true); // credential (underscore)
    expect(tel("notify.telegram.custom")).toBe(true); // rule (dot)
  });

  it("does not invade the neighboring channel", () => {
    expect(chanMatch("discord")("notify.telegram.custom")).toBe(false);
    expect(chanMatch("telegram")("notify.alerts_min_severity")).toBe(false);
  });
});

describe("isChannelRule / isChannelCustom", () => {
  it("the custom switch is not a rule", () => {
    // If `custom` were treated as a rule it would be hidden along with them, and the
    // operator would have no way to turn them back on.
    expect(isChannelCustom("notify.telegram.custom")).toBe(true);
    expect(isChannelRule("notify.telegram.custom")).toBe(false);
  });

  it("per-channel rule is the dot separator one", () => {
    expect(isChannelRule("notify.discord.events")).toBe(true);
    expect(isChannelRule("notify.discord_webhook_url")).toBe(false); // credential
    expect(isChannelRule("notify.events")).toBe(false); // global rule
  });

  it("credential is not a rule — it is what keeps the on/off badge correct", () => {
    // The badge is items.every(filled) over the credentials; including rules (which start
    // empty) would make a configured Telegram appear as "disabled".
    for (const k of ["notify.telegram_bot_token", "notify.telegram_chat_id", "notify.webhook_url"]) {
      expect(isChannelRule(k)).toBe(false);
      expect(isChannelCustom(k)).toBe(false);
    }
  });
});

describe("isGlobalRule", () => {
  it("matches global rules and no per-channel rules", () => {
    expect(isGlobalRule("notify.alerts_min_severity")).toBe(true);
    expect(isGlobalRule("notify.rate_limit_per_min")).toBe(true);
    expect(isGlobalRule("notify.language")).toBe(true);
    expect(isGlobalRule("notify.telegram.rate_limit_per_min")).toBe(false);
  });
});

describe("notifyLeftover", () => {
  it("no real key lands in the Others card", () => {
    // The DoD item for #163 that was left unwritten until there was a runner.
    // ⚠️ The ALL list is kept by hand: this catches MATCHER regressions, not the arrival
    // of a new key in the Control Plane catalog (which wouldn't be in the list).
    expect(notifyLeftover(ALL_KEYS)).toEqual([]);
  });

  it("the count matches what the API returns: 10 + 18 = 28", () => {
    expect(LITERALS).toHaveLength(10);
    expect(BY_CHANNEL).toHaveLength(18);
    expect(ALL_KEYS).toHaveLength(28);
  });

  it("each channel keeps its own, and global ones are left for Sending rules", () => {
    const byChannel = NOTIFY_CHANNELS.map((c) => ALL_KEYS.filter(chanMatch(c)).length);
    expect(byChannel).toEqual([8, 7, 7]); // telegram has 2 credentials; the others, 1
    expect(ALL_KEYS.filter(isGlobalRule)).toHaveLength(6);
    expect(byChannel.reduce((a, b) => a + b, 0) + 6).toBe(ALL_KEYS.length);
  });

  it("unknown key IS reported — the safety net must continue to exist", () => {
    // The "Other" card kept the imgbb (ADR-030) and Google Books keys from
    // disappearing from the screen. The test protects the grouping, it doesn't remove the network.
    expect(notifyLeftover([...ALL_KEYS, "notify.matrix_access_token"])).toEqual([
      "notify.matrix_access_token",
    ]);
  });
});
