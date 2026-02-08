package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/pranavko12/taskforge/internal/config"
	"github.com/pranavko12/taskforge/internal/telemetry"
	"github.com/pranavko12/taskforge/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	shutdown, err := telemetry.Init(context.Background(), cfg, "taskforge-worker")
	if err != nil {
		slog.Error("tracing init error", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()

	throttler := worker.NewThrottler(cfg.QueueName, cfg.WorkerConcurrency, cfg.RateLimitPerSec)
	_ = worker.NewRunner(cfg.QueueName, throttler, nil)

	// Placeholder: actual job polling/processing should use Runner.Execute to enforce limits.
	select {}
}
