#!/usr/bin/env bash
# =============================================================================
# update.sh — keep the toolchain and dependencies current on a build/dev host.
# Run ON the host (uses sudo for apt + the Go install). Idempotent.
#
#   On a build/dev host:
#     ./deploy/update.sh          # OS packages + Go toolchain + npm + reinstall deps
#     ./deploy/update.sh --deps   # ALSO bump go.mod and npm deps, then build+test
#   From your workstation (runs over SSH on the build host; repo must exist there):
#     ./deploy/update.sh root@192.168.10.195
#     REMOTE_REPO=/opt/totem-it ./deploy/update.sh root@build-host --deps
#
# This is a BUILD-HOST script (toolchain + source deps). A prod host that only
# runs the shipped binary is updated with `apt upgrade` + a redeploy, not this.
# The --deps pass changes lockfiles (go.mod/go.sum, package-lock.json); review
# and commit the diff afterwards.
# =============================================================================
set -euo pipefail

# If the first argument is user@host, re-run on that host over SSH and exit.
# REMOTE_REPO tells the remote where the checkout lives (default /root/totem-it).
if [ "${1:-}" ] && [[ "$1" == *@* ]]; then
  REMOTE_HOST="$1"; shift
  exec ssh "$REMOTE_HOST" "REMOTE_REPO='${REMOTE_REPO:-/root/totem-it}' bash -s -- $*" < "$0"
fi

# REPO_ROOT: the forwarded path when run over SSH (via bash -s, where $0 is bash),
# else resolved from this script's location for a direct local run.
REPO_ROOT="${REMOTE_REPO:-$(cd "$(dirname "$0")/.." && pwd)}"
BUMP="${1:-}"

# Use sudo only when not already root (LXC/containers often run as root without
# sudo even installed).
SUDO=""
if [ "$(id -u)" != 0 ]; then
  command -v sudo >/dev/null || { echo "need root (or sudo) to update packages" >&2; exit 1; }
  SUDO="sudo"
fi

echo "==> OS packages (apt)"
if command -v apt-get >/dev/null; then
  $SUDO apt-get update
  # `env` carries DEBIAN_FRONTEND whether or not $SUDO is set — a bare
  # `$SUDO VAR=val cmd` breaks when $SUDO is empty (VAR=val becomes the command).
  $SUDO env DEBIAN_FRONTEND=noninteractive apt-get -y upgrade
  $SUDO apt-get -y autoremove --purge
else
  echo "   (no apt — skipping OS update)"
fi

echo "==> Go toolchain"
LATEST="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1 || true)"   # e.g. go1.26.0
CURRENT="$(go version 2>/dev/null | awk '{print $3}' || echo none)"
echo "   current=$CURRENT latest=$LATEST"
if [ -n "$LATEST" ] && [ "$LATEST" != "$CURRENT" ]; then
  case "$(uname -m)" in
    x86_64|amd64) garch=amd64 ;;
    aarch64|arm64) garch=arm64 ;;
    *) garch=amd64 ;;
  esac
  tarball="${LATEST}.linux-${garch}.tar.gz"
  echo "   installing $tarball -> /usr/local/go"
  curl -fsSL "https://go.dev/dl/${tarball}" -o "/tmp/${tarball}"
  $SUDO rm -rf /usr/local/go
  $SUDO tar -C /usr/local -xzf "/tmp/${tarball}"
  rm -f "/tmp/${tarball}"
  # Make sure new shells find it (the dev LXC already ships this file).
  echo 'export PATH=$PATH:/usr/local/go/bin' | $SUDO tee /etc/profile.d/go.sh >/dev/null
  export PATH=$PATH:/usr/local/go/bin
  echo "   now: $(go version)"
fi

echo "==> Node / npm"
if command -v npm >/dev/null; then
  $SUDO npm install -g npm@latest 2>/dev/null || npm install -g npm@latest || true
  # `npm ci` needs a committed lockfile; fall back to `npm install` if absent.
  ( cd "$REPO_ROOT/src/web" && if [ -f package-lock.json ]; then npm ci; else npm install; fi )
  echo "   node=$(node -v) npm=$(npm -v)"
else
  echo "   (no npm — skipping; install Node.js to build the frontend)"
fi

if [ "$BUMP" = "--deps" ]; then
  echo "==> bumping Go modules (go get -u + tidy), then build + test"
  ( cd "$REPO_ROOT/src" && go get -u ./... && go mod tidy && go build ./... && go test ./... )
  echo "==> bumping npm deps (npm update), then type-check"
  ( cd "$REPO_ROOT/src/web" && npm update && npx tsc --noEmit )
  echo "   review and commit go.mod/go.sum + package-lock.json changes."
fi

echo "done."
