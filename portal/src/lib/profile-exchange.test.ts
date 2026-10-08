import { describe, expect, it } from "vitest";
import type { ImageHostSpec, ImportDescription, ImportImageHost, ImportPlan } from "../api";
import {
  DESCRIPTION_KEY,
  IMAGE_HOST_KEY,
  MAX_IMPORT_BYTES,
  checkImportFile,
  descriptionApplies,
  describeChanges,
  describeResult,
  effectiveCounts,
  filenameFromDisposition,
  hasChanges,
  imageHostApplies,
  overwriteKey,
  protectedItems,
  protectedText,
  siteLabel,
} from "./profile-exchange";
import { AXES } from "./taxonomy";
import { translate, type TFn } from "../i18n";

// The assertions check the Portuguese text; the Portal standard is English (ADR-041).
const t: TFn = (key, params) => translate("pt-BR", key, params);

const axes = AXES.map((a) => a.axis);

const plan = (over: Partial<ImportPlan["counts"]> = {}, items: Partial<ImportPlan["items"]> = {}): ImportPlan => ({
  site: { name: "Exemplo", host: "tracker.exemplo.org" },
  family: "unit3d",
  schema_version: 1,
  items: { category: [], type: [], resolution: [], ...items },
  counts: { new: 0, renamed: 0, same: 0, protected: 0, kind_set: 0, local_only: 0, ...over },
  warnings: [],
});

describe("filenameFromDisposition", () => {
  it("reads the name the server emits", () => {
    expect(filenameFromDisposition('attachment; filename="seedgrid-profile-unit3d-blutopia-example.json"')).toBe(
      "seedgrid-profile-unit3d-blutopia-example.json",
    );
  });

  it("never lets an unexpected value become a path", () => {
    for (const h of [
      null,
      "",
      "attachment",
      'attachment; filename="../../etc/passwd"',
      'attachment; filename="a/b.json"',
      'attachment; filename="..\\x.json"',
      'attachment; filename=".bashrc"',
      'attachment; filename="' + "a".repeat(500) + '"',
      'attachment; filename="a b.json"',
    ]) {
      expect(filenameFromDisposition(h)).toBe("seedgrid-profile.json");
    }
  });
});

describe("checkImportFile", () => {
  it("accepts a file of reasonable size", () => {
    expect(checkImportFile(t, { name: "x.json", size: 4096 })).toBe("");
    expect(checkImportFile(t, { name: "x.json", size: MAX_IMPORT_BYTES })).toBe("");
  });

  it("rejects empty and above the limit — before reading and sending", () => {
    expect(checkImportFile(t, { name: "x.json", size: 0 })).toMatch(/vazio/);
    expect(checkImportFile(t, { name: "x.json", size: MAX_IMPORT_BYTES + 1 })).toMatch(/máximo é 512 KiB/);
  });
});

describe("override requests", () => {
  it("the key is the one the server reads in ?overwrite", () => {
    expect(overwriteKey("category", 2)).toBe("category:2");
  });

  it("only what the import would keep because it was manually adjusted can be requested", () => {
    const p = plan(
      { protected: 1, same: 1, new: 1 },
      {
        category: [
          { axis: "category", id: 1, name: "Filmes", status: "same" },
          { axis: "category", id: 2, name: "Series", status: "protected", current_name: "Seriados", current_origin: "manual" },
        ],
        type: [{ axis: "type", id: 3, name: "Encode", status: "new" }],
      },
    );
    expect(protectedItems(p, axes).map((it) => overwriteKey(it.axis, it.id))).toEqual(["category:2"]);
  });

  it("each request moves one term from kept to renamed — without exceeding what exists", () => {
    const p = plan({ protected: 2, renamed: 1 });
    expect(effectiveCounts(p, new Set())).toMatchObject({ protected: 2, renamed: 1 });
    expect(effectiveCounts(p, new Set(["category:2"]))).toMatchObject({ protected: 1, renamed: 2 });
    // one extra request beyond the protected ones (stale state on screen) doesn't make the count go negative
    expect(effectiveCounts(p, new Set(["a:1", "b:2", "c:3"]))).toMatchObject({ protected: 0, renamed: 3 });
  });
});

describe("hasChanges — when there is something to confirm", () => {
  it("only protected and already in place: nothing to apply", () => {
    expect(hasChanges(plan({ same: 3, protected: 1 }), new Set())).toBe(false);
  });

  it("new, renamed or role to fill: there are", () => {
    expect(hasChanges(plan({ new: 1 }), new Set())).toBe(true);
    expect(hasChanges(plan({ renamed: 1 }), new Set())).toBe(true);
    expect(hasChanges(plan({ kind_set: 1 }), new Set())).toBe(true);
  });

  it("requesting to overwrite a protected one means there is something to apply", () => {
    expect(hasChanges(plan({ protected: 1 }), new Set(["category:2"]))).toBe(true);
  });
});

