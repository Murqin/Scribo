package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"scribo/config"
	"scribo/history"
)

// Each command answers to an English canonical name and a Turkish alias, and
// the pair has to work under either language: the alias exists precisely so a
// user who switches SCRIBO_LANG does not lose the command they had learned.
func TestCommands_BothNamesWorkInEveryLanguage(t *testing.T) {
	for _, lang := range []string{"tr", "en"} {
		for _, name := range []string{"/last", "/son"} {
			t.Run(lang+name, func(t *testing.T) {
				useLanguage(t, lang)

				store, path := tempHistory(t)
				store.Append(history.Entry{At: time.Now(), ChatID: 42, Text: "kaydedilmiş çıktı"})

				runner := &BotRunner{
					cfg:     &config.Config{AllowAllUsers: true},
					api:     &mockTelegramClient{},
					history: reopen(t, path),
				}
				runner.handleMessage(context.Background(), commandMessage(42, name))

				texts := sentTexts(runner.api.(*mockTelegramClient))
				if !containsText(texts, "kaydedilmiş çıktı") {
					t.Fatalf("%s under %s did not replay the last output: %v", name, lang, texts)
				}
			})
		}

		for _, name := range []string{"/start", "/basla"} {
			t.Run(lang+name, func(t *testing.T) {
				useLanguage(t, lang)

				runner := &BotRunner{
					cfg: &config.Config{AllowAllUsers: true},
					api: &mockTelegramClient{},
				}
				runner.handleMessage(context.Background(), commandMessage(42, name))

				texts := strings.Join(sentTexts(runner.api.(*mockTelegramClient)), "\n")
				if !strings.Contains(texts, "Scribo Bot") {
					t.Fatalf("%s under %s did not greet: %s", name, lang, texts)
				}
			})
		}
	}
}

// The greeting is the only place the commands are advertised, so it has to name
// the one that reads naturally in the active language — an English catalog
// telling the reader to type a Turkish word is the wart the aliases fix.
func TestStartGreeting_AdvertisesTheLanguagesOwnCommandName(t *testing.T) {
	tests := []struct {
		lang    string
		want    string
		notWant string
	}{
		{lang: "en", want: "/last", notWant: "/son"},
		{lang: "tr", want: "/son", notWant: "/last"},
	}

	for _, tc := range tests {
		t.Run(tc.lang, func(t *testing.T) {
			useLanguage(t, tc.lang)

			runner := &BotRunner{
				cfg: &config.Config{AllowAllUsers: true},
				api: &mockTelegramClient{},
			}
			runner.handleMessage(context.Background(), commandMessage(42, "/start"))

			texts := strings.Join(sentTexts(runner.api.(*mockTelegramClient)), "\n")
			if !strings.Contains(texts, tc.want) {
				t.Errorf("the %s greeting does not mention %s:\n%s", tc.lang, tc.want, texts)
			}
			if strings.Contains(texts, tc.notWant) {
				t.Errorf("the %s greeting still advertises %s:\n%s", tc.lang, tc.notWant, texts)
			}
		})
	}
}

// An unknown command must not be mistaken for one of the aliases and must not
// silently replay history.
func TestCommands_UnknownNameIsNotTreatedAsAnAlias(t *testing.T) {
	useLanguage(t, "tr")

	store, path := tempHistory(t)
	store.Append(history.Entry{At: time.Now(), ChatID: 42, Text: "sızmaması gereken çıktı"})

	runner := &BotRunner{
		cfg:     &config.Config{AllowAllUsers: true},
		api:     &mockTelegramClient{},
		history: reopen(t, path),
	}
	runner.handleMessage(context.Background(), commandMessage(42, "/sonuncu"))

	texts := sentTexts(runner.api.(*mockTelegramClient))
	if containsText(texts, "sızmaması gereken çıktı") {
		t.Fatalf("/sonuncu was handled as if it were /son: %v", texts)
	}
}
