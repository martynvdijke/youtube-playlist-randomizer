package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/admin"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/gotify"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/handlers"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/job"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/telemetry"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
)

const version = "1.12.0"

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

func printQuotaBanner(q *store.QuotaInfo, dataDir string) {
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

	dataDir := *dataDirFlag

	if *reauth {
		tokenPath := filepath.Join(dataDir, "token.json")
		if err := os.Remove(tokenPath); err == nil {
			fmt.Printf("Deleted cached token at %s\n", tokenPath)
		} else if !os.IsNotExist(err) {
			fmt.Printf("Warning: could not delete token: %v\n", err)
		}
	}

	// --- Database ---
	dbPath := filepath.Join(dataDir, "ypr.db")
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// --- Structured logger ---
	defaultLevel := logging.WARN
	levelStr, _ := db.GetSetting("log_level")
	if levelStr != "" {
		defaultLevel = logging.ParseSeverity(levelStr)
	}
	logger := logging.New(logging.LogOptions{
		Store:    db,
		MinLevel: defaultLevel,
		Service:  "ypr",
	})
	adminHandlers := admin.New(db, logger)
	logger.Infoc(context.Background(), "Application started", "version", version)

	// --- Telemetry (OpenTelemetry) ---
	otelCfg := telemetry.DefaultSettings()
	if endp, _ := db.GetSetting("otel_endpoint"); endp != "" {
		otelCfg.Endpoint = endp
	}
	if te, _ := db.GetSetting("otel_traces_enabled"); te == "false" {
		otelCfg.TracesEnabled = false
	}
	if me, _ := db.GetSetting("otel_metrics_enabled"); me == "false" {
		otelCfg.MetricsEnabled = false
	}
	if sr, _ := db.GetSetting("otel_trace_sample_rate"); sr != "" {
		if r, err := strconv.ParseFloat(sr, 64); err == nil {
			otelCfg.TraceSampleRate = r
		}
	}
	if hdrs, _ := db.GetSetting("otel_headers"); hdrs != "" {
		otelCfg.Headers = telemetry.ParseHeadersJSON(hdrs)
	}
	otel, err := telemetry.New(otelCfg)
	if err != nil {
		logger.Warnc(context.Background(), "failed to initialize telemetry", "error", err.Error())
		otel = nil
	}
	if otel != nil {
		defer otel.Shutdown(context.Background())
	}

	// --- Gotify notifications ---
	gURL, _ := db.GetSetting("gotify_url")
	gToken, _ := db.GetSetting("gotify_token")
	gEnabled, _ := db.GetSetting("gotify_enabled")
	gotifyClient := gotify.New(gURL, gToken, gEnabled == "true")

	// --- YouTube API client ---
	ctx := context.Background()
	var ytClient *youtube.Client
	var oauthSetup *youtube.OAuthSetup
	var clientSecretPath string

	if *mockMode {
		logger.Infoc(ctx, "Running in mock mode (no YouTube API)")
	} else {
		secretPath := *input
		if secretPath == "" {
			secretPath = findClientSecret()
		}
		clientSecretPath = secretPath
		oauthSetup, err = youtube.LoadConfig(secretPath)
		if err != nil {
			logger.Warnc(ctx, "failed to load OAuth config", "error", err.Error())
		}

		ytClient, err = youtube.NewClient(ctx, secretPath, dataDir, otel, logger)
		if err != nil {
			if err == youtube.ErrNoToken {
				logger.Infoc(ctx, "No cached OAuth token found. YouTube API not available until user authenticates via the web UI.")
			} else {
				logger.Warnc(ctx, "failed to create YouTube client", "error", err.Error())
				logger.Infoc(ctx, "Server will start without YouTube API access. Re-authenticate to restore functionality.")
			}
			ytClient = nil
		}

		if ytClient != nil {
			if _, err := db.AddQuota(store.QuotaListPlaylists); err != nil {
				logger.Warnc(ctx, "failed to track quota", "error", err.Error())
			}
		}
	}

	quota, err := db.GetQuota()
	if err != nil {
		log.Fatalf("Failed to get quota: %v", err)
	}
	printQuotaBanner(quota, dataDir)

	// --- Job runner ---
	jobRunner := job.New(db, logger, ytClient, otel, gotifyClient)

	if !*mockMode {
		jobRunner.ResumePending(ctx)
		go jobRunner.Poller(ctx)
	}

	// --- HTTP handlers ---
	h := handlers.New(&handlers.Config{
		Store:            db,
		Logger:           logger,
		YTClient:         ytClient,
		OTel:             otel,
		OAuthSetup:       oauthSetup,
		GotifyClient:     gotifyClient,
		AdminHandlers:    adminHandlers,
		JobRunner:        jobRunner,
		ClientSecretPath: clientSecretPath,
		DataDir:          dataDir,
		Version:          version,
	})

	// --- HTTP server ---
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	var handler http.Handler = h.CORSMiddleware(mux)
	if otel != nil {
		handler = otel.Middleware(handler)
	}

	addr := fmt.Sprintf(":%d", *port)
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
