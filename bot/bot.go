package bot

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"scribo/budget"
	"scribo/config"
	"scribo/mode"
	"scribo/provider"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramClient interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	GetFileDirectURL(fileID string) (string, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
	GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel
	StopReceivingUpdates()
}

type BotRunner struct {
	cfg                *config.Config
	api                TelegramClient
	googleProvider     provider.AIProvider
	openRouterProvider provider.AIProvider
	budget             *budget.Tracker
	httpClient         *http.Client
	locksMu            sync.Mutex
	activeLocks        map[string]bool
	workerSem          chan struct{}
	wg                 sync.WaitGroup
}

func (b *BotRunner) tryLock(key string) bool {
	b.locksMu.Lock()
	defer b.locksMu.Unlock()
	if b.activeLocks == nil {
		b.activeLocks = make(map[string]bool)
	}
	if b.activeLocks[key] {
		return false
	}
	b.activeLocks[key] = true
	return true
}

func (b *BotRunner) unlock(key string) {
	b.locksMu.Lock()
	defer b.locksMu.Unlock()
	if b.activeLocks != nil {
		delete(b.activeLocks, key)
	}
}

func (b *BotRunner) SetTelegramClient(client TelegramClient) {
	b.api = client
}

func (b *BotRunner) SetProviders(googleProv, openRouterProv provider.AIProvider) {
	if googleProv != nil {
		b.googleProvider = googleProv
	}
	if openRouterProv != nil {
		b.openRouterProvider = openRouterProv
	}
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

	if len(cfg.AllowedUserIDs) == 0 {
		if cfg.AllowAllUsers {
			slog.Warn("⚠️ Bot herkese açık (ALLOW_ALL_USERS=true). API kotanız yabancılar tarafından harcanabilir.")
		} else {
			slog.Warn("⚠️ ALLOWED_USER_ID tanımlı değil — bot hiçbir kullanıcıya yanıt vermeyecek. " +
				"Kendi Telegram ID'nizi @userinfobot'tan alıp .env dosyasına yazın.")
		}
	}

	return &BotRunner{
		cfg:                cfg,
		api:                bot,
		googleProvider:     provider.NewGoogleProvider(cfg.GeminiAPIKey, cfg.GoogleModel),
		openRouterProvider: provider.NewOpenRouterProvider(cfg.OpenRouterAPIKey, cfg.OpenRouterModel),
		budget:             budget.New(cfg.DailyCostLimit, cfg.MonthlyCostLimit),
		httpClient:         &http.Client{Timeout: 60 * time.Second},
		activeLocks:        make(map[string]bool),
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
	if len(b.cfg.AllowedUserIDs) == 0 {
		return b.cfg.AllowAllUsers
	}
	for _, id := range b.cfg.AllowedUserIDs {
		if id == userID {
			return true
		}
	}
	return false
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

// videoMimeTypeFromExt maps a file extension to one of the video MIME types
// Gemini accepts. Anything unrecognised falls back to video/mp4, which is what
// Telegram produces for every video it transcodes itself.
func videoMimeTypeFromExt(ext string) string {
	switch ext {
	case ".mpeg", ".mpg":
		return "video/mpeg"
	case ".mov":
		return "video/mov"
	case ".avi":
		return "video/avi"
	case ".flv":
		return "video/x-flv"
	case ".webm":
		return "video/webm"
	case ".wmv":
		return "video/wmv"
	case ".3gp", ".3gpp":
		return "video/3gpp"
	default:
		return "video/mp4"
	}
}

// videoMimeType prefers the MIME type declared by the sender, but only when it
// is one Gemini understands — senders are free to declare anything, and an
// unsupported value would be rejected by the API instead of falling back.
func videoMimeType(declared, fileName string) string {
	if declared != "" {
		switch strings.ToLower(declared) {
		case "video/mp4", "video/mpeg", "video/mov", "video/avi",
			"video/x-flv", "video/webm", "video/wmv", "video/3gpp":
			return strings.ToLower(declared)
		}
	}
	return videoMimeTypeFromExt(strings.ToLower(filepath.Ext(fileName)))
}

func isVideoMimeType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "video/")
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
	if msg.Video != nil {
		name := msg.Video.FileName
		if name == "" {
			name = "Video"
		}
		return &AudioTarget{
			FileID:   msg.Video.FileID,
			FileSize: msg.Video.FileSize,
			Duration: msg.Video.Duration,
			Name:     name,
			MimeType: videoMimeType(msg.Video.MimeType, msg.Video.FileName),
		}
	}
	if msg.VideoNote != nil {
		// Round video messages carry neither a file name nor a MIME type;
		// Telegram always encodes them as MP4.
		return &AudioTarget{
			FileID:   msg.VideoNote.FileID,
			FileSize: msg.VideoNote.FileSize,
			Duration: msg.VideoNote.Duration,
			Name:     "Video Mesajı",
			MimeType: "video/mp4",
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
		case ".mp4", ".mpeg", ".mpg", ".mov", ".avi", ".flv", ".webm", ".wmv", ".3gp", ".3gpp":
			return &AudioTarget{
				FileID:   msg.Document.FileID,
				FileSize: msg.Document.FileSize,
				Duration: 0,
				Name:     msg.Document.FileName,
				MimeType: videoMimeType(msg.Document.MimeType, msg.Document.FileName),
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
		reply := tgbotapi.NewMessage(msg.Chat.ID, "🎙️ <b>Scribo Bot Hazır!</b>\nBir ses kaydı, video, video mesajı veya ses dosyası gönderin.")
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
		reply := tgbotapi.NewMessage(msg.Chat.ID, "🎙️ Lütfen analiz edilmek üzere bir ses kaydı (Voice note), ses dosyası (MP3, M4A, WAV, FLAC, OGG), video veya video mesajı (MP4, MOV, WEBM, AVI) gönderin.")
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
	if !b.tryLock(lockKey) {
		slog.Warn("Çift tıklama veya aynı mesaj üzerinde devam eden işlem engellendi", "key", lockKey)
		return
	}

	select {
	case b.workerSem <- struct{}{}:
	case <-ctx.Done():
		b.unlock(lockKey)
		slog.Warn("Bağlam sonlandırıldı, işlem iptal edildi", "key", lockKey)
		return
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer func() { <-b.workerSem }()
		defer b.unlock(lockKey)

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
			detail := "<b>Google Free Tier</b> (<code>$0.00000</code>)"
			if res.PromptTokens > 0 || res.CompletionTokens > 0 {
				detail = fmt.Sprintf("<b>Google Free Tier</b> (<code>$0.00000</code>)\n├ Token: %d (P: %d, C: %d)",
					res.PromptTokens+res.CompletionTokens, res.PromptTokens, res.CompletionTokens)
			}
			b.sendSuccessResponse(chatID, statusMsgID, res.Text, detail, modeInfo.Format)
			return
		}

		safeErr := b.cfg.Redact(gErr.Error())

		// OpenRouter carries media in an input_audio content part, so there is
		// no paid fallback worth offering for video: the call could only fail.
		if isVideoMimeType(mimeType) {
			slog.Warn("Google API başarısız, video için OpenRouter devri atlandı", "error", safeErr)
			b.sendError(chatID, statusMsgID, modeID,
				fmt.Sprintf("Google ile işlenemedi: %s\n\nVideo yalnızca Google üzerinden işlenebiliyor.", gErr))
			return
		}

		// Offering the paid button when the ceiling is already reached would
		// only lead to a refusal one tap later.
		if limitErr := b.budget.Check(); limitErr != nil {
			slog.Warn("Google API başarısız, harcama tavanı dolu olduğu için OpenRouter devri sunulmadı",
				"error", safeErr, "limit", limitErr)
			b.sendError(chatID, statusMsgID, modeID,
				fmt.Sprintf("Google ile işlenemedi: %s\n\n%s", safeErr, budgetRefusalText(limitErr)))
			return
		}

		slog.Warn("Google API başarısız, OpenRouter onayı soruluyor", "error", safeErr)
		errShort := html.EscapeString(safeErr)
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
	if isVideoMimeType(mimeType) {
		b.sendError(chatID, statusMsgID, modeID,
			"Video yalnızca Google üzerinden işlenebiliyor; OpenRouter ses dışı içerik kabul etmiyor.")
		return
	}

	// The last gate before money is spent. Every paid path reaches this point,
	// including the "paid:" callback the user tapped on the fallback prompt.
	if limitErr := b.budget.Check(); limitErr != nil {
		slog.Warn("Harcama tavanı nedeniyle ücretli çağrı reddedildi", "limit", limitErr)
		b.sendError(chatID, statusMsgID, modeID, budgetRefusalText(limitErr))
		return
	}

	orMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, fmt.Sprintf("🔄 <b>%s</b> hazırlanıyor... (OpenRouter)", modeInfo.Label))
	orMsg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(orMsg)

	res, err := b.openRouterProvider.Generate(ctx, systemPrompt, base64Audio, mimeType)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, fmt.Sprintf("OpenRouter Hatası: %v", err))
		return
	}

	b.budget.Record(res.TotalCost)

	costLine, budgetLine := "└", ""
	if line := budgetSummaryLine(b.budget.Snapshot()); line != "" {
		costLine, budgetLine = "├", "\n"+line
	}
	costInfo := fmt.Sprintf("<b>OpenRouter</b>\n├ Token: %d (P: %d, C: %d)\n%s Maliyet: <code>$%s</code>%s",
		res.PromptTokens+res.CompletionTokens, res.PromptTokens, res.CompletionTokens,
		costLine, fmt.Sprintf("%.5f", res.TotalCost), budgetLine)

	b.sendSuccessResponse(chatID, statusMsgID, res.Text, costInfo, modeInfo.Format)
}

