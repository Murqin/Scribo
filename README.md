# Scribo 🎙️ (Go / Golang Edition)

<p align="center">
  <img src="assets/mascot.jpg" alt="Scribo Mascot" width="200" style="border-radius: 50%;"/>
</p>

> **A high-performance, ultra-lightweight Telegram bot written in Go (Golang) running 24/7 on Oracle Cloud Infrastructure (OCI). Intercepts voice messages, transcribes, formats, and transforms them using Google Gemini API (Free Tier) with interactive fallback to OpenRouter (Paid Gemini 3.6 Flash), using under 15 MB RAM.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)
[![Tech: Go](https://img.shields.io/badge/Language-Go%201.26-00ADD8?style=flat-square&logo=go&logoColor=white)](#)
[![Model: Gemini 3.6 Flash](https://img.shields.io/badge/Model-Gemini%203.6%20Flash-red?style=flat-square&logo=google&logoColor=white)](#)
[![Infrastructure: Oracle Cloud](https://img.shields.io/badge/Infrastructure-Oracle%20Cloud%20Always%20Free-black?style=flat-square&logo=oracle&logoColor=white)](#)

---

## ⚡ Performance Highlights (Go vs Python)
- **RAM Usage:** **~8-12 MB** (vs ~60 MB in Python).
- **Binary Size:** Single **8.9 MB** standalone static binary. Zero external runtime dependencies.
- **Speed:** Instant startup with zero cold-starts and high-concurrency Goroutine execution.

---

## ✨ Features

- **🆓 Google Free Tier First Strategy:** Tries official Google Gemini API (Free Tier) first. If rate limits or errors occur, prompts the user interactively before falling back to paid OpenRouter service.
- **🎙️ Direct Audio Modality:** Bypasses conventional, slow speech-to-text converters. Encodes raw `.ogg` voice buffers to base64 and streams them directly to Gemini's native audio-sensing model.
- **🏷️ Smart Interactive Modes (9 Modes):**
  - **📝 Özet (TL;DR):** Generates a concise, 1st-person Turkish summary without intro/outro text.
  - **✍️ Transkript:** Resolves a precise, word-for-word literal transcription.
  - **🛠️ Düzelt:** Transcribes the audio while correcting syntax and spelling errors.
  - **📓 Obsidian Notu:** Creates a copy-paste ready structured Obsidian note.
  - **📰 Blog Yazısı:** Converts voice notes to blog posts in a markdown format.
  - **🧠 Fikir Geliştir:** Analyzes concepts/ideas to generate a structured project report (SWOT & Next Steps).
  - **📱 Sosyal Medya:** Generates ready-to-use posts for LinkedIn and X (Twitter Thread).
  - **🇬🇧 İngilizce Çeviri:** Natively translates Turkish audio into fluent English.
  - **🎯 Master Prompt:** Synthesizes an expert, reusable Master Prompt for ChatGPT/Claude/Gemini.

---

## 🏗️ Building & Deploying

### Build Locally
```bash
make build
# Creates single binary: ./scribo
```

### Run Tests
```bash
make test
```

### Environment Configuration
```env
TELEGRAM_TOKEN=your_telegram_bot_token
OPENROUTER_API_KEY=your_openrouter_developer_api_key
GEMINI_API_KEY=your_google_ai_studio_free_tier_api_key
DEFAULT_PROVIDER=google
GOOGLE_MODEL=gemini-3.6-flash
OPENROUTER_MODEL=google/gemini-3.6-flash
ALLOWED_USER_ID=your_numerical_telegram_user_id
```

### Run 7/24 Service on Oracle VPS (Automated Setup)
```bash
chmod +x setup_service.sh
sudo ./setup_service.sh
```

---

## 📂 Project Architecture

```text
scribo/
├── main.go               # Primary Go application entrypoint
├── config/               # Environment configuration loader (.env support)
├── mode/                 # 9 Interactive modes & inline keyboard builder
├── provider/             # Google Gemini Direct & OpenRouter API clients
├── setup_keepalive.sh    # Oracle Cloud idle reclaim prevention setup script
├── Makefile              # Build & test helpers
├── go.mod                # Go module specification
├── PYTHON_SNAPSHOT.md    # Reference snapshot of original Python implementation
└── README.md
```

---

## 📄 License

Licensed under the terms of the MIT License. See [LICENSE](LICENSE) for more details.
