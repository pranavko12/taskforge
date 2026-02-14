package retry

import (
	"context"
	"errors"
	"net"
	"syscall"
)

type FailureClass string

const (
	ClassRetryable FailureClass = "retryable"
	ClassTerminal  FailureClass = "terminal"
)

type retryableError struct {
	err error
}

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }

type terminalError struct {
	err error
}

func (e terminalError) Error() string { return e.err.Error() }
func (e terminalError) Unwrap() error { return e.err }

func Retryable(err error) error {
	if err == nil {
		return nil
	}
	return retryableError{err: err}
}

func Terminal(err error) error {
	if err == nil {
		return nil
	}
	return terminalError{err: err}
}

func ClassifyError(err error) FailureClass {
	if err == nil {
		return ClassTerminal
	}

	var explicitRetry retryableError
	if errors.As(err, &explicitRetry) {
		return ClassRetryable
	}
	var explicitTerminal terminalError
	if errors.As(err, &explicitTerminal) {
		return ClassTerminal
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ClassRetryable
	}
	if errors.Is(err, net.ErrClosed) {
		return ClassRetryable
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ClassRetryable
	}

	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return ClassRetryable
	}

	return ClassTerminal
}
