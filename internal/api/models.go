package api

import (
	"encoding/json"
	"time"
)

type SubmitJobRequest struct {
	JobType           string          `json:"jobType"`
	Payload           json.RawMessage `json:"payload"`
	IdempotencyKey    string          `json:"idempotencyKey"`
	MaxRetries        int             `json:"maxRetries"`
	MaxAttempts       int             `json:"maxAttempts"`
	InitialDelayMs    int             `json:"initialDelayMs"`
	BackoffMultiplier float64         `json:"backoffMultiplier"`
	MaxDelayMs        int             `json:"maxDelayMs"`
	Jitter            float64         `json:"jitter"`
}

type SubmitJobResponse struct {
	JobID string `json:"jobId"`
}

type JobStatusResponse struct {
	JobID             string     `json:"jobId"`
	JobType           string     `json:"jobType"`
	State             string     `json:"state"`
	RetryCount        int        `json:"retryCount"`
	MaxRetries        int        `json:"maxRetries"`
	MaxAttempts       int        `json:"maxAttempts"`
	AttemptCount      int        `json:"attemptCount"`
	InitialDelayMs    int        `json:"initialDelayMs"`
	BackoffMultiplier float64    `json:"backoffMultiplier"`
	MaxDelayMs        int        `json:"maxDelayMs"`
	Jitter            float64    `json:"jitter"`
	NextRunAt         time.Time  `json:"nextRunAt"`
	LastError         string     `json:"lastError"`
	ScheduledAt       time.Time  `json:"scheduledAt"`
	AvailableAt       time.Time  `json:"availableAt"`
	StartedAt         *time.Time `json:"startedAt,omitempty"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
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
