package api

import (
	"context"
	"net/http"
	"time"
)

type StatsPoint struct {
	Ts      string `json:"ts"`
	Total   int    `json:"total"`
	Pending int    `json:"pending"`
	Failed  int    `json:"failed"`
	DLQ     int    `json:"dlq"`
}

type StatsResponse struct {
	Total   int         `json:"total"`
	Pending int         `json:"pending"`
	Failed  int         `json:"failed"`
	DLQ     int         `json:"dlq"`
	Points []StatsPoint `json:"points"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "invalid_method", "method not allowed", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	counts, err := s.store.Stats(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "failed to load stats", nil)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	resp := StatsResponse{
		Total:   counts.Total,
		Pending: counts.Pending,
		Failed:  counts.Failed,
		DLQ:     counts.DLQ,
		Points: []StatsPoint{
			{Ts: now, Total: counts.Total, Pending: counts.Pending, Failed: counts.Failed, DLQ: counts.DLQ},
		},
	}

	writeJSON(w, http.StatusOK, resp)
}
