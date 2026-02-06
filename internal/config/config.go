package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr          string
	QueueName         string
	UIDir             string
	LogLevel          string
	PostgresDSN       string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	WorkerConcurrency int
	RateLimitPerSec   int
}

type Error struct {
	Issues []string
}

func (e *Error) Error() string {
	return "invalid config: " + strings.Join(e.Issues, "; ")
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
		QueueName:         getEnv("QUEUE_NAME", "jobs:ready"),
		UIDir:             getEnv("UI_DIR", "./internal/api/ui"),
		LogLevel:          strings.ToLower(getEnv("LOG_LEVEL", "info")),
		PostgresDSN:       strings.TrimSpace(os.Getenv("POSTGRES_DSN")),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		RedisDB:           getEnvInt("REDIS_DB", 0),
		WorkerConcurrency: getEnvInt("WORKER_CONCURRENCY", 10),
		RateLimitPerSec:   getEnvInt("RATE_LIMIT_PER_SEC", 0),
	}

	var issues []string
	if cfg.PostgresDSN == "" {
		issues = append(issues, "POSTGRES_DSN is required")
	}
	if cfg.HTTPAddr == "" {
		issues = append(issues, "HTTP_ADDR must not be empty")
	}
	if cfg.RedisAddr == "" {
		issues = append(issues, "REDIS_ADDR must not be empty")
	}
	if cfg.QueueName == "" {
		issues = append(issues, "QUEUE_NAME must not be empty")
	}
	if cfg.LogLevel != "debug" && cfg.LogLevel != "info" && cfg.LogLevel != "warn" && cfg.LogLevel != "error" {
		issues = append(issues, fmt.Sprintf("LOG_LEVEL must be one of debug, info, warn, error (got %q)", cfg.LogLevel))
	}
	if cfg.RedisDB < 0 {
		issues = append(issues, "REDIS_DB must be >= 0")
	}
	if cfg.WorkerConcurrency <= 0 {
		issues = append(issues, "WORKER_CONCURRENCY must be >= 1")
	}
	if cfg.RateLimitPerSec < 0 {
		issues = append(issues, "RATE_LIMIT_PER_SEC must be >= 0")
	}
	if len(issues) > 0 {
		return Config{}, &Error{Issues: issues}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
