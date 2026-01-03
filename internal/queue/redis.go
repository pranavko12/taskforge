package queue

import (
	"context"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	Client *redis.Client
}

func NewRedis() *Redis {
	addr := os.Getenv("REDIS_ADDR")
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
		MinIdleConns: 2,
	})
	return &Redis{Client: rdb}
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}
