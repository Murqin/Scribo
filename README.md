# Scribo 🎙️ (Go / Golang Edition)

<p align="center">
  <img src="assets/mascot.jpg" alt="Scribo Mascot" width="180" style="border-radius: 50%;"/>
</p>

> **Scribo is a high-performance, ultra-lightweight Telegram bot written in Go (Golang). It captures voice notes, MP3s, and audio files, processing them natively using Google Gemini AI (Free Tier) with an interactive OpenRouter fallback. Runs 24/7 on Oracle Cloud VPS consuming under 10 MB RAM.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)
[![Tech: Go](https://img.shields.io/badge/Language-Go-00ADD8?style=flat-square&logo=go&logoColor=white)](#)
[![Model: Gemini 3.6 Flash](https://img.shields.io/badge/Model-Gemini%203.6%20Flash-red?style=flat-square&logo=google&logoColor=white)](#)
[![Infrastructure: Oracle Cloud](https://img.shields.io/badge/Infrastructure-Oracle%20Cloud%20Always%20Free-black?style=flat-square&logo=oracle&logoColor=white)](#)

---

## ⚡ Performance Highlights (Go Architecture)

- **Memory Footprint:** **~6-10 MB RAM** (vs ~60 MB in Python).
- **Binary Size:** **~6.4 MB** standalone static binary. Zero runtime dependencies.
- **Startup Speed:** Instant startup (<10ms) with zero cold-starts and high-concurrency Goroutines.
- **Portless & SSL-Free:** Uses 100% Outbound Telegram Long Polling (no domain, no SSL, no open ports needed).

---

## ✨ Features

- **🆓 Google Free Tier First Strategy:** Direct connection to official Google Gemini API ($0.00). If rate limits occur, interactively prompts the user for OpenRouter fallback.
- **🎙️ Native Audio Processing:** Streams raw audio buffers directly to Gemini's multi-modal audio engine. Supports Voice Notes (`.ogg`), Audio (`.mp3`, `.m4a`, `.wav`), and Document audio files up to 20 MB.
- **🧩 100% JSON-Driven Modes (`modes.json`):** Prompt instructions and Telegram inline keyboard buttons are managed dynamically via JSON without recompiling code!
- **⚡ Typing Chat Action Indicator:** Sends real-time "typing..." status while downloading audio and generating AI responses.
- **🛡️ Security & Privacy:** Restricts access via `ALLOWED_USER_ID`. Hardened Systemd service security.
- **📦 Zero-Code Distribution:** Ready-to-run release archives for non-developer end-users (`make release`).

---

## 🚀 Quick Start for End-Users (Zero-Code)

Non-developers can run Scribo without installing Go or compiling code:

1. Download the pre-compiled release archive for your server architecture (`amd64` or `arm64`).
2. Extract and enter the directory:
   ```bash
   tar -xzvf scribo-linux-amd64.tar.gz
   cd scribo
   ```
3. Edit your API keys in `.env`:
   ```bash
   nano .env
   ```
4. Run the 1-command 7/24 service installer:
   ```bash
   sudo ./setup_service.sh
   ```

---

## ⚙️ Environment Configuration (`.env`)

```env
# Telegram Bot Token (from @BotFather)
TELEGRAM_TOKEN=123456789:ABCdefGHIjklMNOpqrsTUVwxyz

# Authorized Telegram User ID (from @userinfobot)
ALLOWED_USER_ID=123456789

# AI Provider API Keys
GEMINI_API_KEY=your_google_ai_studio_api_key
OPENROUTER_API_KEY=your_openrouter_api_key

# Default Provider (google or openrouter)
DEFAULT_PROVIDER=google

# Models
GOOGLE_MODEL=gemini-3.6-flash
OPENROUTER_MODEL=google/gemini-3.6-flash
```

---

## 🧩 Custom Modes & Prompts (`modes.json`)

To customize button names or add custom AI prompts, create a `modes.json` file in the working directory (or copy `modes.example.json`):

```json
{
  "tldr": {
    "label": "📝 Özet",
    "prompt": "Sen profesyonel bir ses analiz asistanısın..."
  },
  "trans": {
    "label": "✍️ Transkript",
    "prompt": "Sen hassas bir ses deşifre sistemisin..."
  },
  "fix": {
    "label": "🛠️ Düzelt",
    "prompt": "Sen uzman bir editör ve dil düzeltme sistemisin..."
  }
}
```

Scribo automatically detects `modes.json` at startup, re-creates the Telegram inline keyboard dynamically, and applies your custom prompts!

---

## 🛠️ Developer Commands

### Test Codebase
```bash
make test
```

### Build Binary Locally
```bash
make build
```

### Build Release Archives
```bash
make release
# Generates release packages in dist/ directory
```

---

## 📊 Monitoring Logs (Systemd)

```bash
# Follow live logs
sudo journalctl -u scribo -f

# View last 50 log entries
sudo journalctl -u scribo -n 50 --no-pager
```

---

## 📄 License

Licensed under the terms of the **MIT License**. See [LICENSE](LICENSE) for details.
