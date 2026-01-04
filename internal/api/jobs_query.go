package api

import (
	"context"
	"time"
)

func (s *Server) queryJobs(ctx context.Context, state, jobType, search string, limit, offset int) ([]JobListItem, int, error) {
	where := `WHERE 1=1`
	args := []any{}
	i := 1

	if state != "" {
		where += ` AND state = $` + itoa(i)
		args = append(args, state)
		i++
	}
	if jobType != "" {
		where += ` AND job_type = $` + itoa(i)
		args = append(args, jobType)
		i++
	}
	if search != "" {
		where += ` AND job_id ILIKE '%' || $` + itoa(i) + ` || '%'`
		args = append(args, search)
		i++
	}

	var total int
	row := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM jobs `+where, args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT job_id, job_type, state, retry_count, max_retries, COALESCE(last_error, ''), created_at, updated_at
		FROM jobs
	`+where+`
		ORDER BY created_at DESC
		LIMIT $`+itoa(i)+` OFFSET $`+itoa(i+1), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]JobListItem, 0, limit)
	for rows.Next() {
		var it JobListItem
		var createdAt time.Time
		var updatedAt time.Time
		if err := rows.Scan(&it.JobID, &it.JobType, &it.State, &it.RetryCount, &it.MaxRetries, &it.LastError, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}
		it.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		it.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func itoa(n int) string {
	buf := [20]byte{}
	i := len(buf)
	for {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			break
		}
	}
	return string(buf[i:])
}
