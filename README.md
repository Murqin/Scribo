# Telegram Voice TL;DR Bot (tldr-bot) 🎙️

> **A serverless, FastAPI-powered Telegram bot running on Vercel that intercepts voice messages, transcribes or summarizes them using Gemini 2.0 Flash (via OpenRouter audio modality), and audits execution costs in real-time.**

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)](LICENSE)
[![Tech: FastAPI](https://img.shields.io/badge/FastAPI-v0.100%2B-009688?style=flat-square&logo=fastapi&logoColor=white)](#)
[![Tech: Python](https://img.shields.io/badge/Python-3.10%2B-blue?style=flat-square&logo=python&logoColor=white)](#)
[![Model: Gemini 2.0 Flash](https://img.shields.io/badge/Model-Gemini%202.0%20Flash-red?style=flat-square&logo=google&logoColor=white)](#)
[![Infrastructure: Vercel](https://img.shields.io/badge/Infrastructure-Vercel%20Serverless-black?style=flat-square&logo=vercel&logoColor=white)](#)

---

## ✨ Features

- **⚡ Serverless FastAPI Webhook:** Built for serverless deployment on Vercel (`api/index.py`), executing instantly with zero persistent container overhead.
- **🎙️ Direct Audio Modality:** Bypasses conventional, slow speech-to-text converters. Encodes raw `.ogg` voice buffers to base64 and streams them directly to Gemini 2.0 Flash's native audio-sensing model.
- **🏷️ Smart Interactive Modes:** Features inline button selectors situated directly inside Telegram chats:
  - **📝 Özet (TL;DR):** Generates a concise, 1st-person Turkish summary written in a casual, friendly jargon ("agalar" slang) with 2-3 precise bullet points.
  - **✍️ Transkript:** Resolves a precise, word-for-word literal translation of the spoken recording.
  - **🛠️ Düzelt:** Transcribes the audio while correcting syntax, spelling errors, and outputting a highly fluent, reading-friendly block.
- **📊 Real-time Cost Auditing:** Queries OpenRouter's live API model pricing endpoint on-the-fly. Calculates exact input/output token usage costs and displays a detailed balance summary (e.g., `Maliyet: $0.00004`) after every request.
- **🔒 Webhook Authentication:** Features robust security checking, validating secret tokens against headers (`X-Telegram-Bot-Api-Secret-Token`) to reject unauthorized webhook invocations.
- **👤 User Whitelisting:** Enforces a rigid system-level environment whitelist (`ALLOWED_USER_ID`) to restrict bot execution strictly to the owner.

---

## 🏗️ Execution Flow

```mermaid
sequenceDiagram
    autonumber
    actor User as Telegram User
    participant TG as Telegram API
    participant Bot as FastAPI (Vercel)
    participant OR as OpenRouter (Gemini)

    User->>TG: Sends voice message (.ogg)
    TG->>Bot: Post Webhook Event (with X-Secret-Token)
    Note over Bot: Validate Secret Token & User ID
    Bot->>TG: Ask choice (Özet / Transkript / Düzelt)
    User->>TG: Selects mode button
    TG->>Bot: Post Callback Query
    Bot->>TG: Downloads .ogg to /tmp
    Bot->>OR: Post Base64 Audio Buffer & System Prompt
    OR->>Bot: Returns Text response & Token usage
    Bot->>Bot: Fetch OpenRouter prices & Calculate Cost
    Bot->>TG: Edit message to show code-block text + cost details
```

---

## 🛠️ Environment Variables Configuration

Create a local `.env` file (or set keys inside your Vercel deployment console):

```env
TELEGRAM_TOKEN=your_telegram_bot_token
OPENROUTER_API_KEY=your_openrouter_developer_api_key
WEBHOOK_SECRET=your_custom_secure_secret_token
ALLOWED_USER_ID=your_numerical_telegram_user_id
```

---

## 🚀 Build & Deployment

### 1. Running Locally
To launch a local development server for testing:
```bash
# Clone the repository
git clone https://github.com/Murqin/tldr-bot.git
cd tldr-bot

# Install requirements
pip install -r requirements.txt

# Launch FastAPI using Uvicorn
uvicorn api.index:app --reload
```

### 2. Register Telegram Webhook
Point Telegram's API endpoint to your hosted server instance:
```bash
curl -X POST "https://api.telegram.org/bot<YOUR_TELEGRAM_TOKEN>/setWebhook" \
     -H "Content-Type: application/json" \
     -d '{"url": "https://your-app.vercel.app/webhook", "secret_token": "<YOUR_WEBHOOK_SECRET>"}'
```

### 3. Deploying to Vercel
Deploy seamlessly using the Vercel CLI:
```bash
# Install Vercel CLI
npm install -g vercel

# Deploy production build
vercel --prod
```

---

## 📂 Project Architecture

```text
tldr-bot/
├── api/
│   └── index.py            # Primary FastAPI entry point & bot controller
├── requirements.txt         # Python package dependencies
├── vercel.json              # Vercel serverless routing configuration
└── README.md
```

---

## 📄 License

Licensed under the terms of the MIT License. See [LICENSE](LICENSE) for more details.
