package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type submitJobRequest struct {
	JobType        string          `json:"jobType"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey string          `json:"idempotencyKey"`
	MaxRetries     int             `json:"maxRetries,omitempty"`
	MaxAttempts    int             `json:"maxAttempts,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "enqueue":
		cmdEnqueue(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "cancel":
		cmdCancel(os.Args[2:])
	case "dlq-list":
		cmdDLQList(os.Args[2:])
	case "dlq-replay":
		cmdDLQReplay(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Print(`taskforge-cli

Usage:
  taskforge-cli <command> [flags]

Commands:
  enqueue     Submit a job
  status      Get job status
  cancel      Cancel a job (moves to DLQ with reason)
  dlq-list    List DLQ entries
  dlq-replay  Replay a DLQ job

Global flags:
  --api string   Base API URL (default from TASKFORGE_API or http://localhost:8080)
`)
}

func apiBase(fs *flag.FlagSet) *string {
	defaultAPI := os.Getenv("TASKFORGE_API")
	if defaultAPI == "" {
		defaultAPI = "http://localhost:8080"
	}
	return fs.String("api", defaultAPI, "Base API URL")
}

func cmdEnqueue(args []string) {
	fs := flag.NewFlagSet("enqueue", flag.ExitOnError)
	api := apiBase(fs)
	jobType := fs.String("job-type", "", "Job type")
	idempotencyKey := fs.String("idempotency-key", "", "Idempotency key")
	payload := fs.String("payload", "", "JSON payload string")
	payloadFile := fs.String("payload-file", "", "Path to JSON payload file")
	maxRetries := fs.Int("max-retries", 0, "Max retries (optional)")
	maxAttempts := fs.Int("max-attempts", 0, "Max attempts (optional)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if *jobType == "" || *idempotencyKey == "" {
		fmt.Fprintln(os.Stderr, "job-type and idempotency-key are required")
		fs.Usage()
		os.Exit(2)
	}

	raw, err := readPayload(*payload, *payloadFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	req := submitJobRequest{
		JobType:        *jobType,
		Payload:        json.RawMessage(raw),
		IdempotencyKey: *idempotencyKey,
	}
	if *maxRetries > 0 {
		req.MaxRetries = *maxRetries
	}
	if *maxAttempts > 0 {
		req.MaxAttempts = *maxAttempts
	}

	body, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	resp, err := httpPost(*api+"/jobs", body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(resp))
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	api := apiBase(fs)
	jobID := fs.String("id", "", "Job ID")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *jobID == "" {
		fmt.Fprintln(os.Stderr, "id is required")
		fs.Usage()
		os.Exit(2)
	}

	resp, err := httpGet(*api + "/jobs/" + *jobID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(resp))
}

func cmdCancel(args []string) {
	fs := flag.NewFlagSet("cancel", flag.ExitOnError)
	api := apiBase(fs)
	jobID := fs.String("id", "", "Job ID")
	reason := fs.String("reason", "canceled", "Cancel reason")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *jobID == "" {
		fmt.Fprintln(os.Stderr, "id is required")
		fs.Usage()
		os.Exit(2)
	}

	body, _ := json.Marshal(map[string]string{"reason": *reason})
	_, err := httpPost(*api+"/jobs/"+*jobID+"/cancel", body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

func cmdDLQList(args []string) {
	fs := flag.NewFlagSet("dlq-list", flag.ExitOnError)
	api := apiBase(fs)
	limit := fs.Int("limit", 50, "Limit")
	offset := fs.Int("offset", 0, "Offset")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	url := fmt.Sprintf("%s/dlq?limit=%d&offset=%d", *api, *limit, *offset)
	resp, err := httpGet(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(resp))
}

func cmdDLQReplay(args []string) {
	fs := flag.NewFlagSet("dlq-replay", flag.ExitOnError)
	api := apiBase(fs)
	jobID := fs.String("id", "", "Job ID")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *jobID == "" {
		fmt.Fprintln(os.Stderr, "id is required")
		fs.Usage()
		os.Exit(2)
	}

	_, err := httpPost(*api+"/dlq/"+*jobID+"/replay", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

func readPayload(inline string, file string) ([]byte, error) {
	if inline == "" && file == "" {
		return nil, fmt.Errorf("payload or payload-file is required")
	}
	if inline != "" && file != "" {
		return nil, fmt.Errorf("use either payload or payload-file, not both")
	}
	if inline != "" {
		return []byte(inline), nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(data), nil
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
