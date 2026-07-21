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
GEMINI_API_KEY = os.environ.get("GEMINI_API_KEY") or os.environ.get("GOOGLE_API_KEY")
WEBHOOK_SECRET = os.environ.get("WEBHOOK_SECRET")
ALLOWED_USER_ID = os.environ.get("ALLOWED_USER_ID")

app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
bot = Bot(token=TELEGRAM_TOKEN)

# Model Yapılandırması (Env üzerinden esnek ve dinamik tanımlama)
DEFAULT_MODEL = os.environ.get("MODEL") or os.environ.get("GEMINI_MODEL") or "gemini-3.6-flash"
GOOGLE_MODEL = os.environ.get("GOOGLE_MODEL") or DEFAULT_MODEL
TARGET_MODEL = os.environ.get("OPENROUTER_MODEL") or (f"google/{DEFAULT_MODEL}" if not DEFAULT_MODEL.startswith("google/") else DEFAULT_MODEL)
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
    },
    "brainstorm": {
        "label": "🧠 Fikir Geliştir",
        "prompt": (
            "Sen vizyoner bir iş geliştirme uzmanı ve beyin fırtınası ortağısın. İletilen Türkçe ses kaydındaki fikirleri, konseptleri veya projeleri analiz et ve şu kurallara göre yapılandırılmış bir fikir geliştirme raporu oluştur:\n"
            "1. Kesinlikle dışarıdan bir giriş veya açıklama cümlesi ekleme, doğrudan markdown çıktısını üret.\n"
            "2. Raporu şu markdown bölümleriyle oluştur:\n"
            "   - `# Fikir/Proje Raporu` (Konuşulan konuya uygun yaratıcı bir başlık ekle)\n"
            "   - `## 💡 Temel Konsept` (Fikrin ne olduğunu ve çözdüğü problemi 2-3 cümleyle netçe açıkla)\n"
            "   - `## 🚀 Güçlü Yönler & Fırsatlar` (Fikrin en cazip ve avantajlı 3 yönünü listele)\n"
            "   - `## ⚠️ Dikkat Edilmesi Gereken Riskler` (Karşılaşılabilecek 2-3 potansiyel zorluğu listele)\n"
            "   - `## 🛠️ Sonraki Somut Adımlar` (Fikri hayata geçirmek için atılabilecek ilk 3 pratik adımı yaz)\n"
            "3. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n"
            "4. Vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__`, `_` veya HTML etiketleri kullanma)."
        )
    },
    "social": {
        "label": "📱 Sosyal Medya",
        "prompt": (
            "Sen profesyonel bir sosyal medya yöneticisi ve metin yazarısın. İletilen Türkçe ses kaydındaki konuyu analiz et ve iki farklı sosyal medya gönderi taslağı oluştur:\n"
            "1. Kesinlikle giriş veya açıklama cümlesi yazma, doğrudan gönderi taslaklarını üret.\n"
            "2. Raporu şu markdown yapısıyla oluştur:\n"
            "   - `# 📱 Sosyal Medya Paylaşımları`\n"
            "   - `## 🔗 LinkedIn Gönderisi` (Profesyonel, sektörel, emojiler içeren, ilgi çekici kanca cümleyle başlayan ve okumayı kolaylaştıran boşluklara sahip bir LinkedIn postu)\n"
            "   - `## 🐦 X (Twitter) Serisi` (Konunun ana hatlarını içeren, 3 veya 4 tweetlik numaralandırılmış bir flood/tweet serisi. Her tweet maksimum 280 karakter olmalı)\n"
            "3. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n"
            "4. Vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__`, `_` veya HTML etiketleri kullanma)."
        )
    },
    "translate": {
        "label": "🇬🇧 İngilizce Çeviri",
        "prompt": (
            "Sen profesyonel bir çevirmen ve dil uzmanısın. İletilen Türkçe ses kaydını dinle ve şu kurallara göre İngilizceye çevir:\n"
            "1. Ses kaydındaki konuşmayı anlam bütünlüğünü, duygusunu ve tonunu koruyarak akıcı, profesyonel bir İngilizceye (English) çevir.\n"
            "2. Çeviriye hiçbir yorum, düzeltme, açıklama veya ön söz/son söz ekleme. Doğrudan çevrilmiş İngilizce metni üret.\n"
            "3. Çevrilen metni paragraflar halinde yapılandır, gerekirse bölümleri ayırmak için H2 (`## `) veya H3 (`### `) başlıklar kullan.\n"
            "4. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n"
            "5. Vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__`, `_` veya HTML etiketleri kullanma)."
        )
    },
    "master": {
        "label": "🎯 Master Prompt",
        "prompt": (
            "Sen dünya çapında uzman bir Prompt Mühendisisin (Prompt Engineer). İletilen Türkçe ses kaydındaki konuyu, hedefleri, kapsamı ve detayları derinlemesine analiz et ve başka bir AI modelinin (GPT-4, Gemini, Claude vb.) bu konuyu en üst düzeyde icra edebilmesi için mükemmel, optimize edilmiş bir Master Prompt (Sistem İstemi) oluştur:\n"
            "1. Kesinlikle dışarıdan bir giriş, açıklama veya ön söz/son söz ekleme (Örn: \"İşte hazırladığım master prompt:\" deme), doğrudan üretilen promptun kendisini markdown formatında ver.\n"
            "2. Master Prompt yapısı tam olarak şu bölümleri içermelidir:\n"
            "   - `# 🎯 Master Prompt: [Konu/Hedef Başlığı]`\n"
            "   - `## 🎭 Rol ve Persona` (AI'ın üstleneceği uzmanlık rolü ve tonu)\n"
            "   - `## 📋 Bağlam ve Amaç` (Ses kaydında anlatılan konunun ve hedefin net tanımı)\n"
            "   - `## 🛠️ Temel Yönergeler` (Adım adım yapılması gerekenler ve kurallar)\n"
            "   - `## 📥 Girdi Verisi Yapısı` (Girdi olarak verilecek verinin formatı/yer tutucuları `[GİRDİ_VERİSİ]`)\n"
            "   - `## 📤 Çıktı Formatı ve Sınırlamalar` (Beklenen çıktının yapısı, kullanılacak format ve kaçınılacak şeyler)\n"
            "3. Çıktıyı kopyalanıp doğrudan başka bir yapay zekaya verilebilecek nitelikte, son derece profesyonel, anlaşılır ve kapsayıcı bir Türkçe ile hazırla.\n"
            "4. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma."
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
            InlineKeyboardButton(MODES["blog"]["label"], callback_data="blog"),
            InlineKeyboardButton(MODES["brainstorm"]["label"], callback_data="brainstorm")
        ],
        [
            InlineKeyboardButton(MODES["social"]["label"], callback_data="social"),
            InlineKeyboardButton(MODES["translate"]["label"], callback_data="translate")
        ],
        [
            InlineKeyboardButton(MODES["master"]["label"], callback_data="master")
        ]
    ]
    return InlineKeyboardMarkup(buttons)

