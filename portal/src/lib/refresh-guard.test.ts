import { describe, expect, it } from "vitest";
import { refreshGuard } from "./refresh-guard";

describe("refreshGuard", () => {
  it("applies the answer of the only refresh running", () => {
    const g = refreshGuard();
    const fresh = g.begin();
    expect(fresh()).toBe(true);
  });

  it("drops a refresh that started before a change (the race of #34)", () => {
    const g = refreshGuard();
    const timer = g.begin(); // the timer's refresh reads the old mode
    g.changed(); // the operator switches the mode
    const afterChange = g.begin(); // the refresh made after the switch
    expect(timer()).toBe(false);
    expect(afterChange()).toBe(true);
  });

  it("drops it even when it answers before the refresh made after the change starts", () => {
    const g = refreshGuard();
    const timer = g.begin();
    g.changed();
    // The old answer arrives while the change is still being saved.
    expect(timer()).toBe(false);
  });

  it("drops an older refresh that answers after a newer one", () => {
    const g = refreshGuard();
    const older = g.begin();
    const newer = g.begin();
    expect(newer()).toBe(true);
    expect(older()).toBe(false);
  });
});
