package api

import (
	"context"
	"encoding/json"
	"net/http"
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
	mux.HandleFunc("/jobs/", s.jobSubroutes)
	mux.HandleFunc("/stats", s.stats)

	h, err := uiHandler()
	if err == nil {
		mux.Handle("/ui/", http.StripPrefix("/ui/", h))
		mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/ui/", http.StatusMovedPermanently)
		})
	}

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
	w.Write([]byte("ok"))
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.submitJob(w, r)
	case http.MethodGet:
		s.listJobs(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) submitJob(w http.ResponseWriter, r *http.Request) {
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

	if req.JobType == WebhookDeliverJobType {
		if err := validateWebhookDeliverPayload(req.Payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
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

	queue := env("QUEUE_NAME", "jobs:ready")
	if err := s.rdb.LPush(ctx, queue, jobID).Err(); err != nil {
		http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusAccepted, SubmitJobResponse{JobID: jobID})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func isUniqueViolation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "jobs_idempotency_key_uq")
}
