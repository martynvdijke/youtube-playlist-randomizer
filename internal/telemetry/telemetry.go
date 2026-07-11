package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
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
	// Endpoint is the OTLP endpoint URL.
	// Falls back to OTEL_EXPORTER_OTLP_ENDPOINT env var if empty.
	Endpoint string

	// OTLPProtocol selects the OTLP transport protocol.
	// Falls back to OTEL_EXPORTER_OTLP_PROTOCOL env var if empty.
	// Supported: "grpc" (default), "http".
	OTLPProtocol string

	// TracesEnabled controls whether traces are exported.
	// When false, a no-op TracerProvider is used.
	TracesEnabled bool

	// MetricsEnabled controls whether metrics are exported.
	// When false, a no-op MeterProvider is used.
	MetricsEnabled bool

	// LogsEnabled controls whether OTel logs are exported.
	// When false, a no-op LoggerProvider is used.
	LogsEnabled bool

	// TraceSampleRate is the probability (0.0–1.0) for trace sampling.
	// Defaults to 1.0 (export all traces) when the env var sampler is not set.
	TraceSampleRate float64

	// Headers are custom headers sent with every OTLP export request.
	// Map keys are header names, values are header values.
	Headers map[string]string
}

// Telemetry bundles OTel trace, metric, and log providers with pre-created
// instruments for HTTP monitoring, quota tracking, jobs, and YouTube API calls.
type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	LoggerProvider *sdklog.LoggerProvider
	Tracer         trace.Tracer
	Meter          metric.Meter
	Logger         log.Logger

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

// DefaultSettings returns a Settings with sensible defaults:
// traces, metrics, and logs enabled; sample rate 1.0; gRPC protocol.
func DefaultSettings() Settings {
	return Settings{
		TracesEnabled:   true,
		MetricsEnabled:  true,
		LogsEnabled:     true,
		TraceSampleRate: 1.0,
	}
}

// New creates a Telemetry instance with the given settings.
//
// Endpoint detection order: cfg.Endpoint → OTEL_EXPORTER_OTLP_ENDPOINT env var.
// Protocol detection order: cfg.OTLPProtocol → OTEL_EXPORTER_OTLP_PROTOCOL env var → "grpc".
//
// Exporter creation failures are handled gracefully: a warning is printed and
// a no-op provider is used so the server never crashes due to OTel setup.
func New(cfg Settings) (*Telemetry, error) {
	name := serviceName()

	if cfg.TraceSampleRate <= 0 || cfg.TraceSampleRate > 1 {
		cfg.TraceSampleRate = 1.0
	}

	// --- Resource ---
	res, err := newResource(name)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	proto := cfg.OTLPProtocol
	if proto == "" {
		proto = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}
	if proto == "" {
		proto = "grpc" // OTel default
	}

	hasExportTarget := endpoint != ""

	// --- Providers ---
	tp := newTracerProvider(hasExportTarget, cfg.TracesEnabled, proto, endpoint, cfg.Headers, res, cfg.TraceSampleRate)
	mp := newMeterProvider(hasExportTarget, cfg.MetricsEnabled, proto, endpoint, cfg.Headers, res)
	lp := newLoggerProvider(hasExportTarget, cfg.LogsEnabled, proto, endpoint, cfg.Headers, res)

	if tp != nil {
		otel.SetTracerProvider(tp)
	}
	if mp != nil {
		otel.SetMeterProvider(mp)
	}

	// Wire the slog bridge: slog log records flow through OTel logs SDK
	// with trace context correlation. The lp is never nil (newLoggerProvider
	// always returns a valid provider).
	slogLogger := otelslog.NewLogger(name,
		otelslog.WithLoggerProvider(lp),
		otelslog.WithSource(false),
	)
	_ = slogLogger // kept for future use when migrating to slog

	tracer := tp.Tracer(name)
	meter := mp.Meter(name)
	logger := lp.Logger(name)

	t := &Telemetry{
		TracerProvider: tp,
		MeterProvider:  mp,
		LoggerProvider: lp,
		Tracer:         tracer,
		Meter:          meter,
		Logger:         logger,
		cfg:            cfg,
	}

	if err := t.initInstruments(); err != nil {
		return nil, err
	}

	return t, nil
}

// newResource builds an OTel Resource from service info, version, and the
// OTEL_RESOURCE_ATTRIBUTES environment variable.
func newResource(name string) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(name),
		attribute.String("service.version", os.Getenv("VERSION")),
	}

	// Parse OTEL_RESOURCE_ATTRIBUTES (key1=val1,key2=val2)
	if ra := os.Getenv("OTEL_RESOURCE_ATTRIBUTES"); ra != "" {
		for _, pair := range strings.Split(ra, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) == 2 {
				attrs = append(attrs, attribute.String(strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])))
			}
		}
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(attrs...),
	)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// newTracerProvider creates a TracerProvider with the appropriate OTLP exporter
