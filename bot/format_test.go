package bot

import (
	"strings"
	"testing"

	"scribo/config"
	"scribo/mode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMarkdownToHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold", "bu **çok** önemli", "bu <b>çok</b> önemli"},
		{"bold underscore", "bu __çok__ önemli", "bu <b>çok</b> önemli"},
		{"italic", "bu *biraz* önemli", "bu <i>biraz</i> önemli"},
		{"italic underscore", "bu _biraz_ önemli", "bu <i>biraz</i> önemli"},
		{"bold not eaten by italic", "**kalın**", "<b>kalın</b>"},
		{"strikethrough", "~~silindi~~", "<s>silindi</s>"},
		{"heading", "# Başlık", "<b>Başlık</b>"},
		{"heading with trailing hashes", "## Başlık ##", "<b>Başlık</b>"},
		{"bullet dash", "- madde", "• madde"},
		{"bullet star", "* madde", "• madde"},
		{"nested bullet keeps indent", "  - alt madde", "  • alt madde"},
		{"link", "[Scribo](https://example.com)", `<a href="https://example.com">Scribo</a>`},
		{"inline code", "`kod` çalışır", "<code>kod</code> çalışır"},
		{"html is escaped", "5 < 6 & 7 > 6", "5 &lt; 6 &amp; 7 &gt; 6"},
		{"markup inside inline code is literal", "`**not bold**`", "<code>**not bold**</code>"},
		{"numbered list untouched", "1. birinci", "1. birinci"},
		{"underscore inside word is not italic", "snake_case_name", "snake_case_name"},
		{"multiline", "# Not\n\n- bir\n- **iki**", "<b>Not</b>\n\n• bir\n• <b>iki</b>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := markdownToHTML(tt.in); got != tt.want {
				t.Errorf("markdownToHTML(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMarkdownToHTML_FencedBlockKeepsContentsVerbatim(t *testing.T) {
	got := markdownToHTML("Şuna bak:\n```go\nif a < b { *x = 1 }\n```\nbitti")

	if !strings.Contains(got, "<pre>if a &lt; b { *x = 1 }\n</pre>") {
		t.Errorf("fenced block not preserved verbatim: %q", got)
	}
	if strings.Contains(got, "<i>") {
		t.Errorf("inline rules leaked into fenced block: %q", got)
	}
}

// A NUL byte in model output must not be able to forge a stash marker and pull an
// already-rendered fragment into an unintended position.
func TestMarkdownToHTML_NulCannotForgePlaceholder(t *testing.T) {
	got := markdownToHTML("\x000\x00 `gerçek`")

	if strings.Count(got, "<code>gerçek</code>") != 1 {
		t.Errorf("forged placeholder duplicated a stashed fragment: %q", got)
	}
	if strings.Contains(got, "\x00") {
		t.Errorf("NUL survived into output: %q", got)
	}
}

func TestRenderChunk(t *testing.T) {
	tests := []struct {
		name      string
		format    mode.Format
		in        string
		wantText  string
		wantParse string
	}{
		{"code wraps and escapes", mode.FormatCode, "a < b", "<code>a &lt; b</code>", tgbotapi.ModeHTML},
		{"unknown format falls back to code", mode.Format("bogus"), "a", "<code>a</code>", tgbotapi.ModeHTML},
		{"empty format falls back to code", mode.Format(""), "a", "<code>a</code>", tgbotapi.ModeHTML},
		{"plain is verbatim with no parse mode", mode.FormatPlain, "a < b", "a < b", ""},
		{"markdown converts", mode.FormatMarkdown, "**a**", "<b>a</b>", tgbotapi.ModeHTML},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, parse := renderChunk(tt.in, tt.format)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if parse != tt.wantParse {
				t.Errorf("parse mode = %q, want %q", parse, tt.wantParse)
			}
		})
	}
}

// Escaping multiplies length, so a fixed source-side margin is not enough: 3900 '&'
// runes render to 19500 units. Every chunk must fit after rendering, not before.
func TestSplitForFormat_AccountsForEscapeExpansion(t *testing.T) {
	for _, f := range []mode.Format{mode.FormatCode, mode.FormatMarkdown, mode.FormatPlain} {
		t.Run(string(f), func(t *testing.T) {
			chunks := splitForFormat(strings.Repeat("&", 8000), f)
			if len(chunks) == 0 {
				t.Fatal("expected chunks, got none")
			}
			for i, c := range chunks {
				out, _ := renderChunk(c, f)
				if utf16Len(out) > telegramMessageLimit {
					t.Errorf("chunk %d renders to %d units, over the %d limit",
						i, utf16Len(out), telegramMessageLimit)
				}
			}
		})
	}
}

func TestSplitForFormat_ShortTextStaysOneChunk(t *testing.T) {
	chunks := splitForFormat("kısa metin", mode.FormatMarkdown)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestSendSuccessResponse_MarkdownFormatIsNotCodeWrapped(t *testing.T) {
	mock := &mockTelegramClient{}
	runner := &BotRunner{cfg: &config.Config{}, api: mock}

	runner.sendSuccessResponse(1, 2, "# Başlık\n\n- **bir**", "cost", mode.FormatMarkdown)

	if len(mock.sentMessages) == 0 {
		t.Fatal("expected sent messages, got 0")
	}
	edit, ok := mock.sentMessages[0].(tgbotapi.EditMessageTextConfig)
	if !ok {
		t.Fatalf("expected EditMessageTextConfig, got %T", mock.sentMessages[0])
	}
	if strings.HasPrefix(edit.Text, "<code>") {
		t.Errorf("markdown mode still wrapped in <code>: %q", edit.Text)
	}
	if edit.Text != "<b>Başlık</b>\n\n• <b>bir</b>" {
		t.Errorf("unexpected rendered text: %q", edit.Text)
	}
	if edit.ParseMode != tgbotapi.ModeHTML {
		t.Errorf("parse mode = %q, want HTML", edit.ParseMode)
	}
}

func TestSendSuccessResponse_PlainFormatSendsVerbatim(t *testing.T) {
	mock := &mockTelegramClient{}
	runner := &BotRunner{cfg: &config.Config{}, api: mock}

	runner.sendSuccessResponse(1, 2, "a < b & **c**", "cost", mode.FormatPlain)

	edit, ok := mock.sentMessages[0].(tgbotapi.EditMessageTextConfig)
	if !ok {
		t.Fatalf("expected EditMessageTextConfig, got %T", mock.sentMessages[0])
	}
	if edit.Text != "a < b & **c**" {
		t.Errorf("plain text was altered: %q", edit.Text)
	}
	if edit.ParseMode != "" {
		t.Errorf("parse mode = %q, want empty", edit.ParseMode)
	}
}
