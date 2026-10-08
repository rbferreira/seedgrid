import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  composeGameDescription,
  composeMediaDescription,
  resetDescriptionConfig,
  saveDescriptionConfig,
  type GameCandidate,
} from "./api";

// The Portal contract with the Control Plane for composing the description (#192): what goes in the
// body (the names the server reads), and that a FAILURE reaches the caller — this is how the publishing
// panel leaves the description field as it was instead of clearing it.

type Call = { url: string; method?: string; body?: unknown; auth?: string };
let calls: Call[];

function stubFetch(respond: (url: string) => { status: number; json?: unknown }) {
  calls = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({
        url,
        method: init?.method,
        body: init?.body ? JSON.parse(String(init.body)) : undefined,
        auth: (init?.headers as Record<string, string> | undefined)?.Authorization,
      });
      const r = respond(url);
      return { ok: r.status >= 200 && r.status < 300, status: r.status, json: async () => r.json } as Response;
    }),
  );
}

beforeEach(() => {
  const store: Record<string, string> = { "seedgrid_token": "tok" };
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => store[k] ?? null,
    setItem: (k: string, v: string) => void (store[k] = v),
    removeItem: (k: string) => void delete store[k],
  });
});
afterEach(() => vi.unstubAllGlobals());

const game: GameCandidate = {
  igdb_id: 7,
  name: "Elden Ring",
  year: 2022,
  summary: "An RPG.",
  cover_url: "https://images.igdb.com/c.jpg",
  genres: ["RPG"],
  platforms: ["PC"],
};

describe("composeMediaDescription", () => {
  it("calls POST /publish/description with the tracker, the kind and the fields the server reads", async () => {
    stubFetch(() => ({ status: 200, json: { description: "TEXT" } }));
    const media = { poster: "https://p", overview: "S", keywords: "k", screenshots: "[b]x[/b]", mediainfo: "General" };
    await expect(composeMediaDescription("t1", media)).resolves.toBe("TEXT");
    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({ url: "/api/v1/publish/description", method: "POST", auth: "Bearer tok" });
    expect(calls[0].body).toEqual({ tracker_id: "t1", kind: "media", media });
  });

  it("a failure reaches the caller, with the server message", async () => {
    stubFetch(() => ({ status: 404, json: { error: { code: "not_found", message: "tracker not found" } } }));
    const err = await composeMediaDescription("t1", { poster: "", overview: "", keywords: "", screenshots: "", mediainfo: "" }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(404);
    expect(err.message).toBe("tracker not found");
  });
});

describe("composeGameDescription", () => {
  it("sends only the game fields (without igdb_id), with the names the server reads", async () => {
    stubFetch(() => ({ status: 200, json: { description: "GAME" } }));
    await expect(composeGameDescription("t1", game)).resolves.toBe("GAME");
    expect(calls[0].body).toEqual({
      tracker_id: "t1",
      kind: "game",
      game: { cover_url: "https://images.igdb.com/c.jpg", name: "Elden Ring", year: 2022, summary: "An RPG.", genres: ["RPG"], platforms: ["PC"] },
    });
  });

  it("null genres and platforms (as returned by IGDB) become empty lists — the server rejects the wrong type", async () => {
    stubFetch(() => ({ status: 200, json: { description: "" } }));
    await composeGameDescription("t1", { ...game, genres: null, platforms: null });
    expect(calls[0].body).toMatchObject({ game: { genres: [], platforms: [] } });
  });

  it("a server failure reaches the caller", async () => {
    stubFetch(() => ({ status: 500, json: { error: { code: "internal_error", message: "failure" } } }));
    await expect(composeGameDescription("t1", game)).rejects.toMatchObject({ status: 500 });
  });
});

describe("the description section of the profile", () => {
  it("PUT saves the config as received; DELETE restores it", async () => {
    stubFetch(() => ({ status: 200, json: { family: "unit3d", schema_version: 1, taxonomy: {} } }));
    const cfg = { media_blocks: ["poster"], game_blocks: [], media_info_tag: "pre" };
    await saveDescriptionConfig("t1", cfg);
    await resetDescriptionConfig("t1");
    expect(calls[0]).toMatchObject({ url: "/api/v1/trackers/t1/profile/description", method: "PUT", body: cfg });
    expect(calls[1]).toMatchObject({ url: "/api/v1/trackers/t1/profile/description", method: "DELETE" });
  });

  it("the server validation rejection (400 with the field) reaches the editor intact", async () => {
    stubFetch(() => ({ status: 400, json: { error: { code: "invalid_profile", message: "description.media_blocks[1]: bloco \"x\" desconhecido" } } }));
    const err = await saveDescriptionConfig("t1", { media_blocks: ["poster", "x"], game_blocks: [], media_info_tag: "mediainfo" }).catch((e) => e);
    expect(err.status).toBe(400);
    expect(err.message).toContain("description.media_blocks[1]");
  });
});
