import { describe, expect, it } from "vitest";
import type { ProposalItem } from "../api";
import {
  buildApplyItems,
  canPublish,
  defaultCategory,
  defaultSelection,
  findByLabel,
  isApplicable,
  kindOfCategory,
  normalizeLabel,
  prefillFromParsed,
  proposalCounts,
  proposalKey,
  termOptions,
} from "./taxonomy";
import { TAXONOMY_EXAMPLE, otherSite, exampleSite, t, empty } from "./taxonomy.fixtures";

describe("termOptions", () => {
  it("offers exactly the profile terms, in their order", () => {
    for (const axis of ["category", "type", "resolution"] as const) {
      const got = termOptions(exampleSite, axis).map((o) => [o.id, o.label]);
      expect(got).toEqual(TAXONOMY_EXAMPLE[axis].map(([id, name]) => [id, name]));
    }
  });

  it("another site offers its options", () => {
    expect(termOptions(otherSite, "type").map((o) => o.id)).toEqual([1, 3, 8, 12]);
  });

  it("missing or empty profile has no options — and doesn't break", () => {
    expect(termOptions(undefined, "category")).toEqual([]);
    expect(termOptions(empty, "type")).toEqual([]);
  });
});

describe("canPublish", () => {
  it("requires category and type; resolution is optional in UNIT3D", () => {
    expect(canPublish(exampleSite)).toBe(true);
    expect(canPublish(empty)).toBe(false);
    expect(canPublish(undefined)).toBe(false);
    expect(canPublish({ ...empty, taxonomy: { ...empty.taxonomy, category: [t(1, "x")] } })).toBe(false);
    expect(
      canPublish({ ...empty, taxonomy: { category: [t(1, "x")], type: [t(1, "y")], resolution: [] } }),
    ).toBe(true);
  });
});

describe("kindOfCategory — the role comes from the profile, not the number", () => {
  it("in the example, series and anime are tv and category 5 is game", () => {
    expect(kindOfCategory(exampleSite, 2)).toBe("tv");
    expect(kindOfCategory(exampleSite, 3)).toBe("tv");
    expect(kindOfCategory(exampleSite, 5)).toBe("game");
    expect(kindOfCategory(exampleSite, 1)).toBe("movie");
  });

  it("category without a role doesn't get special behavior", () => {
    expect(kindOfCategory(exampleSite, 4)).toBeUndefined(); // Courses
    expect(kindOfCategory(exampleSite, 999)).toBeUndefined();
  });

  it("in another site, category 5 is NOT a game — it was the hardcoded number that was wrong here", () => {
    // Before: isGame = category_id === 5. On a site where 5 is something else, it would open the
    // IGDB search for a non-game category, and hide the season/episode of a series.
    expect(kindOfCategory(otherSite, 5)).toBeUndefined();
    expect(kindOfCategory(otherSite, 7)).toBe("game");
    expect(kindOfCategory(otherSite, 2)).toBe("tv");
  });
});

describe("defaultCategory", () => {
  it("starts with movie; without movie role, on the first one available", () => {
    expect(defaultCategory(exampleSite)).toBe(1);
    expect(defaultCategory({ ...empty, taxonomy: { ...empty.taxonomy, category: [t(9, "x"), t(4, "y")] } })).toBe(9);
    expect(defaultCategory(empty)).toBe(0);
    expect(defaultCategory(undefined)).toBe(0);
  });
});

describe("normalizeLabel / findByLabel", () => {
  it("ignores case, accents, and repeated spaces", () => {
    expect(normalizeLabel("  Séries   HD ")).toBe("series hd");
    expect(findByLabel(exampleSite.taxonomy.type, "web-dl")?.id).toBe(4);
    expect(findByLabel(exampleSite.taxonomy.category, "series")?.id).toBe(2);
  });

  it("empty label matches nothing", () => {
    expect(findByLabel(exampleSite.taxonomy.type, "")).toBeUndefined();
    expect(findByLabel(exampleSite.taxonomy.type, "   ")).toBeUndefined();
  });

  it("doesn't match by approximation — 1080 is not 1080p", () => {
    expect(findByLabel(exampleSite.taxonomy.resolution, "1080")).toBeUndefined();
  });

  it("ambiguous label doesn't pre-fill: two ids for the same name, neither is chosen", () => {
    // an imported term (id 1) that duplicates the name of one the operator already had (id 2)
    const terms = [t(1, "Séries", "tv", "imported"), t(2, "Series", "tv", "manual"), t(3, "Filmes", "movie")];
    expect(findByLabel(terms, "series")).toBeUndefined();
    expect(findByLabel(terms, "SÉRIES")).toBeUndefined();
    // the unambiguous ones keep matching
    expect(findByLabel(terms, "filmes")?.id).toBe(3);
  });

  it("prefill doesn't choose an ambiguous type — stays 0, and the operator selects", () => {
    const profile = {
      ...exampleSite,
      taxonomy: { ...exampleSite.taxonomy, type: [t(1, "Encode", undefined, "imported"), t(3, "encode", undefined, "manual")] },
    };
    const got = prefillFromParsed(profile, { is_tv: false, type: "Encode", resolution: "1080p" });
    expect(got.type_id).toBe(0);
    expect(got.resolution_id).toBe(3);
  });

  // Parity with the server: trackerprofile.NormalizeLabel must produce the same result.
  it("normalizes like the server (NFD, no accents, lowercase, collapsed spaces)", () => {
    expect(normalizeLabel("HQs e Mangás")).toBe("hqs e mangas");
    expect(normalizeLabel("  AÇÃO   &  Aventura ")).toBe("acao & aventura");
    expect(normalizeLabel("Músicas")).toBe("musicas");
  });
});

