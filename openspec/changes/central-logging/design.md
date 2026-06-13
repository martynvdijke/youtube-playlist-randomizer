## Context

The application is a Go-based YouTube Playlist Randomizer with an HTMX-driven frontend served from `static/`. It has existing OpenTelemetry instrumentation (traces + metrics via OTLP) in `internal/telemetry/` but no centralized logging — all logs use `log.Printf` scattered across handlers and job runners. There is no admin panel, no settings persistence, and no way for operators to control log verbosity. The app uses SQLite via `internal/store/` with schema migrations in `store.migrate()`.

## Goals / Non-Goals

**Goals:**
- Create an admin panel shell as a nav-accessible section of the UI
- Build a centralized structured logging system that captures all application events
- Provide an HTMX-powered log viewer in the admin panel with filtering by severity, source, time
- Default log viewer shows WARN and above; operator can adjust verbosity
- Export all captured logs to OpenTelemetry (spans + events) alongside existing metrics/traces
- Create placeholder settings endpoints for email and AI configuration that emit structured logs
- Persist log verbosity setting in a new `app_settings` table

**Non-Goals:**
- No authentication/authorization for the admin panel (single-user app)
- No real-time log streaming (WebSocket/SSE) — uses HTMX polling for simplicity
- No log retention policies or archival (logs accumulate indefinitely in DB)
- No email/AI actual send or AI inference — only settings endpoints that log
- No log aggregation beyond OTEL export (no log shipping to external systems beyond the configured OTLP endpoint)

## Decisions

| Decision | Choice | Rationale | Alternatives Considered |
|---|---|---|---|
| Log storage | SQLite `logs` table | Existing SQLite DB, no extra infra. Simple querying for the log viewer. | Flat files (hard to query/filter), dedicated log DB (overkill) |
| Log severity levels | DEBUG, INFO, WARN, ERROR | Standard syslog-like levels compatible with OTEL. WARN is default minimum. | TRACE/CRITICAL (unnecessary granularity for this app) |
| Log viewer pattern | HTMX polling (`hx-trigger="every 5s"`) | Follows existing app pattern; no extra JS dependencies. | SSE (complexity), WebSocket (overkill), full page refresh (poor UX) |
| Admin panel routing | `/admin/*` prefix on HTTP mux | Clean separation from main app routes; easy to extend. | Subdomain (overkill), query param toggle (messy) |
| Log-to-OTEL bridge | Add structured log events as span events on a dedicated `logs` tracer | Leverages existing OTEL SDK without new deps. Events appear alongside existing traces. | Dedicated log appender (requires new exporter), stdout OTEL logs (not queryable in-app) |
| Verbosity persistence | `app_settings` SQLite table with key-value pairs | Simple, extensible for future settings (email, AI, etc.). | Env vars (can't change at runtime), config file (no live update) |
| Settings endpoints | Plain HTTP handlers under `/api/admin/settings/` | Consistent with existing `/api/` pattern. Each emits structured logs. | Single monolithic handler (less extensible) |

## Risks / Trade-offs

- [**SQLite contention**] The `logs` table will see frequent writes. → Mitigation: Use WAL mode (already enabled), batch inserts, and separate write path. Keep log viewer reads on a short timeout to avoid DB lock buildup.
- [**Log table growth**] Unbounded log accumulation could slow queries. → Mitigation: Future work can add a retention/pruning job. Acceptable for single-user app for now.
- [**OTEL overhead**] Every log write triggers a span event export. → Mitigation: Only events at or above the configured minimum level are recorded; OTEL exporter already batches every 60s.
- [**HTMX polling latency**] 5s polling means logs appear up to 5s late. → Mitigation: Acceptable for operational visibility; real-time is not a requirement.
