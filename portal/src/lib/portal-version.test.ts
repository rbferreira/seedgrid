import { describe, expect, it } from "vitest";
import { bundleOf, isStale } from "./portal-version";

// The real index.html Vite's build emits (#39): the entry bundle goes in a type=module <script> in
// the <head>, with a hash in its name.
const INDEX = `<!doctype html>
<html lang="en" class="dark">
  <head>
    <meta charset="UTF-8" />
    <title>SeedGrid</title>
    <script type="module" crossorigin src="/assets/index-BVYM_IQ-.js"></script>
    <link rel="stylesheet" crossorigin href="/assets/index-B2kd_9Qz.css">
  </head>
  <body><div id="root"></div></body>
</html>`;

describe("bundleOf", () => {
  it("finds the entry bundle in index.html, not the CSS", () => {
    expect(bundleOf(INDEX)).toBe("/assets/index-BVYM_IQ-.js");
  });

  it("accepts the absolute URL of the loaded <script>", () => {
    expect(bundleOf("https://controller.example/assets/index-BVYM_IQ-.js")).toBe("/assets/index-BVYM_IQ-.js");
  });

  it("in dev (Vite serves /src/main.tsx) there is no bundle", () => {
    expect(bundleOf('<script type="module" src="/src/main.tsx"></script>')).toBeNull();
  });

  it("a proxy error page or an empty body has no bundle", () => {
    expect(bundleOf("<html><body>502 Bad Gateway</body></html>")).toBeNull();
    expect(bundleOf("")).toBeNull();
    expect(bundleOf(null)).toBeNull();
  });
});

describe("isStale", () => {
  it("a published bundle different from the loaded one = a new version", () => {
    expect(isStale("/assets/index-AAA.js", "/assets/index-BBB.js")).toBe(true);
  });

  it("the same bundle doesn't ask for a reload", () => {
    expect(isStale("/assets/index-AAA.js", "/assets/index-AAA.js")).toBe(false);
  });

  it("without one of the sides it claims nothing (network down, dev)", () => {
    expect(isStale("/assets/index-AAA.js", null)).toBe(false);
    expect(isStale(null, "/assets/index-BBB.js")).toBe(false);
  });
});
