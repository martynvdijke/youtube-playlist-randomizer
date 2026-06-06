## ADDED Requirements

### Requirement: Admin can configure Umami analytics
The system SHALL provide a settings form in the admin panel for configuring Umami self-hosted analytics. The form SHALL include fields for Umami server URL and Website ID, with an optional custom script URL field.

#### Scenario: Settings tab shows analytics form
- **WHEN** admin clicks the Settings tab in the admin panel
- **THEN** the system loads and displays the analytics configuration form via HTMX

#### Scenario: Admin saves valid Umami configuration
- **WHEN** admin enters a valid Umami server URL (https://) and Website ID and clicks Save
- **THEN** the system stores the configuration and shows a success message

#### Scenario: Admin submits empty URL
- **WHEN** admin submits the form with an empty Umami server URL
- **THEN** the system SHALL reject the submission and show a validation error

#### Scenario: Admin submits empty Website ID
- **WHEN** admin submits the form with an empty Website ID
- **THEN** the system SHALL reject the submission and show a validation error

#### Scenario: Admin saves optional custom script URL
- **WHEN** admin enters a custom script URL in addition to server URL and Website ID
- **THEN** the system stores all three values and uses the custom script URL for tracking

### Requirement: System injects Umami tracking script
The system SHALL inject the Umama tracking script tag into the HTML `<head>` for every page response when analytics are configured.

#### Scenario: Analytics configured — script injected
- **WHEN** a page is served and both `umami_url` and `umami_website_id` are set
- **THEN** the HTML includes `<script defer src="<umami_script_url>" data-website-id="<umami_website_id>"></script>`

#### Scenario: Analytics not configured — no script
- **WHEN** a page is served and either `umami_url` or `umami_website_id` is empty
- **THEN** the HTML does NOT include any Umami script tag

#### Scenario: Custom script URL is used when set
- **WHEN** a page is served and `umami_script_url` is set and non-empty
- **THEN** the script tag's `src` attribute uses `umami_script_url` instead of the default `<umami_url>/script.js`

### Requirement: Admin can view current Umami configuration
The system SHALL provide a GET endpoint that returns the current Umami configuration as JSON.

#### Scenario: Admin opens settings — current values shown
- **WHEN** admin loads the Settings tab after previously configuring analytics
- **THEN** the form pre-fills with the saved Umami URL, Website ID, and custom script URL

#### Scenario: No configuration saved — empty form shown
- **WHEN** admin loads the Settings tab and no Umami configuration exists
- **THEN** the form fields are empty

### Requirement: Admin can clear Umami configuration
The system SHALL allow saving all empty fields to clear the analytics configuration.

#### Scenario: Admin clears all fields and saves
- **WHEN** admin clears all fields in the analytics form and submits
- **THEN** the system clears stored values and analytics tracking stops
