#!/usr/bin/env bash
set -e

echo "🗑️ Scribo Systemd servisi kaldırılıyor..."

sudo systemctl stop scribo 2>/dev/null || true
sudo systemctl disable scribo 2>/dev/null || true
sudo rm -f /etc/systemd/system/scribo.service
sudo systemctl daemon-reload

echo "✅ Scribo 7/24 servisi sistemden tamamen kaldırıldı!"
