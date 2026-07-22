package bot

import (
	"context"
	"strings"
	"testing"

	"scribo/config"

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

func TestIsAuthorized(t *testing.T) {
	// 1. Restricted User ID
	runnerRestricted := &BotRunner{
		cfg: &config.Config{
			AllowedUserID: "123456",
		},
	}

	if !runnerRestricted.isAuthorized(123456) {
		t.Error("user 123456 should be authorized")
	}

	if runnerRestricted.isAuthorized(999999) {
		t.Error("user 999999 should not be authorized")
	}

	// 2. Empty AllowedUserID (Public mode)
	runnerPublic := &BotRunner{
		cfg: &config.Config{
			AllowedUserID: "",
		},
	}

	if !runnerPublic.isAuthorized(999999) {
		t.Error("user 999999 should be authorized when AllowedUserID is empty")
	}
}

type mockTelegramClient struct {
	sentMessages []tgbotapi.Chattable
	fileURL      string
	fileURLErr   error
}

func (m *mockTelegramClient) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sentMessages = append(m.sentMessages, c)
	return tgbotapi.Message{}, nil
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
			AllowedUserID: "100",
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

func TestBotRunner_SendSuccessResponse_TapToCopyPreFormatting(t *testing.T) {
	mock := &mockTelegramClient{}
	runner := &BotRunner{
		cfg: &config.Config{},
		api: mock,
	}

	runner.sendSuccessResponse(12345, 99, "Hello <World>", "cost info")

	if len(mock.sentMessages) == 0 {
		t.Fatal("expected sent messages, got 0")
	}

	firstEdit, ok := mock.sentMessages[0].(tgbotapi.EditMessageTextConfig)
	if !ok {
		t.Fatalf("expected first message to be EditMessageTextConfig, got %T", mock.sentMessages[0])
	}

	expectedText := "<pre>Hello &lt;World&gt;</pre>"
	if firstEdit.Text != expectedText {
		t.Errorf("expected formatted text %q, got %q", expectedText, firstEdit.Text)
	}
}
