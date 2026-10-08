import type { Catalog } from "../types";

export const app: Catalog["app"] = {
  router: {},
  app: {
    seedWatcher: {
      started: "🌱 «{name}» started seeding",
    },
  },
};
