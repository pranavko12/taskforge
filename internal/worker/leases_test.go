package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireLeaseExclusive(t *testing.T) {
	store := newFakeLeaseStore()
	jobID := "job-1"
	now := time.Now().UTC()

	w1 := New(store, "worker-1", 10*time.Second)
	w2 := New(store, "worker-2", 10*time.Second)

	ok, err := w1.Acquire(context.Background(), jobID, now)
	if err != nil || !ok {
		t.Fatalf("worker-1 acquire failed: %v ok=%v", err, ok)
	}
	ok, err = w2.Acquire(context.Background(), jobID, now)
	if err != nil {
		t.Fatalf("worker-2 acquire error: %v", err)
	}
	if ok {
		t.Fatalf("expected worker-2 acquire to fail while lease held")
	}
}

func TestRenewLeaseKeepsOwnership(t *testing.T) {
	store := newFakeLeaseStore()
	jobID := "job-1"
	now := time.Now().UTC()

	w1 := New(store, "worker-1", 5*time.Second)
	w2 := New(store, "worker-2", 5*time.Second)

	ok, err := w1.Acquire(context.Background(), jobID, now)
	if err != nil || !ok {
		t.Fatalf("worker-1 acquire failed: %v ok=%v", err, ok)
	}

	ok, err = w1.Renew(context.Background(), jobID)
	if err != nil || !ok {
		t.Fatalf("worker-1 renew failed: %v ok=%v", err, ok)
	}

	ok, err = w2.Renew(context.Background(), jobID)
	if err != nil {
		t.Fatalf("worker-2 renew error: %v", err)
	}
	if ok {
		t.Fatalf("worker-2 should not renew someone else's lease")
	}
}

func TestRenewLeaseFailsWithWrongLeaseID(t *testing.T) {
	store := newFakeLeaseStore()
	jobID := "job-1"
	now := time.Now().UTC()

	ok, err := store.AcquireLease(context.Background(), jobID, "lease-1", now, 5*time.Second)
	if err != nil || !ok {
		t.Fatalf("initial acquire failed: %v ok=%v", err, ok)
	}

	ok, err = store.RenewLease(context.Background(), jobID, "wrong-lease-id", 5*time.Second)
	if err != nil {
		t.Fatalf("renew returned error: %v", err)
	}
	if ok {
		t.Fatal("expected renew to fail with wrong lease_id")
	}
}

func TestRequeueExpiredLease(t *testing.T) {
	store := newFakeLeaseStore()
	queue := &fakeQueue{}
	reaper := NewLeaseReaper(store, queue, "jobs:ready")

	jobID := "job-1"
	now := time.Now().UTC()

	w1 := New(store, "worker-1", 1*time.Second)
	ok, err := w1.Acquire(context.Background(), jobID, now)
	if err != nil || !ok {
		t.Fatalf("worker-1 acquire failed: %v ok=%v", err, ok)
	}

	expiredAt := now.Add(2 * time.Second)
	n, err := reaper.RequeueExpiredLeases(context.Background(), expiredAt)
	if err != nil {
		t.Fatalf("reaper error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 requeued, got %d", n)
	}
	if len(queue.enqueued) != 1 || queue.enqueued[0] != jobID {
		t.Fatalf("expected job requeued")
	}

	// Now another worker should be able to acquire.
	w2 := New(store, "worker-2", 1*time.Second)
	ok, err = w2.Acquire(context.Background(), jobID, expiredAt)
	if err != nil || !ok {
		t.Fatalf("worker-2 acquire failed after requeue: %v ok=%v", err, ok)
	}
}

