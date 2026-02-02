package scheduler

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/pranavko12/taskforge/internal/retry"
)

func TestScheduleRetrySetsNextRunAtDeterministically(t *testing.T) {
	now := time.Date(2026, 2, 2, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		job: RetryJob{
			JobID:             "job-1",
			RetryCount:        0,
			MaxAttempts:       5,
			InitialDelayMs:    1000,
			BackoffMultiplier: 2,
			MaxDelayMs:        60000,
			Jitter:            0.25,
		},
	}
	q := &fakeQueue{}
	s := New(store, q, "jobs:ready")

	seed := int64(42)
	got, err := s.ScheduleRetry(context.Background(), "job-1", now, seed)
	if err != nil {
		t.Fatalf("ScheduleRetry error: %v", err)
	}

	policy := retry.Policy{
		MaxAttempts:       5,
		InitialDelay:      1 * time.Second,
		BackoffMultiplier: 2,
		MaxDelay:          60 * time.Second,
		Jitter:            0.25,
	}
	want := retry.NextRunAt(now, 1, policy, randFromSeed(seed))
	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if store.updateNextRunAt.IsZero() {
		t.Fatalf("expected store to be updated")
	}
	if !store.updateNextRunAt.Equal(want) {
		t.Fatalf("store next_run_at mismatch: %v", store.updateNextRunAt)
	}
	if store.updateRetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", store.updateRetryCount)
	}
}

func TestScheduleRetryStopsAtMaxAttempts(t *testing.T) {
	store := &fakeStore{
		job: RetryJob{
			JobID:       "job-1",
			RetryCount:  2,
			MaxAttempts: 3,
		},
	}
	s := New(store, &fakeQueue{}, "jobs:ready")
	_, err := s.ScheduleRetry(context.Background(), "job-1", time.Now(), 1)
	if !errors.Is(err, ErrMaxAttemptsExceeded) {
		t.Fatalf("expected max attempts error, got %v", err)
	}
}

func TestEnqueueDueRetries(t *testing.T) {
	store := &fakeStore{due: []string{"job-1", "job-2"}}
	q := &fakeQueue{}
	s := New(store, q, "jobs:ready")

	n, err := s.EnqueueDueRetries(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("EnqueueDueRetries error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
	if len(q.enqueued) != 2 {
		t.Fatalf("expected 2 enqueued, got %d", len(q.enqueued))
	}
	if len(store.marked) != 2 {
		t.Fatalf("expected 2 marked, got %d", len(store.marked))
	}
}

func randFromSeed(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

type fakeStore struct {
	job              RetryJob
	updateRetryCount int
	updateNextRunAt  time.Time
	due              []string
	marked           []string
}

func (f *fakeStore) GetRetryJob(ctx context.Context, jobID string) (RetryJob, error) {
	return f.job, nil
}

func (f *fakeStore) UpdateRetrySchedule(ctx context.Context, jobID string, retryCount int, nextRunAt time.Time) error {
	f.updateRetryCount = retryCount
	f.updateNextRunAt = nextRunAt
	return nil
}

func (f *fakeStore) ListDueRetries(ctx context.Context, now time.Time, limit int) ([]string, error) {
	return f.due, nil
}

func (f *fakeStore) MarkRetryEnqueued(ctx context.Context, jobID string) error {
	f.marked = append(f.marked, jobID)
	return nil
}

type fakeQueue struct {
	enqueued []string
}

func (f *fakeQueue) Enqueue(ctx context.Context, queueName string, jobID string) error {
	f.enqueued = append(f.enqueued, jobID)
	return nil
}
