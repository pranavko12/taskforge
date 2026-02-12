package api

const (
	StatePending    = "PENDING"
	StateInProgress = "IN_PROGRESS"
	StateCompleted  = "COMPLETED"
	StateFailed     = "FAILED"
	StateRetrying   = "RETRYING"
	StateDLQ        = "DLQ"
	StateDead       = "DEAD"
)

func isAllowedTransition(fromState, toState string) bool {
	if fromState == toState {
		return true
	}

	switch fromState {
	case StatePending:
		return toState == StateInProgress || toState == StateDLQ
	case StateInProgress:
		return toState == StateCompleted || toState == StateFailed || toState == StateRetrying || toState == StateDLQ || toState == StatePending
	case StateFailed:
		return toState == StateRetrying || toState == StateDLQ || toState == StatePending
	case StateRetrying:
		return toState == StatePending || toState == StateDLQ
	case StateDLQ:
		return toState == StatePending
	default:
		return false
	}
}
