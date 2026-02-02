package retry

import (
	"math/rand"
	"testing"
	"time"
)

func TestNextDelayNoJitter(t *testing.T) {
	p := Policy{
		MaxAttempts:       5,
		InitialDelay:      1 * time.Second,
		BackoffMultiplier: 2,
		MaxDelay:          10 * time.Second,
		Jitter:            0,
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
		if got := NextDelay(c.attempt, p, nil); got != c.want {
			t.Fatalf("attempt %d: expected %v, got %v", c.attempt, c.want, got)
		}
	}
}

func TestNextRunAtDeterministicWithSeed(t *testing.T) {
	p := Policy{
		MaxAttempts:       3,
		InitialDelay:      1 * time.Second,
		BackoffMultiplier: 2,
		MaxDelay:          10 * time.Second,
		Jitter:            0.25,
	}

	now := time.Date(2026, 2, 2, 12, 0, 0, 0, time.UTC)

	rng1 := rand.New(rand.NewSource(42))
	rng2 := rand.New(rand.NewSource(42))

	got1 := NextRunAt(now, 2, p, rng1)
	got2 := NextRunAt(now, 2, p, rng2)

	if !got1.Equal(got2) {
		t.Fatalf("expected deterministic times, got %v and %v", got1, got2)
	}
}
