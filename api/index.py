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
from fastapi.responses import HTMLResponse
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
    "blog": {
        "label": "📰 Blog Yazısı",
        "prompt": (
            "Sen uzman bir editör ve blog yazarıysın. İletilen Türkçe ses kaydındaki konuşmayı, konuşmacının anlattığı tüm fikirleri ve detayları koruyarak profesyonel bir blog yazısı formatında düzenle:\n"
            "1. Kesinlikle konuşmada geçmeyen, ses kaydında bahsedilmeyen dışsal bilgileri, konuları veya fikirleri yazıya ekleme. Sadece konuşmacının kendi anlattığı içeriği temel al.\n"
            "2. Konuşmadaki dil bilgisi hatalarını ve devrik cümleleri düzelt, ancak konuşmacının anlatmak istediği ana fikri, argümanları ve detayları aynen koruyarak akıcı ve yapılandırılmış bir blog yazısı oluştur.\n"
            "3. Yazıyı yapılandırmak için sadece şu markdown öğelerini kullan (HTML etiketleri kesinlikle kullanma):\n"
            "   - En üste konuyu özetleyen tek bir `# Başlık` ekle.\n"
            "   - Bölümleri ayırmak için `## Alt Başlık` veya `### Alt Başlık` kullan.\n"
            "   - Paragraflar arasında boş satırlar bırak.\n"
            "   - Önemli vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__` veya `_` veya HTML etiketleri kullanma).\n"
            "   - Alıntılar için `> ` işaretiyle başlayan bloklar kullan.\n"
            "   - Maddeler için sadece `- ` (liste) veya `1. ` (numaralı liste) kullan (iç içe listelerden kaçın).\n"
            "4. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n"
            "5. Giriş/açıklama cümlesi yazma (Örn: \"İşte blog yazınız:\", \"Hazırladığım blog yazısı:\" deme), doğrudan blog yazısının kendisini üret."
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

def get_mode_keyboard(obsidian_url=None):
    buttons = []
    if obsidian_url:
        buttons.append([InlineKeyboardButton("📓 Obsidian'a Aktar", url=obsidian_url)])
        
    buttons.extend([
        [
            InlineKeyboardButton(MODES["tldr"]["label"], callback_data="tldr"),
            InlineKeyboardButton(MODES["trans"]["label"], callback_data="trans")
        ],
        [
            InlineKeyboardButton(MODES["fix"]["label"], callback_data="fix"),
            InlineKeyboardButton(MODES["note"]["label"], callback_data="note")
        ],
        [
            InlineKeyboardButton(MODES["blog"]["label"], callback_data="blog")
        ]
    ])
    return InlineKeyboardMarkup(buttons)

async def process_voice(chat_id, file_id, mode="tldr", message_id=None, base_url=None):
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

            clean_text = res_text

            # Parse Obsidian URL if in note or blog mode
            obsidian_url = None
            if mode in ("note", "blog") and base_url:
                note_title = "Ses Notu" if mode == "note" else "Blog Yazısı"
                lines = clean_text.split("\n")
                for line in lines:
                    if line.startswith("# "):
                        note_title = line.replace("# ", "").strip()
                        break
                
                safe_content = clean_text
                if len(safe_content) > 1500:
                    safe_content = safe_content[:1500] + "\n\n...(Kırpıldı)"
                
                params = {
                    "name": note_title,
                    "content": safe_content
                }
                obsidian_url = f"{base_url}obsidian?{urllib.parse.urlencode(params)}"

            chunks = split_message(clean_text)
            if not chunks:
                chunks = ["İşlem tamamlandı."]
            
            copyable_text = f"<code>{html.escape(chunks[0])}</code>"
            
            keyboard = get_mode_keyboard(obsidian_url=obsidian_url)

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

@app.get("/obsidian", response_class=HTMLResponse)
async def open_obsidian(name: str = "", content: str = ""):
    # URL'den gelen verileri javascript ile obsidian://new formatına yönlendiriyoruz
    html_content = f"""
    <!DOCTYPE html>
    <html>
    <head>
        <title>Obsidian'a Aktarılıyor...</title>
        <meta charset="utf-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <style>
            body {{
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                height: 100vh;
                margin: 0;
                background-color: #1e1e1e;
                color: #ffffff;
                text-align: center;
                padding: 20px;
            }}
            .loader {{
                border: 4px solid #333;
                border-top: 4px solid #8b5cf6;
                border-radius: 50%;
                width: 40px;
                height: 40px;
                animation: spin 1s linear infinite;
                margin-bottom: 20px;
            }}
            @keyframes spin {{
                0% {{ transform: rotate(0deg); }}
                100% {{ transform: rotate(360deg); }}
            }}
            .btn {{
                background-color: #8b5cf6;
                color: white;
                border: none;
                padding: 10px 20px;
                border-radius: 5px;
                cursor: pointer;
                font-weight: bold;
                text-decoration: none;
                margin-top: 20px;
            }}
        </style>
    </head>
    <body>
        <div class="loader"></div>
        <h2>Obsidian Açılıyor...</h2>
        <p>Notunuz aktarılıyor. Eğer uygulama açılmazsa aşağıdaki butona tıklayabilirsiniz:</p>
        <a id="obsidian-link" class="btn" href="#">Obsidian'ı Aç</a>

        <script>
            const name = {repr(name)};
            const content = {repr(content)};
            const uri = "obsidian://new?name=" + encodeURIComponent(name) + "&content=" + encodeURIComponent(content);
            
            document.getElementById("obsidian-link").href = uri;
            
            // Otomatik yönlendir
            window.location.href = uri;
        </script>
    </body>
    </html>
    """
    return HTMLResponse(content=html_content)

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
                base_url = str(request.base_url)
                await process_voice(query.message.chat.id, voice_msg.voice.file_id, mode=mode, message_id=query.message.message_id, base_url=base_url)
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