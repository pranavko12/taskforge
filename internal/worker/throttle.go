package worker

import (
	"context"
	"time"
)

type Throttler struct {
	sem        chan struct{}
	tokens     chan struct{}
	interval   time.Duration
	stopTokens chan struct{}
}

func NewThrottler(concurrency int, ratePerSec int) *Throttler {
	t := &Throttler{}
	if concurrency > 0 {
		t.sem = make(chan struct{}, concurrency)
		for i := 0; i < concurrency; i++ {
			t.sem <- struct{}{}
		}
	}
	if ratePerSec > 0 {
		t.tokens = make(chan struct{}, ratePerSec)
		for i := 0; i < ratePerSec; i++ {
			t.tokens <- struct{}{}
		}
		t.interval = time.Second / time.Duration(ratePerSec)
		t.stopTokens = make(chan struct{})
		go t.refillTokens()
	}
	return t
}

func (t *Throttler) refillTokens() {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			select {
			case t.tokens <- struct{}{}:
			default:
			}
		case <-t.stopTokens:
			return
		}
	}
}

func (t *Throttler) Close() {
	if t.stopTokens != nil {
		close(t.stopTokens)
	}
}

func (t *Throttler) Acquire(ctx context.Context) error {
	if t.sem != nil {
		select {
		case <-t.sem:
		default:
			metricConcurrencyThrottled.Add(1)
			select {
			case <-t.sem:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	if t.tokens != nil {
		select {
		case <-t.tokens:
		default:
			metricRateThrottled.Add(1)
			select {
			case <-t.tokens:
			case <-ctx.Done():
				if t.sem != nil {
					t.sem <- struct{}{}
				}
				return ctx.Err()
			}
		}
	}
	return nil
}

func (t *Throttler) Release() {
	if t.sem != nil {
		t.sem <- struct{}{}
	}
}
