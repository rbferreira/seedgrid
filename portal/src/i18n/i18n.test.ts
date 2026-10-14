import { describe, expect, it } from "vitest";
import { ptBR } from "./pt-BR";
import { en } from "./en";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { detectLang, isPlural, render, rich, translate } from "./index";
import type { Plural } from "./types";

// Recursively traverses a catalog object and returns [path, value][] of the leaves.
function flatten(
  obj: Record<string, unknown>,
  prefix = "",
): [string, string | Plural][] {
  const out: [string, string | Plural][] = [];
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k;
    if (typeof v === "string") {
      out.push([path, v]);
    } else if (isPlural(v)) {
      out.push([path, v]);
    } else if (typeof v === "object" && v !== null) {
      out.push(...flatten(v as Record<string, unknown>, path));
    }
  }
  return out;
}

// Extracts the names of {parameter} and {snippet:…} from a string — both languages must
// use the same ones (the code passes the same nodes to rich, regardless of the language).
function paramNames(text: string): string[] {
  const names: string[] = [];
  const re = /\{(\w+)(?::[^{}]*)?\}/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) names.push(m[1]);
  return names.sort();
}

describe("catalog parity", () => {
  const ptEntries = flatten(ptBR as unknown as Record<string, unknown>);
  const enEntries = flatten(en as unknown as Record<string, unknown>);

  it("en has exactly the same keys as ptBR", () => {
    const ptKeys = ptEntries.map(([k]) => k).sort();
    const enKeys = enEntries.map(([k]) => k).sort();
    expect(enKeys).toEqual(ptKeys);
  });

  it("for each key, the {parameter} names are the same in both languages", () => {
    const enMap = new Map(enEntries);
    for (const [key, ptVal] of ptEntries) {
      const enVal = enMap.get(key)!;
      if (typeof ptVal === "string") {
        expect(paramNames(enVal as string)).toEqual(paramNames(ptVal));
      } else {
        // Plural
        expect(paramNames((enVal as Plural).one)).toEqual(paramNames(ptVal.one));
        expect(paramNames((enVal as Plural).other)).toEqual(paramNames(ptVal.other));
      }
    }
  });

  // The render only understands {name}; an ICU format key ("{count, plural, one {…}}") passes
  // parameter parity and appears raw on the screen. Plural is a { one, other } object.
  it("no leaf uses keys outside {name} and {name:snippet} (no ICU)", () => {
    for (const [key, val] of [...ptEntries, ...enEntries]) {
      for (const text of typeof val === "string" ? [val] : Object.values(val)) {
        expect(text.replace(/\{\w+(?::[^{}]*)?\}/g, ""), key).not.toMatch(/[{}]/);
      }
    }
  });

  // Markup within the phrase appears raw on the screen ("<b>Cluster</b>"). The element enters via
  // rich(), in place of a {marker}.
  it("no leaf contains HTML markup", () => {
    for (const [key, val] of [...ptEntries, ...enEntries]) {
      for (const text of typeof val === "string" ? [val] : Object.values(val)) {
        expect(text, key).not.toMatch(/<\/?[a-z][a-z0-9]*[\s>\/]/i);
      }
    }
  });

  // "(ADR-032)" or "(#112)" means something to whoever reads the code, not to the operator — and
  // an issue number may point to a tracker the reader has no access to. Comments keep theirs.
  it("no leaf cites an ADR or an issue number", () => {
    for (const [key, val] of [...ptEntries, ...enEntries]) {
      for (const text of typeof val === "string" ? [val] : Object.values(val)) {
        expect(text, key).not.toMatch(/ADR-\d+|#\d+/);
      }
    }
  });

  it("every plural leaf has `other` in both languages", () => {
    for (const [key, ptVal] of ptEntries) {
      if (typeof ptVal !== "string") {
        const enVal = new Map(enEntries).get(key)!;
        expect(typeof (enVal as Plural).other).toBe("string");
        expect(typeof ptVal.other).toBe("string");
      }
    }
  });
});

describe("detectLang", () => {
  it("valid saved wins", () => {
    expect(detectLang("pt-BR", ["en-US", "en"])).toBe("pt-BR");
    expect(detectLang("en", ["pt-BR"])).toBe("en");
  });

  it("invalid saved is ignored", () => {
    expect(detectLang("fr", ["en"])).toBe("en");
    expect(detectLang("", ["pt-BR"])).toBe("pt-BR");
  });

  it("[\"pt-PT\",\"en\"] → pt-BR", () => {
    expect(detectLang(null, ["pt-PT", "en"])).toBe("pt-BR");
  });

  it("[\"fr\",\"en-GB\"] → en", () => {
    expect(detectLang(null, ["fr", "en-GB"])).toBe("en");
  });

  it("[\"fr\"] → en", () => {
    expect(detectLang(null, ["fr"])).toBe("en");
  });

  it("empty list → en", () => {
    expect(detectLang(null, [])).toBe("en");
  });
});

describe("render", () => {
  it("interpolates named parameters", () => {
    expect(render("pt-BR", "Olá, {name}: {n} Nodes", { name: "Ana", n: 3 })).toBe("Olá, Ana: 3 Nodes");
    expect(render("en", "{a} and {a}", { a: "x" })).toBe("x and x");
  });

  it("missing parameter remains visible as {name}, without throwing or becoming undefined", () => {
    expect(render("en", "Hello, {name} ({role})", { name: "Ana" })).toBe("Hello, Ana ({role})");
    expect(render("en", "Hello, {name}")).toBe("Hello, {name}");
  });

  it("plural chooses the form by count, in each language", () => {
    const leaf = { one: "{count} Node", other: "{count} Nodes" };
    expect(render("en", leaf, { count: 1 })).toBe("1 Node");
    expect(render("en", leaf, { count: 0 })).toBe("0 Nodes");
    expect(render("en", leaf, { count: 2 })).toBe("2 Nodes");
    // In Portuguese zero is singular ("0 Node"), unlike English: the rule belongs to the
    // language, not the catalog.
    expect(render("pt-BR", leaf, { count: 0 })).toBe("0 Node");
    expect(render("pt-BR", leaf, { count: 5 })).toBe("5 Nodes");
  });

  it("variant object with a single \"other\" key is not plural", () => {
    expect(isPlural({ one: "a", other: "b" })).toBe(true);
    expect(isPlural({ timeout: "a", other: "b" })).toBe(false);
  });

  it("plural without count falls back to a valid number instead of breaking", () => {
    expect(render("en", { one: "um", other: "outros" })).toBe("outros");
  });
});

describe("rich", () => {
  it("places the node in place of the marker, at the position required by the language", () => {
    const html = renderToStaticMarkup(
      createElement("p", null, ...rich("O hostname ({host}) fica", { host: createElement("b", null, "srv-1") })),
    );
    expect(html).toBe("<p>O hostname (<b>srv-1</b>) fica</p>");
  });

  it("marker without a corresponding node remains as text", () => {
    expect(rich("a {x} b", {})).toEqual(["a ", "{x}", " b"]);
  });

  it("{name:snippet} wraps the translated snippet, where the language's sentence places it", () => {
    const strong = (s: string) => createElement("strong", null, s);
    const pt = renderToStaticMarkup(createElement("p", null, ...rich("Aqui você escolhe {s:quais entram}.", { s: strong })));
    const en = renderToStaticMarkup(createElement("p", null, ...rich("Choose {s:which go in} here.", { s: strong })));
    expect(pt).toBe("<p>Aqui você escolhe <strong>quais entram</strong>.</p>");
    expect(en).toBe("<p>Choose <strong>which go in</strong> here.</p>");
  });

  it("{name:snippet} without a corresponding function shows only the snippet, without the markers", () => {
    expect(rich("a {s:b} c", {})).toEqual(["a ", "b", " c"]);
  });

  it("{strong:…}, {em:…} and {code:…} work without the component passing them", () => {
    const html = renderToStaticMarkup(createElement("p", null, ...rich("{strong:a} {em:b} {code:c}")));
    expect(html).toBe("<p><strong>a</strong> <em>b</em> <code>c</code></p>");
  });
});

describe("translate", () => {
  it("returns the phrase for the key in the requested language", () => {
    expect(translate("pt-BR", "login.submit")).toBe("Entrar");
    expect(translate("en", "login.submit")).toBe("Sign in");
    expect(translate("pt-BR", "nav.downloaded")).toBe("Baixados");
    expect(translate("en", "nav.downloaded")).toBe("Downloaded");
  });
});
