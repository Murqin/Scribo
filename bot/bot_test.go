package bot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"scribo/budget"
	"scribo/config"
	"scribo/mode"
	"scribo/provider"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMimeTypeFromExt(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".mp3", "audio/mp3"},
		{".m4a", "audio/m4a"},
		{".wav", "audio/wav"},
		{".aac", "audio/aac"},
		{".flac", "audio/flac"},
		{".opus", "audio/opus"},
		{".ogg", "audio/ogg"},
		{".unknown", "audio/ogg"},
	}

	for _, tt := range tests {
		got := mimeTypeFromExt(tt.ext)
		if got != tt.expected {
			t.Errorf("mimeTypeFromExt(%s) = %s; want %s", tt.ext, got, tt.expected)
		}
	}
}

func TestSplitMessage(t *testing.T) {
	// Empty string
	if chunks := splitMessage("", 100); chunks != nil {
		t.Errorf("expected nil for empty string, got %v", chunks)
	}

	// Short text
	shortText := "Short text"
	chunks := splitMessage(shortText, 100)
	if len(chunks) != 1 || chunks[0] != shortText {
		t.Errorf("expected 1 chunk matching short text, got %v", chunks)
	}

	// Long text split by newlines
	longText := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	chunks = splitMessage(longText, 15)
	if len(chunks) < 2 {
		t.Fatalf("expected long text to be split into multiple chunks, got %d chunks", len(chunks))
	}

	// Reassembled text check
	joined := strings.Join(chunks, "\n")
	if strings.ReplaceAll(joined, "\n\n", "\n") != longText {
		t.Errorf("reassembled text does not match original: got %q, want %q", joined, longText)
	}
}

func TestExtractAudioTarget_Voice(t *testing.T) {
	msg := &tgbotapi.Message{
		Voice: &tgbotapi.Voice{
			FileID:   "file_123",
			FileSize: 1024,
			Duration: 15,
		},
	}

	target := extractAudioTarget(msg)
	if target == nil {
		t.Fatal("expected non-nil AudioTarget")
	}
	if target.FileID != "file_123" || target.MimeType != "audio/ogg" {
		t.Errorf("unexpected target: %+v", target)
	}
}

func TestExtractAudioTarget_Audio(t *testing.T) {
	msg := &tgbotapi.Message{
		Audio: &tgbotapi.Audio{
			FileID:   "file_456",
			FileName: "sample.mp3",
			FileSize: 2048,
			Duration: 45,
		},
	}

	target := extractAudioTarget(msg)
	if target == nil {
		t.Fatal("expected non-nil AudioTarget")
	}
	if target.FileID != "file_456" || target.MimeType != "audio/mp3" {
		t.Errorf("unexpected target: %+v", target)
	}
}

func TestExtractAudioTarget_Document(t *testing.T) {
	// 1. Audio Document (.flac)
	msgAudioDoc := &tgbotapi.Message{
		Document: &tgbotapi.Document{
			FileID:   "doc_flac_789",
			FileName: "recording.flac",
			FileSize: 4096,
		},
	}

	target := extractAudioTarget(msgAudioDoc)
	if target == nil {
		t.Fatal("expected non-nil AudioTarget for audio document")
	}
	if target.FileID != "doc_flac_789" || target.MimeType != "audio/flac" {
		t.Errorf("unexpected target: %+v", target)
	}

	// 2. Non-audio Document (.pdf)
	msgPdfDoc := &tgbotapi.Message{
		Document: &tgbotapi.Document{
			FileID:   "doc_pdf_000",
			FileName: "document.pdf",
			FileSize: 8192,
		},
	}

	if extractAudioTarget(msgPdfDoc) != nil {
		t.Error("expected nil AudioTarget for non-audio document (.pdf)")
	}
}

