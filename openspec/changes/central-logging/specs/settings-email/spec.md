## ADDED Requirements

### Requirement: Email settings endpoint

The system SHALL provide an HTTP endpoint under `/api/admin/settings/email` for configuring email settings. The endpoint SHALL accept configuration values via POST and return current settings via GET. Every operation on this endpoint SHALL emit a structured log entry via the central logging system.

#### Scenario: GET email settings
- **WHEN** a GET request is made to `/api/admin/settings/email`
- **THEN** the system SHALL return the current email settings as JSON
- **AND** SHALL log an INFO event with source "settings-email" and message "Email settings retrieved"

#### Scenario: POST email settings
- **WHEN** a POST request is made to `/api/admin/settings/email` with valid JSON body containing email configuration
- **THEN** the system SHALL store the provided settings
- **AND** SHALL log a WARN event with source "settings-email" and message "Email settings updated"
- **AND** return HTTP 200

#### Scenario: POST with empty body
- **WHEN** a POST request is made to `/api/admin/settings/email` with an empty body
- **THEN** the system SHALL log an ERROR event with source "settings-email" and message "Failed to update email settings: empty body"
- **AND** return HTTP 400