// (gRPC or HTTP), falling back to a no-op provider when exporting is disabled
// or exporter creation fails.
func newTracerProvider(hasExportTarget, enabled bool, proto, endpoint string, headers map[string]string, res *resource.Resource, sampleRate float64) *sdktrace.TracerProvider {
	if !enabled || !hasExportTarget {
		return sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
	}

	exporter, err := newTraceExporter(proto, endpoint, headers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: trace exporter creation failed (%v), using noop\n", err)
		return sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
		)
	}

	sampler := newSampler(sampleRate)

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
}

// newTraceExporter creates the appropriate trace exporter based on protocol.
func newTraceExporter(proto, endpoint string, headers map[string]string) (sdktrace.SpanExporter, error) {
	switch proto {
	case "grpc":
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithTimeout(10 * time.Second),
		}
		if endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpointURL(endpoint))
		}
		if len(headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(headers))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return otlptracegrpc.New(ctx, opts...)

	default: // "http"
		opts := []otlptracehttp.Option{
			otlptracehttp.WithTimeout(10 * time.Second),
		}
		if endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpointURL(endpoint))
		}
		if len(headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(headers))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return otlptracehttp.New(ctx, opts...)
	}
}

// newSampler returns an sdktrace.Sampler based on the OTEL_TRACES_SAMPLER and
// OTEL_TRACES_SAMPLER_ARG env vars, falling back to the given sampleRate.
func newSampler(sampleRate float64) sdktrace.Sampler {
	sampler := os.Getenv("OTEL_TRACES_SAMPLER")
	arg := os.Getenv("OTEL_TRACES_SAMPLER_ARG")

	switch sampler {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		ratio := parseSampleRatio(arg, sampleRate)
		return sdktrace.TraceIDRatioBased(ratio)
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		ratio := parseSampleRatio(arg, sampleRate)
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	default:
		// Default: parent-based trace ID ratio
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))
	}
}

// parseSampleRatio parses a ratio string or falls back to the default.
func parseSampleRatio(arg string, fallback float64) float64 {
	if arg == "" {
		return fallback
	}
	r, err := strconv.ParseFloat(arg, 64)
	if err != nil || r < 0 || r > 1 {
		return fallback
	}
	return r
}

// newMeterProvider creates a MeterProvider with the appropriate OTLP exporter
// (gRPC or HTTP), falling back to a no-op provider when exporting is disabled
// or exporter creation fails.
func newMeterProvider(hasExportTarget, enabled bool, proto, endpoint string, headers map[string]string, res *resource.Resource) *sdkmetric.MeterProvider {
	if !enabled || !hasExportTarget {
		return sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
		)
	}

	exporter, err := newMetricExporter(proto, endpoint, headers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: metric exporter creation failed (%v), using noop\n", err)
		return sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
		)
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(60*time.Second),
		)),
		sdkmetric.WithResource(res),
	)
}

// newMetricExporter creates the appropriate metric exporter based on protocol.
func newMetricExporter(proto, endpoint string, headers map[string]string) (sdkmetric.Exporter, error) {
	switch proto {
	case "grpc":
		opts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithTimeout(10 * time.Second),
		}
		if endpoint != "" {
			opts = append(opts, otlpmetricgrpc.WithEndpointURL(endpoint))
		}
		if len(headers) > 0 {
			opts = append(opts, otlpmetricgrpc.WithHeaders(headers))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return otlpmetricgrpc.New(ctx, opts...)

	default: // "http"
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithTimeout(10 * time.Second),
		}
		if endpoint != "" {
			opts = append(opts, otlpmetrichttp.WithEndpointURL(endpoint))
		}
		if len(headers) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(headers))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return otlpmetrichttp.New(ctx, opts...)
	}
}

// newLoggerProvider creates a LoggerProvider with the appropriate OTLP log
// exporter (gRPC or HTTP), falling back to a no-op provider when log exporting
// is disabled or exporter creation fails.
func newLoggerProvider(hasExportTarget, enabled bool, proto, endpoint string, headers map[string]string, res *resource.Resource) *sdklog.LoggerProvider {
	if !enabled || !hasExportTarget {
		return sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
		)
	}

	exporter, err := newLogExporter(proto, endpoint, headers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: log exporter creation failed (%v), using noop\n", err)
		return sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
		)
	}

	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter,
			sdklog.WithExportInterval(5*time.Second),
		)),
		sdklog.WithResource(res),
	)
}

// newLogExporter creates the appropriate log exporter based on protocol.
func newLogExporter(proto, endpoint string, headers map[string]string) (sdklog.Exporter, error) {
	switch proto {
	case "grpc":
		opts := []otlploggrpc.Option{
			otlploggrpc.WithTimeout(10 * time.Second),
		}
		if endpoint != "" {
			opts = append(opts, otlploggrpc.WithEndpointURL(endpoint))
		}
		if len(headers) > 0 {
			opts = append(opts, otlploggrpc.WithHeaders(headers))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return otlploggrpc.New(ctx, opts...)

	default: // "http"
		opts := []otlploghttp.Option{
			otlploghttp.WithTimeout(10 * time.Second),
		}
		if endpoint != "" {
			opts = append(opts, otlploghttp.WithEndpointURL(endpoint))
		}
		if len(headers) > 0 {
			opts = append(opts, otlploghttp.WithHeaders(headers))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return otlploghttp.New(ctx, opts...)
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
