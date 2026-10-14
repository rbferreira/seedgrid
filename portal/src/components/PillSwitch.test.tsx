import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { PillSwitch, type PillOption } from "./PillSwitch";

const MODES: PillOption<"sg" | "manual">[] = [
  { value: "sg", content: "SG" },
  { value: "manual", content: "Manual", tone: "amber" },
];

const render = (value: "sg" | "manual", busy = false) =>
  renderToStaticMarkup(<PillSwitch label="Mode" options={MODES} value={value} onChange={() => {}} busy={busy} />);

describe("PillSwitch", () => {
  it("is a labelled radiogroup with one radio per option", () => {
    const html = render("sg");
    expect(html).toContain('role="radiogroup"');
    expect(html).toContain('aria-label="Mode"');
    expect([...html.matchAll(/role="radio"/g)]).toHaveLength(2);
  });

  it("the knob takes one slot and sits on the selected option", () => {
    expect(render("sg")).toContain("width:calc((100% - 4px) / 2);transform:translateX(0%)");
    expect(render("manual")).toContain("transform:translateX(100%)");
  });

  it("the knob takes the selected option's tone, emerald by default", () => {
    expect(render("sg")).toContain("bg-emerald-600");
    expect(render("manual")).toContain("bg-amber-600");
    expect(render("manual")).not.toContain("bg-emerald-600");
  });

  it("only the selected option is checked and in the Tab order", () => {
    const radios = [...render("manual").matchAll(/<button[^>]*>/g)].map((m) => m[0]);
    expect(radios.map((r) => r.includes('aria-checked="true"'))).toEqual([false, true]);
    expect(radios.map((r) => r.includes('tabindex="0"'))).toEqual([false, true]);
  });

  it("busy stays visible and says so to assistive technology", () => {
    const html = render("sg", true);
    expect(html).toContain('aria-busy="true"');
    expect(html).toContain("SG");
    expect(render("sg")).not.toContain("aria-busy");
  });
});
