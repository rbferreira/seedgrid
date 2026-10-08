import { describe, expect, it } from "vitest";
import type { DescriptionConfig, TrackerProfile } from "../api";
import {
  BLOCK_HINT_KEY,
  BLOCK_LABEL_KEY,
  MEDIAINFO_TAGS,
  blockLabel,
  describeBlocks,
  describeConfig,
  effectiveConfig,
  fromDraft,
  moveBlock,
  sameConfig,
  toDraft,
  toggleBlock,
} from "./description";
import { descriptionDefault, exampleSite } from "./taxonomy.fixtures";
import { translate, type TFn } from "../i18n";

// The assertions check the Portuguese text; the Portal default is English (ADR-041).
const t: TFn = (key, params) => translate("pt-BR", key, params);

// The description composition editor (#192): what it shows, what counts as a change, and what
// it tells to save. The composition itself (the BBCode) is from the Control Plane and has tests there.

const own: DescriptionConfig = { media_blocks: ["mediainfo", "poster"], game_blocks: ["title", "platforms"], media_info_tag: "code" };
const profileWith = (description?: DescriptionConfig): TrackerProfile => ({ ...exampleSite, ...(description ? { description } : {}) });

describe("the catalog and labels", () => {
  it("every catalog block has a label and a support line — a new id on the server does not appear raw", () => {
    for (const id of [...descriptionDefault.media_blocks, ...descriptionDefault.game_blocks]) {
      expect(BLOCK_LABEL_KEY[id], `rótulo de ${id}`).toBeTruthy();
      expect(BLOCK_HINT_KEY[id], `apoio de ${id}`).toBeTruthy();
    }
  });

  it("the MediaInfo tags are the ones the server accepts", () => {
    expect([...MEDIAINFO_TAGS]).toEqual(["mediainfo", "code", "pre", "spoiler"]);
  });

  it("an unknown id falls back to the id itself", () => {
    expect(blockLabel(t, "poster")).toBe("Pôster");
    expect(blockLabel(t, "novo_bloco")).toBe("novo_bloco");
  });
});

describe("effectiveConfig — what applies to the tracker", () => {
  it("without the section in the profile, the server default", () => {
    expect(effectiveConfig(profileWith(), descriptionDefault)).toBe(descriptionDefault);
    expect(effectiveConfig(undefined, descriptionDefault)).toBe(descriptionDefault);
  });

  it("with the section, the profile one", () => {
    expect(effectiveConfig(profileWith(own), descriptionDefault)).toEqual(own);
  });

  it("list or tag omitted by the server falls back to the default; empty list is empty", () => {
    const partial = { media_info_tag: "pre" } as unknown as DescriptionConfig;
    expect(effectiveConfig(profileWith(partial), descriptionDefault)).toEqual({ ...descriptionDefault, media_info_tag: "pre" });
    const none: DescriptionConfig = { media_blocks: [], game_blocks: [], media_info_tag: "mediainfo" };
    expect(effectiveConfig(profileWith(none), descriptionDefault)).toEqual(none);
  });
});

describe("toDraft e fromDraft", () => {
  it("the default becomes all blocks enabled, in the original order — and round-trips unchanged", () => {
    const d = toDraft(descriptionDefault, descriptionDefault);
    expect(d.media.map((b) => [b.id, b.on])).toEqual([
      ["poster", true],
      ["synopsis", true],
      ["tags", true],
      ["screenshots", true],
      ["mediainfo", true],
    ]);
    expect(fromDraft(d)).toEqual(descriptionDefault);
  });

  it("enabled ones come first, in config order; disabled ones, then, in catalog order", () => {
    const d = toDraft(own, descriptionDefault);
    expect(d.media.map((b) => b.id)).toEqual(["mediainfo", "poster", "synopsis", "tags", "screenshots"]);
    expect(d.media.map((b) => b.on)).toEqual([true, true, false, false, false]);
    expect(d.game.map((b) => b.id)).toEqual(["title", "platforms", "cover", "about", "genres"]);
    expect(d.tag).toBe("code");
    expect(fromDraft(d)).toEqual(own);
  });

  it("the empty list is no enabled blocks — and survives the round trip", () => {
    const none: DescriptionConfig = { media_blocks: [], game_blocks: [], media_info_tag: "mediainfo" };
    const d = toDraft(none, descriptionDefault);
    expect(d.media.every((b) => !b.on)).toBe(true);
    expect(fromDraft(d)).toEqual(none);
  });

  it("id outside the catalog or repeated is neither invented nor duplicated", () => {
    const odd: DescriptionConfig = { media_blocks: ["poster", "poster", "x"], game_blocks: ["y"], media_info_tag: "pre" };
    const d = toDraft(odd, descriptionDefault);
    expect(d.media.filter((b) => b.id === "poster")).toHaveLength(1);
    expect(d.media.map((b) => b.id)).not.toContain("x");
    expect(d.media).toHaveLength(5);
    expect(d.game).toHaveLength(5);
    expect(fromDraft(d).media_blocks).toEqual(["poster"]);
    expect(fromDraft(d).game_blocks).toEqual([]);
  });
});

