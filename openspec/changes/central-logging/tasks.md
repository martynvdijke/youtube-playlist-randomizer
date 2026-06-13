## 1. Database Schema Extensions

- [ ] 1.1 Add `logs` table migration in `internal/store/store.go` (columns: id AUTOINCREMENT, timestamp TEXT, severity TEXT, source TEXT, message TEXT, attributes TEXT, created_at TEXT)
- [ ] 1.2 Add `app_settings` table migration in `internal/store/store.go` (columns: key TEXT PRIMARY KEY, value TEXT)
- [ ] 1.3 Add store methods: `InsertLog(entry)`, `GetLogs(minLevel, source, limit, offset)`, `GetLogCounts()` for querying logs
- [ ] 1.4 Add store methods: `GetSetting(key)`, `SetSetting(key, value)` for app_settings

## 2. Centralized Logging Package (`internal/logging/`)

- [ ] 2.1 Create `internal/logging/` package with `Severity` type (DEBUG, INFO, WARN, ERROR) and string conversion
- [ ] 2.2 Implement `Logger` struct that holds store reference, current min severity, and OTEL tracer
- [ ] 2.3 Implement `Logger.Debug()`, `Logger.Info()`, `Logger.Warn()`, `Logger.Error()` methods with variadic key-value attributes
- [ ] 2.4 Implement severity filtering: skip log entries below configured minimum level
- [ ] 2.5 Implement OTEL bridge: each logged entry creates a span event on a "logs" tracer
- [ ] 2.6 Expose `SetMinLevel(severity)` method that updates the minimum severity at runtime
- [ ] 2.7 Wire the logger into `main.go` — initialize it alongside the store and telemetry, pass to handlers

## 3. Admin Panel HTTP Handlers

- [ ] 3.1 Create `GET /api/admin/logs/html` handler that renders the log viewer HTML fragment with current filters (min_level, source)
- [ ] 3.2 Implement log query with filtering by severity (min_level query param, default WARN) and source text search
- [ ] 3.3 Return paginated log entries rendered as an HTML table with Timestamp, Severity, Source, Message columns
- [ ] 3.4 Include log count summary in the response ("Showing N entries — X DEBUG, Y INFO, Z WARN, W ERROR")
- [ ] 3.5 Register admin routes in `main.go` under `/api/admin/` prefix

## 4. App Settings & Verbosity Control

- [ ] 4.1 Implement `GET /api/admin/settings/log_level` handler that returns current log level
- [ ] 4.2 Implement `POST /api/admin/settings/log_level` handler that updates the log level in the database and calls `logger.SetMinLevel()`
- [ ] 4.3 Add verbosity dropdown component (DEBUG, INFO, WARN, ERROR) in the admin panel log viewer fragment
- [ ] 4.4 Wire verbosity control to POST to `/api/admin/settings/log_level` via HTMX on change

## 5. Admin Panel Frontend

- [ ] 5.1 Add "Admin" navigation link to `static/index.html` header that shows the admin panel
- [ ] 5.2 Create admin panel layout with tab navigation (Logs tab active by default, Settings tab placeholder)
- [ ] 5.3 Implement Log Viewer UI: HTML table with log entries, severity and source filter controls
- [ ] 5.4 Wire HTMX polling on the log viewer (hx-trigger="every 5s") for auto-refresh
- [ ] 5.5 Add admin panel and log viewer styles to `static/style.css`
- [ ] 5.6 Implement admin panel tab switching via HTMX

## 6. Settings Endpoints Onboarding

- [ ] 6.1 Create `GET/POST /api/admin/settings/email` handler stub that stores/retrieves email config JSON in app_settings
- [ ] 6.2 Create `GET/POST /api/admin/settings/ai` handler stub that stores/retrieves AI config JSON in app_settings
- [ ] 6.3 Wire both settings endpoints to emit structured logs (INFO on GET, WARN on successful POST, ERROR on bad POST)
- [ ] 6.4 Register settings-email and settings-ai routes in `main.go` under `/api/admin/settings/`

## 7. Migrate Existing Log Calls

- [ ] 7.1 Replace `log.Printf` calls in `main.go` handlers with structured logger calls (quota, playlists, jobs, etc.)
- [ ] 7.2 Replace `log.Printf` calls in `internal/telemetry/` with structured logger calls where appropriate
- [ ] 7.3 Ensure job lifecycle events (created, paused, resumed, completed, failed) are logged via the central logger

## 8. Testing & Verification

- [ ] 8.1 Write unit tests for `internal/logging/` package (severity filtering, log capture, OTEL export)
- [ ] 8.2 Write unit tests for store methods (InsertLog, GetLogs, GetLogCounts, GetSetting, SetSetting)
- [ ] 8.3 Write unit tests for admin panel HTTP handlers
- [ ] 8.4 Manually verify: admin panel loads, logs appear, verbosity control works, settings endpoints respond
- [ ] 8.5 Verify OTEL export: run with `OTEL_EXPORTER_OTLP_ENDPOINT` set, confirm log span events appear
