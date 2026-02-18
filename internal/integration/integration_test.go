//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	redis "github.com/redis/go-redis/v9"

	"github.com/pranavko12/taskforge/internal/api"
	"github.com/pranavko12/taskforge/internal/config"
	"github.com/pranavko12/taskforge/internal/queue"
	"github.com/pranavko12/taskforge/internal/retry"
	"github.com/pranavko12/taskforge/internal/scheduler"
	"github.com/pranavko12/taskforge/internal/worker"
)

func TestEndToEndEnqueueExecuteStatus(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		HTTPAddr:          ":0",
		QueueName:         env("QUEUE_NAME", "jobs:ready"),
		UIDir:             "./internal/api/ui",
		LogLevel:          "info",
		PostgresDSN:       env("POSTGRES_DSN", "postgres://taskforge:taskforge@localhost:5432/taskforge?sslmode=disable"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     "",
		RedisDB:           0,
		WorkerConcurrency: 1,
		RateLimitPerSec:   0,
	}

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		t.Fatalf("postgres connect: %v", err)
	}
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	if err := applyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := api.NewPostgresStore(pool)
	q := queue.NewRedis(cfg)
	srv := api.NewServer(cfg, store, q, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	testServer := httptest.NewServer(srv.Handler())
	t.Cleanup(testServer.Close)

	jobID := enqueueJob(t, testServer.URL)
	executeJob(t, ctx, pool, rdb, cfg.QueueName, jobID)

	status := getJobStatus(t, testServer.URL, jobID)
	if status.State != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s", status.State)
	}
}

func TestWorkerCrashSimulationCancelMidJob(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		HTTPAddr:          ":0",
		QueueName:         env("QUEUE_NAME", "jobs:ready"),
		UIDir:             "./internal/api/ui",
		LogLevel:          "info",
		PostgresDSN:       env("POSTGRES_DSN", "postgres://taskforge:taskforge@localhost:5432/taskforge?sslmode=disable"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     "",
		RedisDB:           0,
		WorkerConcurrency: 1,
		RateLimitPerSec:   0,
	}

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		t.Fatalf("postgres connect: %v", err)
	}
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	if err := applyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := api.NewPostgresStore(pool)
	q := queue.NewRedis(cfg)
	srv := api.NewServer(cfg, store, q, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	testServer := httptest.NewServer(srv.Handler())
	t.Cleanup(testServer.Close)

	body := `{"jobType":"test","payload":{"ok":true},"idempotencyKey":"it-crash-001"}`
	resp, err := httpPost(testServer.URL+"/jobs", []byte(body))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	var parsed struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatalf("parse enqueue: %v", err)
	}
	if parsed.JobID == "" {
		t.Fatal("missing job id")
	}

	pop, err := rdb.BRPop(ctx, 5*time.Second, cfg.QueueName).Result()
	if err != nil || len(pop) != 2 {
		t.Fatalf("queue pop: %v %v", err, pop)
	}
	if pop[1] != parsed.JobID {
		t.Fatalf("expected %s, got %s", parsed.JobID, pop[1])
	}

	leaseStore := worker.NewPostgresStore(pool)
	ok, err := leaseStore.AcquireLease(ctx, parsed.JobID, "worker-it-crash", time.Now().UTC(), 50*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire lease: %v ok=%v", err, ok)
	}

	loop := worker.NewLoop(leaseStore, cfg.QueueName, "worker-it-crash", 50*time.Millisecond)
	execCtx, cancelExec := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancelExec()
	}()

	if err := loop.ProcessOne(execCtx, parsed.JobID, func(ctx context.Context, jobID string) error {
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatalf("process one: %v", err)
	}

	status := getJobStatus(t, testServer.URL, parsed.JobID)
	if status.State != "FAILED" {
		t.Fatalf("expected FAILED, got %s", status.State)
	}
	if status.LastError == "" {
		t.Fatal("expected last_error to be populated")
	}
}

