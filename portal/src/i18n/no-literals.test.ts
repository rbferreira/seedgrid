import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// The guard checks the ENTIRE Portal (all .ts/.tsx in src/, excluding tests and the
// catalog itself). PENDING are the files that extraction (ADR-041, #32) hasn't converted yet:
// the list only shrinks — each batch takes its files from here —, and a new file is already
// watched, without anyone needing to remember to register it.
const PENDING = new Set<string>([
]);

// All .ts/.tsx in src/, relative to the Portal root, excluding tests, fixtures, the catalog and
// type files.
function sourceFiles(root: string, dir = "src"): string[] {
  const out: string[] = [];
  for (const e of readdirSync(join(root, dir), { withFileTypes: true })) {
    const rel = `${dir}/${e.name}`;
    if (e.isDirectory()) {
      if (rel !== "src/i18n") out.push(...sourceFiles(root, rel));
    } else if (/\.tsx?$/.test(e.name) && !/\.(test|fixtures)\.tsx?$/.test(e.name) && !e.name.endsWith(".d.ts")) {
      out.push(rel);
    }
  }
  return out.sort();
}

// Removes comments (// for line and /* */ for block) preserving the line count, without
// getting confused by "//" inside strings (URLs).
function stripComments(source: string): string {
  let out = "";
  let i = 0;
  let quote = "";
  while (i < source.length) {
    const c = source[i];
    const two = source.slice(i, i + 2);
    if (quote) {
      out += c;
      if (c === "\\") {
        out += source[i + 1] ?? "";
        i += 2;
        continue;
      }
      if (c === quote) quote = "";
      i++;
    } else if (c === '"' || c === "'" || c === "`") {
      quote = c;
      out += c;
      i++;
    } else if (two === "//") {
      while (i < source.length && source[i] !== "\n") i++;
    } else if (two === "/*") {
      const end = source.indexOf("*/", i + 2);
      const stop = end < 0 ? source.length : end + 2;
      out += source.slice(i, stop).replace(/[^\n]/g, "");
      i = stop;
    } else {
      out += c;
      i++;
    }
  }
  return out;
}

