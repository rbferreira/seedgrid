import type { Catalog } from "../types";

export const ui: Catalog["ui"] = {
  chart: {
    noData: "No data for this period.",
  },
  ui: {
    healthBadge: {
      label: "health",
    },
  },
  otherVersions: {
    title: "You already have this title",
    summary: {
      one: "The cluster already has one version of {title}. Add this one too?",
      other: "The cluster already has {count} versions of {title}. Add this one too?",
    },
    inCluster: "Already in the cluster",
    adding: "You're adding",
    unknownResolution: "resolution ?",
    unknownCodec: "codec ?",
    onNode: "on {node}",
    cancel: "Cancel",
    confirm: "Add anyway",
  },
};
