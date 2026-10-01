#!/usr/bin/env bash
################################################################################
# harden.sh
#
# Purpose : Apply OS-level hardening to the totemd LXC container.
#           Run once after initial deployment (Terraform apply); safe to re-run
#           (idempotent). Same shape as the WireGuard/WAF repos' harden scripts.
#
# Usage   : harden.sh -h HOST [OPTIONS]
#
# Required:
#   -h HOST    Container IP or hostname
#
# Optional:
#   -u USER           SSH user (default: root). If not root, commands run via
#                      `sudo -n` — the user must have passwordless sudo; this
#                      script never prompts for a sudo password.
#   -p PORT           SSH port (default: 22)
#   -i KEY            SSH private key (default: ~/.ssh/id_ed25519)
#   --ssh-allow CIDR  Skip LAN auto-detection; restrict SSH to this CIDR.
#                      Required on a VPS (no "LAN" to detect).
#                      Example: --ssh-allow 203.0.113.7/32
#   --app-port PORT   totemd's listen port (default: 8080).
#   --app-allow CIDR  Allow inbound to --app-port from this CIDR (e.g. the WAF
#                      host). Omit entirely when totemd is reached over localhost
#                      or a tunnel only — then the app port stays closed to the
#                      network, which is the default and most secure posture.
#   --data-dir DIR    totemd data dir to lock down (default: /var/lib/totemd).
#
# What this script does:
#   1.  Disable root password login (SSH key only)
#   2.  Disable SSH password authentication entirely
#   3.  Restrict SSH to the Ed25519 host key algorithm
#   4.  Set SSH idle timeout (ClientAliveInterval)
#   5.  Disable unused services (avahi, bluetooth if present)
#   6.  Enable unattended-upgrades for automatic security patches
#   7.  Install and configure fail2ban for SSH brute-force protection
#   8.  Restrict the totemd data dir permissions (700 dir / 600 files)
#   9.  iptables INPUT lockdown: established + loopback + SSH(LAN) +
#        optional app-port(allow-CIDR), drop the rest
#   10. Persist iptables rules via iptables-persistent
################################################################################

set -euo pipefail

SSH_HOST=""
SSH_USER="root"
SSH_PORT=22
SSH_KEY="${HOME}/.ssh/id_ed25519"
SSH_ALLOW_CIDR=""
APP_PORT=8080
APP_ALLOW_CIDR=""
DATA_DIR="/var/lib/totemd"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h) SSH_HOST="$2";          shift 2 ;;
    -u) SSH_USER="$2";          shift 2 ;;
    -p) SSH_PORT="$2";          shift 2 ;;
    -i) SSH_KEY="$2";           shift 2 ;;
    --ssh-allow) SSH_ALLOW_CIDR="$2"; shift 2 ;;
    --app-port)  APP_PORT="$2";       shift 2 ;;
    --app-allow) APP_ALLOW_CIDR="$2"; shift 2 ;;
    --data-dir)  DATA_DIR="$2";       shift 2 ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 -h HOST [-u USER] [-p PORT] [-i KEY] [--ssh-allow CIDR] [--app-port PORT] [--app-allow CIDR] [--data-dir DIR]"
      exit 1 ;;
  esac
done

if [[ -z "${SSH_HOST}" ]]; then
  echo "Error: -h HOST is required."
  exit 1
fi

SSH_OPTS="-p ${SSH_PORT} -i ${SSH_KEY} -o StrictHostKeyChecking=accept-new -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=60 -o ServerAliveCountMax=4"
SSH_TARGET="${SSH_USER}@${SSH_HOST}"

if [[ "${SSH_USER}" == "root" ]]; then
  REMOTE_SUDO=""
else
  REMOTE_SUDO="sudo -n"
fi

# Restrict SSH to a CIDR. On a LAN-attached Proxmox LXC we auto-detect the /24
# from eth0. On a VPS there is no LAN, so --ssh-allow must be given explicitly.
if [[ -n "${SSH_ALLOW_CIDR}" ]]; then
  LAN_SUBNET="${SSH_ALLOW_CIDR}"
  echo "Using explicit SSH-allow CIDR: ${LAN_SUBNET}"
else
  # shellcheck disable=SC2086
  LAN_SUBNET="$(ssh ${SSH_OPTS} "${SSH_TARGET}" "
    ip -4 addr show eth0 | grep 'inet ' | awk '{print \$2}' | \
    awk -F. '{print \$1\".\"\$2\".\"\$3\".0/24\"}'
  ")"
  echo "Detected LAN subnet: ${LAN_SUBNET}"
fi

echo ""
echo "Applying hardening to ${SSH_TARGET}..."
echo "  SSH allow : ${LAN_SUBNET}"
echo "  app port  : ${APP_PORT} (${APP_ALLOW_CIDR:-closed to network — localhost/tunnel only})"
echo "  data dir  : ${DATA_DIR}"