func TestVideoMimeType(t *testing.T) {
	tests := []struct {
		name     string
		declared string
		fileName string
		expected string
	}{
		{"declared mp4 wins", "video/mp4", "clip.avi", "video/mp4"},
		{"declared uppercase is normalised", "VIDEO/WEBM", "clip.mp4", "video/webm"},
		{"unsupported declared type falls back to extension", "video/x-matroska", "clip.mov", "video/mov"},
		{"empty declared type uses extension", "", "clip.webm", "video/webm"},
		{"unknown extension defaults to mp4", "", "clip.mkv", "video/mp4"},
		{"missing name defaults to mp4", "", "", "video/mp4"},
		{"mpg maps to mpeg", "", "clip.mpg", "video/mpeg"},
		{"3gp maps to 3gpp", "", "clip.3gp", "video/3gpp"},
		{"flv keeps the x- prefix", "", "clip.flv", "video/x-flv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := videoMimeType(tt.declared, tt.fileName); got != tt.expected {
				t.Errorf("videoMimeType(%q, %q) = %s; want %s", tt.declared, tt.fileName, got, tt.expected)
			}
		})
	}
}

func TestExtractAudioTarget_Video(t *testing.T) {
	msg := &tgbotapi.Message{
		Video: &tgbotapi.Video{
			FileID:   "vid_123",
			FileName: "meeting.mp4",
			MimeType: "video/mp4",
			FileSize: 1_048_576,
			Duration: 90,
		},
	}

	target := extractAudioTarget(msg)
	if target == nil {
		t.Fatal("expected non-nil AudioTarget for video")
	}
	if target.FileID != "vid_123" || target.MimeType != "video/mp4" {
		t.Errorf("unexpected target: %+v", target)
	}
	if target.Name != "meeting.mp4" || target.Duration != 90 || target.FileSize != 1_048_576 {
		t.Errorf("unexpected target metadata: %+v", target)
	}
	if !isVideoMimeType(target.MimeType) {
		t.Errorf("expected %s to be recognised as video", target.MimeType)
	}
}

func TestExtractAudioTarget_VideoWithoutFileName(t *testing.T) {
	// Telegram omits the file name for videos it transcoded itself.
	msg := &tgbotapi.Message{
		Video: &tgbotapi.Video{
			FileID:   "vid_456",
			FileSize: 2048,
			Duration: 12,
		},
	}

	target := extractAudioTarget(msg)
	if target == nil {
		t.Fatal("expected non-nil AudioTarget for video without file name")
	}
	if target.Name != "Video" || target.MimeType != "video/mp4" {
		t.Errorf("unexpected target: %+v", target)
	}
}

func TestExtractAudioTarget_VideoNote(t *testing.T) {
	msg := &tgbotapi.Message{
		VideoNote: &tgbotapi.VideoNote{
			FileID:   "note_789",
			FileSize: 4096,
			Duration: 20,
		},
	}

	target := extractAudioTarget(msg)
	if target == nil {
		t.Fatal("expected non-nil AudioTarget for video note")
	}
	if target.FileID != "note_789" || target.MimeType != "video/mp4" {
		t.Errorf("unexpected target: %+v", target)
	}
	if target.Name != "Video Mesajı" || target.Duration != 20 {
		t.Errorf("unexpected target metadata: %+v", target)
	}
}

func TestExtractAudioTarget_VideoDocument(t *testing.T) {
	// A video sent "as file" arrives as a Document.
	msgVideoDoc := &tgbotapi.Message{
		Document: &tgbotapi.Document{
			FileID:   "doc_mov_111",
			FileName: "screen.mov",
			MimeType: "video/quicktime",
			FileSize: 8192,
		},
	}

	target := extractAudioTarget(msgVideoDoc)
	if target == nil {
		t.Fatal("expected non-nil AudioTarget for video document")
	}
	// video/quicktime is not in Gemini's list, so the extension decides.
	if target.MimeType != "video/mov" {
		t.Errorf("unexpected mime type: %s", target.MimeType)
	}

	msgMkvDoc := &tgbotapi.Message{
		Document: &tgbotapi.Document{
			FileID:   "doc_mkv_222",
			FileName: "movie.mkv",
			FileSize: 8192,
		},
	}

	if extractAudioTarget(msgMkvDoc) != nil {
		t.Error("expected nil AudioTarget for unsupported video container (.mkv)")
	}
}

