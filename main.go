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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/models"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
)

const version = "0.5.2"

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobFetching  JobStatus = "fetching"
	JobShuffling JobStatus = "shuffling"
	JobInserting JobStatus = "inserting"
	JobDone      JobStatus = "done"
	JobError     JobStatus = "error"
)

type Job struct {
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

var (
	ytClient *youtube.Client
	jobs     = make(map[string]*Job)
	jobsMu   sync.Mutex
)

func main() {
	port := flag.Int("p", 6270, "Port to listen on")
	input := flag.String("i", "client_secret.json", "Client secret JSON file")
	showVersion := flag.Bool("version", false, "Print version")

	flag.Parse()

	if *showVersion {
		fmt.Printf("youtube-playlist-randomizer version %s\n", version)
		os.Exit(0)
	}

	ctx := context.Background()

	var err error
	ytClient, err = youtube.NewClient(ctx, *input)
	if err != nil {
		log.Fatalf("Failed to create YouTube client: %v", err)
	}

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

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

	log.Printf("YouTube Playlist Randomizer web UI started at http://localhost:%d", *port)
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

func handlePlaylists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	playlists, err := ytClient.GetPlaylists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	job := &Job{Status: JobPending}

	jobsMu.Lock()
	jobs[jobID] = job
	jobsMu.Unlock()

	go runJob(context.Background(), jobID, job, req.PlaylistID, req.NewName)

	writeJSON(w, http.StatusAccepted, JobResponse{JobID: jobID, Status: job.Status})
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

	jobsMu.Lock()
	job, exists := jobs[jobID]
	jobsMu.Unlock()

	if !exists {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	job.mu.RLock()
	defer job.mu.RUnlock()

	writeJSON(w, http.StatusOK, job)
}

func runJob(ctx context.Context, jobID string, job *Job, playlistID, newName string) {
	setJobStatus := func(s JobStatus) {
		job.mu.Lock()
		job.Status = s
		job.mu.Unlock()
	}

	setJobError := func(errMsg string) {
		job.mu.Lock()
		job.Status = JobError
		job.Error = errMsg
		job.mu.Unlock()
	}

	setJobStatus(JobFetching)

	items, err := ytClient.GetPlaylistItems(ctx, playlistID)
	if err != nil {
		setJobError(fmt.Sprintf("Failed to fetch playlist items: %v", err))
		return
	}

	if len(items) == 0 {
		setJobError("Playlist has no items")
		return
	}

	setJobStatus(JobShuffling)

	shuffled := make([]models.PlayListItem, len(items))
	copy(shuffled, items)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	newPlaylistID, err := ytClient.CreatePlaylist(ctx, newName)
	if err != nil {
		setJobError(fmt.Sprintf("Failed to create playlist: %v", err))
		return
	}

	job.mu.Lock()
	job.NewPlaylistID = newPlaylistID
	job.Total = len(shuffled)
	job.mu.Unlock()

	setJobStatus(JobInserting)

	for i, item := range shuffled {
		if err := ytClient.InsertPlaylistItem(ctx, newPlaylistID, item.VideoID, int64(i)); err != nil {
			log.Printf("warning: failed to insert item %s (video %s): %v", item.ID, item.VideoID, err)
			continue
		}
		time.Sleep(10 * time.Millisecond)

		job.mu.Lock()
		job.Done = i + 1
		job.mu.Unlock()

		if (i+1)%190 == 0 {
			log.Printf("reached %d items, sleeping for 25h...", i+1)
			time.Sleep(25 * time.Hour)
		}
	}

	log.Printf("successfully inserted %d items into playlist %s", len(shuffled), newPlaylistID)

	job.mu.Lock()
	job.Status = JobDone
	job.mu.Unlock()
}
