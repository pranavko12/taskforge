package worker

import (
	"context"
)

type Runner struct {
	throttler *Throttler
}

func NewRunner(throttler *Throttler) *Runner {
	return &Runner{throttler: throttler}
}

func (r *Runner) Execute(ctx context.Context, fn func(context.Context) error) error {
	if r.throttler != nil {
		if err := r.throttler.Acquire(ctx); err != nil {
			return err
		}
		defer r.throttler.Release()
	}
	return fn(ctx)
}
