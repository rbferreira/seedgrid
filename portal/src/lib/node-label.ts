/**
 * Display name of a Node (#145).
 *
 * The Node joins the cluster with the OS hostname. The operator can assign a NICKNAME
 * (`display_name`) that lives only inside SeedGrid — the hostname remains the technical
 * data, untouched. Every screen showing a Node's name uses these functions, so the
 * rule ("nickname when it exists, hostname when not") has a single owner.
 */

/**
 * Nickname limit, in characters. Mirrors node.MaxDisplayNameLen in the Control Plane, which is
 * what actually validates it — here it just avoids the round-trip for a name that is too long.
 */
export const NODE_NAME_MAX = 64;

/** A Node's name fields, as delivered by the API (also in Activity and Analytics). */
export type NodeNameFields = { hostname?: string | null; display_name?: string | null };

/** The nickname, or "" when there is none (absent, null or just spaces). */
function nicknameOf(n: NodeNameFields | null | undefined): string {
  return n?.display_name?.trim() ?? "";
}

/**
 * nodeLabel: the name the Portal displays — the nickname when it exists, the hostname otherwise.
 * "" when there is no Node at all (the caller falls back to the short id with `|| shortId(id)`).
 */
export function nodeLabel(n: NodeNameFields | null | undefined): string {
  return nicknameOf(n) || n?.hostname || "";
}

/** nodeLabelById: the label of the Node for an id, or "" if it is not in the list. */
export function nodeLabelById(nodes: (NodeNameFields & { id: string })[], id: string): string {
  return nodeLabel(nodes.find((n) => n.id === id));
}

/**
 * hasNickname: the Node has a nickname that actually differs from the hostname — meaning the hostname
 * is extra information, and not the same thing said twice.
 */
export function hasNickname(n: NodeNameFields | null | undefined): boolean {
  const nick = nicknameOf(n);
  return nick !== "" && nick !== n?.hostname;
}

/**
 * nodeLabelFull: "Nickname (hostname)" when there is a nickname, the hostname otherwise. It is the text for
 * destructive confirmations (remove, restart the Engine, update the Agent): the operator
 * recognizes the Node by its nickname, but when confirming an irreversible action they must also see
 * the actual machine that will undergo it.
 */
export function nodeLabelFull(n: NodeNameFields | null | undefined): string {
  return hasNickname(n) ? `${nodeLabel(n)} (${n?.hostname})` : nodeLabel(n);
}

type Labeled = { labels: Record<string, string> };

/**
 * relabelByNode swaps, in Prometheus series, the hostname from `labels.instance`
 * for the Node's displayed label — the legend and tooltip of the graphs now show the
 * nickname. Series whose `instance` is not from a Node (the Control Plane) remain unchanged.
 *
 * The graph indexes series by the text of `instance`, so two identical labels would
 * merge into a single series. The Control Plane prevents a nickname equal to the nickname/hostname of
 * another Node, but a Node that joins LATER with a hostname equal to an existing nickname
 * bypasses this check; in this case the nickname gets the hostname in parentheses.
 */
export function relabelByNode<T extends Labeled>(series: T[], nodes: NodeNameFields[]): T[] {
  const byLabel = new Map<string, number>();
  for (const n of nodes) byLabel.set(nodeLabel(n), (byLabel.get(nodeLabel(n)) ?? 0) + 1);

  const byHostname = new Map<string, string>();
  for (const n of nodes) {
    if (!n.hostname) continue;
    const ambiguous = (byLabel.get(nodeLabel(n)) ?? 0) > 1;
    byHostname.set(n.hostname, ambiguous ? nodeLabelFull(n) : nodeLabel(n));
  }

  return series.map((s) => {
    const updated = byHostname.get(s.labels.instance);
    return updated && updated !== s.labels.instance ? { ...s, labels: { ...s.labels, instance: updated } } : s;
  });
}
