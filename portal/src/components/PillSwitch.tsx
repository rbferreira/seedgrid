import { useRef, type KeyboardEvent, type ReactNode } from "react";
import { nextOption } from "../lib/pill-switch";

export type PillOption<T extends string> = {
  value: T;
  // What the option shows: a word or an icon. With an icon, `name` is what screen readers say.
  content: ReactNode;
  name?: string;
  // Knob colour while this option is selected. Default: the brand emerald.
  tone?: "emerald" | "amber";
};

const KNOB_TONE = { emerald: "bg-emerald-600", amber: "bg-amber-600" } as const;

// PillSwitch is the Portal's switch between a few options: a rounded pill with a knob that slides
// to the selected option (theme, cluster admission, per-Node mode). Options have equal widths, so
// the knob moves exactly one slot per position. A radiogroup: only the selected option is in the
// Tab order, the arrow keys move the selection (roving tabindex) and the focus ring goes on the
// whole pill. `busy` keeps it visible but ignores input while a change is being saved.
export function PillSwitch<T extends string>({
  options,
  value,
  onChange,
  label,
  busy = false,
  iconOnly = false,
}: {
  options: readonly PillOption<T>[];
  value: T;
  onChange: (value: T) => void;
  label: string;
  busy?: boolean;
  iconOnly?: boolean;
}) {
  const refs = useRef<Partial<Record<T, HTMLButtonElement | null>>>({});
  const index = Math.max(0, options.findIndex((o) => o.value === value));
  const tone = options[index]?.tone ?? "emerald";

  const choose = (v: T) => {
    if (!busy && v !== value) onChange(v);
  };

  const onKeyDown = (e: KeyboardEvent) => {
    const next = nextOption(
      options.map((o) => o.value),
      value,
      e.key,
    );
    if (!next) return;
    e.preventDefault();
    choose(next);
    refs.current[next]?.focus();
  };

  return (
    <div
      role="radiogroup"
      aria-label={label}
      aria-busy={busy || undefined}
      onKeyDown={onKeyDown}
      className={`relative inline-grid shrink-0 auto-cols-fr grid-flow-col rounded-full border border-slate-700 bg-slate-950 p-0.5 text-xs has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-emerald-500/70 has-[:focus-visible]:ring-offset-2 has-[:focus-visible]:ring-offset-slate-900 ${
        busy ? "opacity-70" : ""
      }`}
    >
      <span
        aria-hidden="true"
        className={`pointer-events-none absolute inset-y-0.5 left-0.5 rounded-full shadow-sm shadow-black/30 transition-[transform,background-color] duration-300 ease-[cubic-bezier(0.65,0,0.35,1)] ${KNOB_TONE[tone]}`}
        style={{ width: `calc((100% - 4px) / ${options.length})`, transform: `translateX(${index * 100}%)` }}
      />
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[o.value] = el;
            }}
            type="button"
            role="radio"
            aria-checked={on}
            aria-label={o.name}
            title={o.name}
            tabIndex={on ? 0 : -1}
            onClick={() => choose(o.value)}
            className={`relative z-10 grid place-items-center rounded-full font-medium transition-colors duration-200 focus:outline-none ${
              iconOnly ? "h-6 w-6" : "px-3 py-1"
            } ${busy ? "cursor-wait" : on ? "cursor-default" : "cursor-pointer"} ${
              // The light theme remaps text-white to near-black; on the coloured knob the text stays white.
              on ? "text-[#fff]" : "text-slate-500 hover:text-slate-300"
            }`}
          >
            {o.content}
          </button>
        );
      })}
    </div>
  );
}
