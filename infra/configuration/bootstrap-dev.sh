#!/usr/bin/env bash
# =============================================================================
# bootstrap-dev.sh — install the Totem dev toolchain on a fresh Debian LXC.
#
# Installs: build-essential, git, sqlite3, Python 3, Go (pinned), Node.js LTS.
# Idempotent-ish: safe to re-run; it re-installs Go to the requested version and
# skips apt work already done.
#
# Args (positional, passed by the Terraform remote-exec provisioner):
#   $1  GO_VERSION   e.g. 1.23.4
#   $2  NODE_MAJOR   e.g. 22
#   $3  TIMEZONE     e.g. Europe/Brussels
#   $4  PUBLIC_HOST  e.g. dev.totem.nvdm.eu (this box's public hostname; may be empty)
#
# Requirements: Debian 12, run as root.
# =============================================================================
set -euo pipefail

GO_VERSION="${1:-1.26.8}"
NODE_MAJOR="${2:-22}"
TIMEZONE="${3:-Europe/Brussels}"
PUBLIC_HOST="${4:-}"
ARCH="amd64"

# Record this box's public host so the Vite dev server allowlists only it
# (never a cross-environment hostname). run-dev.sh sources this file.
if [ -n "$PUBLIC_HOST" ]; then
  echo "VITE_ALLOWED_HOSTS=${PUBLIC_HOST}" > /etc/totem-web.env
  echo "[0/7] public host: wrote /etc/totem-web.env (VITE_ALLOWED_HOSTS=${PUBLIC_HOST})"
fi

echo "[1/7] Timezone + apt base packages..."
ln -sf "/usr/share/zoneinfo/${TIMEZONE}" /etc/localtime || true
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y \
  ca-certificates curl gnupg git build-essential \
  sqlite3 libsqlite3-dev \
  python3 python3-venv python3-pip \
  rsync jq

echo "[2/7] Installing Go ${GO_VERSION}..."
GO_TARBALL="go${GO_VERSION}.linux-${ARCH}.tar.gz"
curl -fsSLo "/tmp/${GO_TARBALL}" "https://go.dev/dl/${GO_TARBALL}"
rm -rf /usr/local/go
tar -C /usr/local -xzf "/tmp/${GO_TARBALL}"
rm -f "/tmp/${GO_TARBALL}"
# Put Go (and the default GOPATH bin) on PATH for all login shells.
cat > /etc/profile.d/go.sh <<'EOF'
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
EOF
chmod +x /etc/profile.d/go.sh

echo "[3/7] Installing Node.js ${NODE_MAJOR}.x (NodeSource)..."
mkdir -p /etc/apt/keyrings
curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key |
  gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg
echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
  > /etc/apt/sources.list.d/nodesource.list
apt-get update -y
apt-get install -y nodejs

echo "[4/7] Versions:"
/usr/local/go/bin/go version
node --version
npm --version
python3 --version
sqlite3 --version

echo "[5/7] Hardening SSH to key-only (match the WAF/WireGuard house standard)..."
# The key is already injected by Terraform; make password SSH impossible so the
# root_password is console-only. prohibit-password is the Debian default for root
# anyway — this makes it explicit and also covers any non-root user added later.
install -d -m 755 /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/10-totem-dev.conf <<'EOF'
PasswordAuthentication no
PermitRootLogin prohibit-password
EOF
systemctl restart ssh 2>/dev/null || systemctl restart sshd 2>/dev/null || true

echo "[6/7] Where your code goes..."
mkdir -p /root/totem-it
echo "  -> rsync your working tree into /root/totem-it (see infra/sync.sh)"

echo "[7/7] Done. Log out and back in (or 'source /etc/profile.d/go.sh') to get go on PATH."
