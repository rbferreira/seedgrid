import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { LANGS, lookup, setLang } from "../i18n";
import { categoryName, choiceLabel, settingText } from "./setting-text";

// Settings contract (ADR-041 §4): the Control Plane catalog (exported to
// shared/settings.json by TestCatalogExported) against the Portal catalogs. New Setting
// without label and description in both languages fails here.

type Spec = { key: string; label: string; description: string; choices?: { value: string; label: string }[] };
type Exported = { categories: string[]; specs: Spec[]; channels: string[]; channel_specs: Spec[] };

const exported: Exported = JSON.parse(readFileSync(join(__dirname, "..", "..", "..", "shared", "settings.json"), "utf-8"));

function leafAt(lang: (typeof LANGS)[number], path: string): unknown {
  return lookup(lang, path);
}

describe("settings contract: shared/settings.json × catalogs", () => {
  afterEach(() => setLang("en"));

  for (const lang of LANGS) {
    it(`${lang}: every setting has a label and a description`, () => {
      for (const s of exported.specs) {
        for (const field of ["label", "description"]) {
          expect(typeof leafAt(lang, `settings.specs.${s.key}.${field}`), `${s.key}.${field}`).toBe("string");
        }
      }
      for (const s of exported.channel_specs) {
        expect(leafAt(lang, `settings.channelSpecs.${s.key}.label`), `${s.key}.label`).toMatch(/\{channel\}/);
        expect(typeof leafAt(lang, `settings.channelSpecs.${s.key}.description`), `${s.key}.description`).toBe("string");
      }
    });

    it(`${lang}: every trail choice and every category has a name`, () => {
      for (const s of exported.specs) {
        const kind = s.key === "notify.events" ? "event" : s.key === "notify.activities" ? "activity" : "";
        for (const c of kind ? (s.choices ?? []) : []) {
          expect(typeof leafAt(lang, `settings.choices.${kind}.${c.value}`), `${kind}.${c.value}`).toBe("string");
        }
      }
      for (const cat of exported.categories) {
        expect(typeof leafAt(lang, `settings.categoryNames.${cat}`), cat).toBe("string");
      }
    });
  }
});

describe("settingText", () => {
  afterEach(() => setLang("en"));

  it("translates by key; the setting per channel uses the template with the channel name", () => {
    // The API text is the English one (#64): the en catalog says the same, pt-BR translates it.
    const ch = exported.channel_specs[0];
    setLang("pt-BR");
    expect(settingText(`notify.discord.${ch.key}`, "label", "x")).toMatch(/^Discord — /);
    setLang("en");
    expect(settingText(`notify.discord.${ch.key}`, "label", "x")).toBe(`Discord — ${ch.label}`);
    expect(settingText(exported.specs[0].key, "label", "x")).toBe(exported.specs[0].label);
  });

  it("setting unknown to Portal shows API text", () => {
    expect(settingText("new.setting", "label", "API Label")).toBe("API Label");
    expect(choiceLabel("notify.events", "NEW_CODE", "da API")).toBe("da API");
    expect(categoryName("new", "New")).toBe("New");
  });
});
