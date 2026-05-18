package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
)

const version = "1.0.0"

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
	ytClient *youtube.Client
	db       *store.Store
	dataDir  string
	jobsMu   sync.Mutex
)

func findClientSecret() string {
	paths := []string{"client_secret.json", "/app/client_secret.json"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "client_secret.json"
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
	dataDirFlag := flag.String("d", ".", "Data directory for DB and cached token")
	mockMode := flag.Bool("mock", false, "Run in mock mode (no YouTube API credentials needed)")

	flag.Parse()

	if *showVersion {
		fmt.Printf("youtube-playlist-randomizer version %s\n", version)
		os.Exit(0)
	}

	dataDir = *dataDirFlag

	dbPath := filepath.Join(dataDir, "ypr.db")
	var err error
	db, err = store.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	if *mockMode {
		log.Println("Running in mock mode (no YouTube API)")
		ytClient = nil
	} else {
		secretPath := *input
		if secretPath == "" {
			secretPath = findClientSecret()
		}

		ytClient, err = youtube.NewClient(ctx, secretPath, dataDir)
		if err != nil {
			log.Fatalf("Failed to create YouTube client: %v", err)
		}

		if _, err := db.AddQuota(store.QuotaListPlaylists); err != nil {
			log.Printf("warning: failed to track quota: %v", err)
		}
	}

	quota, err := db.GetQuota()
	if err != nil {
		log.Fatalf("Failed to get quota: %v", err)
	}
	printQuotaBanner(quota)

	if !*mockMode {
		pausedJobs, err := db.GetPendingJobs()
		if err != nil {
			log.Printf("warning: failed to check for pending jobs: %v", err)
		}

		for _, j := range pausedJobs {
			fmt.Printf("\nFound paused job: %s -> %s (%d/%d items)\n",
				j.SourceTitle, j.NewName, j.InsertedItems, j.TotalItems)

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

			fmt.Printf("Resuming job %s (%d items remaining, ~%d quota needed, %d remaining)...\n",
				j.ID, remainingItems, needed, quota.Remaining)

			jobsMu.Lock()
			jp := &jobProgress{
				Status:        JobInserting,
				Total:         j.TotalItems,
				Done:          j.InsertedItems,
				NewPlaylistID: j.NewPlaylistID,
			}
			jobsMu.Unlock()

			go resumeJob(ctx, j, jp)
		}
	}

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/api/quota", handleQuota)
	mux.HandleFunc("/api/playlists", handlePlaylists)
	mux.HandleFunc("/api/randomize", handleRandomize)
	mux.HandleFunc("/api/jobs/", handleJobStatus)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "static/index.html")
	})

	addr := fmt.Sprintf(":%d", *port)
	server := &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(mux),
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

	quota, err := db.GetQuota()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if quota.Remaining < store.QuotaCreatePlaylist+store.QuotaListPlaylistItems+store.QuotaInsertItem {
		writeError(w, http.StatusTooManyRequests,
			fmt.Sprintf("Insufficient API quota remaining (%d). Wait for quota reset.", quota.Remaining))
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
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	jobID := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "Missing job ID")
		return
	}

	jp := getJobProgress(jobID)
	if jp == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	writeJSON(w, http.StatusOK, jp)
}

func runJob(ctx context.Context, jobID string, jp *jobProgress, playlistID, newName string) {
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

	updateStatus(JobFetching)

	items, err := ytClient.GetPlaylistItems(ctx, playlistID)
	if err != nil {
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
			return
		}

		if err := ytClient.InsertPlaylistItem(ctx, newPlaylistID, item.VideoID, int64(item.Position)); err != nil {
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
}

func resumeJob(ctx context.Context, j store.Job, jp *jobProgress) {
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
			return
		}

		if err := ytClient.InsertPlaylistItem(ctx, newPlaylistID, item.VideoID, int64(item.Position)); err != nil {
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
}
