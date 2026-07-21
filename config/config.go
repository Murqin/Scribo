package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	TelegramToken    string
	OpenRouterAPIKey string
	GeminiAPIKey     string
	AllowedUserID    string
	DefaultModel     string
	GoogleModel      string
	OpenRouterModel  string
	DefaultProvider  string
}

func LoadConfig() *Config {
	loadDotEnv(".env")

	defaultModel := getEnv("MODEL", "gemini-3.6-flash")
	googleModel := getEnv("GOOGLE_MODEL", defaultModel)
	openRouterModel := getEnv("OPENROUTER_MODEL", "google/gemini-3.6-flash")

	geminiKey := getEnv("GEMINI_API_KEY", getEnv("GOOGLE_API_KEY", ""))
	defaultProvider := strings.ToLower(getEnv("DEFAULT_PROVIDER", getEnv("PROVIDER", "google")))

	return &Config{
		TelegramToken:    getEnv("TELEGRAM_TOKEN", ""),
		OpenRouterAPIKey: getEnv("OPENROUTER_API_KEY", ""),
		GeminiAPIKey:     geminiKey,
		AllowedUserID:    getEnv("ALLOWED_USER_ID", ""),
		DefaultModel:     defaultModel,
		GoogleModel:      googleModel,
		OpenRouterModel:  openRouterModel,
		DefaultProvider:  defaultProvider,
	}
}

func (c *Config) Validate() error {
	if c.TelegramToken == "" {
		return fmt.Errorf("TELEGRAM_TOKEN zorunludur ancak tanımlı değil")
	}
	if c.GeminiAPIKey == "" && c.OpenRouterAPIKey == "" {
		return fmt.Errorf("En az bir AI API anahtarı (GEMINI_API_KEY veya OPENROUTER_API_KEY) tanımlanmalıdır")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func loadDotEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}
