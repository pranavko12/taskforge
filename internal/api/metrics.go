package api

import (
	"context"
	"net/http"
	"sync"

	"github.com/pranavko12/taskforge/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var metricsOnce sync.Once

func metricsHandler(store Store, queue Queue, queueName string) http.Handler {
	metricsOnce.Do(func() {
		reg, ok := prometheus.DefaultRegisterer.(*prometheus.Registry)
		if ok {
			metrics.Register(reg)
		}
		p := queueDLQProvider{store: store, queue: queue, queueName: queueName}
		prometheus.MustRegister(metrics.NewQueueDLQCollector(queueName, p))
	})
	return promhttp.Handler()
}

type queueDLQProvider struct {
	store     Store
	queue     Queue
	queueName string
}

func (p queueDLQProvider) QueueDepth(ctx context.Context) (int64, error) {
	return p.queue.QueueDepth(ctx, p.queueName)
}

func (p queueDLQProvider) DLQCount(ctx context.Context) (int, error) {
	stats, err := p.store.Stats(ctx)
	if err != nil {
		return 0, err
	}
	return stats.DLQ, nil
}
