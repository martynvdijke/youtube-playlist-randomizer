package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/admin"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/telemetry"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const version = "1.9.0"

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobFetching  JobStatus = "fetching"
	JobShuffling JobStatus = "shuffling"
	JobInserting JobStatus = "inserting"
	JobDone      JobStatus = "done"
	JobError     JobStatus = "error"
	JobPaused    JobStatus = "paused"
)

type jobProgress struct {
	mu            sync.RWMutex
	Status        JobStatus `json:"status"`
	Progress      int       `json:"progress"`
	Total         int       `json:"total"`
	Done          int       `json:"done"`
	NewPlaylistID string    `json:"newPlaylistId,omitempty"`
	Error         string    `json:"error,omitempty"`
}

type PlaylistResponse struct {
	ID        string `json:"ID"`
	Title     string `json:"Title"`
	ItemCount int    `json:"ItemCount"`
}

type RandomizeRequest struct {
	PlaylistID string `json:"playlistId"`
	NewName    string `json:"newName"`
}

type JobResponse struct {
	JobID  string    `json:"jobId"`
	Status JobStatus `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type QuotaResponse struct {
	Used      int    `json:"used"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
	Date      string `json:"date"`
}

var (
	ytClient         *youtube.Client
	db               *store.Store
	dataDir          string
	jobsMu           sync.Mutex
	otel             *telemetry.Telemetry
	oauthSetup       *youtube.OAuthSetup
	clientSecretPath string
	logger           *logging.Logger
	adminHandlers    *admin.Handlers
)

