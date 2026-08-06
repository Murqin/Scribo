package bot

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"scribo/config"
)

func setMyCommands(t *testing.T, tg *mockTelegramClient) tgbotapi.SetMyCommandsConfig {
	t.Helper()
	for _, req := range tg.requested() {
		if cfg, ok := req.(tgbotapi.SetMyCommandsConfig); ok {
			return cfg
		}
	}
	t.Fatal("no setMyCommands request was made")
	return tgbotapi.SetMyCommandsConfig{}
}

func TestRegisterCommands_PublishesTheLanguagesOwnNames(t *testing.T) {
	tests := []struct {
		lang string
		want []string
	}{
		{lang: "en", want: []string{"start", "last"}},
		{lang: "tr", want: []string{"basla", "son"}},
	}

	for _, tc := range tests {
		t.Run(tc.lang, func(t *testing.T) {
			useLanguage(t, tc.lang)

			tg := &mockTelegramClient{}
			runner := &BotRunner{cfg: &config.Config{}, api: tg}
			runner.registerCommands()

			cfg := setMyCommands(t, tg)
			if len(cfg.Commands) != len(tc.want) {
				t.Fatalf("registered %d commands, want %d: %+v", len(cfg.Commands), len(tc.want), cfg.Commands)
			}
			for i, want := range tc.want {
				if cfg.Commands[i].Command != want {
					t.Errorf("command %d = %q, want %q", i, cfg.Commands[i].Command, want)
				}
				if cfg.Commands[i].Description == "" {
					t.Errorf("command %q was registered without a description", want)
				}
			}
		})
	}
}

// The menu names come from the catalog while the dispatcher is a switch in Go,
// so nothing but this test stops the two from drifting apart and leaving the
// menu advertising a command that does nothing.
func TestRegisterCommands_EveryAdvertisedNameIsActuallyHandled(t *testing.T) {
	for _, lang := range []string{"tr", "en"} {
		t.Run(lang, func(t *testing.T) {
			useLanguage(t, lang)

			for _, cmd := range commandMenu() {
				tg := &mockTelegramClient{}
				runner := &BotRunner{cfg: &config.Config{AllowAllUsers: true}, api: tg}
				runner.handleMessage(context.Background(), commandMessage(42, "/"+cmd.Command))

				if len(sentTexts(tg)) == 0 {
					t.Errorf("/%s is advertised in the %s menu but the bot does not answer it", cmd.Command, lang)
				}
			}
		})
	}
}

// language_code is deliberately unset: Telegram matches it against the user's
// own client language, whereas Scribo's language is one process-wide setting.
// Filling it in would put the menu back out of step with the replies.
func TestRegisterCommands_DoesNotScopeTheMenuToAClientLanguage(t *testing.T) {
	useLanguage(t, "tr")

	tg := &mockTelegramClient{}
	runner := &BotRunner{cfg: &config.Config{}, api: tg}
	runner.registerCommands()

	cfg := setMyCommands(t, tg)
	if cfg.LanguageCode != "" {
		t.Errorf("LanguageCode = %q, want empty", cfg.LanguageCode)
	}
	if cfg.Scope != nil {
		t.Errorf("Scope = %+v, want nil (the default scope covers every chat)", cfg.Scope)
	}
}

// Telegram refusing the menu must not be fatal; the bot's actual work does not
// depend on it.
func TestRegisterCommands_SurvivesATelegramFailure(t *testing.T) {
	useLanguage(t, "tr")

	tg := &mockTelegramClient{requestErr: errors.New("429 Too Many Requests")}
	runner := &BotRunner{cfg: &config.Config{AllowAllUsers: true}, api: tg}

	runner.registerCommands()

	runner.handleMessage(context.Background(), commandMessage(42, "/basla"))
	if len(sentTexts(tg)) == 0 {
		t.Error("the bot stopped answering after the command menu failed to register")
	}
}

// The token can reach a Telegram error message through the URL it quotes, and
// net/http puts the full URL in its error text — so the warning has to be
// redacted before it is logged, not after.
func TestRegisterCommands_RedactsTheTokenFromAFailure(t *testing.T) {
	useLanguage(t, "tr")

	logged := captureLogs(t)

	const token = "123456:SUPER_SECRET_BOT_TOKEN"
	tg := &mockTelegramClient{requestErr: errors.New("Post https://api.telegram.org/bot" + token + "/setMyCommands: refused")}
	runner := &BotRunner{cfg: &config.Config{TelegramToken: token}, api: tg}

	runner.registerCommands()

	if strings.Contains(logged.String(), token) {
		t.Errorf("the bot token was written to the log verbatim:\n%s", logged.String())
	}
	if !strings.Contains(logged.String(), "[REDACTED]") {
		t.Errorf("the failure was logged without redaction:\n%s", logged.String())
	}
}

// captureLogs redirects the default logger for the duration of one test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return buf
}

// StartPolling is the only caller, so without this the whole feature can be
// unwired without a single test noticing.
func TestStartPolling_RegistersTheCommandMenu(t *testing.T) {
	useLanguage(t, "tr")

	tg := &mockTelegramClient{}
	runner := &BotRunner{cfg: &config.Config{AllowAllUsers: true}, api: tg}

	// A cancelled context makes StartPolling take its shutdown branch straight
	// away, after the registration it does on the way in.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.StartPolling(ctx); err != nil {
		t.Fatalf("StartPolling: %v", err)
	}

	cfg := setMyCommands(t, tg)
	if len(cfg.Commands) == 0 {
		t.Fatal("StartPolling registered an empty command menu")
	}
	if cfg.Commands[0].Command != "basla" {
		t.Errorf("first registered command = %q, want basla", cfg.Commands[0].Command)
	}
}
