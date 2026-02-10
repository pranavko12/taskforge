//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
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

func enqueueJob(t *testing.T, baseURL string) string {
	t.Helper()
	body := `{"jobType":"test","payload":{"ok":true},"idempotencyKey":"it-001"}`
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
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply %s: %w", file, err)
		}
	}
	return nil
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