func findClientSecret() string {
	if os.Getenv("DOCKER") == "true" {
		paths := []string{"/config/client_secret.json", "client_secret.json"}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return "/config/client_secret.json"
	}
	paths := []string{"client_secret.json", "/config/client_secret.json"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "client_secret.json"
}

func defaultDataDir() string {
	if os.Getenv("DOCKER") == "true" {
		return "/db"
	}
	return "."
}

func printQuotaBanner(q *store.QuotaInfo) {
	pct := float64(q.Used) / float64(q.Limit) * 100
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("  YouTube Playlist Randomizer v" + version)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  Data directory : %s\n", dataDir)
	fmt.Printf("  DB path       : %s\n", filepath.Join(dataDir, "ypr.db"))
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("  API Quota\n")
	fmt.Printf("    Used      : %d / %d (%.1f%%)\n", q.Used, q.Limit, pct)
	fmt.Printf("    Remaining : %d\n", q.Remaining)
	fmt.Println(strings.Repeat("=", 50))
}

func main() {
	port := flag.Int("p", 6270, "Port to listen on")
	input := flag.String("i", "", "Client secret JSON file path")
	showVersion := flag.Bool("version", false, "Print version")
	dataDirFlag := flag.String("d", defaultDataDir(), "Data directory for DB and cached token")
	mockMode := flag.Bool("mock", false, "Run in mock mode (no YouTube API credentials needed)")
	reauth := flag.Bool("reauth", false, "Force re-authentication by deleting cached token and re-running OAuth flow")

	flag.Parse()

	if *showVersion {
		fmt.Printf("youtube-playlist-randomizer version %s\n", version)
		os.Exit(0)
	}

	dataDir = *dataDirFlag

	if *reauth {
		tokenPath := filepath.Join(dataDir, "token.json")
		if err := os.Remove(tokenPath); err == nil {
			fmt.Printf("Deleted cached token at %s\n", tokenPath)
		} else if !os.IsNotExist(err) {
			fmt.Printf("Warning: could not delete token: %v\n", err)
		}
	}

	var err error
	otel, err = telemetry.New()
	if err != nil {
		log.Printf("warning: failed to initialize telemetry: %v", err)
		otel = nil
	}

	dbPath := filepath.Join(dataDir, "ypr.db")
	db, err = store.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	if otel != nil {
		defer otel.Shutdown(context.Background())
	}

	// Initialize structured logger with default WARN level
	defaultLevel := logging.WARN
	levelStr, _ := db.GetSetting("log_level")
	if levelStr != "" {
		defaultLevel = logging.ParseSeverity(levelStr)
	}
	logger = logging.New(logging.LogOptions{
		Store:    db,
		MinLevel: defaultLevel,
		Service:  "ypr",
	})
	adminHandlers = admin.New(db, logger)
	logger.Infoc(context.Background(), "Application started", "version", version)

	ctx := context.Background()

	if *mockMode {
		log.Println("Running in mock mode (no YouTube API)")
		ytClient = nil
	} else {
		secretPath := *input
		if secretPath == "" {
			secretPath = findClientSecret()
		}
		clientSecretPath = secretPath
		oauthSetup, err = youtube.LoadConfig(secretPath)
		if err != nil {
			log.Printf("WARNING: Failed to load OAuth config: %v", err)
		}

		ytClient, err = youtube.NewClient(ctx, secretPath, dataDir, otel)
		if err != nil {
			log.Printf("WARNING: Failed to create YouTube client: %v", err)
			log.Printf("Server will start without YouTube API access. Re-authenticate to restore functionality.")
			ytClient = nil
		}

		if ytClient != nil {
			if _, err := db.AddQuota(store.QuotaListPlaylists); err != nil {
				log.Printf("warning: failed to track quota: %v", err)
			}
		}
	}

	quota, err := db.GetQuota()
	if err != nil {
		log.Fatalf("Failed to get quota: %v", err)
	}
	printQuotaBanner(quota)

	if !*mockMode {
		resumePendingJobs(ctx)

		go jobPoller(ctx)
	}

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/api/quota", handleQuota)
	mux.HandleFunc("/api/quota/html", handleQuotaHTML)
	mux.HandleFunc("/api/playlists", handlePlaylists)
	mux.HandleFunc("/api/playlists/html", handlePlaylistsHTML)
	mux.HandleFunc("/api/modal/html", handleModalHTML)
	mux.HandleFunc("/api/randomize", handleRandomize)
	mux.HandleFunc("/api/randomize/html", handleRandomizeHTML)
	mux.HandleFunc("/api/jobs/", handleJobStatus)
	mux.HandleFunc("/api/jobs/resume", handleForceResume)
	mux.HandleFunc("/api/jobs/queue/html", handleJobQueueHTML)
	mux.HandleFunc("/callback", handleOAuthCallback)
	mux.HandleFunc("/api/auth", handleAuth)

	// Admin panel routes
	mux.HandleFunc("/api/admin/logs/html", adminHandlers.HandleLogsHTML)
	mux.HandleFunc("/api/admin/settings/log_level", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			adminHandlers.HandleLogLevelGet(w, r)
		case http.MethodPost:
			adminHandlers.HandleLogLevelSet(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/admin/settings/email", adminHandlers.HandleSettingsEmail)
	mux.HandleFunc("/api/admin/settings/ai", adminHandlers.HandleSettingsAI)
	mux.HandleFunc("/api/admin/settings/umami", adminHandlers.HandleSettingsUmami)
	mux.HandleFunc("/api/admin/settings/umami/html", adminHandlers.HandleSettingsUmamiHTML)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		pageHTML, err := os.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		content := strings.Replace(string(pageHTML), `id="version-badge"></span>`, fmt.Sprintf(`id="version-badge">v%s</span>`, version), 1)

		// Inject Umami tracking script if configured
		umamiURL, _ := db.GetSetting("umami_url")
		umamiWebsiteID, _ := db.GetSetting("umami_website_id")
		if umamiURL != "" && umamiWebsiteID != "" {
			umamiScriptURL, _ := db.GetSetting("umami_script_url")
			if umamiScriptURL == "" {
				umamiScriptURL = strings.TrimRight(umamiURL, "/") + "/script.js"
			}
			scriptTag := fmt.Sprintf(`<script defer src="%s" data-website-id="%s"></script>`, html.EscapeString(umamiScriptURL), html.EscapeString(umamiWebsiteID))
			content = strings.Replace(content, "<!-- umami -->", scriptTag, 1)
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, content)
	})

	addr := fmt.Sprintf(":%d", *port)

	var handler http.Handler = corsMiddleware(mux)
	if otel != nil {
		handler = otel.Middleware(handler)
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("\nWeb UI started at http://localhost:%d\n", *port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

func handleQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	q, err := db.GetQuota()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if otel != nil {
		otel.RecordQuotaMetrics(r.Context(), q.Used, q.Limit, q.Remaining)
	}
	writeJSON(w, http.StatusOK, QuotaResponse{
		Used:      q.Used,
		Limit:     q.Limit,
		Remaining: q.Remaining,
		Date:      q.Date,
	})
}

func handlePlaylists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if ytClient == nil {
		writeJSON(w, http.StatusOK, []PlaylistResponse{})
		return
	}

	playlists, err := ytClient.GetPlaylists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := db.AddQuota(store.QuotaListPlaylists); err != nil {
		log.Printf("warning: failed to track quota: %v", err)
	}

	resp := make([]PlaylistResponse, 0, len(playlists))
	for _, pl := range playlists {
		resp = append(resp, PlaylistResponse{
			ID:        pl.ID,
			Title:     pl.Title,
			ItemCount: pl.ItemCount,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func handleRandomize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if ytClient == nil {
		writeError(w, http.StatusBadRequest, "YouTube API not available in mock mode")
		return
	}

	var req RandomizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.PlaylistID == "" || req.NewName == "" {
		writeError(w, http.StatusBadRequest, "playlistId and newName are required")
		return
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())

	if err := db.CreateJob(jobID, req.PlaylistID, "", req.NewName); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create job")
		return
	}

	jp := &jobProgress{Status: JobPending}

	jobsMu.Lock()
	jobsMu.Unlock()

	if otel != nil {
		otel.RecordJobCreated(r.Context())
	}

	go runJob(context.Background(), jobID, jp, req.PlaylistID, req.NewName)

	writeJSON(w, http.StatusAccepted, JobResponse{JobID: jobID, Status: JobPending})
}

func getJobProgress(jobID string) *jobProgress {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	j, err := db.GetJob(jobID)
	if err != nil {
		return nil
	}

	jp := &jobProgress{
		Status:        JobStatus(j.Status),
		Total:         j.TotalItems,
		Done:          j.InsertedItems,
		NewPlaylistID: j.NewPlaylistID,
		Error:         j.Error,
	}
	return jp
}

func handleJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	// Strip /html suffix if present for htmx endpoint
	if strings.HasSuffix(jobID, "/html") {
		jobID = strings.TrimSuffix(jobID, "/html")
		if r.Header.Get("HX-Request") != "true" {
			r.Header.Set("HX-Request", "true")
		}
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if jobID == "" {
		writeError(w, http.StatusBadRequest, "Missing job ID")
		return
	}

	jp := getJobProgress(jobID)
	if jp == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		writeJobProgressHTML(w, jobID, jp)
		return
	}

	writeJSON(w, http.StatusOK, jp)
}

func handleForceResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	jobID := r.FormValue("jobId")
	if jobID == "" {
		jobID = r.URL.Query().Get("jobId")
	}
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "Missing job ID")
		return
	}

	j, err := db.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	ctx := context.Background()
	if ytClient == nil {
		writeError(w, http.StatusBadRequest, "YouTube API not available")
		return
	}

	switch j.Status {
	case "pending":
		jp := &jobProgress{Status: JobPending}
		go runJob(ctx, j.ID, jp, j.SourcePlaylistID, j.NewName)
		fmt.Fprintf(w, `<div id="progress-content" class="modal-content" hx-get="/api/jobs/%s/html" hx-trigger="every 1500ms" hx-swap="outerHTML">
  <p>Force-resumed! Starting job...</p>
</div>`, html.EscapeString(jobID))

	case "paused", "fetching", "shuffling", "inserting":
		jp := &jobProgress{
			Status:        JobInserting,
			Total:         j.TotalItems,
			Done:          j.InsertedItems,
			NewPlaylistID: j.NewPlaylistID,
		}
		go resumeJob(ctx, *j, jp)
		fmt.Fprintf(w, `<div id="progress-content" class="modal-content" hx-get="/api/jobs/%s/html" hx-trigger="every 1500ms" hx-swap="outerHTML">
  <p>Force-resumed! Continuing job...</p>
</div>`, html.EscapeString(jobID))

	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Job is in state %s and cannot be resumed", j.Status))
	}
}

func handleJobQueueHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs, err := db.GetPendingJobs()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if len(jobs) == 0 {
		fmt.Fprint(w, `<div id="job-queue" class="job-queue hidden"></div>`)
		return
	}

	fmt.Fprint(w, `<div id="job-queue" class="job-queue">`)
	fmt.Fprint(w, `<h3>Resume Queue</h3><table class="job-table"><thead><tr><th>Status</th><th>Playlist</th><th>New Name</th><th>Progress</th><th>Created</th></tr></thead><tbody>`)
	for _, j := range jobs {
		label := j.SourceTitle
		if label == "" {
			label = j.SourcePlaylistID
		}
		progress := ""
		if j.TotalItems > 0 {
			progress = fmt.Sprintf("%d / %d", j.InsertedItems, j.TotalItems)
		} else {
			progress = "-"
		}
		created := j.CreatedAt
		if len(created) > 19 {
			created = created[:19]
		}
		created = strings.Replace(created, "T", " ", 1)
		fmt.Fprintf(w, `<tr><td class="status-%s">%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(j.Status), html.EscapeString(j.Status),
			html.EscapeString(label), html.EscapeString(j.NewName),
			html.EscapeString(progress), html.EscapeString(created))
	}
	fmt.Fprint(w, `</tbody></table></div>`)
}

func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	if oauthSetup == nil {
		http.Error(w, "OAuth not configured", http.StatusInternalServerError)
		return
	}

	if err := youtube.HandleCallback(oauthSetup, code, dataDir); err != nil {
		log.Printf("OAuth callback error: %v", err)
		http.Error(w, "Authentication failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("OAuth authentication successful! Token saved. Recreating YouTube client...")

	newClient, err := youtube.NewClient(context.Background(), clientSecretPath, dataDir, otel)
	if err == nil && newClient != nil {
		ytClient = newClient
		log.Printf("YouTube client recreated successfully!")
	} else if err == nil && newClient == nil {
		log.Printf("Warning: token still invalid after callback (unexpected)")
	} else {
		log.Printf("Warning: new client error (non-critical): %v", err)
		// If we got here but have a client with no service (quota exhausted),
		// still accept it — the token is valid
		if newClient != nil {
			ytClient = newClient
		}
	}

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<html><body style="background:#0f0f0f;color:#eee;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;flex-direction:column">
<h1 style="color:#4caf50">✓ Authentication successful!</h1>
<p style="color:#aaa;margin-top:8px">YouTube API is now available.</p>
<p style="color:#888;font-size:13px;margin-top:16px">You may close this window and <a href="/" style="color:#ff4444">reload the app</a>.</p>
</body></html>`)
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	if oauthSetup == nil {
		http.Error(w, "OAuth not configured (no client_secret.json found)", http.StatusInternalServerError)
		return
	}

	url := youtube.AuthURL(oauthSetup)
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="auth-error">
  <p><strong>YouTube API authentication required.</strong></p>
  <p style="margin:12px 0;color:#8899aa">Sign in with Google to allow shuffle access to your playlists.</p>
  <div style="text-align:center;margin:16px 0">
    <a class="btn btn-primary" href="%s" style="display:inline-block;padding:12px 28px;text-decoration:none">Sign in with Google</a>
  </div>
  <p style="font-size:12px;color:#668">After signing in you'll be redirected back — then <a href="/" style="color:#ff4444">reload the app</a>.</p>
</div>`, html.EscapeString(url))
}

func writeQuotaPct(used, limit int) (float64, string) {
	pct := 0.0
	if limit > 0 {
		pct = float64(used) / float64(limit) * 100
	}
	displayPct := pct
	if displayPct > 100 {
		displayPct = 100
	}
	fillClass := "quota-fill"
	if pct > 80 {
		fillClass += " quota-critical"
	} else if pct > 50 {
		fillClass += " quota-warning"
	}
	return displayPct, fillClass
}

func handleQuotaHTML(w http.ResponseWriter, r *http.Request) {
	q, err := db.GetQuota()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	pct, fillClass := writeQuotaPct(q.Used, q.Limit)
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<div class="quota-label"><span>Quota: %d / %d used (%d remaining)</span></div>
<div class="quota-track"><div class="%s" style="width:%.1f%%"></div></div>`,
		q.Used, q.Limit, q.Remaining, fillClass, pct)
}

