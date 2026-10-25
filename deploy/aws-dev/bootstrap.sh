#!/bin/bash
# EC2 user-data for Amazon Linux 2023 (arm64, t4g.small). Runs once at first boot.
# Installs docker, swap, and a systemd unit that brings the stack up on every start
# and pings Telegram when the API is healthy.
set -euxo pipefail

dnf install -y docker git
systemctl enable --now docker
mkdir -p /usr/local/lib/docker/cli-plugins /data/pg
curl -fsSL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-aarch64 \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

# 2 GB swap so Go image builds don't OOM
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab

cat > /usr/local/bin/dev-notify-ready.sh <<'EOF'
#!/bin/bash
set -a; source /opt/blueprint-backend/.env.dev; set +a
for i in $(seq 1 60); do
  curl -fsS http://127.0.0.1:8080/health >/dev/null 2>&1 && break
  sleep 5
done
curl -fsS "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
  -d chat_id="${TELEGRAM_CHAT_ID}" \
  -d text="Dev server is UP: ${DEV_API_URL}" >/dev/null || true
EOF
chmod +x /usr/local/bin/dev-notify-ready.sh

cat > /etc/systemd/system/blueprint-dev.service <<'EOF'
[Unit]
Description=Blueprint dev stack
After=docker.service network-online.target
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/blueprint-backend
ExecStart=/usr/bin/docker compose -f deploy/aws-dev/docker-compose.dev.yml --env-file .env.dev up -d
ExecStartPost=-/usr/local/bin/dev-notify-ready.sh
ExecStop=/usr/bin/docker compose -f deploy/aws-dev/docker-compose.dev.yml --env-file .env.dev stop
TimeoutStopSec=120

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable blueprint-dev.service   # starts on every boot; first start is done manually after cloning
