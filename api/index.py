import os
import logging
import base64
from fastapi import FastAPI, Request, Header, HTTPException
from telegram import Update, Bot
from openai import OpenAI

logging.basicConfig(level=logging.INFO)

TELEGRAM_TOKEN = os.environ.get("TELEGRAM_TOKEN")
OPENROUTER_API_KEY = os.environ.get("OPENROUTER_API_KEY")
WEBHOOK_SECRET = os.environ.get("WEBHOOK_SECRET") # Dışarıdan sömürülmeyi engelleyecek gizli şifre

app = FastAPI()
bot = Bot(token=TELEGRAM_TOKEN)
client = OpenAI(base_url="https://openrouter.ai/api/v1", api_key=OPENROUTER_API_KEY)

def split_message(text, max_length=1000):
    """Instagram DM sınırı (1000 karakter) için kelimeleri bölmeden parçalar."""
    chunks = []
    while len(text) > max_length:
        split_idx = text.rfind('\n', 0, max_length)
        if split_idx == -1:
            split_idx = text.rfind(' ', 0, max_length)
        if split_idx == -1:
            split_idx = max_length
        chunks.append(text[:split_idx].strip())
        text = text[split_idx:].strip()
    if text:
        chunks.append(text)
    return chunks

@app.post("/webhook")
async def telegram_webhook(request: Request, x_telegram_bot_api_secret_token: str = Header(None)):
    # 🔒 GÜVENLİK DUVARI: Şifreyi bilmeyen dışarıdan kimse içeri giremez
    if x_telegram_bot_api_secret_token != WEBHOOK_SECRET:
        raise HTTPException(status_code=403, detail="Erişim Engellendi. Yetkisiz Istek.")

    try:
        data = await request.json()
        update = Update.de_json(data, bot)
        
        if update.message and update.message.voice:
            chat_id = update.message.chat.id
            status_msg = await bot.send_message(chat_id=chat_id, text="Ses kaydı alındı, fabrikada işleniyor...")
            
            # Serverless ortamda geçici klasör olarak /tmp kullanmak zorundayız
            file_path = f"/tmp/{update.message.voice.file_id}.ogg"
            voice_file = await bot.get_file(update.message.voice.file_id)
            await voice_file.download_to_drive(file_path)
            
            with open(file_path, "rb") as audio_file:
                base64_audio = base64.b64encode(audio_file.read()).decode("utf-8")
            
            # OpenRouter Çağrısı (İngilizce Talimat -> Türkçe Çıktı)
            response = client.chat.completions.create(
                model="google/gemini-2.5-flash", 
                max_tokens=400, 
                temperature=0.3,
                messages=[
                    {
                        "role": "system",
                        "content": (
                            "You are a minimalist software engineer assistant. Analyze the provided audio content. "
                            "Clean up any technical mess, brainstorming, or rants. Create a concise, direct TL;DR (summary) "
                            "consisting of 2-3 bullet points. Do not include introductory or concluding sentences. "
                            "Jump straight to the bullet points. Use a friendly, casual, and brotherly tone ('agalar' jargon). "
                            "CRITICAL REQUIREMENT: The entire output MUST be written in Turkish."
                        )
                    },
                    {
                        "role": "user",
                        "content": [
                            {"type": "text", "text": "Summarize this audio according to the system instructions, outputting strictly in Turkish."},
                            {"type": "input_audio", "input_audio": {"data": base64_audio, "format": "ogg"}}
                        ]
                    }
                ]
            )
            
            response_text = response.choices[0].message.content
            
            if os.path.exists(file_path):
                os.remove(file_path)
                
            # Instagram DM parçalayıcı ve mesaj gönderme döngüsü
            messages_to_send = split_message(response_text, max_length=1000)
            await bot.edit_message_text(chat_id=chat_id, message_id=status_msg.message_id, text=messages_to_send[0])
            
            for extra_message in messages_to_send[1:]:
                await bot.send_message(chat_id=chat_id, text=extra_message)
                
        return {"status": "ok"}
    except Exception as e:
        logging.error(f"Fabrika Hatasi: {e}")
        return {"status": "error", "details": str(e)}