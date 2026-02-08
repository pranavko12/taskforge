package scheduler

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) GetRetryJob(ctx context.Context, jobID string) (RetryJob, error) {
	var job RetryJob
	err := s.pool.QueryRow(ctx, `
		SELECT job_id, retry_count, max_attempts, initial_delay_ms, backoff_multiplier, max_delay_ms, jitter, COALESCE(traceparent, '')
		FROM jobs
		WHERE job_id = $1
	`, jobID).Scan(
		&job.JobID,
		&job.RetryCount,
		&job.MaxAttempts,
		&job.InitialDelayMs,
		&job.BackoffMultiplier,
		&job.MaxDelayMs,
		&job.Jitter,
		&job.Traceparent,
	)
	if err != nil {
		return RetryJob{}, err
	}
	return job, nil
}

func (s *PostgresStore) UpdateRetrySchedule(ctx context.Context, jobID string, retryCount int, nextRunAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET retry_count = $1,
			state = 'RETRYING',
			next_run_at = $2,
			updated_at = NOW()
		WHERE job_id = $3
	`, retryCount, nextRunAt, jobID)
	return err
}

func (s *PostgresStore) ListDueRetries(ctx context.Context, now time.Time, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT job_id
		FROM jobs
		WHERE state = 'RETRYING' AND next_run_at <= $1
		ORDER BY next_run_at ASC
		LIMIT $2
	`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *PostgresStore) MarkRetryEnqueued(ctx context.Context, jobID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'PENDING',
			available_at = NOW(),
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	return err
}

func (s *PostgresStore) MarkTerminalFailure(ctx context.Context, jobID string, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET state = 'DLQ',
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		return err
	}
	if reason == "" {
		reason = "terminal failure"
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO dlq_entries (job_id, reason)
		VALUES ($1, $2)
		ON CONFLICT (job_id) DO UPDATE
		SET reason = EXCLUDED.reason, created_at = NOW()
	`, jobID, reason)
	if err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
