import { describe, expect, it } from "vitest";
import type { Torrent } from "../api";
import { translate, type TFn } from "../i18n";
import { addedValue, cmpName, filterTorrents, fmtAdded, ratioValue, sortTorrents } from "./downloaded";

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
  const empty = { q: "", node: "", status: "", minSize: 0, addedSince: 0 };

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

  it("filters by when it was added, boundary included", () => {
    const ts = [
      t({ name: "old", created_at: "2026-10-01T00:00:00Z" }),
      t({ name: "onTheEdge", created_at: "2026-10-10T00:00:00Z" }),
      t({ name: "fresh", created_at: "2026-10-12T08:00:00Z" }),
    ];
    const since = Date.parse("2026-10-10T00:00:00Z");
    expect(names(filterTorrents(ts, { ...empty, addedSince: since }))).toEqual(["onTheEdge", "fresh"]);
  });

  it("a torrent with no added date drops out under the period filter", () => {
    // Same rule as size: without the data you can't claim it is recent.
    const ts = [t({ name: "noDate" }), t({ name: "garbage", created_at: "not a date" })];
    expect(filterTorrents(ts, { ...empty, addedSince: 1 })).toHaveLength(0);
    expect(filterTorrents(ts, empty)).toHaveLength(2);
  });
});

describe("addedValue / fmtAdded", () => {
  const en: TFn = (key, params) => translate("en", key, params);
  const ptBR: TFn = (key, params) => translate("pt-BR", key, params);
  const now = Date.parse("2026-10-13T12:00:00Z");
  const ago = (ms: number) => t({ created_at: new Date(now - ms).toISOString() });
  const MINUTE = 60_000;
  const HOUR = 60 * MINUTE;
  const DAY = 24 * HOUR;

  it("a missing or invalid date is worth 0 and renders as a dash", () => {
    expect(addedValue(t({}))).toBe(0);
    expect(addedValue(t({ created_at: "not a date" }))).toBe(0);
    expect(fmtAdded(en, t({}), now)).toBe("—");
  });

  it("is relative up to 30 days, in the largest whole unit", () => {
    expect(fmtAdded(en, ago(20_000), now)).toBe("just now");
    expect(fmtAdded(en, ago(59 * MINUTE), now)).toBe("59 min ago");
    expect(fmtAdded(en, ago(HOUR), now)).toBe("1 h ago");
    expect(fmtAdded(en, ago(23 * HOUR + 59 * MINUTE), now)).toBe("23 h ago");
    expect(fmtAdded(en, ago(DAY), now)).toBe("1 d ago");
    expect(fmtAdded(en, ago(29 * DAY), now)).toBe("29 d ago");
  });

  it("follows the Portal language", () => {
    expect(fmtAdded(ptBR, ago(20_000), now)).toBe("agora");
    expect(fmtAdded(ptBR, ago(3 * HOUR), now)).toBe("há 3 h");
  });

  it("past 30 days shows the date, not a day count", () => {
    const old = t({ created_at: "2026-03-05T15:00:00Z" });
    expect(fmtAdded(en, old, now)).toMatch(/2026|26/);
    expect(fmtAdded(en, old, now)).not.toMatch(/ago/);
  });

  it("a browser clock behind the server does not produce a negative age", () => {
    expect(fmtAdded(en, ago(-5 * MINUTE), now)).toBe("just now");
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

  it("sorts by added: newest first, no date last", () => {
    const ts = [
      t({ name: "noDate" }),
      t({ name: "yesterday", created_at: "2026-10-12T10:00:00Z" }),
      t({ name: "today", created_at: "2026-10-13T10:00:00Z" }),
      t({ name: "lastMonth", created_at: "2026-09-13T10:00:00Z" }),
    ];
    expect(names(sortTorrents(ts, { key: "added", dir: "desc" }, withoutNode))).toEqual([
      "today",
      "yesterday",
      "lastMonth",
      "noDate",
    ]);
  });

  it("treats missing numeric fields as 0", () => {
    const ts = [t({ name: "with", size_bytes: 100 }), t({ name: "without" })];
    expect(names(sortTorrents(ts, { key: "size", dir: "desc" }, withoutNode))).toEqual(["with", "without"]);
  });
});
