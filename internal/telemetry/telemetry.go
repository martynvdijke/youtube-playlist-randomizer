package telemetry

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const serviceName = "youtube-playlist-randomizer"

type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Tracer         trace.Tracer
	Meter          metric.Meter

	// HTTP metrics
	HTTPRequestCount  metric.Int64Counter
	HTTPRequestDur    metric.Float64Histogram
	HTTPRequestsInFly metric.Int64UpDownCounter

	// Quota metrics
	QuotaUsed      metric.Int64Gauge
	QuotaRemaining metric.Int64Gauge
	QuotaLimit     metric.Int64Gauge

	// Job metrics
	JobsCreated   metric.Int64Counter
	JobsCompleted metric.Int64Counter
	JobsPaused    metric.Int64Counter
	JobsFailed    metric.Int64Counter
	ItemsInserted metric.Int64Counter

	// YouTube API call metrics
	YouTubeAPICalls metric.Int64Counter
}

func New() (*Telemetry, error) {
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("service.version", os.Getenv("VERSION")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
		stdouttrace.WithWriter(os.Stderr),
	)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	metricExporter, err := stdoutmetric.New(
		stdoutmetric.WithWriter(os.Stderr),
	)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(10*time.Second),
		)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	tracer := tp.Tracer(serviceName)
	meter := mp.Meter(serviceName)

	t := &Telemetry{
		TracerProvider: tp,
		MeterProvider:  mp,
		Tracer:         tracer,
		Meter:          meter,
	}

	if err := t.initInstruments(); err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Telemetry) initInstruments() error {
	var err error

	t.HTTPRequestCount, err = t.Meter.Int64Counter(
		"http.requests.total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return fmt.Errorf("create http.requests.total: %w", err)
	}

	t.HTTPRequestDur, err = t.Meter.Float64Histogram(
		"http.requests.duration",
		metric.WithDescription("Duration of HTTP requests"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(10, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
	)
	if err != nil {
		return fmt.Errorf("create http.requests.duration: %w", err)
	}

	t.HTTPRequestsInFly, err = t.Meter.Int64UpDownCounter(
		"http.requests.in_flight",
		metric.WithDescription("Number of HTTP requests currently in flight"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return fmt.Errorf("create http.requests.in_flight: %w", err)
	}

	t.QuotaUsed, err = t.Meter.Int64Gauge(
		"quota.used",
		metric.WithDescription("Current API quota used"),
		metric.WithUnit("{unit}"),
	)
	if err != nil {
		return fmt.Errorf("create quota.used: %w", err)
	}

	t.QuotaRemaining, err = t.Meter.Int64Gauge(
		"quota.remaining",
		metric.WithDescription("Current API quota remaining"),
		metric.WithUnit("{unit}"),
	)
	if err != nil {
		return fmt.Errorf("create quota.remaining: %w", err)
	}

	t.QuotaLimit, err = t.Meter.Int64Gauge(
		"quota.limit",
		metric.WithDescription("API quota limit"),
		metric.WithUnit("{unit}"),
	)
	if err != nil {
		return fmt.Errorf("create quota.limit: %w", err)
	}

	t.JobsCreated, err = t.Meter.Int64Counter(
		"jobs.created",
		metric.WithDescription("Total number of jobs created"),
		metric.WithUnit("{job}"),
	)
	if err != nil {
		return fmt.Errorf("create jobs.created: %w", err)
	}

	t.JobsCompleted, err = t.Meter.Int64Counter(
		"jobs.completed",
		metric.WithDescription("Total number of jobs completed"),
		metric.WithUnit("{job}"),
	)
	if err != nil {
		return fmt.Errorf("create jobs.completed: %w", err)
	}

	t.JobsPaused, err = t.Meter.Int64Counter(
		"jobs.paused",
		metric.WithDescription("Total number of jobs paused due to quota"),
		metric.WithUnit("{job}"),
	)
	if err != nil {
		return fmt.Errorf("create jobs.paused: %w", err)
	}

	t.JobsFailed, err = t.Meter.Int64Counter(
		"jobs.failed",
		metric.WithDescription("Total number of jobs failed"),
		metric.WithUnit("{job}"),
	)
	if err != nil {
		return fmt.Errorf("create jobs.failed: %w", err)
	}

	t.ItemsInserted, err = t.Meter.Int64Counter(
		"items.inserted",
		metric.WithDescription("Total number of playlist items inserted"),
		metric.WithUnit("{item}"),
	)
	if err != nil {
		return fmt.Errorf("create items.inserted: %w", err)
	}

	t.YouTubeAPICalls, err = t.Meter.Int64Counter(
		"youtube.api.calls",
		metric.WithDescription("Total number of YouTube API calls"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return fmt.Errorf("create youtube.api.calls: %w", err)
	}

	return nil
}


