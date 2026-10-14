import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ThemeSwitch } from "./ThemeSwitch";
import { setLang } from "../i18n";

beforeEach(() => setLang("en"));

const render = (value: "light" | "system" | "dark") =>
  renderToStaticMarkup(<ThemeSwitch value={value} onChange={() => {}} />);

describe("ThemeSwitch", () => {
  it("is a labelled radiogroup with light, system and dark, in that order", () => {
    const html = render("system");
    expect(html).toContain('role="radiogroup"');
    expect(html).toContain('aria-label="Theme"');
    const labels = [...html.matchAll(/role="radio"[^>]*aria-label="([^"]+)"/g)].map((m) => m[1]);
    expect(labels).toEqual(["Light theme", "System theme", "Dark theme"]);
  });

  it("only the selected option is checked and in the Tab order", () => {
    const html = render("dark");
    const radios = [...html.matchAll(/<button[^>]*>/g)].map((m) => m[0]);
    expect(radios).toHaveLength(3);
    expect(radios.map((r) => r.includes('aria-checked="true"'))).toEqual([false, false, true]);
    expect(radios.map((r) => r.includes('tabindex="0"'))).toEqual([false, false, true]);
  });

  it("follows the Portal language", () => {
    setLang("pt-BR");
    expect(render("light")).toContain('aria-label="Tema claro"');
  });
});
