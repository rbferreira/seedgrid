import type { Catalog } from "../types";

export const audit: Catalog["audit"] = {
  log: {
    title: "Audit Log",
    description: "What each operator did in SeedGrid — in order: who, when, and from where. Separate trail from Alerts, Events, and Feed.",
    updatedAt: "updated {time}",
    filter: {
      all: "All",
    },
    error: {
      load: "Failed to load audit log",
    },
    status: {
      failed: "failed",
      failedWithCode: "failed ({code})",
    },
    time: {
      seconds: "{count}s ago",
      minutes: "{count}m ago",
      hours: "{count}h ago",
      days: "{count}d ago",
    },
    empty: "No actions recorded yet. Logins and changes (torrents, Nodes, settings, mode) will appear here.",
  },
  importSections: {
    description: {
      new: "; description layout: applied",
      replaced: "; description layout: replaced on request",
      protected: "; description layout: kept the operator's",
      same: "; description layout: already the same",
    },
    image_host: {
      new: "; image host: applied",
      replaced: "; image host: replaced on request",
      protected: "; image host: kept the operator's",
      same: "; image host: already the same",
    },
  },
};
