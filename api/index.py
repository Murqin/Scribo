import os
import logging
import base64
import httpx
import secrets
import asyncio
import urllib.parse
from fastapi import FastAPI, Request, Header, HTTPException
from telegram import Update, Bot, InlineKeyboardButton, InlineKeyboardMarkup
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

def get_mode_keyboard(file_id: str, current_text: str = None):
    """Mod seçimi ve Paylaş butonlarını oluşturur."""
    buttons = []
    # Üst sıra: Modlar
    buttons.append([
        InlineKeyboardButton(MODES["tldr"]["label"], callback_data=f"tldr"),
        InlineKeyboardButton(MODES["trans"]["label"], callback_data=f"trans"),
        InlineKeyboardButton(MODES["fix"]["label"], callback_data=f"fix")
    ])
    
    # Alt sıra: Paylaş butonu (Eğer metin varsa)
    if current_text:
        # Telegram içi paylaşım linki (En stabil yöntem)
        # Bu linke tıklandığında Telegram bir kişi seçmeni ister ve metni oraya yapıştırır.
        # Instagram'a doğrudan metin pushlamak teknik olarak kısıtlı olduğundan en hızlı yol budur.
        share_url = f"https://t.me/share/url?url=&text={urllib.parse.quote(current_text)}"
        buttons.append([InlineKeyboardButton("📲 Paylaş (Telegram/Diğer)", url=share_url)])
        
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

            payload = {
                "model": "google/gemini-2.0-flash-001",
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

            async with httpx.AsyncClient(timeout=25.0) as client:
                response = await client.post(
                    "https://openrouter.ai/api/v1/chat/completions",
                    headers={"Authorization": f"Bearer {OPENROUTER_API_KEY}", "Content-Type": "application/json"},
                    json=payload
                )

            if response.status_code != 200:
                raise Exception(f"API Hatası: {response.status_code}")

            res_text = response.json()["choices"][0]["message"]["content"]
            chunks = split_message(res_text)
            
            await bot.edit_message_text(
                chat_id=chat_id, 
                message_id=status_msg.message_id, 
                text=chunks[0],
                reply_markup=get_mode_keyboard(file_id, current_text=chunks[0])
            )

            for c in chunks[1:]:
                await bot.send_message(chat_id=chat_id, text=c)

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
                reply_markup=get_mode_keyboard(update.message.voice.file_id)
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