func handlePlaylistsHTML(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))

	if ytClient == nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<p>No playlists found.</p>`)
		return
	}

	playlists, err := ytClient.GetPlaylists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := db.AddQuota(store.QuotaListPlaylists); err != nil {
		log.Printf("warning: failed to track quota: %v", err)
	}

	quota, _ := db.GetQuota()

	var filtered []PlaylistResponse
	for _, pl := range playlists {
		if query == "" || strings.Contains(strings.ToLower(pl.Title), query) {
			filtered = append(filtered, PlaylistResponse{
				ID: pl.ID, Title: pl.Title, ItemCount: pl.ItemCount,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html")
	if len(filtered) == 0 {
		if query != "" {
			fmt.Fprint(w, `<p class="no-results">No playlists match your filter.</p>`)
		} else {
			fmt.Fprint(w, `<p>No playlists found.</p>`)
		}
		return
	}

	for _, pl := range filtered {
		cost := store.QuotaCreatePlaylist + pl.ItemCount*store.QuotaInsertItem
		insufficient := quota != nil && quota.Remaining < cost
		btnClass := "btn btn-randomize"
		btnText := "Randomize"
		if insufficient {
			btnClass += " btn-warning"
			btnText = "Randomize (may resume later)"
		}

		itemCountStr := "?"
		if pl.ItemCount > 0 {
			itemCountStr = strconv.Itoa(pl.ItemCount)
		}

		titleEncoded := url.QueryEscape(pl.Title)

		fmt.Fprintf(w, `<div class="playlist-card">
  <div class="playlist-info">
    <span class="playlist-title">%s</span>
    <span class="playlist-count">%s videos &middot; ~%d quota</span>
  </div>
  <button class="%s" hx-get="/api/modal/html?id=%s&amp;itemCount=%d&amp;title=%s" hx-target="#modal" hx-swap="innerHTML" hx-on::after-request="showModal()">%s</button>
</div>`,
			html.EscapeString(pl.Title), itemCountStr, cost,
			btnClass, pl.ID, pl.ItemCount, titleEncoded, btnText)
	}
}

func handleModalHTML(w http.ResponseWriter, r *http.Request) {
	playlistID := r.URL.Query().Get("id")
	itemCountStr := r.URL.Query().Get("itemCount")
	itemCount, _ := strconv.Atoi(itemCountStr)
	title := r.URL.Query().Get("title")
	if title == "" {
		// If no title provided, try to fetch from YouTube
		title = "Selected Playlist"
	}

	quota, _ := db.GetQuota()
	cost := store.QuotaCreatePlaylist + itemCount*store.QuotaInsertItem

	now := time.Now()
	monthYear := now.Format("January 2006")
	defaultName := fmt.Sprintf("%s-randomized-%s", title, monthYear)

	w.Header().Set("Content-Type", "text/html")
	lowQuota := quota != nil && quota.Remaining < cost
	warningHtml := ""
	if lowQuota {
		warningHtml = `<div class="quota-warning-banner"><p>⚠️ Insufficient quota for one session. The job will save progress and auto-resume when quota is available (within ~24h).</p></div>`
	}
	fmt.Fprintf(w, `<div class="modal-content">
  <h2>Randomize Playlist</h2>
  <p>%s</p>
  <p class="quota-cost %s">Estimated quota cost: %d units (%s remaining)</p>
  %s
  <form hx-post="/api/randomize/html" hx-target="#progress-modal" hx-swap="innerHTML" hx-on::after-request="showProgressModal()">
    <input type="hidden" name="playlistId" value="%s">
    <label for="new-name">Name for new randomized playlist:</label>
    <input type="text" id="new-name" name="newName" placeholder="Enter playlist name" value="%s" required>
    <div class="modal-actions">
      <button type="button" class="btn btn-secondary" onclick="closeModal()">Cancel</button>
      <button type="submit" class="btn btn-primary">Randomize</button>
    </div>
  </form>
