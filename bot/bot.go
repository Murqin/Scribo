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
	"scribo/history"
	"scribo/i18n"
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
	history            *history.Store
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

	slog.Info("🤖 Telegram bot authorised", "username", bot.Self.UserName, "maxConcurrentJobs", cfg.MaxConcurrentJobs, "language", i18n.Language())

	if len(cfg.AllowedUserIDs) == 0 {
		if cfg.AllowAllUsers {
			slog.Warn("⚠️ the bot is open to everyone (ALLOW_ALL_USERS=true); strangers can spend your API quota")
		} else {
			slog.Warn("⚠️ ALLOWED_USER_ID is unset — the bot will answer nobody. " +
				"Get your Telegram ID from @userinfobot and put it in .env.")
		}
	}

	store, err := history.Open(cfg.HistoryFile)
	if err != nil {
		return nil, fmt.Errorf("could not open the history file (%s): %w", cfg.HistoryFile, err)
	}
	if store == nil {
		slog.Warn("⚠️ HISTORY_FILE is empty — outputs will not be stored and the spending counter resets on every restart")
	}

	tracker := budget.New(cfg.DailyCostLimit, cfg.MonthlyCostLimit)
	if err := seedBudget(tracker, store, cfg); err != nil {
		return nil, err
	}

	return &BotRunner{
		cfg:                cfg,
		api:                bot,
		googleProvider:     provider.NewGoogleProvider(cfg.GeminiAPIKey, cfg.GoogleModel),
		openRouterProvider: provider.NewOpenRouterProvider(cfg.OpenRouterAPIKey, cfg.OpenRouterModel),
		budget:             tracker,
		history:            store,
		httpClient:         &http.Client{Timeout: 60 * time.Second},
		activeLocks:        make(map[string]bool),
		workerSem:          make(chan struct{}, cfg.MaxConcurrentJobs),
	}, nil
}

// seedBudget restores today's and this month's spend from the history file.
//
// An unreadable history is fatal only when a ceiling is configured: starting
// from zero would silently hand out a second full allowance, which is exactly
// the failure the ceiling exists to prevent. Without a ceiling the same error is
// a warning, because nothing depends on the number.
func seedBudget(tracker *budget.Tracker, store *history.Store, cfg *config.Config) error {
	daily, monthly, err := store.Spend(time.Now())
	if err != nil {
		if cfg.DailyCostLimit > 0 || cfg.MonthlyCostLimit > 0 {
			return fmt.Errorf("a spending ceiling is configured but the history could not be read (%s): %w", store.Path(), err)
		}
		slog.Warn("⚠️ history unreadable, the spending counter starts from zero", "error", err)
		return nil
	}

	tracker.Seed(daily, monthly)
	if daily > 0 || monthly > 0 {
		slog.Info("💰 spending counter restored from history", "daily", daily, "monthly", monthly)
	}
	return nil
}

func (b *BotRunner) StartPolling(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			slog.Info("🛑 stopping polling, waiting for active jobs to finish...")
			b.wg.Wait()
			slog.Info("✅ all active jobs finished")
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
			Name:     i18n.T("media.voice"),
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
			name = i18n.T("media.video")
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
			Name:     i18n.T("media.video_note"),
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

	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			reply := tgbotapi.NewMessage(msg.Chat.ID, i18n.T("bot.start"))
			reply.ParseMode = tgbotapi.ModeHTML
			b.sendMsg(reply)
			return
		case "son":
			b.sendLastEntry(msg.Chat.ID, msg.MessageID)
			return
		}
	}

	audioTarget := extractAudioTarget(msg)
	if audioTarget != nil {
		// Check 20 MB Telegram bot limit
		if audioTarget.FileSize > 20*1024*1024 {
			reply := tgbotapi.NewMessage(msg.Chat.ID, i18n.T("bot.file_too_large"))
			reply.ParseMode = tgbotapi.ModeHTML
			reply.ReplyToMessageID = msg.MessageID
			b.sendMsg(reply)
			return
		}

		warningText := ""
		if audioTarget.Duration > 90 {
			warningText = i18n.T("bot.long_media_warning", audioTarget.Duration)
		}

		replyText := i18n.T("bot.media_received", html.EscapeString(audioTarget.Name), warningText)
		reply := tgbotapi.NewMessage(msg.Chat.ID, replyText)
		reply.ParseMode = tgbotapi.ModeHTML
		reply.ReplyToMessageID = msg.MessageID
		reply.ReplyMarkup = mode.GetModeKeyboard()

		b.sendMsg(reply)
		return
	}

	// Guidance message for unsupported inputs
	if !msg.IsCommand() {
		reply := tgbotapi.NewMessage(msg.Chat.ID, i18n.T("bot.unsupported_input"))
		reply.ReplyToMessageID = msg.MessageID
		b.sendMsg(reply)
	}
}