describe("protectedText — who owns the name the import keeps", () => {
  it("manual belongs to the operator; discovered is what their site confirmed; the rest, only 'in the profile'", () => {
    expect(protectedText(t, { current_name: "Seriados", current_origin: "manual" })).toBe("mantido — você chamou de «Seriados»");
    expect(protectedText(t, { current_name: "Series", current_origin: "discovered" })).toBe("mantido — o seu site usa «Series»");
    expect(protectedText(t, { current_name: "X", current_origin: undefined })).toBe("mantido — no perfil: «X»");
  });
});

describe("texts", () => {
  it("describeChanges only says what is not zero, with the correct plural", () => {
    expect(describeChanges(t, plan({ new: 4, renamed: 1, kind_set: 2 }).counts)).toBe("4 novos, 1 renomeado, 2 papéis preenchidos");
    expect(describeChanges(t, plan({ new: 1 }).counts)).toBe("1 novo");
    expect(describeChanges(t, plan().counts)).toBe("nada a mudar");
  });

  it("describeResult counts what the server EXECUTED and what it kept", () => {
    expect(describeResult(t, plan({ new: 4, protected: 1 }))).toBe(
      "Importado: 4 novos. 1 termo foi mantido (ajustado à mão ou já confirmado pelo seu site).",
    );
    expect(describeResult(t, plan({ new: 1, protected: 3 }))).toContain(
      "3 termos foram mantidos (ajustados à mão ou já confirmados pelo seu site)",
    );
    expect(describeResult(t, plan({ same: 5 }))).toMatch(/Nada mudou/);
  });

  it("siteLabel shows the host when the file includes it", () => {
    expect(siteLabel({ name: "Exemplo", host: "tracker.exemplo.org" })).toBe("Exemplo (tracker.exemplo.org)");
    expect(siteLabel({ name: "Exemplo" })).toBe("Exemplo");
  });
});

// ---- Description composition in the import (#192) -----------------------------------------

const cfg = { media_blocks: ["poster", "mediainfo"], game_blocks: ["title"], media_info_tag: "code" };
const withDesc = (status: ImportDescription["status"], over: Partial<ImportPlan["counts"]> = {}): ImportPlan => ({
  ...plan(over),
  description: { status, config: cfg, ...(status === "protected" || status === "replaced" ? { current: { ...cfg, media_info_tag: "pre" } } : {}) },
});

describe("descriptionApplies — would the file composition be applied?", () => {
  it("new and replaced yes; same no; file without the section no", () => {
    expect(descriptionApplies(withDesc("new"), new Set())).toBe(true);
    expect(descriptionApplies(withDesc("replaced"), new Set())).toBe(true);
    expect(descriptionApplies(withDesc("same"), new Set())).toBe(false);
    expect(descriptionApplies(plan({ new: 1 }), new Set())).toBe(false);
  });

  it("the operator's is kept, unless they request it — and requesting is via ?overwrite=description", () => {
    expect(DESCRIPTION_KEY).toBe("description");
    expect(descriptionApplies(withDesc("protected"), new Set())).toBe(false);
    expect(descriptionApplies(withDesc("protected"), new Set([DESCRIPTION_KEY]))).toBe(true);
    // asking to overwrite an unprotected composition changes nothing
    expect(descriptionApplies(withDesc("same"), new Set([DESCRIPTION_KEY]))).toBe(false);
  });
});

describe("the composition does not mix with the term count", () => {
  it("the request to overwrite the description does not remove a term from 'kept'", () => {
    const p = withDesc("protected", { protected: 2 });
    expect(effectiveCounts(p, new Set([DESCRIPTION_KEY]))).toMatchObject({ protected: 2, renamed: 0 });
    expect(effectiveCounts(p, new Set([DESCRIPTION_KEY, "category:2"]))).toMatchObject({ protected: 1, renamed: 1 });
  });

  it("a file that only includes the composition already has something to apply", () => {
    expect(hasChanges(withDesc("new"), new Set())).toBe(true);
    expect(hasChanges(withDesc("protected"), new Set())).toBe(false);
    expect(hasChanges(withDesc("protected"), new Set([DESCRIPTION_KEY]))).toBe(true);
    expect(hasChanges(withDesc("same"), new Set())).toBe(false);
  });
});