func TestExtractAudioTarget_VoiceWinsOverVideo(t *testing.T) {
	// A single Telegram message never carries both, but the branch order is
	// what keeps audio on the cheaper audio path if it ever did.
	msg := &tgbotapi.Message{
		Voice: &tgbotapi.Voice{FileID: "voice_1", Duration: 5},
		Video: &tgbotapi.Video{FileID: "video_1", Duration: 5},
	}

	target := extractAudioTarget(msg)
	if target == nil || target.FileID != "voice_1" || target.MimeType != "audio/ogg" {
		t.Errorf("expected voice to take precedence, got %+v", target)
	}
}

func TestIsVideoMimeType(t *testing.T) {
	for _, mt := range []string{"video/mp4", "video/webm", "video/x-flv"} {
		if !isVideoMimeType(mt) {
			t.Errorf("expected %s to be video", mt)
		}
	}
	for _, mt := range []string{"audio/ogg", "audio/mp3", ""} {
		if isVideoMimeType(mt) {
			t.Errorf("expected %s not to be video", mt)
		}
	}
}

func TestIsAuthorized(t *testing.T) {
	// 1. Restricted User ID
	runnerRestricted := &BotRunner{
		cfg: &config.Config{
			AllowedUserIDs: []int64{123456},
		},
	}

	if !runnerRestricted.isAuthorized(123456) {
		t.Error("user 123456 should be authorized")
	}

	if runnerRestricted.isAuthorized(999999) {
		t.Error("user 999999 should not be authorized")
	}

	// 2. No allowed IDs configured: deny by default.
	runnerClosed := &BotRunner{cfg: &config.Config{}}
	if runnerClosed.isAuthorized(999999) {
		t.Error("user 999999 must be denied when no allowed IDs are configured")
	}

	// 3. Explicit opt-out: ALLOW_ALL_USERS=true opens the bot.
	runnerPublic := &BotRunner{cfg: &config.Config{AllowAllUsers: true}}
	if !runnerPublic.isAuthorized(999999) {
		t.Error("user 999999 should be authorized when AllowAllUsers is set")
	}

	// 4. Multiple IDs are all honoured.
	runnerMulti := &BotRunner{cfg: &config.Config{AllowedUserIDs: []int64{111, 222}}}
	if !runnerMulti.isAuthorized(111) || !runnerMulti.isAuthorized(222) {
		t.Error("both configured IDs should be authorized")
	}
	if runnerMulti.isAuthorized(333) {
		t.Error("unlisted ID 333 must be denied")
	}
}

type mockTelegramClient struct {
	// mu guards sentMessages: processVoice runs a typing-indicator goroutine
	// that keeps sending while the test reads what was sent.
	mu           sync.Mutex
	sentMessages []tgbotapi.Chattable
	fileURL      string
	fileURLErr   error
}

func (m *mockTelegramClient) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{}, nil
}

func (m *mockTelegramClient) sent() []tgbotapi.Chattable {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]tgbotapi.Chattable(nil), m.sentMessages...)
}

func (m *mockTelegramClient) GetFileDirectURL(fileID string) (string, error) {
	if m.fileURLErr != nil {
		return "", m.fileURLErr
	}
	return m.fileURL, nil
}

func (m *mockTelegramClient) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (m *mockTelegramClient) GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
	return nil
}

func (m *mockTelegramClient) StopReceivingUpdates() {}

func TestBotRunner_SetTelegramClient(t *testing.T) {
	runner := &BotRunner{
		cfg: &config.Config{},
	}
	mock := &mockTelegramClient{fileURL: "http://example.com/audio.ogg"}
	runner.SetTelegramClient(mock)

	if runner.api != mock {
		t.Error("expected runner.api to be set to mock")
	}
}

func TestBotRunner_HandleCallbackQuery_Unauthorized(t *testing.T) {
	mock := &mockTelegramClient{}
	runner := &BotRunner{
		cfg: &config.Config{
			AllowedUserIDs: []int64{100},
		},
		api: mock,
	}

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb_123",
		From: &tgbotapi.User{ID: 999}, // Unauthorized
	}

	runner.handleCallbackQuery(context.Background(), cb)

	if len(mock.sentMessages) != 0 {
		t.Errorf("expected 0 sent messages for unauthorized callback, got %d", len(mock.sentMessages))
	}
}

