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
// otelSettings holds the OpenTelemetry endpoint configuration.
type otelSettings struct {
	Endpoint string `json:"endpoint"`
}

func (h *Handlers) loadOTelSettings() otelSettings {
	endpoint, _ := h.store.GetSetting("otel_endpoint")
	return otelSettings{Endpoint: endpoint}
}

// HandleSettingsOTel handles GET/POST for OpenTelemetry endpoint settings.
func (h *Handlers) HandleSettingsOTel(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := h.loadOTelSettings()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)

	case http.MethodPost:
		endpoint := r.FormValue("endpoint")

		if endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			h.logger.Errorc(r.Context(), "Invalid OTel endpoint URL scheme", "url", endpoint)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"url must start with http:// or https://"}`)
			return
		}

		if err := h.store.SetSetting("otel_endpoint", endpoint); err != nil {
			h.logger.Errorc(r.Context(), "Failed to save OTel endpoint", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"internal error"}`)
			return
		}
		h.logger.Infoc(r.Context(), "OTel endpoint updated")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSettingsOTelHTML renders the OTel endpoint settings form as an HTML fragment.
func (h *Handlers) HandleSettingsOTelHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := h.loadOTelSettings()

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="admin-settings-otel">
  <h3>OpenTelemetry</h3>
  <p class="settings-desc">Configure an OTLP endpoint to send traces and metrics to a collector (e.g. Grafana Tempo, SigNoz, Grafana Cloud).</p>
  <form hx-post="/api/admin/settings/otel" hx-target="#otel-status" hx-swap="innerHTML">
    <div class="form-field">
      <label for="otel-endpoint">OTLP Endpoint URL</label>
      <input type="url" id="otel-endpoint" name="endpoint" placeholder="http://otel-collector:4318" value="%s">
      <p class="field-hint">Leave empty to disable OTLP exporting. Uses <code>OTEL_EXPORTER_OTLP_ENDPOINT</code> env var as fallback.</p>
    </div>
    <div class="form-actions">
      <button type="submit" class="btn btn-primary">Save</button>
      <span id="otel-status"></span>
    </div>
  </form>
</div>`, html.EscapeString(settings.Endpoint))
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
