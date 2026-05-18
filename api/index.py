import os
import logging
import base64
import httpx
import secrets
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

if not all([TELEGRAM_TOKEN, OPENROUTER_API_KEY, WEBHOOK_SECRET, ALLOWED_USER_ID]):
    logger.error("KRİTİK HATA: Ortam değişkenleri eksik!")

app = FastAPI()
bot = Bot(token=TELEGRAM_TOKEN)

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

async def process_voice(chat_id, file_id, message_id=None):
    """Ses dosyasını indirir, özetler ve mesajı günceller."""
    status_msg = None
    try:
        if message_id:
            status_msg = await bot.edit_message_text(chat_id=chat_id, message_id=message_id, text="🔄 Yeniden deneniyor, işleniyor...")
        else:
            status_msg = await bot.send_message(chat_id=chat_id, text="🎙️ Ses kaydı alındı, işleniyor...")

        voice_file = await bot.get_file(file_id)
        file_path = f"/tmp/{file_id}.ogg"
        await voice_file.download_to_drive(file_path)

        try:
            with open(file_path, "rb") as audio_file:
                base64_audio = base64.b64encode(audio_file.read()).decode("utf-8")

            headers = {
                "Authorization": f"Bearer {OPENROUTER_API_KEY}",
                "Content-Type": "application/json",
                "HTTP-Referer": "https://github.com/murqin/tldr-tool",
            }

            payload = {
                "model": "google/gemini-2.0-flash-001",
                "messages": [
                    {
                        "role": "system",
                        "content": (
                            "Sen bir asistan gibi değil, kullanıcının bizzat kendisi gibi konuşmalısın. "
                            "Ses kaydındaki düşünceleri, sanki sen (kullanıcı) arkadaşlarına anlatıyormuşsun gibi 'ben' diliyle (1. tekil şahıs) özetle. "
                            "Giriş cümleleri veya sonuç cümleleri ASLA kullanma. "
                            "Doğrudan konuya gir. Samimi, aşırı rahat, 'agalar' jargonuna sahip bir dil kullan. "
                            "Maksimum 2-3 kısa madde (bullet point) kullan. Çıktı tamamen Türkçe olmalı."
                        )
                    },
                    {
                        "role": "user",
                        "content": [
                            {"type": "text", "text": "Bu ses kaydını arkadaşlarına anlatıyormuşsun gibi, ben diliyle ve giriş/sonuç cümlesi eklemeden Türkçe özetle."},
                            {
                                "type": "input_audio",
                                "input_audio": {"data": base64_audio, "format": "ogg"}
                            }
                        ]
                    }
                ]
            }

            async with httpx.AsyncClient(timeout=60.0) as client:
                response = await client.post(
                    "https://openrouter.ai/api/v1/chat/completions",
                    headers=headers,
                    json=payload
                )

            if response.status_code != 200:
                raise Exception(f"API Hatası ({response.status_code})")

            response_json = response.json()
            response_text = response_json["choices"][0]["message"]["content"]

            messages_to_send = split_message(response_text)
            await bot.edit_message_text(chat_id=chat_id, message_id=status_msg.message_id, text=messages_to_send[0])

            for extra_message in messages_to_send[1:]:
                await bot.send_message(chat_id=chat_id, text=extra_message)

        finally:
            if os.path.exists(file_path):
                os.remove(file_path)

    except Exception as e:
        logger.error(f"İşleme Hatası: {e}", exc_info=True)
        keyboard = InlineKeyboardMarkup([
            [InlineKeyboardButton("🔄 Tekrar Dene", callback_data=f"retry:{file_id}")]
        ])
        hata_metni = f"❌ Bir sorun oluştu.\n{str(e)[:100]}"
        
        if status_msg:
            await bot.edit_message_text(chat_id=chat_id, message_id=status_msg.message_id, text=hata_metni, reply_markup=keyboard)
        else:
            await bot.send_message(chat_id=chat_id, text=hata_metni, reply_markup=keyboard)

@app.post("/webhook")
async def telegram_webhook(request: Request, x_telegram_bot_api_secret_token: str = Header(None)):
    if not WEBHOOK_SECRET or not secrets.compare_digest(x_telegram_bot_api_secret_token or "", WEBHOOK_SECRET):
        raise HTTPException(status_code=403, detail="Erişim Engellendi.")

    try:
        body = await request.body()
        if len(body) > 1024 * 1024:
            raise HTTPException(status_code=413, detail="İstek çok büyük.")
            
        data = await request.json()
        update = Update.de_json(data, bot)
        user = update.effective_user

        if not user or str(user.id) != ALLOWED_USER_ID:
            return {"status": "unauthorized"}

        if update.message and update.message.voice:
            await process_voice(update.message.chat.id, update.message.voice.file_id)

        elif update.callback_query:
            query = update.callback_query
            await query.answer()
            
            if query.data.startswith("retry:"):
                file_id = query.data.split(":")[1]
                await process_voice(query.message.chat.id, file_id, message_id=query.message.message_id)

        return {"status": "ok"}

    except Exception as e:
        logger.error(f"Sistem Hatası: {e}", exc_info=True)
        return {"status": "error"}