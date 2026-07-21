#!/usr/bin/env bash
set -e

echo "🔄 Scribo servisi yeniden başlatılıyor..."
sudo systemctl restart scribo
echo "✅ Scribo servisi başarıyla yeniden başlatıldı!"
echo "----------------------------------------------------"
sudo systemctl status scribo --no-pager
