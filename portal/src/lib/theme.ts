// Theme preference. The operator picks light, dark or system; <html data-theme> always holds the
// RESOLVED theme ("light" | "dark"), which is what the CSS and the charts (useTheme) read.
// The inline script in index.html repeats readThemePref + resolveTheme before the first paint:
// keep the two in sync.

export type ThemePref = "light" | "system" | "dark";
export type ResolvedTheme = "light" | "dark";

export const THEME_KEY = "sg-theme";
export const THEME_PREFS: readonly ThemePref[] = ["light", "system", "dark"];

const DARK_QUERY = "(prefers-color-scheme: dark)";

function isPref(v: unknown): v is ThemePref {
  return v === "light" || v === "system" || v === "dark";
}

// readThemePref returns the saved choice, or "system" when there is none (or storage is blocked).
export function readThemePref(storage: Pick<Storage, "getItem"> | undefined = safeStorage()): ThemePref {
  try {
    const v = storage?.getItem(THEME_KEY);
    return isPref(v) ? v : "system";
  } catch {
    return "system";
  }
}

export function saveThemePref(pref: ThemePref, storage: Pick<Storage, "setItem"> | undefined = safeStorage()): void {
  try {
    storage?.setItem(THEME_KEY, pref);
  } catch {
    /* storage blocked — the theme still applies, it just does not persist */
  }
}

// systemPrefersDark reads the operating system preference; without matchMedia it assumes dark,
// the Portal's original look.
export function systemPrefersDark(mm: typeof window.matchMedia | undefined = safeMatchMedia()): boolean {
  return mm ? mm(DARK_QUERY).matches : true;
}

export function resolveTheme(pref: ThemePref, prefersDark: boolean): ResolvedTheme {
  if (pref === "system") return prefersDark ? "dark" : "light";
  return pref;
}

// applyTheme writes the resolved theme on <html>.
export function applyTheme(pref: ThemePref): void {
  document.documentElement.dataset.theme = resolveTheme(pref, systemPrefersDark());
}

// watchSystemTheme calls onChange whenever the operating system switches between light and dark.
// Returns the function that stops watching.
export function watchSystemTheme(onChange: () => void): () => void {
  const mm = safeMatchMedia();
  if (!mm) return () => {};
  const q = mm(DARK_QUERY);
  q.addEventListener("change", onChange);
  return () => q.removeEventListener("change", onChange);
}

// nextThemePref is the keyboard behaviour of the switch (radiogroup): arrows move the selection
// and wrap around; any other key changes nothing (null).
export function nextThemePref(current: ThemePref, key: string): ThemePref | null {
  const i = THEME_PREFS.indexOf(current);
  const n = THEME_PREFS.length;
  if (key === "ArrowRight" || key === "ArrowDown") return THEME_PREFS[(i + 1) % n];
  if (key === "ArrowLeft" || key === "ArrowUp") return THEME_PREFS[(i - 1 + n) % n];
  return null;
}

function safeStorage(): Storage | undefined {
  try {
    return typeof localStorage === "undefined" ? undefined : localStorage;
  } catch {
    return undefined;
  }
}

function safeMatchMedia(): typeof window.matchMedia | undefined {
  return typeof window !== "undefined" && typeof window.matchMedia === "function"
    ? window.matchMedia.bind(window)
    : undefined;
}
