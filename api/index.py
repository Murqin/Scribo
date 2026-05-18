import os
import logging
import base64
import httpx
import secrets
import asyncio
import urllib.parse
from fastapi import FastAPI, Request, Header, HTTPException
from telegram import Update, Bot, InlineKeyboardButton, InlineKeyboardMarkup
from telegram.constants import ParseMode
from dotenv import load_dotenv

load_dotenv()

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

TELEGRAM_TOKEN = os.environ.get("TELEGRAM_TOKEN")
OPENROUTER_API_KEY = os.environ.get("OPENROUTER_API_KEY")
WEBHOOK_SECRET = os.environ.get("WEBHOOK_SECRET")
ALLOWED_USER_ID = os.environ.get("ALLOWED_USER_ID")

app = FastAPI()
bot = Bot(token=TELEGRAM_TOKEN)

# Sabitler
TARGET_MODEL = "google/gemini-2.0-flash-001"
MODEL_PRICES_CACHE = {}

MODES = {
    "tldr": {
        "label": "📝 Özet",
        "prompt": (
            "Kullanıcının kendisi gibi, 1. tekil şahısla, samimi/agalar jargonunda, "
            "giriş/sonuç cümlesiz, 2-3 maddeyle Türkçe özetle."
        )
    },
    "trans": {
        "label": "✍️ Transkript",
        "prompt": "Ses kaydını kelimesi kelimesine Türkçe transkript yap. Yorum ekleme."
    },
    "fix": {
        "label": "🛠️ Düzelt",
        "prompt": "Transkripti çıkarırken imla ve anlatımı düzelt, akıcı Türkçe ile yeniden yaz."
    }
}

async def get_dynamic_pricing(model_id: str):
    """OpenRouter API'den güncel fiyatlandırmayı çeker."""
    global MODEL_PRICES_CACHE
    if model_id in MODEL_PRICES_CACHE:
        return MODEL_PRICES_CACHE[model_id]
    
    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            response = await client.get("https://openrouter.ai/api/v1/models")
            if response.status_code == 200:
                models = response.json().get("data", [])
                for m in models:
                    if m["id"] == model_id:
                        pricing = m.get("pricing", {})
                        p = {
                            "prompt": float(pricing.get("prompt", 0)),
                            "completion": float(pricing.get("completion", 0))
                        }
                        MODEL_PRICES_CACHE[model_id] = p
                        logger.info(f"Fiyatlandırma güncellendi: {model_id} -> {p}")
                        return p
    except Exception as e:
        logger.error(f"Fiyat çekme hatası: {e}")
    
    # Fallback (API patlarsa varsayılan değerler)
    return {"prompt": 0.0000001, "completion": 0.0000004}

def split_message(text: str, max_length: int = 4000) -> list[str]:
    if not text: return []
    chunks = []
    while len(text) > max_length:
        split_idx = text.rfind('\n', 0, max_length)
        if split_idx <= 0: split_idx = text.rfind(' ', 0, max_length)
        if split_idx <= 0: split_idx = max_length
        chunks.append(text[:split_idx].strip())
        text = text[split_idx:].strip()
    if text: chunks.append(text)
    return chunks

def get_mode_keyboard():
    buttons = [[
        InlineKeyboardButton(MODES["tldr"]["label"], callback_data="tldr"),
        InlineKeyboardButton(MODES["trans"]["label"], callback_data="trans"),
        InlineKeyboardButton(MODES["fix"]["label"], callback_data="fix")
    ]]
    return InlineKeyboardMarkup(buttons)

