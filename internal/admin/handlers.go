package admin

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

type Handlers struct {
	store  *store.Store
	logger *logging.Logger
}

func New(s *store.Store, l *logging.Logger) *Handlers {
	return &Handlers{store: s, logger: l}
}

// HandleLogsHTML renders the log viewer HTML fragment.
// Query params: min_level (default WARN), source (optional), offset (default 0)
func (h *Handlers) HandleLogsHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	minLevel := r.URL.Query().Get("min_level")
	if minLevel == "" {
		minLevel = "WARN"
	}
	source := r.URL.Query().Get("source")
	offsetStr := r.URL.Query().Get("offset")
	offset, _ := strconv.Atoi(offsetStr)
	if offset < 0 {
		offset = 0
	}

	entries, err := h.store.GetLogs(minLevel, source, 200, offset)
	if err != nil {
		h.logger.Errorc(r.Context(), "Failed to query logs", "error", err.Error())
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	counts, err := h.store.GetLogCounts()
	if err != nil {
		counts = nil
	}

	w.Header().Set("Content-Type", "text/html")
	h.writeLogViewer(w, entries, counts, minLevel, source)
}

func selected(current, value string) string {
	if current == value {
		return ` selected`
	}
	return ""
}

func shortTimestamp(ts string) string {
	// RFC3339 -> short: "15:04:05"
	if len(ts) >= 19 {
		return ts[11:19]
	}
	return ts
}

func truncateAttrs(attrs string, maxLen int) string {
	if len(attrs) > maxLen {
		return attrs[:maxLen] + "..."
	}
	return attrs
}

func (h *Handlers) writeLogViewer(w io.Writer, entries []store.LogEntry, counts []store.LogCount, minLevel, source string) {
	total := len(entries)
	countMap := map[string]int{"DEBUG": 0, "INFO": 0, "WARN": 0, "ERROR": 0}
	for _, c := range counts {
		countMap[c.Severity] = c.Count
	}
	countStr := fmt.Sprintf("Showing %d entries (%d DEBUG, %d INFO, %d WARN, %d ERROR)",
		total, countMap["DEBUG"], countMap["INFO"], countMap["WARN"], countMap["ERROR"])

	// Severity filter dropdown
	fmt.Fprintf(w, `<div class="admin-log-controls">
  <div class="log-count">%s</div>
  <div class="log-filters">
    <label>Severity: </label>
    <select name="min_level" hx-get="/api/admin/logs/html" hx-trigger="change" hx-target="#admin-content" hx-swap="innerHTML" hx-include="this">
      <option value="DEBUG"%s>DEBUG</option>
      <option value="INFO"%s>INFO</option>
      <option value="WARN"%s>WARN</option>
      <option value="ERROR"%s>ERROR</option>
    </select>
    <label>Source: </label>
    <input type="text" name="source" placeholder="Filter by source..." value="%s" hx-get="/api/admin/logs/html" hx-trigger="keyup changed delay:300ms" hx-target="#admin-content" hx-swap="innerHTML" hx-include="this">
  </div>
</div>`,
		countStr,
		selected(minLevel, "DEBUG"), selected(minLevel, "INFO"),
		selected(minLevel, "WARN"), selected(minLevel, "ERROR"),
		html.EscapeString(source))

	// Verbosity control
	fmt.Fprintf(w, `<div class="admin-verbosity">
  <label>Minimum log level: </label>
  <select name="log_level" hx-post="/api/admin/settings/log_level" hx-trigger="change" hx-target="#verbosity-status" hx-swap="innerHTML">
    <option value="DEBUG"%s>DEBUG</option>
    <option value="INFO"%s>INFO</option>
    <option value="WARN"%s>WARN</option>
    <option value="ERROR"%s>ERROR</option>
  </select>
  <span id="verbosity-status"></span>
</div>`,
		selected(minLevel, "DEBUG"), selected(minLevel, "INFO"),
		selected(minLevel, "WARN"), selected(minLevel, "ERROR"))

	// Log table
	fmt.Fprint(w, `<table class="log-table">
  <thead>
    <tr>
      <th>Timestamp</th>
      <th>Severity</th>
      <th>Source</th>
      <th>Message</th>
      <th>Attributes</th>
    </tr>
  </thead>
  <tbody>`)

	if len(entries) == 0 {
		fmt.Fprint(w, `<tr><td colspan="5" class="log-empty">No log entries found.</td></tr>`)
	} else {
		for _, e := range entries {
			sevClass := "log-sev-" + strings.ToLower(e.Severity)
			fmt.Fprintf(w, `<tr class="%s">
  <td class="log-ts">%s</td>
  <td class="log-sev">%s</td>
  <td class="log-src">%s</td>
  <td class="log-msg">%s</td>
  <td class="log-attrs">%s</td>
</tr>`,
				sevClass,
				html.EscapeString(shortTimestamp(e.Timestamp)),
				html.EscapeString(e.Severity),
				html.EscapeString(e.Source),
				html.EscapeString(e.Message),
				html.EscapeString(truncateAttrs(e.Attributes, 60)))
		}
	}

	fmt.Fprint(w, `</tbody></table>`)
}

