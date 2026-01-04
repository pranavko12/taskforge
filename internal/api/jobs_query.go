package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
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
	where := "WHERE 1=1"
	args := make([]any, 0, 6)
	argn := 1

	if q.State != "" {
		where += " AND state = $" + itoa(argn)
		args = append(args, q.State)
		argn++
	}
	if q.JobType != "" {
		where += " AND job_type = $" + itoa(argn)
		args = append(args, q.JobType)
		argn++
	}
	if q.Q != "" {
		where += " AND (job_id ILIKE $" + itoa(argn) + " OR idempotency_key ILIKE $" + itoa(argn) + ")"
		args = append(args, "%"+q.Q+"%")
		argn++
	}

	var total int
	countSQL := "SELECT COUNT(1) FROM jobs " + where
	if err := s.db.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listSQL := `
		SELECT job_id, job_type, state, retry_count, max_retries, COALESCE(last_error, ''),
		       created_at, updated_at
		FROM jobs
	` + where + `
		ORDER BY created_at DESC
		LIMIT $` + itoa(argn) + ` OFFSET $` + itoa(argn+1)

	args = append(args, q.Limit, q.Offset)

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

func itoa(n int) string {
	return pgx.Identifier{string(rune('a'))}.Sanitize() + ""[:0] + strconvItoa(n)
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return sign + string(b[i:])
}
