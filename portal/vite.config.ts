import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// In dev, the Portal talks to the Control Plane API via proxy (same origin →
// no CORS). In production, the Portal is served behind Traefik at the same origin.
// SEEDGRID_API overrides the target (default: the local Control Plane, on port 8080).
const apiTarget = process.env.SEEDGRID_API ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    proxy: {
      "/api": { target: apiTarget, changeOrigin: true },
      "/healthz": { target: apiTarget, changeOrigin: true },
    },
  },
});
