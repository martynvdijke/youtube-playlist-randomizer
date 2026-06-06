## Why

The application currently has no user-facing analytics — there's no way to track page views, user engagement, or feature adoption. Adding support for Umami (a privacy-first, self-hosted analytics platform) gives operators visibility into usage without sending data to third parties like Google Analytics. The existing admin panel has a "Settings" tab that's currently a placeholder, and this fills it with a meaningful first setting.

## What Changes

- Add an "Analytics" settings section in the admin panel to configure Umami self-hosted analytics
- Store Umami configuration (server URL, website ID, optional custom script URL) via existing `app_settings` table
- Inject the Umami tracking script into the HTML response when analytics are configured
- Add a backend API endpoint to GET/POST Umami settings
- Update the Settings tab in the admin panel to render the analytics configuration form

## Capabilities

### New Capabilities
- `umami-analytics-admin`: Admin panel UI and backend API for configuring Umami self-hosted analytics (server URL, website ID, script URL). Supports enabling/disabling analytics and validation of required fields.

### Modified Capabilities
- (none — no existing specs to modify)

## Impact

- **Backend**: New route `/api/admin/settings/umami` in `internal/admin/handlers.go` (GET/POST). Minor change to `main.go` to register the route and conditionally inject the Umami script tag into the HTML response.
- **Frontend**: Updates to `static/index.html` (conditionally render Umami script) and `static/style.css` (form styling in admin settings). The Settings tab will render live content instead of showing a placeholder alert.
- **Data**: No new database tables — reuses existing `app_settings` store with keys `umami_url`, `umami_website_id`, `umami_script_url`.
- **Dependencies**: No new Go dependencies. The Umami tracking script is loaded from the user's self-hosted server at runtime.
