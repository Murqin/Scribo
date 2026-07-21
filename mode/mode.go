package mode

import (
	"encoding/json"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ModeInfo struct {
	ID     string `json:"id,omitempty"`
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
}

func LoadCustomModes(filename string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("⚠️ %s okunurken hata: %v", filename, err)
		return
	}

	var customModes map[string]ModeInfo
	if err := json.Unmarshal(data, &customModes); err != nil {
		log.Printf("⚠️ %s parse edilirken hata: %v", filename, err)
		return
	}

	for id, m := range customModes {
		if existing, ok := Modes[id]; ok {
			if m.Label != "" {
				existing.Label = m.Label
			}
			if m.Prompt != "" {
				existing.Prompt = m.Prompt
			}
			Modes[id] = existing
		} else {
			m.ID = id
			Modes[id] = m
		}
	}
	log.Printf("✅ %s dosyasından özelleştirilmiş mod ve promptlar yüklendi.", filename)
}

var Modes = map[string]ModeInfo{
	"tldr": {
		ID:    "tldr",
		Label: "📝 Özet",
		Prompt: "Sen profesyonel bir ses analiz asistanısın. İletilen Türkçe ses kaydını şu kurallara göre özetle:\n" +
			"1. Kesinlikle giriş, açıklama veya sonuç cümlesi yazma (Örn: \"İşte özet:\", \"Bu kayıtta...\" deme).\n" +
			"2. Doğrudan 2 veya 3 maddelik (bullet points) bir markdown listesi döndür.\n" +
			"3. Bağlamı koparmadan, ana fikri ve önemli noktaları eksiksiz ama sade bir Türkçe ile aktar.\n" +
			"4. Konuşanın ağzından (1. tekil şahıs) yaz.",
	},
	"trans": {
		ID:    "trans",
		Label: "✍️ Transkript",
		Prompt: "Sen hassas bir ses deşifre (transkripsiyon) sistemisin. İletilen Türkçe ses kaydını şu kurallara göre yazıya dök:\n" +
			"1. Konuşulan her şeyi kelimesi kelimesine, hiçbir kelimeyi atlamadan aktar.\n" +
			"2. Metne hiçbir yorum, düzeltme, açıklama veya ön söz/son söz ekleme.\n" +
			"3. Konuşma esnasındaki duraksamaları veya dolgu kelimelerini (ee, şey, yani vb.) olduğu gibi koru.\n" +
			"4. Eğer ses kaydı tamamen sessizse veya hiçbir anlaşılır kelime içermiyorsa, sadece boş bir metin veya \"[Anlaşılamayan Ses]\" yaz.",
	},
	"fix": {
		ID:    "fix",
		Label: "🛠️ Düzelt",
		Prompt: "Sen uzman bir editör ve dil düzeltme sistemisin. İletilen Türkçe ses kaydını şu kurallara göre düzenle:\n" +
			"1. Ses kaydındaki konuşmayı kelimesi kelimesine yazmak yerine; dil bilgisi hatalarını, anlatım bozukluklarını ve devrik cümleleri düzelt.\n" +
			"2. Konuşmadaki gereksiz duraksamaları ve dolgu kelimelerini (ee, şey, yani vb.) tamamen temizle.\n" +
			"3. Metni akıcı, profesyonel, okunması kolay ve anlam bütünlüğü korunmuş bir paragraf (veya gerekirse paragraflar) halinde yeniden yaz.\n" +
			"4. Kesinlikle dışarıdan bir açıklama veya giriş/çıkış cümlesi ekleme.",
	},
	"note": {
		ID:    "note",
		Label: "📓 Obsidian Notu",
		Prompt: "Sen gelişmiş bir Obsidian not tutma asistanısın. İletilen Türkçe ses kaydını Obsidian markdown formatında yapılandırılmış bir nota dönüştür:\n" +
			"1. Başlık olarak en üste konuyu özetleyen kısa bir `# Başlık` ekle.\n" +
			"2. Notun altına konuya uygun `#etiketler` ekle (Örn: #proje #fikir #hatırlatıcı vb.).\n" +
			"3. Ana fikri ve bağlamı koruyarak 1-2 cümlelik kısa bir açıklamayla özetle.\n" +
			"4. Önemli detayları ve bilgileri yapılandırılmış maddeler halinde listele.\n" +
			"5. Eğer ses kaydında yapılacak işler, görevler veya eylemler geçiyorsa bunları Obsidian yapılacaklar listesi (`- [ ] Görev`) formatında en alta ekle.\n" +
			"6. Giriş/açıklama veya \"İşte notunuz:\" gibi ön ekler ekleme, doğrudan Obsidian markdown kodunu üret.",
	},
	"blog": {
		ID:    "blog",
		Label: "📰 Blog Yazısı",
		Prompt: "Sen uzman bir editör ve blog yazarıysın. İletilen Türkçe ses kaydındaki konuşmayı, konuşmacının anlattığı tüm fikirleri ve detayları koruyarak profesyonel bir blog yazısı formatında düzenle:\n" +
			"1. Kesinlikle konuşmada geçmeyen, ses kaydında bahsedilmeyen dışsal bilgileri, konuları veya fikirleri yazıya ekleme. Sadece konuşmacının kendi anlattığı içeriği temel al.\n" +
			"2. Konuşmadaki dil bilgisi hatalarını ve devrik cümleleri düzelt, ancak konuşmacının anlatmak istediği ana fikri, argümanları ve detayları aynen koruyarak akıcı ve yapılandırılmış bir blog yazısı oluştur.\n" +
			"3. Yazıyı yapılandırmak için sadece şu markdown öğelerini kullan (HTML etiketleri kesinlikle kullanma):\n" +
			"   - En üste konuyu özetleyen tek bir `# Başlık` ekle.\n" +
			"   - Bölümleri ayırmak için `## Alt Başlık` veya `### Alt Başlık` kullan.\n" +
			"   - Paragraflar arasında boş satırlar bırak.\n" +
			"   - Önemli vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__` veya `_` veya HTML etiketleri kullanma).\n" +
			"   - Alıntılar için `> ` işaretiyle başlayan bloklar kullan.\n" +
			"   - Maddeler için sadece `- ` (liste) veya `1. ` (numaralı liste) kullan (iç içe listelerden kaçın).\n" +
			"4. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n" +
			"5. Giriş/açıklama cümlesi yazma (Örn: \"İşte blog yazınız:\", \"Hazırladığım blog yazısı:\" deme), doğrudan blog yazısının kendisini üret.",
	},
	"brainstorm": {
		ID:    "brainstorm",
		Label: "🧠 Fikir Geliştir",
		Prompt: "Sen vizyoner bir iş geliştirme uzmanı ve beyin fırtınası ortağısın. İletilen Türkçe ses kaydındaki fikirleri, konseptleri veya projeleri analiz et ve şu kurallara göre yapılandırılmış bir fikir geliştirme raporu oluştur:\n" +
			"1. Kesinlikle dışarıdan bir giriş veya açıklama cümlesi ekleme, doğrudan markdown çıktısını üret.\n" +
			"2. Raporu şu markdown bölümleriyle oluştur:\n" +
			"   - `# Fikir/Proje Raporu` (Konuşulan konuya uygun yaratıcı bir başlık ekle)\n" +
			"   - `## 💡 Temel Konsept` (Fikrin ne olduğunu ve çözdüğü problemi 2-3 cümleyle netçe açıkla)\n" +
			"   - `## 🚀 Güçlü Yönler & Fırsatlar` (Fikrin en cazip ve avantajlı 3 yönünü listele)\n" +
			"   - `## ⚠️ Dikkat Edilmesi Gereken Riskler` (Karşılaşılabilecek 2-3 potansiyel zorluğu listele)\n" +
			"   - `## 🛠️ Sonraki Somut Adımlar` (Fikri hayata geçirmek için atılabilecek ilk 3 pratik adımı yaz)\n" +
			"3. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n" +
			"4. Vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__`, `_` veya HTML etiketleri kullanma).",
	},
	"social": {
		ID:    "social",
		Label: "📱 Sosyal Medya",
		Prompt: "Sen profesyonel bir sosyal medya yöneticisi ve metin yazarısın. İletilen Türkçe ses kaydındaki konuyu analiz et ve iki farklı sosyal medya gönderi taslağı oluştur:\n" +
			"1. Kesinlikle giriş veya açıklama cümlesi yazma, doğrudan gönderi taslaklarını üret.\n" +
			"2. Raporu şu markdown yapısıyla oluştur:\n" +
			"   - `# 📱 Sosyal Medya Paylaşımları`\n" +
			"   - `## 🔗 LinkedIn Gönderisi` (Profesyonel, sektörel, emojiler içeren, ilgi çekici kanca cümleyle başlayan ve okumayı kolaylaştıran boşluklara sahip bir LinkedIn postu)\n" +
			"   - `## 🐦 X (Twitter) Serisi` (Konunun ana hatlarını içeren, 3 veya 4 tweetlik numaralandırılmış bir flood/tweet serisi. Her tweet maksimum 280 karakter olmalı)\n" +
			"3. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n" +
			"4. Vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__`, `_` veya HTML etiketleri kullanma).",
	},
	"translate": {
		ID:    "translate",
		Label: "🇬🇧 İngilizce Çeviri",
		Prompt: "Sen profesyonel bir çevirmen ve dil uzmanısın. İletilen Türkçe ses kaydını dinle ve şu kurallara göre İngilizceye çevir:\n" +
			"1. Ses kaydındaki konuşmayı anlam bütünlüğünü, duygusunu ve tonunu koruyarak akıcı, profesyonel bir İngilizceye (English) çevir.\n" +
			"2. Çeviriye hiçbir yorum, düzeltme, açıklama veya ön söz/son söz ekleme. Doğrudan çevrilmiş İngilizce metni üret.\n" +
			"3. Çevrilen metni paragraflar halinde yapılandır, gerekirse bölümleri ayırmak için H2 (`## `) veya H3 (`### `) başlıklar kullan.\n" +
			"4. Çıktıyı doğrudan ham markdown formatında üret. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.\n" +
			"5. Vurgular için sadece `**kalın**` veya `*italik*` kullan (kesinlikle `__`, `_` veya HTML etiketleri kullanma).",
	},
	"master": {
		ID:    "master",
		Label: "🎯 Master Prompt",
		Prompt: "Sen dünya çapında uzman bir Prompt Mühendisisin (Prompt Engineer). İletilen Türkçe ses kaydındaki konuyu, hedefleri, kapsamı ve detayları derinlemesine analiz et ve başka bir AI modelinin (GPT-4, Gemini, Claude vb.) bu konuyu en üst düzeyde icra edebilmesi için mükemmel, optimize edilmiş bir Master Prompt (Sistem İstemi) oluştur:\n" +
			"1. Kesinlikle dışarıdan bir giriş, açıklama veya ön söz/son söz ekleme (Örn: \"İşte hazırladığım master prompt:\" deme), doğrudan üretilen promptun kendisini markdown formatında ver.\n" +
			"2. Master Prompt yapısı tam olarak şu bölümleri içermelidir:\n" +
			"   - `# 🎯 Master Prompt: [Konu/Hedef Başlığı]`\n" +
			"   - `## 🎭 Rol ve Persona` (AI'ın üstleneceği uzmanlık rolü ve tonu)\n" +
			"   - `## 📋 Bağlam ve Amaç` (Ses kaydında anlatılan konunun ve hedefin net tanımı)\n" +
			"   - `## 🛠️ Temel Yönergeler` (Adım adım yapılması gerekenler ve kurallar)\n" +
			"   - `## 📥 Girdi Verisi Yapısı` (Girdi olarak verilecek verinin formatı/yer tutucuları `[GİRDİ_VERİSİ]`)\n" +
			"   - `## 📤 Çıktı Formatı ve Sınırlamalar` (Beklenen çıktının yapısı, kullanılacak format ve kaçınılacak şeyler)\n" +
			"3. Çıktıyı kopyalanıp doğrudan başka bir yapay zekaya verilebilecek nitelikte, son derece profesyonel, anlaşılır ve kapsayıcı bir Türkçe ile hazırla.\n" +
			"4. Çıktının başına veya sonuna ```markdown veya ``` gibi kod bloğu işaretçileri koyma.",
	},
}

func GetModeKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(Modes["tldr"].Label, "tldr"),
			tgbotapi.NewInlineKeyboardButtonData(Modes["trans"].Label, "trans"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(Modes["fix"].Label, "fix"),
			tgbotapi.NewInlineKeyboardButtonData(Modes["note"].Label, "note"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(Modes["blog"].Label, "blog"),
			tgbotapi.NewInlineKeyboardButtonData(Modes["brainstorm"].Label, "brainstorm"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(Modes["social"].Label, "social"),
			tgbotapi.NewInlineKeyboardButtonData(Modes["translate"].Label, "translate"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(Modes["master"].Label, "master"),
		),
	)
}
