CREATE TABLE IF NOT EXISTS dead_letters (
  job_id UUID PRIMARY KEY REFERENCES jobs(job_id) ON DELETE CASCADE,
  queue_name TEXT NOT NULL,
  job_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  attempts INT NOT NULL DEFAULT 0,
  failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS dead_letters_created_at_idx ON dead_letters (created_at DESC);

INSERT INTO dead_letters (
  job_id, queue_name, job_type, payload, reason, last_error, attempts, failed_at, created_at, updated_at
)
SELECT
  j.job_id,
  j.queue_name,
  j.job_type,
  j.payload,
  d.reason,
  COALESCE(j.last_error, ''),
  COALESCE(j.attempt_count, 0),
  d.created_at,
  d.created_at,
  NOW()
FROM dlq_entries d
JOIN jobs j ON j.job_id = d.job_id
ON CONFLICT (job_id) DO NOTHING;
