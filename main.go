package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"scribo/bot"
	"scribo/config"
	"scribo/i18n"
	"scribo/mode"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("🚀 Scribo bot starting...")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.LoadConfig()

	// The language has to be settled before the modes are: the mode package
	// initialises itself in the default language at import time, and both the
	// button labels and the prompts it hands to the model depend on this.
	lang := i18n.SetLanguage(cfg.Language)
	mode.LoadDefaultModes()

	// Load custom prompts and button labels from modes.json if present
	mode.LoadCustomModes("modes.json")

	runner, err := bot.NewBotRunner(cfg)
	if err != nil {
		slog.Error("❌ bot startup failed", "error", err)
		os.Exit(1)
	}

	slog.Info("⚙️ configuration loaded",
		"Language", lang,
		"GoogleModel", cfg.GoogleModel,
		"OpenRouterModel", cfg.OpenRouterModel,
		"DefaultProvider", cfg.DefaultProvider,
		"DailyCostLimit", cfg.DailyCostLimit,
		"MonthlyCostLimit", cfg.MonthlyCostLimit,
		"HistoryFile", cfg.HistoryFile,
	)

	if err := runner.StartPolling(ctx); err != nil {
		slog.Error("❌ bot polling failed", "error", err)
		os.Exit(1)
	}

	slog.Info("👋 Scribo bot shut down cleanly")
}
