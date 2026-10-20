import type { Migration } from "../api";
import type { Key, TFn } from "../i18n";
import type { ConfirmOptions } from "../components/ConfirmDialog";

// Display rules for migrations (#87), outside the component to be testable.

// Status label in the OPERATOR's language — the internal code (QUEUED, PENDING) is our
// vocabulary, not theirs. PENDING covers the transfer AND the recheck verification at the destination (which
// can take hours, and is where the operator most needs the output), so the label is generic.
const STATUS_LABEL_KEYS: Record<string, Key> = {
  QUEUED: "nodes.migrations.status.queued",
  PENDING: "nodes.migrations.status.pending",
  COMPLETED: "nodes.migrations.status.completed",
  FAILED: "nodes.migrations.status.failed",
};

// Canceled by the operator is FAILED with `cancelled` (the CP acknowledges the reason): it is not a failure
// of the migration, and must not be displayed or painted as one.
export function migrationStatusLabel(
  t: TFn,
  m: Pick<Migration, "status" | "cancelled">,
): string {
  if (m.cancelled) return t("nodes.migrations.status.cancelled");
  const key = STATUS_LABEL_KEYS[m.status];
  return key ? t(key) : m.status;
}

// Only in-flight migrations can be canceled: queued (nothing was touched) or in progress.
export function canCancelMigration(m: Pick<Migration, "status">): boolean {
  return m.status === "QUEUED" || m.status === "PENDING";
}

// Confirmation. Canceling DISCARDS the destination copy — and the in-flight migration
// is precisely saving the torrent's actions — so the operator needs to know what they
// lose and what they DON'T lose (the torrent stays at the source). The action itself is a
// "cancel", so the other button is "Keep", and the two don't share a name (#45).
export function cancelMigrationPrompt(
  t: TFn,
  source: string,
  dest: string,
  status: string,
): ConfirmOptions {
  const migrationPath = `${source || t("nodes.migrations.prompt.originFallback")} → ${dest || t("nodes.migrations.prompt.destFallback")}`;
  const title = t("nodes.migrations.prompt.title", { route: migrationPath });
  const cancelLabel = t("nodes.migrations.prompt.keep");
  if (status === "QUEUED") {
    return {
      title,
      message: t("nodes.migrations.prompt.queuedMessage"),
      confirmLabel: t("nodes.migrations.prompt.confirmQueued"),
      cancelLabel,
    };
  }
  return {
    title,
    message: t("nodes.migrations.prompt.pendingMessage"),
    confirmLabel: t("nodes.migrations.prompt.confirmPending"),
    cancelLabel,
    tone: "danger",
  };
}
