import type { ReactElement } from "react";
import { useT, type Key } from "../i18n";
import { THEME_PREFS, type ThemePref } from "../lib/theme";
import { PillSwitch } from "./PillSwitch";

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

// ThemeSwitch is the light / system / dark switch: the Portal's PillSwitch with icons.
export function ThemeSwitch({ value, onChange }: { value: ThemePref; onChange: (pref: ThemePref) => void }) {
  const t = useT();
  return (
    <PillSwitch
      label={t("theme.label")}
      iconOnly
      value={value}
      onChange={onChange}
      options={THEME_PREFS.map((p) => {
        const Icon = ICON[p];
        return { value: p, content: <Icon />, name: t(LABEL[p]) };
      })}
    />
  );
}
