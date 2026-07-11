## Why

youtube-playlist-randomizer already has a solid OTel tracing and metrics foundation: OTLP HTTP trace export (`otlptracehttp`), OTLP HTTP metric export (`otlpmetrichttp`), `otelhttp` for HTTP instrumentation, and a telemetry setup in `internal/telemetry/telemetry.go` with service name from `OTEL_SERVICE_NAME` env var. It even has an admin UI for OTel settings (traces/metrics enabled, sample rate). However, the implementation is incomplete: there are no OTel logs (no log SDK), no DB query tracing, only HTTP exporters (no gRPC for alignment), and the slog bridge is not wired.

Completing the OTel support — adding logs, DB query tracing, gRPC exporters, and the slog bridge — unlocks full observability across all three pillars (traces, metrics, logs) with structured log correlation. Aligning on gRPC-primary OTLP export matches the shared convention across all projects.

## What Changes

- **Add OTLP gRPC exporters** (`otlptracegrpc`, `otlpmetricgrpc`) alongside existing HTTP exporters for gRPC primary alignment
- **Add OTel logs** — OTel logs SDK with OTLP log export (`otlploggrpc`/`otlploghttp`) and slog bridge for log-to-trace correlation
- **Add DB query tracing** — instrument SQLite (modernc.org/sqlite) queries with OTel spans to capture DB latency in traces
- **Add OTel metric instruments** for HTTP request count (`otel_http_requests_total`) and duration (`otel_http_request_duration_seconds`) (may already have some via otelhttp — verify and supplement)
- **Add configurable sampling and resource attributes** — support `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`, `OTEL_RESOURCE_ATTRIBUTES` env vars
- **Graceful degradation** — if OTel is not configured (no OTLP endpoint), fall back to no-op propagation without crashing
- **Tests** — unit tests for telemetry initialization and middleware, integration test verifying trace/metric/log export configuration

## Capabilities

### New Capabilities
- `otel-telemetry`: OpenTelemetry-based distributed tracing, metrics, and logs with configurable OTLP export (gRPC + HTTP), HTTP instrumentation, DB query tracing, and slog bridge

### Modified Capabilities
<!-- No existing capabilities are having their requirements changed -->

## Impact

- `go.mod`: add `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`, `otlpmetricgrpc`, `go.opentelemetry.io/otel/log`, `sdk/log`, `otlploggrpc`, `otlploghttp`, OTel slog bridge
- `internal/telemetry/telemetry.go`: extend to support gRPC exporters, add logger provider, add slog bridge
- `main.go`: update telemetry initialization and shutdown (otelhttp already integrated)
- New file for DB query tracing helper
- New env vars: `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_TRACES_SAMPLER`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`
- `Dockerfile`: add `OTEL_*` env vars documentation
- CI: no pipeline changes needed — OTel is a pure code addition