#!/usr/bin/env bash
# =============================================================================
# run-dev.sh — start the WHOLE app in dev mode with one command, on the dev box.
#
# What it does for you:
#   1. builds the Go backend (totemd)
#   2. seeds the encrypted database the first time (if it doesn't exist yet)
#   3. starts the backend on 127.0.0.1:8683
#   4. starts the React dev server (with hot-reload) on port 5173, open to the LAN
#
# Usage (on the box):   cd ~/totem-it && ./run-dev.sh
# Then open the URL it prints in your laptop's browser. Ctrl-C stops everything.
# =============================================================================
set -euo pipefail
cd "$(dirname "$0")"                       # repo root, wherever this is run from
source /etc/profile.d/go.sh 2>/dev/null || true   # put `go` on PATH

export TOTEM_DB_KEY="${TOTEM_DB_KEY:-dev-only-key}"   # dev key (not for production)
export TOTEM_DB_PATH="${TOTEM_DB_PATH:-$HOME/totem.db}"
export TOTEM_IMAGE_DIR="$PWD/data/images"
export TOTEM_EMOJI_FILE="$PWD/data/emoji.json"
export TOTEM_ADDR="127.0.0.1:8683"

# Per-box public hostname for the Vite dev-server allowlist, written by Terraform
# (bootstrap-dev.sh) to /etc/totem-web.env. No file (e.g. laptop) -> localhost only.
[ -f /etc/totem-web.env ] && . /etc/totem-web.env
export VITE_ALLOWED_HOSTS="${VITE_ALLOWED_HOSTS:-}"

echo "→ stopping any old backend / dev server…"
pkill -x totemd 2>/dev/null || true
pkill -f "node_modules/.bin/vite" 2>/dev/null || true   # clear stale Vite servers (5173, 5174…)

echo "→ building backend…"
( cd src && go build -o /tmp/totemd ./cmd/totemd )

if [ ! -f "$TOTEM_DB_PATH" ]; then
  echo "→ no database found — seeding $TOTEM_DB_PATH (first run)…"
  ( cd src && go run ./cmd/totem-seed --db "$TOTEM_DB_PATH" --force )
fi

echo "→ starting backend on $TOTEM_ADDR…"
/tmp/totemd >/tmp/totemd.log 2>&1 & BACK=$!
trap 'echo; echo "stopping…"; kill "$BACK" 2>/dev/null || true' EXIT

# Wait for the backend to actually answer — if it died, show why instead of
# leaving you with a silently-empty page.
for _ in $(seq 1 20); do
  if curl -sf "http://$TOTEM_ADDR/healthz" >/dev/null 2>&1; then break; fi
  if ! kill -0 "$BACK" 2>/dev/null; then
    echo "✗ backend failed to start — log:"; cat /tmp/totemd.log; exit 1
  fi
  sleep 1
done
echo "✓ backend healthy"

cd src/web
[ -d node_modules ] || { echo "→ installing frontend deps (first run, ~1 min)…"; npm install; }

IP="$(hostname -I | awk '{print $1}')"
echo
echo "=================================================================="
echo "  Open this in your browser:   http://$IP:5173"
echo "  (press Ctrl-C here to stop both servers)"
echo "=================================================================="
echo
npm run dev -- --host 0.0.0.0
