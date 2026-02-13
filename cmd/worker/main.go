package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pranavko12/taskforge/internal/config"
	"github.com/pranavko12/taskforge/internal/storage"
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

	pg, err := storage.NewPostgres(context.Background(), cfg)
	if err != nil {
		slog.Error("postgres init error", "err", err)
		os.Exit(1)
	}
	defer pg.Pool.Close()

	leaseStore := worker.NewPostgresStore(pg.Pool)
	leaseFor := 30 * time.Second
	leaseID := "worker-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	loop := worker.NewLoop(leaseStore, cfg.QueueName, leaseID, leaseFor)
	runner := worker.NewRunner(cfg.QueueName, worker.NewThrottler(cfg.QueueName, cfg.WorkerConcurrency, cfg.RateLimitPerSec), leaseStore)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	execute := func(execCtx context.Context, jobID string) error {
		return runner.ExecuteJob(execCtx, jobID, 0, func(context.Context) error {
			// Placeholder: plug real job-type dispatch here.
			return nil
		})
	}

	if err := loop.Run(ctx, execute); err != nil {
		slog.Error("worker loop stopped with error", "err", err)
		os.Exit(1)
	}
}