// HandleLogLevelGet returns the current log level setting as plain text.
func (h *Handlers) HandleLogLevelGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	level, err := h.store.GetSetting("log_level")
	if err != nil || level == "" {
		level = "WARN"
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, level)
}

// HandleLogLevelSet updates the log level and applies it to the logger.
// otelAdminSettings holds the OpenTelemetry configuration as stored in DB.
type otelAdminSettings struct {
	Endpoint        string `json:"endpoint"`
	TracesEnabled   string `json:"tracesEnabled"`
	MetricsEnabled  string `json:"metricsEnabled"`
	TraceSampleRate string `json:"traceSampleRate"`
	Headers         string `json:"headers"`
}

func (h *Handlers) loadOTelSettings() otelAdminSettings {
	get := func(key, def string) string {
		v, _ := h.store.GetSetting(key)
		if v == "" {
			return def
		}
		return v
	}
	return otelAdminSettings{
		Endpoint:        get("otel_endpoint", ""),
		TracesEnabled:   get("otel_traces_enabled", "true"),
		MetricsEnabled:  get("otel_metrics_enabled", "true"),
		TraceSampleRate: get("otel_trace_sample_rate", "1.0"),
		Headers:         get("otel_headers", ""),
	}
}

func checked(val, expected string) string {
	if val == expected {
		return ` checked`
	}
	return ""
}

