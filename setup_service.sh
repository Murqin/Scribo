#!/usr/bin/env bash
set -e

echo "🚀 Scribo Telegram Bot (Go) 7/24 Systemd Servisi Kuruluyor..."

WORKING_DIR="$(pwd)"
SERVICE_NAME="scribo"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

if [ ! -f "${WORKING_DIR}/scribo" ]; then
    echo "⚙️ Executable bulunamadı, proje derleniyor..."
    make build || go build -o scribo main.go
fi

if [ ! -f "${WORKING_DIR}/.env" ]; then
    echo "⚠️ UYARI: ${WORKING_DIR}/.env dosyası bulunamadı!"
    echo "   Lütfen .env dosyasını oluşturup API anahtarlarınızı ekleyin."
fi

CURRENT_USER="$(whoami)"

echo "📝 Systemd servisi oluşturuluyor: ${SERVICE_FILE} (Kullanıcı: ${CURRENT_USER})"

sudo bash -c "cat <<EOF > ${SERVICE_FILE}
[Unit]
Description=Scribo Telegram Bot (Go Edition) 24/7 Service
After=network.target

[Service]
Type=simple
User=${CURRENT_USER}
WorkingDirectory=${WORKING_DIR}
ExecStart=${WORKING_DIR}/scribo
Restart=always
RestartSec=5
EnvironmentFile=-${WORKING_DIR}/.env

[Install]
WantedBy=multi-user.target
EOF"

echo "🔄 Systemd servisleri güncelleniyor ve başlatılıyor..."
sudo systemctl daemon-reload
sudo systemctl enable --now "${SERVICE_NAME}"

echo "✅ Scribo 7/24 servisi başarıyla kuruldu ve başlatıldı!"
echo "----------------------------------------------------"
sudo systemctl status "${SERVICE_NAME}" --no-pager
