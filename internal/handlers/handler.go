// Package handlers provides HTTP handlers for the YouTube Playlist Randomizer
// web UI and API endpoints.
package handlers

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"

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
var tmpl = template.Must(template.New("").ParseFS(templateFS, "templates/*.gohtml"))

// Config holds all dependencies needed by the handlers.
type Config struct {
	Store         *store.Store
	Logger        *logging.Logger
	YTClient      *youtube.Client
	OTel          *telemetry.Telemetry
	OAuthSetup    *youtube.OAuthSetup
	GotifyClient  *gotify.Client
	AdminHandlers *admin.Handlers
	JobRunner     *job.Runner
	ClientSecretPath string
	DataDir       string
	Version       string
}

// Handlers groups all HTTP handler methods and their dependencies.
type Handlers struct {
	store         *store.Store
	logger        *logging.Logger
	ytClient      *youtube.Client
	otel          *telemetry.Telemetry
	oauthSetup    *youtube.OAuthSetup
	gotifyClient  *gotify.Client
	admin         *admin.Handlers
	jobRunner     *job.Runner
	clientSecretPath string
	dataDir       string
	version       string
}

// New creates a new Handlers from the given config.
func New(cfg *Config) *Handlers {
	return &Handlers{
		store:         cfg.Store,
		logger:        cfg.Logger,
		ytClient:      cfg.YTClient,
		otel:          cfg.OTel,
		oauthSetup:    cfg.OAuthSetup,
		gotifyClient:  cfg.GotifyClient,
		admin:         cfg.AdminHandlers,
		jobRunner:     cfg.JobRunner,
		clientSecretPath: cfg.ClientSecretPath,
		dataDir:       cfg.DataDir,
		version:       cfg.Version,
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
	mux.HandleFunc("/api/jobs/", h.handleJobStatus)
	mux.HandleFunc("/api/jobs/resume", h.handleForceResume)
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
		PlaylistID string `json:"playlistId"`
		NewName    string `json:"newName"`
	}

	JobResponse struct {
		JobID  string    `json:"jobId"`
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
		ID          string
		Title       string
		ItemCount   int
		ItemCountStr string
		Cost        int
		ButtonClass string
		ButtonText  string
		ModalURL    template.URL
	}

	ModalData struct {
		PlaylistID     string
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
		JobID         string
		Pct           int
		Done          int
		Total         int
		Label         string
		NewPlaylistURL template.URL
		Error         string
		ResumeBtnAttr template.HTML
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
	}
)

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func (h *Handlers) CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
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
