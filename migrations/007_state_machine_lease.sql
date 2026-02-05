CREATE OR REPLACE FUNCTION job_state_transition_ok(old_state job_state, new_state job_state)
RETURNS BOOLEAN AS $$
BEGIN
  IF old_state = new_state THEN
    RETURN TRUE;
  END IF;

  IF old_state = 'PENDING' AND new_state IN ('IN_PROGRESS', 'DLQ') THEN
    RETURN TRUE;
  END IF;
  IF old_state = 'IN_PROGRESS' AND new_state IN ('COMPLETED', 'FAILED', 'RETRYING', 'DLQ', 'PENDING') THEN
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
