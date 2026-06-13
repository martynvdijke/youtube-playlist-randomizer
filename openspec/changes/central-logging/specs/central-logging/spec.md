## ADDED Requirements

### Requirement: Structured log capture

The system SHALL capture application events as structured log entries. Each log entry SHALL contain: timestamp, severity level (DEBUG, INFO, WARN, ERROR), source identifier, message, and optional key-value attributes. The logging SHALL be performed via a new `internal/logging/` Go package.

#### Scenario: Log an INFO event
- **WHEN** any component calls `logger.Info("message", "key", "value")`
- **THEN** a log entry SHALL be written to the `logs` table with severity=INFO, the given message, and the given attributes

#### Scenario: Log a WARN event
- **WHEN** any component calls `logger.Warn("message")`
- **THEN** a log entry SHALL be written to the `logs` table with severity=WARN and the given message

#### Scenario: Log an ERROR event
- **WHEN** any component calls `logger.Error("message", err)`
- **THEN** a log entry SHALL be written to the `logs` table with severity=ERROR, the message, and the error string included as an attribute

#### Scenario: Log a DEBUG event
- **WHEN** any component calls `logger.Debug("message")`
- **THEN** a log entry SHALL be written to the `logs` table with severity=DEBUG and the given message

### Requirement: Log filtering by minimum severity

The logging package SHALL respect a configurable minimum severity level. Log entries below the minimum SHALL NOT be stored or exported.

#### Scenario: Minimum level set to WARN
- **WHEN** the minimum log level is set to WARN
- **AND** an INFO-level log call is made
- **THEN** the log entry SHALL NOT be written to the database
- **AND** it SHALL NOT be exported to OTEL

#### Scenario: Minimum level set to DEBUG
- **WHEN** the minimum log level is set to DEBUG
- **AND** any log call is made
- **THEN** the log entry SHALL be written regardless of its severity

### Requirement: Database schema for logs

The system SHALL have a `logs` table with columns: `id` (AUTOINCREMENT), `timestamp` (TEXT, RFC3339), `severity` (TEXT), `source` (TEXT), `message` (TEXT), `attributes` (TEXT, JSON), `created_at` (TEXT).

#### Scenario: Log table creation
- **WHEN** the application starts
- **THEN** the `logs` table SHALL be created by the migration in `store.migrate()` if it does not already exist

### Requirement: OTEL log export

Each log entry at or above the minimum severity SHALL be recorded as a span event on a dedicated "logs" tracer, using the existing `go.opentelemetry.io/otel` SDK. The span event SHALL include the severity, message, source, and attributes.

#### Scenario: Log appears as OTEL span event
- **WHEN** a WARN log entry is captured
- **THEN** a span event SHALL be created on the logs tracer
- **AND** the event SHALL contain the severity, message, source, and attributes
