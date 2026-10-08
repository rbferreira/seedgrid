import { describe, expect, it } from "vitest";
import { mergeSeries, transferKind } from "./Chart";

// The transfer color comes from `labels.transfer`, never from the series name (#130): the name is
// translated, so a pt-BR legend ("Baixado") must keep the download blue.
describe("mergeSeries", () => {
  it("keeps the transfer kind per key, whatever the legend says", () => {
    const { keys, kinds } = mergeSeries([
      { labels: { instance: "Baixado", transfer: "down" }, points: [{ t: 1, v: 1 }] },
      { labels: { instance: "Enviado", transfer: "up" }, points: [{ t: 1, v: 2 }] },
      { labels: { instance: "node-01" }, points: [{ t: 1, v: 3 }] },
    ]);
    expect(keys).toEqual(["Baixado", "Enviado", "node-01"]);
    expect(kinds).toEqual({ Baixado: "down", Enviado: "up", "node-01": undefined });
  });
});

describe("transferKind", () => {
  it("accepts only down and up", () => {
    expect(transferKind({ transfer: "down" })).toBe("down");
    expect(transferKind({ transfer: "up" })).toBe("up");
    expect(transferKind({ transfer: "sideways" })).toBeUndefined();
    expect(transferKind({ instance: "Download" })).toBeUndefined();
  });
});