</div>`,
		html.EscapeString(title),
		quotaCostClass(quota, cost), cost, quotaText(quota, cost),
		warningHtml,
		html.EscapeString(playlistID),
		html.EscapeString(defaultName))
}

func quotaCostClass(quota *store.QuotaInfo, cost int) string {
	if quota == nil {
		return "quota-cost quota-low"
	}
	if quota.Remaining >= cost {
		return "quota-cost quota-ok"
	}
	return "quota-cost quota-warning"
}

func quotaText(quota *store.QuotaInfo, cost int) string {
	if quota == nil {
		return "Unknown"
	}
	if quota.Remaining >= cost {
		return "Sufficient"
	}
	return "Low (will resume)"
}

func handleRandomizeHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, `<p class="error">Method not allowed</p>`)
		return
	}

	if ytClient == nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `<p class="error">YouTube API not available in mock mode</p>`)
		return
	}

	playlistID := r.FormValue("playlistId")
	newName := r.FormValue("newName")

	if playlistID == "" || newName == "" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `<p class="error">playlistId and newName are required</p>`)
		return
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())

	if err := db.CreateJob(jobID, playlistID, "", newName); err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `<p class="error">Failed to create job: %s</p>`, html.EscapeString(err.Error()))
		return
	}

	jp := &jobProgress{Status: JobPending}
	go runJob(context.Background(), jobID, jp, playlistID, newName)

	w.Header().Set("Content-Type", "text/html")
	writeJobProgressHTML(w, jobID, jp)
}

func writeJobProgressHTML(w http.ResponseWriter, jobID string, jp *jobProgress) {
	jp.mu.RLock()
	status := jp.Status
	total := jp.Total
	done := jp.Done
	newPlaylistID := jp.NewPlaylistID
	errStr := jp.Error
	jp.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html")

	switch status {
	case JobPending:
		fmt.Fprintf(w, `<div id="progress-content" class="modal-content" hx-get="/api/jobs/%s/html" hx-trigger="every 30000ms" hx-swap="outerHTML">
  <h2>Queued</h2>
  <div class="progress-bar"><div class="progress-fill" style="width:0%%"></div></div>
  <p>Waiting for API quota to become available. The job will run automatically when quota resets.</p>
  <div style="text-align:center;margin-top:12px">
    <button class="btn btn-primary" onclick="closeProgressModal()">OK</button>
  </div>
</div>`, html.EscapeString(jobID))

	case JobFetching, JobShuffling:
		label := "Starting..."
		pct := 0
		switch status {
		case JobFetching:
			label = "Fetching playlist items..."
			pct = 25
		case JobShuffling:
			label = "Shuffling items..."
			pct = 50
		}
		fmt.Fprintf(w, `<div id="progress-content" class="modal-content" hx-get="/api/jobs/%s/html" hx-trigger="every 1500ms" hx-swap="outerHTML">
  <h2>Randomizing...</h2>
  <div class="progress-bar"><div class="progress-fill" style="width:%d%%"></div></div>
  <p>%s</p>
</div>`, html.EscapeString(jobID), pct, html.EscapeString(label))

	case JobInserting:
		pct := 50
		if total > 0 {
			pct = int(float64(done)/float64(total)*50) + 50
			if pct > 99 {
				pct = 99
			}
		}
		fmt.Fprintf(w, `<div id="progress-content" class="modal-content" hx-get="/api/jobs/%s/html" hx-trigger="every 1500ms" hx-swap="outerHTML">
  <h2>Randomizing...</h2>
  <div class="progress-bar"><div class="progress-fill" style="width:%d%%"></div></div>
  <p>Inserting items... %d / %d</p>
</div>`, html.EscapeString(jobID), pct, done, total)

	case JobDone:
		link := ""
		if newPlaylistID != "" {
			link = fmt.Sprintf(`<a id="playlist-link" href="https://www.youtube.com/playlist?list=%s" target="_blank">Open in YouTube</a>`, html.EscapeString(newPlaylistID))
		}
		// Also update quota display after job completes
		fmt.Fprintf(w, `<div id="progress-result" class="modal-content" hx-get="/api/quota/html" hx-trigger="load" hx-target="#quota-bar" hx-swap="innerHTML">
  <h2>Randomizing...</h2>
  <div class="progress-bar"><div class="progress-fill progress-done" style="width:100%%"></div></div>
  <p>Playlist created successfully!</p>
  <div id="progress-result" style="text-align:center">
    %s
    <button class="btn btn-primary" onclick="closeProgressModal()">Done</button>
  </div>
