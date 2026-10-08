import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { TAXONOMY_EXAMPLE, otherSite, exampleSite, empty } from "../../lib/taxonomy.fixtures";
import { TaxonomySelects, type TaxonomyValues } from "./TaxonomySelects";
import { setLang } from "../../i18n";

// This test checks the text in Portuguese: the default for the Portal is English (ADR-041).
beforeEach(() => setLang("pt-BR"));

// Server-side rendering: no DOM, no new dependencies, and proves what #196 requires of the
// Portal — that the publishing panel renders the tracker PROFILE options and not from
// lists hardcoded in the frontend.

const render = (profile: Parameters<typeof TaxonomySelects>[0]["profile"], values: TaxonomyValues) =>
  renderToStaticMarkup(
    <TaxonomySelects profile={profile} values={values} onChange={() => {}} inputCls="x" />,
  );

/** Extracts the options from a <select aria-label="..."> as [value, label]. */
function options(html: string, label: string): [number, string][] {
  const sel = new RegExp(`<select[^>]*aria-label="${label}"[^>]*>([\\s\\S]*?)</select>`).exec(html);
  if (!sel) throw new Error(`select ${label} não encontrado em: ${html}`);
  return [...sel[1].matchAll(/<option value="(\d+)"[^>]*>([^<]*)<\/option>/g)].map((m): [number, string] => [
    Number(m[1]),
    m[2],
  ]);
}

const zero: TaxonomyValues = { category_id: 0, type_id: 0, resolution_id: 0 };

describe("TaxonomySelects", () => {
  it("the example shows exactly the profile's options", () => {
    const html = render(exampleSite, { category_id: 1, type_id: 3, resolution_id: 3 });
    // 0 is the "— select —" placeholder; the rest are the profile, in order.
    expect(options(html, "Categoria").slice(1)).toEqual(TAXONOMY_EXAMPLE.category.map(([id, n]) => [id, n]));
    expect(options(html, "Tipo").slice(1)).toEqual(TAXONOMY_EXAMPLE.type.map(([id, n]) => [id, n]));
    expect(options(html, "Resolução").slice(1)).toEqual(TAXONOMY_EXAMPLE.resolution.map(([id, n]) => [id, n]));
  });

  it("another site shows its options, with its IDs", () => {
    const html = render(otherSite, zero);
    expect(options(html, "Tipo")).toEqual([
      [0, "— selecione —"],
      [1, "Disc"],
      [3, "Remux"],
      [8, "WEB-DL"],
      [12, "Encode"],
    ]);
    // No option from the example leaked here.
    expect(html).not.toContain("Full Disc");
    expect(html).not.toContain("Materiais de Apoio");
  });

  it("marks the current value's option as selected", () => {
    const html = render(exampleSite, { category_id: 2, type_id: 4, resolution_id: 5 });
    expect(html).toMatch(/<option value="2" selected="">Séries<\/option>/);
    expect(html).toMatch(/<option value="4" selected="">WEB-DL<\/option>/);
    expect(html).toMatch(/<option value="5" selected="">720p<\/option>/);
  });

  it("empty profile doesn't invent options: says there are none", () => {
    const html = render(empty, zero);
    expect(options(html, "Categoria")).toEqual([[0, "— sem opções no perfil —"]]);
    expect(options(html, "Tipo")).toEqual([[0, "— sem opções no perfil —"]]);
  });

  it("missing profile (tracker from an old API version) doesn't break either", () => {
    expect(() => render(undefined, zero)).not.toThrow();
  });

  it("value not in profile appears as out of profile — it is not silently swapped", () => {
    const html = render(otherSite, { category_id: 5, type_id: 3, resolution_id: 0 });
    expect(options(html, "Categoria")).toContainEqual([5, "#5 (fora do perfil)"]);
  });
});
