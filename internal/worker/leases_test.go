package worker

import (
	"context"
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

	ok, err = w1.Renew(context.Background(), jobID, now.Add(2*time.Second))
	if err != nil || !ok {
		t.Fatalf("worker-1 renew failed: %v ok=%v", err, ok)
	}

	ok, err = w2.Renew(context.Background(), jobID, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("worker-2 renew error: %v", err)
	}
	if ok {
		t.Fatalf("worker-2 should not renew someone else's lease")
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

type fakeQueue struct {
	enqueued []string
}

func (q *fakeQueue) Enqueue(ctx context.Context, queueName string, jobID string) error {
	q.enqueued = append(q.enqueued, jobID)
	return nil
}

type fakeLeaseStore struct {
	owner     string
	expiresAt time.Time
}

func newFakeLeaseStore() *fakeLeaseStore {
	return &fakeLeaseStore{}
}

func (s *fakeLeaseStore) AcquireLease(ctx context.Context, jobID string, owner string, now time.Time, leaseFor time.Duration) (bool, error) {
	if s.owner != "" && s.expiresAt.After(now) {
		return false, nil
	}
	s.owner = owner
	s.expiresAt = now.Add(leaseFor)
	return true, nil
}

func (s *fakeLeaseStore) RenewLease(ctx context.Context, jobID string, owner string, now time.Time, leaseFor time.Duration) (bool, error) {
	if s.owner != owner {
		return false, nil
	}
	s.expiresAt = now.Add(leaseFor)
	return true, nil
}

func (s *fakeLeaseStore) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]string, error) {
	if s.owner != "" && !s.expiresAt.After(now) {
		return []string{"job-1"}, nil
	}
	return nil, nil
}

func (s *fakeLeaseStore) ResetLease(ctx context.Context, jobID string) error {
	s.owner = ""
	s.expiresAt = time.Time{}
	return nil
}