describe("describeResult with the composition", () => {
  it("says the composition was applied, with or without terms", () => {
    expect(describeResult(t, withDesc("new", { new: 2 }))).toBe("Importado: 2 novos e a composição da descrição.");
    expect(describeResult(t, withDesc("replaced"))).toBe("Importado: a composição da descrição.");
  });

  it("says the operator's was kept, and does not mistakenly say 'nothing changed' when something changed", () => {
    expect(describeResult(t, withDesc("protected", { new: 1 }))).toBe(
      "Importado: 1 novo. A composição da descrição que você configurou foi mantida.",
    );
    expect(describeResult(t, withDesc("protected"))).toBe(
      "Nada mudou: o perfil já tinha tudo o que o arquivo traz. A composição da descrição que você configurou foi mantida.",
    );
    expect(describeResult(t, withDesc("same"))).toMatch(/Nada mudou/);
  });
});

// ---- The image_host section in the import (#193, ADR-040) ---------------------------------

const hostSpec: ImageHostSpec = {
  name: "MeuHost",
  upload_url: "https://host.exemplo/api/1/upload",
  file_field: "source",
  auth_header: "X-API-Key",
  auth_prefix: "",
  url_path: "image.url",
  thumbnail_path: "image.thumb_url",
};
const withHost = (status: ImportImageHost["status"], over: Partial<ImportPlan["counts"]> = {}): ImportPlan => ({
  ...plan(over),
  image_host: { status, config: hostSpec, ...(status === "protected" || status === "replaced" ? { current: { ...hostSpec, name: "Outro" } } : {}) },
});
// File with both sections — the result sentence must mention both.
const withBoth = (descStatus: ImportDescription["status"], hostStatus: ImportImageHost["status"]): ImportPlan => ({
  ...withDesc(descStatus),
  image_host: withHost(hostStatus).image_host,
});

describe("imageHostApplies — would the file section be applied? (#193)", () => {
  it("same rule as description: new and replaced yes; same no; file without the section no", () => {
    expect(imageHostApplies(withHost("new"), new Set())).toBe(true);
    expect(imageHostApplies(withHost("replaced"), new Set())).toBe(true);
    expect(imageHostApplies(withHost("same"), new Set())).toBe(false);
    expect(imageHostApplies(plan({ new: 1 }), new Set())).toBe(false);
  });

  it("the operator's is kept, unless they request it — and requesting is via ?overwrite=image_host", () => {
    expect(IMAGE_HOST_KEY).toBe("image_host");
    expect(imageHostApplies(withHost("protected"), new Set())).toBe(false);
    expect(imageHostApplies(withHost("protected"), new Set([IMAGE_HOST_KEY]))).toBe(true);
  });

  it("does not mix with the term count or with the description request", () => {
    const p = withHost("protected", { protected: 2 });
    expect(effectiveCounts(p, new Set([IMAGE_HOST_KEY]))).toMatchObject({ protected: 2, renamed: 0 });
    expect(effectiveCounts(p, new Set([DESCRIPTION_KEY, IMAGE_HOST_KEY, "category:2"]))).toMatchObject({ protected: 1, renamed: 1 });
  });

  it("a file that only includes the host already has something to apply", () => {
    expect(hasChanges(withHost("new"), new Set())).toBe(true);
    expect(hasChanges(withHost("protected"), new Set())).toBe(false);
    expect(hasChanges(withHost("protected"), new Set([IMAGE_HOST_KEY]))).toBe(true);
    expect(hasChanges(withHost("same"), new Set())).toBe(false);
  });
});

describe("describeResult with the image host (#193)", () => {
  it("says the host was applied, with or without terms", () => {
    expect(describeResult(t, withHost("new", { new: 2 }))).toBe("Importado: 2 novos e o host de imagem.");
    expect(describeResult(t, withHost("replaced"))).toBe("Importado: o host de imagem.");
  });

  it("both sections applied: a single sentence for both", () => {
    expect(describeResult(t, withBoth("new", "new"))).toBe("Importado: a composição da descrição e o host de imagem.");
    expect(describeResult(t, withBoth("replaced", "new"))).toBe("Importado: a composição da descrição e o host de imagem.");
    expect(describeResult(t, { ...withBoth("new", "replaced"), counts: { ...plan().counts, new: 3 } })).toBe(
      "Importado: 3 novos, a composição da descrição e o host de imagem.",
    );
  });

  it("states that the operator's host was kept", () => {
    expect(describeResult(t, withHost("protected", { new: 1 }))).toBe(
      "Importado: 1 novo. O host de imagem que você configurou foi mantido.",
    );
    expect(describeResult(t, withHost("protected"))).toBe(
      "Nada mudou: o perfil já tinha tudo o que o arquivo traz. O host de imagem que você configurou foi mantido.",
    );
    // both kept: each in its own sentence
    expect(describeResult(t, withBoth("protected", "protected"))).toBe(
      "Nada mudou: o perfil já tinha tudo o que o arquivo traz. A composição da descrição que você configurou foi mantida. O host de imagem que você configurou foi mantido.",
    );
  });
});
