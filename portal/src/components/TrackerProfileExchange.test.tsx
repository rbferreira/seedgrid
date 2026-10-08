import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ImportPreviewView, TrackerProfileExchange } from "./TrackerProfileExchange";
import type { ImportPlan, ImportPreview, Tracker } from "../api";
import { descriptionDefault, exampleSite } from "../lib/taxonomy.fixtures";
import { setLang } from "../i18n";

// This test checks the Portuguese text: the Portal default is English (ADR-041).
beforeEach(() => setLang("pt-BR"));

// Export/import the profile (#197): the gate is by BUTTON — hides the button, never the data
// (docs/11 - Portal.md §13.1) — and the preview shows what would change without saving anything.

const tracker: Tracker = {
  id: "t1",
  name: "Exemplo",
  host: "tracker.exemplo.org",
  source: "EXEMPLO",
  api_url: "https://tracker.example",
  has_api_token: true,
  has_image_host_key: false,
  image_host_key_host: "",
  is_default: true,
  created_at: "2026-10-01T00:00:00Z",
  profile: exampleSite,
  description_default: descriptionDefault,
};

const render = (can: (perm: string) => boolean, blocked = false) =>
  renderToStaticMarkup(
    <TrackerProfileExchange tracker={tracker} can={can} blocked={blocked} onChanged={() => {}} onLogout={() => {}} />,
  );

const only = (...perms: string[]) => (p: string) => perms.includes(p);

describe("TrackerProfileExchange — button gate", () => {
  it("user with tracker read and write permissions sees both buttons", () => {
    const html = render(only("trackers:read", "trackers:write"));
    expect(html).toContain("Exportar perfil");
    expect(html).toContain("Importar perfil");
  });

  it("export is read (trackers:read): read-only user sees Export and not Import", () => {
    const html = render(only("trackers:read"));
    expect(html).toContain("Exportar perfil");
    expect(html).not.toContain("Importar perfil");
    expect(html).not.toContain('type="file"');
  });

  it("import is write (trackers:write): the same permission as the route", () => {
    const html = render(only("trackers:write"));
    expect(html).not.toContain("Exportar perfil");
    expect(html).toContain("Importar perfil");
  });

  it("without either, the entire section disappears — and only the current screen's profile remains", () => {
    expect(render(only())).toBe("");
  });

  it("unsaved changes block both: the file would take the saved profile, and import would swap it underneath", () => {
    const html = render(only("trackers:read", "trackers:write"), true);
    expect(html.match(/disabled=""/g)?.length).toBe(2);
    expect(html).toContain("Salve ou descarte as alterações do perfil antes");
  });

  it("says what the file takes and, highlighted, what it never takes", () => {
    const html = render(only("trackers:read"));
    expect(html).toMatch(/Nunca[\s\S]*token da API[\s\S]*passkey[\s\S]*announce/);
  });
});

const basePlan: ImportPlan = {
  site: { name: "Blutopia", host: "blutopia.example" },
  family: "unit3d",
  schema_version: 1,
  items: {
    category: [
      { axis: "category", id: 1, name: "Filmes", kind: "movie", status: "same", sets_kind: true },
      { axis: "category", id: 2, name: "Series", kind: "tv", status: "protected", current_name: "Seriados", current_origin: "manual" },
      { axis: "category", id: 3, name: "Animes", status: "renamed", current_name: "Anime", current_origin: "discovered" },
    ],
    type: [{ axis: "type", id: 12, name: "Encode", status: "new" }],
    resolution: [],
  },
  counts: { new: 1, renamed: 1, same: 1, protected: 1, kind_set: 1, local_only: 2 },
  warnings: ["O arquivo é do site «Blutopia» (blutopia.example) e este tracker é «Exemplo» (tracker.exemplo.org)."],
};
const preview: ImportPreview = { plan: basePlan, notice: "Este perfil veio de um arquivo, não do seu site." };

const view = (overwrite: string[] = [], plan: ImportPlan = basePlan) =>
  renderToStaticMarkup(
    <ImportPreviewView
      preview={{ ...preview, plan }}
      fileName="blutopia.json"
      overwrite={new Set(overwrite)}
      applying={false}
      onToggle={() => {}}
      onApply={() => {}}
      onCancel={() => {}}
    />,
  );

