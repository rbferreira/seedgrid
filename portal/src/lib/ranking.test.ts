import { describe, expect, it } from "vitest";
import { byWorstRatio, ratioOf, topBy } from "./ranking";

describe("ratioOf", () => {
  it("no download means no ratio — not zero ratio", () => {
    // It's the #173 bug: the Control Plane leaves `ratio` at 0 in this case, and sorting by it
    // put the 438 stopped ones at the top of "worst ratio".
    expect(ratioOf({ uploaded_total: 0, downloaded_total: 0 })).toBeNull();
  });

  it("seeded without downloading is infinite, not null", () => {
    expect(ratioOf({ uploaded_total: 500, downloaded_total: 0 })).toBe(Infinity);
  });

  it("calculates ratio when there was a download", () => {
    expect(ratioOf({ uploaded_total: 300, downloaded_total: 100 })).toBe(3);
    expect(ratioOf({ uploaded_total: 0, downloaded_total: 100 })).toBe(0);
  });

  it("missing field counts as zero", () => {
    expect(ratioOf({})).toBeNull();
  });
});

describe("byWorstRatio", () => {
  const name = (r: ReturnType<typeof mk>) => `${r.uploaded_total}/${r.downloaded_total}`;
  function mk(up: number, dl: number) {
    return { uploaded_total: up, downloaded_total: dl };
  }

  it("sorts from worst to best", () => {
    const l = [mk(300, 100), mk(10, 100), mk(150, 100)].sort(byWorstRatio);
    expect(l.map(name)).toEqual(["10/100", "150/100", "300/100"]);
  });

  it("those without ratio go to the END, not the top", () => {
    const l = [mk(0, 0), mk(10, 100), mk(0, 0), mk(1, 100)].sort(byWorstRatio);
    // The two real worst first; those with no download after.
    expect(l.slice(0, 2).map(name)).toEqual(["1/100", "10/100"]);
    expect(l.slice(2).every((t) => t.downloaded_total === 0)).toBe(true);
  });

  it("infinite ratio is the best, and comes before those without ratio", () => {
    const l = [mk(0, 0), mk(500, 0), mk(10, 100)].sort(byWorstRatio);
    expect(l.map(name)).toEqual(["10/100", "500/0", "0/0"]);
  });

  it("list of only no-ratio torrents doesn't break sorting", () => {
    expect([mk(0, 0), mk(0, 0)].sort(byWorstRatio)).toHaveLength(2);
  });
});

describe("topBy", () => {
  const t = (id: string, up: number, dl: number) => ({ id, uploaded_total: up, downloaded_total: dl });

  it("sorts from highest to lowest and respects the limit", () => {
    const l = topBy([t("a", 100, 100), t("b", 900, 100), t("c", 300, 100)], ratioOf, 2);
    expect(l.map((x) => x.id)).toEqual(["b", "c"]);
  });

  it("excludes those who seeded without downloading — infinite ratio would top the list without providing any info", () => {
    const l = topBy([t("infinite", 999, 0), t("real", 300, 100)], ratioOf, 10);
    expect(l.map((x) => x.id)).toEqual(["real"]);
  });

  it("excludes those without the metric", () => {
    const l = topBy([t("without", 0, 0), t("zero", 0, 100), t("real", 50, 100)], ratioOf, 10);
    expect(l.map((x) => x.id)).toEqual(["real"]);
  });

  it("empty list returns empty list", () => {
    expect(topBy([], ratioOf, 10)).toEqual([]);
  });

  it("also works for byte metrics", () => {
    const b = [{ n: 5 }, { n: 0 }, { n: 90 }];
    expect(topBy(b, (x) => x.n, 10).map((x) => x.n)).toEqual([90, 5]);
  });
});
