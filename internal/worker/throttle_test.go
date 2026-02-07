package worker

import (
	"context"
	"testing"
	"time"
)

func TestConcurrencyThrottleBlocks(t *testing.T) {
	tl := NewThrottler("jobs:ready", 1, 0)
	defer tl.Close()

	ctx := context.Background()
	if err := tl.Acquire(ctx); err != nil {
		t.Fatalf("acquire 1 failed: %v", err)
	}

	start := time.Now()
	done := make(chan struct{})
	go func() {
		_ = tl.Acquire(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	tl.Release()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("second acquire did not proceed after release")
	}

	if time.Since(start) < 50*time.Millisecond {
		t.Fatalf("expected throttled acquire to wait")
	}
	tl.Release()
}

func TestRateLimitBlocks(t *testing.T) {
	tl := NewThrottler("jobs:ready", 0, 1) // 1/s => ~1s interval
	defer tl.Close()

	ctx := context.Background()
	if err := tl.Acquire(ctx); err != nil {
		t.Fatalf("acquire 1 failed: %v", err)
	}

	start := time.Now()
	if err := tl.Acquire(ctx); err != nil {
		t.Fatalf("acquire 2 failed: %v", err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Fatalf("expected rate limit wait")
	}
}
