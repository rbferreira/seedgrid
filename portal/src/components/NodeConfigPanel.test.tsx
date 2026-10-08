import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { NodeConfigView, canWriteNodeConfig } from "./NodeConfigPanel";
import { NodeOverridesList } from "./NodeOverridesPanel";
import type { NodeSettingItem, NodeSettings, SettingOverride } from "../api";
import { setLang } from "../i18n";

// This test checks the text in Portuguese: the Portal default is English (ADR-041).
beforeEach(() => setLang("pt-BR"));

// "This Node's Settings" (ADR-039): the effective value, its origin, and the "up to date" badge are
// GIVEN and stay on screen for any role; the buttons to set/remove the custom value require
// nodes:write — the gate is per button, not per data (docs/11 - Portal.md §13.1).

const downloads: NodeSettingItem = {
  key: "engine.max_active_downloads",
  type: "int",
  label: "Downloads simultâneos por Node",
  description: "Teto de downloads que o Engine mantém ativos.",
  unit: "",
  min: 1,
  max: 200,
  value: "3",
  source: "node",
  cluster_value: "10",
  cluster_source: "default",
  override: "3",
};
const heartbeat: NodeSettingItem = {
  key: "agent.heartbeat_interval",
  type: "duration",
  label: "Intervalo de heartbeat",
  description: "De quantos em quantos segundos o Agent avisa que está vivo.",
  unit: "",
  min: 2,
  max: 60,
  value: "10s",
  source: "cluster",
  cluster_value: "10s",
  cluster_source: "default",
  override: null,
};
const queueing: NodeSettingItem = {
  key: "engine.queueing_enabled",
  type: "bool",
  label: "Fila do Engine",
  description: "Liga a fila do Engine.",
  unit: "",
  min: null,
  max: null,
  value: "true",
  source: "cluster",
  cluster_value: "true",
  cluster_source: "default",
  override: null,
};

function data(status: NodeSettings["status"]): NodeSettings {
  return {
    node_id: "n4",
    status,
    effective_version: "abc123def456",
    applied_version: status === "unsupported" ? null : "abc123def456",
    applied_at: status === "unsupported" ? null : "2026-10-03T12:00:00Z",
    seeded_at: null,
    items: [downloads, heartbeat, queueing],
  };
}

const noop = async () => "";

function render(d: NodeSettings, canWrite: boolean) {
  return renderToStaticMarkup(<NodeConfigView data={d} canWrite={canWrite} onSave={noop} onReset={noop} />);
}

describe("NodeConfigView — button gating", () => {
  it("users who can edit Nodes see set, change and return to cluster", () => {
    const html = render(data("in_sync"), true);
    expect(html).toContain("Valor próprio"); // on a key that follows the cluster
    expect(html).toContain("Alterar"); // on a key that already has this Node's value
    expect(html).toContain("Voltar ao cluster");
    expect(html).toContain("Ações");
  });

  it("read-only users see no buttons or the actions column...", () => {
    const html = render(data("in_sync"), false);
    expect(html).not.toContain("<button");
    for (const buttonLabel of ["Valor próprio", "Alterar", "Voltar ao cluster", "Salvar", "Ações"]) {
      expect(html).not.toContain(buttonLabel);
    }
  });

  it("...but still see the data: effective value, source and what the cluster would say", () => {
    const html = render(data("in_sync"), false);
    expect(html).toContain("Downloads simultâneos por Node");
    expect(html).toContain("engine.max_active_downloads");
    expect(html).toContain(">3<"); // the effective value of the Node
    expect(html).toContain("valor deste Node"); // comes from an override
    expect(html).toContain("cluster: 10"); // what the Node would have without it
    expect(html).toContain("10s"); // key that follows the cluster
    expect(html).toContain("Ligado"); // readable bool, not "true"
  });

  it("the gating permission is nodes:write — the same one the backend requires", () => {
    const requested: string[] = [];
    canWriteNodeConfig((perm) => {
      requested.push(perm);
      return false;
    });
    expect(requested).toEqual(["nodes:write"]);
    expect(canWriteNodeConfig(() => true)).toBe(true);
    expect(canWriteNodeConfig(() => false)).toBe(false);
  });
});

describe("NodeConfigView — is the Agent up to date?", () => {
  it("up to date", () => {
    const html = render(data("in_sync"), false);
    expect(html).toContain("Em dia");
    expect(html).toContain("aplicada pelo Agent em");
  });

  it("waiting for the Agent (changed and not yet arrived)", () => {
    expect(render(data("pending"), false)).toContain("Aguardando o Agent");
  });

  it("unsupported Agent (prior to ADR-039) is not an error and does not fake an application time", () => {
    const html = render(data("unsupported"), false);
    expect(html).toContain("Agent sem suporte");
    expect(html).not.toContain("aplicada pelo Agent em");
  });
});

describe("NodeOverridesList — those with their own value (Settings)", () => {
  const rows: SettingOverride[] = [
    {
      node_id: "n4",
      node_label: "node-04",
      key: "engine.max_active_downloads",
      label: "Downloads simultâneos por Node",
      unit: "",
      value: "3",
      cluster_value: "10",
      updated_at: "2026-10-03T12:00:00Z",
    },
  ];

  it("lists the Node, its value and the cluster's, with a link to the detail", () => {
    const html = renderToStaticMarkup(<NodeOverridesList rows={rows} />);
    expect(html).toContain("node-04");
    expect(html).toContain('href="#/nodes/n4"');
    expect(html).toContain("Downloads simultâneos por Node");
    expect(html).toContain(">3<");
    expect(html).toContain(">10<");
  });

  it("without overrides, says that changing the cluster applies to the entire fleet", () => {
    const html = renderToStaticMarkup(<NodeOverridesList rows={[]} />);
    expect(html).toContain("Nenhum Node tem valor próprio");
    expect(html).not.toContain("<table");
  });
});
