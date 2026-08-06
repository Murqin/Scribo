package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("MODEL", "")

	cfg := LoadConfig()

	if cfg.DefaultModel != "gemini-3.6-flash" {
		t.Errorf("expected default model gemini-3.6-flash, got %s", cfg.DefaultModel)
	}

	if cfg.MaxConcurrentJobs != 5 {
		t.Errorf("expected default MaxConcurrentJobs 5, got %d", cfg.MaxConcurrentJobs)
	}

	if err := cfg.Validate(); err == nil {
		t.Error("expected error on empty token/key, got nil")
	}
}

func TestLoadConfig_EnvCascades(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "fallback_google_key")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("MODEL", "custom-model")
	t.Setenv("GOOGLE_MODEL", "")
	t.Setenv("PROVIDER", "OPENROUTER")
	t.Setenv("DEFAULT_PROVIDER", "")

	cfg := LoadConfig()

	if cfg.GeminiAPIKey != "fallback_google_key" {
		t.Errorf("expected GeminiAPIKey fallback to GOOGLE_API_KEY, got %s", cfg.GeminiAPIKey)
	}

	if cfg.GoogleModel != "custom-model" {
		t.Errorf("expected GoogleModel fallback to MODEL, got %s", cfg.GoogleModel)
	}

	if cfg.DefaultProvider != "openrouter" {
		t.Errorf("expected DefaultProvider fallback to PROVIDER (lowercased), got %s", cfg.DefaultProvider)
	}
}

func TestLoadConfig_MaxConcurrentJobs(t *testing.T) {
	tests := []struct {
		envVal   string
		expected int
	}{
		{"10", 10},
		{"invalid", 5},
		{"10_jobs", 5},
		{"0", 5},
		{"-5", 5},
	}

	for _, tt := range tests {
		t.Setenv("MAX_CONCURRENT_JOBS", tt.envVal)
		cfg := LoadConfig()
		if cfg.MaxConcurrentJobs != tt.expected {
			t.Errorf("MAX_CONCURRENT_JOBS=%s: expected %d, got %d", tt.envVal, tt.expected, cfg.MaxConcurrentJobs)
		}
	}
}

func TestLoadConfig_ValidationMatrix(t *testing.T) {
	// 1. Missing Token
	cfgNoToken := &Config{TelegramToken: "", GeminiAPIKey: "key"}
	if err := cfgNoToken.Validate(); err == nil {
		t.Error("expected error for missing TELEGRAM_TOKEN")
	}

	// 2. Missing both AI keys
	cfgNoKeys := &Config{TelegramToken: "token", GeminiAPIKey: "", OpenRouterAPIKey: ""}
	if err := cfgNoKeys.Validate(); err == nil {
		t.Error("expected error for missing both AI keys")
	}

	// 3. OpenRouter key only (Valid)
	cfgOpenRouter := &Config{TelegramToken: "token", OpenRouterAPIKey: "openrouter_key"}
	if err := cfgOpenRouter.Validate(); err != nil {
		t.Errorf("expected valid config with OpenRouter key only, got error: %v", err)
	}
}

func TestLoadDotEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	envContent := `# Comment line
TEST_KEY_1=value1
TEST_KEY_2="quoted_value"
TEST_KEY_3='single_quoted'
`
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	loadDotEnv(envPath)

	if os.Getenv("TEST_KEY_1") != "value1" {
		t.Errorf("expected value1, got %s", os.Getenv("TEST_KEY_1"))
	}
	if os.Getenv("TEST_KEY_2") != "quoted_value" {
		t.Errorf("expected quoted_value, got %s", os.Getenv("TEST_KEY_2"))
	}
	if os.Getenv("TEST_KEY_3") != "single_quoted" {
		t.Errorf("expected single_quoted, got %s", os.Getenv("TEST_KEY_3"))
	}
}

func TestRedact(t *testing.T) {
	cfg := &Config{
		TelegramToken:    "123456:AAHsuperSECRETtoken",
		GeminiAPIKey:     "AIzaSyREAL_SECRET_KEY",
		OpenRouterAPIKey: "sk-or-v1-abcdefghijklmnop",
	}

	tests := []struct {
		name string
		in   string
	}{
		{"telegram file url", `Get "https://api.telegram.org/file/bot123456:AAHsuperSECRETtoken/voice/f.oga": no host`},
		{"gemini generate url", `Post "https://x/v1beta/models/m:generateContent?key=AIzaSyREAL_SECRET_KEY": no host`},
		{"openrouter key in body", `unauthorized for key sk-or-v1-abcdefghijklmnop`},
	}

	for _, tt := range tests {
		got := cfg.Redact(tt.in)
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("%s: expected redaction placeholder, got %q", tt.name, got)
		}
		for _, secret := range []string{cfg.TelegramToken, cfg.GeminiAPIKey, cfg.OpenRouterAPIKey} {
			if strings.Contains(got, secret) {
				t.Errorf("%s: secret %q survived redaction in %q", tt.name, secret, got)
			}
		}
	}

	// An empty config must not corrupt the message.
	empty := &Config{}
	if got := empty.Redact("hello world"); got != "hello world" {
		t.Errorf("empty config altered message: got %q", got)
	}
}

