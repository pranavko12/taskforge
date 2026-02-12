package api

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Server) queuesSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/queues/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_queue", "missing queue", nil)
		return
	}

	parts := strings.Split(path, "/")
	queueName := strings.TrimSpace(parts[0])
	if queueName == "" {
		writeAPIError(w, http.StatusBadRequest, "missing_queue", "missing queue", nil)
		return
	}
	if len(parts) != 2 || parts[1] != "jobs" {
		writeAPIError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}

	qp := r.URL.Query()
	limit := parseInt(qp.Get("limit"), defaultListLimit)
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	offset := parseInt(qp.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	status := strings.TrimSpace(qp.Get("status"))
	jobType := strings.TrimSpace(qp.Get("jobType"))
	search := strings.TrimSpace(qp.Get("q"))

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	items, total, err := s.store.QueryJobs(ctx, JobsQuery{
		Limit:   limit,
		Offset:  offset,
		Queue:   queueName,
		State:   status,
		JobType: jobType,
		Q:       search,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to list jobs", nil)
		return
	}

	writeJSON(w, http.StatusOK, JobsListResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}