// budgetRefusalText explains a refused paid call and names the setting behind
// it, so a ceiling never reads like an outage. It returns plain text: sendError
// escapes and wraps the message itself.
func budgetRefusalText(err error) string {
	var limitErr *budget.LimitError
	if !errors.As(err, &limitErr) {
		return "💸 Harcama tavanı denetimi nedeniyle ücretli çağrı yapılmadı."
	}

	window, envVar := "Günlük", "DAILY_COST_LIMIT"
	if limitErr.Window == budget.WindowMonthly {
		window, envVar = "Aylık", "MONTHLY_COST_LIMIT"
	}

	return fmt.Sprintf(
		"💸 %s harcama tavanına ulaşıldı ($%.5f / $%.5f), ücretli OpenRouter çağrısı yapılmadı.\n"+
			"Tavanı .env dosyasındaki %s ile değiştirebilirsiniz. Sayaç süreç içinde tutulur, bot yeniden başlatılırsa sıfırlanır.",
		window, limitErr.Spent, limitErr.Limit, envVar)
}

// budgetSummaryLine reports remaining budget in the usage summary. It is empty
// when no ceiling is configured, so the default setup gains no extra noise.
func budgetSummaryLine(s budget.Status) string {
	if !s.Enabled() {
		return ""
	}

	var parts []string
	if s.DailyLimit > 0 {
		parts = append(parts, fmt.Sprintf("günlük $%.5f/$%.5f", s.DailySpent, s.DailyLimit))
	}
	if s.MonthlyLimit > 0 {
		parts = append(parts, fmt.Sprintf("aylık $%.5f/$%.5f", s.MonthlySpent, s.MonthlyLimit))
	}

	prefix := "└ Bütçe"
	if s.NearLimit() {
		prefix = "└ ⚠️ Bütçe"
	}
	return prefix + ": " + strings.Join(parts, " · ")
}

