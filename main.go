package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"scribo/bot"
	"scribo/config"
	"scribo/mode"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("🚀 Scribo Bot (Go/Golang) başlatılıyor...")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.LoadConfig()

	// Load custom prompts and button labels from modes.json if present
	mode.LoadCustomModes("modes.json")

	runner, err := bot.NewBotRunner(cfg)
	if err != nil {
		slog.Error("❌ Bot başlatma hatası", "error", err)
		os.Exit(1)
	}

	slog.Info("⚙️ Yapılandırma yüklendi",
		"GoogleModel", cfg.GoogleModel,
		"OpenRouterModel", cfg.OpenRouterModel,
		"DefaultProvider", cfg.DefaultProvider,
	)

	if err := runner.StartPolling(ctx); err != nil {
		slog.Error("❌ Bot polling hatası", "error", err)
		os.Exit(1)
	}

	slog.Info("👋 Scribo Bot temiz bir şekilde kapatıldı.")
}
