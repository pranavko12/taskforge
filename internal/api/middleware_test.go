package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDPropagationFromHeader(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFromContext(r.Context()); got != "req-123" {
			t.Fatalf("expected context request_id req-123, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(requestIDHeader, "req-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeader); got != "req-123" {
		t.Fatalf("expected response header request_id req-123, got %q", got)
	}
}

func TestLoggingMiddlewareEmitsJSONWithRequestFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := requestIDMiddleware(loggingMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})))

	req := httptest.NewRequest(http.MethodPost, "/jobs", nil)
	req.Header.Set(requestIDHeader, "req-abc")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected structured log output")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("expected JSON log output: %v", err)
	}

	if payload["msg"] != "request" {
		t.Fatalf("expected msg=request, got %v", payload["msg"])
	}
	if payload["method"] != http.MethodPost {
		t.Fatalf("expected method=%s, got %v", http.MethodPost, payload["method"])
	}
	if payload["path"] != "/jobs" {
		t.Fatalf("expected path=/jobs, got %v", payload["path"])
	}
	if payload["status"] != float64(http.StatusCreated) {
		t.Fatalf("expected status=%d, got %v", http.StatusCreated, payload["status"])
	}
	if payload["request_id"] != "req-abc" {
		t.Fatalf("expected request_id=req-abc, got %v", payload["request_id"])
	}
	if _, ok := payload["latency_ms"]; !ok {
		t.Fatal("expected latency_ms in log payload")
	}
}

