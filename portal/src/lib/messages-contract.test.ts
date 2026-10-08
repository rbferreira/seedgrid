import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { isPlural, lookup, setLang, LANGS } from "../i18n";
import { DERIVED, VARIANT, catalogPath, messageText } from "./messages";

// Contract test for ADR-041 §3: the Control Plane registry (control-plane/internal/messages,
// exported to shared/messages.json) against the Portal catalogs. New code without a phrase,
// a phrase that requests data the payload doesn't provide, or a phraseless variant fails here.

type Entry = {
  kind: "alert" | "event" | "activity" | "error" | "audit";
  code: string;
  payload: string[];
  variant?: string;
  variants?: string[];
  // Error that currently covers different situations: the Portal shows the error.message (#34, 34d).
  generic?: boolean;
};

const registry: Entry[] = JSON.parse(
  readFileSync(join(__dirname, "..", "..", "..", "shared", "messages.json"), "utf-8"),
);

// Outside the payload: the Activity brings the torrent name in the entry itself; the error brings the
// error.message, which the phrases cite as {detail}.
const OUTSIDE_PAYLOAD: Record<Entry["kind"], string[]> = {
  alert: [],
  event: [],
  activity: ["torrent_name"],
  error: ["detail"],
  audit: [],
};

function phrases(leaf: unknown): string[] {
  if (typeof leaf === "string") return [leaf];
  if (isPlural(leaf)) return Object.values(leaf);
  return [];
}

function allowed(e: Entry): Set<string> {
  const ok = new Set([...e.payload, ...OUTSIDE_PAYLOAD[e.kind]]);
  for (const [name, sources] of Object.entries(DERIVED)) {
    if (sources.some((s) => e.payload.includes(s))) ok.add(name);
  }
  return ok;
}

describe("message contract: shared/messages.json × catalogs", () => {
  afterEach(() => setLang("en"));

  it("the record was read and is not empty", () => {
    expect(registry.length).toBeGreaterThan(20);
  });

  for (const lang of LANGS) {
    for (const e of registry.filter((x) => !x.generic)) {
      it(`${lang}: ${e.kind}/${e.code} has a phrase, and it only uses data the payload carries`, () => {
        const node = lookup(lang, catalogPath(e.kind, e.code));
        expect(node, "sem frase no catálogo").toBeDefined();
        const ok = allowed(e);
        const leaves: string[] = [];
        if (e.variant) {
          expect(VARIANT[`${e.kind}/${e.code}`], "VARIANT em lib/messages diverge do registro").toBe(e.variant);
          for (const v of e.variants ?? []) {
            const leaf = (node as Record<string, unknown>)[v];
            expect(phrases(leaf).length, `variante ${v} sem frase`).toBeGreaterThan(0);
            leaves.push(...phrases(leaf));
          }
        } else {
          expect(VARIANT[`${e.kind}/${e.code}`], "VARIANT em lib/messages tem um código sem variante").toBeUndefined();
          leaves.push(...phrases(node));
          expect(leaves.length, "a frase tem de ser texto (ou plural)").toBeGreaterThan(0);
        }
        for (const text of leaves) {
          for (const m of text.matchAll(/\{(\w+)\}/g)) {
            expect(ok.has(m[1]), `a frase pede {${m[1]}}, que o payload de ${e.code} não traz: "${text}"`).toBe(true);
          }
        }
      });
    }

    it(`${lang}: the catalog has no phrase for a code the Control Plane does not emit`, () => {
      const known = new Set(registry.map((e) => `${e.kind}/${e.code}`));
      for (const kind of ["alert", "event", "activity", "error", "audit"] as const) {
        const codes = Object.keys((lookup(lang, catalogPath(kind, "").slice(0, -1)) as Record<string, unknown>) ?? {});
        for (const code of codes) expect(known.has(`${kind}/${code}`), `${kind}/${code}`).toBe(true);
      }
    });
  }
});

