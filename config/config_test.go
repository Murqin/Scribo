package config

import (
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("TELEGRAM_TOKEN", "")
	t.Setenv("GEMINI_API_KEY", "")
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

func TestLoadConfig_CustomMaxConcurrentJobs(t *testing.T) {
	t.Setenv("MAX_CONCURRENT_JOBS", "10")
	cfg := LoadConfig()
	if cfg.MaxConcurrentJobs != 10 {
		t.Errorf("expected MaxConcurrentJobs 10, got %d", cfg.MaxConcurrentJobs)
	}

	t.Setenv("MAX_CONCURRENT_JOBS", "invalid")
	cfg2 := LoadConfig()
	if cfg2.MaxConcurrentJobs != 5 {
		t.Errorf("expected fallback MaxConcurrentJobs 5 for invalid input, got %d", cfg2.MaxConcurrentJobs)
	}
}

func TestLoadConfig_Validation(t *testing.T) {
	cfg := &Config{
		TelegramToken: "test_token",
		GeminiAPIKey:  "test_gemini_key",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}
