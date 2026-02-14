package api

import (
	"encoding/json"
	"time"
)

type SubmitJobRequest struct {
	JobType        string          `json:"jobType"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotencyKey"`
	MaxRetries     int             `json:"maxRetries"`
	MaxAttempts    int             `json:"maxAttempts"`
	InitialDelay   int             `json:"initialDelay"`
	Backoff        float64         `json:"backoff"`
	MaxDelay       int             `json:"maxDelay"`
	Jitter         bool            `json:"jitter"`
	// Legacy aliases kept for backward compatibility.
	InitialDelayMs    int     `json:"initialDelayMs"`
	BackoffMultiplier float64 `json:"backoffMultiplier"`
	MaxDelayMs        int     `json:"maxDelayMs"`
}

type SubmitJobResponse struct {
	JobID string `json:"jobId"`
}

type JobStatusResponse struct {
	JobID        string     `json:"jobId"`
	JobType      string     `json:"jobType"`
	State        string     `json:"state"`
	RetryCount   int        `json:"retryCount"`
	MaxRetries   int        `json:"maxRetries"`
	MaxAttempts  int        `json:"maxAttempts"`
	AttemptCount int        `json:"attemptCount"`
	InitialDelay int        `json:"initialDelay"`
	Backoff      float64    `json:"backoff"`
	MaxDelay     int        `json:"maxDelay"`
	Jitter       bool       `json:"jitter"`
	NextRunAt    time.Time  `json:"nextRunAt"`
	LastError    string     `json:"lastError"`
	ScheduledAt  time.Time  `json:"scheduledAt"`
	AvailableAt  time.Time  `json:"availableAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	Traceparent  string     `json:"traceparent,omitempty"`
}

type JobsListResponse struct {
	Items  []JobStatusResponse `json:"items"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

type JobsQuery struct {
	Limit   int
	Offset  int
	Queue   string
	State   string
	JobType string
	Q       string
}

type DLQEntry struct {
	JobID     string    `json:"jobId"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

type DLQRequest struct {
	Reason string `json:"reason"`
}

type DLQListResponse struct {
	Items  []DLQEntry `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type DLQInspectResponse struct {
	Entry DLQEntry          `json:"entry"`
	Job   JobStatusResponse `json:"job"`
}
