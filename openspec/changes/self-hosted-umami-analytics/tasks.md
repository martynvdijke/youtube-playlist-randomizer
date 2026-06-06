## 1. Backend — Admin Handler for Umami Settings

- [ ] 1.1 Add `HandleSettingsUmami` method to `admin.Handlers` in `internal/admin/handlers.go` with GET (returns JSON) and POST (accepts JSON, validates, saves via store)
- [ ] 1.2 Register `/api/admin/settings/umami` route in `main.go` pointing to `adminHandlers.HandleSettingsUmami`

## 2. Backend — Umami Script Injection

- [ ] 2.1 Add injection marker comment `<!-- umami -->` to `static/index.html` in the `<head>` section
- [ ] 2.2 In `main.go` root handler (line 267), after version badge replacement, read `umami_url`, `umami_website_id`, `umami_script_url` from store and conditionally inject the Umami `<script>` tag if both URL and Website ID are non-empty

## 3. Frontend — Admin Settings Tab

- [ ] 3.1 Replace the Settings tab `onclick` alert in `static/index.html` with an HTMX trigger that loads `/api/admin/settings/umami/html` (new endpoint)
- [ ] 3.2 Add HTML fragment rendering to `HandleSettingsUmami` for the settings form (server URL, website ID, optional script URL fields + Save button with hx-post)
- [ ] 3.3 Add CSS styles in `static/style.css` for the analytics settings form

## 4. Verification

- [ ] 4.1 Run `go build ./...` to verify backend compiles without errors
- [ ] 4.2 Start the server in mock mode and verify the Settings tab renders the Umami form, saving works, and script is injected into HTML
