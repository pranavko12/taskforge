package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func ContextWithTraceparent(traceparent string) context.Context {
	if traceparent == "" {
		return context.Background()
	}
	carrier := propagation.MapCarrier{}
	carrier.Set("traceparent", traceparent)
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}
