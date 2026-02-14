ALTER TABLE jobs
  ADD COLUMN initial_delay INT NOT NULL DEFAULT 1000,
  ADD COLUMN backoff DOUBLE PRECISION NOT NULL DEFAULT 2.0,
  ADD COLUMN max_delay INT NOT NULL DEFAULT 60000;

UPDATE jobs
SET initial_delay = initial_delay_ms,
    backoff = backoff_multiplier,
    max_delay = max_delay_ms;

ALTER TABLE jobs
  ALTER COLUMN jitter DROP DEFAULT,
  ALTER COLUMN jitter TYPE BOOLEAN USING FALSE,
  ALTER COLUMN jitter SET DEFAULT FALSE;
