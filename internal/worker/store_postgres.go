package worker

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

func (s *PostgresStore) AcquireLease(ctx context.Context, jobID string, owner string, now time.Time, leaseFor time.Duration) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'IN_PROGRESS',
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

func (s *PostgresStore) RenewLease(ctx context.Context, jobID string, owner string, now time.Time, leaseFor time.Duration) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET lease_expires_at = $1,
			updated_at = NOW()
		WHERE job_id = $2
			AND state = 'IN_PROGRESS'
			AND lease_owner = $3
	`, now.Add(leaseFor), jobID, owner)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
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
