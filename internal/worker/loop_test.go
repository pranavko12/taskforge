package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoopGracefulShutdownFinishesCurrentJob(t *testing.T) {
	store := newFakeLeaseStore()
	loop := NewLoop(store, "jobs:ready", "lease-1", 40*time.Millisecond)
	loop.pollInterval = 5 * time.Millisecond

	started := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	runCtx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = loop.Run(runCtx, func(ctx context.Context, jobID string) error {
			started <- struct{}{}
			time.Sleep(30 * time.Millisecond)
			done <- struct{}{}
			return nil
		})
	}()

	<-started
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected in-flight job to finish during graceful shutdown")
	}
}

func TestProcessOneCrashSimulationCancelContextMidJob(t *testing.T) {
	store := newFakeLeaseStore()
	now := time.Now().UTC()
	ok, err := store.AcquireLease(context.Background(), "job-1", "lease-1", now, 30*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire lease failed: %v ok=%v", err, ok)
	}

	loop := NewLoop(store, "jobs:ready", "lease-1", 30*time.Millisecond)
	execCtx, cancel := context.WithCancel(context.Background())

	var renewBefore int64
	go func() {
		time.Sleep(40 * time.Millisecond)
		renewBefore = atomic.LoadInt64(&store.renewCount)
		cancel()
	}()

	err = loop.ProcessOne(execCtx, "job-1", func(ctx context.Context, jobID string) error {
		<-ctx.Done()
		return context.Canceled
	})
	if err != nil {
		t.Fatalf("process one returned error: %v", err)
	}
	if store.failedCount != 1 {
		t.Fatalf("expected failedCount=1, got %d", store.failedCount)
	}

	time.Sleep(80 * time.Millisecond)
	renewAfter := atomic.LoadInt64(&store.renewCount)
	if renewAfter > renewBefore+1 {
		t.Fatalf("expected heartbeat to stop after cancel, renewBefore=%d renewAfter=%d", renewBefore, renewAfter)
	}
}

func TestProcessOneSucceedFailTransitions(t *testing.T) {
	store := newFakeLeaseStore()
	now := time.Now().UTC()
	ok, err := store.AcquireLease(context.Background(), "job-1", "lease-1", now, 50*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire lease failed: %v ok=%v", err, ok)
	}
	loop := NewLoop(store, "jobs:ready", "lease-1", 50*time.Millisecond)

	if err := loop.ProcessOne(context.Background(), "job-1", func(ctx context.Context, jobID string) error { return nil }); err != nil {
		t.Fatalf("success path failed: %v", err)
	}
	if store.succeededCount != 1 {
		t.Fatalf("expected succeededCount=1 got %d", store.succeededCount)
	}

	store.state = "PENDING"
	store.owner = ""
	store.expiresAt = time.Time{}
	ok, err = store.AcquireLease(context.Background(), "job-1", "lease-1", time.Now().UTC(), 50*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("second acquire lease failed: %v ok=%v", err, ok)
	}
	if err := loop.ProcessOne(context.Background(), "job-1", func(ctx context.Context, jobID string) error { return errors.New("boom") }); err != nil {
		t.Fatalf("failure path failed: %v", err)
	}
	if store.failedCount != 1 {
		t.Fatalf("expected failedCount=1 got %d", store.failedCount)
	}
}

