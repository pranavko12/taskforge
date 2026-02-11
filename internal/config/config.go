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
	TracingEnabled    bool
	TracingExporter   string
}

type Error struct {
	Issues []string
}

func (e *Error) Error() string {
	return "invalid config: " + strings.Join(e.Issues, "; ")
}

func Load() (Config, error) {
	var issues []string

	redisDB, err := getEnvInt("REDIS_DB", 0)
	if err != nil {
		issues = append(issues, err.Error())
	}
	workerConcurrency, err := getEnvInt("WORKER_CONCURRENCY", 10)
	if err != nil {
		issues = append(issues, err.Error())
	}
	rateLimitPerSec, err := getEnvInt("RATE_LIMIT_PER_SEC", 0)
	if err != nil {
		issues = append(issues, err.Error())
	}
	tracingEnabled, err := getEnvBool("TRACING_ENABLED", false)
	if err != nil {
		issues = append(issues, err.Error())
	}

	cfg := Config{
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
		QueueName:         getEnv("QUEUE_NAME", "jobs:ready"),
		UIDir:             getEnv("UI_DIR", "./internal/api/ui"),
		LogLevel:          strings.ToLower(getEnv("LOG_LEVEL", "info")),
		PostgresDSN:       strings.TrimSpace(os.Getenv("POSTGRES_DSN")),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		RedisDB:           redisDB,
		WorkerConcurrency: workerConcurrency,
		RateLimitPerSec:   rateLimitPerSec,
		TracingEnabled:    tracingEnabled,
		TracingExporter:   strings.ToLower(getEnv("TRACING_EXPORTER", "stdout")),
	}

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
	if cfg.TracingExporter != "stdout" && cfg.TracingExporter != "none" {
		issues = append(issues, fmt.Sprintf("TRACING_EXPORTER must be stdout or none (got %q)", cfg.TracingExporter))
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

func getEnvInt(key string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer (got %q)", key, v)
	}
	return n, nil
}

func getEnvBool(key string, fallback bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a valid boolean (true/false, 1/0, yes/no; got %q)", key, v)
	}
}
