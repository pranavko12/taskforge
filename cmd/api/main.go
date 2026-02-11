package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pranavko12/taskforge/internal/api"
	"github.com/pranavko12/taskforge/internal/config"
	"github.com/pranavko12/taskforge/internal/queue"
	"github.com/pranavko12/taskforge/internal/storage"
	"github.com/pranavko12/taskforge/internal/telemetry"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	shutdown, err := telemetry.Init(ctx, cfg, "taskforge-api")
	if err != nil {
		log.Fatal(err)
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

	store := api.NewPostgresStore(pg.Pool)
	srv := api.NewServer(cfg, store, rd, nil, logger)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "err", err)
	}
}
