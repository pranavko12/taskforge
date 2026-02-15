# TaskForge - Distributed Job Queue & Scheduler

TaskForge is a fault-tolerant distributed job queue and scheduler designed to execute asynchronous tasks reliably at scale. The system targets at-least-once delivery, retries with exponential backoff, worker leases with visibility timeouts, and full observability via logs, metrics, and tracing.

---

## API Endpoints

- GET `/healthz` (liveness; always 200 if process is up)
- GET `/readyz` (readiness; checks Postgres + Redis)
- GET `/stats`
- GET `/jobs`
- POST `/jobs`
- POST `/jobs` with an existing `idempotencyKey` returns the existing job id
- GET `/jobs/{id}`
- GET `/queues/{q}/jobs?status=...`
- POST `/jobs/{id}/retry`
- POST `/jobs/{id}/dlq` (optional body: `{ "reason": "..." }`)
- POST `/jobs/{id}/cancel` (optional body: `{ "reason": "..." }`)
- GET `/dlq`
- GET `/dlq/{id}`
- POST `/dlq/{id}/replay`
- GET `/metrics`

All error responses use a consistent JSON shape: `{ "code": "...", "message": "...", "details": ... }`.
Each response includes an `X-Request-ID` header for tracing.

`GET /jobs` and `GET /queues/{q}/jobs` pagination:
- Query params: `limit` (default `50`, hard max `200`), `offset` (default `0`).
- Values above max are clamped; negative offsets are normalized to `0`.
- `GET /queues/{q}/jobs` supports `status` (mapped to lifecycle state filter).
- `GET /jobs/{id}` is a single-resource lookup and is not paginated.

---

## curl Examples

Health and readiness:
```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

Enqueue a job:
```bash
curl -sS -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"jobType":"email","payload":{"to":"a@b.com"},"idempotencyKey":"abc-123"}'
```

Get job status:
```bash
curl -sS http://localhost:8080/jobs/<job-id>
```

List jobs for a queue/status:
```bash
curl -sS "http://localhost:8080/queues/jobs:ready/jobs?status=PENDING&limit=50&offset=0"
```

Cancel a job:
```bash
curl -sS -X POST http://localhost:8080/jobs/<job-id>/cancel \
  -H "Content-Type: application/json" \
  -d '{"reason":"user requested"}'
```

List DLQ and replay:
```bash
curl -sS "http://localhost:8080/dlq?limit=20&offset=0"
curl -sS -X POST http://localhost:8080/dlq/<job-id>/replay
```

Prometheus metrics:
```bash
curl -sS http://localhost:8080/metrics
```

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
- `LOG_LEVEL` (debug|info|warn|error, default `info`)
- `WORKER_CONCURRENCY` (default `10`)
- `RATE_LIMIT_PER_SEC` (default `0`, disabled)
- `TRACING_ENABLED` (default `false`)
- `TRACING_EXPORTER` (stdout|none, default `stdout`)

Config is validated at startup and fails fast with a readable error if invalid.

---

## Logging and Request IDs

- Structured JSON logging with request metadata (method, path, status, latency).
- `X-Request-ID` is generated if missing and returned on every response.

---

## Job Lifecycle and Reliability

State machine is enforced in the database and code.

Core behaviors:
- Idempotency keys on job creation (reused keys return existing job).
- Retries with exponential backoff via policy fields: `maxAttempts`, `initialDelay`, `backoff`, `maxDelay`, `jitter`.
- In v2, jitter is off by default (`jitter=false`) for deterministic scheduling.
- Scheduler computes `next_run_at` from attempt number and retry policy.
- Worker leases with visibility timeouts and heartbeat-based renewal.
- Failure classification: retryable failures transition to `FAILED`; terminal failures transition to `DLQ`.
- Concurrency limits and optional rate limiting per queue.

---

## Dead-Letter Queue (DLQ)

- DLQ is stored in Postgres table `dead_letters`.
- Each entry stores `job_id`, job `payload`, `reason`, `last_error`, `attempts`, and timestamps (`failed_at`, `created_at`, `updated_at`).
- Replay re-enqueues the job and resets execution state (`retry_count=0`, `attempt_count=0`, `state=PENDING`, `next_run_at=NOW()`).

API:
- GET `/dlq`
- GET `/dlq/{id}`
- POST `/dlq/{id}/replay`

---

## Metrics

Exposed at `GET /metrics` in Prometheus format.

Core metrics:
- `taskforge_queue_depth{queue}`
- `taskforge_dlq_count`
- `taskforge_job_attempts_total{queue}`
- `taskforge_job_success_total{queue}`
- `taskforge_job_failure_total{queue}`
- `taskforge_job_runtime_seconds_bucket{queue,...}`
- `taskforge_job_time_in_queue_seconds_bucket{queue,...}`
- `taskforge_worker_utilization{queue}`
- `taskforge_worker_concurrency_throttled_total{queue}`
- `taskforge_worker_rate_throttled_total{queue}`

---

## Tracing

OpenTelemetry tracing is a thin slice and disabled by default. When enabled, trace context is propagated from API -> scheduler -> worker via `traceparent`.

Config:
- `TRACING_ENABLED=true`
- `TRACING_EXPORTER=stdout` (writes spans to stdout)

Logs include `trace_id` when tracing is enabled. Spans include `job_id` and `queue` attributes.

---

## CLI

Build or run via `go run ./cmd/cli`.

Examples:
```
taskforge-cli enqueue --job-type email --idempotency-key abc123 --payload '{"to":"a@b.com"}'
taskforge-cli status --id 7b5b4f8e-2a7d-4e6f-9d5b-3a6b7f9a0c12
taskforge-cli cancel --id 7b5b4f8e-2a7d-4e6f-9d5b-3a6b7f9a0c12 --reason "user requested"
taskforge-cli dlq-list --limit 20
taskforge-cli dlq-replay --id 7b5b4f8e-2a7d-4e6f-9d5b-3a6b7f9a0c12
```

---

## Integration Tests

Run end-to-end tests with Docker:
```
bash scripts/integration-test.sh
```

This spins up Postgres and Redis via docker-compose and runs an enqueue -> execute -> status flow.

---

## Architecture Overview

### API Service
- Accepts job submissions via REST endpoints
- Provides job status querying and cancellation
- Persists job metadata and state transitions in PostgreSQL
- Publishes jobs to Redis queues for execution

### Scheduler
- Computes `next_run_at` for retries
- Enforces retry policies and transitions jobs
- Handles visibility timeouts and re-queues expired leases

### Worker Pool
- Stateless workers with configurable concurrency and rate limiting
- Lease-based execution with heartbeats
- Emits metrics for throttling and utilization

### Persistent Store (PostgreSQL)
- Stores job metadata and lifecycle state machine
- Tracks attempts, retry policy, and timing
- Stores DLQ entries with failure reasons

### Redis Layer
- Primary job queues
- Queue depth for metrics
