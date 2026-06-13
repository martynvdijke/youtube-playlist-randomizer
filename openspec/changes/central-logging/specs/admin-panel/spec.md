## ADDED Requirements

### Requirement: Admin panel shell

The system SHALL provide an admin panel accessible from the main application header. The admin panel SHALL contain navigation tabs, including at minimum a "Logs" tab. Future tabs (e.g., "Settings") SHALL be accommodated by the navigation structure.

#### Scenario: Navigate to admin panel
- **WHEN** user clicks "Admin" link in the header
- **THEN** the main content area SHALL replace the playlist view with the admin panel layout
- **AND** the admin panel SHALL display a "Logs" tab as the default active tab

#### Scenario: Admin panel uses HTMX fragments
- **WHEN** user navigates to the admin panel
- **THEN** the admin panel shell SHALL load its content via HTMX from `/api/admin/logs/html`
- **AND** tab switching SHALL use HTMX to swap content without full page reload

### Requirement: Admin panel route isolation

The admin panel SHALL be served under an `/api/admin/*` route prefix on the Go HTTP mux.

#### Scenario: Admin routes are prefixed
- **WHEN** a request is made to `/api/admin/logs`
- **THEN** it SHALL be handled by the admin-specific handler
- **AND** it SHALL NOT conflict with existing `/api/playlists`, `/api/quota`, etc. routes