func TestBotRunner_SendSuccessResponse_TapToCopyCodeFormatting(t *testing.T) {
	mock := &mockTelegramClient{}
	runner := &BotRunner{
		cfg: &config.Config{},
		api: mock,
	}

	runner.sendSuccessResponse(12345, 99, "Hello <World>", "cost info", mode.FormatCode)

	if len(mock.sentMessages) == 0 {
		t.Fatal("expected sent messages, got 0")
	}

	firstEdit, ok := mock.sentMessages[0].(tgbotapi.EditMessageTextConfig)
	if !ok {
		t.Fatalf("expected first message to be EditMessageTextConfig, got %T", mock.sentMessages[0])
	}

	expectedText := "<code>Hello &lt;World&gt;</code>"
	if firstEdit.Text != expectedText {
		t.Errorf("expected formatted text %q, got %q", expectedText, firstEdit.Text)
	}
}

func TestBotRunner_LockUnlock(t *testing.T) {
	runner := &BotRunner{
		cfg: &config.Config{},
	}

	if !runner.tryLock("key1") {
		t.Error("expected first tryLock to succeed")
	}

	if runner.tryLock("key1") {
		t.Error("expected second tryLock with same key to fail")
	}

	runner.unlock("key1")

	if !runner.tryLock("key1") {
		t.Error("expected tryLock to succeed after unlock")
	}
}

func TestSendError_RedactsSecrets(t *testing.T) {
	mock := &mockTelegramClient{}
	runner := &BotRunner{
		cfg: &config.Config{
			TelegramToken: "123456:AAHsuperSECRETtoken",
			GeminiAPIKey:  "AIzaSyREAL_SECRET_KEY",
		},
		api: mock,
	}

	runner.sendError(1, 2, "tldr",
		`Get "https://api.telegram.org/file/bot123456:AAHsuperSECRETtoken/f.oga": dial tcp: no host`)

	if len(mock.sentMessages) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(mock.sentMessages))
	}

	edit, ok := mock.sentMessages[0].(tgbotapi.EditMessageTextConfig)
	if !ok {
		t.Fatalf("expected EditMessageTextConfig, got %T", mock.sentMessages[0])
	}
	if strings.Contains(edit.Text, "AAHsuperSECRETtoken") {
		t.Errorf("bot token leaked into user-facing message: %q", edit.Text)
	}
	if !strings.Contains(edit.Text, "[REDACTED]") {
		t.Errorf("expected redaction placeholder in message, got %q", edit.Text)
	}
}

func TestSplitMessage_MultiByteSafety(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxUnits int
	}{
		{"turkish without spaces", strings.Repeat("ğ", 100), 51},
		{"emoji", strings.Repeat("🎙", 60), 40},
		{"cjk", strings.Repeat("音", 100), 30},
		{"ordinary prose", strings.Repeat("merhaba dünya ", 50), 40},
		{"newline heavy", strings.Repeat("satır bir\n", 30), 35},
	}

	for _, tt := range tests {
		chunks := splitMessage(tt.text, tt.maxUnits)
		if len(chunks) == 0 {
			t.Fatalf("%s: expected at least one chunk", tt.name)
		}

		var rejoined strings.Builder
		for i, c := range chunks {
			if !utf8.ValidString(c) {
				t.Errorf("%s: chunk %d is not valid UTF-8", tt.name, i)
			}
			if utf16Len(c) > tt.maxUnits {
				t.Errorf("%s: chunk %d is %d UTF-16 units, limit is %d",
					tt.name, i, utf16Len(c), tt.maxUnits)
			}
			rejoined.WriteString(c)
		}

		// Whitespace moves around at chunk boundaries; compare with it removed.
		strip := func(s string) string { return strings.Join(strings.Fields(s), "") }
		if strip(rejoined.String()) != strip(tt.text) {
			t.Errorf("%s: content lost while splitting", tt.name)
		}
	}
}

type mockAIProvider struct {
	mu     sync.Mutex
	name   string
	calls  int
	prompt string
	result *provider.AIResult
	err    error
}

func (m *mockAIProvider) Name() string { return m.name }

