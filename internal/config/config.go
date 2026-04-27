package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const (
	defaultMainDomain = "localtest.me:6000"
	defaultListenAddr = "localhost:6000"
)

type Config struct {
	MainDomain string
	ListenAddr string
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		MainDomain: getEnv("TUNNELWAY_MAIN_DOMAIN", defaultMainDomain),
		ListenAddr: getEnv("TUNNELWAY_LISTEN_ADDR", defaultListenAddr),
	}
}

func getEnv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
