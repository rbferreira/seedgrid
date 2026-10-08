import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentCell } from "./Dashboard";
import type { AgentRelease, Node } from "../api";

// The Dashboard is the fallback page for anyone without any permissions (docs/11 - Portal.md
// §13.1). The Agent update button requires nodes:write: without it the operator still sees there is a
// new version — it's data —, but in an inert badge, not in a button that would return 403.
const node: Node = {
  id: "n1",
  uuid: "u1",
  hostname: "seedgrid-node-02",
  status: "ONLINE",
  agent_version: "v1.0.0",
  architecture: "arm64",
};
const release: AgentRelease = { version: "v1.1.0", archs: ["arm64", "amd64"] };

function render(canUpdate: boolean, available: AgentRelease | null = release) {
  return renderToStaticMarkup(
    <AgentCell node={node} available={available} busy={false} canUpdate={canUpdate} onUpdate={() => {}} />,
  );
}

describe("AgentCell — gated by button", () => {
  it("with nodes:write the update badge is a button", () => {
    const html = render(true);
    expect(html).toContain("<button");
    expect(html).toContain("v1.1.0");
  });

  it("without nodes:write the badge remains visible, but is not a button", () => {
    const html = render(false);
    expect(html).not.toContain("<button");
    expect(html).toContain("v1.0.0"); // current version
    expect(html).toContain("v1.1.0"); // available version — the data doesn't disappear
  });

  it("without available update, shows only the current version, with or without permission", () => {
    for (const can of [true, false]) {
      const html = render(can, { version: "v1.0.0", archs: ["arm64"] });
      expect(html).not.toContain("<button");
      expect(html).toContain("v1.0.0");
    }
  });
});
