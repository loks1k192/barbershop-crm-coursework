// Package config читает конфигурацию сервиса из переменных окружения.
// Секреты (строки подключения с паролями) в исходном коде не хранятся.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr           string        // CORE_HTTP_ADDR
	DatabaseURL        string        // CORE_DB_URL — роль crm_app
	MaxConns           int32         // CORE_DB_MAX_CONNS
	KPIRefreshInterval time.Duration // CORE_KPI_REFRESH_INTERVAL (0 — выключено)
	LogLevel           string        // CORE_LOG_LEVEL
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:    env("CORE_HTTP_ADDR", ":8090"),
		DatabaseURL: os.Getenv("CORE_DB_URL"),
		LogLevel:    env("CORE_LOG_LEVEL", "info"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("CORE_DB_URL is required")
	}
	n, err := strconv.Atoi(env("CORE_DB_MAX_CONNS", "10"))
	if err != nil || n < 1 {
		return nil, fmt.Errorf("CORE_DB_MAX_CONNS must be a positive integer")
	}
	cfg.MaxConns = int32(n)
	cfg.KPIRefreshInterval, err = time.ParseDuration(env("CORE_KPI_REFRESH_INTERVAL", "10m"))
	if err != nil {
		return nil, fmt.Errorf("CORE_KPI_REFRESH_INTERVAL: %w", err)
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
