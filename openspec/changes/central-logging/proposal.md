## Why

The application currently lacks any centralized observability — logs are scattered as `log.Printf` calls throughout the codebase, there is no admin interface to view application status, and there is no way for operators to configure logging verbosity or wire application events into OpenTelemetry. As the app grows (with planned email and AI settings endpoints), this gap will only widen. A central logging tab in an admin panel provides operators with real-time visibility into system behavior, surfaces issues faster, and lays the groundwork for structured observability.

## What Changes

- **New Admin Panel** — A new admin section of the UI with a dedicated "Logs" tab, accessible via a nav link in the existing header.
- **Centralized Logging Backend** — A new `internal/logging/` package that captures application events (HTTP requests, job lifecycle, quota events, errors) into a structured, queryable log store with configurable severity levels (DEBUG, INFO, WARN, ERROR).
- **Log Viewer UI** — An HTMX-powered log viewer within the admin panel that displays logs in reverse-chronological order, with filtering by severity, source, and time range. Default view shows WARN and above.
- **Verbosity Control Settings** — UI controls in the admin panel to configure minimum log level (default: WARN). Setting persists to the database.
- **OpenTelemetry Export** — All logs are exported to OpenTelemetry as events/spans so they appear in the configured OTLP endpoint alongside existing metrics and traces.
- **Settings Endpoints Onboarding** — Placeholder settings endpoints for email and AI configurations, each emitting structured logs on every operation and wired into the central logging view.
- **Database Schema Extension** — New tables for log entries and app settings (log verbosity, etc.).

## Capabilities

### New Capabilities
- `admin-panel`: Admin UI shell with navigation, hosting the logs tab and future settings tabs.
- `central-logging`: Structured log capture, storage, query API, and OTEL export.
- `log-viewer`: HTMX-based log browser in the admin panel with filtering and verbosity controls.
- `app-settings`: Persistent key-value settings store for app configuration (e.g., log level).
- `settings-email`: Email settings endpoint wired into central logging.
- `settings-ai`: AI settings endpoint wired into central logging.

### Modified Capabilities

*(No existing specs to modify — first change in this project)*

## Impact

- **New Go package**: `internal/logging/` — log capture, storage, OTEL bridge, HTTP handlers
- **New Go package**: `internal/admin/` — admin panel HTTP handlers
- **New Go files**: Handlers in `main.go` or a new routes file for admin/log endpoints
- **Database migration**: New `logs` and `app_settings` tables in `internal/store/store.go`
- **Frontend**: New static HTML templates for admin panel, log viewer, settings
- **Static assets**: `static/admin/` directory with admin-related HTML fragments
- **CSS**: Additional styles for admin panel and log viewer in `static/style.css`
- **Dependencies**: No new external dependencies; uses existing OTEL SDK