describe("ImportPreviewView — the preview", () => {
  it("says which site the file is from, that nothing was saved, and the third-party content warning", () => {
    const html = view();
    expect(html).toContain("Blutopia (blutopia.example)");
    expect(html).toContain("blutopia.json");
    expect(html).toContain("Nada foi gravado ainda");
    expect(html).toContain("veio de um arquivo, não do seu site");
  });

  it("shows server warnings, including the one for a file from another site", () => {
    expect(view()).toContain("O arquivo é do site «Blutopia»");
  });

  it("shows each term with what happens: new, already exists, renames, and kept", () => {
    const html = view();
    expect(html).toContain("novo");
    expect(html).toContain("já está · preenche o papel");
    expect(html).toContain("renomeia");
    expect(html).toContain("mantido — você chamou de «Seriados»");
    // the old name appears crossed out and the new one next to it
    expect(html).toMatch(/line-through[^>]*>Anime<\/span> → Animes/);
  });

  it("summarizes what will happen and reminds that import never removes", () => {
    const html = view();
    expect(html).toContain("1 novo, 1 renomeado, 1 papel preenchido");
    expect(html).toContain("1 serão mantidos (ajustados à mão ou já confirmados pelo seu site)");
    expect(html).toContain("2 termo(s) do seu perfil que o arquivo não traz ficam como estão");
  });

  it("only the protected one has the option to overwrite; checking it changes the count", () => {
    const html = view();
    expect(html.match(/type="checkbox"/g)?.length).toBe(1);
    expect(html).toContain("usar o nome do arquivo (Categoria 2)");

    const marked = view(["category:2"]);
    expect(marked).toContain("checked");
    expect(marked).toContain("1 novo, 2 renomeados, 1 papel preenchido");
    expect(marked).not.toContain("serão mantidos");
  });

  it("says whose protected name it is: the operator's (manual) or their site's (discovered)", () => {
    const plan: ImportPlan = {
      ...basePlan,
      items: {
        category: [
          { axis: "category", id: 2, name: "Series", status: "protected", current_name: "Seriados", current_origin: "manual" },
          { axis: "category", id: 4, name: "Cursos", status: "protected", current_name: "Course", current_origin: "discovered" },
        ],
        type: [],
        resolution: [],
      },
      counts: { new: 0, renamed: 0, same: 0, protected: 2, kind_set: 0, local_only: 0 },
    };
    const html = view([], plan);
    expect(html).toContain("mantido — você chamou de «Seriados»");
    expect(html).toContain("mantido — o seu site usa «Course»");
    expect(html.match(/type="checkbox"/g)?.length).toBe(2);
  });

  it("the term that starts sharing the name with another has the warning on its own line", () => {
    const warn = "o nome é igual, sem caixa nem acento, ao do id 2 «Series»: com dois ids para o mesmo nome o painel de publicação não pré-preenche";
    const plan: ImportPlan = {
      ...basePlan,
      items: {
        category: [{ axis: "category", id: 1, name: "Séries", kind: "tv", status: "new", warnings: [warn] }],
        type: [],
        resolution: [],
      },
      counts: { new: 1, renamed: 0, same: 0, protected: 0, kind_set: 0, local_only: 1 },
    };
    const html = view([], plan);
    expect(html).toContain(`⚠ ${warn}`);
  });

  it("overwriting a protected one requested by the operator appears as such", () => {
    const plan: ImportPlan = {
      ...basePlan,
      items: {
        category: [
          { axis: "category", id: 2, name: "Series", status: "renamed", current_name: "Seriados", current_origin: "manual", overwrites_protected: true },
        ],
        type: [],
        resolution: [],
      },
      counts: { new: 0, renamed: 1, same: 0, protected: 0, kind_set: 0, local_only: 0 },
    };
    expect(view([], plan)).toContain("sobrescreve o que é seu");
  });

  it("with nothing to change, the apply button is disabled and the reason is on the screen", () => {
    const nothing: ImportPlan = {
      ...basePlan,
      items: { category: [{ axis: "category", id: 1, name: "Filmes", status: "same" }], type: [], resolution: [] },
      counts: { new: 0, renamed: 0, same: 1, protected: 0, kind_set: 0, local_only: 0 },
      warnings: [],
    };
    const html = view([], nothing);
    expect(html).toMatch(/disabled=""[^>]*>Aplicar importação/);
    expect(html).toContain("Não há nada a mudar");
  });

  it("with something to apply, the button is enabled", () => {
    expect(view()).not.toMatch(/disabled=""[^>]*>Aplicar importação/);
  });
});

// ---- Description composition in the preview (#192) ------------------------------------

describe("ImportPreviewView — the description composition", () => {
  const fileCfg = { media_blocks: ["poster", "mediainfo"], game_blocks: ["title", "about"], media_info_tag: "spoiler" };
  const mine = { media_blocks: ["synopsis"], game_blocks: [], media_info_tag: "pre" };
  const withDesc = (description: ImportPlan["description"]): ImportPlan => ({
    ...basePlan,
    items: { category: [], type: [], resolution: [] },
    counts: { new: 0, renamed: 0, same: 0, protected: 0, kind_set: 0, local_only: 0 },
    warnings: [],
    description,
  });

  it("without the section in the file, nothing about description appears", () => {
    expect(view([], withDesc(undefined))).not.toContain("Composição da descrição");
  });

  it("shows what the file brings in words, with the order and the tag", () => {
    const html = view([], withDesc({ status: "new", config: fileCfg }));
    expect(html).toContain("Composição da descrição");
    expect(html).toContain("Pôster → MediaInfo");
    expect(html).toContain("Título → Sobre");
    expect(html).toContain("[spoiler]");
    expect(html).toContain("será aplicada");
    // just applying the composition is already a change: the button is enabled
    expect(html).not.toMatch(/disabled=""[^>]*>Aplicar importação/);
  });

  it("the operator's composition is kept, with the option to swap — and theirs appears", () => {
    const item: ImportPlan["description"] = { status: "protected", config: fileCfg, current: mine };
    const html = view([], withDesc(item));
    expect(html).toContain("mantida — você já configurou a composição deste tracker");
    expect(html).toContain("usar a composição de descrição do arquivo");
    expect(html.match(/type="checkbox"/g)?.length).toBe(1);
    expect(html).toContain("A sua hoje: Sinopse · jogos: nenhum bloco · [pre]");
    // without asking, there is nothing to apply
    expect(html).toMatch(/disabled=""[^>]*>Aplicar importação/);

    const marked = view(["description"], withDesc(item));
    expect(marked).toContain("checked");
    expect(marked).not.toMatch(/disabled=""[^>]*>Aplicar importação/);
  });

  it("equal and replaced have their text, without choice", () => {
    const same = view([], withDesc({ status: "same", config: fileCfg }));
    expect(same).toContain("já é a do seu perfil");
    expect(same).not.toContain('type="checkbox"');
    expect(same).toMatch(/disabled=""[^>]*>Aplicar importação/);

    const replaced = view([], withDesc({ status: "replaced", config: fileCfg, current: mine, overwrites_protected: true }));
    expect(replaced).toContain("substitui a que você configurou");
    expect(replaced).not.toContain('type="checkbox"');
  });
});
