package api

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Server) jobSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	parts := strings.Split(path, "/")
	jobID := parts[0]

	if jobID == "" {
		http.Error(w, "missing job id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		s.jobByID(w, r, jobID)
		return
	}

	switch parts[1] {
	case "retry":
		s.retryJob(w, r, jobID)
	case "dlq":
		s.dlqJob(w, r, jobID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) jobByID(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	job, err := s.getJob(ctx, jobID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	s.db.Exec(ctx, `
		UPDATE jobs
		SET state = 'PENDING', last_error = '', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)

	queue := env("QUEUE_NAME", "jobs:ready")
	s.rdb.LPush(ctx, queue, jobID)

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) dlqJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	s.db.Exec(ctx, `
		UPDATE jobs
		SET state = 'DLQ', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)

	w.WriteHeader(http.StatusAccepted)
}
