CREATE TABLE dlq_entries (
  job_id UUID PRIMARY KEY REFERENCES jobs(job_id) ON DELETE CASCADE,
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX dlq_entries_created_at_idx ON dlq_entries (created_at DESC);
