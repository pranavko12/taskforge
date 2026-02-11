package config

import (
	"strings"
	"testing"
)

func TestLoadFailsWhenPostgresDSNMissing(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when POSTGRES_DSN is missing")
	}
	if !strings.Contains(err.Error(), "POSTGRES_DSN is required") {
		t.Fatalf("expected missing POSTGRES_DSN error, got: %v", err)
	}
}

func TestLoadFailsOnInvalidIntegerEnv(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://example")
	t.Setenv("WORKER_CONCURRENCY", "abc")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid WORKER_CONCURRENCY")
	}
	if !strings.Contains(err.Error(), `WORKER_CONCURRENCY must be a valid integer`) {
		t.Fatalf("expected integer parse error, got: %v", err)
	}
}

func TestLoadFailsOnInvalidBooleanEnv(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://example")
	t.Setenv("TRACING_ENABLED", "maybe")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid TRACING_ENABLED")
	}
	if !strings.Contains(err.Error(), `TRACING_ENABLED must be a valid boolean`) {
		t.Fatalf("expected boolean parse error, got: %v", err)
	}
}