func (b *BotRunner) handleCallbackQuery(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if !b.isAuthorized(cb.From.ID) {
		callbackCfg := tgbotapi.NewCallback(cb.ID, i18n.T("bot.unauthorized"))
		callbackCfg.ShowAlert = true
		b.api.Request(callbackCfg)
		return
	}

	b.api.Request(tgbotapi.NewCallback(cb.ID, ""))

	data := cb.Data
	if data == "cancel_paid" {
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, i18n.T("bot.cancelled"))
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
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, i18n.T("bot.source_not_found"))
		b.sendMsg(editMsg)
		return
	}

	lockKey := fmt.Sprintf("%d:%d", cb.Message.Chat.ID, cb.Message.MessageID)
	if !b.tryLock(lockKey) {
		slog.Warn("double tap or a job already running on this message was blocked", "key", lockKey)
		return
	}

	select {
	case b.workerSem <- struct{}{}:
	case <-ctx.Done():
		b.unlock(lockKey)
		slog.Warn("context cancelled, job dropped", "key", lockKey)
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

	msg := tgbotapi.NewEditMessageText(chatID, statusMsgID, i18n.T("bot.preparing", modeInfo.Label))
	msg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(msg)

	fileURL, err := b.api.GetFileDirectURL(fileID)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, i18n.T("err.file_url", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, i18n.T("err.request_create", err))
		return
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, i18n.T("err.download", err))
		return
	}
	defer resp.Body.Close()

	// Safe limited read to prevent OOM
	limitReader := io.LimitReader(resp.Body, 20*1024*1024+1024)
	audioBytes, err := io.ReadAll(limitReader)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, i18n.T("err.read", err))
		return
	}

	if len(audioBytes) > 20*1024*1024 {
		b.sendError(chatID, statusMsgID, modeID, i18n.T("err.too_large_downloaded"))
		return
	}

	base64Audio := base64.StdEncoding.EncodeToString(audioBytes)
	currentTimeStr := time.Now().Format(i18n.T("prompt.date_format"))
	systemPrompt := i18n.T("prompt.date_note", modeInfo.Prompt, currentTimeStr)

	selectedProvider := forceProvider
	if selectedProvider == "" {
		selectedProvider = b.cfg.DefaultProvider
	}

	// 1. Google Provider Try
	if selectedProvider != "openrouter" && b.cfg.GeminiAPIKey != "" {
		gMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, i18n.T("bot.preparing_google", modeInfo.Label))
		gMsg.ParseMode = tgbotapi.ModeHTML
		b.sendMsg(gMsg)

		res, gErr := b.googleProvider.Generate(ctx, systemPrompt, base64Audio, mimeType)
		if gErr == nil {
			detail := i18n.T("cost.google")
			if res.PromptTokens > 0 || res.CompletionTokens > 0 {
				detail = i18n.T("cost.google_tokens",
					res.PromptTokens+res.CompletionTokens, res.PromptTokens, res.CompletionTokens)
			}
			b.record(chatID, modeInfo, "google", mimeType, res)
			b.sendSuccessResponse(chatID, statusMsgID, res.Text, detail, modeInfo.Format)
			return
		}

		safeErr := b.cfg.Redact(gErr.Error())

		// OpenRouter carries media in an input_audio content part, so there is
		// no paid fallback worth offering for video: the call could only fail.
		if isVideoMimeType(mimeType) {
			slog.Warn("Google failed; no OpenRouter fallback is offered for video", "error", safeErr)
			b.sendError(chatID, statusMsgID, modeID,
				i18n.T("err.google_failed", safeErr, i18n.T("err.video_google_only")))
			return
		}

		// Offering the paid button when the ceiling is already reached would
		// only lead to a refusal one tap later.
		if limitErr := b.budget.Check(); limitErr != nil {
			slog.Warn("Google failed; the paid fallback was not offered because the spending ceiling is reached",
				"error", safeErr, "limit", limitErr)
			b.sendError(chatID, statusMsgID, modeID,
				i18n.T("err.google_failed", safeErr, budgetRefusalText(limitErr)))
			return
		}

		slog.Warn("Google failed, asking for OpenRouter confirmation", "error", safeErr)
		errShort := html.EscapeString(safeErr)
		if len(errShort) > 200 {
			errShort = errShort[:200] + "..."
		}

		promptText := i18n.T("bot.paid_prompt", errShort, html.EscapeString(b.cfg.OpenRouterModel))

		confirmKeyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(i18n.T("bot.btn_paid"), "paid:"+modeID),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(i18n.T("bot.btn_cancel"), "cancel_paid"),
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
		b.sendError(chatID, statusMsgID, modeID, i18n.T("err.video_no_fallback"))
		return
	}

	// The last gate before money is spent. Every paid path reaches this point,
	// including the "paid:" callback the user tapped on the fallback prompt.
	if limitErr := b.budget.Check(); limitErr != nil {
		slog.Warn("paid call refused by the spending ceiling", "limit", limitErr)
		b.sendError(chatID, statusMsgID, modeID, budgetRefusalText(limitErr))
		return
	}

	orMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, i18n.T("bot.preparing_openrouter", modeInfo.Label))
	orMsg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(orMsg)

	res, err := b.openRouterProvider.Generate(ctx, systemPrompt, base64Audio, mimeType)
	if err != nil {
		b.sendError(chatID, statusMsgID, modeID, i18n.T("err.openrouter", err))
		return
	}

	b.budget.Record(res.TotalCost)
	b.record(chatID, modeInfo, "openrouter", mimeType, res)

	costLine, budgetLine := "└", ""
	if line := budgetSummaryLine(b.budget.Snapshot()); line != "" {
		costLine, budgetLine = "├", "\n"+line
	}
	costInfo := i18n.T("cost.openrouter",
		res.PromptTokens+res.CompletionTokens, res.PromptTokens, res.CompletionTokens,
		costLine, fmt.Sprintf("%.5f", res.TotalCost), budgetLine)

	b.sendSuccessResponse(chatID, statusMsgID, res.Text, costInfo, modeInfo.Format)
}

