package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// TraceDBQuery wraps a database operation in an OTel span.
//
// Usage:
//
//	err := telemetry.TraceDBQuery(ctx, tel.Tracer, "GetJob", func(ctx context.Context) error {
//	    return store.GetJob(id)
//	})
//
// The operation name becomes "db.<operation>" in the span name.
func TraceDBQuery(ctx context.Context, tracer trace.Tracer, operation string, fn func(context.Context) error) error {
	if tracer == nil {
		return fn(ctx)
	}
	newCtx, span := tracer.Start(ctx, "db."+operation)
	defer span.End()
	return fn(newCtx)
}
