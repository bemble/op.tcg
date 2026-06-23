import { defineConfig } from "vite";

// Absolute base: required for History-API (clean-URL) routing — assets must
// resolve to /assets/... regardless of the current route (e.g. /collection/OP16),
// not relative to it. The app is therefore expected to be served at the site
// root. Dev server proxies /api to the backend.
export default defineConfig({
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
