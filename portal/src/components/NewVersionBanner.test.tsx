import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { NewVersionBanner, NewVersionBar } from "./NewVersionBanner";
import { setLang } from "../i18n";

beforeEach(() => setLang("en"));

describe("NewVersionBanner", () => {
  it("shows nothing before knowing there's a new version", () => {
    expect(renderToStaticMarkup(<NewVersionBanner />)).toBe("");
  });

  it("the banner warns and offers to reload, as a status for assistive technology", () => {
    const html = renderToStaticMarkup(<NewVersionBar onReload={() => {}} />);
    expect(html).toContain('role="status"');
    expect(html).toContain("There&#x27;s a new version of SeedGrid");
    expect(html).toContain("Reload");
  });

  it("speaks Portuguese too", () => {
    setLang("pt-BR");
    const html = renderToStaticMarkup(<NewVersionBar onReload={() => {}} />);
    expect(html).toContain("Há uma versão nova do SeedGrid");
    expect(html).toContain("Recarregar");
  });
});
