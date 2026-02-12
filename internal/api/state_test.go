package api

import "testing"

func TestIsAllowedTransitionValid(t *testing.T) {
	cases := []struct {
		from string
		to   string
	}{
		{StatePending, StateInProgress},
		{StateInProgress, StateCompleted},
		{StateInProgress, StatePending},
		{StateFailed, StateRetrying},
		{StateFailed, StatePending},
		{StateRetrying, StatePending},
		{StateDLQ, StatePending},
	}

	for _, c := range cases {
		if !isAllowedTransition(c.from, c.to) {
			t.Fatalf("expected transition %s -> %s to be allowed", c.from, c.to)
		}
	}
}

func TestIsAllowedTransitionInvalid(t *testing.T) {
	cases := []struct {
		from string
		to   string
	}{
		{StatePending, StateCompleted},
		{StateCompleted, StatePending},
		{StateRetrying, StateCompleted},
		{StateDLQ, StateCompleted},
		{StateDead, StatePending},
	}

	for _, c := range cases {
		if isAllowedTransition(c.from, c.to) {
			t.Fatalf("expected transition %s -> %s to be rejected", c.from, c.to)
		}
	}
}

