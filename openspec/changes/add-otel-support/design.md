## Context

youtube-playlist-randomizer is a Go 1.26 application using the standard **net/http** library and **SQLite** via `modernc.org/sqlite` (pure Go, no CGO). It already has a solid OTel setup in `internal/telemetry/telemetry.go`: OTLP HTTP trace export (`otlptracehttp`), OTLP HTTP metric export (`otlpmetrichttp`), `otelhttp` for HTTP instrumentation (incoming + outgoing), and service name from `OTEL_SERVICE_NAME` env var. It also has an admin UI for OTel settings (`internal/admin/handlers.go` references traces/metrics enabled, sample rate).

Gaps: no OTel logs (no log SDK), no DB query tracing, only HTTP exporters (no gRPC for alignment), no slog bridge. The stable OTel SDK is at v1.44.0, contrib instrumentation at v0.67.0.

## Goals / Non-Goals

**Goals:**
- Add gRPC exporters (`otlptracegrpc`, `otlpmetricgrpc`) for gRPC primary alignment with shared convention
- OTel logs with OTLP export (`otlploggrpc`/`otlploghttp`) and slog bridge for log-to-trace correlation
- DB query tracing — wrap SQLite (modernc) queries with OTel spans
- Verify and supplement OTel metric instruments for HTTP request count and duration
- Standard OTel env var support: `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_TRACES_SAMPLER`, `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_SERVICE_NAME`
- Graceful degradation: no OTel config → noop, partial failure → warn + fallback
- Unit tests for telemetry init and integration test for exporter configuration
- All existing tests pass, CI stays green

**Non-Goals:**
- Not replacing any existing logging infrastructure — OTel logs are additive
- Not re-instrumenting HTTP requests (otelhttp already handles this)
- Not adding OTel auto-instrumentation agents or sidecars
- Not changing the Dockerfile — OTel config is env-var driven
- Not modifying the admin UI OTel settings (existing traces/metrics toggles remain)

## Decisions

**Decision 1: Add OTLP gRPC exporters alongside existing HTTP exporters**

Both `otlptracegrpc`/`otlpmetricgrpc`/`otlploggrpc` and `otlptracehttp`/`otlpmetrichttp`/`otlploghttp` will be supported. The protocol is selected via `OTEL_EXPORTER_OTLP_PROTOCOL` (default: `grpc`).

Rationale: gRPC is the default OTel protocol and the most efficient for high-throughput. The existing HTTP exporters remain as fallback. Supporting both aligns with the shared convention.

Alternative considered: Keep HTTP-only. Rejected: gRPC primary is the shared alignment convention.

**Decision 2: OTel logs with OTLP export and slog bridge**

Add OTel logs SDK (`otel/log v0.20.0`, `sdk/log v0.20.0`) with OTLP log export (`otlploggrpc`/`otlploghttp`) and the OTel slog bridge to route slog records through the OTel logs SDK with automatic trace context injection.

Rationale: Bridging slog to OTel logs provides log-to-trace correlation without changing every log call site. This completes the third pillar.

**Decision 3: DB query tracing via helper function wrapper**

Create a DB query tracing helper with a `TraceDBQuery(ctx, operation, dbFunc)` function that wraps a SQLite (modernc) query in an OTel span.

Rationale: A wrapper function allows per-query opt-in without touching every call site at once. Key queries (playlist lookups, randomization queries) will be wrapped first.

**Decision 4: Config via standard OTel env vars only**

The app relies on the Go OTel SDK's automatic env var detection. Do NOT duplicate OTEL_* vars in app config.

Rationale: The OTel SDK already reads all standard env vars. The existing admin UI settings (traces/metrics enabled, sample rate) are app-level toggles that complement but do not replace the standard env vars.

**Decision 5: Extend internal/telemetry/telemetry.go**

- `internal/telemetry/telemetry.go`: extend to support gRPC exporters, add logger provider, add slog bridge
- `main.go`: update `initTelemetry()` call and shutdown (otelhttp already integrated)
- New file for DB query tracing helper

Rationale: The existing telemetry.go is the natural home for all OTel initialization. Extending it keeps concerns centralized.

**Decision 6: Verify and supplement metric instruments**

The existing `otelhttp` instrumentation may already provide some HTTP metrics. Verify what's present and add explicit OTel metric instruments for HTTP request count (`otel_http_requests_total`) and duration (`otel_http_request_duration_seconds`) if not already present.

Rationale: otelhttp provides spans but not always custom metric instruments. Explicit instruments ensure consistent metric naming across all projects.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| OTLP gRPC exporter connection blocks startup | Move exporter connection to background goroutine with timeout; server starts with noop fallback |
| Adding logs to existing telemetry.go increases complexity | Keep init functions modular within telemetry.go |
| DB query tracing adds overhead to every query | No overhead when no exporter is registered; sampling reduces overhead in production |
| Logs SDK is still v0.20.0 (unstable) | Pin version explicitly; API may change in future |
| Existing admin UI OTel settings may conflict with standard env vars | Document the relationship: admin UI toggles are app-level on/off; env vars configure the SDK details |
| modernc.org/sqlite (pure Go) may have different tracing hooks than CGO sqlite | TraceDBQuery wraps at the query call site, not the driver level — works with any SQLite driver |

## Open Questions

- Should the admin UI OTel settings be extended to include logs on/off? — Consider during implementation
- Should we add a health check endpoint for the OTel exporter? — Deferred
- Should the existing `central-logging` openspec change be merged with this one? — Keep separate; central-logging is about log aggregation, this is about OTel logs