ALTER TABLE jobs
  ADD COLUMN lease_owner TEXT,
  ADD COLUMN lease_expires_at TIMESTAMPTZ;

CREATE INDEX jobs_lease_expires_at_idx ON jobs (lease_expires_at);
