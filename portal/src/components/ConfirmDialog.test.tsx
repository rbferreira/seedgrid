import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { answerConfirm, askConfirm, ConfirmDialog, currentConfirm, type ConfirmOptions } from "./ConfirmDialog";
import { removeTorrentPrompt, restartEnginePrompt } from "../lib/confirm-prompts";
import { setLang, translate, type Lang, type TFn } from "../i18n";

beforeEach(() => setLang("en"));

const tIn =
  (lang: Lang): TFn =>
  (key, params) =>
    translate(lang, key, params);
const t = tIn("en");

const render = (o: ConfirmOptions) =>
  renderToStaticMarkup(<ConfirmDialog {...o} onConfirm={() => {}} onCancel={() => {}} />);

// Answers "no" to whatever a test left pending, so the queue doesn't leak between tests.
afterEach(() => {
  for (let p = currentConfirm(); p; p = currentConfirm()) answerConfirm(p.key, false);
});

describe("ConfirmDialog", () => {
  it("is a modal alertdialog labelled by the title and described by the message", () => {
    const html = render({ title: "Delete “X”?", message: "Deletes the data.", confirmLabel: "Delete" });
    expect(html).toContain('role="alertdialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain('aria-labelledby="confirm-title"');
    expect(html).toContain('aria-describedby="confirm-message"');
    expect(html).toContain("Delete “X”?");
    expect(html).toContain("Deletes the data.");
  });

  it("the button names the action, and the other one is Cancel by default", () => {
    const html = render({ title: "t", confirmLabel: "Restart" });
    expect(html).toContain(">Restart<");
    expect(html).toContain(">Cancel<");
    expect(html).not.toContain(">OK<");
  });

  it("the default Cancel follows the language", () => {
    setLang("pt-BR");
    expect(render({ title: "t", confirmLabel: "Reiniciar" })).toContain(">Cancelar<");
  });

  it("cancelLabel changes the label of the button that does nothing", () => {
    const html = render({ title: "t", confirmLabel: "Cancel the migration", cancelLabel: "Keep" });
    expect(html).toContain(">Keep<");
    expect(html).not.toContain(">Cancel<");
  });

  it("danger paints the button red; the default doesn't", () => {
    expect(render({ title: "t", confirmLabel: "Delete", tone: "danger" })).toContain("bg-red-600");
    expect(render({ title: "t", confirmLabel: "Update" })).not.toContain("bg-red-600");
  });

  it("without a message, aria-describedby points at nothing", () => {
    expect(render({ title: "t", confirmLabel: "Go" })).not.toContain("aria-describedby");
  });
});

describe("askConfirm", () => {
  it("resolves with the answer given to the question", async () => {
    const yes = askConfirm({ title: "a", confirmLabel: "Yes" });
    answerConfirm(currentConfirm()!.key, true);
    expect(await yes).toBe(true);

    const no = askConfirm({ title: "b", confirmLabel: "Yes" });
    answerConfirm(currentConfirm()!.key, false);
    expect(await no).toBe(false);
    expect(currentConfirm()).toBeNull();
  });

  it("two questions in a row queue up — none is left unanswered", async () => {
    const first = askConfirm({ title: "1st", confirmLabel: "Yes" });
    const second = askConfirm({ title: "2nd", confirmLabel: "Yes" });
    expect(currentConfirm()?.title).toBe("1st");
    answerConfirm(currentConfirm()!.key, true);
    expect(currentConfirm()?.title).toBe("2nd");
    answerConfirm(currentConfirm()!.key, false);
    expect(await first).toBe(true);
    expect(await second).toBe(false);
  });

  it("answering with a stale key doesn't answer the next question", () => {
    void askConfirm({ title: "1st", confirmLabel: "Yes" });
    const stale = currentConfirm()!.key;
    answerConfirm(stale, true);
    void askConfirm({ title: "2nd", confirmLabel: "Yes" });
    answerConfirm(stale, true); // a late click on the previous dialog
    expect(currentConfirm()?.title).toBe("2nd");
  });
});

describe("shared prompts", () => {
  it("deleting a torrent is danger and says the data is deleted", () => {
    const p = removeTorrentPrompt(t, "Movie.2024.1080p");
    expect(p.title).toContain("“Movie.2024.1080p”");
    expect(p.message).toContain("deleted");
    expect(p.confirmLabel).toBe("Delete");
    expect(p.tone).toBe("danger");
  });

  it("restarting the Engine names the machine and isn't danger", () => {
    const p = restartEnginePrompt(t, "Living room (seedgrid-node-01)");
    expect(p.title).toContain("“Living room (seedgrid-node-01)”");
    expect(p.confirmLabel).toBe("Restart");
    expect(p.tone).not.toBe("danger");
  });

  it("the prompts come in the operator's language", () => {
    const p = removeTorrentPrompt(tIn("pt-BR"), "Filme.2024.1080p");
    expect(p.title).toBe("Excluir «Filme.2024.1080p»?");
    expect(p.confirmLabel).toBe("Excluir");
  });
});
