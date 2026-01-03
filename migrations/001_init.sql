CREATE TYPE job_state AS ENUM (
  'PENDING',
  'IN_PROGRESS',
  'COMPLETED',
  'FAILED',
  'RETRYING',
  'DEAD'
);

CREATE TABLE jobs (
  job_id UUID PRIMARY KEY,
  job_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  idempotency_key TEXT NOT NULL,
  state job_state NOT NULL DEFAULT 'PENDING',
  retry_count INT NOT NULL DEFAULT 0,
  max_retries INT NOT NULL DEFAULT 5,
  last_error TEXT NOT NULL DEFAULT '',
  scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX jobs_idempotency_key_uq ON jobs (idempotency_key);
CREATE INDEX jobs_state_available_at_idx ON jobs (state, available_at);
