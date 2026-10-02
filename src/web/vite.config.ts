import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev: proxy the API (and image/emoji routes) to the Go backend on :8683.
// Build: emits to dist/, which totemd embeds via embed.FS in Phase 3.
//
// The WAF proxies here forwarding its own Host header, and Vite's dev server
// blocks unknown hosts unless allowlisted. The allowed host is supplied PER
// ENVIRONMENT via VITE_ALLOWED_HOSTS (set from the Terraform `public_host` var,
// written to /etc/totem-web.env and exported by run-dev.sh) — there is NO
// hardcoded default, so a prod box never silently accepts the dev hostname.
// NOTE: this is a *dev-server* setting only. In production the SPA should be a
// static `npm run build` served by the WAF (or embedded in totemd), where no
// allowlist applies; totemd itself does not validate Host (localhost behind the
// WAF), so the backend needs no per-host config.
const allowedHosts = (process.env.VITE_ALLOWED_HOSTS ?? "")
  .split(",")
  .map((h) => h.trim())
  .filter(Boolean);

export default defineConfig({
  plugins: [react()],
  server: {
    allowedHosts,
    proxy: {
      "/api": "http://127.0.0.1:8683",
    },
  },
  build: { outDir: "dist" },
});
