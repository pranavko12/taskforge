package queue

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"

	"github.com/pranavko12/taskforge/internal/config"
)

type Redis struct {
	Client *redis.Client
}

func NewRedis(cfg config.Config) *Redis {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})

	return &Redis{Client: client}
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}

func (r *Redis) Enqueue(ctx context.Context, queueName string, jobID string) error {
	return r.Client.LPush(ctx, queueName, jobID).Err()
}
