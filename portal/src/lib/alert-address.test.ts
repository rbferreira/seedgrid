import { describe, expect, it } from "vitest";
import { currentBtEndpoint } from "./alert-address";

const node = { id: "n1", public_ipv6: "2001:db8:0:2::1", bt_port: 6881 };
const openAlert = { code: "NODE_INBOUND_SILENT", status: "OPEN", node_id: "n1" };
const openFirewalled = { code: "NODE_FIREWALLED", status: "OPEN", node_id: "n1" };

/**
 * The text for NODE_INBOUND_SILENT and NODE_FIREWALLED no longer includes the address (#203, #223):
 * it was recorded on opening and became incorrect when the Node changed IPv6 while the alert was open.
 * The displayed address comes from the Node, so it tracks the change.
 */
describe("currentBtEndpoint", () => {
  it("shows the Node's CURRENT address, not the one at opening", () => {
    expect(currentBtEndpoint(openAlert, [node])).toBe("[2001:db8:0:2::1]:6881");
    // The Node changes address while the alert is open: the tooltip tracks it.
    expect(currentBtEndpoint(openAlert, [{ ...node, public_ipv6: "2001:db8:0:3::1" }])).toBe("[2001:db8:0:3::1]:6881");
  });

  it("also applies to NODE_FIREWALLED (#223)", () => {
    expect(currentBtEndpoint(openFirewalled, [node])).toBe("[2001:db8:0:2::1]:6881");
    // The Node changes address while the alert is open: the tooltip tracks it.
    expect(currentBtEndpoint(openFirewalled, [{ ...node, public_ipv6: "2001:db8:0:3::1" }])).toBe("[2001:db8:0:3::1]:6881");
  });

  it("only applies to open alert: today's address does not explain a closed episode", () => {
    expect(currentBtEndpoint({ ...openAlert, status: "RESOLVED" }, [node])).toBeNull();
    expect(currentBtEndpoint({ ...openFirewalled, status: "RESOLVED" }, [node])).toBeNull();
  });

  it("only applies to codes whose text does not carry the address", () => {
    // NODE_IPV6_CHANGED still includes the address in the text itself (it's part of the condition).
    expect(currentBtEndpoint({ ...openAlert, code: "NODE_IPV6_CHANGED" }, [node])).toBeNull();
    expect(currentBtEndpoint({ ...openAlert, code: "NODE_OFFLINE" }, [node])).toBeNull();
  });

  it("unknown does not become an instruction: without Node, address, or port, it shows nothing", () => {
    expect(currentBtEndpoint({ ...openAlert, node_id: undefined }, [node])).toBeNull();
    expect(currentBtEndpoint(openAlert, [])).toBeNull();
    expect(currentBtEndpoint(openAlert, [{ ...node, public_ipv6: null }])).toBeNull();
    expect(currentBtEndpoint(openAlert, [{ ...node, public_ipv6: "" }])).toBeNull();
    expect(currentBtEndpoint(openAlert, [{ ...node, bt_port: undefined }])).toBeNull();
    // Same behavior for NODE_FIREWALLED.
    expect(currentBtEndpoint(openFirewalled, [])).toBeNull();
    expect(currentBtEndpoint(openFirewalled, [{ ...node, public_ipv6: "" }])).toBeNull();
  });

  it("finds the right Node among several", () => {
    const other = { id: "n2", public_ipv6: "2001:db8:0:9::1", bt_port: 6882 };
    expect(currentBtEndpoint({ ...openAlert, node_id: "n2" }, [node, other])).toBe("[2001:db8:0:9::1]:6882");
    expect(currentBtEndpoint({ ...openFirewalled, node_id: "n2" }, [node, other])).toBe("[2001:db8:0:9::1]:6882");
  });
});
