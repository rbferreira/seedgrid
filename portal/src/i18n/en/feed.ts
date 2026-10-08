import type { Catalog } from "../types";

export const feed: Catalog["feed"] = {
  feed: {
    score: "score {score}",
    title: "Feed",
    live: "Live",
    description: "SeedGrid orchestration decisions and torrent transitions in real time — separate from Alerts and Events.",
    updatedAt: "updated {time}",
    error: {
      loadFailed: "Failed to load feed",
    },
    empty: "No activity yet. As soon as SeedGrid makes decisions (schedule, hold, promote) or a torrent changes state, the magic will appear here.",
    kind: {
      admissionHold: "Hold",
      admissionResume: "Promoted",
      scheduled: "Scheduled",
      downloading: "Downloading",
      seeding: "Seeding",
      screenshotHostFailed: "Image host",
      fallback: "Activity",
    },
    filter: {
      all: "All",
    },
    time: {
      secondsAgo: {
        one: "{count}s ago",
        other: "{count}s ago",
      },
      minutesAgo: {
        one: "{count}m ago",
        other: "{count}m ago",
      },
      hoursAgo: {
        one: "{count}h ago",
        other: "{count}h ago",
      },
      daysAgo: {
        one: "{count}d ago",
        other: "{count}d ago",
      },
    },
  },
};
