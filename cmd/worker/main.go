package main

import (
	"log/slog"
	"os"

	"github.com/pranavko12/taskforge/internal/config"
	"github.com/pranavko12/taskforge/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	_ = worker.NewRunner(worker.NewThrottler(cfg.WorkerConcurrency, cfg.RateLimitPerSec))

	// Placeholder: actual job polling/processing should use Runner.Execute to enforce limits.
	select {}
}
