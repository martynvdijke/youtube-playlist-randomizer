## 1. Add OTel Dependencies

- [x] 1.1 Add `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` for gRPC trace export (otlptracehttp already present)
- [x] 1.2 Add `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` for gRPC metric export (otlpmetrichttp already present)
- [x] 1.3 Add `go.opentelemetry.io/otel/log`, `sdk/log`, `otlploggrpc`, `otlploghttp` for OTel logs
- [x] 1.4 Add OTel slog bridge dependency for log-to-trace correlation
- [x] 1.5 Run `go mod tidy` to resolve all new dependencies

## 2. Extend internal/telemetry/telemetry.go — gRPC + Logs

- [x] 2.1 Add gRPC trace exporter selection when `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` (default), keeping HTTP as fallback
- [x] 2.2 Add gRPC metric exporter selection alongside existing HTTP exporter
- [x] 2.3 Initialize OTel logger provider with OTLP log exporter (gRPC primary, HTTP secondary)
- [x] 2.4 Configure `OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG` via OTel SDK sampler
- [x] 2.5 Configure `OTEL_RESOURCE_ATTRIBUTES` via OTel SDK resource detection, with `OTEL_SERVICE_NAME` defaulting to the app name
- [x] 2.6 Add graceful shutdown: `defer tp.Shutdown()` with timeout, flush pending spans/metrics/logs
- [x] 2.7 Add graceful degradation: if OTLP exporter connection fails, log warning and fall back to noop

## 3. Verify and Supplement OTel Metrics

- [x] 3.1 Verify what HTTP metrics otelhttp already provides
- [x] 3.2 Create explicit OTel meter instruments for HTTP request count (`http.requests.total`) and duration (`http.requests.duration`) with method/path/status labels - already present in initInstruments() and middleware.go
- [x] 3.3 Verify metric export via OTLP pipeline (gRPC + HTTP) - handled by PeriodicReader in MeterProvider

## 4. Verify HTTP Request Tracing (Already Integrated)

- [x] 4.1 Verify custom telemetry middleware is correctly wired in `main.go` for incoming requests
- [x] 4.2 Verify outgoing HTTP clients (YouTube API calls) - plain http.Client without otelhttp transport (existing behavior, out of scope)
- [x] 4.3 Verify trace context propagation from incoming `traceparent` headers - handled by OTel TracerProvider via context propagation

## 5. Add DB Query Tracing

- [ ] 5.1 Create a DB query tracing helper with `TraceDBQuery(ctx, operation, dbFunc)` function
- [ ] 5.2 Wrap key DB queries (playlist lookups, randomization queries) with tracing spans
- [ ] 5.3 Ensure spans link to parent request trace via context propagation

## 6. Add OTel Logs

- [ ] 6.1 Initialize OTel logger provider with OTLP log exporter (gRPC primary, HTTP secondary)
- [ ] 6.2 Wire the OTel slog bridge so slog log records flow through the OTel logs SDK with trace context
- [ ] 6.3 Verify log-to-trace correlation: logs emitted within a span include trace_id and span_id

## 7. Write Tests

- [ ] 7.1 Write unit tests for `initTelemetry()`: OTLP gRPC/HTTP config, noop fallback, sampling config, resource attributes
- [ ] 7.2 Write unit test for DB query tracing helper
- [ ] 7.3 Write integration test that starts the server with OTel env vars and verifies metrics are exported
- [ ] 7.4 Write test that verifies graceful degradation (unreachable OTLP endpoint doesn't crash server)

## 8. Docker & Verification

- [ ] 8.1 Update `Dockerfile`: document `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` env vars
- [ ] 8.2 Run `go vet ./...` — no new warnings
- [ ] 8.3 Run `go test ./...` — all tests pass
- [ ] 8.4 Run `go build -o /dev/null .` — binary compiles cleanly
- [ ] 8.5 Commit all changes with a conventional commit message