func (m *mockAIProvider) Generate(ctx context.Context, systemPrompt, audioBase64, mimeType string) (*provider.AIResult, error) {
	m.mu.Lock()
	m.calls++
	m.prompt = systemPrompt
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func (m *mockAIProvider) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// lastPrompt is what the model was actually told to do — the only place the
// answer language is decided.
func (m *mockAIProvider) lastPrompt() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.prompt
}

// sentTexts collects the message bodies the bot produced, whatever envelope
// they arrived in.
func sentTexts(m *mockTelegramClient) []string {
	var texts []string
	for _, c := range m.sent() {
		switch msg := c.(type) {
		case tgbotapi.EditMessageTextConfig:
			texts = append(texts, msg.Text)
		case tgbotapi.MessageConfig:
			texts = append(texts, msg.Text)
		}
	}
	return texts
}

func containsText(texts []string, needle string) bool {
	for _, t := range texts {
		if strings.Contains(t, needle) {
			return true
		}
	}
	return false
}

// offersPaidButton reports whether any sent message carries the paid-fallback
// confirmation keyboard.
func offersPaidButton(m *mockTelegramClient) bool {
	for _, c := range m.sent() {
		edit, ok := c.(tgbotapi.EditMessageTextConfig)
		if !ok || edit.ReplyMarkup == nil {
			continue
		}
		for _, row := range edit.ReplyMarkup.InlineKeyboard {
			for _, btn := range row {
				if btn.CallbackData != nil && strings.HasPrefix(*btn.CallbackData, "paid:") {
					return true
				}
			}
		}
	}
	return false
}

// budgetTestRunner wires a runner whose audio download succeeds, so tests can
// reach the provider-selection logic.
func budgetTestRunner(t *testing.T, cfg *config.Config, tracker *budget.Tracker) (*BotRunner, *mockTelegramClient, *mockAIProvider, *mockAIProvider) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake-audio-bytes"))
	}))
	t.Cleanup(srv.Close)

	tg := &mockTelegramClient{fileURL: srv.URL + "/voice.ogg"}
	google := &mockAIProvider{name: "google"}
	openRouter := &mockAIProvider{name: "openrouter"}

	runner := &BotRunner{
		cfg:                cfg,
		api:                tg,
		googleProvider:     google,
		openRouterProvider: openRouter,
		budget:             tracker,
		httpClient:         srv.Client(),
	}
	return runner, tg, google, openRouter
}

func TestProcessVoice_BudgetCeilingRefusesPaidCall(t *testing.T) {
	tracker := budget.New(0.10, 0)
	tracker.Record(0.10)

	runner, tg, _, openRouter := budgetTestRunner(t,
		&config.Config{OpenRouterModel: "test/model", DailyCostLimit: 0.10}, tracker)
	runner.openRouterProvider.(*mockAIProvider).result = &provider.AIResult{Text: "must not be produced"}

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "openrouter", "audio/ogg")

	if openRouter.callCount() != 0 {
		t.Fatalf("paid provider was called despite a full budget (%d calls)", openRouter.callCount())
	}

	texts := sentTexts(tg)
	if !containsText(texts, "harcama tavanına ulaşıldı") {
		t.Errorf("refusal reason missing from the reply: %v", texts)
	}
	if !containsText(texts, "DAILY_COST_LIMIT") {
		t.Errorf("refusal does not name the setting that caused it: %v", texts)
	}
}

func TestProcessVoice_BudgetUnderCeilingAllowsPaidCallAndRecordsCost(t *testing.T) {
	tracker := budget.New(0.10, 1.0)

	runner, tg, _, openRouter := budgetTestRunner(t,
		&config.Config{OpenRouterModel: "test/model", DailyCostLimit: 0.10, MonthlyCostLimit: 1.0}, tracker)
	openRouter.result = &provider.AIResult{Text: "sonuç", PromptTokens: 10, CompletionTokens: 5, TotalCost: 0.02}

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "openrouter", "audio/ogg")

	if openRouter.callCount() != 1 {
		t.Fatalf("expected exactly 1 paid call under the ceiling, got %d", openRouter.callCount())
	}
	if s := tracker.Snapshot(); s.DailySpent != 0.02 || s.MonthlySpent != 0.02 {
		t.Errorf("cost of the call was not recorded: %+v", s)
	}
	if texts := sentTexts(tg); !containsText(texts, "Bütçe") {
		t.Errorf("usage summary does not report remaining budget: %v", texts)
	}
}