func (b *BotRunner) sendSuccessResponse(chatID int64, statusMsgID int, cleanText string, costDetail string, format mode.Format) {
	chunks := splitForFormat(cleanText, format)
	if len(chunks) == 0 {
		chunks = []string{"İşlem tamamlandı."}
	}

	firstChunkText, firstParseMode := renderChunk(chunks[0], format)
	kb := mode.GetModeKeyboard()

	editMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, firstChunkText)
	editMsg.ParseMode = firstParseMode
	editMsg.ReplyMarkup = &kb
	_, err := b.api.Send(editMsg)
	if err != nil && firstParseMode != "" {
		// Fallback to plain text if HTML parsing fails
		editMsg.ParseMode = ""
		editMsg.Text = chunks[0]
		b.sendMsg(editMsg)
	}

	for _, c := range chunks[1:] {
		chunkText, parseMode := renderChunk(c, format)
		msg := tgbotapi.NewMessage(chatID, chunkText)
		msg.ParseMode = parseMode
		_, err := b.api.Send(msg)
		if err != nil && parseMode != "" {
			msg.ParseMode = ""
			msg.Text = c
			b.sendMsg(msg)
		}
	}

	costMsgText := fmt.Sprintf("📊 <b>Kullanım Özeti:</b>\n└ Servis: %s", costDetail)
	costMsg := tgbotapi.NewMessage(chatID, costMsgText)
	costMsg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(costMsg)
}

func (b *BotRunner) sendError(chatID int64, statusMsgID int, modeID string, errText string) {
	errText = b.cfg.Redact(errText)
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

// utf16Len reports the length Telegram actually enforces. The 4096-character message
// limit counts UTF-16 code units, so emoji and other astral characters cost two.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// splitMessage breaks text into chunks that fit Telegram's limit. It iterates runes
// rather than bytes: slicing by byte offset used to cut multi-byte characters in half,
// producing invalid UTF-8 that Telegram rejects outright.
func splitMessage(text string, maxUnits int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var chunks []string
	for utf16Len(text) > maxUnits {
		cut, units, lastSpace, lastNewline := 0, 0, -1, -1
		for i, r := range text {
			w := 1
			if r > 0xFFFF {
				w = 2
			}
			if units+w > maxUnits {
				break
			}
			units += w
			cut = i + utf8.RuneLen(r)
			if r == '\n' {
				lastNewline = cut
			} else if r == ' ' {
				lastSpace = cut
			}
		}

		// Prefer a line break, then a word break, then the hard rune boundary.
		split := cut
		if lastNewline > 0 {
			split = lastNewline
		} else if lastSpace > 0 {
			split = lastSpace
		}

		chunks = append(chunks, strings.TrimSpace(text[:split]))
		text = strings.TrimSpace(text[split:])
	}

	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}