async def process_voice(chat_id, file_id, mode="tldr", message_id=None):
    status_msg = None
    try:
        current_label = MODES[mode]["label"]
        if message_id:
            status_msg = await bot.edit_message_text(chat_id=chat_id, message_id=message_id, text=f"🔄 {current_label} hazırlanıyor...")
        else:
            status_msg = await bot.send_message(chat_id=chat_id, text=f"🎙️ {current_label} hazırlanıyor...")

        voice_file = await bot.get_file(file_id)
        file_path = f"/tmp/{file_id}.ogg"
        await voice_file.download_to_drive(file_path)

        try:
            with open(file_path, "rb") as audio_file:
                base64_audio = base64.b64encode(audio_file.read()).decode("utf-8")

            # Fiyatları ve AI yanıtını paralel veya sırayla al (Zaman tasarrufu için fiyatı önden çekebiliriz)
            pricing_task = asyncio.create_task(get_dynamic_pricing(TARGET_MODEL))

            payload = {
                "model": TARGET_MODEL,
                "messages": [
                    {"role": "system", "content": MODES[mode]["prompt"]},
                    {
                        "role": "user",
                        "content": [
                            {"type": "text", "text": "İşle."},
                            {"type": "input_audio", "input_audio": {"data": base64_audio, "format": "ogg"}}
                        ]
                    }
                ]
            }

            async with httpx.AsyncClient(timeout=30.0) as client:
                response = await client.post(
                    "https://openrouter.ai/api/v1/chat/completions",
                    headers={"Authorization": f"Bearer {OPENROUTER_API_KEY}", "Content-Type": "application/json"},
                    json=payload
                )

            if response.status_code != 200:
                raise Exception(f"API Hatası: {response.status_code}")

            res_data = response.json()
            res_text = res_data["choices"][0]["message"]["content"]
            usage = res_data.get("usage", {})
            
            # Dinamik fiyatları al
            current_prices = await pricing_task
            
            prompt_tokens = usage.get("prompt_tokens", 0)
            completion_tokens = usage.get("completion_tokens", 0)
            total_cost = (prompt_tokens * current_prices["prompt"]) + (completion_tokens * current_prices["completion"])

            chunks = split_message(res_text)
            copyable_text = f"<code>{chunks[0]}</code>"
            
            await bot.edit_message_text(
                chat_id=chat_id, 
                message_id=status_msg.message_id, 
                text=copyable_text,
                parse_mode=ParseMode.HTML,
                reply_markup=get_mode_keyboard()
            )

            for c in chunks[1:]:
                await bot.send_message(chat_id=chat_id, text=f"<code>{c}</code>", parse_mode=ParseMode.HTML)

            cost_msg = (
                f"📊 <b>Kullanım Özeti:</b>\n"
                f"├ Token: {prompt_tokens + completion_tokens} (P: {prompt_tokens}, C: {completion_tokens})\n"
                f"└ Maliyet: <code>${total_cost:.5f}</code>"
            )
            await bot.send_message(chat_id=chat_id, text=cost_msg, parse_mode=ParseMode.HTML)

        finally:
            if os.path.exists(file_path): os.remove(file_path)

    except Exception as e:
        logger.error(f"Hata: {e}", exc_info=True)
        kb = InlineKeyboardMarkup([[InlineKeyboardButton("🔄 Tekrar Dene", callback_data=mode)]])
        err_txt = f"❌ Hata: {str(e)[:50]}"
        if status_msg:
            await bot.edit_message_text(chat_id=chat_id, message_id=status_msg.message_id, text=err_txt, reply_markup=kb)
        else:
            await bot.send_message(chat_id=chat_id, text=err_txt, reply_markup=kb)

@app.post("/webhook")
async def telegram_webhook(request: Request, x_telegram_bot_api_secret_token: str = Header(None)):
    if not WEBHOOK_SECRET or not secrets.compare_digest(x_telegram_bot_api_secret_token or "", WEBHOOK_SECRET):
        return {"status": "forbidden"}

    try:
        data = await request.json()
        update = Update.de_json(data, bot)
        user = update.effective_user

        if not user or str(user.id) != ALLOWED_USER_ID:
            return {"status": "unauthorized"}

        if update.message and update.message.voice:
            await bot.send_message(
                chat_id=update.message.chat.id,
                text="🚀 Ses kaydı alındı! Seçiniz:",
                reply_to_message_id=update.message.message_id,
                reply_markup=get_mode_keyboard()
            )

        elif update.callback_query:
            query = update.callback_query
            await query.answer()
            
            mode = query.data
            voice_msg = query.message.reply_to_message
            
            if voice_msg and voice_msg.voice:
                await process_voice(query.message.chat.id, voice_msg.voice.file_id, mode=mode, message_id=query.message.message_id)
            else:
                await bot.edit_message_text(
                    chat_id=query.message.chat.id,
                    message_id=query.message.message_id,
                    text="❌ Kaynak ses dosyası bulunamadı."
                )

        return {"status": "ok"}
    except Exception as e:
        logger.error(f"Webhook Hatası: {e}", exc_info=True)
        return {"status": "error"}