func TestParseUserIDs(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{"", 0},
		{"123", 1},
		{"123,456", 2},
		{" 123 , 456 ", 2},
		{"123,,456", 2},
		{"123,abc,456", 2},
		{"abc", 0},
	}

	for _, tt := range tests {
		if got := parseUserIDs(tt.raw); len(got) != tt.want {
			t.Errorf("parseUserIDs(%q) returned %d ids, want %d", tt.raw, len(got), tt.want)
		}
	}

	ids := parseUserIDs("111,222")
	if len(ids) != 2 || ids[0] != 111 || ids[1] != 222 {
		t.Errorf("parseUserIDs(\"111,222\") = %v, want [111 222]", ids)
	}
}

func TestLoadDotEnv_InlineComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	content := "PLAIN=value1 # trailing comment\n" +
		"QUOTED=\"value with # hash\"\n" +
		"HASHY=abc#def\n" +
		"# whole line comment\n" +
		"CLEAN=value2\n"

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("could not write test env file: %v", err)
	}

	for _, k := range []string{"PLAIN", "QUOTED", "HASHY", "CLEAN"} {
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range []string{"PLAIN", "QUOTED", "HASHY", "CLEAN"} {
			os.Unsetenv(k)
		}
	})

	loadDotEnv(path)

	tests := []struct{ key, want string }{
		{"PLAIN", "value1"},
		{"QUOTED", "value with # hash"},
		{"HASHY", "abc#def"},
		{"CLEAN", "value2"},
	}
	for _, tt := range tests {
		if got := os.Getenv(tt.key); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestLoadConfig_CostLimits(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "token")
	t.Setenv("GEMINI_API_KEY", "key")

	tests := []struct {
		name      string
		daily     string
		monthly   string
		wantDaily float64
		wantMonth float64
		wantErr   bool
	}{
		{name: "unset means no ceiling"},
		{name: "both parsed", daily: "0.50", monthly: "12", wantDaily: 0.50, wantMonth: 12},
		{name: "zero is accepted as unlimited", daily: "0"},
		{name: "malformed daily is an error", daily: "1,50", wantErr: true},
		{name: "negative monthly is an error", monthly: "-3", wantErr: true},
		{name: "trailing unit is an error", daily: "5usd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DAILY_COST_LIMIT", tt.daily)
			t.Setenv("MONTHLY_COST_LIMIT", tt.monthly)

			cfg := LoadConfig()
			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected a malformed spending limit to fail validation")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if cfg.DailyCostLimit != tt.wantDaily {
				t.Errorf("DailyCostLimit = %v, want %v", cfg.DailyCostLimit, tt.wantDaily)
			}
			if cfg.MonthlyCostLimit != tt.wantMonth {
				t.Errorf("MonthlyCostLimit = %v, want %v", cfg.MonthlyCostLimit, tt.wantMonth)
			}
		})
	}
}

func TestLoadConfig_HistoryFile(t *testing.T) {
	t.Run("unset gets the default file", func(t *testing.T) {
		// t.Setenv first so the cleanup restores whatever the caller had.
		t.Setenv("HISTORY_FILE", "yer-tutucu")
		os.Unsetenv("HISTORY_FILE")

		if got := LoadConfig().HistoryFile; got != defaultHistoryFile {
			t.Errorf("HistoryFile = %q, want the default %q", got, defaultHistoryFile)
		}
	})

	t.Run("explicitly empty disables persistence", func(t *testing.T) {
		// This is the one setting where empty must not mean "use the default":
		// it is how a user turns transcript storage off.
		t.Setenv("HISTORY_FILE", "")

		if got := LoadConfig().HistoryFile; got != "" {
			t.Errorf("HistoryFile = %q, want empty so persistence stays off", got)
		}
	})

	t.Run("custom path is used verbatim", func(t *testing.T) {
		t.Setenv("HISTORY_FILE", "  /veri/scribo.jsonl  ")

		if got := LoadConfig().HistoryFile; got != "/veri/scribo.jsonl" {
			t.Errorf("HistoryFile = %q, want the trimmed custom path", got)
		}
	})
}

func TestLoadConfig_LanguagePrefersScriboLangOverLocale(t *testing.T) {
	tests := []struct {
		name       string
		scriboLang string
		lang       string
		want       string
	}{
		{"neither set", "", "", ""},
		{"locale only", "", "en_US.UTF-8", "en_US.UTF-8"},
		{"explicit only", "en", "", "en"},
		// The realistic trap: a Turkish desktop exports LANG, and loadDotEnv
		// refuses to overwrite it, so LANG=en in .env would never take effect.
		{"explicit wins over locale", "en", "tr_TR.UTF-8", "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SCRIBO_LANG", tt.scriboLang)
			t.Setenv("LANG", tt.lang)

			if got := LoadConfig().Language; got != tt.want {
				t.Errorf("Language = %q, want %q", got, tt.want)
			}
		})
	}
}
