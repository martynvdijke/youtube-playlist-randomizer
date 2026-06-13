## ADDED Requirements

### Requirement: AI settings endpoint

The system SHALL provide an HTTP endpoint under `/api/admin/settings/ai` for configuring AI settings. The endpoint SHALL accept configuration values via POST and return current settings via GET. Every operation on this endpoint SHALL emit a structured log entry via the central logging system.

#### Scenario: GET AI settings
- **WHEN** a GET request is made to `/api/admin/settings/ai`
- **THEN** the system SHALL return the current AI settings as JSON
- **AND** SHALL log an INFO event with source "settings-ai" and message "AI settings retrieved"

#### Scenario: POST AI settings
- **WHEN** a POST request is made to `/api/admin/settings/ai` with valid JSON body containing AI configuration
- **THEN** the system SHALL store the provided settings
- **AND** SHALL log a WARN event with source "settings-ai" and message "AI settings updated"
- **AND** return HTTP 200

#### Scenario: POST with invalid JSON
- **WHEN** a POST request is made to `/api/admin/settings/ai` with malformed JSON body
- **THEN** the system SHALL log an ERROR event with source "settings-ai" and message "Failed to update AI settings: invalid JSON"
- **AND** return HTTP 400
