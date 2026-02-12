-- Compatibility lifecycle columns using requested naming.
-- Existing runtime code keeps using canonical columns (state, attempt_count, lease_expires_at, lease_owner).
-- These columns expose: status, attempt, max_attempts, next_run_at, leased_until, lease_id, last_error.

ALTER TABLE jobs
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'PENDING',
  ADD COLUMN IF NOT EXISTS attempt INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS leased_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS lease_id TEXT,
  ADD COLUMN IF NOT EXISTS queue TEXT NOT NULL DEFAULT 'jobs:ready';

UPDATE jobs
SET
  status = state::text,
  attempt = attempt_count,
  leased_until = lease_expires_at,
  lease_id = lease_owner;

CREATE OR REPLACE FUNCTION jobs_lifecycle_compat_sync()
RETURNS TRIGGER AS $$
BEGIN
  NEW.status := NEW.state::text;
  NEW.attempt := NEW.attempt_count;
  NEW.leased_until := NEW.lease_expires_at;
  NEW.lease_id := NEW.lease_owner;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS jobs_lifecycle_compat_sync ON jobs;
CREATE TRIGGER jobs_lifecycle_compat_sync
BEFORE INSERT OR UPDATE OF state, attempt_count, lease_expires_at, lease_owner ON jobs
FOR EACH ROW
EXECUTE FUNCTION jobs_lifecycle_compat_sync();

CREATE INDEX IF NOT EXISTS jobs_queue_status_next_run_at_idx ON jobs (queue, status, next_run_at);
CREATE INDEX IF NOT EXISTS jobs_status_next_run_at_idx ON jobs (status, next_run_at);

COMMENT ON INDEX jobs_queue_status_next_run_at_idx IS 'Queue pull pattern: queue + status + next_run_at.';
COMMENT ON INDEX jobs_status_next_run_at_idx IS 'Status scan pattern: status + next_run_at.';
