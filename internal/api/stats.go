package api

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

type StatsPoint struct {
	Ts        string `json:"ts"`
	Created   int    `json:"created"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	DLQ       int    `json:"dlq"`
}

type StatsResponse struct {
	Points []StatsPoint `json:"points"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	mins := 60
	if v := r.URL.Query().Get("mins"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1440 {
			mins = n
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	points, err := s.queryStats(ctx, mins)
	if err != nil {
		http.Error(w, "failed to get stats", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, StatsResponse{Points: points})
}

func (s *Server) queryStats(ctx context.Context, mins int) ([]StatsPoint, error) {
	rows, err := s.db.Query(ctx, `
		WITH buckets AS (
		  SELECT generate_series(
		    date_trunc('minute', NOW() - ($1::int * interval '1 minute')),
		    date_trunc('minute', NOW()),
		    interval '1 minute'
		  ) AS ts
		),
		created AS (
		  SELECT date_trunc('minute', created_at) AS ts, COUNT(*) AS c
		  FROM jobs
		  WHERE created_at >= NOW() - ($1::int * interval '1 minute')
		  GROUP BY 1
		),
		succeeded AS (
		  SELECT date_trunc('minute', updated_at) AS ts, COUNT(*) AS c
		  FROM jobs
		  WHERE state = 'SUCCEEDED' AND updated_at >= NOW() - ($1::int * interval '1 minute')
		  GROUP BY 1
		),
		failed AS (
		  SELECT date_trunc('minute', updated_at) AS ts, COUNT(*) AS c
		  FROM jobs
		  WHERE state = 'FAILED' AND updated_at >= NOW() - ($1::int * interval '1 minute')
		  GROUP BY 1
		),
		dlq AS (
		  SELECT date_trunc('minute', updated_at) AS ts, COUNT(*) AS c
		  FROM jobs
		  WHERE state = 'DLQ' AND updated_at >= NOW() - ($1::int * interval '1 minute')
		  GROUP BY 1
		)
		SELECT
		  b.ts,
		  COALESCE(cr.c, 0),
		  COALESCE(sc.c, 0),
		  COALESCE(fc.c, 0),
		  COALESCE(dc.c, 0)
		FROM buckets b
		LEFT JOIN created cr ON cr.ts = b.ts
		LEFT JOIN succeeded sc ON sc.ts = b.ts
		LEFT JOIN failed fc ON fc.ts = b.ts
		LEFT JOIN dlq dc ON dc.ts = b.ts
		ORDER BY b.ts ASC
	`, mins)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StatsPoint{}
	for rows.Next() {
		var ts time.Time
		var c1, c2, c3, c4 int
		if err := rows.Scan(&ts, &c1, &c2, &c3, &c4); err != nil {
			return nil, err
		}
		out = append(out, StatsPoint{
			Ts:        ts.UTC().Format(time.RFC3339),
			Created:   c1,
			Succeeded: c2,
			Failed:    c3,
			DLQ:       c4,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
