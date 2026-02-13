package worker

import (
	"context"
	"time"
)

type LeaseStore interface {
	LeaseNextJob(ctx context.Context, queueName string, owner string, now time.Time, leaseFor time.Duration) (string, bool, error)
	AcquireLease(ctx context.Context, jobID string, owner string, now time.Time, leaseFor time.Duration) (bool, error)
	RenewLease(ctx context.Context, jobID string, leaseID string, extendBy time.Duration) (bool, error)
	MarkJobSucceeded(ctx context.Context, jobID string, leaseID string) (bool, error)
	MarkJobFailed(ctx context.Context, jobID string, leaseID string, lastError string) (bool, error)
	ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]string, error)
	ResetLease(ctx context.Context, jobID string) error
	GetTraceparent(ctx context.Context, jobID string) (string, error)
}

type Queue interface {
	Enqueue(ctx context.Context, queueName string, jobID string) error
}

type LeaseReaper struct {
	store     LeaseStore
	queue     Queue
	queueName string
	limit     int
}

func NewLeaseReaper(store LeaseStore, queue Queue, queueName string) *LeaseReaper {
	return &LeaseReaper{
		store:     store,
		queue:     queue,
		queueName: queueName,
		limit:     100,
	}
}

func (r *LeaseReaper) RequeueExpiredLeases(ctx context.Context, now time.Time) (int, error) {
	ids, err := r.store.ListExpiredLeases(ctx, now, r.limit)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := r.store.ResetLease(ctx, id); err != nil {
			return 0, err
		}
		if err := r.queue.Enqueue(ctx, r.queueName, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}
