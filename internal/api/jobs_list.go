package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
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

	state := strings.TrimSpace(qp.Get("state"))
	jobType := strings.TrimSpace(qp.Get("jobType"))
	search := strings.TrimSpace(qp.Get("q"))

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	items, total, err := s.store.QueryJobs(ctx, JobsQuery{
		Limit:   limit,
		Offset:  offset,
		State:   state,
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

func parseInt(v string, fallback int) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
