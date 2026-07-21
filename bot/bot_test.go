package bot

import (
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
	shortText := "Short text"
	chunks := splitMessage(shortText, 100)
	if len(chunks) != 1 || chunks[0] != shortText {
		t.Errorf("expected 1 chunk matching short text, got %v", chunks)
	}

	longText := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	chunks = splitMessage(longText, 15)
	if len(chunks) < 2 {
		t.Errorf("expected long text to be split into multiple chunks, got %d chunks", len(chunks))
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

func TestIsAuthorized(t *testing.T) {
	runner := &BotRunner{
		cfg: &config.Config{
			AllowedUserID: "123456",
		},
	}

	if !runner.isAuthorized(123456) {
		t.Error("user 123456 should be authorized")
	}

	if runner.isAuthorized(999999) {
		t.Error("user 999999 should not be authorized")
	}
}
