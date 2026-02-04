# TaskForge – Distributed Job Queue & Scheduler

TaskForge is a fault-tolerant distributed job queue and scheduler designed to execute asynchronous tasks reliably at scale. The system guarantees at-least-once delivery semantics, supports retries with exponential backoff, handles worker failures gracefully, and scales horizontally through stateless workers.

This project is built to demonstrate real-world backend system design principles, including reliability, concurrency, observability, and performance measurement.

---

## API Endpoints

- GET `/healthz` (liveness; always 200 if process is up)
- GET `/readyz` (readiness; checks Postgres + Redis)
- GET `/stats`
- GET `/jobs`
- POST `/jobs`
- POST `/jobs` with an existing `idempotencyKey` returns the existing job id
- GET `/jobs/{id}`
- POST `/jobs/{id}/retry`
- POST `/jobs/{id}/dlq` (optional body: `{ "reason": "..." }`)
- GET `/dlq`
- GET `/dlq/{id}`
- POST `/dlq/{id}/replay`

All error responses use a consistent JSON shape: `{ "code": "...", "message": "...", "details": ... }`.
Each response includes an `X-Request-ID` header for tracing.

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
- `UI_DIR` (default `./internal/api/ui`)

---

## Architecture Overview

TaskForge is composed of the following core components:

### API Service
- Accepts job submissions via REST endpoints
- Provides job status querying and cancellation
- Persists job metadata and state transitions in PostgreSQL
- Publishes jobs to Redis queues for execution

### Scheduler
- Pulls pending jobs from Redis
- Assigns jobs to workers
- Enforces visibility timeouts
- Re-queues jobs when workers fail or exceed execution deadlines

### Worker Pool
- Stateless workers with configurable concurrency
- Execute jobs pulled from Redis queues
- Use heartbeat-based liveness reporting
- Support idempotent execution to prevent duplicate processing

### Persistent Store (PostgreSQL)
- Stores job metadata and lifecycle state
- Tracks retry counts and timestamps
- Enables crash recovery and auditing

### Redis Layer
- Primary job queues
- Distributed locks
- Rate limiting
- Dead-letter queue for unrecoverable jobs

---

## Job Lifecycle

Each job follows a strict lifecycle:

