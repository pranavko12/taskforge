# TaskForge – Distributed Job Queue & Scheduler

TaskForge is a fault-tolerant distributed job queue and scheduler designed to execute asynchronous tasks reliably at scale. The system guarantees at-least-once delivery semantics, supports retries with exponential backoff, handles worker failures gracefully, and scales horizontally through stateless workers.

This project is built to demonstrate real-world backend system design principles, including reliability, concurrency, observability, and performance measurement.

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

