package bot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scribo/budget"
	"scribo/config"
	"scribo/history"
	"scribo/mode"
	"scribo/provider"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func tempHistory(t *testing.T) (*history.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store, err := history.Open(path)
	if err != nil {
		t.Fatalf("history.Open: %v", err)
	}
	return store, path
}

// reopen builds a store over the same file a previous runner wrote, which is
// what a bot restart amounts to as far as persistence is concerned.
func reopen(t *testing.T, path string) *history.Store {
	t.Helper()
	store, err := history.Open(path)
	if err != nil {
		t.Fatalf("reopen history: %v", err)
	}
	return store
}

func commandMessage(chatID int64, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID: 10,
		From:      &tgbotapi.User{ID: 1},
		Chat:      &tgbotapi.Chat{ID: chatID},
		Text:      text,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: len(text)},
		},
	}
}

func TestProcessVoice_RecordsGoogleResultInHistory(t *testing.T) {
	store, _ := tempHistory(t)

	runner, _, google, _ := budgetTestRunner(t,
		&config.Config{GeminiAPIKey: "test-key", AllowAllUsers: true}, nil)
	runner.history = store
	google.result = &provider.AIResult{Text: "google dökümü", PromptTokens: 3, CompletionTokens: 4}

	runner.processVoice(context.Background(), 42, "file_1", "tldr", 2, "", "audio/ogg")

	entry, found, err := store.Last(42)
	if err != nil || !found {
		t.Fatalf("Last after a successful Google run: found=%v err=%v", found, err)
	}
	if entry.Text != "google dökümü" {
		t.Errorf("stored text = %q, want the model output", entry.Text)
	}
	if entry.Provider != "google" {
		t.Errorf("stored provider = %q, want google", entry.Provider)
	}
	if entry.Mode != "tldr" {
		t.Errorf("stored mode = %q, want tldr", entry.Mode)
	}
	if entry.Cost != 0 {
		t.Errorf("stored cost = %v, want 0 for a free-tier call", entry.Cost)
	}
}

func TestProcessVoice_RecordsPaidCallCostInHistory(t *testing.T) {
	store, _ := tempHistory(t)

	runner, _, _, openRouter := budgetTestRunner(t,
		&config.Config{OpenRouterModel: "test/model"}, budget.New(0, 0))
	runner.history = store
	openRouter.result = &provider.AIResult{Text: "ücretli döküm", TotalCost: 0.04}

	runner.processVoice(context.Background(), 42, "file_1", "tldr", 2, "openrouter", "audio/ogg")

	entry, found, err := store.Last(42)
	if err != nil || !found {
		t.Fatalf("Last after a paid run: found=%v err=%v", found, err)
	}
	if entry.Cost != 0.04 {
		t.Errorf("stored cost = %v, want 0.04 — the ceiling is seeded from this field", entry.Cost)
	}
	if entry.Provider != "openrouter" {
		t.Errorf("stored provider = %q, want openrouter", entry.Provider)
	}
}

// TestSon_ReturnsLastTranscriptAfterRestart is the phase acceptance criterion:
// a transcript produced by one runner has to come back from a second one that
// shares nothing but the file.
func TestSon_ReturnsLastTranscriptAfterRestart(t *testing.T) {
	store, path := tempHistory(t)

	first, _, google, _ := budgetTestRunner(t,
		&config.Config{GeminiAPIKey: "test-key"}, nil)
	first.history = store
	google.result = &provider.AIResult{Text: "yeniden başlatmayı aşan döküm"}
	first.processVoice(context.Background(), 42, "file_1", "tldr", 2, "", "audio/ogg")

	restarted := &BotRunner{
		cfg:     &config.Config{AllowAllUsers: true},
		api:     &mockTelegramClient{},
		history: reopen(t, path),
	}
	restarted.handleMessage(context.Background(), commandMessage(42, "/son"))

	texts := sentTexts(restarted.api.(*mockTelegramClient))
	if !containsText(texts, "yeniden başlatmayı aşan döküm") {
		t.Fatalf("/son did not return the transcript written before the restart: %v", texts)
	}
	if !containsText(texts, "Son çıktı") {
		t.Errorf("/son sent the text without its header: %v", texts)
	}
}

func TestSon_IgnoresOtherChatsTranscripts(t *testing.T) {
	store, path := tempHistory(t)
	store.Append(history.Entry{At: time.Now(), ChatID: 99, Text: "başka sohbetin dökümü"})

	runner := &BotRunner{
		cfg:     &config.Config{AllowAllUsers: true},
		api:     &mockTelegramClient{},
		history: reopen(t, path),
	}
	runner.handleMessage(context.Background(), commandMessage(42, "/son"))

	texts := sentTexts(runner.api.(*mockTelegramClient))
	if containsText(texts, "başka sohbetin dökümü") {
		t.Fatalf("/son leaked another chat's transcript: %v", texts)
	}
	if !containsText(texts, "kayıtlı çıktı yok") {
		t.Errorf("/son should report an empty history, got: %v", texts)
	}
}

func TestSon_ReportsDisabledHistory(t *testing.T) {
	runner := &BotRunner{
		cfg:     &config.Config{AllowAllUsers: true},
		api:     &mockTelegramClient{},
		history: nil,
	}
	runner.handleMessage(context.Background(), commandMessage(42, "/son"))

	texts := sentTexts(runner.api.(*mockTelegramClient))
	if !containsText(texts, "HISTORY_FILE") {
		t.Errorf("/son with persistence off must name the setting responsible: %v", texts)
	}
}

func TestSon_RendersWithTheStoredFormat(t *testing.T) {
	t.Cleanup(mode.LoadDefaultModes)
	if !mode.LoadModesFromBytes([]byte(`{"blog":{"label":"Blog","prompt":"p","format":"markdown"}}`), "test") {
		t.Fatal("failed to load the test mode set")
	}

	store, path := tempHistory(t)
	store.Append(history.Entry{At: time.Now(), ChatID: 42, Mode: "blog", Format: "markdown", Text: "**kalın**"})

	runner := &BotRunner{
		cfg:     &config.Config{AllowAllUsers: true},
		api:     &mockTelegramClient{},
		history: reopen(t, path),
	}
	runner.handleMessage(context.Background(), commandMessage(42, "/son"))

	texts := sentTexts(runner.api.(*mockTelegramClient))
	if !containsText(texts, "<b>kalın</b>") {
		t.Errorf("/son ignored the stored format; want rendered markdown, got: %v", texts)
	}
	if !containsText(texts, "Blog") {
		t.Errorf("/son header should carry the mode label: %v", texts)
	}
}

func TestSon_FallsBackToCodeForAnUnknownStoredFormat(t *testing.T) {
	store, path := tempHistory(t)
	store.Append(history.Entry{At: time.Now(), ChatID: 42, Mode: "tldr", Format: "uydurma", Text: "metin"})

	runner := &BotRunner{
		cfg:     &config.Config{AllowAllUsers: true},
		api:     &mockTelegramClient{},
		history: reopen(t, path),
	}
	runner.handleMessage(context.Background(), commandMessage(42, "/son"))

	if !containsText(sentTexts(runner.api.(*mockTelegramClient)), "<code>metin</code>") {
		t.Error("an unrecognised stored format must fall back to <code>, not drop the entry")
	}
}

func TestSon_IsRefusedForUnauthorizedUsers(t *testing.T) {
	store, path := tempHistory(t)
	store.Append(history.Entry{At: time.Now(), ChatID: 42, Text: "gizli döküm"})

	runner := &BotRunner{
		cfg:     &config.Config{AllowedUserIDs: []int64{7}},
		api:     &mockTelegramClient{},
		history: reopen(t, path),
	}
	msg := commandMessage(42, "/son")
	msg.From.ID = 999
	runner.handleMessage(context.Background(), msg)

	if texts := sentTexts(runner.api.(*mockTelegramClient)); len(texts) != 0 {
		t.Fatalf("/son answered an unauthorized user: %v", texts)
	}
}

func TestStartMentionsSonCommand(t *testing.T) {
	runner := &BotRunner{
		cfg: &config.Config{AllowAllUsers: true},
		api: &mockTelegramClient{},
	}
	runner.handleMessage(context.Background(), commandMessage(42, "/start"))

	if !containsText(sentTexts(runner.api.(*mockTelegramClient)), "/son") {
		t.Error("/start does not tell the user that /son exists")
	}
}

