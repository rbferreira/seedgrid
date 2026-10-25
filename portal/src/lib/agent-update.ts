import type { AgentRelease, Node } from "../api";

// agentOutdated: the Node runs an older Agent than the published one AND there is a binary for
// its architecture — the update would work (ADR-027). Without `available` (agent.artifacts_url not
// configured) or without a binary for the architecture there is nothing to offer. The per-Node
// badge and "Update all" both use it, so they agree on which Nodes are behind.
export function agentOutdated(node: Node, available: AgentRelease | null): boolean {
  const current = node.agent_version ?? "";
  const hasBinary = !!node.architecture && !!available?.archs.includes(node.architecture);
  return !!available && !!current && current !== available.version && hasBinary;
}

// bulkUpdateTargets: the Nodes "Update all" acts on (ADR-046) — behind AND online. An offline
// Node's command would wait in the queue (ADR-014) and the Portal could not confirm it; it keeps
// its own button.
export function bulkUpdateTargets(nodes: Node[], available: AgentRelease | null): Node[] {
  return nodes.filter((n) => n.status === "ONLINE" && agentOutdated(n, available));
}

// updateWithCanary updates the first Node and the others only after it came back on the new
// version (ADR-046). `update` resolves true when that Node reported the target version. Resolves
// false when the canary did not: the others were not touched.
export async function updateWithCanary(nodes: Node[], update: (n: Node) => Promise<boolean>): Promise<boolean> {
  const [canary, ...rest] = nodes;
  if (!canary) return true;
  if (!(await update(canary))) return false;
  await Promise.all(rest.map(update));
  return true;
}