// HandleSettingsOTel handles GET/POST for OpenTelemetry settings.
func (h *Handlers) HandleSettingsOTel(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := h.loadOTelSettings()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)

	case http.MethodPost:
		endpoint := r.FormValue("endpoint")
		tracesEnabled := r.FormValue("tracesEnabled")
		metricsEnabled := r.FormValue("metricsEnabled")
		traceSampleRate := r.FormValue("traceSampleRate")
		headers := r.FormValue("headers")

		if tracesEnabled == "" {
			tracesEnabled = "false"
		}
		if metricsEnabled == "" {
			metricsEnabled = "false"
		}
		if traceSampleRate == "" {
			traceSampleRate = "1.0"
		}

		if endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			h.logger.Errorc(r.Context(), "Invalid OTel endpoint URL scheme", "url", endpoint)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `<span class="error">URL must start with http:// or https://</span>`)
			return
		}

		// Validate sample rate
		if rate, err := strconv.ParseFloat(traceSampleRate, 64); err != nil || rate < 0 || rate > 1 {
			h.logger.Errorc(r.Context(), "Invalid OTel trace sample rate", "rate", traceSampleRate)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `<span class="error">Trace sample rate must be a number between 0 and 1</span>`)
			return
		}

		// Validate headers is valid JSON object if not empty
		if headers != "" {
			var dummy map[string]string
			if err := json.Unmarshal([]byte(headers), &dummy); err != nil {
				h.logger.Errorc(r.Context(), "Invalid OTel headers JSON", "error", err.Error())
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `<span class="error">Headers must be a valid JSON object</span>`)
				return
			}
		}

		saves := map[string]string{
			"otel_endpoint":          endpoint,
			"otel_traces_enabled":    tracesEnabled,
			"otel_metrics_enabled":   metricsEnabled,
			"otel_trace_sample_rate": traceSampleRate,
			"otel_headers":           headers,
		}
		for key, val := range saves {
			if err := h.store.SetSetting(key, val); err != nil {
				h.logger.Errorc(r.Context(), "Failed to save OTel setting", "key", key, "error", err.Error())
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `<span class="error">Failed to save setting</span>`)
				return
			}
		}

		h.logger.Infoc(r.Context(), "OTel settings updated", "traces", tracesEnabled, "metrics", metricsEnabled, "sample_rate", traceSampleRate)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="restart-required">✓ Saved — restart required to apply</span>`)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSettingsOTelHTML renders the OTel settings form as an HTML fragment.
func (h *Handlers) HandleSettingsOTelHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := h.loadOTelSettings()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="admin-settings-otel">
  <h3>OpenTelemetry</h3>
  <p class="settings-desc">Configure OTLP exporting for traces and metrics. Changes require a server restart to take effect.</p>
  <form hx-post="/api/admin/settings/otel" hx-target="#otel-status" hx-swap="innerHTML">
    <div class="form-field">
      <label for="otel-endpoint">OTLP Endpoint URL</label>
      <input type="url" id="otel-endpoint" name="endpoint" placeholder="http://otel-collector:4318" value="%s">
      <p class="field-hint">Leave empty to disable OTLP exporting. Falls back to <code>OTEL_EXPORTER_OTLP_ENDPOINT</code> env var.</p>
    </div>
    <div class="form-field form-field-row">
      <label class="toggle-label">
        <input type="checkbox" name="tracesEnabled" value="true"%s>
        <span class="toggle-text">Enable Traces</span>
      </label>
      <label class="toggle-label">
        <input type="checkbox" name="metricsEnabled" value="true"%s>
        <span class="toggle-text">Enable Metrics</span>
      </label>
    </div>
    <div class="form-field">
      <label for="otel-sample-rate">Trace Sample Rate</label>
      <input type="text" id="otel-sample-rate" name="traceSampleRate" placeholder="1.0" value="%s">
      <p class="field-hint">Value between 0.0 (no traces) and 1.0 (all traces). Applied when traces are enabled.</p>
    </div>
    <div class="form-field">
      <label for="otel-headers">OTLP Headers <span class="field-optional">(optional JSON)</span></label>
      <textarea id="otel-headers" name="headers" rows="3" placeholder='{"Authorization":"Bearer my-api-key"}'>%s</textarea>
      <p class="field-hint">JSON object of headers sent with every OTLP request. Supports env var expansion (e.g. <code>${OTEL_AUTH_TOKEN}</code>).</p>
    </div>
    <div class="form-actions">
      <button type="submit" class="btn btn-primary">Save</button>
      <span id="otel-status"></span>
    </div>
  </form>
  <div class="settings-notice settings-notice-restart">
    <p>⚠️ Saving settings stores them in the database. You must restart the server for new OTel settings to take effect.</p>
  </div>
</div>`,
		html.EscapeString(settings.Endpoint),
		checked(settings.TracesEnabled, "true"),
		checked(settings.MetricsEnabled, "true"),
		html.EscapeString(settings.TraceSampleRate),
		html.EscapeString(settings.Headers))
}

