package api

import (
	"context"
	"strconv"
	"strings"
	"time"
)

type JobsQuery struct {
	Limit   int
	Offset  int
	State   string
	JobType string
	Q       string
}

type JobsListResponse struct {
	Items []JobStatusResponse `json:"items"`
	Total int                 `json:"total"`
}

func (s *Server) queryJobs(ctx context.Context, q JobsQuery) ([]JobStatusResponse, int, error) {
	whereParts := make([]string, 0, 4)
	args := make([]any, 0, 8)

	addArg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	if q.State != "" {
		whereParts = append(whereParts, "state = "+addArg(q.State))
	}
	if q.JobType != "" {
		whereParts = append(whereParts, "job_type = "+addArg(q.JobType))
	}
	if q.Q != "" {
		p := addArg("%" + q.Q + "%")
		whereParts = append(whereParts, "(job_id ILIKE "+p+" OR idempotency_key ILIKE "+p+")")
	}

	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}

	var total int
	countSQL := "SELECT COUNT(1) FROM jobs" + where
	if err := s.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitPH := addArg(q.Limit)
	offsetPH := addArg(q.Offset)

	listSQL := `
SELECT job_id, job_type, state, retry_count, max_retries, COALESCE(last_error, ''),
       created_at, updated_at
FROM jobs` + where + `
ORDER BY created_at DESC
LIMIT ` + limitPH + ` OFFSET ` + offsetPH

	rows, err := s.db.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]JobStatusResponse, 0, q.Limit)

	for rows.Next() {
		var resp JobStatusResponse
		var createdAt time.Time
		var updatedAt time.Time

		if err := rows.Scan(
			&resp.JobID,
			&resp.JobType,
			&resp.State,
			&resp.RetryCount,
			&resp.MaxRetries,
			&resp.LastError,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, 0, err
		}

		resp.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		resp.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		items = append(items, resp)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
