package bot

import (
	"os"
	"path/filepath"
	"testing"

	"scribo/config"
	"scribo/mode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// The whole point of the format field, end to end: a modes.json on disk decides how a
// reply is rendered, and a file written before the field existed still gets <code>.
func TestSendSuccessResponse_FormatFromDiskDrivesRendering(t *testing.T) {
	t.Cleanup(mode.LoadDefaultModes)

	modesFile := filepath.Join(t.TempDir(), "modes.json")
	if err := os.WriteFile(modesFile, []byte(`{
		"blog":   {"label": "Blog", "prompt": "p", "format": "markdown"},
		"legacy": {"label": "Legacy", "prompt": "p"}
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	mode.LoadCustomModes(modesFile)

	const reply = "# Başlık\n\n- **bir**"
	tests := []struct {
		id   string
		want string
	}{
		{"blog", "<b>Başlık</b>\n\n• <b>bir</b>"},
		{"legacy", "<code># Başlık\n\n- **bir**</code>"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			modeInfo, ok := mode.GetMode(tt.id)
			if !ok {
				t.Fatalf("mode %q was not loaded from disk", tt.id)
			}

			mock := &mockTelegramClient{}
			runner := &BotRunner{cfg: &config.Config{}, api: mock}
			runner.sendSuccessResponse(1, 2, reply, "cost", modeInfo.Format)

			if len(mock.sentMessages) == 0 {
				t.Fatal("expected sent messages, got 0")
			}
			edit, ok := mock.sentMessages[0].(tgbotapi.EditMessageTextConfig)
			if !ok {
				t.Fatalf("expected EditMessageTextConfig, got %T", mock.sentMessages[0])
			}
			if edit.Text != tt.want {
				t.Errorf("rendered text\n got: %q\nwant: %q", edit.Text, tt.want)
			}
		})
	}
}
