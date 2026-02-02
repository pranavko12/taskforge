ALTER TYPE job_state ADD VALUE IF NOT EXISTS 'DLQ';

UPDATE jobs
SET state = 'DLQ'
WHERE state = 'DEAD';

CREATE OR REPLACE FUNCTION job_state_transition_ok(old_state job_state, new_state job_state)
RETURNS BOOLEAN AS $$
BEGIN
  IF old_state = new_state THEN
    RETURN TRUE;
  END IF;

  IF old_state = 'PENDING' AND new_state IN ('IN_PROGRESS', 'DLQ') THEN
    RETURN TRUE;
  END IF;
  IF old_state = 'IN_PROGRESS' AND new_state IN ('COMPLETED', 'FAILED', 'RETRYING', 'DLQ') THEN
    RETURN TRUE;
  END IF;
  IF old_state = 'FAILED' AND new_state IN ('RETRYING', 'DLQ', 'PENDING') THEN
    RETURN TRUE;
  END IF;
  IF old_state = 'RETRYING' AND new_state IN ('PENDING', 'DLQ') THEN
    RETURN TRUE;
  END IF;
  IF old_state = 'DLQ' AND new_state IN ('PENDING') THEN
    RETURN TRUE;
  END IF;

  RETURN FALSE;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION job_state_transition_guard()
RETURNS TRIGGER AS $$
BEGIN
  IF NOT job_state_transition_ok(OLD.state, NEW.state) THEN
    RAISE EXCEPTION 'invalid job state transition from % to %', OLD.state, NEW.state
      USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS jobs_state_transition_guard ON jobs;
CREATE TRIGGER jobs_state_transition_guard
BEFORE UPDATE OF state ON jobs
FOR EACH ROW
EXECUTE FUNCTION job_state_transition_guard();
