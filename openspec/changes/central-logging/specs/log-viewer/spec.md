## ADDED Requirements

### Requirement: Log viewer page

The system SHALL provide a log viewer page within the admin panel that displays recent log entries in reverse-chronological order (newest first). The viewer SHALL be loaded via HTMX from the `/api/admin/logs/html` endpoint.

#### Scenario: View logs default
- **WHEN** the admin Logs tab is selected
- **THEN** the system SHALL call `GET /api/admin/logs/html`
- **AND** display a table of log entries with columns: Timestamp, Severity, Source, Message
- **AND** only show entries with severity WARN and above by default

### Requirement: Filter by severity

The log viewer SHALL provide filter controls to narrow displayed logs by minimum severity level.

#### Scenario: Filter by severity
- **WHEN** user selects "ERROR" in the severity filter
- **THEN** the viewer SHALL display only ERROR entries
- **AND** the filter SHALL be applied via HTMX query parameter to `/api/admin/logs/html?min_level=ERROR`

### Requirement: Filter by source

The log viewer SHALL provide a filter to narrow logs by source identifier.

#### Scenario: Filter by source
- **WHEN** user types a source filter text
- **THEN** the viewer SHALL display only entries whose source contains the filter text
- **AND** the filter SHALL be applied via HTMX query parameter to `/api/admin/logs/html?source=jobs`

### Requirement: Auto-refresh

The log viewer SHALL automatically refresh its content every 5 seconds using HTMX polling.

#### Scenario: Auto-refresh logs
- **WHEN** the log viewer is displayed
- **THEN** it SHALL poll `GET /api/admin/logs/html?min_level=<current>&source=<current>` every 5 seconds
- **AND** update the log table without full page reload

### Requirement: Log count display

The log viewer SHALL show the total number of displayed entries and a count per severity level.

#### Scenario: Log counts shown
- **WHEN** logs are displayed
- **THEN** the viewer SHALL show "Showing N entries (X DEBUG, Y INFO, Z WARN, W ERROR)"
- **AND** counts SHALL update on each refresh