func TestProcessVoice_BudgetCeilingHidesPaidFallbackButton(t *testing.T) {
	tracker := budget.New(0, 1.0)
	tracker.Record(1.0)

	runner, tg, google, openRouter := budgetTestRunner(t,
		&config.Config{GeminiAPIKey: "test-key", OpenRouterModel: "test/model", MonthlyCostLimit: 1.0}, tracker)
	google.err = errors.New("HTTP 429: quota exceeded")

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "", "audio/ogg")

	if offersPaidButton(tg) {
		t.Error("paid fallback was offered even though the budget is exhausted")
	}
	if openRouter.callCount() != 0 {
		t.Errorf("paid provider was called (%d times)", openRouter.callCount())
	}

	texts := sentTexts(tg)
	if !containsText(texts, "harcama tavanına ulaşıldı") {
		t.Errorf("refusal reason missing from the reply: %v", texts)
	}
	if !containsText(texts, "MONTHLY_COST_LIMIT") {
		t.Errorf("monthly ceiling must name MONTHLY_COST_LIMIT: %v", texts)
	}
}

func TestProcessVoice_PaidFallbackStillOfferedWithBudgetLeft(t *testing.T) {
	runner, tg, google, _ := budgetTestRunner(t,
		&config.Config{GeminiAPIKey: "test-key", OpenRouterModel: "test/model", DailyCostLimit: 1.0},
		budget.New(1.0, 0))
	google.err = errors.New("HTTP 429: quota exceeded")

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "", "audio/ogg")

	if !offersPaidButton(tg) {
		t.Errorf("paid fallback should still be offered while budget remains: %v", sentTexts(tg))
	}
}

func TestProcessVoice_NoBudgetConfiguredKeepsSummaryUnchanged(t *testing.T) {
	runner, tg, _, openRouter := budgetTestRunner(t,
		&config.Config{OpenRouterModel: "test/model"}, budget.New(0, 0))
	openRouter.result = &provider.AIResult{Text: "sonuç", TotalCost: 0.02}

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "openrouter", "audio/ogg")

	if openRouter.callCount() != 1 {
		t.Fatalf("expected the call to go through without a ceiling, got %d calls", openRouter.callCount())
	}
	if texts := sentTexts(tg); containsText(texts, "Bütçe") {
		t.Errorf("budget line must stay out of the summary when no ceiling is set: %v", texts)
	}
}

func TestBudgetRefusalText(t *testing.T) {
	daily := budgetRefusalText(&budget.LimitError{Window: budget.WindowDaily, Spent: 0.5, Limit: 0.5})
	if !strings.Contains(daily, "Günlük") || !strings.Contains(daily, "DAILY_COST_LIMIT") {
		t.Errorf("daily refusal text is wrong: %q", daily)
	}

	monthly := budgetRefusalText(&budget.LimitError{Window: budget.WindowMonthly, Spent: 12, Limit: 10})
	if !strings.Contains(monthly, "Aylık") || !strings.Contains(monthly, "MONTHLY_COST_LIMIT") {
		t.Errorf("monthly refusal text is wrong: %q", monthly)
	}

	// An unexpected error must still produce a usable sentence.
	if generic := budgetRefusalText(errors.New("boom")); generic == "" {
		t.Error("expected a fallback message for a non-LimitError")
	}
}

func TestBudgetSummaryLine(t *testing.T) {
	if line := budgetSummaryLine(budget.Status{}); line != "" {
		t.Errorf("expected no summary line without a ceiling, got %q", line)
	}

	calm := budgetSummaryLine(budget.Status{DailySpent: 0.1, DailyLimit: 1})
	if !strings.Contains(calm, "günlük") || strings.Contains(calm, "⚠️") {
		t.Errorf("unexpected calm summary line: %q", calm)
	}

	loud := budgetSummaryLine(budget.Status{DailySpent: 0.9, DailyLimit: 1, MonthlySpent: 0.9, MonthlyLimit: 10})
	if !strings.Contains(loud, "⚠️") || !strings.Contains(loud, "aylık") {
		t.Errorf("expected a warning covering both windows: %q", loud)
	}
}
