package bot

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"scribo/config"
	"scribo/mode"
	"scribo/provider"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotRunner struct {
	cfg *config.Config
	api *tgbotapi.BotAPI
}

func NewBotRunner(cfg *config.Config) (*BotRunner, error) {
	if cfg.TelegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_TOKEN tanımlı değil")
	}

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, err
	}

	log.Printf("🤖 Telegram Bot yetkilendirildi: @%s", bot.Self.UserName)
	return &BotRunner{cfg: cfg, api: bot}, nil
}

func (b *BotRunner) StartPolling() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			b.handleCallbackQuery(update.CallbackQuery)
		}
	}
	return nil
}

func (b *BotRunner) isAuthorized(userID int64) bool {
	if b.cfg.AllowedUserID == "" {
		return true
	}
	return fmt.Sprintf("%d", userID) == b.cfg.AllowedUserID
}

func (b *BotRunner) handleMessage(msg *tgbotapi.Message) {
	if !b.isAuthorized(msg.From.ID) {
		return
	}

	if msg.IsCommand() && msg.Command() == "start" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "🎙️ Scribo Bot hazır! Bir ses kaydı gönderin.")
		b.api.Send(reply)
		return
	}

	if msg.Voice != nil {
		warningText := ""
		if msg.Voice.Duration > 90 {
			warningText = fmt.Sprintf("\n\n⚠️ <b>Uyarı:</b> Ses kaydı uzun (%d sn).", msg.Voice.Duration)
		}

		replyText := fmt.Sprintf("🚀 Ses kaydı alındı! Seçiniz:%s", warningText)
		reply := tgbotapi.NewMessage(msg.Chat.ID, replyText)
		reply.ParseMode = tgbotapi.ModeHTML
		reply.ReplyToMessageID = msg.MessageID
		reply.ReplyMarkup = mode.GetModeKeyboard()

		b.api.Send(reply)
	}
}

func (b *BotRunner) handleCallbackQuery(cb *tgbotapi.CallbackQuery) {
	if !b.isAuthorized(cb.From.ID) {
		callbackCfg := tgbotapi.NewCallback(cb.ID, "Yetkisiz kullanıcı.")
		callbackCfg.ShowAlert = true
		b.api.Request(callbackCfg)
		return
	}

	callbackCfg := tgbotapi.NewCallback(cb.ID, "")
	b.api.Request(callbackCfg)

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

	var voiceMsg *tgbotapi.Message
	if cb.Message.ReplyToMessage != nil && cb.Message.ReplyToMessage.Voice != nil {
		voiceMsg = cb.Message.ReplyToMessage
	}

	if voiceMsg == nil {
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "❌ Kaynak ses dosyası bulunamadı.")
		b.api.Send(editMsg)
		return
	}

	go b.processVoice(cb.Message.Chat.ID, voiceMsg.Voice.FileID, targetMode, cb.Message.MessageID, forceProvider)
}

func (b *BotRunner) processVoice(chatID int64, fileID string, modeID string, statusMsgID int, forceProvider string) {
	modeInfo, ok := mode.Modes[modeID]
	if !ok {
		modeInfo = mode.Modes["tldr"]
	}

	editMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 %s hazırlanıyor...", modeInfo.Label))
	b.api.Send(editMsg)

	fileURL, err := b.api.GetFileDirectURL(fileID)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("Ses dosyası alınamadı: %v", err))
		return
	}

	resp, err := http.Get(fileURL)
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
		b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 %s hazırlanıyor... (Google Free Tier)", modeInfo.Label)))

		resText, gErr := provider.CallGoogleAPI(b.cfg.GeminiAPIKey, b.cfg.GoogleModel, systemPrompt, base64Audio)
		if gErr == nil {
			b.sendSuccessResponse(chatID, statusMsgID, resText, "<b>Google Free Tier</b> (<code>$0.00000</code>)")
			return
		}

		log.Printf("Google API başarısız: %v. OpenRouter onayı soruluyor.", gErr)
		errShort := html.EscapeString(gErr.Error())
		if len(errShort) > 150 {
			errShort = errShort[:150]
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
	b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 %s hazırlanıyor... (OpenRouter)", modeInfo.Label)))

	res, err := provider.CallOpenRouterAPI(b.cfg.OpenRouterAPIKey, b.cfg.OpenRouterModel, systemPrompt, base64Audio)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("OpenRouter Hatası: %v", err))
		return
	}

	costInfo := fmt.Sprintf("<b>OpenRouter</b>\n├ Token: %d (P: %d, C: %d)\n└ Maliyet: <code>$%s</code>",
		res.PromptTokens+res.CompletionTokens, res.PromptTokens, res.CompletionTokens, fmt.Sprintf("%.5f", res.TotalCost))

	b.sendSuccessResponse(chatID, statusMsgID, res.Text, costInfo)
}

func (b *BotRunner) sendSuccessResponse(chatID int64, statusMsgID int, cleanText string, costDetail string) {
	chunks := splitMessage(cleanText, 4000)
	if len(chunks) == 0 {
		chunks = []string{"İşlem tamamlandı."}
	}

	copyableText := fmt.Sprintf("<code>%s</code>", html.EscapeString(chunks[0]))
	kb := mode.GetModeKeyboard()

	editMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, copyableText)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &kb
	b.api.Send(editMsg)

	for _, c := range chunks[1:] {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("<code>%s</code>", html.EscapeString(c)))
		msg.ParseMode = tgbotapi.ModeHTML
		b.api.Send(msg)
	}

	costMsgText := fmt.Sprintf("📊 <b>Kullanım Özeti:</b>\n└ Servis: %s", costDetail)
	costMsg := tgbotapi.NewMessage(chatID, costMsgText)
	costMsg.ParseMode = tgbotapi.ModeHTML
	b.api.Send(costMsg)
}

func (b *BotRunner) sendError(chatID int64, statusMsgID int, modeID string, errText string) {
	if len(errText) > 50 {
		errText = errText[:50]
	}
	txt := fmt.Sprintf("❌ Hata: %s", errText)
	retryKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Tekrar Dene", modeID),
		),
	)

	editMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, txt)
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
