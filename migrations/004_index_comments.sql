COMMENT ON INDEX jobs_idempotency_key_uq IS 'Ensure idempotency_key uniqueness for safe retries.';
COMMENT ON INDEX jobs_state_available_at_idx IS 'Supports pulling ready jobs by state and availability window.';
COMMENT ON INDEX jobs_state_next_run_at_idx IS 'Supports retry scheduling by next_run_at for backoff policies.';
