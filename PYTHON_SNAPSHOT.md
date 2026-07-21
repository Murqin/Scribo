# PYTHON SNAPSHOT & BENCHMARK DOKÜMANI (v1.0.0-python)

> **Amaç:** Go (Golang) diline geçiş (migration) sürecinde ve sonrasında tüm modların, promptların, API payload yapılarının ve klavye düzeninin birebir korunduğunu test edip doğrulamak için alınan teknik referans snapshot'ı.

---

## 1. ⚙️ Çevre Değişkenleri (Environment Variables)

```env
TELEGRAM_TOKEN=your_telegram_bot_token
OPENROUTER_API_KEY=your_openrouter_api_key
GEMINI_API_KEY=your_google_ai_studio_free_tier_key
MODEL=gemini-3.6-flash
GOOGLE_MODEL=gemini-3.6-flash
OPENROUTER_MODEL=google/gemini-3.6-flash
WEBHOOK_SECRET=your_webhook_secret
ALLOWED_USER_ID=your_telegram_user_id
```

---

## 2. 🏷️ Desteklenen Modlar ve Sistem Promptları (9 Mod)

| Mod ID | Buton Etiketi (Label) | Tanım |
| :--- | :--- | :--- |
| `tldr` | `📝 Özet` | 2-3 maddelik Türkçe özet (1. şahıs ağzından) |
| `trans` | `✍️ Transkript` | Kelimesi kelimesine birebir deşifre |
| `fix` | `🛠️ Düzelt` | Dil bilgisi/anlatım düzeltilmiş akıcı paragraf |
| `note` | `📓 Obsidian Notu` | Başlık, etiketler, özet ve görev checklist'li Obsidian notu |
| `blog` | `📰 Blog Yazısı` | Yapılandırılmış H1/H2/H3 markdown blog yazısı |
| `brainstorm` | `🧠 Fikir Geliştir` | Konsept, SWOT ve sonraki adımları içeren fikir raporu |
| `social` | `📱 Sosyal Medya` | LinkedIn ve X (Twitter) flood gönderi taslağı |
| `translate` | `🇬🇧 İngilizce Çeviri` | Akıcı profesyonel İngilizce çeviri |
| `master` | `🎯 Master Prompt` | Başka AI'larda kullanılabilir uzman Sistem İstemi (Master Prompt) |

---

## 3. 🌐 API İstek Yapıları (Payloads)

### A. Primary Provider: Google Direct API (Free Tier)
- **Endpoint:** `POST https://generativelanguage.googleapis.com/v1beta/models/{GOOGLE_MODEL}:generateContent?key={GEMINI_API_KEY}`
- **Payload:**
```json
{
  "system_instruction": {
    "parts": [{"text": "<SYSTEM_PROMPT>\n\nNot: Bugünün tarihi: <DATE_TIME_STRING>"}]
  },
  "contents": [
    {
      "parts": [
        {"text": "İşle."},
        {
          "inline_data": {
            "mime_type": "audio/ogg",
            "data": "<BASE64_AUDIO_DATA>"
          }
        }
      ]
    }
  ]
}
```

### B. Fallback Provider: OpenRouter API (Paid)
- **Endpoint:** `POST https://openrouter.ai/api/v1/chat/completions`
- **Headers:** `Authorization: Bearer <OPENROUTER_API_KEY>`, `Content-Type: application/json`
- **Payload:**
```json
{
  "model": "<TARGET_MODEL>",
  "messages": [
    {
      "role": "system",
      "content": "<SYSTEM_PROMPT>\n\nNot: Bugünün tarihi: <DATE_TIME_STRING>"
    },
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "İşle."},
        {"type": "input_audio", "input_audio": {"data": "<BASE64_AUDIO_DATA>", "format": "ogg"}}
      ]
    }
  ]
}
```

---

## 4. ⌨️ Telegram İnline Klavye Yapısı (Keyboard Layout)

```text
[ 📝 Özet ]         [ ✍️ Transkript ]
[ 🛠️ Düzelt ]       [ 📓 Obsidian Notu ]
[ 📰 Blog Yazısı ]   [ 🧠 Fikir Geliştir ]
[ 📱 Sosyal Medya ]  [ 🇬🇧 İngilizce Çeviri ]
[ 🎯 Master Prompt ] (Geniş Satır)
```

---

## 5. ⚡ Hata Yönetimi & Fallback Akışı

1. Kullanıcı ses kaydı iletir -> Mod butonunu seçer.
2. Bot önce **Google Free Tier** dener.
3. Google isteği patlarsa (Quota / Rate Limit / Error):
   - Kullanıcıya interaktif soru sorulur: `Ücretli OpenRouter (<TARGET_MODEL>) servisi üzerinden devam etmek istiyor musunuz?`
   - Butonlar: `[ 💳 Ücretli (OpenRouter) İle Çalıştır ]` (callback: `paid:<mode>`) ve `[ ❌ İptal Et ]` (callback: `cancel_paid`).
4. Kullanıcı onaylarsa OpenRouter isteği atılır, token ve dinamik maliyet hesaplanır.

---

## 6. 📸 Snapshot Doğrulama Test Kriterleri (Benchmark Suite)

Go migrasyonu tamamlandığında şu 4 testin %100 geçmesi gerekir:
1. [ ] **Ses Analiz Testi:** Her 9 mod için gönderilen test `.ogg` ses dosyası doğru mod kurallarına göre metne dönüştürülmeli.
2. [ ] **Kullanıcı Yetkilendirme Testi:** `ALLOWED_USER_ID` haricindeki Telegram ID'leri engellenmeli.
3. [ ] **Google -> OpenRouter Fallback Testi:** Geçersiz `GEMINI_API_KEY` verildiğinde bot kullanıcıya ücretli geçiş sorusunu sormalı.
4. [ ] **Uzun Mesaj Bölme Testi:** 4000 karakteri aşan yanıtlar Telegram mesaj sınırı nedeniyle düzgün parçalara bölünerek iletilmeli.
