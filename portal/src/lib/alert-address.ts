import type { Alert, Node } from "../api";

// Alerts whose text does NOT carry the Node address (#203; NODE_FIREWALLED since #223).
//
// An alert's text is recorded once, at opening, and the alert can stay open for days.
// An address inside it ages: the Node changed its IPv6 almost 38h after NODE_INBOUND_SILENT
// opened and the dashboard kept sending to unblock an address that no longer existed. The text is now
// neutral and the address comes from the Node, which the page loads and updates each cycle — it is the
// current address, not the one from opening.
const ENDPOINT_HINT_CODES = new Set(["NODE_INBOUND_SILENT", "NODE_FIREWALLED"]);

/**
 * Current BT address of the Node for an open alert, in `[ipv6]:port` format — what the operator
 * needs to check in the firewall. `null` when not applicable: alert from another code, alert
 * already closed (today's address says nothing about a closed episode), Node disappeared from the
 * list, or the Node hasn't reported an address/port yet (unknown doesn't become "[]:0").
 */
export function currentBtEndpoint(
  alert: Pick<Alert, "code" | "status" | "node_id">,
  nodes: Pick<Node, "id" | "public_ipv6" | "bt_port">[],
): string | null {
  if (alert.status !== "OPEN" || !ENDPOINT_HINT_CODES.has(alert.code) || !alert.node_id) return null;
  const node = nodes.find((n) => n.id === alert.node_id);
  if (!node?.public_ipv6 || !node.bt_port) return null;
  return `[${node.public_ipv6}]:${node.bt_port}`;
}
