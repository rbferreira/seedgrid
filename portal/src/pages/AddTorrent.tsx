import { useState, useRef, useCallback, useEffect, type DragEvent } from "react";
import { TrackerUploadPanel } from "./publish/TrackerUploadPanel";
import { fmtSize } from "./publish/bbcode";
import { nodeLabel, nodeLabelById } from "../lib/node-label";
import { canManageTrackers } from "../lib/tracker-admin";
import { useOtherVersionsPrompt } from "../components/OtherVersionsDialog";
import {
  addTorrentMagnet,
  addTorrentFile,
  publishContent,
  inferPublish,
  getTrackers,
  getNodes,
  getPendingPublish,
  removeTorrent,
  otherVersionsOf,
  ApiError,
  type Tracker,
  type Node,
  type Torrent,
  type PendingPublish,
} from "../api";
import { rich, useT } from "../i18n";
import { askConfirm } from "../components/ConfirmDialog";

// skipped: didn't go in and isn't an error — the file was already in the cluster, or the operator
// cancelled on seeing the other version (#37).
type JobStatus = "pending" | "sending" | "ok" | "skipped" | "error";
type Job = {
  key: string;
  label: string;
  kind: "file" | "magnet";
  file?: File;
  magnet?: string;
  status: JobStatus;
  detail?: string;
};

const STATUS_UI: Record<JobStatus, { icon: string; cls: string }> = {
  pending: { icon: "•", cls: "text-slate-500" },
  sending: { icon: "…", cls: "text-blue-400" },
  ok: { icon: "✓", cls: "text-emerald-400" },
  skipped: { icon: "–", cls: "text-slate-400" },
  error: { icon: "✕", cls: "text-red-400" },
};

// AddTorrent: adds an EXISTING torrent (magnet/.torrent). Publishing local content
// (upload → the Cluster creates the .torrent and seeds, ADR-019/020) is the dedicated `Publish` page.
export function AddTorrent({ onLogout }: { onLogout: () => void }) {
  const t = useT();
  return (
    <>
      <h1 className="text-lg font-semibold text-white">{t("torrents.addTorrent.title")}</h1>
      <AddExisting onLogout={onLogout} />
    </>
  );
}

