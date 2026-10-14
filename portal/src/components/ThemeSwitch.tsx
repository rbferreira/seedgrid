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
    <svg aria-hidden="true" fill="none" width="16" height="16" viewBox="0 0 16 16">
      <circle cx="8" cy="8" r="3.5" stroke="currentColor" strokeWidth="1.5" />
      <path
        d="M8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1.06 1.06M11.54 11.54l1.06 1.06M3.4 12.6l1.06-1.06M11.54 4.46l1.06-1.06"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
    </svg>
  );
}

function MonitorIcon() {
  return (
    <svg aria-hidden="true" fill="none" width="16" height="16" viewBox="0 0 16 16">
      <rect x="1.75" y="2.75" width="12.5" height="8.5" rx="1.5" stroke="currentColor" strokeWidth="1.5" />
      <path d="M5.5 14h5M8 11.25V14" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg aria-hidden="true" fill="none" width="16" height="16" viewBox="0 0 16 16">
      <path
        d="M13.5 9.6A5.75 5.75 0 0 1 6.4 2.5a5.75 5.75 0 1 0 7.1 7.1z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
    </svg>
  );
}

const ICON: Record<ThemePref, () => ReactElement> = { light: SunIcon, system: MonitorIcon, dark: MoonIcon };

// ThemeSwitch is the light / system / dark switch, in the same visual language as the Dashboard
// admission switch. A radiogroup: only the selected option is in the Tab order, and the arrow
// keys move the selection (roving tabindex). `wide` stretches it to the container (sidebar);
// without it, it is compact (mobile header).
export function ThemeSwitch({
  value,
  onChange,
  wide = false,
}: {
  value: ThemePref;
  onChange: (pref: ThemePref) => void;
  wide?: boolean;
}) {
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
      className={`${wide ? "flex w-full" : "inline-flex"} overflow-hidden rounded-lg border border-slate-700 text-xs`}
    >
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
            className={`${wide ? "flex-1" : ""} grid place-items-center px-2.5 py-1.5 transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-blue-400 ${
              on ? "bg-blue-600 text-white" : "bg-slate-900 text-slate-400 hover:text-slate-200"
            }`}
          >
            <Icon />
          </button>
        );
      })}
    </div>
  );
}
