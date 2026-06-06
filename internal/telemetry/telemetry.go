package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

func serviceName() string {
	if n := os.Getenv("OTEL_SERVICE_NAME"); n != "" {
		return n
	}
	return "youtube-playlist-randomizer"
}

// Settings holds all configurable telemetry options.
type Settings struct {
	// Endpoint is the OTLP HTTP endpoint URL.
	// Falls back to OTEL_EXPORTER_OTLP_ENDPOINT env var if empty.
	Endpoint string

	// TracesEnabled controls whether traces are exported.
	// When false, a no-op TracerProvider is used.
	TracesEnabled bool

	// MetricsEnabled controls whether metrics are exported.
	// When false, a no-op MeterProvider is used.
	MetricsEnabled bool

	// TraceSampleRate is the probability (0.0–1.0) for trace sampling.
	// Defaults to 1.0 (export all traces).
	TraceSampleRate float64

	// Headers are custom headers sent with every OTLP export request.
	// Map keys are header names, values are header values.
	Headers map[string]string
}

type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Tracer         trace.Tracer
	Meter          metric.Meter

	HTTPRequestCount  metric.Int64Counter
	HTTPRequestDur    metric.Float64Histogram
	HTTPRequestsInFly metric.Int64UpDownCounter

	QuotaUsed      metric.Int64Gauge
	QuotaRemaining metric.Int64Gauge
	QuotaLimit     metric.Int64Gauge

	JobsCreated   metric.Int64Counter
	JobsCompleted metric.Int64Counter
	JobsPaused    metric.Int64Counter
	JobsFailed    metric.Int64Counter
	ItemsInserted metric.Int64Counter

	YouTubeAPICalls metric.Int64Counter

	// settings applied on creation
	cfg Settings
}

// New creates a Telemetry instance with the given settings.
// An empty Endpoint falls back to the OTEL_EXPORTER_OTLP_ENDPOINT env var.
// If TracesEnabled or MetricsEnabled is false, the corresponding provider
// is a no-op (no export). TraceSampleRate defaults to 1.0 when zero.
func New(cfg Settings) (*Telemetry, error) {
	name := serviceName()

	if cfg.TraceSampleRate <= 0 || cfg.TraceSampleRate > 1 {
		cfg.TraceSampleRate = 1.0
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(name),
			attribute.String("service.version", os.Getenv("VERSION")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	hasExportTarget := endpoint != ""

	var tp *sdktrace.TracerProvider
	var mp *sdkmetric.MeterProvider

	// --- TracerProvider ---
	if !cfg.TracesEnabled || !hasExportTarget {
		tp = sdktrace.NewTracerProvider()
	} else {
		traceOpts := []otlptracehttp.Option{}
		if len(cfg.Headers) > 0 {
			traceOpts = append(traceOpts, otlptracehttp.WithHeaders(cfg.Headers))
		}

		traceExporter, err := otlptracehttp.New(context.Background(), traceOpts...)
		if err != nil {
			return nil, fmt.Errorf("create trace exporter: %w", err)
		}

		tp = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter,
				sdktrace.WithBatchTimeout(5*time.Second),
			),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.ParentBased(
				sdktrace.TraceIDRatioBased(cfg.TraceSampleRate),
			)),
		)
		otel.SetTracerProvider(tp)
	}

	// --- MeterProvider ---
	if !cfg.MetricsEnabled || !hasExportTarget {
		mp = sdkmetric.NewMeterProvider()
	} else {
		metricOpts := []otlpmetrichttp.Option{}
		if len(cfg.Headers) > 0 {
			metricOpts = append(metricOpts, otlpmetrichttp.WithHeaders(cfg.Headers))
		}

		metricExporter, err := otlpmetrichttp.New(context.Background(), metricOpts...)
		if err != nil {
			return nil, fmt.Errorf("create metric exporter: %w", err)
		}

		mp = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
				sdkmetric.WithInterval(60*time.Second),
			)),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(mp)
	}

	tracer := tp.Tracer(name)
	meter := mp.Meter(name)

	t := &Telemetry{
		TracerProvider: tp,
		MeterProvider:  mp,
		Tracer:         tracer,
		Meter:          meter,
		cfg:            cfg,
	}

	if err := t.initInstruments(); err != nil {
		return nil, err
	}

	return t, nil
}

// DefaultSettings returns a Settings with sensible defaults:
// traces and metrics enabled, sample rate 1.0, no endpoint.
func DefaultSettings() Settings {
	return Settings{
		TracesEnabled:   true,
		MetricsEnabled:  true,
		TraceSampleRate: 1.0,
	}
}

// ParseHeadersJSON parses a JSON object string into a map suitable for OTLP headers.
// Returns an empty map on empty input or parse error.
func ParseHeadersJSON(raw string) map[string]string {
	raw = os.ExpandEnv(raw)
	if raw == "" {
		return nil
	}
	var h map[string]string
	if err := json.Unmarshal([]byte(raw), &h); err != nil {
		return nil
	}
	if len(h) == 0 {
		return nil
	}
	return h
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


