package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pranavko12/taskforge/internal/api"
	"github.com/pranavko12/taskforge/internal/queue"
	"github.com/pranavko12/taskforge/internal/storage"
)

func main() {
	ctx := context.Background()

	pg, err := storage.NewPostgres(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pg.Pool.Close()

	rd := queue.NewRedis()
	if err := rd.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	srv := api.NewServer(pg.Pool, rd.Client)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
