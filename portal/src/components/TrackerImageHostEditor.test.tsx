import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { TrackerImageHostEditor } from "./TrackerImageHostEditor";
import type { ImageHostSpec, Tracker } from "../api";
import { descriptionDefault, exampleSite } from "../lib/taxonomy.fixtures";
import { setLang } from "../i18n";

// This test checks the Portuguese text: the Portal default is English (ADR-041).
beforeEach(() => setLang("pt-BR"));

// The image host editor (#193, ADR-040): gated by BUTTON — whoever reads the tracker SEES the Spec
// and the key state; only the writer sees the controls (docs/11 - Portal.md §13.1).

const spec: ImageHostSpec = {
  name: "MeuHost",
  upload_url: "https://host.exemplo/api/1/upload",
  file_field: "source",
  auth_header: "X-API-Key",
  auth_prefix: "",
  url_path: "image.url",
  thumbnail_path: "image.thumb_url",
};

const tracker = (imageHost?: ImageHostSpec, opts?: { hasKey?: boolean; keyHost?: string }): Tracker => ({
  id: "t1",
  name: "Exemplo",
  host: "tracker.exemplo.org",
  source: "EXEMPLO",
  api_url: "https://tracker.example",
  has_api_token: true,
  has_image_host_key: opts?.hasKey ?? false,
  image_host_key_host: opts?.keyHost ?? "",
  is_default: true,
  created_at: "2026-10-01T00:00:00Z",
  profile: { ...exampleSite, ...(imageHost ? { image_host: imageHost } : {}) },
  description_default: descriptionDefault,
});

const render = (can: (perm: string) => boolean, t: Tracker = tracker()) =>
  renderToStaticMarkup(<TrackerImageHostEditor tracker={t} can={can} onChanged={() => {}} onLogout={() => {}} />);

const only = (...perms: string[]) => (p: string) => perms.includes(p);

describe("TrackerImageHostEditor — button gating", () => {
  it("users who can write tracker see the seven fields and the Save button", () => {
    const html = render(only("trackers:write"));
    expect(html.match(/<input/g)?.length ?? 0).toBeGreaterThanOrEqual(7);
    expect(html).toContain("Nome");
    expect(html).toContain("URL de upload");
    expect(html).toContain("Campo do arquivo");
    expect(html).toContain("Header de autenticação");
    expect(html).toContain("Prefixo do header");
    expect(html).toContain("Caminho da URL na resposta");
    expect(html).toContain("Caminho da miniatura");
    expect(html).toContain("Salvar host");
  });

  it("read-only users see the Spec as text — the data stays, the controls disappear", () => {
    const html = render(only("trackers:read"), tracker(spec));
    expect(html).toContain("MeuHost");
    expect(html).toContain("https://host.exemplo/api/1/upload");
    expect(html).toContain("X-API-Key");
    expect(html).not.toContain("Salvar host");
    expect(html).not.toContain("Remover host");
    expect(html).not.toContain('type="password"');
  });

  it("with no permission, the section remains visible (button gating)", () => {
    const html = render(only(), tracker(spec));
    expect(html).toContain("MeuHost");
    expect(html).not.toContain("Salvar host");
  });

  it("without the section in the profile and without write permission: says it uses only imgbb", () => {
    const html = render(only("trackers:read"));
    expect(html).toContain("só o imgbb");
  });
});