// record archives a finished run. A failed write is logged and swallowed: the
// user is about to receive the transcript either way, and an unwritable archive
// is no reason to withhold work that has already been paid for.
//
// The cost recorded here is what seeds the spending ceiling on the next start,
// so the warning says plainly what a lost line costs.
func (b *BotRunner) record(chatID int64, modeInfo mode.ModeInfo, providerName, mimeType string, res *provider.AIResult) {
	err := b.history.Append(history.Entry{
		At:               time.Now(),
		ChatID:           chatID,
		Mode:             modeInfo.ID,
		Format:           string(modeInfo.Format),
		Provider:         providerName,
		MimeType:         mimeType,
		Text:             res.Text,
		PromptTokens:     res.PromptTokens,
		CompletionTokens: res.CompletionTokens,
		Cost:             res.TotalCost,
	})
	if err != nil {
		slog.Error("could not write to history; /son will not return this output and its cost is forgotten on restart",
			"error", err, "cost", res.TotalCost)
	}
}

// sendLastEntry answers /son with the most recent output of this chat, rendered
// the way its mode would have rendered it live.
func (b *BotRunner) sendLastEntry(chatID int64, replyTo int) {
	reply := func(text string) {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ReplyToMessageID = replyTo
		b.sendMsg(msg)
	}

	if b.history == nil {
		reply(i18n.T("history.disabled"))
		return
	}

	entry, found, err := b.history.Last(chatID)
	if err != nil {
		slog.Error("could not read the history", "error", err)
		reply(i18n.T("history.read_error"))
		return
	}
	if !found {
		reply(i18n.T("history.empty"))
		return
	}

	label := entry.Mode
	if m, ok := mode.GetMode(entry.Mode); ok && m.Label != "" {
		label = m.Label
	}

	header := tgbotapi.NewMessage(chatID, i18n.T("history.last_header",
		html.EscapeString(label), html.EscapeString(entry.Provider),
		entry.At.Local().Format(i18n.T("history.time_format"))))
	header.ParseMode = tgbotapi.ModeHTML
	header.ReplyToMessageID = replyTo
	b.sendMsg(header)

	// An unknown stored format is not worth rejecting the entry over: renderChunk
	// falls back to <code>, which is what the mode would have used anyway.
	for _, chunk := range splitForFormat(entry.Text, mode.Format(entry.Format)) {
		b.sendChunk(chatID, chunk, mode.Format(entry.Format))
	}
}