describe("messageText", () => {
  afterEach(() => setLang("en"));

  it("builds the sentence in the active language from the payload", () => {
    setLang("pt-BR");
    expect(messageText("event", "TORRENT_PAUSED", { name: "Debian", hash: "abc" }, "msg gravada")).toBe(
      "Torrent pausado: Debian",
    );
    setLang("en");
    expect(messageText("event", "TORRENT_PAUSED", { name: "Debian", hash: "abc" }, "msg gravada")).toBe(
      "Torrent paused: Debian",
    );
  });

  it("chooses the variant by payload value", () => {
    setLang("en");
    const p = { name: "x", hash: "h", hostname: "n" };
    expect(messageText("event", "TORRENT_REMOVED", { ...p, reason: "inventory_reconcile" }, "")).toContain("inventory");
    expect(messageText("event", "TORRENT_MIGRATION_FAILED", { ...p, cancelled: true }, "")).toContain("cancelled");
  });

  it("old record without the phrase data falls back to the stored message", () => {
    setLang("en");
    // TORRENT_REMOVED before #33: without `reason`, there is no way to choose the variant.
    expect(messageText("event", "TORRENT_REMOVED", { name: "x" }, "Torrent removido do Node: x")).toBe(
      "Torrent removido do Node: x",
    );
    // NODE_IPV6_CHANGED without the addresses in the payload.
    expect(messageText("alert", "NODE_IPV6_CHANGED", {}, "texto antigo")).toBe("texto antigo");
  });

  it("error: phrase from code, with the numbers from error.params; generic keeps the message", () => {
    setLang("en");
    const p = { need_bytes: 3 * 1024 ** 3, min_free_bytes: 1024 ** 3, best_free_bytes: 2 * 1024 ** 3 };
    expect(messageText("error", "no_node_with_space", p, "msg pt", { detail: "msg pt" })).toContain("needs 3.00 GB");
    expect(messageText("error", "no_node_with_space", {}, "msg pt", { detail: "msg pt" })).toBe("msg pt");
    expect(messageText("error", "invalid_request", {}, "nome obrigatório", { detail: "nome obrigatório" })).toBe("nome obrigatório");
    expect(messageText("error", "tracker_upload_failed", {}, "HTTP 500", { detail: "HTTP 500" })).toBe(
      "The tracker rejected the upload: HTTP 500",
    );
  });

  it("audit: variant, setting label in the reader's language and import sections", () => {
    setLang("en");
    expect(messageText("audit", "node_renamed", { change: "renamed", hostname: "n1", before: "a", after: "b" }, "pt")).toBe(
      "Renamed Node «a» to «b»",
    );
    expect(
      messageText("audit", "node_override_set", { key: "k.x", label: "Rótulo", node: "n1", had_before: false, value: "3", effective: "3" }, "pt"),
    ).toBe("Set «Rótulo» on Node «n1» to 3 (it followed the cluster before)");
    expect(messageText("audit", "node_drained", { source: "a", count: 2, dest: "b" }, "pt")).toBe("Drained Node «a» (2 torrents) to Node «b»");
    expect(messageText("audit", "setting_changed", { key: "k.x", label: "Rótulo", value: "", value_kind: "secret" }, "pt")).toBe(
      "Changed setting «Rótulo» (value hidden)",
    );
    const counts = { tracker: "T", new: 1, renamed: 0, kind_set: 0, protected: 0 };
    expect(messageText("audit", "tracker_profile_imported", { ...counts, description: "", image_host: "" }, "pt")).toMatch(/its site$/);
    expect(messageText("audit", "tracker_profile_imported", { ...counts, description: "new", image_host: "same" }, "pt")).toMatch(
      /its site; description layout: applied; image host: already the same$/,
    );
    expect(messageText("audit", "route_torrent_pause", null, "Pausou um torrent")).toBe("Paused a torrent");
    // Torrent without a name (already removed): the recorded fallback, not «».
    expect(messageText("audit", "torrent_paused", { name: "" }, "Pausou um torrent")).toBe("Pausou um torrent");
  });

  it("unknown code shows the message, and without message the code itself", () => {
    expect(messageText("alert", "CODIGO_NOVO", {}, "do servidor")).toBe("do servidor");
    expect(messageText("alert", "CODIGO_NOVO", {}, undefined)).toBe("CODIGO_NOVO");
  });

  it("derives what the phrase needs: short hash without name, formatted disk deficit", () => {
    setLang("en");
    expect(messageText("event", "TORRENT_PAUSED", { hash: "0123456789abcdef" }, "")).toMatch(/^Torrent paused: \S+/);
    expect(
      messageText("alert", "NODE_DISK_FULL", { hostname: "n1", min_free_bytes: 3 * 1024 ** 3, projected_free_bytes: 1024 ** 3 }, ""),
    ).toContain("2.00 GB");
  });
});
