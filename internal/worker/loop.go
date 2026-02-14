package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/pranavko12/taskforge/internal/retry"
)

type ExecuteFunc func(ctx context.Context, jobID string) error

type Loop struct {
	worker       *Worker
	store        LeaseStore
	queueName    string
	pollInterval time.Duration
}

func NewLoop(store LeaseStore, queueName string, leaseID string, leaseFor time.Duration) *Loop {
	return &Loop{
		worker:       New(store, leaseID, leaseFor),
		store:        store,
		queueName:    queueName,
		pollInterval: 100 * time.Millisecond,
	}
}

func (l *Loop) Run(ctx context.Context, execute ExecuteFunc) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		jobID, ok, err := l.worker.LeaseNext(ctx, l.queueName, time.Now().UTC())
		if err != nil {
			return err
		}
		if !ok {
			timer := time.NewTimer(l.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			continue
		}

		// Graceful shutdown: once leased, finish the current job even if run context is canceled.
		runCtx := context.WithoutCancel(ctx)
		if err := l.ProcessOne(runCtx, jobID, execute); err != nil {
			return err
		}
	}
}

func (l *Loop) ProcessOne(ctx context.Context, jobID string, execute ExecuteFunc) error {
	hbCtx, stopHeartbeat := context.WithCancel(context.Background())
	hbDone := make(chan error, 1)
	go func() {
		hbDone <- l.worker.Heartbeat(hbCtx, jobID)
	}()

	runErr := execute(ctx, jobID)

	stopHeartbeat()
	hbErr := <-hbDone
	if hbErr != nil {
		return hbErr
	}

	if runErr != nil {
		if retry.ClassifyError(runErr) == retry.ClassRetryable {
			ok, err := l.store.MarkJobFailed(context.Background(), jobID, l.worker.leaseID, runErr.Error())
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("failed to mark job %s failed: lease mismatch or invalid state", jobID)
			}
			return nil
		}

		ok, err := l.store.MarkJobTerminal(context.Background(), jobID, l.worker.leaseID, runErr.Error())
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("failed to mark job %s terminal: lease mismatch or invalid state", jobID)
		}
		return nil
	}

	ok, err := l.store.MarkJobSucceeded(context.Background(), jobID, l.worker.leaseID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("failed to mark job %s succeeded: lease mismatch or invalid state", jobID)
	}
	return nil
}
