package worker

import "github.com/pranavko12/taskforge/internal/metrics"

func incConcurrencyThrottled(queue string) {
	metrics.IncConcurrencyThrottled(queue)
}

func incRateThrottled(queue string) {
	metrics.IncRateThrottled(queue)
}
