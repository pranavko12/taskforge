package worker

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) LeaseNextJob(ctx context.Context, queueName string, leaseID string, now time.Time, leaseFor time.Duration) (string, bool, error) {
	var jobID string
	err := s.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT job_id
			FROM jobs
			WHERE queue_name = $1
				AND state = 'PENDING'
				AND next_run_at <= $2
			ORDER BY next_run_at ASC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE jobs j
		SET state = 'IN_PROGRESS',
			attempt_count = attempt_count + 1,
			lease_owner = $3,
			lease_expires_at = $4,
			started_at = COALESCE(started_at, $2),
			updated_at = NOW()
		FROM candidate
		WHERE j.job_id = candidate.job_id
		RETURNING j.job_id
	`, queueName, now, leaseID, now.Add(leaseFor)).Scan(&jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return jobID, true, nil
}

func (s *PostgresStore) AcquireLease(ctx context.Context, jobID string, owner string, now time.Time, leaseFor time.Duration) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'IN_PROGRESS',
			attempt_count = attempt_count + 1,
			lease_owner = $1,
			lease_expires_at = $2,
			started_at = COALESCE(started_at, NOW()),
			updated_at = NOW()
		WHERE job_id = $3
			AND state = 'PENDING'
			AND (lease_expires_at IS NULL OR lease_expires_at <= $4)
	`, owner, now.Add(leaseFor), jobID, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStore) RenewLease(ctx context.Context, jobID string, leaseID string, extendBy time.Duration) (bool, error) {
	expiresAt := time.Now().UTC().Add(extendBy)
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET lease_expires_at = $1,
			updated_at = NOW()
		WHERE job_id = $2
			AND state = 'IN_PROGRESS'
			AND lease_owner = $3
	`, expiresAt, jobID, leaseID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStore) MarkJobSucceeded(ctx context.Context, jobID string, leaseID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'COMPLETED',
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = '',
			completed_at = NOW(),
			updated_at = NOW()
		WHERE job_id = $1
			AND state = 'IN_PROGRESS'
			AND lease_owner = $2
	`, jobID, leaseID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStore) MarkJobFailed(ctx context.Context, jobID string, leaseID string, lastError string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'FAILED',
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = $3,
			updated_at = NOW()
		WHERE job_id = $1
			AND state = 'IN_PROGRESS'
			AND lease_owner = $2
	`, jobID, leaseID, lastError)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresStore) MarkJobTerminal(ctx context.Context, jobID string, leaseID string, lastError string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	tag, err := tx.Exec(ctx, `
		UPDATE jobs
		SET state = 'DLQ',
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = $3,
			updated_at = NOW()
		WHERE job_id = $1
			AND state = 'IN_PROGRESS'
			AND lease_owner = $2
	`, jobID, leaseID, lastError)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		err = tx.Commit(ctx)
		if err != nil {
			return false, err
		}
		return false, nil
	}

	reason := lastError
	if reason == "" {
		reason = "terminal failure"
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO dlq_entries (job_id, reason)
		VALUES ($1, $2)
		ON CONFLICT (job_id) DO UPDATE
		SET reason = EXCLUDED.reason, created_at = NOW()
	`, jobID, reason); err != nil {
		return false, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *PostgresStore) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT job_id
		FROM jobs
		WHERE state = 'IN_PROGRESS'
			AND lease_expires_at IS NOT NULL
			AND lease_expires_at <= $1
		ORDER BY lease_expires_at ASC
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

func (s *PostgresStore) ResetLease(ctx context.Context, jobID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'PENDING',
			lease_owner = NULL,
			lease_expires_at = NULL,
			available_at = NOW(),
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	return err
}

func (s *PostgresStore) GetTraceparent(ctx context.Context, jobID string) (string, error) {
	var traceparent string
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(traceparent, '') FROM jobs WHERE job_id = $1`, jobID).Scan(&traceparent)
	return traceparent, err
}
