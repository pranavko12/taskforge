package api

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	tag, err := s.db.Exec(ctx, `
		UPDATE jobs
		SET state = 'PENDING', last_error = '', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		http.Error(w, "failed to retry job", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	queue := env("QUEUE_NAME", "jobs:ready")
	if err := s.rdb.LPush(ctx, queue, jobID).Err(); err != nil {
		http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dlqJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	tag, err := s.db.Exec(ctx, `
		UPDATE jobs
		SET state = 'DLQ', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		http.Error(w, "failed to dlq job", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
