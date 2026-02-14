package retry

import (
	"errors"
	"math"
	"time"
)

type Policy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	Backoff      float64
	MaxDelay     time.Duration
	Jitter       bool
}

func (p Policy) Validate() error {
	if p.MaxAttempts < 1 {
		return errors.New("maxAttempts must be >= 1")
	}
	if p.InitialDelay < 0 {
		return errors.New("initialDelay must be >= 0")
	}
	if p.Backoff < 1 {
		return errors.New("backoff must be >= 1")
	}
	if p.MaxDelay < 0 {
		return errors.New("maxDelay must be >= 0")
	}
	if p.MaxDelay < p.InitialDelay {
		return errors.New("maxDelay must be >= initialDelay")
	}
	return nil
}

// NextDelay returns the delay for the given 1-based attempt number.
func NextDelay(attempt int, p Policy) time.Duration {
	if attempt < 1 {
		return 0
	}
	base := float64(p.InitialDelay)
	if base < 0 {
		base = 0
	}
	exp := float64(attempt - 1)
	delay := base * math.Pow(p.Backoff, exp)
	if p.MaxDelay > 0 && delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	return time.Duration(delay)
}

func NextRunAt(now time.Time, attempt int, p Policy) time.Time {
	return now.Add(NextDelay(attempt, p))
}
