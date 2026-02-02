package retry

import (
	"errors"
	"math"
	"math/rand"
	"time"
)

type Policy struct {
	MaxAttempts       int
	InitialDelay      time.Duration
	BackoffMultiplier float64
	MaxDelay          time.Duration
	Jitter            float64
}

func (p Policy) Validate() error {
	if p.MaxAttempts < 1 {
		return errors.New("maxAttempts must be >= 1")
	}
	if p.InitialDelay < 0 {
		return errors.New("initialDelay must be >= 0")
	}
	if p.BackoffMultiplier < 1 {
		return errors.New("backoffMultiplier must be >= 1")
	}
	if p.MaxDelay < 0 {
		return errors.New("maxDelay must be >= 0")
	}
	if p.MaxDelay < p.InitialDelay {
		return errors.New("maxDelay must be >= initialDelay")
	}
	if p.Jitter < 0 || p.Jitter > 1 {
		return errors.New("jitter must be between 0 and 1")
	}
	return nil
}

// NextDelay returns the delay for the given 1-based attempt number.
func NextDelay(attempt int, p Policy, rng *rand.Rand) time.Duration {
	if attempt < 1 {
		return 0
	}
	base := float64(p.InitialDelay)
	if base < 0 {
		base = 0
	}
	exp := float64(attempt - 1)
	delay := base * math.Pow(p.BackoffMultiplier, exp)
	if p.MaxDelay > 0 && delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	if p.Jitter > 0 && rng != nil {
		j := (rng.Float64()*2 - 1) * p.Jitter
		delay = delay * (1 + j)
		if delay < 0 {
			delay = 0
		}
	}
	return time.Duration(delay)
}

func NextRunAt(now time.Time, attempt int, p Policy, rng *rand.Rand) time.Time {
	return now.Add(NextDelay(attempt, p, rng))
}
