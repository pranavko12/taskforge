package api

import "context"

type Queue interface {
	Ping(ctx context.Context) error
	Enqueue(ctx context.Context, queueName string, jobID string) error
}
