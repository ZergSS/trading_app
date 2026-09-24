package main

import (
	"log"

	"trade_info/internal/config"
	"trade_info/internal/ui"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("ошибка загрузки конфигурации: %v", err)
	}

	app := ui.NewApp(cfg)
	if err := app.Run(); err != nil {
		log.Fatalf("ошибка запуска приложения: %v", err)
	}
}