func TestDLQReplayResetsCountersAndSchedule(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		HTTPAddr:          ":0",
		QueueName:         env("QUEUE_NAME", "jobs:ready"),
		UIDir:             "./internal/api/ui",
		LogLevel:          "info",
		PostgresDSN:       env("POSTGRES_DSN", "postgres://taskforge:taskforge@localhost:5432/taskforge?sslmode=disable"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     "",
		RedisDB:           0,
		WorkerConcurrency: 1,
		RateLimitPerSec:   0,
	}

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		t.Fatalf("postgres connect: %v", err)
	}
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	if err := applyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := api.NewPostgresStore(pool)
	q := queue.NewRedis(cfg)
	srv := api.NewServer(cfg, store, q, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	testServer := httptest.NewServer(srv.Handler())
	t.Cleanup(testServer.Close)

	jobID := enqueueJob(t, testServer.URL)

	_, err = pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'DLQ',
			retry_count = 2,
			attempt_count = 3,
			last_error = 'bad payload',
			next_run_at = NOW() + INTERVAL '10 minutes',
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		t.Fatalf("prepare dlq job: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO dead_letters (job_id, queue_name, job_type, payload, reason, last_error, attempts, failed_at, created_at, updated_at)
		SELECT job_id, queue_name, job_type, payload, 'manual dlq', 'bad payload', 3, NOW(), NOW(), NOW()
		FROM jobs
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		t.Fatalf("insert dead_letter: %v", err)
	}

	if _, err := httpPost(testServer.URL+"/dlq/"+jobID+"/replay", nil); err != nil {
		t.Fatalf("replay: %v", err)
	}

	var state string
	var retryCount, attemptCount int
	var nextRunAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT state, retry_count, attempt_count, next_run_at
		FROM jobs
		WHERE job_id = $1
	`, jobID).Scan(&state, &retryCount, &attemptCount, &nextRunAt)
	if err != nil {
		t.Fatalf("query replayed job: %v", err)
	}

	if state != "PENDING" {
		t.Fatalf("expected state PENDING, got %s", state)
	}
	if retryCount != 0 || attemptCount != 0 {
		t.Fatalf("expected counters reset, got retry=%d attempt=%d", retryCount, attemptCount)
	}
	if delta := time.Since(nextRunAt); delta < -2*time.Second || delta > 2*time.Second {
		t.Fatalf("expected next_run_at near now, got %v (delta %v)", nextRunAt, delta)
	}

	var dlqCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(1) FROM dead_letters WHERE job_id = $1`, jobID).Scan(&dlqCount); err != nil {
		t.Fatalf("query dead_letters: %v", err)
	}
	if dlqCount != 0 {
		t.Fatalf("expected dead_letter removed on replay, count=%d", dlqCount)
	}
}

