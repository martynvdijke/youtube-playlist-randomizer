// Package handlers provides HTTP handlers for the YouTube Playlist Randomizer
// web UI and API endpoints.
package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/admin"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/gotify"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/job"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/telemetry"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
)

//go:embed templates/*.gohtml
var templateFS embed.FS

// tmpl holds all parsed HTML templates.
var tmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"minus": func(a, b int) int { return a - b },
}).ParseFS(templateFS, "templates/*.gohtml"))

// Config holds all dependencies needed by the handlers.
type Config struct {
	Store            *store.Store
	Logger           *logging.Logger
	YTClient         *youtube.Client
	OTel             *telemetry.Telemetry
	OAuthSetup       *youtube.OAuthSetup
	GotifyClient     *gotify.Client
	AdminHandlers    *admin.Handlers
	JobRunner        *job.Runner
	ClientSecretPath string
	DataDir          string
	Version          string
}

// Handlers groups all HTTP handler methods and their dependencies.
type Handlers struct {
	store            *store.Store
	logger           *logging.Logger
	ytClient         *youtube.Client
	otel             *telemetry.Telemetry
	oauthSetup       *youtube.OAuthSetup
	gotifyClient     *gotify.Client
	admin            *admin.Handlers
	jobRunner        *job.Runner
	clientSecretPath string
	dataDir          string
	version          string
	csrfKey          []byte
}

// New creates a new Handlers from the given config.
func New(cfg *Config) *Handlers {
	csrfKey := make([]byte, 32)
	rand.Read(csrfKey)
	return &Handlers{
		store:            cfg.Store,
		logger:           cfg.Logger,
		ytClient:         cfg.YTClient,
		otel:             cfg.OTel,
		oauthSetup:       cfg.OAuthSetup,
		gotifyClient:     cfg.GotifyClient,
		admin:            cfg.AdminHandlers,
		jobRunner:        cfg.JobRunner,
		clientSecretPath: cfg.ClientSecretPath,
		dataDir:          cfg.DataDir,
		version:          cfg.Version,
		csrfKey:          csrfKey,
	}
}

