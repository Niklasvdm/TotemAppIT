#!/usr/bin/env bash
# =============================================================================
# backup.sh — snapshot the encrypted totem database, with rotation.
#
# The DB is Adiantum-encrypted at rest, so the backup is ciphertext and safe to
# store or ship offsite — but it is ONLY restorable with the same TOTEM_DB_KEY,
# so back the key up separately (and never in the same place).
#
#   From the host:
#     ./deploy/backup.sh                     # hot copy -> /var/backups/totemd, keep 14
#     KEEP=30 BACKUP_DIR=/mnt/bk ./deploy/backup.sh
#     ./deploy/backup.sh --stop              # stop totemd around the copy (consistent)
#   From your workstation (runs over SSH on the host):
#     ./deploy/backup.sh root@192.168.10.195
#     KEEP=30 ./deploy/backup.sh root@prod-host --stop
#
# Restore:  systemctl stop totemd; gunzip -c <backup>.db.gz > /var/lib/totemd/totem.db; systemctl start totemd
#
# Cron on the host (daily 03:30):  30 3 * * *  /opt/totem-it/deploy/backup.sh >>/var/log/totem-backup.log 2>&1
# =============================================================================
set -euo pipefail

# If the first argument is user@host, re-run this script on that host over SSH
# (forwarding the tunable env vars) and exit. Otherwise run locally.
if [ "${1:-}" ] && [[ "$1" == *@* ]]; then
  REMOTE_HOST="$1"; shift
  envp=""
  [ -n "${DB:-}" ]         && envp+="DB='$DB' "
  [ -n "${BACKUP_DIR:-}" ] && envp+="BACKUP_DIR='$BACKUP_DIR' "
  [ -n "${KEEP:-}" ]       && envp+="KEEP='$KEEP' "
  exec ssh "$REMOTE_HOST" "${envp}bash -s -- $*" < "$0"
fi

DB="${DB:-/var/lib/totemd/totem.db}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/totemd}"
KEEP="${KEEP:-14}"
MODE="${1:-}"

[ -f "$DB" ] || { echo "no database at $DB" >&2; exit 1; }
mkdir -p "$BACKUP_DIR"
ts="$(date +%Y%m%d-%H%M%S)"
dest="$BACKUP_DIR/totem-$ts.db.gz"

stopped=0
tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; [ "$stopped" = 1 ] && systemctl start totemd || true; }
trap cleanup EXIT

if [ "$MODE" = "--stop" ] && command -v systemctl >/dev/null; then
  echo "==> stopping totemd for a consistent copy"
  systemctl stop totemd && stopped=1
fi

# Default (rollback-journal) SQLite: the committed state is the .db file alone;
# a -journal only exists mid-transaction and is rolled back on open. Copy first,
# then compress, so the window where the file is read is as short as possible.
cp -p "$DB" "$tmp/totem.db"
gzip -c "$tmp/totem.db" > "$dest"
echo "wrote $dest ($(du -h "$dest" | cut -f1))"

# Retention: keep the newest $KEEP, delete the rest.
ls -1t "$BACKUP_DIR"/totem-*.db.gz 2>/dev/null | tail -n +"$((KEEP + 1))" | xargs -r rm -f
echo "retained newest $KEEP backup(s) in $BACKUP_DIR"