describe("TrackerImageHostEditor — what the screen shows", () => {
  it("without the section in the profile: 'imgbb only' badge, Save disabled, no Remove", () => {
    const html = render(only("trackers:write"));
    expect(html).toContain("só o imgbb");
    expect(html).not.toContain("configurado");
    // React renderToStaticMarkup can render disabled as an empty attribute
    expect(html).toMatch(/disabled[^>]*>Salvar host/);
    expect(html).not.toContain("Remover host");
    expect(html).not.toContain("alterações não salvas");
  });

  it("with the section in the profile: 'configured' badge, filled fields and Remove button", () => {
    const html = render(only("trackers:write"), tracker(spec));
    expect(html).toContain("configurado");
    expect(html).not.toContain("só o imgbb");
    expect(html).toContain("Remover host");
    expect(html).toContain('value="MeuHost"');
    expect(html).toContain('value="https://host.exemplo/api/1/upload"');
    expect(html).toContain('value="X-API-Key"');
  });

  it("with the section and without a key: password field visible and Register button", () => {
    const html = render(only("trackers:write"), tracker(spec, { hasKey: false }));
    expect(html).toContain("Chave do operador");
    expect(html).toContain('type="password"');
    expect(html).toContain("Cadastrar chave");
    expect(html).not.toContain("Apagar chave");
  });

  it("with a registered key: badge with the host and Delete button", () => {
    const html = render(only("trackers:write"), tracker(spec, { hasKey: true, keyHost: "host.exemplo" }));
    expect(html).toContain("chave cadastrada");
    expect(html).toContain("host.exemplo");
    expect(html).toContain("Apagar chave");
    // the password field appears again to be replaced
    expect(html).toContain('type="password"');
  });

  it("key host different from upload_url: mismatch warning", () => {
    const html = render(only("trackers:write"), tracker(spec, { hasKey: true, keyHost: "outro.exemplo" }));
    expect(html).toContain("o host da chave diverge do upload_url");
    expect(html).toContain("outro.exemplo");
    expect(html).toContain("host.exemplo");
  });

  it("section without thumbnail_path (the server omits empty values): does not count as a change", () => {
    const withoutThumbnail: ImageHostSpec = { ...spec };
    delete withoutThumbnail.thumbnail_path;
    const html = render(only("trackers:write"), tracker(withoutThumbnail));
    expect(html).not.toContain("alterações não salvas");
    expect(html).toMatch(/disabled[^>]*>Salvar host/);
  });

  it("key without a saved host: also a mismatch (strict binding)", () => {
    const html = render(only("trackers:write"), tracker(spec, { hasKey: true, keyHost: "" }));
    expect(html).toContain("o host da chave diverge do upload_url");
  });

  it("key host equal to the saved upload_url: no warning", () => {
    const html = render(only("trackers:write"), tracker(spec, { hasKey: true, keyHost: "host.exemplo" }));
    expect(html).not.toContain("o host da chave diverge");
  });

  it("orphan key: without the section in the profile, with a registered key — shows the badge and the Delete button", () => {
    // The profile doesn't have image_host, but the tracker has a key tied to an old host. The
    // operator needs to be able to delete it (#193, ADR-040).
    const html = render(only("trackers:write"), tracker(undefined, { hasKey: true, keyHost: "antigo.exemplo" }));
    expect(html).toContain("chave cadastrada");
    expect(html).toContain("antigo.exemplo");
    expect(html).toContain("Apagar chave");
    // without a section in the profile, "configured" does not appear
    expect(html).toContain("só o imgbb");
    // without a section, the operator cannot register a new key (the API rejects with 409)
    expect(html).not.toContain("Cadastrar chave");
  });

  it("read-only users see the key badge when it exists (button gating)", () => {
    const html = render(only("trackers:read"), tracker(spec, { hasKey: true, keyHost: "host.exemplo" }));
    expect(html).toContain("Chave cadastrada");
    expect(html).toContain("host.exemplo");
    expect(html).not.toContain("Apagar chave");
    expect(html).not.toContain('type="password"');
  });

  it("read-only user sees the key mismatch warning", () => {
    const html = render(only("trackers:read"), tracker(spec, { hasKey: true, keyHost: "outro.exemplo" }));
    expect(html).toContain("o host da chave diverge do upload_url");
    expect(html).toContain("outro.exemplo");
  });

  it("password field sets autoComplete='new-password' to prevent browser autofill", () => {
    const html = render(only("trackers:write"), tracker(spec));
    expect(html).toMatch(/autoComplete="new-password"/);
  });

  it("sent only to <host>: the DNS name of the saved upload_url", () => {
    const html = render(only("trackers:write"), tracker(spec));
    expect(html).toMatch(/enviada só para <span[^>]*>host\.exemplo<\/span>/);
  });

  it("orphan key without section: says there is no configured host for it to go to", () => {
    const html = render(only("trackers:write"), tracker(undefined, { hasKey: true, keyHost: "antigo.exemplo" }));
    expect(html).toMatch(/enviada só para <span[^>]*>\(nenhum host configurado\)<\/span>/);
  });
});
