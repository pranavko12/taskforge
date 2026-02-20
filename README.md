# TaskForge - Distributed Job Queue and Scheduler (v2)

TaskForge is a fault-tolerant background job system using Postgres + Redis with lease-based workers, deterministic retries, DLQ handling, append-only event audit, and Prometheus metrics.

## v2 Scope (Completed)
- Config/env validation with fail-fast startup.
- Health endpoints: `/healthz`, `/readyz` (Postgres + Redis readiness).
- Structured JSON logs + `X-Request-ID`.
- Standard API error shape: `{code,message,details}`.
- Job lifecycle fields, transition enforcement, idempotency per queue.
- Concurrency-safe leasing (`FOR UPDATE SKIP LOCKED`) + lease renewal.
- Retry scheduling + failure classification (`retryable` vs `terminal`).
- DLQ storage and replay.
- Append-only `job_events`.
- CLI commands (`enqueue`, `job get`, `dlq list`, `dlq replay`).
- Integration harness via docker-compose with one command for local and CI.

---

## Quickstart

1) Copy env template:
```bash
cp env.example .env
```

2) Start core services:
```bash
docker compose up --build postgres redis api worker
```

3) Smoke check:
```bash
curl -i http://localhost:8081/healthz
curl -i http://localhost:8081/readyz
```

Notes:
- In `docker-compose.yml`, API is published as `localhost:8081`.
- Use `.env`/environment variables to configure DSNs and queue settings.

---

## Configuration

Required:
- `POSTGRES_DSN`

Common:
- `HTTP_ADDR` (default `:8080`)
- `QUEUE_NAME` (default `jobs:ready`)
- `REDIS_ADDR` (default `localhost:6379`)
- `REDIS_DB` (default `0`)
- `REDIS_PASSWORD` (default empty)
- `LOG_LEVEL` (`debug|info|warn|error`, default `info`)
- `WORKER_CONCURRENCY` (default `10`)
- `RATE_LIMIT_PER_SEC` (default `0`, disabled)
- `TRACING_ENABLED` (default `false`)
- `TRACING_EXPORTER` (`stdout|none`, default `stdout`)

Startup validates environment and returns explicit errors when invalid.

---

## Data Model and Lifecycle

`jobs` includes lifecycle and retry fields:
- `state`, `attempt_count`, `max_attempts`, `next_run_at`
- `lease_owner`, `lease_expires_at`
- `last_error`
- retry policy: `initial_delay`, `backoff`, `max_delay`, `jitter`

State machine is enforced in DB/code.

Retry policy:
- Deterministic exponential backoff with `maxAttempts`, `initialDelay`, `backoff`, `maxDelay`.
- v2 keeps jitter off by default (`jitter=false`).

Failure classification:
- Retryable failures -> `FAILED` (eligible for scheduler retry).
- Terminal failures -> `DLQ`.

Idempotency:
- Unique per queue (`queue_name`, `idempotency_key`).
- Duplicate submission returns existing job.

Append-only events:
- `job_events` records: `leased`, `running`, `heartbeat`, `succeeded`, `failed`, `dlq`.

---

## DLQ

DLQ is persisted in `dead_letters`:
- `job_id`, `queue_name`, `job_type`, `payload`
- `reason`, `last_error`, `attempts`
- `failed_at`, `created_at`, `updated_at`

Replay behavior:
- Re-enqueues job.
- Resets `state=PENDING`, `retry_count=0`, `attempt_count=0`, `next_run_at=NOW()`.

---

## HTTP API

Core:
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /stats`

Jobs:
- `POST /jobs`
- `GET /jobs`
- `GET /jobs/{id}`
- `GET /queues/{q}/jobs?status=...`
- `POST /jobs/{id}/retry`
- `POST /jobs/{id}/dlq`
- `POST /jobs/{id}/cancel`

DLQ:
- `GET /dlq`
- `GET /dlq/{id}`
- `POST /dlq/{id}/replay`

Conventions:
- Error shape is always `{ "code": "...", "message": "...", "details": ... }`.
- `X-Request-ID` is returned on every response.

Pagination:
- `GET /jobs` and `GET /queues/{q}/jobs` support `limit`/`offset`.
- Default `limit=50`, hard max `200`, negative offsets normalized to `0`.

---

## curl Examples

```bash
# enqueue
curl -sS -X POST http://localhost:8081/jobs \
  -H "Content-Type: application/json" \
  -d '{"jobType":"email","payload":{"to":"a@b.com"},"idempotencyKey":"abc-123"}'

# get job
curl -sS http://localhost:8081/jobs/<job-id>

# list queue jobs
curl -sS "http://localhost:8081/queues/jobs:ready/jobs?status=PENDING&limit=50&offset=0"

# dlq list and replay
curl -sS "http://localhost:8081/dlq?limit=20&offset=0"
curl -sS -X POST http://localhost:8081/dlq/<job-id>/replay
```

---

## CLI (Minimal but Real)

Run with `go run ./cmd/cli` or build your own `taskforge` binary.

```bash
taskforge enqueue --job-type email --idempotency-key abc123 --payload '{"to":"a@b.com"}'
taskforge job get --id 7b5b4f8e-2a7d-4e6f-9d5b-3a6b7f9a0c12
taskforge dlq list --limit 20
taskforge dlq replay --id 7b5b4f8e-2a7d-4e6f-9d5b-3a6b7f9a0c12
```

---

## Metrics

Exposed at `GET /metrics` (Prometheus format).

Stable metric names:
- `taskforge_queue_depth{queue}` gauge
- `taskforge_leased_count{queue}` gauge
- `taskforge_dlq_count` gauge
- `taskforge_job_runtime_seconds{queue}` histogram
- `taskforge_job_success_total{queue}` counter
- `taskforge_job_failure_total{queue}` counter
- `taskforge_lease_timeouts_total{queue}` counter
- `taskforge_worker_utilization{queue}` gauge
- `taskforge_worker_concurrency_throttled_total{queue}` counter
- `taskforge_worker_rate_throttled_total{queue}` counter

Stable labels:
- `queue`: queue name for queue/worker/job metrics.
- `taskforge_dlq_count` is global (no labels).

---

## Integration Tests (Local + CI)

Single command:
```bash
bash scripts/integration-test.sh
```

What it does:
- Starts `postgres`, `redis`, `api`, `worker` using `docker-compose.integration.yml`.
- Runs `go test -tags=integration ./internal/integration -count=1`.
- Tears down containers automatically.
- Supports optional focused runs via `INTEGRATION_TEST_PATTERN`, e.g.:
  - `INTEGRATION_TEST_PATTERN=TestIntegrationSuccessThenRetryDlqReplay bash scripts/integration-test.sh`

Covered flow:
- enqueue -> execute -> succeed
- enqueue failing -> retries -> dlq -> replay

---

## Architecture

- API service: submission, inspection, retry/DLQ/cancel endpoints.
- Scheduler: deterministic retry scheduling + due-retry enqueue.
- Worker: atomic leasing, heartbeat renewal, execution, success/failure transitions.
- Postgres: jobs, dead_letters, job_events.
- Redis: ready queue transport and queue-depth signal.
