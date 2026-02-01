package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pranavko12/taskforge/internal/config"
)

const maxJSONBody = 1 << 20 // 1 MiB

type Server struct {
	store     Store
	queue     Queue
	deps      DependencyChecker
	logger    *slog.Logger
	queueName string
	uiDir     string
	http      *http.Server
	handler   http.Handler
}

func NewServer(cfg config.Config, store Store, queue Queue, deps DependencyChecker, logger *slog.Logger) *Server {
	mux := http.NewServeMux()

	s := &Server{
		store:     store,
		queue:     queue,
		deps:      deps,
		logger:    logger,
		queueName: cfg.QueueName,
		uiDir:     cfg.UIDir,
		http: &http.Server{
			Addr:              cfg.HTTPAddr,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}

	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.deps == nil {
		s.deps = NewDependencyChecker(s.store, s.queue)
	}

	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/readyz", s.readyz)
	mux.HandleFunc("/health", s.readyz)

	mux.HandleFunc("/jobs", s.jobs)
	mux.HandleFunc("/jobs/", s.jobsSubroutes)

	// Implemented in stats.go
	mux.HandleFunc("/stats", s.stats)

	// Prefer embedded UI if available; fallback to disk.
	if h, err := uiHandler(); err == nil {
		mux.Handle("/ui/", http.StripPrefix("/ui/", h))
	} else {
		mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(s.uiDir))))
	}

	s.handler = requestIDMiddleware(loggingMiddleware(s.logger, mux))
	s.http.Handler = s.handler

	return s
}

func (s *Server) Start() error {
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.deps.Check(ctx); err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "dependency_not_ready", err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.submitJob(w, r)
		return
	case http.MethodGet:
		// Implemented in jobs_list.go
		s.listJobs(w, r)
		return
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}
}

func (s *Server) jobsSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_job_id", "missing job id", nil)
		return
	}

	parts := strings.Split(path, "/")
	id := strings.TrimSpace(parts[0])
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_job_id", "missing job id", nil)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
			return
		}
		s.getJobByID(w, r, id)
		return
	}

	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
			return
		}
		switch parts[1] {
		case "retry":
			// Implemented in jobs_route.go
			s.retryJob(w, r, id)
			return
		case "dlq":
			// Implemented in jobs_route.go
			s.dlqJob(w, r, id)
			return
		default:
			writeAPIError(w, http.StatusNotFound, "not_found", "unknown action", nil)
			return
		}
	}

	writeAPIError(w, http.StatusNotFound, "not_found", "not found", nil)
}

func (s *Server) submitJob(w http.ResponseWriter, r *http.Request) {
	var req SubmitJobRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid json", err.Error())
		return
	}

	req.JobType = strings.TrimSpace(req.JobType)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)

	if req.JobType == "" || len(req.Payload) == 0 || req.IdempotencyKey == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_required_fields", "missing required fields", nil)
		return
	}
	if req.MaxRetries <= 0 {
		req.MaxRetries = 5
	}

	// Make webhook jobs safe-by-default.
	if req.JobType == WebhookDeliverJobType {
		if err := validateWebhookDeliverPayload(req.Payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_payload", err.Error(), nil)
			return
		}
	}

	jobID := uuid.New().String()

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.store.InsertJob(ctx, jobID, req); err != nil {
		if isUniqueViolation(err) {
			writeAPIError(w, http.StatusConflict, "idempotency_conflict", "duplicate idempotencyKey", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to persist job", nil)
		return
	}

	if err := s.queue.Enqueue(ctx, s.queueName, jobID); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to enqueue job", nil)
		return
	}

	writeJSON(w, http.StatusAccepted, SubmitJobResponse{JobID: jobID})
}

func (s *Server) getJobByID(w http.ResponseWriter, r *http.Request, id string) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp, err := s.store.GetJob(ctx, id)
	if err != nil {
		if errors.Is(err, errNotFound) {
			writeAPIError(w, http.StatusNotFound, "not_found", "not found", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to fetch job", nil)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") || strings.Contains(msg, "jobs_idempotency_key_uq")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("extra data")
	}
	return nil
}
