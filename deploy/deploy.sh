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
BIN_DIR=/usr/local/bin
DATA_DIR=/var/lib/totemd/data
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

CMDS="totemd totem-admin totem-seed"

if [ -n "$BUILDER" ]; then
  echo "==> building on remote builder $BUILDER (no local Go needed)"
  ssh "$BUILDER" "mkdir -p ~/totem-build/src"
  rsync -az --delete --exclude node_modules --exclude .git "$REPO_ROOT/src/" "$BUILDER:~/totem-build/src/"
  ssh "$BUILDER" "source /etc/profile.d/go.sh 2>/dev/null || true
    cd ~/totem-build/src
    for c in $CMDS; do
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o \"../\$c\" \"./cmd/\$c\"
    done"
  for cmd in $CMDS; do scp -q "$BUILDER:~/totem-build/$cmd" "$STAGE/$cmd"; done
else
  command -v go >/dev/null 2>&1 || {
    echo "Go not found locally. Either install it (./deploy/update.sh installs it)," >&2
    echo "or build on a host that has Go:  ./deploy/deploy.sh $HOST --builder root@<buildbox>" >&2
    exit 1
  }
  echo "==> building static linux/amd64 binaries locally"
  ( cd "$REPO_ROOT/src"
    for cmd in $CMDS; do
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -trimpath -ldflags "-s -w" -o "$STAGE/$cmd" "./cmd/$cmd"
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
  echo "==> installing systemd unit (configure the DB key separately — it is NOT shipped)"
  scp -q "$REPO_ROOT/deploy/totemd.service" "$HOST:/etc/systemd/system/totemd.service"
  ssh "$HOST" "systemctl daemon-reload"
fi

echo "==> fixing ownership + restarting totemd"
ssh "$HOST" "chown -R totemd:totemd '$DATA_DIR' 2>/dev/null || true; systemctl restart totemd"

echo "==> healthcheck"
ssh "$HOST" "sleep 2; systemctl is-active totemd; curl -fsS http://127.0.0.1:8683/healthz && echo"

echo "==> startup warnings (should be none):"
ssh "$HOST" "journalctl -u totemd -n 20 --no-pager | grep -i warning || echo '  (none)'"

echo "done."
