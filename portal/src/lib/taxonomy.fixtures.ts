// Tracker profile fixtures for the Portal tests. Not a test: it's what the tests
// of lib/taxonomy and pages/publish/TaxonomySelects share, so that "the example" and
// "another site" are the EXACT same thing in both.

import type { DescriptionConfig, TaxonomyTerm, TrackerProfile } from "../api";

/**
 * The default catalog for description composition (#192), as the server returns it in
 * `description_default` of each tracker: all blocks, in the original order. Mirrors
 * description.DefaultConfig() from the Control Plane (which has a test pinning the list).
 */
export const descriptionDefault: DescriptionConfig = {
  media_blocks: ["poster", "synopsis", "tags", "screenshots", "mediainfo"],
  game_blocks: ["cover", "title", "about", "genres", "platforms"],
  media_info_tag: "mediainfo",
};

export const t = (id: number, name: string, kind?: TaxonomyTerm["kind"], origin: TaxonomyTerm["origin"] = "seed"): TaxonomyTerm => ({
  id,
  name,
  kind,
  origin,
});

/**
 * The taxonomy of an example UNIT3D site (labels in Portuguese, IDs with gaps, like a real
 * site). It is the test template: the panel offers exactly these options, in the profile's
 * order.
 */
export const TAXONOMY_EXAMPLE = {
  category: [
    [1, "Filmes"], [2, "Séries"], [3, "Animes"], [4, "Cursos"], [5, "Games"], [6, "Livros"],
    [7, "HQs e Mangás"], [8, "AudioBooks"], [9, "Programas"], [10, "Materiais de Apoio"],
    [11, "Diversos"], [12, "Músicas"],
  ],
  type: [[1, "Full Disc"], [2, "Remux"], [3, "Encode"], [4, "WEB-DL"], [5, "WEBRip"], [6, "HDTV"]],
  resolution: [[1, "4320p"], [2, "2160p"], [3, "1080p"], [5, "720p"], [8, "480p"], [10, "Outra"]],
} as const;

/** The profile of the example site, with the taxonomy above. */
export const exampleSite: TrackerProfile = {
  family: "unit3d",
  schema_version: 1,
  taxonomy: {
    category: TAXONOMY_EXAMPLE.category.map(([id, name]) =>
      t(id, name, ({ 1: "movie", 2: "tv", 3: "tv", 5: "game" } as Record<number, TaxonomyTerm["kind"]>)[id]),
    ),
    type: TAXONOMY_EXAMPLE.type.map(([id, name]) => t(id, name)),
    resolution: TAXONOMY_EXAMPLE.resolution.map(([id, name]) => t(id, name)),
  },
};

/** Another UNIT3D site: same labels, different IDs — Encode is 12 and 1080p is 2 (measured in upbrr). */
export const otherSite: TrackerProfile = {
  family: "unit3d",
  schema_version: 1,
  taxonomy: {
    category: [t(1, "Movie", "movie"), t(2, "TV", "tv"), t(7, "Game", "game")],
    type: [t(1, "Disc"), t(3, "Remux"), t(8, "WEB-DL"), t(12, "Encode")],
    resolution: [t(2, "1080p"), t(3, "2160p")],
  },
};

export const empty: TrackerProfile = {
  family: "unit3d",
  schema_version: 1,
  taxonomy: { category: [], type: [], resolution: [] },
};