func TestJobEventsOrdering(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		HTTPAddr:          ":0",
		QueueName:         env("QUEUE_NAME", "jobs:ready"),
		UIDir:             "./internal/api/ui",
		LogLevel:          "info",
		PostgresDSN:       env("POSTGRES_DSN", "postgres://taskforge:taskforge@localhost:5432/taskforge?sslmode=disable"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     "",
		RedisDB:           0,
		WorkerConcurrency: 1,
		RateLimitPerSec:   0,
	}

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		t.Fatalf("postgres connect: %v", err)
	}
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Del(ctx, cfg.QueueName).Err(); err != nil {
		t.Fatalf("clear queue: %v", err)
	}

	if err := applyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := api.NewPostgresStore(pool)
	q := queue.NewRedis(cfg)
	srv := api.NewServer(cfg, store, q, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	testServer := httptest.NewServer(srv.Handler())
	t.Cleanup(testServer.Close)

	jobID := enqueueJob(t, testServer.URL)

	pop, err := rdb.BRPop(ctx, 5*time.Second, cfg.QueueName).Result()
	if err != nil || len(pop) != 2 {
		t.Fatalf("queue pop: %v %v", err, pop)
	}
	if pop[1] != jobID {
		t.Fatalf("expected %s, got %s", jobID, pop[1])
	}

	leaseStore := worker.NewPostgresStore(pool)
	ok, err := leaseStore.AcquireLease(ctx, jobID, "worker-events", time.Now().UTC(), 80*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire lease: %v ok=%v", err, ok)
	}

	loop := worker.NewLoop(leaseStore, cfg.QueueName, "worker-events", 80*time.Millisecond)
	if err := loop.ProcessOne(context.Background(), jobID, func(ctx context.Context, id string) error {
		time.Sleep(180 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatalf("process one: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT event_type
		FROM job_events
		WHERE job_id = $1
		ORDER BY event_id ASC
	`, jobID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()

	var events []string
	for rows.Next() {
		var event string
		if err := rows.Scan(&event); err != nil {
			t.Fatalf("scan event: %v", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if len(events) < 4 {
		t.Fatalf("expected at least 4 events, got %v", events)
	}
	if events[0] != "leased" || events[1] != "running" {
		t.Fatalf("expected leased->running prefix, got %v", events)
	}
	if events[len(events)-1] != "succeeded" {
		t.Fatalf("expected last event succeeded, got %v", events[len(events)-1])
	}
	heartbeatSeen := false
	for _, event := range events[2 : len(events)-1] {
		if event == "heartbeat" {
			heartbeatSeen = true
			break
		}
	}
	if !heartbeatSeen {
		t.Fatalf("expected heartbeat before succeeded, got %v", events)
	}
}

func TestIntegrationSuccessThenRetryDlqReplay(t *testing.T) {
	ctx := context.Background()
	cfg := config.Config{
		HTTPAddr:          ":0",
		QueueName:         env("QUEUE_NAME", "jobs:ready"),
		UIDir:             "./internal/api/ui",
		LogLevel:          "info",
		PostgresDSN:       env("POSTGRES_DSN", "postgres://taskforge:taskforge@localhost:5432/taskforge?sslmode=disable"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     "",
		RedisDB:           0,
		WorkerConcurrency: 1,
		RateLimitPerSec:   0,
	}

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		t.Fatalf("postgres connect: %v", err)
	}
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Del(ctx, cfg.QueueName).Err(); err != nil {
		t.Fatalf("clear queue: %v", err)
	}

	if err := applyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := api.NewPostgresStore(pool)
	q := queue.NewRedis(cfg)
	srv := api.NewServer(cfg, store, q, nil, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	testServer := httptest.NewServer(srv.Handler())
	t.Cleanup(testServer.Close)

	leaseStore := worker.NewPostgresStore(pool)
	loop := worker.NewLoop(leaseStore, cfg.QueueName, "worker-it-flow", 80*time.Millisecond)
	retryScheduler := scheduler.New(scheduler.NewPostgresStore(pool), q, cfg.QueueName)

	// Success path: enqueue -> execute -> succeeded.
	successJobID := enqueueJob(t, testServer.URL)
	popSuccess, err := rdb.BRPop(ctx, 5*time.Second, cfg.QueueName).Result()
	if err != nil || len(popSuccess) != 2 {
		t.Fatalf("queue pop success: %v %v", err, popSuccess)
	}
	if popSuccess[1] != successJobID {
		t.Fatalf("expected %s, got %s", successJobID, popSuccess[1])
	}
	ok, err := leaseStore.AcquireLease(ctx, successJobID, "worker-it-flow", time.Now().UTC(), 80*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire success lease: %v ok=%v", err, ok)
	}
	if err := loop.ProcessOne(context.Background(), successJobID, func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("process success job: %v", err)
	}
	successStatus := getJobStatus(t, testServer.URL, successJobID)
	if successStatus.State != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s", successStatus.State)
	}

	// Failure path: enqueue -> fail -> retry -> fail -> dlq -> replay.
	failBody := `{"jobType":"test","payload":{"ok":false},"idempotencyKey":"it-fail-001","maxAttempts":2,"initialDelay":1,"backoff":1,"maxDelay":1,"jitter":false}`
	failJobID := enqueueJobWithBody(t, testServer.URL, failBody)

	popFail, err := rdb.BRPop(ctx, 5*time.Second, cfg.QueueName).Result()
	if err != nil || len(popFail) != 2 {
		t.Fatalf("queue pop fail attempt1: %v %v", err, popFail)
	}
	if popFail[1] != failJobID {
		t.Fatalf("expected %s, got %s", failJobID, popFail[1])
	}
	ok, err = leaseStore.AcquireLease(ctx, failJobID, "worker-it-flow", time.Now().UTC(), 80*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire fail attempt1 lease: %v ok=%v", err, ok)
	}
	if err := loop.ProcessOne(context.Background(), failJobID, func(context.Context, string) error {
		return retry.Retryable(errors.New("temporary upstream error"))
	}); err != nil {
		t.Fatalf("process fail attempt1: %v", err)
	}

	now := time.Now().UTC()
	if _, err := retryScheduler.ScheduleRetry(ctx, failJobID, now, 1); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	if _, err := retryScheduler.EnqueueDueRetries(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatalf("enqueue due retries: %v", err)
	}

	popRetry, err := rdb.BRPop(ctx, 5*time.Second, cfg.QueueName).Result()
	if err != nil || len(popRetry) != 2 {
		t.Fatalf("queue pop fail attempt2: %v %v", err, popRetry)
	}
	if popRetry[1] != failJobID {
		t.Fatalf("expected %s, got %s", failJobID, popRetry[1])
	}
	ok, err = leaseStore.AcquireLease(ctx, failJobID, "worker-it-flow", time.Now().UTC(), 80*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("acquire fail attempt2 lease: %v ok=%v", err, ok)
	}
	if err := loop.ProcessOne(context.Background(), failJobID, func(context.Context, string) error {
		return retry.Retryable(errors.New("temporary upstream error"))
	}); err != nil {
		t.Fatalf("process fail attempt2: %v", err)
	}

	if _, err := retryScheduler.ScheduleRetry(ctx, failJobID, now.Add(3*time.Second), 1); !errors.Is(err, scheduler.ErrMaxAttemptsExceeded) {
		t.Fatalf("expected max attempts exceeded, got %v", err)
	}
	failStatus := getJobStatus(t, testServer.URL, failJobID)
	if failStatus.State != "DLQ" {
		t.Fatalf("expected DLQ, got %s", failStatus.State)
	}

	if _, err := httpPost(testServer.URL+"/dlq/"+failJobID+"/replay", nil); err != nil {
		t.Fatalf("replay failed job: %v", err)
	}
	replayedStatus := getJobStatus(t, testServer.URL, failJobID)
	if replayedStatus.State != "PENDING" {
		t.Fatalf("expected PENDING after replay, got %s", replayedStatus.State)
	}
	if replayedStatus.RetryCount != 0 || replayedStatus.AttemptCount != 0 {
		t.Fatalf("expected counters reset after replay, got retry=%d attempt=%d", replayedStatus.RetryCount, replayedStatus.AttemptCount)
	}
}

func enqueueJob(t *testing.T, baseURL string) string {
	t.Helper()
	body := `{"jobType":"test","payload":{"ok":true},"idempotencyKey":"it-001"}`
	return enqueueJobWithBody(t, baseURL, body)
}

func enqueueJobWithBody(t *testing.T, baseURL, body string) string {
	t.Helper()
	resp, err := httpPost(baseURL+"/jobs", []byte(body))
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	var parsed struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatalf("parse enqueue: %v", err)
	}
	if parsed.JobID == "" {
		t.Fatalf("missing job id")
	}
	return parsed.JobID
}

func executeJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, rdb *redis.Client, queueName, jobID string) {
	t.Helper()
	res := rdb.BRPop(ctx, 5*time.Second, queueName)
	vals, err := res.Result()
	if err != nil || len(vals) != 2 {
		t.Fatalf("queue pop: %v %v", err, vals)
	}
	if vals[1] != jobID {
		t.Fatalf("expected %s, got %s", jobID, vals[1])
	}

	leaseStore := worker.NewPostgresStore(pool)
	ok, err := leaseStore.AcquireLease(ctx, jobID, "worker-it", time.Now().UTC(), 30*time.Second)
	if err != nil || !ok {
		t.Fatalf("acquire lease: %v ok=%v", err, ok)
	}

	_, err = pool.Exec(ctx, `
		UPDATE jobs
		SET state = 'COMPLETED',
			lease_owner = NULL,
			lease_expires_at = NULL,
			completed_at = NOW(),
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		t.Fatalf("complete job: %v", err)
	}
}

func getJobStatus(t *testing.T, baseURL, jobID string) api.JobStatusResponse {
	t.Helper()
	resp, err := httpGet(baseURL + "/jobs/" + jobID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var status api.JobStatusResponse
	if err := json.Unmarshal(resp, &status); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	return status
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	// Reset schema between tests so each test can apply migrations from scratch.
	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;`); err != nil {
		return fmt.Errorf("reset schema: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	for _, file := range files {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		stmts := splitSQLStatements(string(sqlBytes))
		for _, stmt := range stmts {
			if _, err := pool.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("apply %s: %w", file, err)
			}
		}
	}
	return nil
}

func splitSQLStatements(sqlText string) []string {
	var statements []string
	var buf strings.Builder
	inDollar := false

	for i := 0; i < len(sqlText); i++ {
		ch := sqlText[i]
		if ch == '$' && i+1 < len(sqlText) && sqlText[i+1] == '$' {
			inDollar = !inDollar
			buf.WriteByte(ch)
			buf.WriteByte(sqlText[i+1])
			i++
			continue
		}
		if ch == ';' && !inDollar {
			stmt := strings.TrimSpace(buf.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	stmt := strings.TrimSpace(buf.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}
	return statements
}

func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func httpPost(url string, body []byte) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func env(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}