</div>`, link)

	case JobPaused:
		pct := 0
		if total > 0 {
			pct = int(float64(done)/float64(total) * 100)
		}
		fmt.Fprintf(w, `<div id="progress-paused-content" class="modal-content" hx-get="/api/jobs/%s/html" hx-trigger="every 60000ms" hx-swap="outerHTML">
  <h2>Randomizing...</h2>
  <div class="progress-bar"><div class="progress-fill" style="width:%d%%"></div></div>
  <p>Inserted %d / %d items</p>
  <div class="paused-banner">
    <p>Quota exhausted. Progress saved.</p>
    <p class="paused-sub">Auto-resume in ~24h (or force-resume below if quota is available).</p>
    <div style="display:flex;gap:8px;justify-content:center;margin-top:12px">
      <button class="btn btn-primary" onclick="closeProgressModal()">OK</button>
      <button class="btn btn-warning" hx-post="/api/jobs/resume" hx-vals='{"jobId":"%s"}' hx-target="#progress-paused-content" hx-swap="outerHTML">Resume Now</button>
    </div>
  </div>
</div>`, html.EscapeString(jobID), pct, done, total, html.EscapeString(jobID))

	case JobError:
		fmt.Fprintf(w, `<div id="progress-error-content" class="modal-content">
  <h2>Randomizing...</h2>
  <div class="progress-bar"><div class="progress-fill" style="width:100%%"></div></div>
  <p class="error">%s</p>
  <div style="text-align:center;margin-top:12px">
    <button class="btn btn-primary" onclick="closeProgressModal()">OK</button>
  </div>
</div>`, html.EscapeString(errStr))

	default:
		fmt.Fprintf(w, `<div id="progress-content" class="modal-content" hx-get="/api/jobs/%s/html" hx-trigger="every 1500ms" hx-swap="outerHTML">
  <h2>Randomizing...</h2>
  <div class="progress-bar"><div class="progress-fill" style="width:0%%"></div></div>
  <p>Starting...</p>
