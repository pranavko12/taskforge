package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	db   *pgxpool.Pool
	rdb  *redis.Client
	http *http.Server
}

func NewServer(db *pgxpool.Pool, rdb *redis.Client) *Server {
	mux := http.NewServeMux()
	s := &Server{
		db:  db,
		rdb: rdb,
		http: &http.Server{
			Addr:              env("HTTP_ADDR", ":8080"),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/jobs", s.jobs)
	mux.HandleFunc("/jobs/", s.jobByID)

	return s
}

func (s *Server) Start() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		http.Error(w, "postgres not ready", http.StatusServiceUnavailable)
		return
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		http.Error(w, "redis not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req SubmitJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	req.JobType = strings.TrimSpace(req.JobType)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)

	if req.JobType == "" || len(req.Payload) == 0 || req.IdempotencyKey == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}
	if req.MaxRetries <= 0 {
		req.MaxRetries = 5
	}

	jobID := uuid.New().String()

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.insertJob(ctx, jobID, req); err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "duplicate idempotencyKey", http.StatusConflict)
			return
		}
		http.Error(w, "failed to persist job", http.StatusInternalServerError)
		return
	}

	queueName := env("QUEUE_NAME", "jobs:ready")
	if err := s.rdb.LPush(ctx, queueName, jobID).Err(); err != nil {
		http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, SubmitJobResponse{JobID: jobID})
}

func (s *Server) jobByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/jobs/"))
	if id == "" {
		http.Error(w, "missing job id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp, err := s.getJob(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to fetch job", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

var errNotFound = errors.New("not found")

func (s *Server) insertJob(ctx context.Context, jobID string, req SubmitJobRequest) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO jobs (job_id, job_type, payload, idempotency_key, state, max_retries)
		VALUES ($1, $2, $3, $4, 'PENDING', $5)
	`, jobID, req.JobType, req.Payload, req.IdempotencyKey, req.MaxRetries)
	return err
}

func (s *Server) getJob(ctx context.Context, jobID string) (JobStatusResponse, error) {
	var resp JobStatusResponse
	var createdAt time.Time
	var updatedAt time.Time

	row := s.db.QueryRow(ctx, `
		SELECT job_id, job_type, state, retry_count, max_retries, last_error, created_at, updated_at
		FROM jobs
		WHERE job_id = $1
	`, jobID)

	if err := row.Scan(&resp.JobID, &resp.JobType, &resp.State, &resp.RetryCount, &resp.MaxRetries, &resp.LastError, &createdAt, &updatedAt); err != nil {
		return JobStatusResponse{}, errNotFound
	}

	resp.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	resp.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return resp, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func env(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

func isUniqueViolation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") || strings.Contains(msg, "jobs_idempotency_key_uq")
}
