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
	"time"

	"scribo/config"
	"scribo/mode"
	"scribo/provider"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotRunner struct {
	cfg            *config.Config
	api            *tgbotapi.BotAPI
	googleProvider provider.AIProvider
	openRouterProvider provider.AIProvider
}

func NewBotRunner(cfg *config.Config) (*BotRunner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, err
	}

	slog.Info("🤖 Telegram Bot yetkilendirildi", "username", bot.Self.UserName)

	return &BotRunner{
		cfg:                cfg,
		api:                bot,
		googleProvider:     provider.NewGoogleProvider(cfg.GeminiAPIKey, cfg.GoogleModel),
		openRouterProvider: provider.NewOpenRouterProvider(cfg.OpenRouterAPIKey, cfg.OpenRouterModel),
	}, nil
}

func (b *BotRunner) StartPolling(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			slog.Info("🛑 Polling durduruluyor...")
			return nil
		case update, ok := <-updates:
			if !ok {
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
}

func extractAudioTarget(msg *tgbotapi.Message) *AudioTarget {
	if msg.Voice != nil {
		return &AudioTarget{
			FileID:   msg.Voice.FileID,
			FileSize: msg.Voice.FileSize,
			Duration: msg.Voice.Duration,
			Name:     "Ses Kaydı",
		}
	}
	if msg.Audio != nil {
		return &AudioTarget{
			FileID:   msg.Audio.FileID,
			FileSize: msg.Audio.FileSize,
			Duration: msg.Audio.Duration,
			Name:     msg.Audio.FileName,
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
		b.api.Send(reply)
		return
	}

	audioTarget := extractAudioTarget(msg)
	if audioTarget != nil {
		// Check 20 MB Telegram bot limit
		if audioTarget.FileSize > 20*1024*1024 {
			reply := tgbotapi.NewMessage(msg.Chat.ID, "⚠️ <b>Hata:</b> Dosya boyutu çok büyük (Maksimum Telegram bot limiti: 20 MB).")
			reply.ParseMode = tgbotapi.ModeHTML
			reply.ReplyToMessageID = msg.MessageID
			b.api.Send(reply)
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

		b.api.Send(reply)
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
		b.api.Send(editMsg)
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
		b.api.Send(editMsg)
		return
	}

	go b.processVoice(ctx, cb.Message.Chat.ID, audioTarget.FileID, targetMode, cb.Message.MessageID, forceProvider)
}

func (b *BotRunner) sendTypingAction(ctx context.Context, chatID int64) func() {
	stopChan := make(chan struct{})
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		b.api.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
		for {
			select {
			case <-ticker.C:
				b.api.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() {
		close(stopChan)
	}
}

func (b *BotRunner) processVoice(ctx context.Context, chatID int64, fileID string, modeID string, statusMsgID int, forceProvider string) {
	stopTyping := b.sendTypingAction(ctx, chatID)
	defer stopTyping()

	modeInfo, ok := mode.Modes[modeID]
	if !ok {
		modeInfo = mode.Modes["tldr"]
	}

	b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 <b>%s</b> hazırlanıyor...", modeInfo.Label)))

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

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("Ses dosyası indirilemedi: %v", err))
		return
	}
	defer resp.Body.Close()

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("Ses dosyası okunamadı: %v", err))
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
		b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 <b>%s</b> hazırlanıyor... (Google Free Tier)", modeInfo.Label)))

		res, gErr := b.googleProvider.Generate(ctx, systemPrompt, base64Audio)
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
		b.api.Send(promptMsg)
		return
	}

	// 2. OpenRouter Provider Try
	b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 <b>%s</b> hazırlanıyor... (OpenRouter)", modeInfo.Label)))

	res, err := b.openRouterProvider.Generate(ctx, systemPrompt, base64Audio)
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
		b.api.Send(editMsg)
	}

	for _, c := range chunks[1:] {
		msg := tgbotapi.NewMessage(chatID, c)
		msg.ParseMode = tgbotapi.ModeHTML
		_, err := b.api.Send(msg)
		if err != nil {
			msg.ParseMode = ""
			b.api.Send(msg)
		}
	}

	costMsgText := fmt.Sprintf("📊 <b>Kullanım Özeti:</b>\n└ Servis: %s", costDetail)
	costMsg := tgbotapi.NewMessage(chatID, costMsgText)
	costMsg.ParseMode = tgbotapi.ModeHTML
	b.api.Send(costMsg)
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
	b.api.Send(editMsg)
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
