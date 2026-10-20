import { beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { OtherVersionsDialog } from "./OtherVersionsDialog";
import { ApiError, otherVersionsOf, type OtherVersions } from "../api";
import { setLang } from "../i18n";

beforeEach(() => setLang("en"));

const INFO: OtherVersions = {
  versions: [
    { name: "Interstellar.2014.2160p.UHD.BluRay.x265-OTHER", resolution: "2160p", codec: "x265", size_bytes: 40 * 1024 ** 3, node: "node-02" },
    { name: "Interstellar.2014.1080p.BluRay.x264-GRP", resolution: "1080p", codec: "x264", size_bytes: 10 * 1024 ** 3, node: "node-01" },
  ],
  adding: { name: "Interstellar 2014 720p WEB-DL x264", resolution: "720p", codec: "x264", size_bytes: 3 * 1024 ** 3, node: "" },
};

const render = (info = INFO) =>
  renderToStaticMarkup(<OtherVersionsDialog title="Interstellar" info={info} onConfirm={() => {}} onCancel={() => {}} />);

describe("OtherVersionsDialog", () => {
  it("is a modal dialog labelled by its title", () => {
    const html = render();
    expect(html).toContain('role="dialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain('aria-labelledby="other-versions-title"');
  });

  it("shows each cluster version with name, resolution, codec, size and Node", () => {
    const html = render();
    expect(html).toContain("2 versions");
    expect(html).toContain("«Interstellar»");
    for (const s of ["Interstellar.2014.2160p.UHD.BluRay.x265-OTHER", "2160p", "x265", "node-02", "1080p", "node-01"]) {
      expect(html).toContain(s);
    }
  });

  it("shows what is being added, without a Node", () => {
    const html = render();
    expect(html).toContain("You&#x27;re adding");
    expect(html).toContain("Interstellar 2014 720p WEB-DL x264");
  });

  it("one version is singular; without 'adding' the section is hidden", () => {
    const html = render({ versions: [INFO.versions[0]], adding: null });
    expect(html).toContain("one version");
    expect(html).not.toContain("You&#x27;re adding");
  });

  it("offers Cancel and Add anyway — never a 2nd copy of the same file", () => {
    const html = render();
    expect(html).toContain("Cancel");
    expect(html).toContain("Add anyway");
    expect(html).not.toContain("second copy");
  });

  it("speaks Portuguese too", () => {
    setLang("pt-BR");
    const html = render();
    expect(html).toContain("Você já tem este título");
    expect(html).toContain("2 versões");
    expect(html).toContain("Adicionar mesmo assim");
  });
});

describe("otherVersionsOf", () => {
  it("reads the versions of the other_versions 409", () => {
    const e = new ApiError(409, "the cluster already has another version", "other_versions", {
      code: "other_versions",
      versions: INFO.versions,
      adding: INFO.adding,
    });
    expect(otherVersionsOf(e)).toEqual(INFO);
  });

  it("returns null for any other error, including duplicate", () => {
    expect(otherVersionsOf(new ApiError(409, "already exists", "duplicate", { code: "duplicate" }))).toBeNull();
    expect(otherVersionsOf(new ApiError(500, "failure", "internal_error"))).toBeNull();
    expect(otherVersionsOf(new Error("x"))).toBeNull();
  });

  it("copes with a 409 without the list (older Control Plane)", () => {
    expect(otherVersionsOf(new ApiError(409, "x", "other_versions"))).toEqual({ versions: [], adding: null });
  });
});
