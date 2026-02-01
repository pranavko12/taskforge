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
}

type SubmitJobResponse struct {
	JobID string `json:"jobId"`
}

type JobStatusResponse struct {
	JobID       string     `json:"jobId"`
	JobType     string     `json:"jobType"`
	State       string     `json:"state"`
	RetryCount  int        `json:"retryCount"`
	MaxRetries  int        `json:"maxRetries"`
	LastError   string     `json:"lastError"`
	ScheduledAt time.Time  `json:"scheduledAt"`
	AvailableAt time.Time  `json:"availableAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type JobListResponse struct {
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
