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
# By DEFAULT it detaches both servers and gives your terminal back (they also
# keep running after you close the SSH session). Manage it with:
#   ./run-dev.sh              # start detached, return the prompt
#   ./run-dev.sh --stop       # stop both servers
#   ./run-dev.sh --foreground # old behaviour: stay attached, Ctrl-C stops both
#   ./run-dev.sh --logs       # tail both logs
#
# Then open the http://<box-ip>:5173 URL it prints.
# =============================================================================
set -euo pipefail
cd "$(dirname "$0")"                       # repo root, wherever this is run from
source /etc/profile.d/go.sh 2>/dev/null || true   # put `go` on PATH

MODE=detach
for a in "$@"; do
  case "$a" in
    -d|--detach)     MODE=detach ;;
    -f|--foreground) MODE=foreground ;;
    --stop)          MODE=stop ;;
    --logs)          MODE=logs ;;
    -h|--help)       sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "unknown option: $a (try --help)" >&2; exit 2 ;;
  esac
done

stop_servers() {
  pkill -x totemd 2>/dev/null || true
  pkill -f "node_modules/.bin/vite" 2>/dev/null || true   # clears stale Vite (5173, 5174…)
}

case "$MODE" in
  stop) echo "→ stopping dev servers…"; stop_servers; echo "✓ stopped."; exit 0 ;;
  logs) exec tail -n 40 -f /tmp/totemd.log /tmp/vite.log ;;
esac

export TOTEM_DB_KEY="${TOTEM_DB_KEY:-dev-only-key}"   # dev key (not for production)
export TOTEM_DB_PATH="${TOTEM_DB_PATH:-$HOME/totem.db}"
export TOTEM_IMAGE_DIR="$PWD/data/images"
export TOTEM_EMOJI_FILE="$PWD/data/emoji.json"
export TOTEM_ADDR="127.0.0.1:8683"

# Per-box public hostname for the Vite dev-server allowlist, written by Terraform
# (bootstrap-dev.sh) to /etc/totem-web.env. No file (e.g. laptop) -> localhost only.
[ -f /etc/totem-web.env ] && . /etc/totem-web.env
export VITE_ALLOWED_HOSTS="${VITE_ALLOWED_HOSTS:-}"

IP="$(hostname -I | awk '{print $1}')"

# The game WebSocket only accepts an Origin matching the Host the backend sees.
# In production that holds (the WAF forwards its own Host), but Vite's dev proxy
# rewrites Host to 127.0.0.1:8683 while the browser's Origin stays the dev
# server — so the dev origins are allowlisted explicitly here. This is a
# DEV-ONLY widening: totemd defaults to same-origin when the variable is unset.
DEV_ORIGINS="localhost:5173,127.0.0.1:5173,${IP}:5173"
for host in ${VITE_ALLOWED_HOSTS//,/ }; do
  DEV_ORIGINS="$DEV_ORIGINS,$host,$host:5173"
done
export TOTEM_ALLOWED_ORIGINS="$DEV_ORIGINS"

echo "→ stopping any old backend / dev server…"
stop_servers

echo "→ building backend…"
( cd src && go build -o /tmp/totemd ./cmd/totemd )

if [ ! -f "$TOTEM_DB_PATH" ]; then
  echo "→ no database found — seeding $TOTEM_DB_PATH (first run)…"
  ( cd src && go run ./cmd/totem-seed --db "$TOTEM_DB_PATH" --force )
fi

[ -d src/web/node_modules ] || { echo "→ installing frontend deps (first run, ~1 min)…"; ( cd src/web && npm install ); }

# Wait for the backend to actually answer; returns non-zero if it never does.
wait_healthy() {
  for _ in $(seq 1 20); do
    curl -sf "http://$TOTEM_ADDR/healthz" >/dev/null 2>&1 && return 0
    sleep 1
  done
  return 1
}

# ---------------------------------------------------------------------------
# Foreground mode — the old behaviour: stay attached, Ctrl-C stops both.
# ---------------------------------------------------------------------------
if [ "$MODE" = foreground ]; then
  echo "→ starting backend on $TOTEM_ADDR…"
  /tmp/totemd >/tmp/totemd.log 2>&1 & BACK=$!
  trap 'echo; echo "stopping…"; kill "$BACK" 2>/dev/null || true; pkill -f "node_modules/.bin/vite" 2>/dev/null || true' EXIT
  if ! wait_healthy; then echo "✗ backend failed to start — log:"; cat /tmp/totemd.log; exit 1; fi
  echo "✓ backend healthy"
  echo
  echo "=================================================================="
  echo "  Open this in your browser:   http://$IP:5173"
  echo "  (press Ctrl-C here to stop both servers)"
  echo "=================================================================="
  echo
  cd src/web
  exec npm run dev -- --host 0.0.0.0
fi

# ---------------------------------------------------------------------------
# Detached mode (default) — start both in their own session and return the
# terminal. setsid detaches from the controlling tty, so closing SSH won't
# kill them; there is no EXIT trap here, on purpose.
# ---------------------------------------------------------------------------
echo "→ starting backend (detached) on $TOTEM_ADDR…"
setsid bash -c "exec /tmp/totemd" >/tmp/totemd.log 2>&1 </dev/null &
if ! wait_healthy; then echo "✗ backend failed to start — log:"; cat /tmp/totemd.log; exit 1; fi
echo "✓ backend healthy"

echo "→ starting Vite (detached) on :5173…"
setsid bash -c "cd '$PWD/src/web' && exec npm run dev -- --host 0.0.0.0" >/tmp/vite.log 2>&1 </dev/null &
for _ in $(seq 1 20); do ss -ltn 2>/dev/null | grep -q ':5173' && break; sleep 1; done

echo
echo "=================================================================="
echo "  Dev is running — your terminal is free."
echo "  Open:   http://$IP:5173"
echo "  Logs:   ./run-dev.sh --logs     (or tail -f /tmp/{totemd,vite}.log)"
echo "  Stop:   ./run-dev.sh --stop"
echo "=================================================================="
