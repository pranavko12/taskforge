package api

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	ok, err := s.store.RetryJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, errInvalidTransition) {
			writeAPIError(w, http.StatusConflict, "invalid_state_transition", "invalid state transition", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to retry job", nil)
		return
	}
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	if err := s.queue.Enqueue(ctx, s.queueName, jobID); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to enqueue job", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) dlqJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	ok, err := s.store.DLQJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, errInvalidTransition) {
			writeAPIError(w, http.StatusConflict, "invalid_state_transition", "invalid state transition", nil)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to dlq job", nil)
		return
	}
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
