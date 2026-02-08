package worker

import (
	"context"
	"time"

	"github.com/pranavko12/taskforge/internal/metrics"
)

type Runner struct {
	throttler *Throttler
	queueName string
	store     LeaseStore
}

func NewRunner(queueName string, throttler *Throttler, store LeaseStore) *Runner {
	return &Runner{queueName: queueName, throttler: throttler, store: store}
}

func (r *Runner) Execute(ctx context.Context, fn func(context.Context) error) error {
	if r.throttler != nil {
		if err := r.throttler.Acquire(ctx); err != nil {
			return err
		}
		defer r.throttler.Release()
	}
	metrics.IncAttempts(r.queueName)
	start := time.Now()
	err := fn(ctx)
	metrics.ObserveRuntime(r.queueName, time.Since(start).Seconds())
	if err != nil {
		metrics.IncFailure(r.queueName)
		return err
	}
	metrics.IncSuccess(r.queueName)
	return nil
}

func (r *Runner) ExecuteWithQueueTime(ctx context.Context, timeInQueue time.Duration, fn func(context.Context) error) error {
	metrics.ObserveTimeInQueue(r.queueName, timeInQueue.Seconds())
	return r.Execute(ctx, fn)
}

func (r *Runner) ExecuteJob(ctx context.Context, jobID string, timeInQueue time.Duration, fn func(context.Context) error) error {
	traceparent := ""
	if r.store != nil {
		if tp, err := r.store.GetTraceparent(ctx, jobID); err == nil {
			traceparent = tp
		}
	}
	traceCtx, end := StartJobSpan(jobID, r.queueName, traceparent)
	defer end()
	metrics.ObserveTimeInQueue(r.queueName, timeInQueue.Seconds())
	return r.Execute(traceCtx, fn)
}
