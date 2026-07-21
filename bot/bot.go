package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"scribo/config"
	"scribo/mode"
	"scribo/provider"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotRunner struct {
	cfg                *config.Config
	api                *tgbotapi.BotAPI
	googleProvider     provider.AIProvider
	openRouterProvider provider.AIProvider
	httpClient         *http.Client
	activeLocks        sync.Map
	workerSem          chan struct{}
	wg                 sync.WaitGroup
}

func NewBotRunner(cfg *config.Config) (*BotRunner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, err
	}

	slog.Info("🤖 Telegram Bot yetkilendirildi", "username", bot.Self.UserName, "maxConcurrentJobs", cfg.MaxConcurrentJobs)

	return &BotRunner{
		cfg:                cfg,
		api:                bot,
		googleProvider:     provider.NewGoogleProvider(cfg.GeminiAPIKey, cfg.GoogleModel),
		openRouterProvider: provider.NewOpenRouterProvider(cfg.OpenRouterAPIKey, cfg.OpenRouterModel),
		httpClient:         &http.Client{Timeout: 60 * time.Second},
		workerSem:          make(chan struct{}, cfg.MaxConcurrentJobs),
	}, nil
}

func (b *BotRunner) StartPolling(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			slog.Info("🛑 Polling durduruluyor, aktif işlemlerin tamamlanması bekleniyor...")
			b.wg.Wait()
			slog.Info("✅ Tüm aktif işlemler tamamlandı.")
			return nil
		case update, ok := <-updates:
			if !ok {
				b.wg.Wait()
				return nil
			}
			if update.Message != nil {
				b.handleMessage(ctx, update.Message)
			} else if update.CallbackQuery != nil {
				b.handleCallbackQuery(ctx, update.CallbackQuery)
			}
		}
	}
}

func (b *BotRunner) isAuthorized(userID int64) bool {
	if b.cfg.AllowedUserID == "" {
		return true
	}
	return fmt.Sprintf("%d", userID) == b.cfg.AllowedUserID
}

type AudioTarget struct {
	FileID   string
	FileSize int
	Duration int
	Name     string
	MimeType string
}

func mimeTypeFromExt(ext string) string {
	switch ext {
	case ".mp3":
		return "audio/mp3"
	case ".m4a":
		return "audio/m4a"
	case ".wav":
		return "audio/wav"
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".opus":
		return "audio/opus"
	default:
		return "audio/ogg"
	}
}

func extractAudioTarget(msg *tgbotapi.Message) *AudioTarget {
	if msg == nil {
		return nil
	}
	if msg.Voice != nil {
		return &AudioTarget{
			FileID:   msg.Voice.FileID,
			FileSize: msg.Voice.FileSize,
			Duration: msg.Voice.Duration,
			Name:     "Ses Kaydı",
			MimeType: "audio/ogg",
		}
	}
	if msg.Audio != nil {
		ext := strings.ToLower(filepath.Ext(msg.Audio.FileName))
		return &AudioTarget{
			FileID:   msg.Audio.FileID,
			FileSize: msg.Audio.FileSize,
			Duration: msg.Audio.Duration,
			Name:     msg.Audio.FileName,
			MimeType: mimeTypeFromExt(ext),
		}
	}
	if msg.Document != nil {
		ext := strings.ToLower(filepath.Ext(msg.Document.FileName))
		switch ext {
		case ".ogg", ".mp3", ".m4a", ".wav", ".aac", ".flac", ".opus":
			return &AudioTarget{
				FileID:   msg.Document.FileID,
				FileSize: msg.Document.FileSize,
				Duration: 0,
				Name:     msg.Document.FileName,
				MimeType: mimeTypeFromExt(ext),
			}
		}
	}
	return nil
}

func (b *BotRunner) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	if !b.isAuthorized(msg.From.ID) {
		return
	}

	if msg.IsCommand() && msg.Command() == "start" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "🎙️ <b>Scribo Bot Hazır!</b>\nBir ses kaydı, MP3 veya ses dosyası gönderin.")
		reply.ParseMode = tgbotapi.ModeHTML
		b.sendMsg(reply)
		return
	}

	audioTarget := extractAudioTarget(msg)
	if audioTarget != nil {
		// Check 20 MB Telegram bot limit
		if audioTarget.FileSize > 20*1024*1024 {
			reply := tgbotapi.NewMessage(msg.Chat.ID, "⚠️ <b>Hata:</b> Dosya boyutu çok büyük (Maksimum Telegram bot limiti: 20 MB).")
			reply.ParseMode = tgbotapi.ModeHTML
			reply.ReplyToMessageID = msg.MessageID
			b.sendMsg(reply)
			return
		}

		warningText := ""
		if audioTarget.Duration > 90 {
			warningText = fmt.Sprintf("\n\n⚠️ <b>Uyarı:</b> Ses kaydı uzun (%d sn). Processing biraz zaman alabilir.", audioTarget.Duration)
		}

		replyText := fmt.Sprintf("🚀 <b>%s</b> alındı! Bir işlem modu seçiniz:%s", html.EscapeString(audioTarget.Name), warningText)
		reply := tgbotapi.NewMessage(msg.Chat.ID, replyText)
		reply.ParseMode = tgbotapi.ModeHTML
		reply.ReplyToMessageID = msg.MessageID
		reply.ReplyMarkup = mode.GetModeKeyboard()

		b.sendMsg(reply)
		return
	}

	// Guidance message for unsupported inputs
	if !msg.IsCommand() {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "🎙️ Lütfen analiz edilmek üzere bir ses kaydı (Voice note) veya ses dosyası (MP3, M4A, WAV, FLAC, OGG) gönderin.")
		reply.ReplyToMessageID = msg.MessageID
		b.sendMsg(reply)
	}
}