func (h *Handlers) HandleLogLevelSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Errorc(r.Context(), "Failed to read request body", "error", err.Error())
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	level := strings.TrimSpace(string(body))
	if level == "" {
		level = r.FormValue("log_level")
	}
	if level == "" {
		h.logger.Warnc(r.Context(), "Empty log level in request")
		http.Error(w, "Missing log_level", http.StatusBadRequest)
		return
	}

	sev := logging.ParseSeverity(level)
	if err := h.store.SetSetting("log_level", sev.String()); err != nil {
		h.logger.Errorc(r.Context(), "Failed to save log level", "error", err.Error())
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	h.logger.SetMinLevel(sev)
	h.logger.Infoc(r.Context(), "Log level changed", "level", sev.String())

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<span class="verbosity-ok">Set to %s</span>`, sev.String())
}

// HandleSettingsEmail handles GET/POST for email settings.
func (h *Handlers) HandleSettingsEmail(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		val, _ := h.store.GetSetting("settings_email")
		if val == "" {
			val = "{}"
		}
		h.logger.Infoc(r.Context(), "Email settings retrieved")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, val)

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.logger.Errorc(r.Context(), "Failed to update email settings", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"bad request"}`)
			return
		}
		if len(body) == 0 {
			h.logger.Errorc(r.Context(), "Failed to update email settings", "reason", "empty body")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"empty body"}`)
			return
		}
		var dummy interface{}
		if err := json.Unmarshal(body, &dummy); err != nil {
			h.logger.Errorc(r.Context(), "Failed to update email settings", "reason", "invalid JSON")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid JSON"}`)
			return
		}
		if err := h.store.SetSetting("settings_email", string(body)); err != nil {
			h.logger.Errorc(r.Context(), "Failed to update email settings", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		h.logger.Warnc(r.Context(), "Email settings updated")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSettingsAI handles GET/POST for AI settings.
func (h *Handlers) HandleSettingsAI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		val, _ := h.store.GetSetting("settings_ai")
		if val == "" {
			val = "{}"
		}
		h.logger.Infoc(r.Context(), "AI settings retrieved")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, val)

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.logger.Errorc(r.Context(), "Failed to update AI settings", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"bad request"}`)
			return
		}
		if len(body) == 0 {
			h.logger.Errorc(r.Context(), "Failed to update AI settings", "reason", "empty body")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"empty body"}`)
			return
		}
		var dummy interface{}
		if err := json.Unmarshal(body, &dummy); err != nil {
			h.logger.Errorc(r.Context(), "Failed to update AI settings", "reason", "invalid JSON")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid JSON"}`)
			return
		}
		if err := h.store.SetSetting("settings_ai", string(body)); err != nil {
			h.logger.Errorc(r.Context(), "Failed to update AI settings", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		h.logger.Warnc(r.Context(), "AI settings updated")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

type umamiSettings struct {
	URL       string `json:"url"`
	WebsiteID string `json:"websiteId"`
	ScriptURL string `json:"scriptUrl,omitempty"`
}

func (h *Handlers) loadUmamiSettings() umamiSettings {
	url, _ := h.store.GetSetting("umami_url")
	websiteID, _ := h.store.GetSetting("umami_website_id")
	scriptURL, _ := h.store.GetSetting("umami_script_url")
	return umamiSettings{URL: url, WebsiteID: websiteID, ScriptURL: scriptURL}
}

// HandleSettingsUmami handles GET/POST for Umami analytics JSON settings.
func (h *Handlers) HandleSettingsUmami(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := h.loadUmamiSettings()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)

	case http.MethodPost:
		url := r.FormValue("url")
		websiteID := r.FormValue("websiteId")
		scriptURL := r.FormValue("scriptUrl")

		if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			h.logger.Errorc(r.Context(), "Invalid umami URL scheme", "url", url)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"url must start with http:// or https://"}`)
			return
		}

		if err := h.store.SetSetting("umami_url", url); err != nil {
			h.logger.Errorc(r.Context(), "Failed to save umami URL", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		if err := h.store.SetSetting("umami_website_id", websiteID); err != nil {
			h.logger.Errorc(r.Context(), "Failed to save umami website ID", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		if err := h.store.SetSetting("umami_script_url", scriptURL); err != nil {
			h.logger.Errorc(r.Context(), "Failed to save umami script URL", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		h.logger.Infoc(r.Context(), "Umami analytics settings updated")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSettingsUmamiHTML renders the Umami analytics settings form as an HTML fragment.
func (h *Handlers) HandleSettingsUmamiHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := h.loadUmamiSettings()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="admin-settings-umami">
  <h3>Umami Analytics</h3>
  <p class="settings-desc">Configure self-hosted <a href="https://umami.is" target="_blank" rel="noopener">Umami</a> analytics tracking.</p>
  <form hx-post="/api/admin/settings/umami" hx-target="#umami-status" hx-swap="innerHTML">
    <div class="form-field">
      <label for="umami-url">Server URL</label>
      <input type="url" id="umami-url" name="url" placeholder="https://analytics.example.com" value="%s">
    </div>
    <div class="form-field">
      <label for="umami-website-id">Website ID</label>
      <input type="text" id="umami-website-id" name="websiteId" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" value="%s">
    </div>
    <div class="form-field">
      <label for="umami-script-url">Script URL <span class="field-optional">(optional)</span></label>
      <input type="url" id="umami-script-url" name="scriptUrl" placeholder="https://analytics.example.com/script.js" value="%s">
      <p class="field-hint">Defaults to &lt;Server URL&gt;/script.js if not set.</p>
    </div>
    <div class="form-actions">
      <button type="submit" class="btn btn-primary">Save</button>
      <span id="umami-status"></span>
    </div>
  </form>
  <div class="settings-notice">
    <p>After saving, the Umami tracking script will be injected into every page when both Server URL and Website ID are set.</p>
  </div>
</div>`,
		html.EscapeString(settings.URL),
		html.EscapeString(settings.WebsiteID),
		html.EscapeString(settings.ScriptURL))
}

