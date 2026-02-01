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
	s := newTestServer(fakeStore{}, fakeQueue{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyzDependsOnDeps(t *testing.T) {
	s := newTestServer(fakeStore{pingErr: errTest}, fakeQueue{})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	s = newTestServer(fakeStore{}, fakeQueue{})
	req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequestIDHeaderSet(t *testing.T) {
	s := newTestServer(fakeStore{}, fakeQueue{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Header().Get(requestIDHeader) == "" {
		t.Fatalf("expected %s header to be set", requestIDHeader)
	}
}

func TestErrorShapeInvalidJSON(t *testing.T) {
	s := newTestServer(fakeStore{}, fakeQueue{})
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString("{invalid"))
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_json")
}

func TestJobsListOK(t *testing.T) {
	store := fakeStore{
		queryJobsResp: []JobStatusResponse{{JobID: "job-1", JobType: "demo"}},
		queryJobsTotal: 1,
	}
	s := newTestServer(store, fakeQueue{})
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetJobNotFound(t *testing.T) {
	store := fakeStore{getJobErr: errNotFound}
	s := newTestServer(store, fakeQueue{})
	req := httptest.NewRequest(http.MethodGet, "/jobs/missing", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusNotFound, "not_found")
}

func TestStatsOK(t *testing.T) {
	store := fakeStore{statsCounts: StatsCounts{Total: 2, Pending: 1, Failed: 1, DLQ: 0}}
	s := newTestServer(store, fakeQueue{})
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

type fakeStore struct {
	pingErr       error
	insertErr     error
	getJobResp    JobStatusResponse
	getJobErr     error
	queryJobsResp []JobStatusResponse
	queryJobsTotal int
	queryJobsErr  error
	retryOK       bool
	retryErr      error
	dlqOK         bool
	dlqErr        error
	statsCounts   StatsCounts
	statsErr      error
}

func (f fakeStore) Ping(ctx context.Context) error {
	return f.pingErr
}

func (f fakeStore) InsertJob(ctx context.Context, jobID string, req SubmitJobRequest) error {
	return f.insertErr
}

func (f fakeStore) GetJob(ctx context.Context, jobID string) (JobStatusResponse, error) {
	if f.getJobErr != nil {
		return JobStatusResponse{}, f.getJobErr
	}
	return f.getJobResp, nil
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

func (f fakeStore) DLQJob(ctx context.Context, jobID string) (bool, error) {
	if f.dlqErr != nil {
		return false, f.dlqErr
	}
	return f.dlqOK, nil
}

func (f fakeStore) Stats(ctx context.Context) (StatsCounts, error) {
	if f.statsErr != nil {
		return StatsCounts{}, f.statsErr
	}
	return f.statsCounts, nil
}

type fakeQueue struct {
	pingErr   error
	enqueueErr error
}

func (f fakeQueue) Ping(ctx context.Context) error {
	return f.pingErr
}

func (f fakeQueue) Enqueue(ctx context.Context, queueName string, jobID string) error {
	return f.enqueueErr
}
