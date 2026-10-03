#!/usr/bin/env bash
# =============================================================================
# deploy.sh — build and deploy the totemd backend, admin CLI and data to a host.
#
#   ./deploy/deploy.sh root@host                        # build locally (needs Go), ship, restart
#   ./deploy/deploy.sh root@host --install-unit         # also (re)install the systemd unit
#   ./deploy/deploy.sh root@host --builder root@buildbox # build on a host that HAS Go (no local Go needed)
#
# Builds static linux/amd64 binaries (pure Go, no CGO), ships them + the emoji
# map and images, then restarts totemd (which applies pending DB migrations on
# startup). The SPA/front-end is deployed separately (binary embed is pending).
#
# Go is only needed where the BUILD happens: locally by default, or on --builder
# (e.g. the dev LXC, which already has Go) so your workstation needs nothing.
#
# Prerequisites on the host: the `totemd` systemd service configured with the DB
# key (see deploy/totemd.service), and an already-seeded database. First-time
# seed: run `totem-seed` on the host (see README → Deployment).
# =============================================================================
set -euo pipefail

HOST="" ; INSTALL_UNIT=0 ; BUILDER=""
while [ $# -gt 0 ]; do
  case "$1" in
    --install-unit) INSTALL_UNIT=1 ;;
    --builder) BUILDER="${2:?--builder needs user@host}" ; shift ;;
    --builder=*) BUILDER="${1#*=}" ;;
    -*) echo "unknown option: $1" >&2 ; exit 2 ;;
    *) HOST="$1" ;;
  esac
  shift
done
[ -n "$HOST" ] || { echo "usage: deploy.sh user@host [--install-unit] [--builder user@host]" >&2; exit 1; }

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Build stamp. The binaries carry it via -ldflags and the SPA bakes it in at
# `npm run build`, so the running client and server can each say what they are
# (see the game's F3 HUD). Go stamps the commit itself; this is the readable half.
BUILD_VERSION="$(cat "$REPO_ROOT/VERSION" 2>/dev/null || echo dev)"
BUILD_SHA="$(git -C "$REPO_ROOT" rev-parse --short=7 HEAD 2>/dev/null || true)"
BUILD_STAMP="$BUILD_VERSION${BUILD_SHA:++$BUILD_SHA}"
echo "→ build: $BUILD_STAMP"
BIN_DIR=/usr/local/bin
DATA_DIR=/var/lib/totemd/data
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

CMDS="totemd totem-admin totem-seed"

if [ -n "$BUILDER" ]; then
  echo "==> building SPA + binaries on remote builder $BUILDER (no local Go/Node needed)"
  ssh "$BUILDER" "mkdir -p ~/totem-build/src"
  rsync -az --delete --exclude node_modules --exclude .git "$REPO_ROOT/src/" "$BUILDER:~/totem-build/src/"
  ssh "$BUILDER" "set -e; source /etc/profile.d/go.sh 2>/dev/null || true
    export TOTEM_BUILD='$BUILD_STAMP'
    cd ~/totem-build/src/web && npm ci && npm run build      # -> ../internal/web/dist
    cd ~/totem-build/src
    for c in $CMDS; do
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed -trimpath -ldflags \"-s -w -X github.com/Niklasvdm/TotemAppIT/internal/buildinfo.Version=$BUILD_VERSION\" -o \"../\$c\" \"./cmd/\$c\"
    done"
  for cmd in $CMDS; do scp -q "$BUILDER:~/totem-build/$cmd" "$STAGE/$cmd"; done
else
  command -v go >/dev/null 2>&1 || {
    echo "Go not found locally. Either install it (./deploy/update.sh installs it)," >&2
    echo "or build on a host that has Go:  ./deploy/deploy.sh $HOST --builder root@<buildbox>" >&2
    exit 1
  }
  command -v npm >/dev/null 2>&1 || {
    echo "npm not found locally (needed to build the SPA). Use --builder root@<buildbox>." >&2
    exit 1
  }
  echo "==> building SPA + static linux/amd64 binaries locally"
  ( cd "$REPO_ROOT/src/web" && TOTEM_BUILD="$BUILD_STAMP" npm ci && TOTEM_BUILD="$BUILD_STAMP" npm run build )
  ( cd "$REPO_ROOT/src"
    for cmd in $CMDS; do
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -tags embed -trimpath \
          -ldflags "-s -w -X github.com/Niklasvdm/TotemAppIT/internal/buildinfo.Version=$BUILD_VERSION" \
          -o "$STAGE/$cmd" "./cmd/$cmd"
    done
  )
fi

echo "==> ensuring remote data dir"
ssh "$HOST" "mkdir -p '$DATA_DIR'"

echo "==> uploading binaries (atomic rename; safe while totemd runs)"
for cmd in $CMDS; do
  scp -q "$STAGE/$cmd" "$HOST:$BIN_DIR/.$cmd.new"
  ssh "$HOST" "chmod 0755 '$BIN_DIR/.$cmd.new' && mv -f '$BIN_DIR/.$cmd.new' '$BIN_DIR/$cmd'"
done

echo "==> uploading data (emoji map, images + attributions, animals.json)"
rsync -az "$REPO_ROOT/data/emoji.json"   "$HOST:$DATA_DIR/emoji.json"
rsync -az "$REPO_ROOT/data/animals.json" "$HOST:$DATA_DIR/animals.json"
rsync -az --delete "$REPO_ROOT/data/images/" "$HOST:$DATA_DIR/images/"

if [ "$INSTALL_UNIT" = 1 ]; then
  echo "==> ensuring 'totemd' system user (the unit runs as User=totemd)"
  ssh "$HOST" "getent group totemd >/dev/null || groupadd --system totemd
    id -u totemd >/dev/null 2>&1 || useradd --system --gid totemd --no-create-home --home-dir /var/lib/totemd --shell /usr/sbin/nologin totemd
    mkdir -p /etc/totemd"
  echo "==> installing systemd unit (configure the DB key separately — it is NOT shipped)"
  scp -q "$REPO_ROOT/deploy/totemd.service" "$HOST:/etc/systemd/system/totemd.service"
  ssh "$HOST" "systemctl daemon-reload"
fi

echo "==> fixing ownership + restarting totemd"
ssh "$HOST" "chown -R totemd:totemd '$DATA_DIR' 2>/dev/null || true; systemctl restart totemd"

echo "==> healthcheck"
if ssh "$HOST" "sleep 2; curl -fsS http://127.0.0.1:8683/healthz >/dev/null 2>&1"; then
  echo "  healthy ✓"
  ssh "$HOST" "journalctl -u totemd -n 20 --no-pager | grep -i warning || echo '  (no startup warnings)'"
else
  echo "  NOT healthy. On a FIRST-TIME box this is expected until you:"
  echo "    1. Configure the DB key (see deploy/totemd.service): either a systemd"
  echo "       credential, or an EnvironmentFile /etc/totemd/totemd.env with TOTEM_DB_KEY."
  echo "    2. Seed the DB:  totem-seed --db /var/lib/totemd/totem.db \\"
  echo "                       --data /var/lib/totemd/data/animals.json --merge"
  echo "    3. systemctl restart totemd"
  echo "  --- last log lines ---"
  ssh "$HOST" "journalctl -u totemd -n 6 --no-pager | tail -6" || true
fi

echo "done."
