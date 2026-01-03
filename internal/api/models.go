package api

import "encoding/json"

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
	JobID      string `json:"jobId"`
	JobType    string `json:"jobType"`
	State      string `json:"state"`
	RetryCount int    `json:"retryCount"`
	MaxRetries int    `json:"maxRetries"`
	LastError  string `json:"lastError"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}
