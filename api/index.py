import os
import logging
import base64
import html
import httpx
import secrets
import asyncio
import urllib.parse
from datetime import datetime
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

app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
bot = Bot(token=TELEGRAM_TOKEN)

# Sabitler
TARGET_MODEL = "google/gemini-3.5-flash"
MODEL_PRICES_CACHE = {}

MODES = {
    "tldr": {
        "label": "📝 Özet",
        "prompt": (
            "Sen profesyonel bir ses analiz asistanısın. İletilen Türkçe ses kaydını şu kurallara göre özetle:\n"
            "1. Kesinlikle giriş, açıklama veya sonuç cümlesi yazma (Örn: \"İşte özet:\", \"Bu kayıtta...\" deme).\n"
            "2. Doğrudan 2 veya 3 maddelik (bullet points) bir markdown listesi döndür.\n"
            "3. Bağlamı koparmadan, ana fikri ve önemli noktaları eksiksiz ama sade bir Türkçe ile aktar.\n"
            "4. Konuşanın ağzından (1. tekil şahıs) yaz."
        )
    },
    "trans": {
        "label": "✍️ Transkript",
        "prompt": (
            "Sen hassas bir ses deşifre (transkripsiyon) sistemisin. İletilen Türkçe ses kaydını şu kurallara göre yazıya dök:\n"
            "1. Konuşulan her şeyi kelimesi kelimesine, hiçbir kelimeyi atlamadan aktar.\n"
            "2. Metne hiçbir yorum, düzeltme, açıklama veya ön söz/son söz ekleme.\n"
            "3. Konuşma esnasındaki duraksamaları veya dolgu kelimelerini (ee, şey, yani vb.) olduğu gibi koru.\n"
            "4. Eğer ses kaydı tamamen sessizse veya hiçbir anlaşılır kelime içermiyorsa, sadece boş bir metin veya \"[Anlaşılamayan Ses]\" yaz."
        )
    },
    "fix": {
        "label": "🛠️ Düzelt",
        "prompt": (
            "Sen uzman bir editör ve dil düzeltme sistemisin. İletilen Türkçe ses kaydını şu kurallara göre düzenle:\n"
            "1. Ses kaydındaki konuşmayı kelimesi kelimesine yazmak yerine; dil bilgisi hatalarını, anlatım bozukluklarını ve devrik cümleleri düzelt.\n"
            "2. Konuşmadaki gereksiz duraksamaları ve dolgu kelimelerini (ee, şey, yani vb.) tamamen temizle.\n"
            "3. Metni akıcı, profesyonel, okunması kolay ve anlam bütünlüğü korunmuş bir paragraf (veya gerekirse paragraflar) halinde yeniden yaz.\n"
            "4. Kesinlikle dışarıdan bir açıklama veya giriş/çıkış cümlesi ekleme."
        )
    },
    "note": {
        "label": "📓 Obsidian Notu",
        "prompt": (
            "Sen gelişmiş bir Obsidian not tutma asistanısın. İletilen Türkçe ses kaydını Obsidian markdown formatında yapılandırılmış bir nota dönüştür:\n"
            "1. Başlık olarak en üste konuyu özetleyen kısa bir `# Başlık` ekle.\n"
            "2. Notun altına konuya uygun `#etiketler` ekle (Örn: #proje #fikir #hatırlatıcı vb.).\n"
            "3. Ana fikri ve bağlamı koruyarak 1-2 cümlelik kısa bir açıklamayla özetle.\n"
            "4. Önemli detayları ve bilgileri yapılandırılmış maddeler halinde listele.\n"
            "5. Eğer ses kaydında yapılacak işler, görevler veya eylemler geçiyorsa bunları Obsidian yapılacaklar listesi (`- [ ] Görev`) formatında en alta ekle.\n"
            "6. Giriş/açıklama veya \"İşte notunuz:\" gibi ön ekler ekleme, doğrudan Obsidian markdown kodunu üret."
        )
    },
    "task": {
        "label": "📅 Takvim Raporu",
        "prompt": (
            "Sen bir asistan ve zaman yönetimi uzmanısın. İletilen Türkçe ses kaydındaki buluşma, toplantı, görev veya randevu bilgilerini analiz et:\n"
            "1. Ses kaydında geçen tüm görevleri ve yapılacak işleri listele.\n"
            "2. Eğer bir buluşma, toplantı veya tarihli etkinlik varsa, bu etkinliğin detaylarını (Başlık, Tarih, Saat, Açıklama) çıkar.\n"
            "3. Çıktının en sonuna, araya '===CALENDAR===' koyarak kesinlikle şu formatta bilgileri ekle (eğer etkinlik yoksa bu kısmı ekleme veya boş bırak):\n"
            "===CALENDAR===\n"
            "TITLE: [Etkinlik Başlığı]\n"
            "START: [YYYYMMDDTHHMMSSZ formatında UTC başlangıç tarihi/saati ya da YYYYMMDD formatında gün]\n"
            "END: [YYYYMMDDTHHMMSSZ formatında UTC bitiş tarihi/saati ya da YYYYMMDD formatında gün, yoksa başlangıçtan 1 saat sonrası]\n"
            "DETAILS: [Açıklama detayı]\n"
            "4. Bu yapılandırılmış bilginin üstüne (===CALENDAR=== satırının üstüne), kullanıcının okuyabilmesi için normal Türkçe görev özetini yaz.\n"
            "5. Giriş/açıklama veya \"İşte takvim raporu:\" gibi ön ekler ekleme."
        )
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
    return {"prompt": 0.0000015, "completion": 0.000009}

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
    buttons = [
        [
            InlineKeyboardButton(MODES["tldr"]["label"], callback_data="tldr"),
            InlineKeyboardButton(MODES["trans"]["label"], callback_data="trans")
        ],
        [
            InlineKeyboardButton(MODES["fix"]["label"], callback_data="fix"),
            InlineKeyboardButton(MODES["note"]["label"], callback_data="note")
        ],
        [
            InlineKeyboardButton(MODES["task"]["label"], callback_data="task")
        ]
    ]
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

            current_time_str = datetime.now().strftime("%d %B %Y %A, Saat: %H:%M")
            system_prompt = MODES[mode]["prompt"] + f"\n\nNot: Bugünün tarihi: {current_time_str}. Göreceli zamanları (yarın, haftaya vb.) buna göre hesapla."

            payload = {
                "model": TARGET_MODEL,
                "messages": [
                    {"role": "system", "content": system_prompt},
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

            # Parse Calendar info if present
            calendar_url = None
            clean_text = res_text
            if "===CALENDAR===" in res_text:
                parts = res_text.split("===CALENDAR===")
                clean_text = parts[0].strip()
                cal_info = parts[1].strip()
                
                title, start, end, details = "", "", "", ""
                for line in cal_info.split("\n"):
                    if line.startswith("TITLE:"): title = line.replace("TITLE:", "").strip()
                    elif line.startswith("START:"): start = line.replace("START:", "").strip()
                    elif line.startswith("END:"): end = line.replace("END:", "").strip()
                    elif line.startswith("DETAILS:"): details = line.replace("DETAILS:", "").strip()
                
                if title and start:
                    if not end:
                        end = start
                    params = {
                        "action": "TEMPLATE",
                        "text": title,
                        "dates": f"{start}/{end}",
                        "details": details
                    }
                    calendar_url = f"https://calendar.google.com/calendar/render?{urllib.parse.urlencode(params)}"

            chunks = split_message(clean_text)
            if not chunks:
                chunks = ["İşlem tamamlandı, detaylı görev veya takvim kaydı oluşturuldu."]
            
            copyable_text = f"<code>{html.escape(chunks[0])}</code>"
            
            keyboard = get_mode_keyboard()
            if calendar_url:
                keyboard.inline_keyboard.insert(0, [InlineKeyboardButton("📅 Google Takvime Ekle", url=calendar_url)])

            await bot.edit_message_text(
                chat_id=chat_id, 
                message_id=status_msg.message_id, 
                text=copyable_text,
                parse_mode=ParseMode.HTML,
                reply_markup=keyboard
            )

            for c in chunks[1:]:
                await bot.send_message(chat_id=chat_id, text=f"<code>{html.escape(c)}</code>", parse_mode=ParseMode.HTML)

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
            duration = update.message.voice.duration
            warning_text = ""
            if duration > 90:
                warning_text = f"\n\n⚠️ <b>Uyarı:</b> Ses kaydı uzun ({duration} sn). Vercel zaman aşımı (10 sn) sınırına takılma riski var."
            
            await bot.send_message(
                chat_id=update.message.chat.id,
                text=f"🚀 Ses kaydı alındı! Seçiniz:{warning_text}",
                reply_to_message_id=update.message.message_id,
                reply_markup=get_mode_keyboard(),
                parse_mode=ParseMode.HTML
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