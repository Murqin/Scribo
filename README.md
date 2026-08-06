# Scribo 🎙️ (Go / Golang Edition)

<p align="center">
  <img src="assets/mascot.jpg" alt="Scribo Mascot" width="180" style="border-radius: 50%;"/>
</p>

> **Scribo is a high-performance, ultra-lightweight Telegram bot written in Go (Golang). It captures voice notes, MP3s, audio files, and videos, processing them natively using Google Gemini AI (Free Tier) with an interactive OpenRouter fallback. Runs 24/7 on a VPS, a home server or a container consuming under 10 MB RAM.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)
[![Tech: Go](https://img.shields.io/badge/Language-Go-00ADD8?style=flat-square&logo=go&logoColor=white)](#)
[![Model: Gemini 3.6 Flash](https://img.shields.io/badge/Model-Gemini%203.6%20Flash-red?style=flat-square&logo=google&logoColor=white)](#)
[![Infrastructure: Cross-Platform](https://img.shields.io/badge/Infrastructure-Linux%20%7C%20Windows%20%7C%20macOS-blue?style=flat-square)](#)
[![Container: GHCR](https://img.shields.io/badge/Container-ghcr.io-2496ED?style=flat-square&logo=docker&logoColor=white)](https://github.com/Murqin/Scribo/pkgs/container/scribo)

**🌍 Language:** **English** · [Türkçe](README.tr.md)

---

## ⚡ Performance Highlights (Go Architecture)

- **Memory Footprint:** **~6-10 MB RAM** (vs ~60 MB in Python runtime).
- **Binary Size:** **~6.6 MB** standalone static binary. Zero runtime dependencies.
- **Startup Speed:** Instant startup (<10ms local init) with zero cold-starts and high-concurrency Goroutines.
- **Portless & SSL-Free:** Uses 100% Outbound Telegram Long Polling (no domain, no SSL, no open ports needed).

---

## 📸 Screenshots & Demo

<p align="center">
  <img src="assets/demo-preview.png" alt="Scribo Demo Preview" width="80%" style="border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.15);"/>
</p>

---

## ✨ Features

- **🆓 Google Free Tier First Strategy:** Direct connection to official Google Gemini API ($0.00). If rate limits occur, interactively prompts the user for OpenRouter fallback.
- **🎙️ Native Audio & Video Processing:** Streams raw media buffers directly to Gemini's multi-modal engine. Supports Voice Notes (`.ogg`), Audio (`.mp3`, `.m4a`, `.wav`, `.aac`, `.flac`), Video and round Video Messages (`.mp4`, `.mov`, `.webm`, `.avi`, `.mpeg`, `.wmv`, `.flv`, `.3gp`), and the same files sent as Documents — up to 20 MB. Video is processed by Google only; there is no OpenRouter fallback for it.
- **📋 Per-Mode Output Formatting:** Every mode declares how its output is rendered — `code` (default) wraps the reply in Telegram `<code>` tags for instant 1-tap clipboard copying, `markdown` renders the model's markdown as real Telegram formatting for prose modes, `plain` sends it verbatim.
- **💸 Spending Ceiling:** Optional daily and monthly USD caps (`DAILY_COST_LIMIT` / `MONTHLY_COST_LIMIT`) on paid OpenRouter calls. Once a cap is reached the paid fallback is no longer offered and the refusal names the setting behind it; remaining budget is shown in the usage summary. The counter is restored from the history file at startup, so restarting the bot does not hand out a fresh allowance.
- **🗂️ Persistent History & `/son`:** Every finished output is appended to a JSONL file (`HISTORY_FILE`, default `scribo_history.jsonl`). `/son` replays the most recent output of the chat — rendered in its original mode's format — even after a restart. One JSON object per line, no database and no extra dependency.
- **🌍 Full Turkish / English Localization:** `SCRIBO_LANG` (or `LANG`) switches both the Telegram interface and the prompts sent to the model, so the bot answers in the language you picked instead of just labelling Turkish answers in English. Catalogs are embedded in the binary; an unset or unknown language falls back to Turkish.
- **🧩 100% JSON-Driven Modes (`modes.json`):** Prompt instructions and Telegram inline keyboard buttons are managed dynamically via JSON without recompiling code!
- **⚡ Real-Time Chat Action Indicator:** Sends real-time "typing..." status while downloading audio and generating AI responses.
- **🔒 Data Privacy Transparency:** Outlines clear privacy options between Google Free Tier ($0) and Paid/OpenRouter (strict privacy, zero model training).
- **📦 Zero-Code Multi-Arch Distribution:** Ready-to-run release archives for Linux (`amd64`, `arm64`), Windows (`amd64`, `arm64`), and macOS (`Intel amd64`, `Apple Silicon M1-M4 arm64`).
- **🐳 Container-Ready:** A ~15 MB multi-arch image (`amd64`, `arm64`) with a ready-made `docker-compose.yml` for CasaOS, Cosmos Cloud, Portainer and Dockge. Runs unprivileged, needs no published port, and keeps its state on a single volume.

---

## 🚀 Quick Start for End-Users (Zero-Code)

Non-developers can run Scribo without installing Go or compiling code:

1. Download the pre-compiled release archive for your server architecture (`linux-amd64`, `linux-arm64`, `windows-amd64`, `windows-arm64`, `darwin-amd64`, or `darwin-arm64`).
2. Extract and enter the directory:
   - **Linux / macOS:**
     ```bash
     tar -xzvf scribo-darwin-arm64.tar.gz  # or scribo-linux-amd64.tar.gz
     cd scribo
     ```
   - **Windows:** Extract `scribo-windows-amd64.zip`.
   - **Android (Termux):**
     ```bash
     tar -xzvf scribo-linux-arm64.tar.gz
     cd scribo
     ./scribo
     ```
3. Edit your API keys in `.env`:
   ```bash
   nano .env
   ```
4. Run:
   - **Linux Systemd Service (7/24 background):**
     ```bash
     sudo ./setup_service.sh
     ```
   - **Android / Termux:**
     ```bash
     ./scribo
     ```

---

## 🐳 Docker & Home Server Panels

A multi-architecture image (`linux/amd64`, `linux/arm64`) is published to GitHub Container Registry on every push to `main` and every tagged release, so no Go toolchain and no clone are needed:

```
ghcr.io/murqin/scribo:latest
```

```bash
# Fetch the two files you need, without cloning the repository
curl -O https://raw.githubusercontent.com/Murqin/Scribo/main/docker-compose.yml
curl -o .env https://raw.githubusercontent.com/Murqin/Scribo/main/.env.example

nano .env                # fill in TELEGRAM_TOKEN, ALLOWED_USER_ID, GEMINI_API_KEY
docker compose up -d
docker compose logs -f
```

`.env` has to exist before `docker compose up`, because the compose file reads your settings from it.

The runtime image is **~15 MB** and the bot inside it still idles at ~10 MB RAM.

### 🔌 No ports, no domain, no reverse proxy

Scribo reaches Telegram through **outbound long polling only**. The container publishes no port and listens on nothing, so there is no reverse proxy entry, no subdomain and no certificate to arrange. When a panel asks for a port or a URL during installation, leave it blank — the bot is a background worker, not a web app, and it has no web interface.

### 💾 Persistent data (`/data`)

The container's working directory is `/data`, and everything worth keeping lands there:

| File | Purpose |
| :--- | :--- |
| `modes.json` | Written on first run in the language `SCRIBO_LANG` selects. Edit it to change prompts and buttons, then restart. |
| `scribo_history.jsonl` | Transcript history behind `/son`, and the record the spending ceiling is restored from after a restart. |

Losing this volume hands out a fresh daily spending allowance and drops your custom prompts, so include it in backups.

### 👤 File ownership on bind mounts (`PUID` / `PGID`)

With the default named volume nothing needs configuring. If you switch to a bind mount — which CasaOS and Cosmos Cloud usually do for you — the host directory has to be writable by the container's user. Either set `PUID`/`PGID` in `.env` to the owner of that directory, or `chown -R 1000:1000` it. The container starts as root only long enough to hand `/data` over, then drops to the requested user for good.

`TZ` belongs in `.env` too, and it is worth setting: the daily spending ceiling rolls over on a local calendar boundary, so an unset zone resets your cap at UTC midnight rather than yours.

> A panel will not ask you for `PUID`, `PGID` or `TZ` — the image already defaults them to `1000`, `1000` and `UTC`, so they are not declared in the compose file. Set them in `.env`, or add them as extra environment variables in the panel, only if the defaults do not suit you.

### 📥 What a panel asks for

Panels build their import form from the compose file, so Scribo declares exactly the three settings that have no sensible default:

| Field | |
| :--- | :--- |
| `TELEGRAM_TOKEN` | required |
| `ALLOWED_USER_ID` | required unless you set `ALLOW_ALL_USERS=true` |
| `GEMINI_API_KEY` | required unless you supply `OPENROUTER_API_KEY` instead |

Everything else keeps its default. To change one — `OPENROUTER_API_KEY`, `SCRIBO_LANG`, the spending ceilings — add it as an extra environment variable in the panel, using the names from [`.env.example`](.env.example).

Running compose yourself instead? `.env` covers all of it and the file is optional as far as compose is concerned, so the same `docker-compose.yml` works both ways.

### 🏠 Per-platform notes

- **CasaOS** — *Apps → Custom Install → Import*, paste `docker-compose.yml`, then fill the environment variables in the form. Leave the web UI port empty.
- **Cosmos Cloud** — *ServApps → New ServApp → Docker Compose*, paste the same file. Skip the URL/route step; without a listening port there is nothing to route.
- **Portainer / Dockge** — deploy as a stack from this repository or from the pasted compose file; upload your `.env` alongside it.
- **YunoHost** — YunoHost packages apps natively with systemd rather than running containers, so there is no Docker app catalogue to install this from. On a YunoHost box the fitting route is the systemd path (`sudo ./setup_service.sh`) described above. The compose file still works if you installed Docker on the server yourself, but that is outside what YunoHost supports.

### 🔨 Building the image yourself

```bash
make docker-build            # docker build -t scribo:local .
make docker-build DOCKER=podman   # Podman works as a drop-in
```

To run a locally built image, comment out `image:` in `docker-compose.yml` and uncomment the `build:` block underneath it.

---

## 💬 Using the Bot

| Command | Turkish alias | What it does |
| :--- | :--- | :--- |
| `/start` | `/basla` | Confirms the bot is running and lists what you can send it. |
| `/last` | `/son` | Replays the most recent output of this chat, rendered in its original mode's format. Survives a restart, since it is read back from `HISTORY_FILE`. |

Both commands are registered with Telegram at startup, so typing `/` in the chat offers them with descriptions instead of leaving you to remember them.

Both names work whatever `SCRIBO_LANG` is set to; only the name the bot advertises — in its greeting and in that menu — follows the language. `/start` in particular can never stop working, since Telegram's own Start button and `t.me` deep links send it. The alias is spelled `basla` rather than `başla` because Telegram restricts command names to `a-z`, `0-9` and underscore.

There is nothing else to type. Send a voice note, a video, a video message, an audio file — or any of them as a document — and Scribo replies with an inline keyboard of the modes from `modes.json`; pick one and it processes the media. Files are capped at 20 MB by Telegram's bot API.

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

# Interface and answer language: tr or en. Drives the Telegram messages *and* the
# prompts, so it decides which language the model answers in. Unset or unknown
# means Turkish. LANG=en ./scribo works too, but prefer SCRIBO_LANG in this file:
# a LANG line here is ignored whenever your shell already exports one.
SCRIBO_LANG=en

# Worker Pool Concurrency Limit (Maximum simultaneous audio processing jobs)
# Controls how many audio files are processed in parallel. Extra requests wait in queue safely.
MAX_CONCURRENT_JOBS=5

# Spending ceiling in USD for paid OpenRouter calls. Leave empty for no ceiling.
# Google Free Tier is never affected. Counters are restored from HISTORY_FILE at
# startup, so they survive a restart within the same calendar day/month.
DAILY_COST_LIMIT=0.50
MONTHLY_COST_LIMIT=10

# Where finished outputs are appended, one JSON object per line. Powers /son and
# the spending counter. Omit the line for the default; set it to an empty value
# to store nothing (the spending counter then resets on every restart).
HISTORY_FILE=scribo_history.jsonl
```

---

## 🔒 Data Privacy & Model Training Notice

Please review Google AI Studio's terms regarding data privacy between Free Tier and Paid Tier providers:

| Provider / Tier | Cost | Model Training Usage | Rate Limits |
| :--- | :--- | :--- | :--- |
| **Google Free Tier (`google`)** | **$0.00** | ⚠️ **Yes** (Google may use anonymized data to train/improve products) | 15 RPM / 1,500 RPD |
| **OpenRouter (`openrouter`)** | **Paid** | 🛡️ **No** (Data is strictly private, no model training) | High / Uncapped |
| **Google Paid Tier** | **Paid** | 🛡️ **No** (Enterprise privacy, no model training) | High / Uncapped |

> 💡 **Recommendation:** If you process sensitive or confidential audio, set `DEFAULT_PROVIDER=openrouter` or upgrade to Google's Paid Tier to guarantee enterprise-grade data privacy.

> ⚠️ **Local storage:** Transcripts are also written in plain text to `HISTORY_FILE` on the machine running the bot (created with `0600` permissions). Set `HISTORY_FILE=` to an empty value to keep nothing on disk — note that this also makes the spending counter reset on every restart.

---

## 🧩 Custom Modes & Prompts (`modes.json`)

To customize button names or add custom AI prompts, create a `modes.json` file in the working directory (or copy `modes.example.json`):

```json
{
  "tldr": {
    "label": "📝 Özet",
    "prompt": "Sen profesyonel bir ses analiz asistanısın...",
    "format": "code"
  },
  "trans": {
    "label": "✍️ Transkript",
    "prompt": "Sen hassas bir ses deşifre sistemisin...",
    "format": "code"
  },
  "fix": {
    "label": "🛠️ Düzelt",
    "prompt": "Sen uzman bir editör ve dil düzeltme sistemisin...",
    "format": "code"
  },
  "blog": {
    "label": "📰 Blog",
    "prompt": "Kaydı markdown başlıklarıyla bir blog taslağına dönüştür...",
    "format": "markdown"
  }
}
```

The first three are the built-in defaults; `blog` shows how a custom mode opts into markdown rendering.

### Output format (`format`)

Each mode picks how its output is rendered. The field is optional and defaults to `code`, so a `modes.json` written before this field existed keeps working unchanged.

| Value | Rendering | Good for |
|---|---|---|
| `code` (default) | Wrapped in `<code>` — one tap copies the whole reply | Transcripts, corrections, anything you paste elsewhere |
| `markdown` | Model markdown rendered as Telegram formatting: `**bold**`, `*italic*`, `~~strike~~`, links, `` `code` ``, fenced blocks. Headings become bold lines and bullets become `•`, since Telegram has no markup for either | Blog drafts, notes, prose |
| `plain` | Sent verbatim, no formatting applied | Output that already contains markup you want to read literally |

Scribo automatically creates `modes.json` on disk at startup if missing, re-creates the Telegram inline keyboard dynamically with alphabetical custom mode sorting, and applies your custom prompts!

> 🌍 The generated `modes.json` is written in the language `SCRIBO_LANG` selects. If you change the language later, a `modes.json` that is still byte-identical to a shipped default is regenerated automatically — otherwise the interface would switch language while the prompts stayed behind, and the model would keep answering in the old one. A `modes.json` you have edited yourself is never touched: translate it or delete it when you switch.

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

### Build Multi-Platform Binaries
```bash
make build-linux-amd64
make build-linux-arm64
make build-windows-amd64
make build-windows-arm64
make build-darwin-amd64
make build-darwin-arm64
```

### Build Release Archives
```bash
make release
# Generates release packages in dist/ directory (tar.gz & zip)
```

### Container Image
```bash
make docker-build   # build scribo:local
make docker-up      # docker compose up -d
make docker-logs    # docker compose logs -f
make docker-down    # docker compose down
```

---

## 📊 Monitoring Logs

```bash
# Systemd — follow live logs
sudo journalctl -u scribo -f

# Systemd — view last 50 log entries
sudo journalctl -u scribo -n 50 --no-pager

# Docker — follow live logs
docker compose logs -f
```

---

## 📄 License

Licensed under the terms of the **MIT License**. See [LICENSE](LICENSE) for details.
