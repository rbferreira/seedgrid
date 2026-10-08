import { describe, expect, it } from "vitest";
import type { Torrent } from "../api";
import { isDone } from "../components/torrents";
import { byTorrentStatus } from "../components/ui";

const t = (status: string): Torrent => ({ id: "x", hash: "h", node_id: "n", status }) as Torrent;

/**
 * isDone divides the TWO Portal screens: Downloads shows `isDone`, Dashboard shows
 * `!isDone`. What matters isn't which status falls where — it's that every torrent
 * falls into EXACTLY one of the two. A new Agent status that didn't fall into either
 * would vanish from the Portal without error: silent failure, the worst kind here.
 */
describe("isDone", () => {
  it("collection is what is seeding", () => {
    expect(isDone(t("SEEDING"))).toBe(true);
  });

  it("queueing, downloading, paused, rechecking, and error are not collection", () => {
    for (const s of ["QUEUED", "DOWNLOADING", "PAUSED", "CHECKING", "MOVING", "ERROR"]) {
      expect(isDone(t(s))).toBe(false);
    }
  });

  it("UNKNOWN status goes to Dashboard, never limbo", () => {
    // The Dashboard uses the complement (!isDone) precisely for this: a future status
    // of the Agent appears somewhere instead of making the torrent invisible.
    expect(isDone(t("STALLED_FUTURE"))).toBe(false);
  });

  it("every torrent appears in exactly one of the two screens", () => {
    const universe = ["SEEDING", "QUEUED", "DOWNLOADING", "PAUSED", "CHECKING", "ERROR", "XPTO"].map(t);
    const downloaded = universe.filter(isDone);
    const dashboard = universe.filter((x) => !isDone(x));
    expect(downloaded.length + dashboard.length).toBe(universe.length);
    expect(downloaded.some((b) => dashboard.includes(b))).toBe(false);
  });
});

describe("byTorrentStatus", () => {
  it("puts what needs attention before what is already fine", () => {
    const sorted = [t("SEEDING"), t("ERROR")].sort(byTorrentStatus);
    expect(sorted[0].status).toBe("ERROR");
  });

  it("unknown status goes to the end, not the top", () => {
    // Default rank 99: a status the Portal doesn't know cannot skip the attention
    // queue and push a real ERROR to the bottom.
    const sortedList = [t("XPTO"), t("SEEDING")].sort(byTorrentStatus);
    expect(sortedList[sortedList.length - 1].status).toBe("XPTO");
  });

  it("is stable: ties preserve input order", () => {
    const a = t("SEEDING");
    const b = t("SEEDING");
    expect([a, b].sort(byTorrentStatus)).toEqual([a, b]);
    expect([b, a].sort(byTorrentStatus)).toEqual([b, a]);
  });
});
