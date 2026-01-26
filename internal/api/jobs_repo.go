package api

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var errNotFound = errors.New("not found")

func (s *Server) insertJob(ctx context.Context, jobID string, req SubmitJobRequest) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO jobs (job_id, job_type, payload, idempotency_key, state, max_retries)
		VALUES ($1, $2, $3, $4, 'PENDING', $5)
	`, jobID, req.JobType, req.Payload, req.IdempotencyKey, req.MaxRetries)
	return err
}

func (s *Server) getJob(ctx context.Context, jobID string) (JobStatusResponse, error) {
	var resp JobStatusResponse
	var createdAt time.Time
	var updatedAt time.Time

	row := s.db.QueryRow(ctx, `
		SELECT job_id, job_type, state, retry_count, max_retries, COALESCE(last_error, ''),
		       created_at, updated_at
		FROM jobs
		WHERE job_id = $1
	`, jobID)

	if err := row.Scan(
		&resp.JobID,
		&resp.JobType,
		&resp.State,
		&resp.RetryCount,
		&resp.MaxRetries,
		&resp.LastError,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobStatusResponse{}, errNotFound
		}
		return JobStatusResponse{}, err
	}

	resp.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	resp.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return resp, nil
}
