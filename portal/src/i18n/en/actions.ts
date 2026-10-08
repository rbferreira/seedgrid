import type { Catalog } from "../types";

export const actions: Catalog["actions"] = {
  actions: {
    failed: "Failed",
    timeout: "timed out",
    statusQueued: "Queued",
    statusDispatched: "Running on Node",
    statusDone: "Completed",
    sendFailed: "failed to send action",
    starting: "Starting…",
    migrationStartFailed: "failed to start migration",
    nothingToMigrate: "Nothing to migrate",
    stillInProgress: "Still in progress — check Events",
    migrationFailedCount: "Failed ({count}/{total})",
    migrationsCompleted: "{count} completed",
    migrationCompleted: "Completed",
    migrationQueuedCount: "Queued… {done}/{total}",
    migrationQueued: "Queued…",
    migratingCount: "Migrating… {done}/{total}",
    migrating: "Migrating…",
    processing: "processing",
  },
};
