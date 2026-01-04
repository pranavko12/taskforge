package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := parseInt(q.Get("limit"), 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	offset := parseInt(q.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	state := strings.TrimSpace(q.Get("state"))
	jobType := strings.TrimSpace(q.Get("jobType"))
	search := strings.TrimSpace(q.Get("q"))

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	items, total, err := s.queryJobs(ctx, JobsQuery{
		Limit:  limit,
		Offset: offset,
		State:  state,
		JobType: jobType,
		Q:      search,
	})
	if err != nil {
		http.Error(w, "failed to list jobs", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, JobsListResponse{
		Items: items,
		Total: total,
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
