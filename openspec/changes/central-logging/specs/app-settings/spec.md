## ADDED Requirements

### Requirement: Persistent settings store

The system SHALL provide a key-value settings store backed by an `app_settings` table in SQLite. The settings store SHALL support Get and Set operations for string values.

#### Scenario: Save a setting
- **WHEN** `settings.Set("log_level", "DEBUG")` is called
- **THEN** the value `DEBUG` SHALL be stored in the `app_settings` table with key `log_level`
- **AND** any previous value for that key SHALL be overwritten

#### Scenario: Retrieve a setting
- **WHEN** `settings.Get("log_level")` is called
- **THEN** it SHALL return the stored value `"DEBUG"`
- **AND** if the key does not exist, it SHALL return an empty string with no error

### Requirement: Log verbosity setting

The system SHALL allow the operator to change the minimum log level via the admin panel. The setting SHALL be persisted in `app_settings` and take effect immediately without restart.

#### Scenario: Change log verbosity
- **WHEN** user selects a new minimum level (e.g., "DEBUG") in the admin panel verbosity control
- **THEN** a POST request SHALL be sent to `/api/admin/settings/log_level` with the new value
- **AND** the setting SHALL be persisted in `app_settings`
- **AND** the logging package SHALL immediately use the new minimum level

#### Scenario: Default verbosity
- **WHEN** no `log_level` setting exists
- **THEN** the system SHALL default to minimum level WARN
