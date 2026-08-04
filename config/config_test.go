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
