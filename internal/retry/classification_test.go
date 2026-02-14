package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
)

func TestClassifyErrorExplicitRetryable(t *testing.T) {
	err := Retryable(errors.New("upstream unavailable"))
	if got := ClassifyError(err); got != ClassRetryable {
		t.Fatalf("expected retryable, got %s", got)
	}
}

func TestClassifyErrorExplicitTerminal(t *testing.T) {
	err := Terminal(errors.New("payload validation failed"))
	if got := ClassifyError(err); got != ClassTerminal {
		t.Fatalf("expected terminal, got %s", got)
	}
}

func TestClassifyErrorKnownRetryableFailures(t *testing.T) {
	cases := []error{
		context.Canceled,
		context.DeadlineExceeded,
		net.ErrClosed,
		syscall.ECONNRESET,
		syscall.ETIMEDOUT,
	}

	for _, err := range cases {
		if got := ClassifyError(err); got != ClassRetryable {
			t.Fatalf("expected retryable for %T (%v), got %s", err, err, got)
		}
	}
}

func TestClassifyErrorDefaultsToTerminal(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", io.EOF)
	if got := ClassifyError(err); got != ClassTerminal {
		t.Fatalf("expected terminal, got %s", got)
	}
}