describe("toggleBlock e moveBlock", () => {
  const list = toDraft(descriptionDefault, descriptionDefault).media;

  it("enabling and disabling do not change the order or the other rows", () => {
    const out = toggleBlock(list, 1);
    expect(out.map((b) => b.id)).toEqual(list.map((b) => b.id));
    expect(out.map((b) => b.on)).toEqual([true, false, true, true, true]);
    expect(list[1].on).toBe(true); // does not mutate the input
  });

  it("moving up and down swaps with the neighbor", () => {
    expect(moveBlock(list, 2, -1).map((b) => b.id)).toEqual(["poster", "tags", "synopsis", "screenshots", "mediainfo"]);
    expect(moveBlock(list, 0, 1).map((b) => b.id)).toEqual(["synopsis", "poster", "tags", "screenshots", "mediainfo"]);
    expect(list.map((b) => b.id)).toEqual(["poster", "synopsis", "tags", "screenshots", "mediainfo"]); // does not mutate
  });

  it("at the ends and out of bounds, nothing changes", () => {
    expect(moveBlock(list, 0, -1)).toBe(list);
    expect(moveBlock(list, list.length - 1, 1)).toBe(list);
    expect(moveBlock(list, 9, -1)).toBe(list);
    expect(moveBlock(list, -1, 1)).toBe(list);
  });

  it("the screen order is the saved order: reordering changes what fromDraft returns", () => {
    const d = toDraft(descriptionDefault, descriptionDefault);
    const moved = { ...d, media: moveBlock(toggleBlock(d.media, 2), 4, -1) };
    expect(fromDraft(moved).media_blocks).toEqual(["poster", "synopsis", "mediainfo", "screenshots"]);
  });
});

describe("sameConfig — what counts as a change", () => {
  it("equal is equal; the order of enabled ones matters; the order of disabled ones does not", () => {
    expect(sameConfig(descriptionDefault, { ...descriptionDefault })).toBe(true);
    expect(sameConfig(own, { ...own, media_blocks: ["poster", "mediainfo"] })).toBe(false);
    expect(sameConfig(own, { ...own, media_info_tag: "pre" })).toBe(false);
    expect(sameConfig(own, { ...own, game_blocks: ["title"] })).toBe(false);
  });

  it("reordering only disabled blocks does not dirty the editor", () => {
    const d = toDraft(own, descriptionDefault);
    const offOnly = { ...d, media: [d.media[0], d.media[1], ...[d.media[4], d.media[3], d.media[2]]] };
    expect(sameConfig(fromDraft(offOnly), own)).toBe(true);
  });
});

describe("describeBlocks e describeConfig", () => {
  it("the order with arrows, and 'no blocks' for the empty list", () => {
    expect(describeBlocks(t, ["poster", "synopsis", "mediainfo"])).toBe("Pôster → Sinopse → MediaInfo");
    expect(describeBlocks(t, [])).toBe("nenhum bloco");
  });

  it("one row per type", () => {
    expect(describeConfig(t, own)).toEqual([
      { kind: "media", text: "MediaInfo → Pôster" },
      { kind: "game", text: "Título → Plataformas" },
    ]);
  });
});
