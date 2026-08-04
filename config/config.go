package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	TelegramToken     string
	OpenRouterAPIKey  string
	GeminiAPIKey      string
	AllowedUserID     string
	AllowedUserIDs    []int64
	AllowAllUsers     bool
	DefaultModel      string
	GoogleModel       string
	OpenRouterModel   string
	DefaultProvider   string
	MaxConcurrentJobs int
}

func LoadConfig() *Config {
	loadDotEnv(".env")

	defaultModel := getEnv("MODEL", "gemini-3.6-flash")
	googleModel := getEnv("GOOGLE_MODEL", defaultModel)
	openRouterModel := getEnv("OPENROUTER_MODEL", "google/gemini-3.6-flash")

	geminiKey := getEnv("GEMINI_API_KEY", getEnv("GOOGLE_API_KEY", ""))
	defaultProvider := strings.ToLower(getEnv("DEFAULT_PROVIDER", getEnv("PROVIDER", "google")))

	allowedRaw := getEnv("ALLOWED_USER_ID", "")
	maxJobs := 5
	if val := getEnv("MAX_CONCURRENT_JOBS", ""); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			maxJobs = parsed
		} else {
			maxJobs = 5
		}
	}

	return &Config{
		TelegramToken:     getEnv("TELEGRAM_TOKEN", ""),
		OpenRouterAPIKey:  getEnv("OPENROUTER_API_KEY", ""),
		GeminiAPIKey:      geminiKey,
		AllowedUserID:     allowedRaw,
		AllowedUserIDs:    parseUserIDs(allowedRaw),
		AllowAllUsers:     strings.EqualFold(getEnv("ALLOW_ALL_USERS", ""), "true"),
		DefaultModel:      defaultModel,
		GoogleModel:       googleModel,
		OpenRouterModel:   openRouterModel,
		DefaultProvider:   defaultProvider,
		MaxConcurrentJobs: maxJobs,
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
			// Strip trailing inline comments, but only when unquoted: a '#' inside
			// quotes can be a legitimate part of a token or password.
			if !strings.HasPrefix(val, `"`) && !strings.HasPrefix(val, "'") {
				if idx := strings.Index(val, " #"); idx >= 0 {
					val = strings.TrimSpace(val[:idx])
				}
			}
			val = strings.Trim(val, `"'`)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

// Redact replaces credentials with a placeholder. Telegram file URLs and the Gemini
// endpoint carry secrets in the URL itself, and net/http quotes the full URL in its
// error messages — without this, those errors reach chat messages and logs verbatim.
func (c *Config) Redact(s string) string {
	for _, secret := range []string{c.TelegramToken, c.GeminiAPIKey, c.OpenRouterAPIKey} {
		// Short values are skipped: an empty or 3-character secret would otherwise
		// replace unrelated substrings all over the message.
		if len(secret) >= 8 {
			s = strings.ReplaceAll(s, secret, "[REDACTED]")
		}
	}
	return s
}

// parseUserIDs reads a comma-separated ALLOWED_USER_ID list. Unparsable entries are
// dropped rather than failing startup: a typo in one ID must not lock the owner out
// of a bot that is otherwise correctly configured.
func parseUserIDs(raw string) []int64 {
	var ids []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
