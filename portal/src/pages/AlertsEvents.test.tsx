import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { CancelMigrationButton } from "./AlertsEvents";
import type { Migration } from "../api";
import { setLang } from "../i18n";

// This test checks the Portuguese text: the Portal default is English (ADR-041).
beforeEach(() => setLang("pt-BR"));

// The "Cancel migration" button follows the per-button gate (docs/11 - Portal.md §13.1): it only shows
// for users with torrents:migrate — the permission for the route it calls — and only while the
// migration is in flight. Users without permission don't see the button (a click gets 403), but
// they still see the migration: the data comes from the row, not this component.
const base: Migration = {
  id: "m1",
  torrent_id: "t1",
  source_node_id: "n1",
  dest_node_id: "n2",
  status: "PENDING",
  created_at: "2026-10-03T12:00:00Z",
};

function render(m: Partial<Migration>, canMigrate: boolean, busy = false) {
  return renderToStaticMarkup(
    <CancelMigrationButton migration={{ ...base, ...m }} canMigrate={canMigrate} busy={busy} onCancel={() => {}} />,
  );
}

describe("CancelMigrationButton — gated by button", () => {
  it("with torrents:migrate, ongoing or queued migration has the button", () => {
    for (const status of ["PENDING", "QUEUED"]) {
      const html = render({ status }, true);
      expect(html).toContain("<button");
      expect(html).toContain("Cancelar migração");
    }
  });

  it("without torrents:migrate there is no button, in any state", () => {
    for (const status of ["PENDING", "QUEUED", "COMPLETED", "FAILED"]) {
      expect(render({ status }, false)).toBe("");
    }
  });

  it("migration that has already finished cannot be canceled, even with permission", () => {
    expect(render({ status: "COMPLETED" }, true)).toBe("");
    expect(render({ status: "FAILED" }, true)).toBe("");
    // canceled by the operator is FAILED with `cancelled`: it also finished
    expect(render({ status: "FAILED", cancelled: true, error: "cancelada pelo operador" }, true)).toBe("");
  });

  it("while canceling, the button locks and warns — prevents double-click", () => {
    const html = render({ status: "PENDING" }, true, true);
    expect(html).toContain("disabled");
    expect(html).toContain("Cancelando…");
  });
});
