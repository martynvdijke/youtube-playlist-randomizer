package admin

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

//go:embed templates/*.gohtml
var templateFS embed.FS

// adminTmpl holds all parsed admin HTML templates.
var adminTmpl = template.Must(template.New("").ParseFS(templateFS, "templates/*.gohtml"))

// Template data types.
type (
	logViewerData struct {
		Total      int
		CountDebug int
		CountInfo  int
		CountWarn  int
		CountError int
		MinLevel   string
		Source     string
		Entries    []logEntryData
	}

	logEntryData struct {
		Timestamp     string
		Severity      string
		SeverityLower string
		Source        string
		Message       string
		Attributes    string
	}

	otelSettingsData struct {
		Endpoint        string
		TracesEnabled   bool
		MetricsEnabled  bool
		TraceSampleRate string
		Headers         string
	}

	umamiSettingsData struct {
		URL       string
		WebsiteID string
		ScriptURL string
	}

	gotifySettingsData struct {
		URL     string
		Token   string
		Enabled bool
	}

	verbosityData struct {
		Level string
	}

	errorData struct {
		Error string
	}
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

func shortTimestamp(ts string) string {
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

	entryData := make([]logEntryData, 0, len(entries))
	for _, e := range entries {
		entryData = append(entryData, logEntryData{
			Timestamp:     shortTimestamp(e.Timestamp),
			Severity:      e.Severity,
			SeverityLower: strings.ToLower(e.Severity),
			Source:        e.Source,
			Message:       e.Message,
			Attributes:    truncateAttrs(e.Attributes, 60),
		})
	}

	adminTmpl.ExecuteTemplate(w, "logViewer", logViewerData{
		Total:      total,
		CountDebug: countMap["DEBUG"],
		CountInfo:  countMap["INFO"],
		CountWarn:  countMap["WARN"],
		CountError: countMap["ERROR"],
		MinLevel:   minLevel,
		Source:     source,
		Entries:    entryData,
	})
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
			adminTmpl.ExecuteTemplate(w, "errorSpan", errorData{Error: "URL must start with http:// or https://"})
			return
		}

		// Validate sample rate
		if rate, err := strconv.ParseFloat(traceSampleRate, 64); err != nil || rate < 0 || rate > 1 {
			h.logger.Errorc(r.Context(), "Invalid OTel trace sample rate", "rate", traceSampleRate)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadRequest)
			adminTmpl.ExecuteTemplate(w, "errorSpan", errorData{Error: "Trace sample rate must be a number between 0 and 1"})
			return
		}

		// Validate headers is valid JSON object if not empty
		if headers != "" {
			var dummy map[string]string
			if err := json.Unmarshal([]byte(headers), &dummy); err != nil {
				h.logger.Errorc(r.Context(), "Invalid OTel headers JSON", "error", err.Error())
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusBadRequest)
				adminTmpl.ExecuteTemplate(w, "errorSpan", errorData{Error: "Headers must be a valid JSON object"})
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
				adminTmpl.ExecuteTemplate(w, "errorSpan", errorData{Error: "Failed to save setting"})
				return
			}
		}

		h.logger.Infoc(r.Context(), "OTel settings updated", "traces", tracesEnabled, "metrics", metricsEnabled, "sample_rate", traceSampleRate)
		w.Header().Set("Content-Type", "text/html")
		adminTmpl.ExecuteTemplate(w, "savedOk", nil)

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
	adminTmpl.ExecuteTemplate(w, "otelSettings", otelSettingsData{
		Endpoint:        settings.Endpoint,
		TracesEnabled:   settings.TracesEnabled == "true",
		MetricsEnabled:  settings.MetricsEnabled == "true",
		TraceSampleRate: settings.TraceSampleRate,
		Headers:         settings.Headers,
	})
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
		var dummy any
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
		var dummy any
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
	adminTmpl.ExecuteTemplate(w, "umamiSettings", umamiSettingsData{
		URL:       settings.URL,
		WebsiteID: settings.WebsiteID,
		ScriptURL: settings.ScriptURL,
	})
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
	adminTmpl.ExecuteTemplate(w, "gotifySettings", gotifySettingsData{
		URL:     settings.URL,
		Token:   settings.Token,
		Enabled: settings.Enabled == "true",
	})
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
	adminTmpl.ExecuteTemplate(w, "verbosityStatus", verbosityData{Level: sev.String()})
}
