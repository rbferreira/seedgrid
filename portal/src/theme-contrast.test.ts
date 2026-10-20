import { describe, expect, it } from "vitest";

// The light theme remaps --color-white to near-black (index.css), because `text-white` is the main
// text — titles, values, inputs. On a SOLID accent background (bg-emerald-600, bg-red-600…) that
// inverts the contrast: the green button gets black text (#47). There the text has to be
// `text-[#fff]`, which isn't remapped. Backgrounds with alpha (bg-red-500/10) and the neutrals
// (slate…) follow the theme and keep `text-white`.
const sources = import.meta.glob(["./**/*.tsx", "!./**/*.test.tsx"], {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const SOLID_ACCENT_BG = /(?<![\w-])bg-(?!slate-|gray-|zinc-|neutral-|stone-)[a-z]+-[5-9]00(?![\d/])/;

function offenders(): string[] {
  const out: string[] = [];
  for (const [file, src] of Object.entries(sources)) {
    src.split("\n").forEach((line, i) => {
      if (/\btext-white\b/.test(line) && SOLID_ACCENT_BG.test(line)) out.push(`${file}:${i + 1}`);
    });
  }
  return out;
}

describe("light theme contrast", () => {
  it("scans the real components", () => {
    expect(Object.keys(sources).length).toBeGreaterThan(20);
  });

  it("a solid accent background doesn't use text-white", () => {
    expect(offenders()).toEqual([]);
  });

  it("the rule catches the solid one and lets alpha and neutral through", () => {
    expect(SOLID_ACCENT_BG.test("bg-emerald-600 hover:bg-emerald-500")).toBe(true);
    expect(SOLID_ACCENT_BG.test("bg-sky-700")).toBe(true);
    expect(SOLID_ACCENT_BG.test("bg-red-500/10")).toBe(false);
    expect(SOLID_ACCENT_BG.test("bg-slate-700")).toBe(false);
    expect(SOLID_ACCENT_BG.test("bg-emerald-400")).toBe(false);
  });
});
