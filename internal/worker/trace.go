package worker

import (
	"context"

	"github.com/pranavko12/taskforge/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func StartJobSpan(jobID, queueName, traceparent string) (context.Context, func()) {
	ctx := telemetry.ContextWithTraceparent(traceparent)
	tracer := otel.Tracer("taskforge/worker")
	ctx, span := tracer.Start(ctx, "execute_job",
		attribute.String("job_id", jobID),
		attribute.String("queue", queueName),
	)
	return ctx, span.End
}
