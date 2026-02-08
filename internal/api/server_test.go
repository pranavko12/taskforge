package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pranavko12/taskforge/internal/config"
)

func TestHealthzAlwaysOK(t *testing.T) {
	q := &fakeQueue{}
	s := newTestServer(&fakeStore{}, q)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyzDependsOnDeps(t *testing.T) {
	q := &fakeQueue{}
	s := newTestServer(&fakeStore{pingErr: errTest}, q)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	q = &fakeQueue{}
	s = newTestServer(&fakeStore{}, q)
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequestIDHeaderSet(t *testing.T) {
	q := &fakeQueue{}
	s := newTestServer(&fakeStore{}, q)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Header().Get(requestIDHeader) == "" {
		t.Fatalf("expected %s header to be set", requestIDHeader)
	}
}

func TestErrorShapeInvalidJSON(t *testing.T) {
	q := &fakeQueue{}
	s := newTestServer(&fakeStore{}, q)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString("{invalid"))
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_json")
}

func TestIdempotencyReturnsExistingJob(t *testing.T) {
	store := fakeStore{
		insertErr:     errUnique,
		getByKeyResp:  JobStatusResponse{JobID: "existing"},
	}
	q := &fakeQueue{}
	s := newTestServer(&store, q)

	body := `{"jobType":"demo","payload":{"a":1},"idempotencyKey":"abc"}`
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp SubmitJobResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.JobID != "existing" {
		t.Fatalf("expected existing job id, got %q", resp.JobID)
	}
}

func TestJobsListOK(t *testing.T) {
	store := fakeStore{
		queryJobsResp: []JobStatusResponse{{JobID: "job-1", JobType: "demo"}},
		queryJobsTotal: 1,
	}
	q := &fakeQueue{}
	s := newTestServer(&store, q)
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetJobNotFound(t *testing.T) {
	store := fakeStore{getJobErr: errNotFound}
	q := &fakeQueue{}
	s := newTestServer(&store, q)
	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusNotFound, "not_found")
}

func TestStatsOK(t *testing.T) {
	store := fakeStore{statsCounts: StatsCounts{Total: 2, Pending: 1, Failed: 1, DLQ: 0}}
	q := &fakeQueue{}
	s := newTestServer(&store, q)
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func newTestServer(store Store, queue Queue) *Server {
	cfg := testConfig()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewServer(cfg, store, queue, nil, logger)
}

func testConfig() config.Config {
	return config.Config{
		HTTPAddr:  ":0",
		QueueName: "jobs:ready",
		UIDir:     "./internal/api/ui",
		LogLevel:  "info",
	}
}

func assertAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected %d, got %d", status, rec.Code)
	}
	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode api error: %v", err)
	}
	if payload.Code != code {
		t.Fatalf("expected code %q, got %q", code, payload.Code)
	}
}

var errTest = errors.New("test error")
var errUnique = errors.New("duplicate key value violates unique constraint")

type fakeStore struct {
	pingErr       error
	insertErr     error
	getJobResp    JobStatusResponse
	getJobErr     error
	getByKeyResp  JobStatusResponse
	getByKeyErr   error
	queryJobsResp []JobStatusResponse
	queryJobsTotal int
	queryJobsErr  error
	retryOK       bool
	retryErr      error
	dlqOK         bool
	dlqErr        error
	dlqCalled     bool
	dlqReason     string
	statsCounts   StatsCounts
	statsErr      error
	dlqEntries    []DLQEntry
	dlqTotal      int
	getDLQEntryResp DLQEntry
	getDLQEntryErr  error
	replayErr     error
}

func (f fakeStore) Ping(ctx context.Context) error {
	return f.pingErr
}

func (f fakeStore) InsertJob(ctx context.Context, jobID string, req SubmitJobRequest, traceparent string) error {
	return f.insertErr
}

func (f fakeStore) GetJob(ctx context.Context, jobID string) (JobStatusResponse, error) {
	if f.getJobErr != nil {
		return JobStatusResponse{}, f.getJobErr
	}
	return f.getJobResp, nil
}

func (f fakeStore) GetJobByIdempotencyKey(ctx context.Context, key string) (JobStatusResponse, error) {
	if f.getByKeyErr != nil {
		return JobStatusResponse{}, f.getByKeyErr
	}
	return f.getByKeyResp, nil
}

func (f fakeStore) GetTraceparent(ctx context.Context, jobID string) (string, error) {
	return "", nil
}

func (f fakeStore) QueryJobs(ctx context.Context, q JobsQuery) ([]JobStatusResponse, int, error) {
	if f.queryJobsErr != nil {
		return nil, 0, f.queryJobsErr
	}
	return f.queryJobsResp, f.queryJobsTotal, nil
}

func (f fakeStore) RetryJob(ctx context.Context, jobID string) (bool, error) {
	if f.retryErr != nil {
		return false, f.retryErr
	}
	return f.retryOK, nil
}

func (f *fakeStore) DLQJob(ctx context.Context, jobID string, reason string) (bool, error) {
	if f.dlqErr != nil {
		return false, f.dlqErr
	}
	f.dlqCalled = true
	f.dlqReason = reason
	return f.dlqOK, nil
}

func (f fakeStore) Stats(ctx context.Context) (StatsCounts, error) {
	if f.statsErr != nil {
		return StatsCounts{}, f.statsErr
	}
	return f.statsCounts, nil
}

type fakeQueue struct {
	pingErr    error
	enqueueErr error
	enqueued   []string
}

func (f *fakeQueue) Ping(ctx context.Context) error {
	return f.pingErr
}

func (f *fakeQueue) Enqueue(ctx context.Context, queueName string, jobID string) error {
	f.enqueued = append(f.enqueued, jobID)
	return f.enqueueErr
}

func (f *fakeQueue) QueueDepth(ctx context.Context, queueName string) (int64, error) {
	return int64(len(f.enqueued)), nil
}
