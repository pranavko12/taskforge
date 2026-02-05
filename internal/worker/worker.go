package worker

import (
	"context"
	"time"
)

type Worker struct {
	store      LeaseStore
	owner      string
	leaseFor   time.Duration
	renewEvery time.Duration
}

func New(store LeaseStore, owner string, leaseFor time.Duration) *Worker {
	return &Worker{
		store:      store,
		owner:      owner,
		leaseFor:   leaseFor,
		renewEvery: leaseFor / 2,
	}
}

func (w *Worker) Acquire(ctx context.Context, jobID string, now time.Time) (bool, error) {
	return w.store.AcquireLease(ctx, jobID, w.owner, now, w.leaseFor)
}

func (w *Worker) Renew(ctx context.Context, jobID string, now time.Time) (bool, error) {
	return w.store.RenewLease(ctx, jobID, w.owner, now, w.leaseFor)
}

// Heartbeat renews the lease until ctx is cancelled.
func (w *Worker) Heartbeat(ctx context.Context, jobID string) error {
	ticker := time.NewTicker(w.renewEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := w.Renew(ctx, jobID, time.Now().UTC()); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}
