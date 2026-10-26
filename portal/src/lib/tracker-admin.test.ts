import { describe, expect, it } from "vitest";
import { canManageTrackers } from "./tracker-admin";

const holding = (...perms: string[]) => (perm: string) => perms.includes(perm);

describe("canManageTrackers", () => {
  it("needs both Settings and trackers:write", () => {
    expect(canManageTrackers(holding("settings:write", "trackers:write"))).toBe(true);
  });

  it("the Publisher (trackers:read and seed) cannot", () => {
    expect(canManageTrackers(holding("trackers:read", "trackers:seed", "publish:write"))).toBe(false);
  });

  it("one without the other is not enough", () => {
    expect(canManageTrackers(holding("trackers:write"))).toBe(false);
    expect(canManageTrackers(holding("settings:write"))).toBe(false);
  });
});
