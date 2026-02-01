package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var errNotFound = errors.New("not found")

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresStore) InsertJob(ctx context.Context, jobID string, req SubmitJobRequest) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (job_id, job_type, payload, idempotency_key, state, max_retries)
		VALUES ($1, $2, $3, $4, 'PENDING', $5)
	`, jobID, req.JobType, req.Payload, req.IdempotencyKey, req.MaxRetries)
	return err
}

func (s *PostgresStore) GetJob(ctx context.Context, jobID string) (JobStatusResponse, error) {
	var resp JobStatusResponse
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, job_type, state, retry_count, max_retries, COALESCE(last_error, ''),
			scheduled_at, available_at, started_at, completed_at, created_at, updated_at
		FROM jobs
		WHERE job_id = $1
	`, jobID).Scan(
		&resp.JobID,
		&resp.JobType,
		&resp.State,
		&resp.RetryCount,
		&resp.MaxRetries,
		&resp.LastError,
		&resp.ScheduledAt,
		&resp.AvailableAt,
		&resp.StartedAt,
		&resp.CompletedAt,
		&resp.CreatedAt,
		&resp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobStatusResponse{}, errNotFound
		}
		return JobStatusResponse{}, err
	}
	return resp, nil
}

func (s *PostgresStore) QueryJobs(ctx context.Context, q JobsQuery) ([]JobStatusResponse, int, error) {
	whereParts := []string{"1=1"}
	args := []any{}
	argID := 1

	addArg := func(v any) string {
		args = append(args, v)
		placeholder := fmt.Sprintf("$%d", argID)
		argID++
		return placeholder
	}

	if q.State != "" {
		p := addArg(q.State)
		whereParts = append(whereParts, "state = "+p)
	}
	if q.JobType != "" {
		p := addArg(q.JobType)
		whereParts = append(whereParts, "job_type = "+p)
	}
	if q.Q != "" {
		p := addArg("%" + q.Q + "%")
		whereParts = append(whereParts, "(job_id ILIKE "+p+" OR idempotency_key ILIKE "+p+")")
	}

	where := strings.Join(whereParts, " AND ")

	countSQL := "SELECT COUNT(1) FROM jobs WHERE " + where
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	itemsSQL := `
		SELECT job_id, job_type, state, retry_count, max_retries, COALESCE(last_error, ''),
			scheduled_at, available_at, started_at, completed_at, created_at, updated_at
		FROM jobs
		WHERE ` + where + `
		ORDER BY created_at DESC
		LIMIT $` + fmt.Sprintf("%d", argID) + ` OFFSET $` + fmt.Sprintf("%d", argID+1)
	args = append(args, q.Limit, q.Offset)

	rows, err := s.pool.Query(ctx, itemsSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []JobStatusResponse
	for rows.Next() {
		var resp JobStatusResponse
		if err := rows.Scan(
			&resp.JobID,
			&resp.JobType,
			&resp.State,
			&resp.RetryCount,
			&resp.MaxRetries,
			&resp.LastError,
			&resp.ScheduledAt,
			&resp.AvailableAt,
			&resp.StartedAt,
			&resp.CompletedAt,
			&resp.CreatedAt,
			&resp.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, resp)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (s *PostgresStore) RetryJob(ctx context.Context, jobID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'PENDING', last_error = '', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PostgresStore) DLQJob(ctx context.Context, jobID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'DLQ', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PostgresStore) Stats(ctx context.Context) (StatsCounts, error) {
	var counts StatsCounts
	err := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(1),
			COUNT(1) FILTER (WHERE state = 'PENDING'),
			COUNT(1) FILTER (WHERE state = 'FAILED'),
			COUNT(1) FILTER (WHERE state = 'DLQ')
		FROM jobs
	`).Scan(&counts.Total, &counts.Pending, &counts.Failed, &counts.DLQ)
	if err != nil {
		return StatsCounts{}, err
	}
	return counts, nil
}
