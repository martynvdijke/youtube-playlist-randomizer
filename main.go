package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

//go:embed templates/* static/*
var embeddedFiles embed.FS

var (
	oauthConfig      *oauth2.Config
	oauthStateString string
	sessionStore     = make(map[string]int64)
	sessionMu        sync.RWMutex
	jobs             = make(map[string]*ShuffleJob)
	jobsMu           sync.RWMutex
	listenAddr       string
	dbPath           string
	clientSecretPath string
	baseURL          string
	templates        *template.Template
)

func main() {
	listenAddr = getEnv("PORT", "8080")
	dbPath = getEnv("DB_PATH", "./ypr.db")
	clientSecretPath = getEnv("CLIENT_SECRET", "./client_secret.json")
	baseURL = getEnv("BASE_URL", "http://localhost:"+listenAddr)

	if err := initDB(dbPath); err != nil {
		log.Fatalf("database init: %v", err)
	}
	defer db.Close()

	if err := setupOAuth(); err != nil {
		log.Fatalf("oauth setup: %v", err)
	}

	if err := setupTemplates(); err != nil {
		log.Fatalf("template setup: %v", err)
	}

	startSessionCleaner()

	if os.Getenv("DOCKER") == "true" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.MaxMultipartMemory = 8 << 20

	// static files
	r.StaticFS("/static", http.FS(mustSub(embeddedFiles, "static")))

	// public routes
	r.GET("/login", handleLogin)
	r.GET("/auth/callback", handleAuthCallback)
	r.GET("/logout", handleLogout)

	// authenticated routes
	auth := r.Group("/")
	auth.Use(authMiddleware())
	{
		auth.GET("/", handleIndex)
		auth.POST("/playlist/:id/shuffle", handleShuffle)
		auth.GET("/shuffle/:jobId/status", handleShuffleStatus)
	}

	log.Printf("listening on http://localhost:%s", listenAddr)
	if err := r.Run(":" + listenAddr); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func setupOAuth() error {
	b, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return fmt.Errorf("read client secret %s: %w (set CLIENT_SECRET env var)", clientSecretPath, err)
	}

	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/youtube.force-ssl")
	if err != nil {
		return fmt.Errorf("parse client secret: %w", err)
	}

	config.RedirectURL = baseURL + "/auth/callback"
	oauthConfig = config

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return fmt.Errorf("generate oauth state: %w", err)
	}
	oauthStateString = hex.EncodeToString(stateBytes)

	return nil
}

func setupTemplates() error {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"remaining": func() int { return remainingQuota() },
		"used":      func() int { u, _ := getTodayQuota(); return u },
		"limit":     func() int { return getDailyLimit() },
		"quotaPct": func() int {
			used, limit := getTodayQuota()
			if limit == 0 {
				return 0
			}
			return (used * 100) / limit
		},
		"estimateCost": func(count int64) int {
			if count <= 0 {
				return 0
			}
			pages := int(count/50 + 1)
			return quotaListPlaylistItems*pages + quotaInsertPlaylist + quotaInsertPlaylistItem*int(count)
		},
	}).ParseFS(embeddedFiles, "templates/*.html")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	templates = tmpl
	return nil
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func setSessionCookie(c *gin.Context, sessionID string) {
	c.SetCookie("ypr_session", sessionID, 86400, "/", "", false, true)
}

func newSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	sessionID := hex.EncodeToString(b)

	sessionMu.Lock()
	sessionStore[sessionID] = time.Now().Add(24 * time.Hour).Unix()
	sessionMu.Unlock()

	return sessionID
}

func isValidSession(sessionID string) bool {
	sessionMu.RLock()
	exp, ok := sessionStore[sessionID]
	sessionMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().Unix() > exp {
		sessionMu.Lock()
		delete(sessionStore, sessionID)
		sessionMu.Unlock()
		return false
	}
	return true
}

func startSessionCleaner() {
	go func() {
		for range time.Tick(10 * time.Minute) {
			sessionMu.Lock()
			now := time.Now().Unix()
			for id, exp := range sessionStore {
				if now > exp {
					delete(sessionStore, id)
				}
			}
			sessionMu.Unlock()
		}
	}()
}

