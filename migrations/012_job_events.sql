CREATE TABLE IF NOT EXISTS job_events (
  event_id BIGSERIAL PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK (event_type IN ('leased', 'running', 'heartbeat', 'succeeded', 'failed', 'dlq')),
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS job_events_job_id_event_id_idx ON job_events (job_id, event_id);

CREATE OR REPLACE FUNCTION prevent_job_events_update_delete()
RETURNS TRIGGER AS $$
BEGIN
  RAISE EXCEPTION 'job_events is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS job_events_no_update ON job_events;
CREATE TRIGGER job_events_no_update
BEFORE UPDATE ON job_events
FOR EACH ROW
EXECUTE FUNCTION prevent_job_events_update_delete();

DROP TRIGGER IF EXISTS job_events_no_delete ON job_events;
CREATE TRIGGER job_events_no_delete
BEFORE DELETE ON job_events
FOR EACH ROW
EXECUTE FUNCTION prevent_job_events_update_delete();
