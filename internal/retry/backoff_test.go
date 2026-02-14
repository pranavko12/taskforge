package retry

import (
	"testing"
	"time"
)

func TestNextDelayNoJitter(t *testing.T) {
	p := Policy{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Second,
		Backoff:      2,
		MaxDelay:     10 * time.Second,
		Jitter:       false,
	}

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 10 * time.Second},
	}

	for _, c := range cases {
		if got := NextDelay(c.attempt, p); got != c.want {
			t.Fatalf("attempt %d: expected %v, got %v", c.attempt, c.want, got)
		}
	}
}

func TestNextRunAtMatchesExpectedForAttempt(t *testing.T) {
	p := Policy{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Second,
		Backoff:      2,
		MaxDelay:     10 * time.Second,
		Jitter:       false,
	}

	now := time.Date(2026, 2, 2, 12, 0, 0, 0, time.UTC)
	got := NextRunAt(now, 4, p)
	want := now.Add(8 * time.Second)

	if !got.Equal(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
