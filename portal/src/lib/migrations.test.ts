import { describe, expect, it } from "vitest";
import { canCancelMigration, cancelMigrationPrompt, migrationStatusLabel } from "./migrations";
import { translate, type TFn } from "../i18n";

// The assertions check the Portuguese text; the Portal standard is English (ADR-041).
const t: TFn = (key, params) => translate("pt-BR", key, params);

describe("migrationStatusLabel", () => {
  it("speaks the operator's language, not the database's", () => {
    expect(migrationStatusLabel(t, { status: "QUEUED" })).toBe("Na fila");
    expect(migrationStatusLabel(t, { status: "PENDING" })).toBe("Em andamento");
    expect(migrationStatusLabel(t, { status: "COMPLETED" })).toBe("Concluída");
    expect(migrationStatusLabel(t, { status: "FAILED" })).toBe("Falhou");
  });

  it("migration canceled by the operator does not appear as failure", () => {
    expect(migrationStatusLabel(t, { status: "FAILED", cancelled: true })).toBe("Cancelada");
  });

  it("unknown status passes through as it came, instead of disappearing", () => {
    expect(migrationStatusLabel(t, { status: "OUTRO" })).toBe("OUTRO");
  });
});

describe("canCancelMigration", () => {
  it("only in-flight migration can be canceled", () => {
    expect(canCancelMigration({ status: "QUEUED" })).toBe(true);
    expect(canCancelMigration({ status: "PENDING" })).toBe(true);
    expect(canCancelMigration({ status: "COMPLETED" })).toBe(false);
    expect(canCancelMigration({ status: "FAILED" })).toBe(false);
  });
});

describe("cancelMigrationPrompt", () => {
  it("warns that the destination copy is discarded and that the torrent stays at the origin", () => {
    const msg = cancelMigrationPrompt(t, "node-01", "node-02", "PENDING");
    expect(msg).toContain("node-01 → node-02");
    expect(msg).toContain("DESCARTADA");
    expect(msg).toContain("continua no Node de origem");
  });

  it("queued migration does not promise to discard anything — nothing was copied", () => {
    const msg = cancelMigrationPrompt(t, "node-01", "node-02", "QUEUED");
    expect(msg).toContain("nada foi copiado");
    expect(msg).not.toContain("DESCARTADA");
  });

  it("without a Node name, it is still readable", () => {
    expect(cancelMigrationPrompt(t, "", "", "PENDING")).toContain("origem → destino");
  });
});
