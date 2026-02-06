package worker

import "expvar"

var (
	metricConcurrencyThrottled = expvar.NewInt("worker_concurrency_throttled_total")
	metricRateThrottled        = expvar.NewInt("worker_rate_throttled_total")
)
