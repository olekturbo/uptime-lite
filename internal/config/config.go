package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration resolved from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
	RedisURL    string
	WorkerCount int
}

// Load reads configuration from the environment and applies sane defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/uptime_lite?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		WorkerCount: 10,
	}

	if raw := os.Getenv("WORKER_COUNT"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("WORKER_COUNT must be a positive integer, got %q", raw)
		}
		cfg.WorkerCount = n
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
