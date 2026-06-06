## Context

The application is a Go-based YouTube playlist randomizer with a static HTML/HTMX frontend served from the same binary. It has:

- An existing admin panel with "Logs" and "Settings" tabs (`internal/admin/handlers.go`)
- A key-value settings store using SQLite (`app_settings` table) via `store.GetSetting`/`store.SetSetting`
- OpenTelemetry already wired for observability (traces/metrics), separate from user-facing analytics
- The main HTML template is served from `main.go` line 267-280 with string replacement for dynamic content

Umami is a privacy-first, self-hosted analytics platform. It works by embedding a small JavaScript snippet (`<script defer src="<umami-url>/script.js" data-website-id="<id>">`) that tracks page views and events. No cookies, no GDPR friction.

## Goals / Non-Goals

**Goals:**
- Provide an admin UI to configure Umama self-hosted analytics (server URL + website ID)
- Store configuration in the existing `app_settings` table
- Inject the Umami tracking script into every HTML page when analytics are enabled
- Reuse the existing Settings tab in the admin panel (currently a placeholder)

**Non-Goals:**
- Event tracking beyond default page views (future enhancement)
- Umami dashboard embedding (link out to Umami server)
- Google Analytics or any other analytics provider
- New database migrations or schema changes

## Decisions

1. **Settings keys** — Use three `app_settings` keys matching the proposal: `umami_url`, `umami_website_id`, `umami_script_url`. When `umami_url` and `umami_website_id` are both non-empty, analytics is considered enabled. The `umami_script_url` is optional and defaults to `<umami_url>/script.js`.

2. **Admin handler pattern** — Follow the existing `HandleSettingsEmail`/`HandleSettingsAI` pattern in `internal/admin/handlers.go`. GET returns current settings as JSON (or defaults), POST accepts JSON and saves. This keeps the backend consistent with existing conventions.

3. **HTMX-driven settings form** — The settings form is served by the admin handler as an HTML fragment, replacing the current JavaScript `alert()` placeholder in `index.html`. The form uses `hx-get`/`hx-post` for async save without a page reload. Save button posts to `/api/admin/settings/umami`.

4. **Script injection in Go** — Extend the `/` route handler in `main.go` to conditionally inject the Umami `<script>` tag into the HTML `<head>` when config exists. This is the same string-replacement approach already used for the version badge. No template engine needed.

5. **No new dependencies** — Umami tracking is loaded from the user's own server at runtime by the browser. No Go module is needed.

6. **Validation** — Server-side validation: `umami_url` must be a valid URL with http/https scheme. `umami_website_id` must be non-empty. The frontend validates required fields before submit.

## Risks / Trade-offs

- **Analytics config stored in plaintext** in `app_settings` — Acceptable since this is a self-hosted app with no multi-tenant or user accounts. The SQLite database is only accessible to the server process.
- **Script injected via string replacement** — Simple but fragile if HTML structure changes. Mitigation: use a clear injection marker comment in the HTML (`<!-- umami -->`) so replacements are explicit and grep-able.
- **No validation of Umami URL** at save time — A misconfigured URL means analytics silently don't load. Mitigation: Show a success indicator after save but note that connectivity depends on the browser being able to reach the Umami server.
- **Single analytics provider** — If users want a different provider later, the settings storage pattern (key-value) is generic enough to extend.
