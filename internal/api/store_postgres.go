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
var errInvalidTransition = errors.New("invalid state transition")

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresStore) InsertJob(ctx context.Context, jobID string, req SubmitJobRequest, traceparent string, queueName string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (
			job_id, queue_name, job_type, payload, idempotency_key, state, max_retries,
			max_attempts, attempt_count, initial_delay_ms, backoff_multiplier, max_delay_ms, jitter, next_run_at, traceparent
		)
		VALUES ($1, $2, $3, $4, $5, 'PENDING', $6, $7, 0, $8, $9, $10, $11, NOW(), $12)
	`, jobID, queueName, req.JobType, req.Payload, req.IdempotencyKey, req.MaxRetries, req.MaxAttempts, req.InitialDelayMs, req.BackoffMultiplier, req.MaxDelayMs, req.Jitter, traceparent)
	return err
}

func (s *PostgresStore) GetJob(ctx context.Context, jobID string) (JobStatusResponse, error) {
	var resp JobStatusResponse
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, job_type, state, retry_count, max_retries, max_attempts, attempt_count,
			initial_delay_ms, backoff_multiplier, max_delay_ms, jitter, next_run_at, traceparent,
			COALESCE(last_error, ''), scheduled_at, available_at, started_at, completed_at, created_at, updated_at
		FROM jobs
		WHERE job_id = $1
	`, jobID).Scan(
		&resp.JobID,
		&resp.JobType,
		&resp.State,
		&resp.RetryCount,
		&resp.MaxRetries,
		&resp.MaxAttempts,
		&resp.AttemptCount,
		&resp.InitialDelayMs,
		&resp.BackoffMultiplier,
		&resp.MaxDelayMs,
		&resp.Jitter,
		&resp.NextRunAt,
		&resp.Traceparent,
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

func (s *PostgresStore) GetJobByIdempotencyKey(ctx context.Context, key string, queueName string) (JobStatusResponse, error) {
	var resp JobStatusResponse
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, job_type, state, retry_count, max_retries, max_attempts, attempt_count,
			initial_delay_ms, backoff_multiplier, max_delay_ms, jitter, next_run_at, traceparent,
			COALESCE(last_error, ''), scheduled_at, available_at, started_at, completed_at, created_at, updated_at
		FROM jobs
		WHERE idempotency_key = $1 AND queue_name = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, key, queueName).Scan(
		&resp.JobID,
		&resp.JobType,
		&resp.State,
		&resp.RetryCount,
		&resp.MaxRetries,
		&resp.MaxAttempts,
		&resp.AttemptCount,
		&resp.InitialDelayMs,
		&resp.BackoffMultiplier,
		&resp.MaxDelayMs,
		&resp.Jitter,
		&resp.NextRunAt,
		&resp.Traceparent,
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

func (s *PostgresStore) InsertDLQEntry(ctx context.Context, jobID string, reason string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO dlq_entries (job_id, reason)
		VALUES ($1, $2)
		ON CONFLICT (job_id) DO UPDATE
		SET reason = EXCLUDED.reason, created_at = NOW()
	`, jobID, reason)
	return err
}

func (s *PostgresStore) ListDLQ(ctx context.Context, limit, offset int) ([]DLQEntry, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(1) FROM dlq_entries`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT job_id, reason, created_at
		FROM dlq_entries
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []DLQEntry
	for rows.Next() {
		var entry DLQEntry
		if err := rows.Scan(&entry.JobID, &entry.Reason, &entry.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *PostgresStore) GetDLQEntry(ctx context.Context, jobID string) (DLQEntry, error) {
	var entry DLQEntry
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, reason, created_at
		FROM dlq_entries
		WHERE job_id = $1
	`, jobID).Scan(&entry.JobID, &entry.Reason, &entry.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DLQEntry{}, errNotFound
		}
		return DLQEntry{}, err
	}
	return entry, nil
}

func (s *PostgresStore) ReplayDLQ(ctx context.Context, jobID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	tag, err := tx.Exec(ctx, `
		UPDATE jobs
		SET state = 'PENDING',
			retry_count = 0,
			attempt_count = 0,
			last_error = '',
			next_run_at = NOW(),
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound
	}

	_, err = tx.Exec(ctx, `DELETE FROM dlq_entries WHERE job_id = $1`, jobID)
	if err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) GetTraceparent(ctx context.Context, jobID string) (string, error) {
	var traceparent string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(traceparent, '') FROM jobs WHERE job_id = $1`, jobID).Scan(&traceparent)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errNotFound
		}
		return "", err
	}
	return traceparent, nil
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
	if q.Queue != "" {
		p := addArg(q.Queue)
		whereParts = append(whereParts, "queue_name = "+p)
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
		SELECT job_id, job_type, state, retry_count, max_retries, max_attempts, attempt_count,
			initial_delay_ms, backoff_multiplier, max_delay_ms, jitter, next_run_at, traceparent,
			COALESCE(last_error, ''), scheduled_at, available_at, started_at, completed_at, created_at, updated_at
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
			&resp.MaxAttempts,
			&resp.AttemptCount,
			&resp.InitialDelayMs,
			&resp.BackoffMultiplier,
			&resp.MaxDelayMs,
			&resp.Jitter,
			&resp.NextRunAt,
			&resp.Traceparent,
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
	var state string
	if err := s.pool.QueryRow(ctx, `SELECT state FROM jobs WHERE job_id = $1`, jobID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, errNotFound
		}
		return false, err
	}
	if state != StateFailed && state != StateRetrying && state != StateDLQ && state != StateDead {
		return false, errInvalidTransition
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = $1, last_error = '', updated_at = NOW()
		WHERE job_id = $2
	`, StatePending, jobID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *PostgresStore) DLQJob(ctx context.Context, jobID string, reason string) (bool, error) {
	var state string
	if err := s.pool.QueryRow(ctx, `SELECT state FROM jobs WHERE job_id = $1`, jobID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, errNotFound
		}
		return false, err
	}
	if state != StateFailed && state != StateRetrying && state != StateInProgress {
		return false, errInvalidTransition
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = $1, updated_at = NOW()
		WHERE job_id = $2
	`, StateDLQ, jobID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		if reason == "" {
			reason = "manual dlq"
		}
		if err := s.InsertDLQEntry(ctx, jobID, reason); err != nil {
			return false, err
		}
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
