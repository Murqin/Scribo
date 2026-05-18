import os
import logging
import base64
import httpx
import secrets
from fastapi import FastAPI, Request, Header, HTTPException
from telegram import Update, Bot
from dotenv import load_dotenv

load_dotenv()

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# 🔒 KRİTİK: Değişkenler eksikse botun çalışmasını engelle (Açık kapı kalmasın)
TELEGRAM_TOKEN = os.environ.get("TELEGRAM_TOKEN")
OPENROUTER_API_KEY = os.environ.get("OPENROUTER_API_KEY")
WEBHOOK_SECRET = os.environ.get("WEBHOOK_SECRET")
ALLOWED_USER_ID = os.environ.get("ALLOWED_USER_ID")

if not all([TELEGRAM_TOKEN, OPENROUTER_API_KEY, WEBHOOK_SECRET, ALLOWED_USER_ID]):
    logger.error("KRİTİK HATA: Ortam değişkenleri eksik!")
    # Çalışmayı durdurmuyoruz ama fonksiyonlarda katı kontrol yapacağız

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

@app.post("/webhook")
async def telegram_webhook(request: Request, x_telegram_bot_api_secret_token: str = Header(None)):
    # 🔒 GÜVENLİK 1: Webhook Secret Kontrolü (Zaman saldırılarına karşı güvenli karşılaştırma)
    if not WEBHOOK_SECRET or not secrets.compare_digest(x_telegram_bot_api_secret_token or "", WEBHOOK_SECRET):
        logger.warning("Yetkisiz Webhook Erişimi Denemesi!")
        raise HTTPException(status_code=403, detail="Erişim Engellendi.")

    chat_id = None
    status_msg = None

    try:
        # 🔒 GÜVENLİK 2: İstek boyutu kontrolü (Dosya sömürüsünü önlemek için)
        body = await request.body()
        if len(body) > 1024 * 1024: # 1MB limit
            raise HTTPException(status_code=413, detail="İstek çok büyük.")
            
        data = await request.json()
        update = Update.de_json(data, bot)

        # 🔒 GÜVENLİK 3: Kullanıcı Yetki Kontrolü
        user = update.effective_user
        if not user or str(user.id) != ALLOWED_USER_ID:
            logger.warning(f"Yetkisiz kullanıcı engellendi! ID: {user.id if user else 'Unknown'}")
            return {"status": "unauthorized"}

        if update.message and update.message.voice:
            chat_id = update.message.chat.id
            status_msg = await bot.send_message(chat_id=chat_id, text="🎙️ Ses kaydı alındı, işleniyor...")

            voice_file = await bot.get_file(update.message.voice.file_id)
            
            # 🔒 GÜVENLİK 4: Dosya boyutu kontrolü (OpenRouter/Vercel limitleri için)
            if update.message.voice.file_size > 10 * 1024 * 1024: # 10MB limit
                raise Exception("Ses dosyası çok büyük (Max 10MB).")

            file_path = f"/tmp/{update.message.voice.file_id}.ogg"
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
                    raise Exception(f"OpenRouter API Hatası ({response.status_code})")

                response_json = response.json()
                response_text = response_json["choices"][0]["message"]["content"]

                messages_to_send = split_message(response_text)
                await bot.edit_message_text(chat_id=chat_id, message_id=status_msg.message_id, text=messages_to_send[0])

                for extra_message in messages_to_send[1:]:
                    await bot.send_message(chat_id=chat_id, text=extra_message)

            finally:
                if os.path.exists(file_path):
                    os.remove(file_path)

        return {"status": "ok"}

    except Exception as e:
        logger.error(f"Sistem Hatası: {e}", exc_info=True)
        if chat_id:
            hata_mesaji = f"❌ Hata oluştu:\n{str(e)[:200]}"
            try:
                if status_msg:
                    await bot.edit_message_text(chat_id=chat_id, message_id=status_msg.message_id, text=hata_mesaji)
                else:
                    await bot.send_message(chat_id=chat_id, text=hata_mesaji)
            except: pass
        return {"status": "error"}