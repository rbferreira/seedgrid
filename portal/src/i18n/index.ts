import { createElement, Fragment, useSyncExternalStore, type ReactNode } from "react";
import { ptBR } from "./pt-BR";
import { en } from "./en";
import type { Catalog, Plural } from "./types";

export type Lang = "pt-BR" | "en";
export const LANGS: readonly Lang[] = ["pt-BR", "en"] as const;

// detectLang: pure function — valid stored value wins; otherwise the first language
// of the browser that matches (pt, pt-BR, pt-PT → pt-BR; en, en-US... → en); otherwise "en".
export function detectLang(stored: string | null, navigatorLangs: readonly string[]): Lang {
  if (stored === "pt-BR" || stored === "en") return stored;
  for (const tag of navigatorLangs) {
    const base = tag.split("-")[0].toLowerCase();
    if (base === "pt") return "pt-BR";
    if (base === "en") return "en";
  }
  return "en";
}

type Cat = typeof ptBR;
type NestedKeys<T, Prefix extends string = ""> = T extends string
  ? Prefix
  : T extends Plural
    ? Prefix
    : { [K in keyof T & string]: NestedKeys<T[K], Prefix extends "" ? K : `${Prefix}.${K}`> }[keyof T & string];

export type Key = NestedKeys<Cat>;

// Catalog (leaves widened to string), not the literal type of ptBR: it is what both
// catalogs satisfy. The KEYS still come from the ptBR catalog (type Key).
const catalogs: Record<Lang, Catalog> = { "pt-BR": ptBR, en };

// resolveKey: navigates the catalog by path in dot notation.
function resolveKey(cat: Catalog, key: string): string | Plural {
  const parts = key.split(".");
  let node: unknown = cat;
  for (const p of parts) node = (node as Record<string, unknown>)[p];
  return node as string | Plural;
}

// Plural is the object whose keys are ALL categories of Intl.PluralRules (and has `other`).
// A variant object that happens to have an "other" key (the unknown reason for an image
// host) is not plural.
const PLURAL_CATEGORIES = new Set(["zero", "one", "two", "few", "many", "other"]);
export function isPlural(v: unknown): v is Plural {
  return (
    typeof v === "object" &&
    v !== null &&
    "other" in v &&
    Object.keys(v).every((k) => PLURAL_CATEGORIES.has(k))
  );
}

export type Params = Record<string, string | number>;

// render transforms a catalog leaf into the final text. Pure function, separated from
// translate to be tested with any leaves — including the forms that the catalog
// doesn't use yet.
//   - plural: chooses the form via Intl.PluralRules from params.count; a category that
//     the catalog doesn't have (e.g.: "few") falls back to `other`;
//   - {name}: interpolated from params; missing parameter leaves {name} visible, instead of
//     throwing or printing "undefined".
export function render(lang: Lang, leaf: string | Plural, params?: Params): string {
  let text: string;
  if (isPlural(leaf)) {
    const count = typeof params?.count === "number" ? params.count : 0;
    const category = new Intl.PluralRules(lang).select(count);
    text = (leaf as Record<string, string>)[category] ?? leaf.other;
  } else {
    text = leaf;
  }
  if (!params) return text;
  return text.replace(/\{(\w+)\}/g, (_, name: string) => {
    const v = params[name];
    return v === undefined ? `{${name}}` : String(v);
  });
}

// lookup: what is at any path in the catalog (leaf, plural or subtree), without the
// type guarantee of Key — for the phrase built from a code that comes from the API
// (lib/messages). undefined = the catalog doesn't have this path.
export function lookup(lang: Lang, path: string): unknown {
  let node: unknown = catalogs[lang];
  for (const part of path.split(".")) {
    if (typeof node !== "object" || node === null || !Object.prototype.hasOwnProperty.call(node, part)) {
      return undefined;
    }
    node = (node as Record<string, unknown>)[part];
  }
  return node;
}