// Publish is the local content publishing page (ADR-019/020): uploads files,
// the Cluster creates the .torrent, announces and seeds; optionally registers on a tracker
// UNIT3D. Also shows the pending publishing queue (backlog #8).
export function Publish({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  return (
    <>
      <h1 className="text-lg font-semibold text-white">{t("torrents.addTorrent.publishTitle")}</h1>
      <PublishContent can={can} onLogout={onLogout} />
    </>
  );
}

function AddExisting({ onLogout }: { onLogout: () => void }) {
  const t = useT();
  const [magnet, setMagnet] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [dragOver, setDragOver] = useState(false);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState("");
  const [jobs, setJobs] = useState<Job[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);
  const [askOtherVersion, otherVersionDialog] = useOtherVersionsPrompt();

  // Nodes to translate the result's node_id into the Node name (backlog #16 — identify
  // the Node by name, not by UUID). Best-effort: if it fails, falls back to short UUID.
  useEffect(() => {
    (async () => {
      try {
        setNodes(await getNodes());
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) onLogout();
      }
    })();
  }, [onLogout]);
  const nodeName = useCallback(
    (id: string) => nodeLabelById(nodes, id) || id.slice(0, 8),
    [nodes],
  );

  // addFiles: adds valid .torrents, ignores duplicates (by name+size) and warns about invalid ones.
  const addFiles = useCallback((incoming: File[]) => {
    const valid = incoming.filter((f) => f.name.endsWith(".torrent"));
    const invalid = incoming.filter((f) => !f.name.endsWith(".torrent"));
    setError(invalid.length ? t("torrents.addTorrent.ignoredNotTorrent", { names: invalid.map((f) => f.name).join(", ") }) : "");
    setFiles((prev) => {
      const seen = new Set(prev.map((f) => `${f.name}:${f.size}`));
      const merged = [...prev];
      for (const f of valid) {
        const id = `${f.name}:${f.size}`;
        if (!seen.has(id)) {
          seen.add(id);
          merged.push(f);
        }
      }
      return merged;
    });
  }, [t]);

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    addFiles(Array.from(e.dataTransfer.files ?? []));
  };

  const removeFile = (idx: number) => setFiles((prev) => prev.filter((_, i) => i !== idx));

  const magnetLines = () =>
    magnet
      .split(/\r?\n/)
      .map((l) => l.trim())
      .filter((l) => l.length > 0);

  async function submit() {
    const magnets = magnetLines();
    if (files.length === 0 && magnets.length === 0) {
      setError(t("torrents.addTorrent.errorNoInput"));
      return;
    }
    setError("");
    setRunning(true);

    const queue: Job[] = [
      ...files.map((f, i) => ({
        key: `f${i}:${f.name}`,
        label: f.name,
        kind: "file" as const,
        file: f,
        status: "pending" as JobStatus,
      })),
      ...magnets.map((m, i) => ({
        key: `m${i}`,
        label: m.length > 60 ? `${m.slice(0, 57)}…` : m,
        kind: "magnet" as const,
        magnet: m,
        status: "pending" as JobStatus,
      })),
    ];
    setJobs(queue);

    let unauthorized = false;
    for (let i = 0; i < queue.length; i++) {
      setJobs((prev) => prev.map((j, idx) => (idx === i ? { ...j, status: "sending" } : j)));
      const runAdd = (allowOtherVersion: boolean): Promise<Torrent> =>
        queue[i].kind === "file"
          ? addTorrentFile(queue[i].file!, allowOtherVersion)
          : addTorrentMagnet(queue[i].magnet!, undefined, allowOtherVersion);
      const skip = (detail: string) =>
        setJobs((prev) => prev.map((j, idx) => (idx === i ? { ...j, status: "skipped", detail } : j)));
      try {
        let tRes: Torrent;
        try {
          tRes = await runAdd(false);
        } catch (e) {
          // #37: the SAME file is already in the cluster → skipped, without offering a 2nd copy.
          if (e instanceof ApiError && e.status === 409 && e.code === "duplicate") {
            skip(t("torrents.addTorrent.detailAlreadyInCluster"));
            continue;
          }
          // #37: another version of the same work → show what already exists and ask.
          const other = otherVersionsOf(e);
          if (!other) throw e;
          if (!(await askOtherVersion(other.adding?.name || queue[i].label, other))) {
            skip(t("torrents.addTorrent.detailOtherVersionCancelled"));
            continue;
          }
          tRes = await runAdd(true);
        }
        setJobs((prev) =>
          prev.map((j, idx) =>
            idx === i
              ? { ...j, status: "ok", label: tRes.name ?? j.label, detail: `${nodeName(tRes.node_id)} · ${tRes.status}` }
              : j,
          ),
        );
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          unauthorized = true;
          break;
        }
        const detail =
          err instanceof ApiError && err.code === "no_eligible_node"
            ? t("torrents.addTorrent.errorNoEligibleNode")
            : err instanceof ApiError
              ? err.message
              : t("torrents.addTorrent.errorAddFailed");
        setJobs((prev) => prev.map((j, idx) => (idx === i ? { ...j, status: "error", detail } : j)));
      }
    }

    setRunning(false);
    if (unauthorized) {
      onLogout();
      return;
    }
    // clears entries that were accepted; keeps failed ones for manual resubmission
    setFiles([]);
    setMagnet("");
  }

  const okCount = jobs.filter((j) => j.status === "ok").length;
  const errCount = jobs.filter((j) => j.status === "error").length;
  const skipCount = jobs.filter((j) => j.status === "skipped").length;
  const totalPending = files.length + magnetLines().length;

  return (
    <>
      <p className="text-sm text-slate-400">
        {rich(t("torrents.addTorrent.helpText"), { file: <code>{".torrent"}</code> })}
      </p>

      <div className="max-w-2xl space-y-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-6">
        {/* Drop-zone */}
        <div
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={onDrop}
          className={`flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed p-8 text-center transition ${
            dragOver ? "border-emerald-500 bg-emerald-500/5" : "border-slate-700 hover:border-slate-600"
          }`}
        >
          <span className="text-2xl">⬇</span>
          <p className="mt-2 text-sm text-slate-400">
            {rich(t("torrents.addTorrent.dropzoneHint"), { file: <code>{".torrent"}</code> })}
          </p>
          <input
            ref={inputRef}
            type="file"
            accept=".torrent"
            multiple
            className="sg-field hidden"
            onChange={(e) => {
              addFiles(Array.from(e.target.files ?? []));
              e.target.value = ""; // allows re-selecting the same file
            }}
          />
        </div>

        {/* Selected files queue */}
        {files.length > 0 && (
          <ul className="divide-y divide-slate-800 rounded-xl border border-slate-800">
            {files.map((f, i) => (
              <li key={`${f.name}:${f.size}`} className="flex items-center justify-between gap-3 px-3 py-2">
                <span className="truncate text-sm text-slate-200">📄 {f.name}</span>
                <button
                  onClick={() => removeFile(i)}
                  disabled={running}
                  title={t("torrents.addTorrent.removeFile")}
                  className="shrink-0 rounded-full px-2 py-0.5 text-xs text-slate-400 hover:bg-slate-800 hover:text-red-400 disabled:opacity-40"
                >
                  ✕
                </button>
              </li>
            ))}
          </ul>
        )}

        <div className="flex items-center gap-3 text-xs text-slate-500">
          <div className="h-px flex-1 bg-slate-800" /> {t("torrents.addTorrent.orSeparator")} <div className="h-px flex-1 bg-slate-800" />
        </div>

        <div>
          <label className="mb-1 block text-sm text-slate-300">{t("torrents.addTorrent.magnetsLabel")}</label>
          <textarea
            value={magnet}
            onChange={(e) => setMagnet(e.target.value)}
            placeholder={"magnet:?xt=urn:btih:…\nmagnet:?xt=urn:btih:…"}
            rows={3}
            className="sg-textarea text-xs w-full resize-y font-mono"
          />
        </div>

        {error && <p className="text-sm text-red-400">{error}</p>}

        {/* Batch result */}
        {jobs.length > 0 && (
          <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-950/40 p-3">
            <p className="text-xs text-slate-400">
              {running
                ? t("torrents.addTorrent.statusSending", { current: okCount + errCount + skipCount, total: jobs.length })
                : t("torrents.addTorrent.statusDone", { ok: okCount, skipped: skipCount, errors: errCount })}
            </p>
            <ul className="space-y-1">
              {jobs.map((j) => {
                const ui = STATUS_UI[j.status];
                return (
                  <li key={j.key} className="flex items-center justify-between gap-3 text-sm">
                    <span className="flex min-w-0 items-center gap-2">
                      <span className={`w-3 shrink-0 ${ui.cls}`}>{ui.icon}</span>
                      <span className="truncate text-slate-200">{j.label}</span>
                    </span>
                    {j.detail && (
                      <span
                        className={`shrink-0 text-xs ${j.status === "error" ? "text-red-400" : "text-slate-500"}`}
                      >
                        {j.detail}
                      </span>
                    )}
                  </li>
                );
              })}
            </ul>
          </div>
        )}

        <button
          onClick={submit}
          disabled={running || totalPending === 0}
          className="rounded-full bg-emerald-600 px-4 py-2 text-sm font-medium text-[#fff] transition hover:bg-emerald-500 disabled:opacity-50"
        >
          {running
            ? t("torrents.addTorrent.btnAdding")
            : totalPending > 1
              ? t("torrents.addTorrent.btnAddMultiple", { count: totalPending })
              : t("torrents.addTorrent.btnAddSingle")}
        </button>
      </div>
      {otherVersionDialog}
    </>
  );
}

