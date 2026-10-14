import { useRef, type KeyboardEvent, type ReactElement } from "react";
import { useT, type Key } from "../i18n";
import { nextThemePref, THEME_PREFS, type ThemePref } from "../lib/theme";

const LABEL: Record<ThemePref, Key> = {
  light: "theme.light",
  system: "theme.system",
  dark: "theme.dark",
};

function SunIcon() {
  return (
    <svg aria-hidden="true" fill="none" width="14" height="14" viewBox="0 0 16 16">
      <circle cx="8" cy="8" r="3.25" stroke="currentColor" strokeWidth="1.5" />
      <path
        d="M8 1.5v1.25M8 13.25v1.25M1.5 8h1.25M13.25 8h1.25M3.4 3.4l.9.9M11.7 11.7l.9.9M3.4 12.6l.9-.9M11.7 4.3l.9-.9"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
    </svg>
  );
}

function MonitorIcon() {
  return (
    <svg aria-hidden="true" fill="none" width="14" height="14" viewBox="0 0 16 16">
      <rect x="2" y="3" width="12" height="8" rx="1.5" stroke="currentColor" strokeWidth="1.5" />
      <path d="M5.5 13.5h5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg aria-hidden="true" fill="none" width="14" height="14" viewBox="0 0 16 16">
      <path
        d="M13.25 9.75A5.5 5.5 0 0 1 6.25 2.75a5.5 5.5 0 1 0 7 7z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
    </svg>
  );
}

const ICON: Record<ThemePref, () => ReactElement> = { light: SunIcon, system: MonitorIcon, dark: MoonIcon };

// Each option is a 24px slot; the knob slides one slot per position.
const KNOB_SHIFT: Record<ThemePref, string> = {
  light: "translate-x-0",
  system: "translate-x-6",
  dark: "translate-x-12",
};

// ThemeSwitch is the light / system / dark switch: a small pill with a knob that slides to the
// chosen option. A radiogroup: only the selected option is in the Tab order, and the arrow keys
// move the selection (roving tabindex); the focus ring goes on the whole pill.
export function ThemeSwitch({ value, onChange }: { value: ThemePref; onChange: (pref: ThemePref) => void }) {
  const t = useT();
  const refs = useRef<Partial<Record<ThemePref, HTMLButtonElement | null>>>({});

  const onKeyDown = (e: KeyboardEvent) => {
    const next = nextThemePref(value, e.key);
    if (!next) return;
    e.preventDefault();
    onChange(next);
    refs.current[next]?.focus();
  };

  return (
    <div
      role="radiogroup"
      aria-label={t("theme.label")}
      onKeyDown={onKeyDown}
      className="relative inline-flex shrink-0 rounded-full border border-slate-700 bg-slate-950 p-0.5 transition-colors has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-emerald-500/70 has-[:focus-visible]:ring-offset-2 has-[:focus-visible]:ring-offset-slate-900"
    >
      <span
        aria-hidden="true"
        className={`pointer-events-none absolute left-1 top-1 h-5 w-5 rounded-full bg-emerald-600 shadow-sm shadow-black/30 transition-transform duration-300 ease-[cubic-bezier(0.65,0,0.35,1)] ${KNOB_SHIFT[value]}`}
      />
      {THEME_PREFS.map((p) => {
        const Icon = ICON[p];
        const on = value === p;
        return (
          <button
            key={p}
            ref={(el) => {
              refs.current[p] = el;
            }}
            type="button"
            role="radio"
            aria-checked={on}
            aria-label={t(LABEL[p])}
            title={t(LABEL[p])}
            tabIndex={on ? 0 : -1}
            onClick={() => onChange(p)}
            className={`relative z-10 grid h-6 w-6 place-items-center rounded-full transition-colors duration-200 focus:outline-none ${
              on ? "text-[#fff]" : "text-slate-500 hover:text-slate-300"
            }`}
          >
            <Icon />
          </button>
        );
      })}
    </div>
  );
}