async def process_voice(chat_id, file_id, mode="tldr", message_id=None, base_url=None, force_provider=None):
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

            current_time_str = datetime.now().strftime("%d %B %Y %A, Saat: %H:%M")
            system_prompt = MODES[mode]["prompt"] + f"\n\nNot: Bugünün tarihi: {current_time_str}. Göreceli zamanları (yarın, haftaya vb.) buna göre hesapla."

            # 1. Google Provider Denemesi (Eğer zorla openrouter seçilmediyse ve GEMINI_API_KEY tanımlıysa)
            if force_provider != "openrouter" and GEMINI_API_KEY:
                try:
                    await bot.edit_message_text(
                        chat_id=chat_id,
                        message_id=status_msg.message_id,
                        text=f"🔄 {current_label} hazırlanıyor... (Google Free Tier)"
                    )
                    google_url = f"https://generativelanguage.googleapis.com/v1beta/models/{GOOGLE_MODEL}:generateContent?key={GEMINI_API_KEY}"
                    google_payload = {
                        "system_instruction": {
                            "parts": [{"text": system_prompt}]
                        },
                        "contents": [
                            {
                                "parts": [
                                    {"text": "İşle."},
                                    {
                                        "inline_data": {
                                            "mime_type": "audio/ogg",
                                            "data": base64_audio
                                        }
                                    }
                                ]
                            }
                        ]
                    }

                    async with httpx.AsyncClient(timeout=30.0) as client:
                        g_resp = await client.post(google_url, json=google_payload)

                    if g_resp.status_code != 200:
                        raise Exception(f"HTTP {g_resp.status_code}: {g_resp.text[:100]}")

                    g_data = g_resp.json()
                    candidates = g_data.get("candidates", [])
                    if not candidates or "content" not in candidates[0] or "parts" not in candidates[0]["content"]:
                        raise Exception("Yanıtta geçerli içerik/part bulunamadı.")

                    clean_text = candidates[0]["content"]["parts"][0]["text"]

                    chunks = split_message(clean_text)
                    if not chunks:
                        chunks = ["İşlem tamamlandı."]

                    copyable_text = f"<code>{html.escape(chunks[0])}</code>"
                    keyboard = get_mode_keyboard()

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
                        f"└ Servis: <b>Google Free Tier</b> (<code>$0.00000</code>)"
                    )
                    await bot.send_message(chat_id=chat_id, text=cost_msg, parse_mode=ParseMode.HTML)
                    return

                except Exception as g_err:
                    logger.warning(f"Google Provider başarısız: {g_err}. Kullanıcıya OpenRouter onayı soruluyor.")
                    err_short = html.escape(str(g_err)[:150])
                    prompt_text = (
                        f"⚠️ <b>Google Free Tier ile işlem yapılamadı!</b>\n"
                        f"<i>Sebep: {err_short}</i>\n\n"
                        f"Ücretli <b>OpenRouter ({html.escape(TARGET_MODEL)})</b> servisi üzerinden devam etmek istiyor musunuz?"
                    )
                    confirm_keyboard = InlineKeyboardMarkup([
                        [InlineKeyboardButton("💳 Ücretli (OpenRouter) İle Çalıştır", callback_data=f"paid:{mode}")],
                        [InlineKeyboardButton("❌ İptal Et", callback_data="cancel_paid")]
                    ])
                    await bot.edit_message_text(
                        chat_id=chat_id,
                        message_id=status_msg.message_id,
                        text=prompt_text,
                        parse_mode=ParseMode.HTML,
                        reply_markup=confirm_keyboard
                    )
                    return

            # 2. OpenRouter Provider (Google kapalıysa, yoksa veya zorlanmışsa)
            await bot.edit_message_text(
                chat_id=chat_id,
                message_id=status_msg.message_id,
                text=f"🔄 {current_label} hazırlanıyor... (OpenRouter)"
            )
            pricing_task = asyncio.create_task(get_dynamic_pricing(TARGET_MODEL))

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
                raise Exception(f"OpenRouter API Hatası: {response.status_code}")

            res_data = response.json()
            res_text = res_data["choices"][0]["message"]["content"]
            usage = res_data.get("usage", {})

            current_prices = await pricing_task

            prompt_tokens = usage.get("prompt_tokens", 0)
            completion_tokens = usage.get("completion_tokens", 0)
            total_cost = (prompt_tokens * current_prices["prompt"]) + (completion_tokens * current_prices["completion"])

            clean_text = res_text

            chunks = split_message(clean_text)
            if not chunks:
                chunks = ["İşlem tamamlandı."]

            copyable_text = f"<code>{html.escape(chunks[0])}</code>"
            keyboard = get_mode_keyboard()

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
                f"├ Servis: <b>OpenRouter</b>\n"
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
            
            callback_data = query.data

            if callback_data == "cancel_paid":
                await bot.edit_message_text(
                    chat_id=query.message.chat.id,
                    message_id=query.message.message_id,
                    text="❌ İşlem iptal edildi."
                )
                return {"status": "ok"}

            force_provider = None
            if callback_data.startswith("paid:"):
                mode = callback_data.split(":")[1]
                force_provider = "openrouter"
            else:
                mode = callback_data

            voice_msg = query.message.reply_to_message
            
            if voice_msg and voice_msg.voice:
                base_url = str(request.base_url)
                await process_voice(
                    query.message.chat.id,
                    voice_msg.voice.file_id,
                    mode=mode,
                    message_id=query.message.message_id,
                    base_url=base_url,
                    force_provider=force_provider
                )
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