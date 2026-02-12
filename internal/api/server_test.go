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

func TestHealthAliasAlwaysOK(t *testing.T) {
	q := &fakeQueue{}
	s := newTestServer(&fakeStore{pingErr: errTest}, q)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
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

func TestErrorShapeInvalidJSONHasDetails(t *testing.T) {
	q := &fakeQueue{}
	s := newTestServer(&fakeStore{}, q)
	req := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString("{invalid"))
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	payload := assertAPIError(t, rec, http.StatusBadRequest, "invalid_json")
	if payload.Details == nil {
		t.Fatal("expected details for invalid_json error")
	}
}

func TestErrorShapeRetryInvalidStateTransition(t *testing.T) {
	store := fakeStore{retryErr: errInvalidTransition}
	q := &fakeQueue{}
	s := newTestServer(&store, q)
	req := httptest.NewRequest(http.MethodPost, "/jobs/job-1/retry", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	payload := assertAPIError(t, rec, http.StatusConflict, "invalid_state_transition")
	if payload.Message == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestErrorShapeReadyzDependencyNotReady(t *testing.T) {
	store := fakeStore{pingErr: errTest}
	q := &fakeQueue{}
	s := newTestServer(&store, q)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	payload := assertAPIError(t, rec, http.StatusServiceUnavailable, "dependency_not_ready")
	if payload.Message == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestIdempotencyReturnsExistingJob(t *testing.T) {
	store := fakeStore{
		insertErr:    errUnique,
		getByKeyResp: JobStatusResponse{JobID: "existing"},
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
	if store.lastGetByKeyQ != testConfig().QueueName {
		t.Fatalf("expected queue-scoped idempotency lookup on %q, got %q", testConfig().QueueName, store.lastGetByKeyQ)
	}
}

func TestDuplicateSubmissionsReturnExistingJob(t *testing.T) {
	store := fakeStore{
		idemSeen: make(map[string]string),
	}
	q := &fakeQueue{}
	s := newTestServer(&store, q)

	body := `{"jobType":"demo","payload":{"a":1},"idempotencyKey":"dup-key"}`

	req1 := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(body))
	rec1 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusAccepted {
		t.Fatalf("first submission expected 202, got %d", rec1.Code)
	}
	var resp1 SubmitJobResponse
	if err := json.NewDecoder(rec1.Body).Decode(&resp1); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(body))
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second submission expected 200, got %d", rec2.Code)
	}
	var resp2 SubmitJobResponse
	if err := json.NewDecoder(rec2.Body).Decode(&resp2); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if resp2.JobID != resp1.JobID {
		t.Fatalf("expected same job id on duplicate submission, got %q and %q", resp1.JobID, resp2.JobID)
	}
	if store.lastGetByKeyQ != testConfig().QueueName {
		t.Fatalf("expected queue-scoped idempotency lookup on %q, got %q", testConfig().QueueName, store.lastGetByKeyQ)
	}
}

func TestJobsListOK(t *testing.T) {
	store := fakeStore{
		queryJobsResp:  []JobStatusResponse{{JobID: "job-1", JobType: "demo"}},
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

func TestJobsListAppliesHardLimitAndOffsetNormalization(t *testing.T) {
	store := fakeStore{
		queryJobsResp:  []JobStatusResponse{},
		queryJobsTotal: 0,
	}
	q := &fakeQueue{}
	s := newTestServer(&store, q)
	req := httptest.NewRequest(http.MethodGet, "/jobs?limit=9999&offset=-5", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if store.lastQuery.Limit != maxListLimit {
		t.Fatalf("expected clamped limit %d, got %d", maxListLimit, store.lastQuery.Limit)
	}
	if store.lastQuery.Offset != 0 {
		t.Fatalf("expected normalized offset 0, got %d", store.lastQuery.Offset)
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

func assertAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) APIError {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected %d, got %d", status, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode api error: %v", err)
	}
	if payload.Code != code {
		t.Fatalf("expected code %q, got %q", code, payload.Code)
	}
	if payload.Message == "" {
		t.Fatal("expected non-empty message")
	}
	return payload
}

var errTest = errors.New("test error")
var errUnique = errors.New("duplicate key value violates unique constraint")

type fakeStore struct {
	pingErr         error
	insertErr       error
	insertCount     int
	idemSeen        map[string]string
	getJobResp      JobStatusResponse
	getJobErr       error
	getByKeyResp    JobStatusResponse
	getByKeyErr     error
	lastGetByKeyQ   string
	lastQuery       JobsQuery
	queryJobsResp   []JobStatusResponse
	queryJobsTotal  int
	queryJobsErr    error
	retryOK         bool
	retryErr        error
	dlqOK           bool
	dlqErr          error
	dlqCalled       bool
	dlqReason       string
	statsCounts     StatsCounts
	statsErr        error
	dlqEntries      []DLQEntry
	dlqTotal        int
	getDLQEntryResp DLQEntry
	getDLQEntryErr  error
	replayErr       error
}

func (f fakeStore) Ping(ctx context.Context) error {
	return f.pingErr
}

func (f *fakeStore) InsertJob(ctx context.Context, jobID string, req SubmitJobRequest, traceparent string, queueName string) error {
	f.insertCount++
	if f.idemSeen != nil {
		if existingID, ok := f.idemSeen[queueName+"|"+req.IdempotencyKey]; ok {
			f.getByKeyResp = JobStatusResponse{JobID: existingID}
			return errUnique
		}
		f.idemSeen[queueName+"|"+req.IdempotencyKey] = jobID
	}
	return f.insertErr
}

func (f fakeStore) GetJob(ctx context.Context, jobID string) (JobStatusResponse, error) {
	if f.getJobErr != nil {
		return JobStatusResponse{}, f.getJobErr
	}
	return f.getJobResp, nil
}

func (f *fakeStore) GetJobByIdempotencyKey(ctx context.Context, key string, queueName string) (JobStatusResponse, error) {
	f.lastGetByKeyQ = queueName
	if f.getByKeyErr != nil {
		return JobStatusResponse{}, f.getByKeyErr
	}
	return f.getByKeyResp, nil
}

func (f fakeStore) GetTraceparent(ctx context.Context, jobID string) (string, error) {
	return "", nil
}

func (f *fakeStore) QueryJobs(ctx context.Context, q JobsQuery) ([]JobStatusResponse, int, error) {
	f.lastQuery = q
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
