import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { TrackerDescriptionEditor } from "./TrackerDescriptionEditor";
import type { DescriptionConfig, Tracker } from "../api";
import { descriptionDefault, exampleSite } from "../lib/taxonomy.fixtures";
import { setLang } from "../i18n";

// This test checks the Portuguese text: the Portal default is English (ADR-041).
beforeEach(() => setLang("pt-BR"));

// The description composition editor (#192): the gate is by BUTTON — whoever reads the tracker SEES the
// composition, only whoever writes sees the controls (docs/11 - Portal.md §13.1).

const own: DescriptionConfig = { media_blocks: ["mediainfo", "poster"], game_blocks: ["title"], media_info_tag: "spoiler" };

const tracker = (description?: DescriptionConfig): Tracker => ({
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
  profile: { ...exampleSite, ...(description ? { description } : {}) },
  description_default: descriptionDefault,
});

const render = (can: (perm: string) => boolean, t: Tracker = tracker()) =>
  renderToStaticMarkup(<TrackerDescriptionEditor tracker={t} can={can} onChanged={() => {}} onLogout={() => {}} />);

const only = (...perms: string[]) => (p: string) => perms.includes(p);

describe("TrackerDescriptionEditor — button gating", () => {
  it("users who can write tracker see editing: one checkbox per block, the tag and Save", () => {
    const html = render(only("trackers:write"));
    // 5 media blocks + 5 game blocks
    expect(html.match(/type="checkbox"/g)?.length).toBe(10);
    expect(html).toContain("tag do MediaInfo");
    expect(html).toContain("Salvar composição");
    expect(html).toContain("subir Pôster (Filmes e séries)");
    expect(html).toContain("descer Plataformas (Jogos)");
  });

  it("read-only users see the composition as text — the data stays, the control disappears", () => {
    const html = render(only("trackers:read"), tracker(own));
    expect(html).toContain("MediaInfo → Pôster");
    expect(html).toContain("Título");
    expect(html).toContain("[spoiler]");
    expect(html).not.toContain('type="checkbox"');
    expect(html).not.toContain("Salvar composição");
    expect(html).not.toContain("Restaurar o padrão");
    expect(html).not.toContain("<select");
  });

  it("with no permission, the data remains visible (the list route decides who gets here)", () => {
    const html = render(only());
    expect(html).toContain("Pôster → Sinopse → Tags → Screenshots → MediaInfo");
    expect(html).not.toContain("Salvar composição");
  });
});

describe("TrackerDescriptionEditor — what the screen shows", () => {
  it("without the section in the profile: 'default', all enabled, nothing to save and no 'restore'", () => {
    const html = render(only("trackers:write"));
    expect(html).toContain("padrão");
    expect(html).not.toContain("personalizada");
    expect(html.match(/checked=""/g)?.length).toBe(10);
    expect(html).toMatch(/disabled=""[^>]*>Salvar composição/);
    expect(html).not.toContain("Restaurar o padrão");
    expect(html).not.toContain("alterações não salvas");
  });

  it("with the section: 'custom', only its blocks enabled, its tag and the 'restore'", () => {
    const html = render(only("trackers:write"), tracker(own));
    expect(html).toContain("personalizada");
    // mediainfo and poster (media) + title (game)
    expect(html.match(/checked=""/g)?.length).toBe(3);
    expect(html).toMatch(/<option value="spoiler" selected="">\[spoiler\]<\/option>/);
    expect(html).toContain("Restaurar o padrão");
    // the order is from the config: MediaInfo before the Poster
    expect(html.indexOf("incluir MediaInfo (Filmes e séries)")).toBeLessThan(html.indexOf("incluir Pôster (Filmes e séries)"));
  });

  it("the four server tags are the options, and only those", () => {
    const html = render(only("trackers:write"));
    // 4 MediaInfo tags + the 2 content languages (#74).
    expect(html.match(/<option value="/g)?.length).toBe(6);
    for (const t of ["mediainfo", "code", "pre", "spoiler"]) expect(html).toContain(`<option value="${t}"`);
  });

  it("the content language offers English and Brazilian Portuguese, English by default (#74)", () => {
    const html = render(only("trackers:write"));
    expect(html).toContain('<option value="en" selected="">');
    expect(html).toContain('<option value="pt-BR"');
  });

  it("the first row does not move up and the last row does not move down", () => {
    const html = render(only("trackers:write"));
    expect(html).toMatch(/aria-label="subir Pôster \(Filmes e séries\)"[^>]*disabled=""|disabled=""[^>]*aria-label="subir Pôster \(Filmes e séries\)"/);
    expect(html).not.toMatch(/disabled=""[^>]*aria-label="upload Synopsis/);
  });

  it("says that the text remains editable in the publication panel", () => {
    expect(render(only("trackers:write"))).toContain("você continua editando o texto antes de publicar");
  });
});
