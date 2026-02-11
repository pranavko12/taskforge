package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
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
		status, code, message := mapDomainError(err, http.StatusInternalServerError, "internal_error", "failed to retry job")
		writeAPIError(w, status, code, message, nil)
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

	var req DLQRequest
	if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid json", err.Error())
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	ok, err := s.store.DLQJob(ctx, jobID, req.Reason)
	if err != nil {
		status, code, message := mapDomainError(err, http.StatusInternalServerError, "internal_error", "failed to dlq job")
		writeAPIError(w, status, code, message, nil)
		return
	}
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}

	var req DLQRequest
	if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "invalid json", err.Error())
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		req.Reason = "canceled"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	ok, err := s.store.DLQJob(ctx, jobID, req.Reason)
	if err != nil {
		status, code, message := mapDomainError(err, http.StatusInternalServerError, "internal_error", "failed to cancel job")
		writeAPIError(w, status, code, message, nil)
		return
	}
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
