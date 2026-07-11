package telemetry

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.statusCode = http.StatusOK
		rw.written = true
	}
	return rw.ResponseWriter.Write(b)
}

func routePattern(r *http.Request) string {
	path := r.URL.Path
	if r.Method == "" {
		return path
	}
	return fmt.Sprintf("%s %s", r.Method, path)
}

func (t *Telemetry) Middleware(next http.Handler) http.Handler {
	if t == nil || t.Tracer == nil || t.Meter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		pattern := routePattern(r)

		ctx, span := t.Tracer.Start(r.Context(), pattern)
		defer span.End()

		host, _, _ := net.SplitHostPort(r.Host)
		if host == "" {
			host = r.Host
		}
		span.SetAttributes(
			semconv.HTTPMethod(r.Method),
			semconv.HTTPURL(r.URL.String()),
			semconv.NetHostName(host),
			semconv.UserAgentOriginal(r.UserAgent()),
		)

		t.HTTPRequestsInFly.Add(ctx, 1)
		t.HTTPRequestCount.Add(ctx, 1, metric.WithAttributes(
			semconv.HTTPMethod(r.Method),
			attribute.String("http.route", pattern),
		))

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r.WithContext(ctx))

		duration := time.Since(start).Milliseconds()
		status := rw.statusCode

		t.HTTPRequestsInFly.Add(ctx, -1)

		t.HTTPRequestDur.Record(ctx, float64(duration), metric.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.route", pattern),
			attribute.Int("http.status_code", status),
		))

		span.SetAttributes(
			semconv.HTTPStatusCode(status),
		)
		if status >= 400 {
			span.SetStatus(codes.Error, http.StatusText(status))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	})
}

func (t *Telemetry) RecordQuotaMetrics(ctx context.Context, used, limit, remaining int) {
	if t == nil || t.Meter == nil {
		return
	}
	t.QuotaUsed.Record(ctx, int64(used))
	t.QuotaRemaining.Record(ctx, int64(remaining))
	t.QuotaLimit.Record(ctx, int64(limit))
}

func (t *Telemetry) RecordYouTubeAPICall(ctx context.Context, operation string) {
	if t == nil || t.Meter == nil {
		return
	}
	t.YouTubeAPICalls.Add(ctx, 1, metric.WithAttributes(
		attribute.String("youtube.operation", operation),
	))
}

func (t *Telemetry) RecordJobCreated(ctx context.Context) {
	if t == nil || t.Meter == nil {
		return
	}
	t.JobsCreated.Add(ctx, 1)
}

func (t *Telemetry) RecordJobCompleted(ctx context.Context, totalItems int) {
	if t == nil || t.Meter == nil {
		return
	}
	t.JobsCompleted.Add(ctx, 1)
	t.ItemsInserted.Add(ctx, int64(totalItems), metric.WithAttributes(
		attribute.String("result", "completed"),
	))
}

func (t *Telemetry) RecordJobPaused(ctx context.Context, done, total int) {
	if t == nil || t.Meter == nil {
		return
	}
	t.JobsPaused.Add(ctx, 1)
}

func (t *Telemetry) RecordJobFailed(ctx context.Context, reason string) {
	if t == nil || t.Meter == nil {
		return
	}
	t.JobsFailed.Add(ctx, 1, metric.WithAttributes(
		attribute.String("error_reason", reason),
	))
}

func (t *Telemetry) RecordItemsInserted(ctx context.Context, count int) {
	if t == nil || t.Meter == nil {
		return
	}
	t.ItemsInserted.Add(ctx, int64(count))
}

func (t *Telemetry) Shutdown(ctx context.Context) {
	if t == nil {
		return
	}

	shutdownWithTimeout := func(name string, fn func(context.Context) error) {
		if fn == nil {
			return
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := fn(ctx); err != nil {
			log.Printf("telemetry: %s shutdown error: %v", name, err)
		}
	}

	if t.MeterProvider != nil {
		shutdownWithTimeout("meter provider", t.MeterProvider.Shutdown)
	}
	if t.TracerProvider != nil {
		shutdownWithTimeout("tracer provider", t.TracerProvider.Shutdown)
	}
	if t.LoggerProvider != nil {
		shutdownWithTimeout("logger provider", t.LoggerProvider.Shutdown)
	}
}

func GetEnvWithDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
