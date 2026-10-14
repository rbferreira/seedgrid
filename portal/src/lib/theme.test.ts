import { describe, expect, it } from "vitest";
import { readThemePref, resolveTheme, saveThemePref, systemPrefersDark, THEME_KEY } from "./theme";

function memory(initial: Record<string, string> = {}) {
  const data = { ...initial };
  return {
    data,
    getItem: (k: string) => (k in data ? data[k] : null),
    setItem: (k: string, v: string) => {
      data[k] = v;
    },
  };
}

const blocked = {
  getItem: () => {
    throw new Error("blocked");
  },
  setItem: () => {
    throw new Error("blocked");
  },
};

describe("readThemePref", () => {
  it("nothing saved → system", () => {
    expect(readThemePref(memory())).toBe("system");
  });

  it("a saved choice is kept, including the light/dark saved before system existed", () => {
    expect(readThemePref(memory({ [THEME_KEY]: "dark" }))).toBe("dark");
    expect(readThemePref(memory({ [THEME_KEY]: "light" }))).toBe("light");
    expect(readThemePref(memory({ [THEME_KEY]: "system" }))).toBe("system");
  });

  it("an unknown value is ignored", () => {
    expect(readThemePref(memory({ [THEME_KEY]: "sepia" }))).toBe("system");
  });

  it("blocked storage → system, without throwing", () => {
    expect(readThemePref(blocked)).toBe("system");
    expect(readThemePref(undefined)).toBe("system");
  });
});

describe("saveThemePref", () => {
  it("writes under sg-theme", () => {
    const s = memory();
    saveThemePref("system", s);
    expect(s.data[THEME_KEY]).toBe("system");
  });

  it("blocked storage does not throw", () => {
    expect(() => saveThemePref("dark", blocked)).not.toThrow();
  });
});

describe("resolveTheme", () => {
  it("system follows the operating system", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
  });

  it("an explicit choice ignores the operating system", () => {
    expect(resolveTheme("light", true)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
  });
});

describe("systemPrefersDark", () => {
  const mm = (dark: boolean) => ((q: string) => ({ matches: dark && q === "(prefers-color-scheme: dark)" })) as unknown as typeof window.matchMedia;

  it("reads prefers-color-scheme", () => {
    expect(systemPrefersDark(mm(true))).toBe(true);
    expect(systemPrefersDark(mm(false))).toBe(false);
  });

  it("without matchMedia assumes dark", () => {
    expect(systemPrefersDark(undefined)).toBe(true);
  });
});