func (b *BotRunner) handleCallbackQuery(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if !b.isAuthorized(cb.From.ID) {
		callbackCfg := tgbotapi.NewCallback(cb.ID, "Yetkisiz kullanıcı.")
		callbackCfg.ShowAlert = true
		b.api.Request(callbackCfg)
		return
	}

	b.api.Request(tgbotapi.NewCallback(cb.ID, ""))

	data := cb.Data
	if data == "cancel_paid" {
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "❌ İşlem iptal edildi.")
		b.sendMsg(editMsg)
		return
	}

	targetMode := data
	forceProvider := ""

	if strings.HasPrefix(data, "paid:") {
		targetMode = strings.TrimPrefix(data, "paid:")
		forceProvider = "openrouter"
	}

	var audioMsg *tgbotapi.Message
	if cb.Message.ReplyToMessage != nil {
		audioMsg = cb.Message.ReplyToMessage
	}

	audioTarget := extractAudioTarget(audioMsg)
	if audioTarget == nil {
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "❌ Kaynak ses dosyası bulunamadı.")
		b.sendMsg(editMsg)
		return
	}

	lockKey := fmt.Sprintf("%d:%d", cb.Message.Chat.ID, cb.Message.MessageID)
	if _, loaded := b.activeLocks.LoadOrStore(lockKey, true); loaded {
		slog.Warn("Çift tıklama veya aynı mesaj üzerinde devam eden işlem engellendi", "key", lockKey)
		return
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer b.activeLocks.Delete(lockKey)

		select {
		case b.workerSem <- struct{}{}:
			defer func() { <-b.workerSem }()
		case <-ctx.Done():
			slog.Warn("Bağlam sonlandırıldı, işlem iptal edildi", "key", lockKey)
			return
		}

		b.processVoice(ctx, cb.Message.Chat.ID, audioTarget.FileID, targetMode, cb.Message.MessageID, forceProvider, audioTarget.MimeType)
	}()
}

