import { describe, expect, it } from "vitest";
import { fmtBytes, fmtRatio } from "../components/ui";

/**
 * fmtBytes was capped at GB. In a multi-TB cluster it would show "5677.52 GB" — a number
 * the operator has to divide in their head. It wasn't a calculation error, the unit just
 * stopped keeping up with the library; that's why the test covers boundaries, not just the happy path.
 */
describe("fmtBytes", () => {
  it("uses TB above 1 TiB — it was the missing ceiling", () => {
    expect(fmtBytes(1024 ** 4)).toBe("1.00 TB");
    expect(fmtBytes(6_096_000_000_000)).toMatch(/TB$/);
  });

  it("no longer returns thousands of GB", () => {
    // 5,54 TiB: the actual value of the cluster that exposed the bug.
    const s = fmtBytes(6_090_000_000_000);
    expect(s).not.toMatch(/GB$/);
    expect(Number.parseFloat(s)).toBeLessThan(1024);
  });

  it("scales up to PB", () => {
    expect(fmtBytes(1024 ** 5)).toBe("1.00 PB");
  });

  it("keeps smaller units intact", () => {
    expect(fmtBytes(1024 ** 3)).toBe("1.00 GB");
    expect(fmtBytes(1024 ** 2)).toBe("1 MB");
    expect(fmtBytes(1024)).toBe("1 KB");
    expect(fmtBytes(512)).toBe("512 B");
    expect(fmtBytes(0)).toBe("0 B");
    expect(fmtBytes(undefined)).toBe("0 B");
  });

  it("never skips a unit at the boundary", () => {
    // One byte below the cutoff must stay in the lower unit — no gap in the scale.
    expect(fmtBytes(1024 ** 4 - 1)).toMatch(/GB$/);
    expect(fmtBytes(1024 ** 3 - 1)).toMatch(/MB$/);
  });
});

describe("fmtRatio", () => {
  it("with no traffic it shows a dash, not 0.000", () => {
    // The panel showed "0.000" for 438 stopped torrents; "—" is what the rest of the
    // Portal uses and it's honest: there is no ratio, it isn't a zero ratio.
    expect(fmtRatio(0, 0)).toBe("—");
  });

  it("seeded without downloading is infinite", () => {
    expect(fmtRatio(1000, 0)).toBe("∞");
  });

  it("calculates the ratio with 2 decimal places, like the rest of the Portal", () => {
    expect(fmtRatio(300, 100)).toBe("3.00");
    expect(fmtRatio(1, 3)).toBe("0.33");
  });
});
