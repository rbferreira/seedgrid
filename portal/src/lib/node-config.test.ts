import { describe, expect, it } from "vitest";
import { checkConfigValue, configStatusMeta, fmtConfigValue } from "./node-config";
import { translate, type TFn } from "../i18n";
import type { NodeSettingItem } from "../api";

const base: NodeSettingItem = {
  key: "k",
  type: "int",
  label: "L",
  description: "d",
  unit: "",
  min: 1,
  max: 200,
  value: "3",
  source: "cluster",
  cluster_value: "3",
  cluster_source: "default",
  override: null,
};

// The assertions check the text in Portuguese; the Portal standard is English (ADR-041).
const t: TFn = (key, params) => translate("pt-BR", key, params);

describe("configStatusMeta", () => {
  it("up to date is green; waiting is amber; unsupported is neutral (not an error)", () => {
    expect(configStatusMeta(t, "in_sync")).toMatchObject({ label: "Em dia", tone: "ok" });
    expect(configStatusMeta(t, "pending")).toMatchObject({ label: "Aguardando o Agent", tone: "warn" });
    expect(configStatusMeta(t, "unsupported")).toMatchObject({ label: "Agent sem suporte", tone: "muted" });
  });

  it("every state explains itself", () => {
    for (const s of ["in_sync", "pending", "unsupported"] as const) {
      expect(configStatusMeta(t, s).hint.length).toBeGreaterThan(20);
    }
  });
});

describe("fmtConfigValue", () => {
  it("bool becomes On/Off, not true/false", () => {
    expect(fmtConfigValue(t, { type: "bool", unit: "" }, "true")).toBe("Ligado");
    expect(fmtConfigValue(t, { type: "bool", unit: "" }, "false")).toBe("Desligado");
  });
  it("includes the catalog unit when present", () => {
    expect(fmtConfigValue(t, { type: "int", unit: "KB/s" }, "1")).toBe("1 KB/s");
    expect(fmtConfigValue(t, { type: "duration", unit: "" }, "10s")).toBe("10s");
  });
});

// Browser validation is just a quick return: the Control Plane (same cluster Spec) decides.
describe("checkConfigValue", () => {
  it("empty and non-numeric", () => {
    expect(checkConfigValue(t, base, "")).not.toBe("");
    expect(checkConfigValue(t, base, "  ")).not.toBe("");
    expect(checkConfigValue(t, base, "muitos")).not.toBe("");
    expect(checkConfigValue(t, base, "3.5")).not.toBe("");
  });
  it("catalog range", () => {
    expect(checkConfigValue(t, base, "0")).toContain("mínimo");
    expect(checkConfigValue(t, base, "-1")).toContain("mínimo");
    expect(checkConfigValue(t, base, "201")).toContain("máximo");
    expect(checkConfigValue(t, base, "3")).toBe("");
    expect(checkConfigValue(t, base, " 10 ")).toBe("");
  });
  it("duration", () => {
    const d: NodeSettingItem = { ...base, type: "duration", min: 2, max: 60 };
    expect(checkConfigValue(t, d, "10s")).toBe("");
    expect(checkConfigValue(t, d, "1m30s")).toBe("");
    expect(checkConfigValue(t, d, "dez")).not.toBe("");
    expect(checkConfigValue(t, d, "10")).not.toBe(""); // no unit: Go would refuse
  });
  it("bool e enum", () => {
    const b: NodeSettingItem = { ...base, type: "bool", min: null, max: null };
    expect(checkConfigValue(t, b, "true")).toBe("");
    expect(checkConfigValue(t, b, "talvez")).not.toBe("");
    const e: NodeSettingItem = { ...base, type: "string", min: null, max: null, options: ["debug", "info"] };
    expect(checkConfigValue(t, e, "debug")).toBe("");
    expect(checkConfigValue(t, e, "verbose")).toContain("debug, info");
  });
});
