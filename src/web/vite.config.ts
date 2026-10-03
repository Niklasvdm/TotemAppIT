import { execSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Build stamp, baked in at compile time. The browser caches bundles, so "which
// client am I actually running" needs an answer that travels with the bundle
// rather than one the server reports. Both halves are shown in the game HUD.
function buildStamp(): string {
  // The deploy may hand us the stamp directly: its remote builder gets only
  // src/, so neither the VERSION file nor a git tree is there to read.
  if (process.env.TOTEM_BUILD) return process.env.TOTEM_BUILD;

  let version = "dev";
  try {
    version = readFileSync(new URL("../../VERSION", import.meta.url), "utf8").trim();
  } catch {
    /* no VERSION file — stay "dev" */
  }
  try {
    const sha = execSync("git rev-parse --short=7 HEAD", { stdio: ["ignore", "pipe", "ignore"] })
      .toString()
      .trim();
    return `${version}+${sha}`;
  } catch {
    return version; // building outside a git tree (a tarball, a container)
  }
}

// Dev: proxy the API (and image/emoji routes) to the Go backend on :8683.
// Build: emits into the Go package src/internal/web/dist, which totemd embeds
// via //go:embed (go build -tags embed). emptyOutDir is needed because the dir
// is outside this project root.
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
  define: { __BUILD__: JSON.stringify(buildStamp()) },
  plugins: [react()],
  server: {
    allowedHosts,
    proxy: {
      // ws: true also upgrades /api/v1/games/ws to the Go backend. The Host
      // header is passed through unchanged, which is what lets the server's
      // same-origin WebSocket check pass in dev.
      "/api": { target: "http://127.0.0.1:8683", ws: true },
    },
  },
  build: { outDir: "../internal/web/dist", emptyOutDir: true },
});
