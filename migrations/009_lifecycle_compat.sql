-- Compatibility lifecycle columns using requested naming.
-- Existing runtime code keeps using canonical columns (state, attempt_count, lease_expires_at, lease_owner).
-- These columns make the schema expose: status, attempt, max_attempts, next_run_at, leased_until, lease_id, last_error.

ALTER TABLE jobs
  ADD COLUMN IF NOT EXISTS status TEXT GENERATED ALWAYS AS (state::text) STORED,
  ADD COLUMN IF NOT EXISTS attempt INT GENERATED ALWAYS AS (attempt_count) STORED,
  ADD COLUMN IF NOT EXISTS leased_until TIMESTAMPTZ GENERATED ALWAYS AS (lease_expires_at) STORED,
  ADD COLUMN IF NOT EXISTS lease_id TEXT GENERATED ALWAYS AS (lease_owner) STORED,
  ADD COLUMN IF NOT EXISTS queue TEXT NOT NULL DEFAULT 'jobs:ready';

CREATE INDEX IF NOT EXISTS jobs_queue_status_next_run_at_idx ON jobs (queue, status, next_run_at);
CREATE INDEX IF NOT EXISTS jobs_status_next_run_at_idx ON jobs (status, next_run_at);

COMMENT ON INDEX jobs_queue_status_next_run_at_idx IS 'Queue pull pattern: queue + status + next_run_at.';
COMMENT ON INDEX jobs_status_next_run_at_idx IS 'Status scan pattern: status + next_run_at.';
