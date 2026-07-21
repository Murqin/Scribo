package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	TelegramToken    string
	OpenRouterAPIKey string
	GeminiAPIKey     string
	AllowedUserID    string
	WebhookSecret    string
	DefaultModel     string
	GoogleModel      string
	OpenRouterModel  string
}

func LoadConfig() *Config {
	loadDotEnv(".env")

	defaultModel := getEnv("MODEL", getEnv("GEMINI_MODEL", "gemini-3.6-flash"))

	googleModel := getEnv("GOOGLE_MODEL", "")
	if googleModel == "" {
		googleModel = strings.TrimPrefix(defaultModel, "google/")
	} else {
		googleModel = strings.TrimPrefix(googleModel, "google/")
	}

	openRouterModel := getEnv("OPENROUTER_MODEL", "")
	if openRouterModel == "" {
		if !strings.HasPrefix(defaultModel, "google/") {
			openRouterModel = "google/" + defaultModel
		} else {
			openRouterModel = defaultModel
		}
	}

	geminiKey := getEnv("GEMINI_API_KEY", getEnv("GOOGLE_API_KEY", ""))

	return &Config{
		TelegramToken:    getEnv("TELEGRAM_TOKEN", ""),
		OpenRouterAPIKey: getEnv("OPENROUTER_API_KEY", ""),
		GeminiAPIKey:     geminiKey,
		AllowedUserID:    getEnv("ALLOWED_USER_ID", ""),
		WebhookSecret:    getEnv("WEBHOOK_SECRET", ""),
		DefaultModel:     defaultModel,
		GoogleModel:      googleModel,
		OpenRouterModel:  openRouterModel,
	}
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
