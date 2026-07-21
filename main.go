package main

import (
	"log"

	"scribo/bot"
	"scribo/config"
	"scribo/mode"
)

func main() {
	log.Println("🚀 Scribo Bot (Go/Golang) başlatılıyor...")

	cfg := config.LoadConfig()

	// Load custom prompts and button labels from modes.json if present
	mode.LoadCustomModes("modes.json")

	runner, err := bot.NewBotRunner(cfg)
	if err != nil {
		log.Fatalf("❌ Bot başlatma hatası: %v", err)
	}

	log.Printf("⚙️ Yapılandırma: Model=%s, GoogleModel=%s, OpenRouterModel=%s",
		cfg.DefaultModel, cfg.GoogleModel, cfg.OpenRouterModel)

	if err := runner.StartPolling(); err != nil {
		log.Fatalf("❌ Bot polling hatası: %v", err)
	}
}
