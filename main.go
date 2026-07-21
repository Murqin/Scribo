package main

import (
	"log"

	"scribo/bot"
	"scribo/config"
)

func main() {
	log.Println("🚀 Scribo Bot (Go/Golang) başlatılıyor...")

	cfg := config.LoadConfig()

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
