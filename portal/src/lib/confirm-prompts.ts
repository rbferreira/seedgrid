import type { ConfirmOptions } from "../components/ConfirmDialog";
import type { TFn } from "../i18n";

// Questions that appear on more than one screen, written once: the same action has to be
// confirmed with the same words everywhere (#45).

export function removeTorrentPrompt(t: TFn, name: string): ConfirmOptions {
  return {
    title: t("torrents.removePrompt.title", { name }),
    message: t("torrents.removePrompt.message"),
    confirmLabel: t("torrents.removePrompt.confirm"),
    tone: "danger",
  };
}

// Receives the full label (nickname and hostname): the confirmation shows the real machine that
// will undergo the action.
export function restartEnginePrompt(t: TFn, nodeLabel: string): ConfirmOptions {
  return {
    title: t("nodes.restartEnginePrompt.title", { name: nodeLabel }),
    message: t("nodes.restartEnginePrompt.message"),
    confirmLabel: t("nodes.restartEnginePrompt.confirm"),
  };
}
