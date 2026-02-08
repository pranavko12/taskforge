package scheduler

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/pranavko12/taskforge/internal/retry"
	"github.com/pranavko12/taskforge/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var ErrMaxAttemptsExceeded = errors.New("max attempts exceeded")

type RetryJob struct {
	JobID             string
	RetryCount        int
	MaxAttempts       int
	InitialDelayMs    int
	BackoffMultiplier float64
	MaxDelayMs        int
	Jitter            float64
	Traceparent       string
}

type Store interface {
	GetRetryJob(ctx context.Context, jobID string) (RetryJob, error)
	UpdateRetrySchedule(ctx context.Context, jobID string, retryCount int, nextRunAt time.Time) error
	ListDueRetries(ctx context.Context, now time.Time, limit int) ([]string, error)
	MarkRetryEnqueued(ctx context.Context, jobID string) error
	MarkTerminalFailure(ctx context.Context, jobID string, reason string) error
}

type Queue interface {
	Enqueue(ctx context.Context, queueName string, jobID string) error
}

type Scheduler struct {
	store     Store
	queue     Queue
	queueName string
	limit     int
}

func New(store Store, queue Queue, queueName string) *Scheduler {
	return &Scheduler{
		store:     store,
		queue:     queue,
		queueName: queueName,
		limit:     100,
	}
}

// ScheduleRetry computes the next run time for a retry and persists it.
func (s *Scheduler) ScheduleRetry(ctx context.Context, jobID string, now time.Time, seed int64) (time.Time, error) {
	job, err := s.store.GetRetryJob(ctx, jobID)
	if err != nil {
		return time.Time{}, err
	}

	spanCtx := telemetry.ContextWithTraceparent(job.Traceparent)
	tracer := otel.Tracer("taskforge/scheduler")
	spanCtx, span := tracer.Start(spanCtx, "schedule_retry",
		attribute.String("job_id", jobID),
		attribute.String("queue", s.queueName),
	)
	defer span.End()

	nextRetryCount := job.RetryCount + 1
	if job.MaxAttempts > 0 && nextRetryCount >= job.MaxAttempts {
		if err := s.store.MarkTerminalFailure(ctx, jobID, "max attempts exceeded"); err != nil {
			return time.Time{}, err
		}
		return time.Time{}, ErrMaxAttemptsExceeded
	}

	policy := retry.Policy{
		MaxAttempts:       job.MaxAttempts,
		InitialDelay:      time.Duration(job.InitialDelayMs) * time.Millisecond,
		BackoffMultiplier: job.BackoffMultiplier,
		MaxDelay:          time.Duration(job.MaxDelayMs) * time.Millisecond,
		Jitter:            job.Jitter,
	}
	if err := policy.Validate(); err != nil {
		return time.Time{}, err
	}

	rng := rand.New(rand.NewSource(seed))
	nextRunAt := retry.NextRunAt(now, nextRetryCount, policy, rng)
	if err := s.store.UpdateRetrySchedule(ctx, jobID, nextRetryCount, nextRunAt); err != nil {
		return time.Time{}, err
	}
	return nextRunAt, nil
}

// EnqueueDueRetries requeues retryable jobs whose next_run_at has passed.
func (s *Scheduler) EnqueueDueRetries(ctx context.Context, now time.Time) (int, error) {
	ids, err := s.store.ListDueRetries(ctx, now, s.limit)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := s.queue.Enqueue(ctx, s.queueName, id); err != nil {
			return 0, err
		}
		if err := s.store.MarkRetryEnqueued(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}
