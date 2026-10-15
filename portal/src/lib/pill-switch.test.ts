import { describe, expect, it } from "vitest";
import { nextOption } from "./pill-switch";

const THEME = ["light", "system", "dark"] as const;

describe("nextOption", () => {
  it("arrows move through the options and wrap around", () => {
    expect(nextOption(THEME, "light", "ArrowRight")).toBe("system");
    expect(nextOption(THEME, "system", "ArrowDown")).toBe("dark");
    expect(nextOption(THEME, "dark", "ArrowRight")).toBe("light");
    expect(nextOption(THEME, "light", "ArrowLeft")).toBe("dark");
    expect(nextOption(THEME, "system", "ArrowUp")).toBe("light");
  });

  it("two options toggle back and forth", () => {
    expect(nextOption(["full_auto", "flex"], "full_auto", "ArrowRight")).toBe("flex");
    expect(nextOption(["full_auto", "flex"], "flex", "ArrowRight")).toBe("full_auto");
  });

  it("other keys change nothing", () => {
    expect(nextOption(THEME, "system", "Enter")).toBeNull();
    expect(nextOption(THEME, "system", "Tab")).toBeNull();
  });

  it("no options → nothing", () => {
    expect(nextOption([], "x", "ArrowRight")).toBeNull();
  });
});
