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
	"strings"
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
	oauthStates      = make(map[string]bool)
	oauthStatesMu    sync.Mutex
	sessionStore     = make(map[string]sessionInfo)
	sessionMu        sync.RWMutex
	jobs             = make(map[string]*ShuffleJob)
	jobsMu           sync.RWMutex
	listenAddr       string
	dbPath           string
	clientSecretPath string
	baseURL          string
	secureCookies    bool
	templates        *template.Template
)

type sessionInfo struct {
	Expiry int64
	IP     string
}

func main() {
	listenAddr = getEnv("PORT", "8080")
	dbPath = getEnv("DB_PATH", "./ypr.db")
	clientSecretPath = getEnv("CLIENT_SECRET", "./client_secret.json")
	baseURL = getEnv("BASE_URL", "http://localhost:"+listenAddr)
	secureCookies = os.Getenv("DOCKER") == "true" || os.Getenv("HTTPS") == "true"

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

	r.ForwardedByClientIP = true

	r.StaticFS("/static", http.FS(mustSub(embeddedFiles, "static")))

	r.GET("/login", handleLogin)
	r.GET("/auth/callback", handleAuthCallback)
	r.GET("/logout", handleLogout)

	auth := r.Group("/")
	auth.Use(authMiddleware(), csrfMiddleware())
	{
		auth.GET("/", handleIndex)
		auth.POST("/playlist/:id/shuffle", handleShuffle)
		auth.GET("/shuffle/:jobId/status", handleShuffleStatus)
		auth.DELETE("/shuffle/:jobId", handleCancelShuffle)
		auth.GET("/quota/history", handleQuotaHistory)
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
		"pct": func(a, b int) int {
			if b <= 0 {
				return 0
			}
			return (a * 100) / b
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

func getClientIP(c *gin.Context) string {
	fwd := c.Request.Header.Get("X-Forwarded-For")
	if fwd != "" {
		return fwd
	}
	idx := strings.LastIndex(c.Request.RemoteAddr, ":")
	if idx < 0 {
		return c.Request.RemoteAddr
	}
	return c.Request.RemoteAddr[:idx]
}

func setSessionCookie(c *gin.Context, sessionID string) {
	c.SetCookie("ypr_session", sessionID, 86400, "/", "", secureCookies, true)
}

func newSession(c *gin.Context) string {
	b := make([]byte, 32)
	rand.Read(b)
	sessionID := hex.EncodeToString(b)

	ip := getClientIP(c)

	sessionMu.Lock()
	sessionStore[sessionID] = sessionInfo{Expiry: time.Now().Add(24 * time.Hour).Unix(), IP: ip}
	sessionMu.Unlock()

	return sessionID
}

func isValidSession(sessionID string, c *gin.Context) bool {
	sessionMu.RLock()
	info, ok := sessionStore[sessionID]
	sessionMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().Unix() > info.Expiry {
		sessionMu.Lock()
		delete(sessionStore, sessionID)
		sessionMu.Unlock()
		return false
	}
	if info.IP != "" && info.IP != getClientIP(c) {
		return false
	}
	return true
}

func startSessionCleaner() {
	go func() {
		for range time.Tick(10 * time.Minute) {
			sessionMu.Lock()
			now := time.Now().Unix()
			for id, info := range sessionStore {
				if now > info.Expiry {
					delete(sessionStore, id)
				}
			}
			sessionMu.Unlock()
		}
	}()

	go func() {
		for range time.Tick(5 * time.Minute) {
			jobsMu.Lock()
			for id, job := range jobs {
				if job.Status == "done" || job.Status == "error" || job.Status == "cancelled" {
					delete(jobs, id)
				}
			}
			jobsMu.Unlock()
		}
	}()
}

// -- middleware --

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie("ypr_session")
		if err != nil || sessionID == "" || !isValidSession(sessionID, c) {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		origin := c.Request.Header.Get("Origin")
		referer := c.Request.Header.Get("Referer")
		if origin == "" && referer == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "CSRF protection: missing Origin or Referer header"})
			return
		}
		c.Next()
	}
}

// -- handlers --

func handleLogin(c *gin.Context) {
	sessionID, _ := c.Cookie("ypr_session")
	if sessionID != "" && isValidSession(sessionID, c) {
		c.Redirect(http.StatusFound, "/")
		return
	}

	stateBytes := make([]byte, 32)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	oauthStatesMu.Lock()
	oauthStates[state] = true
	oauthStatesMu.Unlock()

	url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	c.Redirect(http.StatusFound, url)
}

func handleAuthCallback(c *gin.Context) {
	state := c.Query("state")
	if state == "" {
		c.String(http.StatusBadRequest, "missing oauth state")
		return
	}

	oauthStatesMu.Lock()
	pending := oauthStates[state]
	delete(oauthStates, state)
	oauthStatesMu.Unlock()

	if !pending {
		c.String(http.StatusBadRequest, "invalid or expired oauth state")
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

	sessionID := newSession(c)
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
	c.SetCookie("ypr_session", "", -1, "/", "", secureCookies, true)
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

	quotaHistory, _ := getQuotaHistory(7)

	used, limit := getTodayQuota()
	templates.ExecuteTemplate(c.Writer, "index.html", gin.H{
		"playlists":    playlists,
		"quotaUsed":    used,
		"quotaLimit":   limit,
		"hasToken":     true,
		"quotaHistory": quotaHistory,
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

	jobID := newSession(c)[:16]
	job := &ShuffleJob{
		ID:            jobID,
		PlaylistID:    playlistID,
		PlaylistTitle: req.Title,
		Status:        "pending",
		Cancel:        make(chan struct{}),
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

func handleCancelShuffle(c *gin.Context) {
	jobID := c.Param("jobId")

	jobsMu.Lock()
	job, ok := jobs[jobID]
	if !ok {
		jobsMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	if job.Status == "done" || job.Status == "error" || job.Status == "cancelled" {
		jobsMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot cancel job in state: %s", job.Status)})
		return
	}

	close(job.Cancel)
	job.Status = "cancelled"
	jobsMu.Unlock()

	log.Printf("shuffle job %s: cancelled by user", jobID)
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func handleQuotaHistory(c *gin.Context) {
	history, err := getQuotaHistory(7)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, history)
}