// translate: the phrase for the key, in the requested language.
export function translate(lang: Lang, key: Key, params?: Params): string {
  return render(lang, resolveKey(catalogs[lang], key), params);
}

// Language state in the module: initializes with detectLang and syncs with localStorage.
const STORAGE_KEY = "sg-lang";

function readStored(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

let currentLang: Lang = detectLang(
  readStored(),
  typeof navigator !== "undefined" ? navigator.languages : [],
);

// The <html lang> follows the language since load (screen readers, hyphenation, the
// browser translator), not just after the first switch.
if (typeof document !== "undefined") {
  document.documentElement.lang = currentLang;
}

type Listener = () => void;
const listeners: Set<Listener> = new Set();

function notify() {
  for (const fn of listeners) fn();
}

export function getLang(): Lang {
  return currentLang;
}

export function setLang(lang: Lang) {
  currentLang = lang;
  try {
    localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    /* localStorage unavailable — the language is only valid for this session */
  }
  if (typeof document !== "undefined") {
    document.documentElement.lang = lang;
  }
  notify();
}

export function subscribe(fn: Listener): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

// useSyncExternalStore needs a snapshot — here it is Lang itself.
const getSnapshot = () => currentLang;

// TFn is the translation function that useT returns. Non-component code (the functions
// in lib/) doesn't call hooks: it RECEIVES a TFn from its caller, and thus follows the
// language switch along with the component that renders it.
export type TFn = (key: Key, params?: Params) => string;

// useT: hook that returns t(key, params?) bound to the current language and subscribes the component.
export function useT(): TFn {
  const lang = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return (key, params) => translate(lang, key, params);
}

// rich: the phrase with markup inside it, in two forms:
//   - {name}: a VALUE with its own style (a hostname in mono font) — the node goes in its place;
//   - {name:fragment}: a SNIPPET of the phrase itself that takes markup (a bold highlight) —
//     the function receives the already translated snippet and wraps it.
// The phrase comes fully translated, with the markers left visible by `t` (without the
// parameter): the position and words are those of the language, not JSX, and the translator sees
// the whole phrase. A marker without a corresponding node stays as text (the snippet, in the ":" form).
// {strong:…}, {em:…} and {code:…} come ready-made (MARKS).
export type RichNodes = Record<string, ReactNode | ((inner: string) => ReactNode)>;

// Emphasis marks that any phrase can use without the code passing them: {strong:…}, {em:…},
// {code:…}. The component only passes what is its own (values with their own style).
const MARKS: RichNodes = {
  strong: (s) => createElement("strong", null, s),
  em: (s) => createElement("em", null, s),
  code: (s) => createElement("code", null, s),
};

export function rich(text: string, nodes: RichNodes = {}): ReactNode[] {
  const all = { ...MARKS, ...nodes };
  return text.split(/(\{\w+(?::[^{}]*)?\})/).map((part, i) => {
    const m = /^\{(\w+)(?::([^{}]*))?\}$/.exec(part);
    if (!m) return part;
    const [, name, inner] = m;
    const node = all[name];
    if (inner !== undefined) {
      return typeof node === "function" ? createElement(Fragment, { key: i }, node(inner)) : inner;
    }
    return name in all && typeof node !== "function" ? createElement(Fragment, { key: i }, node) : part;
  });
}

// useLang: returns [lang, setLang].
export function useLang(): [Lang, (lang: Lang) => void] {
  const lang = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return [lang, setLang];
}

// formatDateTime and formatNumber: just the two, small, with Intl.
export function formatDateTime(lang: Lang, date: Date | string | number): string {
  return new Intl.DateTimeFormat(lang, { dateStyle: "medium", timeStyle: "short" }).format(
    typeof date === "string" || typeof date === "number" ? new Date(date) : date,
  );
}

export function formatNumber(lang: Lang, n: number): string {
  return new Intl.NumberFormat(lang).format(n);
}
