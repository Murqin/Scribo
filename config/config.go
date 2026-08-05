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
	DailyCostLimit    float64
	MonthlyCostLimit  float64
	HistoryFile       string

	// costLimitErr defers a malformed spending limit to Validate so startup
	// fails loudly instead of running without the ceiling that was asked for.
	costLimitErr error
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

	dailyLimit, dailyErr := parseCostLimit("DAILY_COST_LIMIT")
	monthlyLimit, monthlyErr := parseCostLimit("MONTHLY_COST_LIMIT")
	costLimitErr := dailyErr
	if costLimitErr == nil {
		costLimitErr = monthlyErr
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
		DailyCostLimit:    dailyLimit,
		MonthlyCostLimit:  monthlyLimit,
		HistoryFile:       historyFile(),
		costLimitErr:      costLimitErr,
	}
}

// defaultHistoryFile is where transcripts land when HISTORY_FILE says nothing.
const defaultHistoryFile = "scribo_history.jsonl"

// historyFile reads where transcripts are persisted. Unlike the other settings
// it cannot go through getEnv: an explicitly empty HISTORY_FILE means "keep no
// history", and getEnv would read that as "unset" and hand back the default.
func historyFile() string {
	if val, ok := os.LookupEnv("HISTORY_FILE"); ok {
		return strings.TrimSpace(val)
	}
	return defaultHistoryFile
}

// parseCostLimit reads a spending ceiling in USD. An unset or empty value means
// no ceiling; a malformed one is an error rather than a silent fallback,
// because quietly disabling a spending cap is exactly the failure this setting
// exists to prevent.
func parseCostLimit(key string) (float64, error) {
	raw := getEnv(key, "")
	if raw == "" {
		return 0, nil
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil || val < 0 {
		return 0, fmt.Errorf("%s pozitif bir ondalık sayı olmalı (okunan: %q)", key, raw)
	}
	return val, nil
}

func (c *Config) Validate() error {
	if c.TelegramToken == "" {
		return fmt.Errorf("TELEGRAM_TOKEN zorunludur ancak tanımlı değil")
	}
	if c.GeminiAPIKey == "" && c.OpenRouterAPIKey == "" {
		return fmt.Errorf("En az bir AI API anahtarı (GEMINI_API_KEY veya OPENROUTER_API_KEY) tanımlanmalıdır")
	}
	return c.costLimitErr
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
