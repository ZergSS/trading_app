package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	FinamToken       string
	DBPath           string
	UpdateInterval   int
	VolatilityPeriod int
}

func Load(path string) (*Config, error) {
	if err := godotenv.Load(path); err != nil {
		return nil, err
	}

	return &Config{
		FinamToken:       os.Getenv("FINAM_TOKEN"),
		DBPath:           os.Getenv("DB_PATH"),
		UpdateInterval:   getInt("UPDATE_INTERVAL", 60),
		VolatilityPeriod: getInt("VOLATILITY_PERIOD", 5),
	}, nil
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var result int
	if _, err := fmt.Sscanf(v, "%d", &result); err != nil {
		return def
	}
	return result
}