// RegisterRoutes registers all HTTP routes on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/api/quota", h.handleQuota)
	mux.HandleFunc("/api/quota/html", h.handleQuotaHTML)
	mux.HandleFunc("/api/playlists", h.handlePlaylists)
	mux.HandleFunc("/api/playlists/html", h.handlePlaylistsHTML)
	mux.HandleFunc("/api/modal/html", h.handleModalHTML)
	mux.HandleFunc("/api/randomize", h.handleRandomize)
	mux.HandleFunc("/api/randomize/html", h.handleRandomizeHTML)
	mux.HandleFunc("/api/playlists/preview/html", h.handlePlaylistPreviewHTML)
	mux.HandleFunc("/api/jobs/", h.handleJobStatus)
	mux.HandleFunc("/api/jobs/resume", h.handleForceResume)
	mux.HandleFunc("/api/jobs/undo", h.handleUndo)
	mux.HandleFunc("/api/jobs/archive", h.handleArchiveJob)
	mux.HandleFunc("/api/jobs/delete", h.handleDeleteJob)
	mux.HandleFunc("/api/jobs/archived/html", h.handleArchivedJobsHTML)
	mux.HandleFunc("/api/jobs/queue/html", h.handleJobQueueHTML)
	mux.HandleFunc("/callback", h.handleOAuthCallback)
	mux.HandleFunc("/api/auth", h.handleAuth)

	// Admin panel routes
	mux.HandleFunc("/api/admin/logs/html", h.admin.HandleLogsHTML)
	mux.HandleFunc("/api/admin/settings/log_level", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.admin.HandleLogLevelGet(w, r)
		case http.MethodPost:
			h.admin.HandleLogLevelSet(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/admin/settings/email", h.admin.HandleSettingsEmail)
	mux.HandleFunc("/api/admin/settings/ai", h.admin.HandleSettingsAI)
	mux.HandleFunc("/api/admin/settings/umami", h.admin.HandleSettingsUmami)
	mux.HandleFunc("/api/admin/settings/umami/html", h.admin.HandleSettingsUmamiHTML)
	mux.HandleFunc("/api/admin/settings/gotify", h.admin.HandleSettingsGotify)
	mux.HandleFunc("/api/admin/settings/gotify/html", h.admin.HandleSettingsGotifyHTML)
	mux.HandleFunc("/api/admin/settings/otel", h.admin.HandleSettingsOTel)
	mux.HandleFunc("/api/admin/settings/otel/html", h.admin.HandleSettingsOTelHTML)

	// Root handler — serves the SPA entry point
	mux.HandleFunc("/", h.handleRoot)
}

// Response types shared across handlers.
type (
	PlaylistResponse struct {
		ID        string `json:"ID"`
		Title     string `json:"Title"`
		ItemCount int    `json:"ItemCount"`
	}

	RandomizeRequest struct {
		PlaylistID  string   `json:"playlistId"`
		PlaylistIDs []string `json:"playlistIds"`
		NewName     string   `json:"newName"`
	}

	JobResponse struct {
		JobID  string     `json:"jobId"`
		Status job.Status `json:"status"`
	}

	ErrorResponse struct {
		Error string `json:"error"`
	}

	QuotaResponse struct {
		Used      int    `json:"used"`
		Limit     int    `json:"limit"`
		Remaining int    `json:"remaining"`
		Date      string `json:"date"`
	}
)

// Template data types.
type (
	QuotaData struct {
		Used, Limit, Remaining int
		FillClass              string
		Pct                    float64
	}

	AuthRequiredData struct {
		AuthURL string
	}

	PlaylistCardData struct {
		ID           string
		Title        string
		ItemCount    int
		ItemCountStr string
		Cost         int
		ButtonClass  string
		ButtonText   string
		ModalURL     template.URL
		PreviewURL   template.URL
		PlaylistIdx  int
	}

	PreviewItemData struct {
		ThumbnailURL string
		Title        string
		VideoID      string
		ChannelTitle string
	}

	previewModalData struct {
		Title      string
		TotalItems int
		ShowCount  int
		Items      []PreviewItemData
	}

	ModalData struct {
		PlaylistIDs    string
		FirstID        string
		Title          string
		DefaultName    string
		Cost           int
		QuotaCostClass string
		QuotaText      string
		WarningHTML    template.HTML
	}

	RandomizeErrorData struct {
		Error string
	}

	JobProgressData struct {
		JobID          string
		Pct            int
		Done           int
		Total          int
		Label          string
		NewPlaylistURL template.URL
		Error          string
		ResumeBtnAttr  template.HTML
	}

	ForceResumeData struct {
		JobID string
	}

	JobQueueRowData struct {
		Status      string
		StatusClass string
		Title       string
		NewName     string
		Progress    string
		Created     string
		ActionHTML  template.HTML
		UndoHTML    template.HTML
		ArchiveHTML template.HTML
	}

	ArchivedJobRowData struct {
		ID        string
		Status    string
		Title     string
		NewName   string
		Progress  string
		Created   string
		DeleteURL string
	}
)

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

// csrfToken generates a time-bounded CSRF token signed with the server key.
func (h *Handlers) csrfToken() string {
	mac := hmac.New(sha256.New, h.csrfKey)
	// Token is valid for the current 1-hour window
	hour := time.Now().Truncate(time.Hour).Unix()
	fmt.Fprintf(mac, "%d", hour)
	return hex.EncodeToString(mac.Sum(nil))
}

// validateCSRF checks the X-CSRF-Token header or csrf_token form field against
// the csrf_token cookie. Skips validation for paths in skipCSRF.
func (h *Handlers) validateCSRF(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	// Skip CSRF for OAuth callback (external redirect)
	if strings.HasPrefix(r.URL.Path, "/callback") {
		return true
	}
	requestToken := r.Header.Get("X-CSRF-Token")
	if requestToken == "" {
		requestToken = r.FormValue("csrf_token")
	}
	cookie, err := r.Cookie("csrf_token")
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(cookie.Value), []byte(requestToken))
}

// CORSMiddleware adds permissive CORS headers for local development.
func (h *Handlers) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware sets CSRF cookies and validates CSRF tokens on state-changing
// requests.
func (h *Handlers) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CSRF cookie on every response (non-HttpOnly so JS can read it)
		token := h.csrfToken()
		http.SetCookie(w, &http.Cookie{
			Name:     "csrf_token",
			Value:    token,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
			HttpOnly: false,
			Secure:   false,
		})

		if !h.validateCSRF(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "CSRF validation failed"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleRoot serves the main index.html with dynamic version and umami injection.
func (h *Handlers) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	pageHTML, err := os.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	content := strings.Replace(string(pageHTML), `id="version-badge"></span>`, fmt.Sprintf(`id="version-badge">v%s</span>`, h.version), 1)

	// Inject CSRF token meta tag
	content = strings.Replace(content, "<!-- csrf -->",
		fmt.Sprintf(`<meta name="csrf-token" content="%s">`, h.csrfToken()), 1)

	umamiURL, _ := h.store.GetSetting("umami_url")
	umamiWebsiteID, _ := h.store.GetSetting("umami_website_id")
	if umamiURL != "" && umamiWebsiteID != "" {
		umamiScriptURL, _ := h.store.GetSetting("umami_script_url")
		if umamiScriptURL == "" {
			umamiScriptURL = strings.TrimRight(umamiURL, "/") + "/script.js"
		}
		scriptTag := fmt.Sprintf(`<script defer src="%s" data-website-id="%s"></script>`, umamiScriptURL, umamiWebsiteID)
		content = strings.Replace(content, "<!-- umami -->", scriptTag, 1)
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, content)
}

func (h *Handlers) oauthURL() string {
	if h.oauthSetup == nil {
		return ""
	}
	return youtube.AuthURL(h.oauthSetup)
}
