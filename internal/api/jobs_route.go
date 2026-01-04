package api

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Server) jobSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	path = strings.TrimSpace(path)
	if path == "" {
		http.Error(w, "missing job id", http.StatusBadRequest)
		return
	}

	parts := strings.Split(path, "/")
	id := strings.TrimSpace(parts[0])
	if id == "" {
		http.Error(w, "missing job id", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		s.jobByID(w, r)
		return
	}

	action := strings.TrimSpace(parts[1])
	switch action {
	case "retry":
		s.retryJob(w, r, id)
		return
	case "dlq":
		s.dlqJob(w, r, id)
		return
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	_, err := s.db.Exec(ctx, `
		UPDATE jobs
		SET state = 'PENDING', last_error = '', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		http.Error(w, "failed to retry", http.StatusInternalServerError)
		return
	}

	queueName := env("QUEUE_NAME", "jobs:ready")
	if err := s.rdb.LPush(ctx, queueName, jobID).Err(); err != nil {
		http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) dlqJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	_, err := s.db.Exec(ctx, `
		UPDATE jobs
		SET state = 'DLQ', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		http.Error(w, "failed to move to dlq", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
