package main

import (
	"log"
	"os"

	"trade_info/internal/config"
	"trade_info/internal/ui"
)

func main() {
	f, _ := os.OpenFile("debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	defer f.Close()
	log.SetOutput(f)

	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("ошибка загрузки конфигурации: %v", err)
	}

	app := ui.NewApp(cfg)
	if err := app.Run(); err != nil {
		log.Fatalf("ошибка запуска приложения: %v", err)
	}
}
