package metrics

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once sync.Once

	jobAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "taskforge_job_attempts_total",
			Help: "Total job attempts.",
		},
		[]string{"queue"},
	)
	jobSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "taskforge_job_success_total",
			Help: "Total successful jobs.",
		},
		[]string{"queue"},
	)
	jobFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "taskforge_job_failure_total",
			Help: "Total failed jobs.",
		},
		[]string{"queue"},
	)
	jobRuntime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "taskforge_job_runtime_seconds",
			Help:    "Job runtime histogram in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"queue"},
	)
	jobTimeInQueue = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "taskforge_job_time_in_queue_seconds",
			Help:    "Time in queue histogram in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"queue"},
	)
	workerUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "taskforge_worker_utilization",
			Help: "Worker utilization (in-flight / concurrency).",
		},
		[]string{"queue"},
	)
	concurrencyThrottled = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "taskforge_worker_concurrency_throttled_total",
			Help: "Total times work was throttled by concurrency.",
		},
		[]string{"queue"},
	)
	rateThrottled = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "taskforge_worker_rate_throttled_total",
			Help: "Total times work was throttled by rate limit.",
		},
		[]string{"queue"},
	)
)

func Register(reg *prometheus.Registry) {
	once.Do(func() {
		reg.MustRegister(
			jobAttempts,
			jobSuccess,
			jobFailure,
			jobRuntime,
			jobTimeInQueue,
			workerUtilization,
			concurrencyThrottled,
			rateThrottled,
		)
	})
}

func IncAttempts(queue string) {
	jobAttempts.WithLabelValues(queue).Inc()
}

func IncSuccess(queue string) {
	jobSuccess.WithLabelValues(queue).Inc()
}

func IncFailure(queue string) {
	jobFailure.WithLabelValues(queue).Inc()
}

func ObserveRuntime(queue string, seconds float64) {
	jobRuntime.WithLabelValues(queue).Observe(seconds)
}

func ObserveTimeInQueue(queue string, seconds float64) {
	jobTimeInQueue.WithLabelValues(queue).Observe(seconds)
}

func SetWorkerUtilization(queue string, value float64) {
	workerUtilization.WithLabelValues(queue).Set(value)
}

func IncConcurrencyThrottled(queue string) {
	concurrencyThrottled.WithLabelValues(queue).Inc()
}

func IncRateThrottled(queue string) {
	rateThrottled.WithLabelValues(queue).Inc()
}

type QueueDLQProvider interface {
	QueueDepth(ctx context.Context) (int64, error)
	DLQCount(ctx context.Context) (int, error)
}

type QueueDLQCollector struct {
	queueName string
	provider  QueueDLQProvider
	depthDesc *prometheus.Desc
	dlqDesc   *prometheus.Desc
}

func NewQueueDLQCollector(queueName string, provider QueueDLQProvider) *QueueDLQCollector {
	return &QueueDLQCollector{
		queueName: queueName,
		provider:  provider,
		depthDesc: prometheus.NewDesc(
			"taskforge_queue_depth",
			"Current queue depth.",
			[]string{"queue"},
			nil,
		),
		dlqDesc: prometheus.NewDesc(
			"taskforge_dlq_count",
			"Current DLQ count.",
			nil,
			nil,
		),
	}
}

func (c *QueueDLQCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.depthDesc
	ch <- c.dlqDesc
}

func (c *QueueDLQCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	if depth, err := c.provider.QueueDepth(ctx); err == nil {
		ch <- prometheus.MustNewConstMetric(c.depthDesc, prometheus.GaugeValue, float64(depth), c.queueName)
	}
	if count, err := c.provider.DLQCount(ctx); err == nil {
		ch <- prometheus.MustNewConstMetric(c.dlqDesc, prometheus.GaugeValue, float64(count))
	}
}