# shellcheck disable=SC2086
ssh ${SSH_OPTS} "${SSH_TARGET}" "${REMOTE_SUDO} APP_PORT='${APP_PORT}' APP_ALLOW_CIDR='${APP_ALLOW_CIDR}' LAN_SUBNET='${LAN_SUBNET}' DATA_DIR='${DATA_DIR}' bash -s" <<'REMOTE'
set -euo pipefail

echo ""
echo "=== [1/10] SSH: disable root password login ==="
sed -i 's/^#*PermitRootLogin.*/PermitRootLogin without-password/' /etc/ssh/sshd_config
grep "PermitRootLogin" /etc/ssh/sshd_config

echo ""
echo "=== [2/10] SSH: disable password authentication ==="
sed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
sed -i 's/^#*KbdInteractiveAuthentication.*/KbdInteractiveAuthentication no/' /etc/ssh/sshd_config
grep "PasswordAuthentication" /etc/ssh/sshd_config

echo ""
echo "=== [3/10] SSH: restrict to Ed25519 host key algorithm ==="
if ! grep -q "^HostKeyAlgorithms" /etc/ssh/sshd_config; then
  echo "HostKeyAlgorithms ssh-ed25519" >> /etc/ssh/sshd_config
fi

echo ""
echo "=== [4/10] SSH: set idle timeout (10 minutes) ==="
sed -i 's/^#*ClientAliveInterval.*/ClientAliveInterval 120/' /etc/ssh/sshd_config
sed -i 's/^#*ClientAliveCountMax.*/ClientAliveCountMax 5/' /etc/ssh/sshd_config
systemctl reload ssh.service 2>/dev/null || systemctl reload sshd.service 2>/dev/null || true
echo "  SSH reloaded."

echo ""
echo "=== [5/10] Disable unused services ==="
for svc in avahi-daemon bluetooth; do
  if systemctl list-unit-files "${svc}.service" &>/dev/null; then
    systemctl disable --now "${svc}.service" 2>/dev/null || true
    echo "  Disabled: ${svc}"
  fi
done

echo ""
echo "=== [6/10] Enable unattended-upgrades (security only) ==="
DEBIAN_FRONTEND=noninteractive apt-get install -y -q unattended-upgrades apt-listchanges
cat > /etc/apt/apt.conf.d/50unattended-upgrades-totem <<'EOF'
Unattended-Upgrade::Allowed-Origins {
    "${distro_id}:${distro_codename}-security";
};
Unattended-Upgrade::AutoFixInterruptedDpkg "true";
Unattended-Upgrade::MinimalSteps "true";
Unattended-Upgrade::Remove-Unused-Dependencies "true";
Unattended-Upgrade::Automatic-Reboot "false";
EOF
systemctl enable --now unattended-upgrades.service
echo "  unattended-upgrades enabled."

echo ""
echo "=== [7/10] Install and configure fail2ban (SSH) ==="
DEBIAN_FRONTEND=noninteractive apt-get install -y -q fail2ban
cat > /etc/fail2ban/jail.d/sshd-totem.conf <<'EOF'
[sshd]
enabled  = true
port     = ssh
maxretry = 5
findtime = 600
bantime  = 600
EOF
systemctl enable --now fail2ban.service
echo "  fail2ban enabled."

echo ""
echo "=== [8/10] Restrict totemd data dir permissions ==="
# The encrypted DB lives here; only the service user (and root) should read it.
if [[ -d "${DATA_DIR}" ]]; then
  chmod 700 "${DATA_DIR}"
  find "${DATA_DIR}" -type f -exec chmod 600 {} +
  echo "  ${DATA_DIR}: 700, files: 600"
else
  echo "  ${DATA_DIR} does not exist yet — skipping (create it at deploy time)."
fi

echo ""
echo "=== [9/10] Configure iptables INPUT chain ==="
iptables -F INPUT
iptables -A INPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -p tcp --dport 22 -s "${LAN_SUBNET}" -j ACCEPT
# Open the app port ONLY if an allow-CIDR was given (e.g. the WAF host). By
# default totemd is reached over localhost/tunnel, so the port stays closed.
if [[ -n "${APP_ALLOW_CIDR}" ]]; then
  iptables -A INPUT -p tcp --dport "${APP_PORT}" -s "${APP_ALLOW_CIDR}" -j ACCEPT
  echo "  app port ${APP_PORT} allowed from ${APP_ALLOW_CIDR}"
fi
iptables -A INPUT -j DROP
echo "  INPUT chain:"
iptables -L INPUT -v --line-numbers -n

echo ""
echo "=== [10/10] Persist iptables rules ==="
DEBIAN_FRONTEND=noninteractive apt-get install -y -q iptables-persistent
netfilter-persistent save
systemctl enable netfilter-persistent.service
echo "  Rules persisted and service enabled."

echo ""
echo "============================================================"
echo " Hardening complete."
echo "  SSH key-only : yes"
echo "  SSH timeout  : 10 min"
echo "  fail2ban     : enabled (SSH)"
echo "  auto-updates : enabled (security only)"
echo "  iptables     : INPUT restricted (LAN SSH${APP_ALLOW_CIDR:+ + app port})"
echo "  ${DATA_DIR}: 700 / files 600"
echo "============================================================"
REMOTE
