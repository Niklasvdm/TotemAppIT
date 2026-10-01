#!/usr/bin/env bash
# =============================================================================
# sync.sh — push the local working tree to the dev LXC for a fast build loop.
#
#   ./infra/sync.sh root@192.168.10.7          # sync to /root/totem-it
#   ./infra/sync.sh root@192.168.10.7 /opt/x   # sync to a custom dir
#
# Excludes git, build artifacts, caches and terraform state so only source
# crosses the wire. Re-run after every local edit; it's incremental.
# =============================================================================
set -euo pipefail

DEST_HOST="${1:?usage: sync.sh user@host [remote_dir]}"
REMOTE_DIR="${2:-/root/totem-it}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

rsync -az --delete \
  --exclude '.git/' \
  --exclude 'build/' \
  --exclude '__pycache__/' \
  --exclude '*.db' \
  --exclude 'node_modules/' \
  --exclude 'web/dist/' \
  --exclude 'infra/lxc/.terraform/' \
  --exclude 'infra/**/*.tfstate*' \
  --exclude '*.tfvars' \
  "${REPO_ROOT}/" "${DEST_HOST}:${REMOTE_DIR}/"

echo "synced ${REPO_ROOT} -> ${DEST_HOST}:${REMOTE_DIR}"
echo "next: ssh ${DEST_HOST} 'cd ${REMOTE_DIR} && python3 scripts/db_test.py'"
