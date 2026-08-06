package bot

import (
	"context"
	"strings"
	"testing"

	"scribo/budget"
	"scribo/config"
	"scribo/i18n"
	"scribo/mode"
	"scribo/provider"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// useLanguage switches the bot to another language the way main does, and puts
// it back afterwards: both the catalog and the loaded mode set are
// package-level state shared with every other test.
func useLanguage(t *testing.T, lang string) {
	t.Helper()
	t.Cleanup(func() {
		i18n.SetLanguage(i18n.DefaultLanguage)
		mode.LoadDefaultModes()
	})
	i18n.SetLanguage(lang)
	mode.LoadDefaultModes()
}

func TestProcessVoice_EnglishRunSpeaksEnglishToBothTheUserAndTheModel(t *testing.T) {
	useLanguage(t, "en")

	runner, tg, google, _ := budgetTestRunner(t,
		&config.Config{GeminiAPIKey: "test-key", AllowAllUsers: true}, nil)
	google.result = &provider.AIResult{Text: "output", PromptTokens: 3, CompletionTokens: 4}

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "", "audio/ogg")

	texts := strings.Join(sentTexts(tg), "\n")
	for _, want := range []string{"Preparing", "Usage summary"} {
		if !strings.Contains(texts, want) {
			t.Errorf("English interface string %q missing from the reply:\n%s", want, texts)
		}
	}
	for _, unwanted := range []string{"hazırlanıyor", "Kullanım Özeti"} {
		if strings.Contains(texts, unwanted) {
			t.Errorf("Turkish interface string %q leaked into an English run:\n%s", unwanted, texts)
		}
	}

	// The interface being English is only half the criterion: a Turkish prompt
	// would still make the model answer in Turkish.
	prompt := google.lastPrompt()
	if !strings.Contains(prompt, "Answer in English") {
		t.Errorf("the mode prompt sent to the model is not the English one:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Note: today is") {
		t.Errorf("the date note appended to the prompt is not in English:\n%s", prompt)
	}
	if strings.Contains(prompt, "Bugünün tarihi") {
		t.Errorf("the Turkish date note leaked into an English run:\n%s", prompt)
	}
}

func TestProcessVoice_DefaultRunStaysTurkish(t *testing.T) {
	runner, tg, google, _ := budgetTestRunner(t,
		&config.Config{GeminiAPIKey: "test-key", AllowAllUsers: true}, nil)
	google.result = &provider.AIResult{Text: "output", PromptTokens: 3, CompletionTokens: 4}

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "", "audio/ogg")

	texts := strings.Join(sentTexts(tg), "\n")
	if !strings.Contains(texts, "Kullanım Özeti") {
		t.Errorf("an unconfigured bot must stay Turkish:\n%s", texts)
	}
	if !strings.Contains(google.lastPrompt(), "Bugünün tarihi") {
		t.Errorf("an unconfigured bot sent a non-Turkish prompt:\n%s", google.lastPrompt())
	}
}

func TestHandleMessage_GuidanceFollowsTheLanguage(t *testing.T) {
	useLanguage(t, "en")

	tg := &mockTelegramClient{}
	runner := &BotRunner{cfg: &config.Config{AllowAllUsers: true}, api: tg}

	runner.handleMessage(context.Background(), commandMessage(42, "/start"))
	runner.handleMessage(context.Background(), &tgbotapi.Message{
		MessageID: 11,
		From:      &tgbotapi.User{ID: 1},
		Chat:      &tgbotapi.Chat{ID: 42},
		Text:      "merhaba",
	})

	texts := strings.Join(sentTexts(tg), "\n")
	if !strings.Contains(texts, "Scribo Bot is ready") {
		t.Errorf("/start did not answer in English:\n%s", texts)
	}
	if !strings.Contains(texts, "Please send a voice note") {
		t.Errorf("the guidance message did not answer in English:\n%s", texts)
	}
}

func TestBudgetRefusal_FollowsTheLanguage(t *testing.T) {
	useLanguage(t, "en")

	tracker := budget.New(0.10, 0)
	tracker.Record(0.10)

	runner, tg, _, openRouter := budgetTestRunner(t,
		&config.Config{OpenRouterModel: "test/model", DailyCostLimit: 0.10}, tracker)

	runner.processVoice(context.Background(), 1, "file_1", "tldr", 2, "openrouter", "audio/ogg")

	if openRouter.callCount() != 0 {
		t.Fatalf("paid provider was called despite a full budget (%d calls)", openRouter.callCount())
	}
	texts := strings.Join(sentTexts(tg), "\n")
	if !strings.Contains(texts, "spending ceiling has been reached") {
		t.Errorf("the refusal was not translated:\n%s", texts)
	}
	if !strings.Contains(texts, "DAILY_COST_LIMIT") {
		t.Errorf("the translated refusal stopped naming the setting behind it:\n%s", texts)
	}
}

func TestModeKeyboard_LabelsFollowTheLanguage(t *testing.T) {
	useLanguage(t, "en")

	var labels []string
	for _, row := range mode.GetModeKeyboard().InlineKeyboard {
		for _, btn := range row {
			labels = append(labels, btn.Text)
		}
	}

	joined := strings.Join(labels, " ")
	if !strings.Contains(joined, "Summary") {
		t.Errorf("the mode keyboard kept its Turkish labels: %v", labels)
	}
}

func TestExtractAudioTarget_MediaNamesFollowTheLanguage(t *testing.T) {
	useLanguage(t, "en")

	tests := []struct {
		name string
		msg  *tgbotapi.Message
		want string
	}{
		{"voice note", &tgbotapi.Message{Voice: &tgbotapi.Voice{FileID: "v"}}, "Voice note"},
		{"video message", &tgbotapi.Message{VideoNote: &tgbotapi.VideoNote{FileID: "n"}}, "Video message"},
		// A video with no file name of its own falls back to the generic label.
		{"unnamed video", &tgbotapi.Message{Video: &tgbotapi.Video{FileID: "d"}}, "Video"},
	}

	for _, tt := range tests {
		got := extractAudioTarget(tt.msg)
		if got == nil {
			t.Fatalf("%s: expected a target", tt.name)
		}
		if got.Name != tt.want {
			t.Errorf("%s: name = %q, want %q", tt.name, got.Name, tt.want)
		}
	}
}

func TestHandleMessage_MediaAcknowledgementFollowsTheLanguage(t *testing.T) {
	useLanguage(t, "en")

	tg := &mockTelegramClient{}
	runner := &BotRunner{cfg: &config.Config{AllowAllUsers: true}, api: tg}
	runner.handleMessage(context.Background(), &tgbotapi.Message{
		MessageID: 12,
		From:      &tgbotapi.User{ID: 1},
		Chat:      &tgbotapi.Chat{ID: 42},
		VideoNote: &tgbotapi.VideoNote{FileID: "n", Duration: 120},
	})

	texts := strings.Join(sentTexts(tg), "\n")
	if !strings.Contains(texts, "Video message") || !strings.Contains(texts, "Pick a processing mode") {
		t.Errorf("the acknowledgement did not answer in English:\n%s", texts)
	}
	if !strings.Contains(texts, "This recording is long") {
		t.Errorf("the long-media warning did not answer in English:\n%s", texts)
	}
}
