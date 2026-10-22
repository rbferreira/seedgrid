import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { NodeCard } from "./NodeCard";
import type { Node } from "../api";
import { setLang } from "../i18n";

// This test checks the Portuguese text: the Portal default is English (ADR-041).
beforeEach(() => setLang("pt-BR"));

// Button gate (docs/11 - Portal.md §13.1): the destructive button disappears for those who don't have
// permission, but the DATA — disk, memory, storage, heartbeat — remains on the screen.
const node: Node = {
  id: "n1",
  uuid: "u1",
  hostname: "seedgrid-node-02",
  status: "ONLINE",
  storage_ok: true,
  disk_total_bytes: 4_000_000_000_000,
  disk_free_bytes: 1_000_000_000_000,
  last_ip: "10.0.0.7",
  bt_port: 6882,
};

function render(n: Node, can: (perm: string) => boolean) {
  return renderToStaticMarkup(
    <NodeCard
      node={n}
      torrents={[]}
      can={can}
      busy={false}
      running={new Set()}
      onDrain={() => {}}
      onRestartEngine={() => {}}
      onDelete={() => {}}
      onRename={() => {}}
    />,
  );
}

describe("NodeCard — button gating", () => {
  it("users who can operate Nodes see all actions", () => {
    const html = render(node, () => true);
    for (const buttonLabel of ["Renomear", "Esvaziar", "Reiniciar Engine", "Remover"]) {
      expect(html).toContain(buttonLabel);
    }
  });

  it("users who only read metrics see no Node actions...", () => {
    const html = render(node, () => false);
    for (const buttonLabel of ["Renomear", "Esvaziar", "Reiniciar Engine", "Remover"]) {
      expect(html).not.toContain(buttonLabel);
    }
  });

  it("...but still see the data", () => {
    const html = render(node, () => false);
    expect(html).toContain("Storage OK");
    expect(html).toContain("Disco");
    expect(html).toContain("10.0.0.7:6882");
    expect(html).toContain("Detalhes");
  });

  it("the gating permission is nodes:write — the same one the backend requires", () => {
    const requested: string[] = [];
    render(node, (perm) => {
      requested.push(perm);
      return false;
    });
    expect(requested).toContain("nodes:write");
  });
});

describe("NodeCard — alias (#145)", () => {
  it("with a nickname: the title is the nickname and the hostname appears beside it, as technical data", () => {
    const html = render({ ...node, display_name: "Rack da sala" }, () => false);
    expect(html).toContain("Rack da sala");
    expect(html).toContain("seedgrid-node-02");
  });

  it("without a nickname: only the hostname, once", () => {
    const html = render(node, () => false);
    expect(html.match(/seedgrid-node-02/g)).toHaveLength(1);
  });
});

describe("NodeCard — a Node with its own qBittorrent (ADR-043)", () => {
  const adopted: Node = { ...node, engine_adopted: true };

  it("shows the badge and offers no Engine restart, even to who can operate Nodes", () => {
    const html = render(adopted, () => true);
    expect(html).toContain("qBittorrent próprio");
    expect(html).not.toContain("Reiniciar Engine");
    for (const buttonLabel of ["Renomear", "Esvaziar", "Remover"]) {
      expect(html).toContain(buttonLabel);
    }
  });

  it("a Node SeedGrid installs has no badge", () => {
    expect(render(node, () => true)).not.toContain("qBittorrent próprio");
  });
});