type PublishPhase = "idle" | "uploading" | "processing" | "done" | "error";

// PublishContent publishes local files (ADR-019): uploads the content, the Node creates
// the .torrent and starts seeding. Chooses public (DHT) or a registered tracker, and lets
// the Scheduler choose the Node (or forces one).
function PublishContent({ can, onLogout }: { can: (perm: string) => boolean; onLogout: () => void }) {
  const t = useT();
  const [files, setFiles] = useState<File[]>([]);
  const [dragOver, setDragOver] = useState(false);
  const [name, setName] = useState("");
  const [trackers, setTrackers] = useState<Tracker[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [trackerId, setTrackerId] = useState("public");
  const [forceNode, setForceNode] = useState("");
  const [phase, setPhase] = useState<PublishPhase>("idle");
  const [pct, setPct] = useState(0);
  const [msg, setMsg] = useState("");
  // After publishing to a UNIT3D tracker, saves what's needed for site registration.
  const [published, setPublished] = useState<{
    torrentId: string;
    name: string;
    mediaInfo: string;
    tracker: Tracker;
  } | null>(null);
  // Pending publishing queue (backlog #8): torrents created with a tracker
  // UNIT3D but not yet published to it. Allows reopening registration without re-uploading.
  const [pending, setPending] = useState<PendingPublish[]>([]);
  const [pendingBusy, setPendingBusy] = useState(""); // id of the pending item being removed (#18)
  const inputRef = useRef<HTMLInputElement>(null);

  const reloadPending = useCallback(async () => {
    try {
      setPending(await getPendingPublish());
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) onLogout();
    }
  }, [onLogout]);

  useEffect(() => {
    (async () => {
      try {
        const [ts, ns] = await Promise.all([getTrackers(), getNodes()]);
        setTrackers(ts);
        setNodes(ns);
        const def = ts.find((t) => t.is_default);
        if (def) setTrackerId(def.id); // configurable default (ADR-019)
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) onLogout();
      }
    })();
    reloadPending();
  }, [onLogout, reloadPending]);

  // Opens the review panel for a pending item (reuses TrackerUploadPanel). The
  // mediainfo isn't in the queue: the CP falls back to what was extracted on the Node when publishing.
  function resumePending(p: PendingPublish) {
    const tr = trackers.find((t) => t.id === p.tracker_id);
    if (!tr) return;
    setPublished({ torrentId: p.id, name: p.name || "", mediaInfo: "", tracker: tr });
  }

  // Give up on a pending item (backlog #18): cancels the upload — removes the torrent from the Node
  // (the Agent deletes from the Engine + data) and it disappears from the queue (soft delete removes from
  // pending, which filters deleted_at IS NULL). Reuses the same remove from other screens.
  async function abandonPending(p: PendingPublish) {
    const ok = await askConfirm({
      title: t("torrents.addTorrent.confirmAbandon.title", { name: p.name || p.id }),
      message: t("torrents.addTorrent.confirmAbandon.message"),
      confirmLabel: t("torrents.addTorrent.confirmAbandon.confirm"),
      tone: "danger",
    });
    if (!ok) return;
    setPendingBusy(p.id);
    try {
      await removeTorrent(p.id);
      await reloadPending();
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) onLogout();
    } finally {
      setPendingBusy("");
    }
  }

  // Suggests the name in the UNIT3D tracker standard from the 1st file — only when the
  // field is empty (doesn't overwrite what the operator typed). Thus the folder created
  // on the Node is already standardized. The operator can still edit.
  const selTracker = trackers.find((t) => t.id === trackerId);
  useEffect(() => {
    if (!selTracker?.source || files.length === 0 || name.trim() !== "") return;
    let cancelled = false;
    (async () => {
      try {
        const inf = await inferPublish(files[0].name);
        if (!cancelled && inf.parsed.standard_name) setName(inf.parsed.standard_name);
      } catch {
        /* silent: name suggestion is best-effort */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [files, selTracker?.source, name]);

  const addFiles = useCallback((incoming: File[]) => {
    setFiles((prev) => {
      const seen = new Set(prev.map((f) => `${f.name}:${f.size}`));
      return [...prev, ...incoming.filter((f) => !seen.has(`${f.name}:${f.size}`))];
    });
  }, []);

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    addFiles(Array.from(e.dataTransfer.files ?? []));
  };
  const removeFile = (idx: number) => setFiles((prev) => prev.filter((_, i) => i !== idx));

  const totalBytes = files.reduce((s, f) => s + f.size, 0);
  const running = phase === "uploading" || phase === "processing";

  async function submit() {
    if (files.length === 0) {
      setPhase("error");
      setMsg(t("torrents.addTorrent.errorSelectFile"));
      return;
    }
    setPhase("uploading");
    setPct(0);
    setMsg("");
    setPublished(null);
    try {
      const tRes = await publishContent(files, {
        name: name.trim(),
        trackerId,
        forceNodeId: forceNode,
        // UNIT3D tracker + single file → renames the file to the standard (not just the folder).
        standardizeFile: !!selTracker?.source,
        onProgress: (p) => {
          setPct(p);
          if (p >= 100) setPhase("processing");
        },
      });
      setPhase("done");
      const nodeDisplayName = nodeLabelById(nodes, tRes.node_id) || tRes.node_id.slice(0, 8);
      setMsg(t("torrents.addTorrent.msgPublished", { node: nodeDisplayName, name: tRes.name ?? tRes.hash ?? "" }));
      // If the chosen tracker has UNIT3D integration (source + API), opens the site registration.
      const sel = trackers.find((x) => x.id === trackerId);
      if (sel && sel.source && sel.has_api_token) {
        setPublished({ torrentId: tRes.id, name: tRes.name || name.trim(), mediaInfo: tRes.media_info ?? "", tracker: sel });
      }
      setFiles([]);
      setName("");
      reloadPending(); // the new torrent enters the pending queue (until the POST to the tracker)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onLogout();
        return;
      }
      setPhase("error");
      setMsg(
        err instanceof ApiError && err.status === 409
          ? t("torrents.addTorrent.errorNoEligibleOrOffline")
          : err instanceof ApiError
            ? err.message
            : t("torrents.addTorrent.errorPublishFailed"),
      );
    }
  }

  return (
    <>
      <p className="text-sm text-slate-400">
        {rich(t("torrents.addTorrent.publishHelpText"), { file: <code>{".torrent"}</code> })}
      </p>

      {/* Pending publishing queue (backlog #8): torrents created with a tracker that
          have not yet been published to the site. Reopens the review panel without re-upload.
          Always visible (with empty state) to be discoverable. */}
      <div className="max-w-2xl space-y-2 rounded-2xl border border-amber-700/40 bg-amber-500/5 p-4">
        <h2 className="flex items-center gap-2 text-sm font-semibold text-amber-300">
          ⏳ {t("torrents.addTorrent.pendingTitle")}
          <span className="rounded-full bg-amber-500/20 px-2 py-0.5 text-xs text-amber-200">{pending.length}</span>
        </h2>
        {pending.length === 0 ? (
          <p className="text-xs text-slate-400">
            {t("torrents.addTorrent.pendingEmpty")}
          </p>
        ) : (
          <>
            <p className="text-xs text-slate-400">
              {t("torrents.addTorrent.pendingHint")}
            </p>
            <ul className="divide-y divide-slate-800/70 rounded-xl border border-slate-800">
              {pending.map((p) => (
                <li key={p.id} className="flex items-center justify-between gap-3 px-3 py-2">
                  <span className="min-w-0 truncate text-sm text-slate-200" title={p.name || p.id}>
                    📄 {p.name || p.id}
                    <span className="ml-2 text-xs text-slate-500">→ {p.tracker_source || p.tracker_name}</span>
                  </span>
                  <div className="flex shrink-0 items-center gap-2">
                    <button
                      onClick={() => resumePending(p)}
                      disabled={!p.has_file || running || pendingBusy === p.id}
                      title={p.has_file ? t("torrents.addTorrent.resumeTitleReady") : t("torrents.addTorrent.resumeTitleWaiting")}
                      className="rounded-full bg-amber-600 px-3 py-1 text-xs font-medium text-[#fff] transition hover:bg-amber-500 disabled:opacity-40"
                    >
                      {t("torrents.addTorrent.btnReviewPublish")}
                    </button>
                    <button
                      onClick={() => abandonPending(p)}
                      disabled={running || pendingBusy === p.id}
                      title={t("torrents.addTorrent.abandonTitle")}
                      className="rounded-full border border-red-700/50 px-3 py-1 text-xs font-medium text-red-400 transition hover:bg-red-500/10 disabled:opacity-40"
                    >
                      {pendingBusy === p.id ? t("torrents.addTorrent.btnRemoving") : t("torrents.addTorrent.btnAbandon")}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>

      <div className="max-w-2xl space-y-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-6">
        {/* Drop-zone (any file) */}
        <div
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setDragOver(true);
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={onDrop}
          className={`flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed p-8 text-center transition ${
            dragOver ? "border-emerald-500 bg-emerald-500/5" : "border-slate-700 hover:border-slate-600"
          }`}
        >
          <span className="text-2xl">📦</span>
          <p className="mt-2 text-sm text-slate-400">
            {t("torrents.addTorrent.publishDropzoneHint")}
          </p>
          <input
            ref={inputRef}
            type="file"
            multiple
            className="sg-field hidden"
            onChange={(e) => {
              addFiles(Array.from(e.target.files ?? []));
              e.target.value = "";
            }}
          />
        </div>

        {files.length > 0 && (
          <ul className="divide-y divide-slate-800 rounded-xl border border-slate-800">
            {files.map((f, i) => (
              <li key={`${f.name}:${f.size}`} className="flex items-center justify-between gap-3 px-3 py-2">
                <span className="truncate text-sm text-slate-200">
                  📄 {f.name} <span className="text-xs text-slate-500">({fmtSize(f.size)})</span>
                </span>
                <button
                  onClick={() => removeFile(i)}
                  disabled={running}
                  title={t("torrents.addTorrent.removeFile")}
                  className="shrink-0 rounded-full px-2 py-0.5 text-xs text-slate-400 hover:bg-slate-800 hover:text-red-400 disabled:opacity-40"
                >
                  ✕
                </button>
              </li>
            ))}
          </ul>
        )}

        <div>
          <label className="mb-1 block text-sm text-slate-300">{t("torrents.addTorrent.torrentNameLabel")}</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("torrents.addTorrent.torrentNamePlaceholder")}
            className="sg-field w-full"
          />
          {selTracker?.source && (
            <p className="mt-1 text-xs text-sky-300/80">
              {rich(t("torrents.addTorrent.nameSuggestionHint"), { source: <strong>{selTracker.source}</strong> })}
            </p>
          )}
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-sm text-slate-300">{t("torrents.addTorrent.announceLabel")}</label>
            <select
              value={trackerId}
              onChange={(e) => setTrackerId(e.target.value)}
              className="sg-field sg-select w-full"
            >
              <option value="public">{t("torrents.addTorrent.optionPublic")}</option>
              {trackers.map((tr) => (
                <option key={tr.id} value={tr.id}>
                  {tr.name}
                  {tr.is_default ? ` (${t("torrents.addTorrent.defaultSuffix")})` : ""}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-sm text-slate-300">{t("torrents.addTorrent.seedNodeLabel")}</label>
            <select
              value={forceNode}
              onChange={(e) => setForceNode(e.target.value)}
              className="sg-field sg-select w-full"
            >
              <option value="">{t("torrents.addTorrent.optionSchedulerChooses")}</option>
              {nodes
                .filter((n) => n.status === "ONLINE")
                .map((n) => (
                  <option key={n.id} value={n.id}>
                    {nodeLabel(n)}
                  </option>
                ))}
            </select>
          </div>
        </div>

        {trackers.length === 0 &&
          (canManageTrackers(can) ? (
            <p className="text-xs text-slate-500">
              {t("torrents.addTorrent.noTrackersHintPrefix")}{" "}
              <a href="#/settings" className="text-emerald-400 hover:underline">
                {t("torrents.addTorrent.trackersLink")}
              </a>{" "}
              {t("torrents.addTorrent.noTrackersHintSuffix")}
            </p>
          ) : (
            <p className="text-xs text-slate-500">{t("torrents.addTorrent.noTrackersAskAdmin")}</p>
          ))}

        {phase !== "idle" && (
          <div className="space-y-2 rounded-xl border border-slate-800 bg-slate-950/40 p-3 text-sm">
            {phase === "uploading" && (
              <>
                <p className="text-slate-300">{t("torrents.addTorrent.progressUploading", { pct })}</p>
                <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
                  <div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${pct}%` }} />
                </div>
              </>
            )}
            {phase === "processing" && (
              <p className="text-blue-400">{t("torrents.addTorrent.phaseProcessing")}</p>
            )}
            {phase === "done" && <p className="text-emerald-400">✓ {msg}</p>}
            {phase === "error" && <p className="text-red-400">✕ {msg}</p>}
          </div>
        )}

        <button
          onClick={submit}
          disabled={running || files.length === 0}
          className="rounded-full bg-emerald-600 px-4 py-2 text-sm font-medium text-[#fff] transition hover:bg-emerald-500 disabled:opacity-50"
        >
          {phase === "uploading"
            ? t("torrents.addTorrent.progressUploading", { pct })
            : phase === "processing"
              ? t("torrents.addTorrent.btnCreatingTorrent")
              : t("torrents.addTorrent.btnPublish", { size: totalBytes > 0 ? ` (${fmtSize(totalBytes)})` : "" })}
        </button>
      </div>

      {published && (
        <TrackerUploadPanel
          torrentId={published.torrentId}
          name={published.name}
          mediaInfo={published.mediaInfo}
          tracker={published.tracker}
          canManageTrackers={canManageTrackers(can)}
          onLogout={onLogout}
          onPublished={reloadPending}
          onClose={() => setPublished(null)}
        />
      )}
    </>
  );
}
