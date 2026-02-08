package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pranavko12/taskforge/internal/config"
	"github.com/pranavko12/taskforge/internal/queue"
	"github.com/pranavko12/taskforge/internal/scheduler"
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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	shutdown, err := telemetry.Init(ctx, cfg, "taskforge-scheduler")
	if err != nil {
		logger.Error("tracing init error", "err", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()

	pg, err := storage.NewPostgres(ctx, cfg)
	if err != nil {
		logger.Error("failed to connect postgres", "err", err)
		os.Exit(1)
	}
	defer pg.Pool.Close()

	rd := queue.NewRedis(cfg)
	if err := rd.Ping(ctx); err != nil {
		logger.Error("failed to connect redis", "err", err)
		os.Exit(1)
	}

	store := scheduler.NewPostgresStore(pg.Pool)
	s := scheduler.New(store, rd, cfg.QueueName)
	leaseStore := worker.NewPostgresStore(pg.Pool)
	reaper := worker.NewLeaseReaper(leaseStore, rd, cfg.QueueName)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := s.EnqueueDueRetries(ctx, time.Now().UTC()); err != nil {
				logger.Error("enqueue due retries failed", "err", err)
			}
			if _, err := reaper.RequeueExpiredLeases(ctx, time.Now().UTC()); err != nil {
				logger.Error("requeue expired leases failed", "err", err)
			}
		case <-stop:
			return
		}
	}
}
