import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  clearImageHostKey,
  generateScreenshots,
  renderScreenshots,
  resetImageHostSpec,
  saveImageHostSpec,
  setImageHostKey,
  type ImageHostSpec,
} from "./api";

// The Portal's contract with the Control Plane for the image host (#193, ADR-040): what goes in the
// body (the flat Spec, not wrapped), and that the operator key is passed via a dedicated route.

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

const spec: ImageHostSpec = {
  name: "Host do site",
  upload_url: "https://img.tracker.example.org/api/v1/sharex/upload",
  file_field: "file",
  auth_header: "Authorization",
  auth_prefix: "Bearer ",
  url_path: "url",
  thumbnail_path: "thumbnail_url",
};

describe("saveImageHostSpec", () => {
  it("sends the flat Spec in the body (not wrapped)", async () => {
    stubFetch(() => ({ status: 200, json: { family: "unit3d", schema_version: 1, taxonomy: {} } }));
    await saveImageHostSpec("t1", spec);
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/api/v1/trackers/t1/profile/image-host");
    expect(calls[0].method).toBe("PUT");
    expect(calls[0].body).toEqual(spec);
    expect(calls[0].auth).toBe("Bearer tok");
  });

  it("propagates API error", async () => {
    stubFetch(() => ({
      status: 400,
      json: { error: { code: "invalid_profile", message: "upload_url: invalid URL" } },
    }));
    await expect(saveImageHostSpec("t1", spec)).rejects.toMatchObject({
      status: 400,
      code: "invalid_profile",
    });
  });
});

describe("resetImageHostSpec", () => {
  it("sends DELETE without a body", async () => {
    stubFetch(() => ({ status: 200, json: { family: "unit3d", schema_version: 1, taxonomy: {} } }));
    await resetImageHostSpec("t1");
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/api/v1/trackers/t1/profile/image-host");
    expect(calls[0].method).toBe("DELETE");
    expect(calls[0].body).toBeUndefined();
  });
});

describe("setImageHostKey", () => {
  it("sends the key in the body", async () => {
    stubFetch(() => ({ status: 200, json: { status: "ok" } }));
    await setImageHostKey("t1", "my-key");
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/api/v1/trackers/t1/image-host-key");
    expect(calls[0].method).toBe("PUT");
    expect(calls[0].body).toEqual({ key: "my-key" });
  });

  it("propagates 409 when the profile has no image_host", async () => {
    stubFetch(() => ({
      status: 409,
      json: { error: { code: "profile_without_image_host", message: "configure the image_host section first" } },
    }));
    await expect(setImageHostKey("t1", "key")).rejects.toMatchObject({
      status: 409,
      code: "profile_without_image_host",
    });
  });
});

describe("clearImageHostKey", () => {
  it("sends DELETE without a body", async () => {
    stubFetch(() => ({ status: 200, json: { status: "ok" } }));
    await clearImageHostKey("t1");
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/api/v1/trackers/t1/image-host-key");
    expect(calls[0].method).toBe("DELETE");
    expect(calls[0].body).toBeUndefined();
  });
});

describe("generateScreenshots", () => {
  it("sends optional tracker_id in body", async () => {
    stubFetch(() => ({
      status: 200,
      json: { bbcode: "[img]...[/img]", host: "img.tracker.example.org", failures: [] },
    }));
    const res = await generateScreenshots("tor1", "tracker1");
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/api/v1/torrents/tor1/screenshots");
    expect(calls[0].method).toBe("POST");
    expect(calls[0].body).toEqual({ tracker_id: "tracker1" });
    expect(res.bbcode).toBe("[img]...[/img]");
    expect(res.host).toBe("img.tracker.example.org");
    expect(res.failures).toEqual([]);
  });

  it("sends an empty body when trackerId is omitted", async () => {
    stubFetch(() => ({
      status: 200,
      json: { bbcode: "[img]...[/img]", host: "imgbb", failures: [] },
    }));
    await generateScreenshots("tor1");
    expect(calls).toHaveLength(1);
    expect(calls[0].body).toEqual({});
  });

  it("returns failures when a host fails in the chain", async () => {
    stubFetch(() => ({
      status: 200,
      json: {
        bbcode: "[img]...[/img]",
        host: "imgbb",
        failures: [{ host: "img.tracker.example.org", reason: "HTTP 500" }],
      },
    }));
    const res = await generateScreenshots("tor1", "tracker1");
    expect(res.host).toBe("imgbb");
    expect(res.failures).toHaveLength(1);
    expect(res.failures?.[0]?.host).toBe("img.tracker.example.org");
    expect(res.failures?.[0]?.reason).toBe("HTTP 500");
  });
});

describe("renderScreenshots without host and failures (#193)", () => {
  it("handles a response without the host and failures keys", async () => {
    stubFetch(() => ({
      status: 200,
      json: { bbcode: "[img]...[/img]" },
    }));
    const res = await renderScreenshots("cmd1");
    expect(res.bbcode).toBe("[img]...[/img]");
    expect(res.host).toBeUndefined();
    expect(res.failures).toBeUndefined();
  });
});

describe("renderScreenshots", () => {
  it("sends command_id and optional tracker_id", async () => {
    stubFetch(() => ({
      status: 200,
      json: { bbcode: "[img]...[/img]", host: "imgbb", failures: [] },
    }));
    const res = await renderScreenshots("cmd1", "tracker1");
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("/api/v1/screenshots/render");
    expect(calls[0].method).toBe("POST");
    expect(calls[0].body).toEqual({ command_id: "cmd1", tracker_id: "tracker1" });
    expect(res.bbcode).toBe("[img]...[/img]");
  });

  it("sends only command_id when trackerId is omitted", async () => {
    stubFetch(() => ({
      status: 200,
      json: { bbcode: "[img]...[/img]", host: "imgbb", failures: [] },
    }));
    await renderScreenshots("cmd1");
    expect(calls).toHaveLength(1);
    expect(calls[0].body).toEqual({ command_id: "cmd1" });
  });
});
