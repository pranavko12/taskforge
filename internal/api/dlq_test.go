package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDLQListAndInspect(t *testing.T) {
	now := time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC)
	store := fakeStore{
		dlqEntries: []DLQEntry{
			{JobID: "job-1", Reason: "failed", CreatedAt: now},
		},
		dlqTotal: 1,
		getDLQEntryResp: DLQEntry{JobID: "job-1", Reason: "failed", CreatedAt: now},
		getJobResp: JobStatusResponse{JobID: "job-1"},
	}
	q := &fakeQueue{}
	s := newTestServer(&store, q)

	listReq := httptest.NewRequest(http.MethodGet, "/dlq?limit=10", nil)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}
	var listResp DLQListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResp.Total != 1 || len(listResp.Items) != 1 {
		t.Fatalf("unexpected list response: %+v", listResp)
	}

	inspectReq := httptest.NewRequest(http.MethodGet, "/dlq/job-1", nil)
	inspectRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(inspectRec, inspectReq)
	if inspectRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", inspectRec.Code)
	}
	var inspectResp DLQInspectResponse
	if err := json.NewDecoder(inspectRec.Body).Decode(&inspectResp); err != nil {
		t.Fatalf("decode inspect response: %v", err)
	}
	if inspectResp.Entry.JobID != "job-1" {
		t.Fatalf("expected job-1, got %q", inspectResp.Entry.JobID)
	}
}

func TestDLQReplay(t *testing.T) {
	store := fakeStore{
		replayErr: nil,
	}
	q := &fakeQueue{}
	s := newTestServer(&store, q)

	req := httptest.NewRequest(http.MethodPost, "/dlq/job-1/replay", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if len(q.enqueued) != 1 {
		t.Fatalf("expected enqueue, got %d", len(q.enqueued))
	}
}

func TestDLQJobReason(t *testing.T) {
	store := fakeStore{dlqOK: true}
	q := &fakeQueue{}
	s := newTestServer(&store, q)
	body := `{"reason":"worker failed"}`
	req := httptest.NewRequest(http.MethodPost, "/jobs/job-1/dlq", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if !store.dlqCalled || store.dlqReason != "worker failed" {
		t.Fatalf("expected dlq reason to be recorded, got called=%v reason=%q", store.dlqCalled, store.dlqReason)
	}
}

// Extend fakeStore with DLQ methods for tests.
func (f fakeStore) InsertDLQEntry(ctx context.Context, jobID string, reason string) error {
	return nil
}

func (f fakeStore) ListDLQ(ctx context.Context, limit, offset int) ([]DLQEntry, int, error) {
	return f.dlqEntries, f.dlqTotal, nil
}

func (f fakeStore) GetDLQEntry(ctx context.Context, jobID string) (DLQEntry, error) {
	if f.getDLQEntryErr != nil {
		return DLQEntry{}, f.getDLQEntryErr
	}
	return f.getDLQEntryResp, nil
}

func (f fakeStore) ReplayDLQ(ctx context.Context, jobID string) error {
	return f.replayErr
}