// -- middleware --

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie("ypr_session")
		if err != nil || sessionID == "" || !isValidSession(sessionID) {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

// -- handlers --

func handleLogin(c *gin.Context) {
	// if already authenticated, redirect to index
	sessionID, _ := c.Cookie("ypr_session")
	if sessionID != "" && isValidSession(sessionID) {
		c.Redirect(http.StatusFound, "/")
		return
	}

	// regenerate state for each login attempt
	stateBytes := make([]byte, 32)
	rand.Read(stateBytes)
	oauthStateString = hex.EncodeToString(stateBytes)

	url := oauthConfig.AuthCodeURL(oauthStateString, oauth2.AccessTypeOffline)
	c.Redirect(http.StatusFound, url)
}

func handleAuthCallback(c *gin.Context) {
	state := c.Query("state")
	if state != oauthStateString {
		c.String(http.StatusBadRequest, "invalid oauth state")
		return
	}

	code := c.Query("code")
	if code == "" {
		c.String(http.StatusBadRequest, "missing auth code")
		return
	}

	token, err := oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		c.String(http.StatusInternalServerError, "token exchange failed: %v", err)
		return
	}

	if err := saveToken(token); err != nil {
		c.String(http.StatusInternalServerError, "save token failed: %v", err)
		return
	}

	sessionID := newSession()
	setSessionCookie(c, sessionID)

	c.Redirect(http.StatusFound, "/")
}

func handleLogout(c *gin.Context) {
	sessionID, _ := c.Cookie("ypr_session")
	if sessionID != "" {
		sessionMu.Lock()
		delete(sessionStore, sessionID)
		sessionMu.Unlock()
	}
	c.SetCookie("ypr_session", "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login")
}

func handleIndex(c *gin.Context) {
	token, err := loadToken()
	if err != nil || token == nil {
		c.Redirect(http.StatusFound, "/login")
		return
	}

	ctx := context.Background()
	ts := getTokenSource(ctx, token)
	svc, err := getYouTubeService(ctx, token, ts)
	if err != nil {
		templates.ExecuteTemplate(c.Writer, "error.html", gin.H{"error": fmt.Sprintf("YouTube service: %v", err)})
		return
	}

	playlists, err := fetchPlaylists(svc)
	if err != nil {
		templates.ExecuteTemplate(c.Writer, "error.html", gin.H{"error": err.Error()})
		return
	}

	used, limit := getTodayQuota()
	templates.ExecuteTemplate(c.Writer, "index.html", gin.H{
		"playlists":  playlists,
		"quotaUsed":  used,
		"quotaLimit": limit,
		"hasToken":   true,
	})
}

func handleShuffle(c *gin.Context) {
	playlistID := c.Param("id")
	if playlistID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing playlist id"})
		return
	}

	var req struct {
		Title string `json:"title"`
	}
	c.ShouldBindJSON(&req)

	token, err := loadToken()
	if err != nil || token == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	ctx := context.Background()
	ts := getTokenSource(ctx, token)
	svc, err := getYouTubeService(ctx, token, ts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Title == "" {
		req.Title = "Randomized Playlist"
	}

	jobID := newSession()[:16]
	job := &ShuffleJob{
		ID:            jobID,
		PlaylistID:    playlistID,
		PlaylistTitle: req.Title,
		Status:        "pending",
	}

	jobsMu.Lock()
	jobs[jobID] = job
	jobsMu.Unlock()

	go runShuffleJob(job, svc)

	c.JSON(http.StatusOK, gin.H{"jobId": jobID, "status": "pending"})
}

func handleShuffleStatus(c *gin.Context) {
	jobID := c.Param("jobId")

	jobsMu.RLock()
	job, ok := jobs[jobID]
	jobsMu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":           job.Status,
		"progress":         job.Progress,
		"total":            job.Total,
		"done":             job.Done,
		"newPlaylistID":    job.NewPlaylistID,
		"newPlaylistTitle": job.NewPlaylistTitle,
		"error":            job.Error,
		"quotaUsed":        job.QuotaUsed,
		"quotaEstimated":   job.QuotaEstimated,
	})
}
