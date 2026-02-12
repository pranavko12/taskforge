ALTER TABLE jobs
  ADD COLUMN IF NOT EXISTS queue_name TEXT NOT NULL DEFAULT 'jobs:ready';

DROP INDEX IF EXISTS jobs_idempotency_key_uq;
CREATE UNIQUE INDEX IF NOT EXISTS jobs_queue_idempotency_key_uq ON jobs (queue_name, idempotency_key);

COMMENT ON INDEX jobs_queue_idempotency_key_uq IS 'Ensure idempotency_key uniqueness per queue.';