// budgetRefusalText explains a refused paid call and names the setting behind
// it, so a ceiling never reads like an outage. It returns plain text: sendError
// escapes and wraps the message itself.
func budgetRefusalText(err error) string {
	var limitErr *budget.LimitError
	if !errors.As(err, &limitErr) {
		return i18n.T("budget.refused_generic")
	}

	window, envVar := i18n.T("budget.window_daily"), "DAILY_COST_LIMIT"
	if limitErr.Window == budget.WindowMonthly {
		window, envVar = i18n.T("budget.window_monthly"), "MONTHLY_COST_LIMIT"
	}

	return i18n.T("budget.refused", window, limitErr.Spent, limitErr.Limit, envVar)
}

// budgetSummaryLine reports remaining budget in the usage summary. It is empty
// when no ceiling is configured, so the default setup gains no extra noise.
func budgetSummaryLine(s budget.Status) string {
	if !s.Enabled() {
		return ""
	}

	var parts []string
	if s.DailyLimit > 0 {
		parts = append(parts, i18n.T("budget.summary_daily", s.DailySpent, s.DailyLimit))
	}
	if s.MonthlyLimit > 0 {
		parts = append(parts, i18n.T("budget.summary_monthly", s.MonthlySpent, s.MonthlyLimit))
	}

	prefix := i18n.T("budget.summary_prefix")
	if s.NearLimit() {
		prefix = i18n.T("budget.summary_prefix_warning")
	}
	return prefix + ": " + strings.Join(parts, " · ")
}

func (b *BotRunner) sendSuccessResponse(chatID int64, statusMsgID int, cleanText string, costDetail string, format mode.Format) {
	chunks := splitForFormat(cleanText, format)
	if len(chunks) == 0 {
		chunks = []string{i18n.T("bot.empty_result")}
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
		b.sendChunk(chatID, c, format)
	}

	costMsgText := i18n.T("bot.usage_summary", costDetail)
	costMsg := tgbotapi.NewMessage(chatID, costMsgText)
	costMsg.ParseMode = tgbotapi.ModeHTML
	b.sendMsg(costMsg)
}

// sendChunk sends one rendered chunk as a new message, retrying without markup
// if Telegram rejects the rendering. Losing the formatting beats losing the text.
func (b *BotRunner) sendChunk(chatID int64, chunk string, format mode.Format) {
	text, parseMode := renderChunk(chunk, format)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = parseMode
	if _, err := b.api.Send(msg); err != nil && parseMode != "" {
		msg.ParseMode = ""
		msg.Text = chunk
		b.sendMsg(msg)
	}
}

func (b *BotRunner) sendError(chatID int64, statusMsgID int, modeID string, errText string) {
	errText = b.cfg.Redact(errText)
	slog.Error("processing error", "chatID", chatID, "error", errText)
	txt := i18n.T("bot.error", html.EscapeString(errText))
	retryKb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T("bot.btn_retry"), modeID),
		),
	)

	editMsg := tgbotapi.NewEditMessageText(chatID, statusMsgID, txt)
	editMsg.ParseMode = tgbotapi.ModeHTML
	editMsg.ReplyMarkup = &retryKb
	b.sendMsg(editMsg)
}

func (b *BotRunner) sendMsg(chg tgbotapi.Chattable) {
	if _, err := b.api.Send(chg); err != nil {
		slog.Error("sending the Telegram message failed", "error", err)
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
