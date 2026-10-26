/**
 * Who manages trackers (ADR-047).
 *
 * Trackers belong to the cluster and the Administrator manages them in Settings › Trackers. The
 * screens that need a tracker (Publish, the Catalog, the publish panel) send whoever can manage
 * them to that screen, and tell everyone else to ask an administrator — instead of linking to a
 * page they cannot open.
 */

/** The user can open Settings and change trackers there. */
export const canManageTrackers = (can: (perm: string) => boolean): boolean =>
  can("settings:write") && can("trackers:write");
