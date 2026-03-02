package config_test

import (
	"testing"

	"uptime-lite/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("WORKER_COUNT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.WorkerCount != 10 {
		t.Errorf("WorkerCount = %d, want 10", cfg.WorkerCount)
	}
	if cfg.DatabaseURL == "" {
		t.Error("DatabaseURL should have a default value")
	}
	if cfg.RedisURL == "" {
		t.Error("RedisURL should have a default value")
	}
}

func TestLoad_FromEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/test")
	t.Setenv("REDIS_URL", "redis://cache:6379/1")
	t.Setenv("WORKER_COUNT", "25")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.DatabaseURL != "postgres://user:pass@db:5432/test" {
		t.Errorf("DatabaseURL = %q, want different value", cfg.DatabaseURL)
	}
	if cfg.RedisURL != "redis://cache:6379/1" {
		t.Errorf("RedisURL = %q, want different value", cfg.RedisURL)
	}
	if cfg.WorkerCount != 25 {
		t.Errorf("WorkerCount = %d, want 25", cfg.WorkerCount)
	}
}

func TestLoad_InvalidWorkerCount(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"not a number", "abc"},
		{"zero", "0"},
		{"negative", "-5"},
		{"float", "1.5"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WORKER_COUNT", tc.value)
			_, err := config.Load()
			if err == nil {
				t.Errorf("expected error for WORKER_COUNT=%q, got nil", tc.value)
			}
		})
	}
}
