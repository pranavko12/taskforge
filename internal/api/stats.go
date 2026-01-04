package api

import (
	"context"
	"net/http"
	"time"
)

type StatsPoint struct {
	Ts     string `json:"ts"`
	Total  int    `json:"total"`
	Pending int   `json:"pending"`
	DLQ    int    `json:"dlq"`
}

type StatsResponse struct {
	Points []StatsPoint `json:"points"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	var total int
	var pending int
	var dlq int

	err := s.db.QueryRow(ctx, `
		SELECT
			COUNT(1),
			COUNT(1) FILTER (WHERE state = 'PENDING'),
			COUNT(1) FILTER (WHERE state = 'DLQ')
		FROM jobs
	`).Scan(&total, &pending, &dlq)
	if err != nil {
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, StatsResponse{
		Points: []StatsPoint{
			{Ts: now, Total: total, Pending: pending, DLQ: dlq},
		},
	})
}
