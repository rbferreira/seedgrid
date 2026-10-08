import { describe, expect, it } from "vitest";
import { hasNickname, nodeLabel, nodeLabelById, nodeLabelFull, relabelByNode } from "./node-label";

describe("nodeLabel", () => {
  it("uses the nickname when it exists", () => {
    expect(nodeLabel({ hostname: "seedgrid-node-02", display_name: "Office" })).toBe("Office");
  });

  it("falls back to the hostname when there is no nickname", () => {
    expect(nodeLabel({ hostname: "seedgrid-node-02" })).toBe("seedgrid-node-02");
    expect(nodeLabel({ hostname: "seedgrid-node-02", display_name: null })).toBe("seedgrid-node-02");
    expect(nodeLabel({ hostname: "seedgrid-node-02", display_name: "" })).toBe("seedgrid-node-02");
  });

  it("whitespace-only nickname does not count as a nickname", () => {
    expect(nodeLabel({ hostname: "seedgrid-node-02", display_name: "   " })).toBe("seedgrid-node-02");
  });

  it("without a Node returns empty, so the caller falls back to the short id", () => {
    expect(nodeLabel(undefined)).toBe("");
    expect(nodeLabel(null)).toBe("");
    expect(nodeLabel({})).toBe("");
  });
});

describe("nodeLabelById", () => {
  const nodes = [
    { id: "a", hostname: "node-a", display_name: "Office" },
    { id: "b", hostname: "node-b" },
  ];
  it("resolves by id, with nickname or hostname", () => {
    expect(nodeLabelById(nodes, "a")).toBe("Office");
    expect(nodeLabelById(nodes, "b")).toBe("node-b");
  });
  it("id outside the list (Node removed) returns empty", () => {
    expect(nodeLabelById(nodes, "zzz")).toBe("");
  });
});

describe("hasNickname / nodeLabelFull", () => {
  it("with nickname: shows both", () => {
    const n = { hostname: "seedgrid-node-02", display_name: "Office" };
    expect(hasNickname(n)).toBe(true);
    expect(nodeLabelFull(n)).toBe("Office (seedgrid-node-02)");
  });

  it("without nickname: only the hostname, without repeated parentheses", () => {
    const n = { hostname: "seedgrid-node-02" };
    expect(hasNickname(n)).toBe(false);
    expect(nodeLabelFull(n)).toBe("seedgrid-node-02");
  });

  it("nickname equal to hostname is not extra information", () => {
    const n = { hostname: "seedgrid-node-02", display_name: "seedgrid-node-02" };
    expect(hasNickname(n)).toBe(false);
    expect(nodeLabelFull(n)).toBe("seedgrid-node-02");
  });
});

describe("relabelByNode", () => {
  const seriesOf = (instance: string) => ({ labels: { instance }, points: [] as number[] });

  it("replaces the instance hostname with the nickname", () => {
    const out = relabelByNode(
      [seriesOf("node-a"), seriesOf("node-b")],
      [
        { hostname: "node-a", display_name: "Office" },
        { hostname: "node-b" },
      ],
    );
    expect(out.map((s) => s.labels.instance)).toEqual(["Office", "node-b"]);
  });

  it("does not touch a non-Node series (the Control Plane)", () => {
    const cp = seriesOf("seedgrid-controller");
    const out = relabelByNode([cp], [{ hostname: "node-a", display_name: "Office" }]);
    expect(out[0]).toBe(cp);
  });

  it("does not mutate the original series", () => {
    const original = seriesOf("node-a");
    relabelByNode([original], [{ hostname: "node-a", display_name: "Office" }]);
    expect(original.labels.instance).toBe("node-a");
  });

  it("repeated label does not merge series: the nickname gets the hostname", () => {
    // node-c was created LATER with hostname "Office", same as node-a's alias. Without
    // disambiguating, the two series would share the "Office" key in the graph.
    const out = relabelByNode(
      [seriesOf("node-a"), seriesOf("Office")],
      [
        { hostname: "node-a", display_name: "Office" },
        { hostname: "Office" },
      ],
    );
    const names = out.map((s) => s.labels.instance);
    expect(new Set(names).size).toBe(2);
    expect(names[0]).toBe("Office (node-a)");
  });
});
