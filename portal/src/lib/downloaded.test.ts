import { describe, expect, it } from "vitest";
import type { Torrent } from "../api";
import { cmpName, filterTorrents, ratioValue, sortTorrents } from "./downloaded";

// Minimal Torrent: only the fields the Downloaded logic looks at. The cast exists because the
// API type is wide and filling it entirely would hide what each test actually varies.
const t = (over: Partial<Torrent>): Torrent =>
  ({ id: "x", hash: "h", node_id: "n1", name: "T", status: "SEEDING", ...over }) as Torrent;

const names = (ts: Torrent[]) => ts.map((x) => x.name);
const withoutNode = (id: string) => id; // trivial nodeName for tests that don't sort by Node

describe("ratioValue", () => {
  it("is upload/download when there was download", () => {
    expect(ratioValue(t({ uploaded_total: 300, downloaded_total: 100 }))).toBe(3);
  });

  it("seeded without downloading becomes the largest finite value, not Infinity", () => {
    // fmtRatio shows "∞"; in a naive comparator Infinity - Infinity = NaN and the order
    // becomes undefined, shuffling the rows on every refresh.
    const v = ratioValue(t({ uploaded_total: 500, downloaded_total: 0 }));
    expect(Number.isFinite(v)).toBe(true);
    expect(v).toBe(Number.MAX_SAFE_INTEGER);
  });

  it("with no traffic it stays below any real ratio", () => {
    expect(ratioValue(t({ uploaded_total: 0, downloaded_total: 0 }))).toBe(-1);
    expect(ratioValue(t({}))).toBe(-1); // missing fields
  });

  it("two torrents seeded without downloading do not produce NaN in subtraction", () => {
    const a = ratioValue(t({ uploaded_total: 10, downloaded_total: 0 }));
    const b = ratioValue(t({ uploaded_total: 90, downloaded_total: 0 }));
    expect(Number.isNaN(a - b)).toBe(false);
  });
});

describe("cmpName", () => {
  it("sorts S2 before S10 (numeric), not alphabetically", () => {
    const ts = [t({ name: "Series.S10E01" }), t({ name: "Series.S2E01" })].sort(cmpName);
    expect(names(ts)).toEqual(["Series.S2E01", "Series.S10E01"]);
  });

  it("ignores case and accents", () => {
    expect(cmpName(t({ name: "action" }), t({ name: "ACTION" }))).toBe(0);
  });

  it("falls back to the hash when there is no name", () => {
    const withoutName = t({ name: undefined, hash: "aaa" });
    const withName = t({ name: "zzz" });
    expect(cmpName(withoutName, withName)).toBeLessThan(0);
  });
});

describe("filterTorrents", () => {
  const collection = [
    t({ name: "Alpha", node_id: "n1", status: "SEEDING", size_bytes: 5_000 }),
    t({ name: "Beta", node_id: "n2", status: "SEEDING", size_bytes: 50_000 }),
    t({ name: undefined, hash: "cafe", node_id: "n2", status: "PAUSED" }),
  ];
  const empty = { q: "", node: "", status: "", minSize: 0 };

  it("without filter returns everything", () => {
    expect(filterTorrents(collection, empty)).toHaveLength(3);
  });

  it("search matches name case-insensitively", () => {
    expect(names(filterTorrents(collection, { ...empty, q: "alp" }))).toEqual(["Alpha"]);
  });

  it("search matches the hash of a torrent not yet named", () => {
    // This is what the operator sees on screen when there is no name; not finding it
    // would mean having no way to reach the row.
    expect(filterTorrents(collection, { ...empty, q: "cafe" })).toHaveLength(1);
  });

  it("filters by node and status", () => {
    expect(filterTorrents(collection, { ...empty, node: "n2" })).toHaveLength(2);
    expect(names(filterTorrents(collection, { ...empty, status: "PAUSED" }))).toEqual([undefined]);
  });

  it("torrent without size_bytes is excluded when there is a size filter", () => {
    // We can't assert it meets the minimum; letting it pass would be lying.
    const r = filterTorrents(collection, { ...empty, minSize: 10_000 });
    expect(names(r)).toEqual(["Beta"]);
  });

  it("whitespace in the search does not filter anything", () => {
    expect(filterTorrents(collection, { ...empty, q: "   " })).toHaveLength(3);
  });
});

describe("sortTorrents", () => {
  it("does not mutate the input array (comes from React state)", () => {
    const orig = [t({ name: "B" }), t({ name: "A" })];
    const copy = [...orig];
    sortTorrents(orig, { key: "name", dir: "asc" }, withoutNode);
    expect(orig).toEqual(copy);
  });

  it("ALWAYS breaks ties by name — that is what prevents rows from jumping on each poll", () => {
    // Dozens of seeders with upload 0 tie on the column; without a stable tiebreaker the order
    // changes every 10s cycle and the screen "flickers".
    const ts = [
      t({ name: "Charlie", uploaded_total: 0 }),
      t({ name: "Alpha", uploaded_total: 0 }),
      t({ name: "Bravo", uploaded_total: 0 }),
    ];
    const um = names(sortTorrents(ts, { key: "uploaded", dir: "desc" }, withoutNode));
    const reversedNames = names(sortTorrents([...ts].reverse(), { key: "uploaded", dir: "desc" }, withoutNode));
    expect(um).toEqual(["Alpha", "Bravo", "Charlie"]);
    expect(um).toEqual(reversedNames); // same input in a different order → same output
  });

  it("tie break by name does NOT reverse with the direction", () => {
    // The tiebreaker is a stability criterion, not a secondary sort column.
    const ts = [t({ name: "Bravo", size_bytes: 10 }), t({ name: "Alpha", size_bytes: 10 })];
    expect(names(sortTorrents(ts, { key: "size", dir: "desc" }, withoutNode))).toEqual(["Alpha", "Bravo"]);
    expect(names(sortTorrents(ts, { key: "size", dir: "asc" }, withoutNode))).toEqual(["Alpha", "Bravo"]);
  });

  it("sorts by ratio without shuffling when there are infinities", () => {
    const ts = [
      t({ name: "noTraffic", uploaded_total: 0, downloaded_total: 0 }),
      t({ name: "infinityA", uploaded_total: 10, downloaded_total: 0 }),
      t({ name: "real", uploaded_total: 200, downloaded_total: 100 }),
      t({ name: "infinityB", uploaded_total: 90, downloaded_total: 0 }),
    ];
    const r = names(sortTorrents(ts, { key: "ratio", dir: "desc" }, withoutNode));
    expect(r[r.length - 1]).toBe("noTraffic"); // no traffic goes to the end
    expect(r.slice(0, 2).sort()).toEqual(["infinityA", "infinityB"]); // infinities at the top
    expect(r).toContain("real");
  });

  it("sorts by Node using the label, not the id", () => {
    const labelOf = (id: string) => ({ n1: "zulu", n2: "alfa" })[id] ?? id;
    const ts = [t({ name: "A", node_id: "n1" }), t({ name: "B", node_id: "n2" })];
    expect(names(sortTorrents(ts, { key: "node", dir: "asc" }, labelOf))).toEqual(["B", "A"]);
  });

  it("treats missing numeric fields as 0", () => {
    const ts = [t({ name: "with", size_bytes: 100 }), t({ name: "without" })];
    expect(names(sortTorrents(ts, { key: "size", dir: "desc" }, withoutNode))).toEqual(["with", "without"]);
  });
});
