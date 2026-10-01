import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev: proxy the API (and image/emoji routes) to the Go backend on :8683.
// Build: emits to dist/, which totemd embeds via embed.FS in Phase 3.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8683",
    },
  },
  build: { outDir: "dist" },
});