func TestLeaseNextJobConcurrentNoDoubleLease(t *testing.T) {
	store := newFakeLeaseStore()
	now := time.Now().UTC()
	leaseFor := 10 * time.Second
	workers := 8

	results := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			jobID, ok, err := store.LeaseNextJob(context.Background(), "jobs:ready", "worker", now, leaseFor)
			if err != nil {
				t.Errorf("lease next error: %v", err)
				return
			}
			if ok {
				results <- jobID
			}
		}(i)
	}
	wg.Wait()
	close(results)

	var leased []string
	for id := range results {
		leased = append(leased, id)
	}
	if len(leased) != 1 {
		t.Fatalf("expected exactly one successful lease, got %d (%v)", len(leased), leased)
	}
	if leased[0] != "job-1" {
		t.Fatalf("expected leased job-1, got %q", leased[0])
	}
}

type fakeQueue struct {
	enqueued []string
}

func (q *fakeQueue) Enqueue(ctx context.Context, queueName string, jobID string) error {
	q.enqueued = append(q.enqueued, jobID)
	return nil
}

type fakeLeaseStore struct {
	mu             sync.Mutex
	jobID          string
	queueName      string
	state          string
	owner          string
	expiresAt      time.Time
	renewCount     int64
	succeededCount int
	failedCount    int
	terminalCount  int
}

func newFakeLeaseStore() *fakeLeaseStore {
	return &fakeLeaseStore{
		jobID:     "job-1",
		queueName: "jobs:ready",
		state:     "PENDING",
	}
}

func (s *fakeLeaseStore) LeaseNextJob(ctx context.Context, queueName string, owner string, now time.Time, leaseFor time.Duration) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if queueName != s.queueName {
		return "", false, nil
	}
	if s.state != "PENDING" {
		return "", false, nil
	}
	if s.owner != "" && s.expiresAt.After(now) {
		return "", false, nil
	}

	s.owner = owner
	s.expiresAt = now.Add(leaseFor)
	s.state = "IN_PROGRESS"
	return s.jobID, true, nil
}

func (s *fakeLeaseStore) AcquireLease(ctx context.Context, jobID string, owner string, now time.Time, leaseFor time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.owner != "" && s.expiresAt.After(now) {
		return false, nil
	}
	s.owner = owner
	s.expiresAt = now.Add(leaseFor)
	s.state = "IN_PROGRESS"
	return true, nil
}

func (s *fakeLeaseStore) RenewLease(ctx context.Context, jobID string, leaseID string, extendBy time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.owner != leaseID {
		return false, nil
	}
	s.expiresAt = time.Now().UTC().Add(extendBy)
	atomic.AddInt64(&s.renewCount, 1)
	return true, nil
}

func (s *fakeLeaseStore) MarkJobSucceeded(ctx context.Context, jobID string, leaseID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jobID != jobID || s.owner != leaseID || s.state != "IN_PROGRESS" {
		return false, nil
	}
	s.state = "COMPLETED"
	s.owner = ""
	s.expiresAt = time.Time{}
	s.succeededCount++
	return true, nil
}

func (s *fakeLeaseStore) MarkJobFailed(ctx context.Context, jobID string, leaseID string, lastError string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jobID != jobID || s.owner != leaseID || s.state != "IN_PROGRESS" {
		return false, nil
	}
	s.state = "FAILED"
	s.owner = ""
	s.expiresAt = time.Time{}
	s.failedCount++
	return true, nil
}

func (s *fakeLeaseStore) MarkJobTerminal(ctx context.Context, jobID string, leaseID string, lastError string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jobID != jobID || s.owner != leaseID || s.state != "IN_PROGRESS" {
		return false, nil
	}
	s.state = "DLQ"
	s.owner = ""
	s.expiresAt = time.Time{}
	s.terminalCount++
	return true, nil
}

func (s *fakeLeaseStore) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.owner != "" && !s.expiresAt.After(now) {
		return []string{s.jobID}, nil
	}
	return nil, nil
}

func (s *fakeLeaseStore) ResetLease(ctx context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.owner = ""
	s.expiresAt = time.Time{}
	s.state = "PENDING"
	return nil
}

func (s *fakeLeaseStore) GetTraceparent(ctx context.Context, jobID string) (string, error) {
	return "", nil
}
