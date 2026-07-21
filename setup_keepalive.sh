#!/usr/bin/env bash
set -e

echo "🚀 Oracle Cloud Keep-Alive Kurulumu Başlatılıyor..."

# 1. Sistem paketlerini güncelle ve stress-ng yükle
sudo apt update && sudo apt install -y stress-ng htop

# 2. Oracle Keep-Alive systemd servisini oluştur
sudo bash -c 'cat <<EOF > /etc/systemd/system/oracle-keepalive.service
[Unit]
Description=Oracle Cloud Idle Reclaim Prevention
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/stress-ng --cpu 1 --cpu-load 20 --timeout 0
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF'

# 3. Servisi kaydet ve başlat
sudo systemctl daemon-reload
sudo systemctl enable --now oracle-keepalive

echo "✅ Oracle Keep-Alive servisi başarıyla kuruldu ve başlatıldı!"
sudo systemctl status oracle-keepalive --no-pager