describe("prefillFromParsed", () => {
  it("example: parser labels fall into the site's IDs", () => {
    const cases: [{ is_tv: boolean; type: string; resolution: string }, [number, number, number]][] = [
      [{ is_tv: false, type: "Encode", resolution: "1080p" }, [1, 3, 3]],
      [{ is_tv: false, type: "WEB-DL", resolution: "2160p" }, [1, 4, 2]],
      [{ is_tv: true, type: "WEB-DL", resolution: "720p" }, [2, 4, 5]],
      [{ is_tv: false, type: "Remux", resolution: "4320p" }, [1, 2, 1]],
      [{ is_tv: true, type: "HDTV", resolution: "480p" }, [2, 6, 8]],
      [{ is_tv: false, type: "WEBRip", resolution: "1080p" }, [1, 5, 3]],
    ];
    for (const [parsed, [cat, typ, res]] of cases) {
      expect(prefillFromParsed(exampleSite, parsed)).toEqual({ category_id: cat, type_id: typ, resolution_id: res });
    }
  });

  it("another site: translates by label, doesn't reuse a site's number", () => {
    // The parser says "Encode 1080p" (in the example: 3 and 3). Here Encode is 12 and 1080p is 2 —
    // reusing the numbers would choose "Remux" and "2160p" without any warning.
    expect(prefillFromParsed(otherSite, { is_tv: false, type: "Encode", resolution: "1080p" })).toEqual({
      category_id: 1,
      type_id: 12,
      resolution_id: 2,
    });
    expect(prefillFromParsed(otherSite, { is_tv: true, type: "WEB-DL", resolution: "2160p" })).toEqual({
      category_id: 2,
      type_id: 8,
      resolution_id: 3,
    });
  });

  it("what doesn't match stays at 0 — the operator chooses, no one chooses for them", () => {
    expect(prefillFromParsed(otherSite, { is_tv: false, type: "BluRay", resolution: "576p" })).toEqual({
      category_id: 1,
      type_id: 0,
      resolution_id: 0,
    });
    expect(prefillFromParsed(empty, { is_tv: true, type: "WEB-DL", resolution: "1080p" })).toEqual({
      category_id: 0,
      type_id: 0,
      resolution_id: 0,
    });
    expect(prefillFromParsed(undefined, { is_tv: false, type: "", resolution: "" }).category_id).toBe(0);
  });
});

describe("discovery proposal", () => {
  const newItem: ProposalItem = { id: 50, name: "PC", seen: 3, status: "new" };
  const equal: ProposalItem = { id: 1, name: "Filmes", seen: 50, status: "same" };
  const renamable: ProposalItem = {
    id: 10,
    name: "Other",
    seen: 1,
    status: "renamed",
    current_name: "Outra",
    current_origin: "seed",
  };
  const protectedItem: ProposalItem = {
    id: 2,
    name: "Series",
    seen: 78,
    status: "renamed",
    current_name: "Seriados",
    current_origin: "manual",
    protected: true,
  };
  const proposal = { category: [equal, protectedItem], type: [newItem], resolution: [renamable] };

  it("only what is NEW comes checked — renaming is opt-in", () => {
    expect([...defaultSelection(proposal)]).toEqual([proposalKey("type", 50)]);
  });

  it("only 'new' and 'renameable' are applicable; 'already in profile' and 'protected' wouldn't change anything", () => {
    expect(isApplicable(newItem)).toBe(true);
    expect(isApplicable(renamable)).toBe(true);
    expect(isApplicable(equal)).toBe(false);
    expect(isApplicable(protectedItem)).toBe(false);
  });

  it("builds the apply items; non-applicable item doesn't go even if checked", () => {
    const todos = new Set([
      proposalKey("category", 1),
      proposalKey("category", 2),
      proposalKey("type", 50),
      proposalKey("resolution", 10),
    ]);
    expect(buildApplyItems(proposal, todos, {})).toEqual([
      { axis: "type", id: 50, name: "PC", origin: "discovered" },
      { axis: "resolution", id: 10, name: "Other", origin: "discovered" },
    ]);
  });

  it("editing the name in the proposal makes it MANUAL — it's the operator's name, not the site's", () => {
    const sel = new Set([proposalKey("type", 50)]);
    expect(buildApplyItems(proposal, sel, { [proposalKey("type", 50)]: "  Computer  " })).toEqual([
      { axis: "type", id: 50, name: "Computer", origin: "manual" },
    ]);
    // Editing back to the site name is not an edit.
    expect(buildApplyItems(proposal, sel, { [proposalKey("type", 50)]: "PC" })[0].origin).toBe("discovered");
  });

  it("nothing checked, nothing to apply", () => {
    expect(buildApplyItems(proposal, new Set(), {})).toEqual([]);
  });

  it("counts by status", () => {
    expect(proposalCounts(proposal)).toEqual({ new: 1, same: 1, renamed: 2 });
  });
});
