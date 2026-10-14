import { useState } from "react";
import { drainNode, migrateTorrent, type Node } from "../api";
import { startMigration } from "../actions";
import { hasNickname, nodeLabelById, nodeLabel } from "../lib/node-label";
import { shortId } from "./ui";
import { useT } from "../i18n";

/**
 * MigrateTarget describes what will be migrated when the operator chooses the destination.
 *
 * `name` comes from the caller (the torrent name, or the source Node name during drain):
 * whoever opens the picker already has the object at hand, so it makes no sense for the picker to search
 * again in the list — and searching was fragile, because the list might have changed between opening
 * the modal and confirming.
 */
export type MigrateTarget = {
  kind: "torrent" | "node";
  id: string;
  name: string;
  sourceNodeId: string;
  label: string;
};

/**
 * useMigrateTarget stores the chosen target and triggers the migration — a single torrent
 * (ADR-018) or the entire Node drain (ADR-031), which is a batch of migrations.
 *
 * It uses `startMigration` (not `startAction`) because migration is not an instantaneous
 * command: it's a multi-step process, and the toast tracks the PROGRESS until the
 * end. The hook doesn't return JSX — the UI is the `<MigratePicker/>` below, for the caller
 * to decide where the modal goes in the tree.
 */
export function useMigrateTarget({ nodes, reload }: { nodes: Node[]; reload: () => void }) {
  const t = useT();
  const [target, setTarget] = useState<MigrateTarget | null>(null);

  const confirm = (destId: string) => {
    if (!target) return;
    const tgt = target;
    setTarget(null);
    const dest = nodeLabelById(nodes, destId) || shortId(destId);
    if (tgt.kind === "torrent") {
      startMigration({
        id: tgt.id,
        label: t("nodes.migratePicker.toastMigrate", { name: tgt.name, dest }),
        run: async () => [(await migrateTorrent(tgt.id, destId)).id],
        onSettled: reload,
      });
    } else {
      startMigration({
        id: tgt.id,
        label: t("nodes.migratePicker.toastDrain", { name: tgt.name, dest }),
        run: async () => (await drainNode(tgt.id, destId)).data.map((m) => m.id),
        onSettled: reload,
      });
    }
  };

  return { target, setTarget, confirm };
}

/**
 * MigratePicker: destination Node selection modal. It only offers ONLINE Nodes different
 * from the source — migrating to an offline Node would queue a pull that no one executes — and
 * shows the health of each candidate, which is the practical selection criteria.
 *
 * It doesn't gate permissions: the button that opens the picker does the gating.
 */
export function MigratePicker({
  target,
  nodes,
  onPick,
  onClose,
}: {
  target: MigrateTarget | null;
  nodes: Node[];
  onPick: (destNodeId: string) => void;
  onClose: () => void;
}) {
  const t = useT();
  if (!target) return null;
  const candidates = nodes.filter((n) => n.id !== target.sourceNodeId && n.status === "ONLINE");
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <div
        className="w-full max-w-md rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="text-sm font-semibold text-white">{t("nodes.migratePicker.title")}</h2>
        <p className="mt-1 text-xs text-slate-400">{target.label}</p>
        <div className="mt-4 space-y-2">
          {candidates.map((n) => (
            <button
              key={n.id}
              onClick={() => onPick(n.id)}
              className="flex w-full items-center justify-between rounded-lg border border-slate-700 px-3 py-2 text-left text-sm text-white transition hover:bg-slate-800"
            >
              {/* The destination is chosen by the name the operator gave; the hostname goes along
                  when it differs, because migrating to the wrong Node costs time and disk. */}
              <span>
                {nodeLabel(n)}
                {hasNickname(n) && <span className="ml-2 font-mono text-xs text-slate-500">{n.hostname}</span>}
              </span>
              <span className="text-xs text-slate-400">{t("nodes.migratePicker.healthScore", { score: n.health_score ?? "—" })}</span>
            </button>
          ))}
          {candidates.length === 0 && (
            <p className="rounded-lg border border-slate-800 px-3 py-4 text-center text-xs text-slate-500">
              {t("nodes.migratePicker.noCandidates")}
            </p>
          )}
        </div>
        <button
          onClick={onClose}
          className="mt-4 w-full rounded-full px-3 py-2 text-xs text-slate-400 transition hover:bg-slate-800"
        >
          {t("common.cancel")}
        </button>
      </div>
    </div>
  );
}