</div>`, html.EscapeString(jobID))
	}
}

func runJob(ctx context.Context, jobID string, jp *jobProgress, playlistID, newName string) {
	var span trace.Span
	if otel != nil {
		ctx, span = otel.Tracer.Start(ctx, "runJob",
			trace.WithAttributes(
				attribute.String("job.id", jobID),
				attribute.String("playlist.id", playlistID),
				attribute.String("playlist.name", newName),
			),
		)
		defer span.End()
	}

	updateStatus := func(s JobStatus) {
		jp.mu.Lock()
		jp.Status = s
		jp.mu.Unlock()
		db.UpdateJobStatus(jobID, string(s))
	}

	setError := func(errMsg string) {
		jp.mu.Lock()
		jp.Status = JobError
		jp.Error = errMsg
		jp.mu.Unlock()
		db.SetJobError(jobID, errMsg)
		if span != nil {
			span.SetStatus(codes.Error, errMsg)
			span.RecordError(fmt.Errorf("%s", errMsg))
		}
		if otel != nil {
			otel.RecordJobFailed(context.Background(), errMsg)
		}
	}

	updateProgress := func(done, total int, newPlaylistID string) {
		jp.mu.Lock()
		jp.Done = done
		jp.Total = total
		if newPlaylistID != "" {
			jp.NewPlaylistID = newPlaylistID
		}
		jp.mu.Unlock()
		db.UpdateJobProgress(jobID, done, newPlaylistID)
	}

	quota, err := db.GetQuota()
	if err != nil {
		log.Printf("warning: quota check failed for job %s: %v", jobID, err)
	}
	if quota == nil || quota.Remaining < store.QuotaListPlaylistItems {
		log.Printf("Insufficient quota to fetch items for job %s (remaining: %d). Job will wait.", jobID, quota.Remaining)
		updateStatus(JobPending)
		return
	}

	updateStatus(JobFetching)

	items, err := ytClient.GetPlaylistItems(ctx, playlistID)
	if err != nil {
		if youtube.IsQuotaError(err) {
			log.Printf("Quota error fetching items for job %s. Pausing.", jobID)
			updateStatus(JobPaused)
			db.SetJobPaused(jobID)
			if otel != nil {
				otel.RecordJobPaused(context.Background(), 0, 0)
			}
			return
		}
		setError(fmt.Sprintf("Failed to fetch playlist items: %v", err))
		return
	}
	if _, err := db.AddQuota(store.QuotaListPlaylistItems); err != nil {
		log.Printf("warning: failed to track quota: %v", err)
	}

	if len(items) == 0 {
		setError("Playlist has no items")
		return
	}

	updateStatus(JobShuffling)

	shuffled := make([]string, len(items))
	for i, item := range items {
		shuffled[i] = item.VideoID
	}
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	if err := db.SaveShuffledItems(jobID, shuffled); err != nil {
		setError(fmt.Sprintf("Failed to save shuffled items: %v", err))
		return
	}

	newPlaylistID, err := ytClient.CreatePlaylist(ctx, newName)
	if err != nil {
		if youtube.IsQuotaError(err) {
			log.Printf("Quota error creating playlist for job %s. Pausing.", jobID)
			updateStatus(JobPaused)
			db.SetJobPaused(jobID)
			if otel != nil {
				otel.RecordJobPaused(context.Background(), 0, jp.Total)
			}
			return
		}
		setError(fmt.Sprintf("Failed to create playlist: %v", err))
		return
	}
	if _, err := db.AddQuota(store.QuotaCreatePlaylist); err != nil {
		log.Printf("warning: failed to track quota: %v", err)
	}

	updateProgress(0, len(shuffled), newPlaylistID)
	updateStatus(JobInserting)

	uninserted, err := db.GetUninsertedItems(jobID)
	if err != nil {
		setError(fmt.Sprintf("Failed to get uninserted items: %v", err))
		return
	}

	for _, item := range uninserted {
		quota, err := db.GetQuota()
		if err != nil {
			setError(fmt.Sprintf("Failed to check quota: %v", err))
			return
		}
		if quota.Remaining < store.QuotaInsertItem {
			log.Printf("Quota exhausted after %d/%d items. Job %s paused.", jp.Done, jp.Total, jobID)
			updateStatus(JobPaused)
			db.SetJobPaused(jobID)
			if otel != nil {
				otel.RecordJobPaused(context.Background(), jp.Done, jp.Total)
			}
			return
		}

		if err := ytClient.InsertPlaylistItem(ctx, newPlaylistID, item.VideoID, int64(item.Position)); err != nil {
			if youtube.IsQuotaError(err) {
				log.Printf("Quota error during insert at %d/%d. Job %s paused.", jp.Done, jp.Total, jobID)
				updateStatus(JobPaused)
				db.SetJobPaused(jobID)
				if otel != nil {
					otel.RecordJobPaused(context.Background(), jp.Done, jp.Total)
				}
				return
			}
			log.Printf("warning: failed to insert item at position %d (video %s): %v", item.Position, item.VideoID, err)
			continue
		}
		if _, err := db.AddQuota(store.QuotaInsertItem); err != nil {
			log.Printf("warning: failed to track quota: %v", err)
		}
		if err := db.MarkItemInserted(jobID, item.Position); err != nil {
			log.Printf("warning: failed to mark item inserted: %v", err)
		}

		done := jp.Done + 1
		updateProgress(done, jp.Total, "")

		time.Sleep(10 * time.Millisecond)

		if done%100 == 0 {
			log.Printf("Inserted %d/%d items for job %s", done, jp.Total, jobID)
		}
	}

	log.Printf("Successfully inserted %d items into playlist %s", jp.Total, newPlaylistID)
	updateProgress(jp.Total, jp.Total, "")
	updateStatus(JobDone)
	db.SetJobDone(jobID)
	if span != nil {
		span.SetAttributes(attribute.Int("items.total", jp.Total))
		span.SetStatus(codes.Ok, "")
	}
	if otel != nil {
		otel.RecordJobCompleted(context.Background(), jp.Total)
	}
}

func resumeJob(ctx context.Context, j store.Job, jp *jobProgress) {
	var span trace.Span
	if otel != nil {
		ctx, span = otel.Tracer.Start(ctx, "resumeJob",
			trace.WithAttributes(
				attribute.String("job.id", j.ID),
				attribute.String("playlist.id", j.SourcePlaylistID),
				attribute.String("playlist.name", j.NewName),
				attribute.Int("items.inserted", j.InsertedItems),
				attribute.Int("items.total", j.TotalItems),
			),
		)
		defer span.End()
	}

	updateStatus := func(s JobStatus) {
		jp.mu.Lock()
		jp.Status = s
		jp.mu.Unlock()
		db.UpdateJobStatus(j.ID, string(s))
	}

	setError := func(errMsg string) {
		jp.mu.Lock()
		jp.Status = JobError
		jp.Error = errMsg
		jp.mu.Unlock()
		db.SetJobError(j.ID, errMsg)
		if span != nil {
			span.SetStatus(codes.Error, errMsg)
			span.RecordError(fmt.Errorf("%s", errMsg))
		}
	}

	updateProgress := func(done, total int) {
		jp.mu.Lock()
		jp.Done = done
		jp.Total = total
		jp.mu.Unlock()
		db.UpdateJobProgress(j.ID, done, jp.NewPlaylistID)
	}

	newPlaylistID := j.NewPlaylistID
	if newPlaylistID == "" {
		newPlaylistID, _ = func() (string, error) {
			id, err := ytClient.CreatePlaylist(ctx, j.NewName)
			if youtube.IsQuotaError(err) {
				log.Printf("Quota error creating playlist during resume for job %s. Pausing.", j.ID)
				db.SetJobPaused(j.ID)
				return "", err
			}
			if err == nil {
				if _, qErr := db.AddQuota(store.QuotaCreatePlaylist); qErr != nil {
					log.Printf("warning: failed to track quota: %v", qErr)
				}
				db.UpdateJobNewPlaylistID(j.ID, id)
			}
			return id, err
		}()
		if newPlaylistID == "" {
			setError("Failed to create playlist on resume")
			return
		}
		jp.mu.Lock()
		jp.NewPlaylistID = newPlaylistID
		jp.mu.Unlock()
	}

	updateStatus(JobInserting)

	uninserted, err := db.GetUninsertedItems(j.ID)
	if err != nil {
		setError(fmt.Sprintf("Failed to get uninserted items: %v", err))
		return
	}

	jp.mu.Lock()
	jp.Total = j.TotalItems
	jp.mu.Unlock()

	for _, item := range uninserted {
		quota, err := db.GetQuota()
		if err != nil {
			setError(fmt.Sprintf("Failed to check quota: %v", err))
			return
		}
		if quota.Remaining < store.QuotaInsertItem {
			log.Printf("Quota exhausted during resume at %d/%d. Job %s paused again.", jp.Done, jp.Total, j.ID)
			updateStatus(JobPaused)
			db.SetJobPaused(j.ID)
			return
		}

			if err := ytClient.InsertPlaylistItem(ctx, newPlaylistID, item.VideoID, int64(item.Position)); err != nil {
				if youtube.IsQuotaError(err) {
					log.Printf("Quota error during resume insert at %d/%d. Job %s paused.", jp.Done, jp.Total, j.ID)
					updateStatus(JobPaused)
					db.SetJobPaused(j.ID)
					return
				}
				log.Printf("warning: failed to insert item at position %d (video %s): %v", item.Position, item.VideoID, err)
				continue
			}
			if _, qErr := db.AddQuota(store.QuotaInsertItem); qErr != nil {
				log.Printf("warning: failed to track quota: %v", qErr)
			}
			db.MarkItemInserted(j.ID, item.Position)

			done := jp.Done + 1
			updateProgress(done, jp.Total)

			time.Sleep(10 * time.Millisecond)

			if done%100 == 0 {
				log.Printf("Resume: inserted %d/%d items for job %s", done, jp.Total, j.ID)
			}
		}

		log.Printf("Resume complete: inserted %d items into playlist %s", jp.Total, newPlaylistID)
	updateProgress(jp.Total, jp.Total)
	updateStatus(JobDone)
	db.SetJobDone(j.ID)
	if span != nil {
		span.SetAttributes(attribute.Int("items.total", jp.Total))
		span.SetStatus(codes.Ok, "")
	}
}

func resumePendingJobs(ctx context.Context) {
	jobs, err := db.GetPendingJobs()
	if err != nil {
		log.Printf("warning: failed to check for pending jobs: %v", err)
		return
	}
	if len(jobs) > 0 {
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("  Resume queue (%d jobs, oldest first):\n", len(jobs))
		for _, j := range jobs {
			label := j.SourceTitle
			if label == "" {
				label = j.SourcePlaylistID
			}
			fmt.Printf("    [%s] %s -> %s (created: %s)\n", j.Status, label, j.NewName, j.CreatedAt)
		}
		fmt.Println(strings.Repeat("-", 50))
	}
	for _, j := range jobs {
		switch j.Status {
		case "pending":
			fmt.Printf("\nFound queued job: %s -> %s\n", j.SourcePlaylistID, j.NewName)
			jp := &jobProgress{Status: JobPending}
			go runJob(ctx, j.ID, jp, j.SourcePlaylistID, j.NewName)
		case "paused":
			pausedAt, parseErr := time.Parse(time.RFC3339, j.PausedAt)
			if parseErr == nil && time.Since(pausedAt) < 24*time.Hour {
				waitDuration := 24*time.Hour - time.Since(pausedAt)
				log.Printf("Job %s paused less than 24h ago (will retry in %v)", j.ID, waitDuration.Round(time.Second))
				continue
			}
			fmt.Printf("\nResuming paused job: %s -> %s (%d/%d items)\n", j.SourceTitle, j.NewName, j.InsertedItems, j.TotalItems)
			quota, err := db.GetQuota()
			if err != nil {
				log.Printf("warning: skipping resume, quota check failed: %v", err)
				continue
			}
			remainingItems := j.TotalItems - j.InsertedItems
			needed := db.EstimateQuotaNeeded(remainingItems)
			if needed > quota.Remaining {
				log.Printf("Insufficient quota to resume job %s: need %d, have %d", j.ID, needed, quota.Remaining)
				continue
			}
			jp := &jobProgress{
				Status:        JobInserting,
				Total:         j.TotalItems,
				Done:          j.InsertedItems,
				NewPlaylistID: j.NewPlaylistID,
			}
			go resumeJob(ctx, j, jp)
		case "fetching", "shuffling", "inserting":
			fmt.Printf("\nResuming interrupted job: %s -> %s (%d/%d items)\n", j.SourceTitle, j.NewName, j.InsertedItems, j.TotalItems)
			quota, err := db.GetQuota()
			if err != nil {
				log.Printf("warning: skipping resume, quota check failed: %v", err)
				continue
			}
			remainingItems := j.TotalItems - j.InsertedItems
			needed := db.EstimateQuotaNeeded(remainingItems)
			if needed > quota.Remaining {
				log.Printf("Insufficient quota to resume job %s: need %d, have %d", j.ID, needed, quota.Remaining)
				continue
			}
			jp := &jobProgress{
				Status:        JobInserting,
				Total:         j.TotalItems,
				Done:          j.InsertedItems,
				NewPlaylistID: j.NewPlaylistID,
			}
			go resumeJob(ctx, j, jp)
		}
	}
}

func jobPoller(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resumePendingJobs(ctx)
		}
	}
}