// String with an accented letter, anywhere in the code (not just in JSX): it's Portuguese
// that escaped the catalog — an error message built in a function, a label in an object,
// confirmation text. The catalog is the only place where Portuguese can live.
function findAccentedStrings(source: string): { line: number; text: string }[] {
  const hits: { line: number; text: string }[] = [];
  const lines = stripComments(source).split("\n");
  for (let i = 0; i < lines.length; i++) {
    const re = /(["'`])((?:\\.|(?!\1).)*?[À-ÖØ-öø-ÿ](?:\\.|(?!\1).)*?)\1/g;
    let m: RegExpExecArray | null;
    while ((m = re.exec(lines[i]))) {
      if (!excepted(m[2])) hits.push({ line: i + 1, text: m[2] });
    }
  }
  return hits;
}

// Exceptions: text that is the same in any language, by rule — the product name, the names
// of the languages in their own language, and the product vocabulary and technical acronyms that the
// documentation says NOT to translate (CONTRIBUTING, "Documentation in two languages").
const EXCEPTIONS = [
  "SeedGrid",
  "Português",
  "English",
  "Control Plane",
  "Seed Node",
  "Seed Nodes",
  "Node",
  "Nodes",
  "Agent",
  "Engine",
  "Torrents",
  "Seeding",
  "CPU",
  "API",
];

const LETTERS = /[A-Za-zÀ-ÿ]{2,}/;
// Line that is only prose: letters, spaces, and sentence punctuation — no code characters.
const PROSE_LINE = /^[A-Za-zÀ-ÿ0-9 ,.!?:;'’"%&()\-–—…/]+$/;
const KEYWORD_START =
  /^(return|const|let|var|import|export|type|interface|case|default|else|break|continue|throw|new|await|async|function|if|for|while|try|catch|finally|do|switch|yield|as)\b/;

// Checks if a line is a comment (// or /* or * for JSDoc or {/* for JSX).
function isComment(line: string): boolean {
  const trimmed = line.trim();
  return (
    trimmed.startsWith("//") ||
    trimmed.startsWith("/*") ||
    trimmed.startsWith("*") ||
    trimmed.startsWith("{/*")
  );
}

const excepted = (text: string) => EXCEPTIONS.some((e) => text === e);

// Code operator (&&, ||, ??, comparison, assignment, ": Type" for return annotation,
// ternary "a ? b : c"): text containing it is an expression,
// not prose. The accent guard still catches the literal string inside.
const CODE_OPERATOR = /&&|\|\||\?\?|[=!<>]=|\s=\s|\)\s*:|\s\?\s.*\s:\s/;

// Detects interface text written directly in JSX, in the two ways it appears:
//   1. on the same line as the tags:   <label>User</label>
//   2. on its own line between tags:   <p>
//                                        Loose text
//                                      </p>
// (2) is the most common form and has no tag on the line: only a line rule catches it.
function findJsxTextLiterals(source: string): { line: number; text: string }[] {
  const hits: { line: number; text: string }[] = [];
  const lines = source.split("\n");
  let inBlockComment = false;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    // Multi-line block comment (/* … */, including the {/* … */} from JSX): the
    // middle lines don't start with anything that gives them away.
    if (inBlockComment) {
      if (line.includes("*/")) inBlockComment = false;
      continue;
    }
    if (line.includes("/*") && !line.includes("*/")) {
      inBlockComment = true;
      continue;
    }
    if (isComment(line)) continue;
    // (1) >Text< with 2+ consecutive letters, no braces; the "=>" of an arrow function doesn't count,
    // nor does a comparison ("season >= 1 && season <= 99").
    const re = /(?<!=)>([^{}<>]*)</g;
    let m: RegExpExecArray | null;
    while ((m = re.exec(line))) {
      const text = m[1].trim();
      if (LETTERS.test(text) && !excepted(text) && !CODE_OPERATOR.test(text)) hits.push({ line: i + 1, text });
    }
    // (2) prose-only line, with at least two words, that is not a statement.
    const t = line.trim();
    const words = t.match(/[A-Za-zÀ-ÿ]{2,}/g) ?? [];
    // Function call (identifier attached to the parenthesis) and line ending in ";" are
    // code, even without any other character that gives it away.
    // Ternary branches ("? …" / ": …") or a line that is only
    // a string (CSS classes, for example) are also not prose.
    // Nor an object property ("id: n.id," / "kind: \"node\",") nor a standalone member
    // expression on the line ("item.secret", "plan.counts.protected,", "...plan.counts,").
    const isStatement =
      /[A-Za-z_$]\(/.test(t) ||
      t.endsWith(";") ||
      /^[?:"'`]/.test(t) ||
      /^[A-Za-z_$][\w$]*\??:\s/.test(t) ||
      /^(\.\.\.|!)?[A-Za-z_$][\w$]*(\??\.[A-Za-z_$][\w$]*)+,?$/.test(t) ||
      CODE_OPERATOR.test(t);
    if (PROSE_LINE.test(t) && !KEYWORD_START.test(t) && !isStatement && words.length >= 2 && !excepted(t)) {
      hits.push({ line: i + 1, text: t });
    }
  }
  return hits;
}

// Detects attributes with a literal string containing letters: placeholder, title, aria-label, alt, label.
function findAttrLiterals(source: string): { line: number; attr: string; value: string }[] {
  const hits: { line: number; attr: string; value: string }[] = [];
  const lines = source.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (isComment(line)) continue;
    const attrRe = /(placeholder|title|aria-label|alt|label)=["']([^"']*[A-Za-zÀ-ÿ]{2,}[^"']*)["']/g;
    let m: RegExpExecArray | null;
    while ((m = attrRe.exec(line))) {
      if (!excepted(m[2])) hits.push({ line: i + 1, attr: m[1], value: m[2] });
    }
  }
  return hits;
}

describe("i18n guard: no literals in converted files", () => {
  // Paths are relative to the Portal root (portal/), and this file is in src/i18n/.
  const root = join(__dirname, "..", "..");
  const all = sourceFiles(root);

  it("the pending list only references files that exist", () => {
    // A renamed or deleted file cannot remain exempt with the old name.
    expect([...PENDING].filter((p) => !all.includes(p))).toEqual([]);
  });

  for (const rel of all.filter((f) => !PENDING.has(f))) {
    it(`${rel} has no interface text written inline`, () => {
      const source = readFileSync(join(root, rel), "utf-8");
      const problems = [
        ...findJsxTextLiterals(source).map((h) => `  texto literal na linha ${h.line}: "${h.text}"`),
        ...findAttrLiterals(source).map((h) => `  atributo ${h.attr} literal na linha ${h.line}: "${h.value}"`),
        ...findAccentedStrings(source).map((h) => `  string com acento na linha ${h.line}: "${h.text}"`),
      ];
      expect(problems).toEqual([]);
    });
  }

  it("catches string with accent outside JSX and ignores comments and URLs", () => {
    const src = [
      'const msg = "Falha ao carregar a página";',
      "// comentário com acentuação não conta",
      'const url = "https://example.org/a"; // nem este: é só o comentário que tem ç',
      "/* bloco com",
      "   acentuação */",
      "const ok = `Remoção de ${n} torrents`;",
      'const en = "No accents here";',
    ].join("\n");
    expect(findAccentedStrings(src).map((h) => h.line)).toEqual([1, 6]);
  });

  // The guard is only valid if it catches what it promises: the two forms of loose text and attributes,
  // without flagging code that looks like text.
  it("catches text on the same line, text on its own line, and literal attribute", () => {
    const src = [
      "<label>Usuário</label>",
      "<p>",
      "  Nenhum Node registrado ainda.",
      "</p>",
      "<p>",
      "  Confira o disco (ADR-032) antes de tentar de novo.",
      "</p>",
      '<input placeholder="Digite o nome" />',
    ].join("\n");
    expect(findJsxTextLiterals(src).map((h) => h.line)).toEqual([1, 3, 6]);
    expect(findAttrLiterals(src).map((h) => h.line)).toEqual([8]);
  });

  it("does not flag code, translation keys, or exceptions", () => {
    const src = [
      "const f = (xs: number[]) => xs.length < 2;",
      "return null;",
      '<label>{t("login.username")}</label>',
      "<span>SeedGrid</span>",
      '<option value="en">English</option>',
      "  meReady = true,",
      "  children,",
      "    setOk(false);",
      "      id: n.id,",
      '      kind: "node",',
      "              item.secret",
      "      ...plan.counts,",
      '        (tone ?? "bg-slate-800 text-slate-300 ring-slate-700")',
      "    if (season >= 1 && season <= 99) return { season };",
      "    async (action: () => Promise<unknown>, okMsg: string): Promise<string> => {",
      '            dragOver ? "border-emerald-500 bg-emerald-500/5" : "border-slate-700 hover:border-slate-600"',
      '          ? "bg-emerald-600/15 font-medium text-emerald-300"',
      '          : "text-slate-400 hover:bg-slate-800/60"',
      "{/* comentário de bloco que continua",
      "    na linha seguinte, com cara de frase */}",
      "    clearDefaultCreds(); // some o aviso",
      '<input placeholder={t("login.username")} />',
    ].join("\n");
    expect(findJsxTextLiterals(src)).toEqual([]);
    expect(findAttrLiterals(src)).toEqual([]);
  });
});