func (b *BotRunner) sendTypingAction(ctx context.Context, chatID int64) func() {
	stopChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		b.sendMsg(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
		for {
			select {
			case <-ticker.C:
				b.sendMsg(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(stopChan) })
	}
}

func (b *BotRunner) processVoice(ctx context.Context, chatID int64, fileID string, modeID string, statusMsgID int, forceProvider string, mimeType string) {
	stopTyping := b.sendTypingAction(ctx, chatID)
	defer stopTyping()

	modeInfo, ok := mode.GetMode(modeID)
	if !ok {
		modeInfo, _ = mode.GetMode("tldr")
	}

	msg := tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 <b>%s</b> hazırlanıyor...", modeInfo.Label))
	msg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(msg)

	fileURL, err := b.api.GetFileDirectURL(fileID)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("Ses dosyası URL'si alınamadı: %v", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("İstek oluşturulamadı: %v", err))
		return
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("Ses dosyası indirilemedi: %v", err))
		return
	}
	defer resp.Body.Close()

	// Safe limited read to prevent OOM
	limitReader := io.LimitReader(resp.Body, 20*1024*1024+1024)
	audioBytes, err := io.ReadAll(limitReader)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("Ses dosyası okunamadı: %v", err))
		return
	}

	if len(audioBytes) > 20*1024*1024 {
		b.sendError(chatID, statusMsgID, modeID, "İndirilen dosya boyutu 20 MB sınırını aştı.")
		return
	}

	base64Audio := base64.StdEncoding.EncodeToString(audioBytes)
	currentTimeStr := time.Now().Format("02 January 2006 Monday, Saat: 15:04")
	systemPrompt := fmt.Sprintf("%s\n\nNot: Bugünün tarihi: %s. Göreceli zamanları buna göre hesapla.", modeInfo.Prompt, currentTimeStr)

	selectedProvider := forceProvider
	if selectedProvider == "" {
		selectedProvider = b.cfg.DefaultProvider
	}

	// 1. Google Provider Try
	if selectedProvider != "openrouter" && b.cfg.GeminiAPIKey != "" {
		gMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 <b>%s</b> hazırlanıyor... (Google Free Tier)", modeInfo.Label))
		gMsg.ParseMode = tgbotapi.ModeHTML
		b.sendMsg(gMsg)

		res, gErr := b.googleProvider.Generate(ctx, systemPrompt, base64Audio, mimeType)
		if gErr == nil {
			b.sendSuccessResponse(chatID, statusMsgID, res.Text, "<b>Google Free Tier</b> (<code>$0.00000</code>)")
			return
		}

		slog.Warn("Google API başarısız, OpenRouter onayı soruluyor", "error", gErr)
		errShort := html.EscapeString(gErr.Error())
		if len(errShort) > 200 {
			errShort = errShort[:200] + "..."
		}

		promptText := fmt.Sprintf(
			"⚠️ <b>Google Free Tier ile işlem yapılamadı!</b>\n<i>Sebep: %s</i>\n\nÜcretli <b>OpenRouter (%s)</b> servisi üzerinden devam etmek istiyor musunuz?",
			errShort, html.EscapeString(b.cfg.OpenRouterModel),
		)

		confirmKeyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("💳 Ücretli (OpenRouter) İle Çalıştır", "paid:"+modeID),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("❌ İptal Et", "cancel_paid"),
			),
		)

		promptMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, promptText)
		promptMsg.ParseMode = tgbotapi.ModeHTML
		promptMsg.ReplyMarkup = &confirmKeyboard
		b.sendMsg(promptMsg)
		return
	}

	// 2. OpenRouter Provider Try
	orMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 <b>%s</b> hazırlanıyor... (OpenRouter)", modeInfo.Label))
	orMsg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(orMsg)

	res, err := b.openRouterProvider.Generate(ctx, systemPrompt, base64Audio, mimeType)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("OpenRouter Hatası: %v", err))
		return
	}

	costInfo := fmt.Sprintf("<b>OpenRouter</b>\n├ Token: %d (P: %d, C: %d)\n└ Maliyet: <code>$%s</code>",
		res.PromptTokens+res.CompletionTokens, res.PromptTokens, res.CompletionTokens, fmt.Sprintf("%.5f", res.TotalCost))

	b.sendSuccessResponse(chatID, statusMsgID, res.Text, costInfo)
}

func (b *BotRunner) sendSuccessResponse(chatID int64, statusMsgID int, cleanText string, costDetail string) {
	chunks := splitMessage(cleanText, 3900)
	if len(chunks) == 0 {
		chunks = []string{"İşlem tamamlandı."}
	}

	firstChunkText := chunks[0]
	kb := mode.GetModeKeyboard()

	editMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, firstChunkText)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &kb
	_, err := b.api.Send(editMsg)
	if err != nil {
		// Fallback to plain text if HTML parsing fails
		editMsg.ParseMode = ""
		b.sendMsg(editMsg)
	}

	for _, c := range chunks[1:] {
		msg := tgbotapi.NewMessage(chatID, c)
		msg.ParseMode = tgbotapi.ModeHTML
		_, err := b.api.Send(msg)
		if err != nil {
			msg.ParseMode = ""
			b.sendMsg(msg)
		}
	}

	costMsgText := fmt.Sprintf("📊 <b>Kullanım Özeti:</b>\n└ Servis: %s", costDetail)
	costMsg := tgbotapi.NewMessage(chatID, costMsgText)
	costMsg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(costMsg)
}

func (b *BotRunner) sendError(chatID int64, statusMsgID int, modeID string, errText string) {
	slog.Error("İşlem hatası", "chatID", chatID, "error", errText)
	txt := fmt.Sprintf("❌ <b>İşlem Hatası:</b>\n<pre>%s</pre>", html.EscapeString(errText))
	retryKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Tekrar Dene", modeID),
		),
	)

	editMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, txt)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &retryKb
	b.sendMsg(editMsg)
}

func (b *BotRunner) sendMsg(chg tgbotapi.Chattable) {
	if _, err := b.api.Send(chg); err != nil {
		slog.Error("Telegram mesaj gönderimi başarısız", "error", err)
	}
}

func splitMessage(text string, maxLength int) []string {
	if text == "" {
		return nil
	}
	var chunks []string
	for len(text) > maxLength {
		splitIdx := strings.LastIndex(text[:maxLength], "\n")
		if splitIdx <= 0 {
			splitIdx = strings.LastIndex(text[:maxLength], " ")
		}
		if splitIdx <= 0 {
			splitIdx = maxLength
		}
		chunks = append(chunks, strings.TrimSpace(text[:splitIdx]))
		text = strings.TrimSpace(text[splitIdx:])
	}
	if text != "" {
		chunks = append(chunks, strings.TrimSpace(text))
	}
	return chunks
}

