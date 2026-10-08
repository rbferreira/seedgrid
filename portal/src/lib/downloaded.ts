import { getLang } from "../i18n";
import type { Torrent } from "../api";

/**
 * Pure logic for the Downloaded screen: filtering and sorting of the collection.
 *
 * Lives outside the component because this is where silent bugs hide — order that
 * shuffles on every poll, torrent that vanishes from the filter — and none of them need
 * the DOM to be tested. The component only wires state to these functions.
 */

export type SortKey = "name" | "status" | "size" | "node" | "up" | "seeds" | "uploaded" | "ratio";
export type SortDir = "asc" | "desc";
export type Sort = { key: SortKey; dir: SortDir };

export type Filter = {
  q: string;
  node: string;
  status: string;
  minSize: number;
};

/** nameOf: the torrent's label. No name, the hash; neither present, empty string. */
export function nameOf(t: Torrent): string {
  return t.name ?? t.hash ?? "";
}

/**
 * ratioValue returns the ratio as a SORTABLE number. `fmtRatio` shows "∞" (seeded with
 * no download, common for content published by us) and "—" (no traffic); in a naïve
 * comparator, `Infinity - Infinity` yields NaN and the order becomes undefined — rows
 * shuffle on every refresh. Here infinity becomes the largest finite and "no traffic"
 * goes below any real ratio.
 */
export function ratioValue(t: Torrent): number {
  const up = t.uploaded_total ?? 0;
  const dl = t.downloaded_total ?? 0;
  if (dl > 0) return up / dl;
  return up > 0 ? Number.MAX_SAFE_INTEGER : -1;
}

/**
 * filterTorrents applies the screen's four filters. Search matches name OR hash — a
 * torrent without a name yet must still be findable by what the operator sees on screen.
 */
export function filterTorrents(torrents: Torrent[], f: Filter): Torrent[] {
  const term = f.q.trim().toLowerCase();
  return torrents.filter((t) => {
    if (term && !nameOf(t).toLowerCase().includes(term)) return false;
    if (f.node && t.node_id !== f.node) return false;
    if (f.status && t.status !== f.status) return false;
    // Torrent without size_bytes counts as 0: with an active size filter it drops from the list,
    // which is the honest behavior — you can't claim it meets the minimum.
    if (f.minSize && (t.size_bytes ?? 0) < f.minSize) return false;
    return true;
  });
}

/** cmpName: `numeric` puts "S2" before "S10" — the order you'd expect from a release name. */
export function cmpName(a: Torrent, b: Torrent): number {
  return nameOf(a).localeCompare(nameOf(b), getLang(), { sensitivity: "base", numeric: true });
}

/**
 * sortTorrents sorts a COPY (doesn't mutate the input — the array comes from React state).
 *
 * `nodeName` is passed as a parameter because the Node's label depends on the loaded
 * Nodes list, which is component state; passing it keeps this function pure.
 */
export function sortTorrents(
  torrents: Torrent[],
  sort: Sort,
  nodeName: (id: string) => string,
): Torrent[] {
  const val = (t: Torrent): number | string => {
    switch (sort.key) {
      case "name":
        return nameOf(t);
      case "status":
        return t.status;
      case "size":
        return t.size_bytes ?? 0;
      case "node":
        return nodeName(t.node_id);
      case "up":
        return t.up_speed ?? 0;
      case "seeds":
        return t.num_seeds ?? 0;
      case "uploaded":
        return t.uploaded_total ?? 0;
      case "ratio":
        return ratioValue(t);
    }
  };
  const dir = sort.dir === "asc" ? 1 : -1;
  return [...torrents].sort((a, b) => {
    const va = val(a);
    const vb = val(b);
    let d = 0;
    if (typeof va === "string" && typeof vb === "string") {
      d = va.localeCompare(vb, getLang(), { sensitivity: "base", numeric: true });
    } else {
      d = (va as number) - (vb as number);
    }
    // ALWAYS break ties by name: without it, ties (dozens of seeders with upload 0)
    // make rows swap positions on every 10s poll.
    return d !== 0 ? d * dir : cmpName(a, b);
  });
}
