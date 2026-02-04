package api

import "context"

type Store interface {
	Ping(ctx context.Context) error
	InsertJob(ctx context.Context, jobID string, req SubmitJobRequest) error
	GetJob(ctx context.Context, jobID string) (JobStatusResponse, error)
	GetJobByIdempotencyKey(ctx context.Context, key string) (JobStatusResponse, error)
	QueryJobs(ctx context.Context, q JobsQuery) ([]JobStatusResponse, int, error)
	RetryJob(ctx context.Context, jobID string) (bool, error)
	DLQJob(ctx context.Context, jobID string, reason string) (bool, error)
	Stats(ctx context.Context) (StatsCounts, error)
}

type StatsCounts struct {
	Total   int
	Pending int
	Failed  int
	DLQ     int
}