// gotifySettings holds the Gotify notification configuration.
type gotifySettings struct {
	URL     string `json:"url"`
	Token   string `json:"token"`
	Enabled string `json:"enabled"`
}

func (h *Handlers) loadGotifySettings() gotifySettings {
	url, _ := h.store.GetSetting("gotify_url")
	token, _ := h.store.GetSetting("gotify_token")
	enabled, _ := h.store.GetSetting("gotify_enabled")
	return gotifySettings{URL: url, Token: token, Enabled: enabled}
}

// HandleSettingsGotify handles GET/POST for Gotify notification settings.
func (h *Handlers) HandleSettingsGotify(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := h.loadGotifySettings()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)

	case http.MethodPost:
		url := r.FormValue("url")
		token := r.FormValue("token")
		enabled := r.FormValue("enabled")
		if enabled == "" {
			enabled = "false"
		}

		if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			h.logger.Errorc(r.Context(), "Invalid Gotify URL scheme", "url", url)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"url must start with http:// or https://"}`)
			return
		}

		if err := h.store.SetSetting("gotify_url", url); err != nil {
			h.logger.Errorc(r.Context(), "Failed to save Gotify URL", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		if err := h.store.SetSetting("gotify_token", token); err != nil {
			h.logger.Errorc(r.Context(), "Failed to save Gotify token", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		if err := h.store.SetSetting("gotify_enabled", enabled); err != nil {
			h.logger.Errorc(r.Context(), "Failed to save Gotify enabled", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}

		h.logger.Infoc(r.Context(), "Gotify notification settings updated", "enabled", enabled)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSettingsGotifyHTML renders the Gotify notification settings form as an HTML fragment.
func (h *Handlers) HandleSettingsGotifyHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := h.loadGotifySettings()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="admin-settings-gotify">
  <h3>Gotify Notifications</h3>
  <p class="settings-desc">Configure push notifications via a self-hosted <a href="https://gotify.net" target="_blank" rel="noopener">Gotify</a> server. Notifications are sent when a playlist shuffle completes, fails, or pauses.</p>
  <form hx-post="/api/admin/settings/gotify" hx-target="#gotify-status" hx-swap="innerHTML">
    <div class="form-field">
      <label for="gotify-url">Server URL</label>
      <input type="url" id="gotify-url" name="url" placeholder="http://gotify:8080" value="%s">
    </div>
    <div class="form-field">
      <label for="gotify-token">App Token</label>
      <input type="password" id="gotify-token" name="token" placeholder="Gotify application token" value="%s" autocomplete="off">
    </div>
    <div class="form-field form-field-row">
      <label class="toggle-label">
        <input type="checkbox" name="enabled" value="true"%s>
        <span class="toggle-text">Enable Notifications</span>
      </label>
    </div>
    <div class="form-actions">
      <button type="submit" class="btn btn-primary">Save</button>
      <span id="gotify-status"></span>
    </div>
  </form>
  <div class="settings-notice">
    <p>Notifications are sent immediately on job completion, failure, or pause — no restart required.</p>
  </div>
</div>`,
		html.EscapeString(settings.URL),
		html.EscapeString(settings.Token),
		checked(settings.Enabled, "true"))
}