func TestSeedBudget_CarriesTheCeilingAcrossARestart(t *testing.T) {
	store, path := tempHistory(t)
	store.Append(history.Entry{At: time.Now(), ChatID: 42, Cost: 0.12})

	cfg := &config.Config{DailyCostLimit: 0.10}
	tracker := budget.New(cfg.DailyCostLimit, cfg.MonthlyCostLimit)
	if err := seedBudget(tracker, reopen(t, path), cfg); err != nil {
		t.Fatalf("seedBudget: %v", err)
	}

	if tracker.Check() == nil {
		t.Fatal("spend recorded before the restart did not count against the ceiling afterwards")
	}

	// And the refusal has to reach the user, not just the tracker.
	runner, tg, _, openRouter := budgetTestRunner(t,
		&config.Config{OpenRouterModel: "test/model", DailyCostLimit: 0.10}, tracker)
	runner.history = store
	openRouter.result = &provider.AIResult{Text: "must not be produced"}
	runner.processVoice(context.Background(), 42, "file_1", "tldr", 2, "openrouter", "audio/ogg")

	if openRouter.callCount() != 0 {
		t.Errorf("paid provider ran despite a ceiling restored from history (%d calls)", openRouter.callCount())
	}
	if !containsText(sentTexts(tg), "harcama tavanına ulaşıldı") {
		t.Errorf("restored ceiling refused the call without saying why: %v", sentTexts(tg))
	}
}

func TestSeedBudget_ExpiredSpendDoesNotCount(t *testing.T) {
	store, path := tempHistory(t)
	store.Append(history.Entry{At: time.Now().AddDate(0, 0, -2), ChatID: 42, Cost: 5.0})

	cfg := &config.Config{DailyCostLimit: 0.10}
	tracker := budget.New(cfg.DailyCostLimit, 0)
	if err := seedBudget(tracker, reopen(t, path), cfg); err != nil {
		t.Fatalf("seedBudget: %v", err)
	}

	if err := tracker.Check(); err != nil {
		t.Errorf("spend from a previous day must not block today's calls, got %v", err)
	}
}

// breakHistoryFile turns an opened history path into one that opens but neither
// reads nor writes: a directory. Staging the failure after history.Open is the
// only way to reach the error paths that Open itself is meant to catch early.
func breakHistoryFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove history file: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("stage unusable history: %v", err)
	}
}

func TestSeedBudget_UnreadableHistoryIsFatalOnlyWithACeiling(t *testing.T) {
	store, path := tempHistory(t)
	breakHistoryFile(t, path)

	withCeiling := &config.Config{DailyCostLimit: 0.10}
	if err := seedBudget(budget.New(0.10, 0), store, withCeiling); err == nil {
		t.Error("an unreadable history must not silently reset a configured ceiling")
	}

	withoutCeiling := &config.Config{}
	if err := seedBudget(budget.New(0, 0), store, withoutCeiling); err != nil {
		t.Errorf("without a ceiling an unreadable history is only a warning, got %v", err)
	}
}

func TestRecord_HistoryFailureDoesNotWithholdTheTranscript(t *testing.T) {
	// The archive is best-effort: the user already waited for this output.
	store, path := tempHistory(t)
	breakHistoryFile(t, path)

	runner, tg, google, _ := budgetTestRunner(t,
		&config.Config{GeminiAPIKey: "test-key"}, nil)
	runner.history = store
	google.result = &provider.AIResult{Text: "kaydedilemeyen döküm"}

	runner.processVoice(context.Background(), 42, "file_1", "tldr", 2, "", "audio/ogg")

	if !containsText(sentTexts(tg), "kaydedilemeyen döküm") {
		t.Errorf("a failed history write swallowed the transcript: %v", sentTexts(tg))
	}
}

func TestBudgetRefusalTextNoLongerPromisesAResetOnRestart(t *testing.T) {
	txt := budgetRefusalText(&budget.LimitError{Window: budget.WindowDaily, Spent: 1, Limit: 1})
	if strings.Contains(txt, "sıfırlanır") {
		t.Error("the refusal still claims a restart clears the counter, which persistence made false")
	}
	if !strings.Contains(txt, "DAILY_COST_LIMIT") {
		t.Error("the refusal must still name the setting behind it")
	}
}

func TestBudgetRefusalTextOnAnUnknownError(t *testing.T) {
	if txt := budgetRefusalText(errors.New("beklenmedik")); txt == "" {
		t.Error("an unrecognised error must still produce an explanation")
	}
}
