#!/usr/bin/env bash
# One-time server preparation for an Ubuntu 24.04 LTS VDS.
# Run as ROOT on the SERVER:
#   bash server-bootstrap.sh
#
# Does: system update, unattended security upgrades, fail2ban, ufw firewall
# (22/80/443), conservative SSH hardening, swap, a dedicated 'tessera' app user,
# Docker + compose, log rotation, and the /opt/tessera app directory.
#
# Deliberately does NOT disable root SSH login (you log in as root) and does NOT
# change the SSH port — see the printed note to do that yourself afterwards so a
# mistake can't lock you out mid-run.
set -euo pipefail

if [[ $EUID -ne 0 ]]; then echo "Run as root."; exit 1; fi

APP_USER="tessera"
APP_DIR="/opt/tessera"

echo "==> [1/9] System update"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y && apt-get full-upgrade -y

echo "==> [2/9] Base packages (unattended-upgrades, fail2ban, ufw)"
apt-get install -y unattended-upgrades fail2ban ufw ca-certificates curl gnupg
dpkg-reconfigure -f noninteractive unattended-upgrades || true

echo "==> [3/9] fail2ban (SSH brute-force protection)"
cat > /etc/fail2ban/jail.d/sshd.local <<'EOF'
[sshd]
enabled = true
backend = systemd
maxretry = 5
bantime = 1h
findtime = 10m
EOF
systemctl enable --now fail2ban
systemctl restart fail2ban

echo "==> [4/9] Firewall (deny incoming; allow SSH + HTTP/HTTPS)"
ufw --force default deny incoming
ufw --force default allow outgoing
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
# NOTE: Docker writes its own iptables rules that bypass ufw. We only ever
# publish 80/443 (Caddy); Postgres/backend have no host ports, so they stay
# unreachable from the internet regardless. Never add `ports:` to the DB.

echo "==> [5/9] SSH hardening (key-only; root login kept, key-only)"
# Conservative: disable password auth (your key already works) but keep root
# login via key so this session and future ones don't get locked out.
cat > /etc/ssh/sshd_config.d/99-tessera-hardening.conf <<'EOF'
PasswordAuthentication no
PubkeyAuthentication yes
PermitRootLogin prohibit-password
KbdInteractiveAuthentication no
EOF
systemctl restart ssh || systemctl restart sshd || true

echo "==> [6/9] Swap (2G insurance for Postgres spikes)"
if ! swapon --show | grep -q '/swapfile'; then
  fallocate -l 2G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

echo "==> [7/9] Docker Engine + compose plugin"
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
fi

echo "==> [8/9] Docker daemon: log rotation + IPv6 egress"
# IPv6 is needed because some VDS providers (Timeweb) block outbound IPv4 SMTP
# (25/465/587); mail servers like smtp.yandex.ru are reachable over IPv6. With
# ipv6 + ip6tables the container's ULA is masqueraded to the host's global IPv6.
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'EOF'
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "3" },
  "ipv6": true,
  "fixed-cidr-v6": "fd00:dead:beef::/64",
  "ip6tables": true
}
EOF
systemctl restart docker

echo "==> [9/9] Dedicated app user '${APP_USER}' + ${APP_DIR}"
if ! id -u "${APP_USER}" >/dev/null 2>&1; then
  adduser --disabled-password --gecos "" "${APP_USER}"   # own home at /home/${APP_USER}
fi
usermod -aG docker "${APP_USER}"
mkdir -p "${APP_DIR}/apks"
chown -R "${APP_USER}:${APP_USER}" "${APP_DIR}"

cat <<EOF

==> Done. App user '${APP_USER}' (home /home/${APP_USER}), app dir ${APP_DIR}.
    Run the stack as that user so it isn't root:  su - ${APP_USER}

    Next:
      1) Copy deploy/{docker-compose.yml,Caddyfile,.env.example} into ${APP_DIR}.
      2) (as ${APP_USER}) cp .env.example .env && chmod 600 .env && edit secrets.
      3) Transfer the image tarball, then: docker load -i tessera-images-*.tar.gz
      4) docker compose up -d  &&  docker compose exec backend /migrate

    OPTIONAL — change the SSH port yourself (NOT done here, to avoid lockout):
      ufw allow <PORT>/tcp
      echo 'Port <PORT>' > /etc/ssh/sshd_config.d/10-port.conf
      systemctl restart ssh
      # then re-test login on the new port BEFORE closing this session,
      # and afterwards: ufw delete allow OpenSSH
EOF
