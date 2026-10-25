import { describe, expect, it } from "vitest";
import { agentOutdated, bulkUpdateTargets, updateWithCanary } from "./agent-update";
import type { AgentRelease, Node } from "../api";

const release: AgentRelease = { version: "v1.1.0", archs: ["amd64", "arm64"] };

function node(id: string, over: Partial<Node> = {}): Node {
  return { id, uuid: id, hostname: id, status: "ONLINE", agent_version: "v1.0.0", architecture: "amd64", ...over };
}

describe("agentOutdated", () => {
  it("an older version with a binary for the architecture is behind", () => {
    expect(agentOutdated(node("a"), release)).toBe(true);
  });

  it("is not behind on the published version, without a binary, without a version or without a release", () => {
    expect(agentOutdated(node("a", { agent_version: "v1.1.0" }), release)).toBe(false);
    expect(agentOutdated(node("a", { architecture: "riscv64" }), release)).toBe(false);
    expect(agentOutdated(node("a", { architecture: undefined }), release)).toBe(false);
    expect(agentOutdated(node("a", { agent_version: undefined }), release)).toBe(false);
    expect(agentOutdated(node("a"), null)).toBe(false);
  });
});

describe("bulkUpdateTargets", () => {
  it("keeps only the online Nodes that are behind, in the list order", () => {
    const nodes = [
      node("a"),
      node("b", { status: "OFFLINE" }),
      node("c", { agent_version: "v1.1.0" }),
      node("d"),
    ];
    expect(bulkUpdateTargets(nodes, release).map((n) => n.id)).toEqual(["a", "d"]);
  });
});

describe("updateWithCanary", () => {
  it("updates the first Node alone, then the others", async () => {
    const order: string[] = [];
    let canaryDone = false;
    const ok = await updateWithCanary([node("a"), node("b"), node("c")], async (n) => {
      if (n.id !== "a") expect(canaryDone).toBe(true); // nobody starts before the canary confirmed
      order.push(n.id);
      if (n.id === "a") canaryDone = true;
      return true;
    });
    expect(ok).toBe(true);
    expect(order).toEqual(["a", "b", "c"]);
  });

  it("leaves the others alone when the canary fails", async () => {
    const touched: string[] = [];
    const ok = await updateWithCanary([node("a"), node("b"), node("c")], async (n) => {
      touched.push(n.id);
      return false;
    });
    expect(ok).toBe(false);
    expect(touched).toEqual(["a"]);
  });

  it("no Nodes: nothing to do", async () => {
    expect(await updateWithCanary([], async () => false)).toBe(true);
  });
});
