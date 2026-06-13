// Package job provides the playlist shuffle job runner, handling the full
// lifecycle of fetching, shuffling, and inserting YouTube playlist items.
package job

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/gotify"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/telemetry"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Status represents the current state of a shuffle job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusFetching  Status = "fetching"
	StatusShuffling Status = "shuffling"
	StatusInserting Status = "inserting"
	StatusDone      Status = "done"
	StatusError     Status = "error"
	StatusPaused    Status = "paused"
)

// Progress holds the live progress of a running shuffle job, accessible
// concurrently by the runner goroutine and HTTP polling handlers.
type Progress struct {
	mu            sync.RWMutex
	Status        Status `json:"status"`
	Progress      int    `json:"progress"`
	Total         int    `json:"total"`
	Done          int    `json:"done"`
	NewPlaylistID string `json:"newPlaylistId,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Snapshot returns a consistent read-only copy of the progress.
func (p *Progress) Snapshot() Progress {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return Progress{
		Status:        p.Status,
		Progress:      p.Progress,
		Total:         p.Total,
		Done:          p.Done,
		NewPlaylistID: p.NewPlaylistID,
		Error:         p.Error,
	}
}

// Runner executes YouTube playlist shuffle jobs with quota-aware pausing
// and automatic resumption.
type Runner struct {
	store        *store.Store
	logger       *logging.Logger
	ytClient     *youtube.Client
	otel         *telemetry.Telemetry
	gotifyClient *gotify.Client
}

// New creates a new Runner.
func New(s *store.Store, l *logging.Logger, yt *youtube.Client, ot *telemetry.Telemetry, gc *gotify.Client) *Runner {
	return &Runner{
		store:        s,
		logger:       l,
		ytClient:     yt,
		otel:         ot,
		gotifyClient: gc,
	}
}

// Run starts a new playlist shuffle job in a background goroutine and
// returns a Progress object for immediate polling.
func (r *Runner) Run(ctx context.Context, jobID, playlistID, newName string) *Progress {
	jp := &Progress{Status: StatusPending}
	if r.ytClient == nil {
		jp.mu.Lock()
		jp.Status = StatusError
		jp.Error = "YouTube API not available"
		jp.mu.Unlock()
		r.store.SetJobError(jobID, "YouTube API not available")
		return jp
	}
	go r.run(ctx, jobID, playlistID, newName, jp)
	return jp
}

// Resume resumes a previously paused/interrupted job in a background
// goroutine and returns a Progress object for immediate polling.
func (r *Runner) Resume(ctx context.Context, j store.Job) *Progress {
	jp := &Progress{
		Status:        StatusInserting,
		Total:         j.TotalItems,
		Done:          j.InsertedItems,
		NewPlaylistID: j.NewPlaylistID,
	}
	go r.resume(ctx, j, jp)
	return jp
}

// run is the full shuffle job life cycle, run in a goroutine.
func (r *Runner) run(ctx context.Context, jobID, playlistID, newName string, jp *Progress) {
	var span trace.Span
	if r.otel != nil {
		ctx, span = r.otel.Tracer.Start(ctx, "runJob",
			trace.WithAttributes(
				attribute.String("job.id", jobID),
				attribute.String("playlist.id", playlistID),
				attribute.String("playlist.name", newName),
			),
		)
		defer span.End()
	}

	updateStatus := func(s Status) {
		jp.mu.Lock()
		jp.Status = s
		jp.mu.Unlock()
		r.store.UpdateJobStatus(jobID, string(s))
	}

	setError := func(errMsg string) {
		jp.mu.Lock()
		jp.Status = StatusError
		jp.Error = errMsg
		jp.mu.Unlock()
		r.store.SetJobError(jobID, errMsg)
		if span != nil {
			span.SetStatus(codes.Error, errMsg)
			span.RecordError(fmt.Errorf("%s", errMsg))
		}
		if r.otel != nil {
			r.otel.RecordJobFailed(context.Background(), errMsg)
		}
		r.sendNotification("❌ Shuffle Failed", fmt.Sprintf("Playlist %q: %s", newName, errMsg))
	}

	progress := func(done, total int, newPlaylistID string) {
		jp.mu.Lock()
		jp.Done = done
		jp.Total = total
		if newPlaylistID != "" {
			jp.NewPlaylistID = newPlaylistID
		}
		jp.mu.Unlock()
		r.store.UpdateJobProgress(jobID, done, newPlaylistID)
	}

	// --- Phase 1: Quota check ---
	quota, err := r.store.GetQuota()
	if err != nil {
		r.logger.Warnc(ctx, fmt.Sprintf("quota check failed for job %s", jobID), "error", err.Error())
	}
	if quota == nil || quota.Remaining < store.QuotaListPlaylistItems {
		r.logger.Warnc(ctx, fmt.Sprintf("Insufficient quota to fetch items for job %s (remaining: %d). Job will wait.", jobID, quota.Remaining))
		updateStatus(StatusPending)
		r.sendNotification("⏳ Shuffle Queued", fmt.Sprintf("Playlist %q queued — waiting for API quota to become available.", newName))
		return
	}

	// --- Phase 2: Fetch items ---
	updateStatus(StatusFetching)

	items, err := r.ytClient.GetPlaylistItems(ctx, playlistID)
	if err != nil {
		if youtube.IsQuotaError(err) {
			r.logger.Warnc(ctx, fmt.Sprintf("Quota error fetching items for job %s. Pausing.", jobID))
			updateStatus(StatusPaused)
			r.store.SetJobPaused(jobID)
			r.sendNotification("⏸ Shuffle Paused", fmt.Sprintf("Playlist %q paused — API quota exhausted while fetching items.", newName))
			if r.otel != nil {
				r.otel.RecordJobPaused(context.Background(), 0, 0)
			}
			return
		}
		setError(fmt.Sprintf("Failed to fetch playlist items: %v", err))
		return
	}
	if _, err := r.store.AddQuota(store.QuotaListPlaylistItems); err != nil {
		r.logger.Warnc(ctx, "failed to track quota", "error", err.Error())
	}

	if len(items) == 0 {
		setError("Playlist has no items")
		return
	}

	// --- Phase 3: Shuffle ---
	updateStatus(StatusShuffling)

	shuffled := make([]string, len(items))
	for i, item := range items {
		shuffled[i] = item.VideoID
	}
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	if err := r.store.SaveShuffledItems(jobID, shuffled); err != nil {
		setError(fmt.Sprintf("Failed to save shuffled items: %v", err))
		return
	}

	// --- Phase 4: Create new playlist ---
	newPlaylistID, err := r.ytClient.CreatePlaylist(ctx, newName)
	if err != nil {
		if youtube.IsQuotaError(err) {
			r.logger.Warnc(ctx, fmt.Sprintf("Quota error creating playlist for job %s. Pausing.", jobID))
			updateStatus(StatusPaused)
			r.store.SetJobPaused(jobID)
			r.sendNotification("⏸ Shuffle Paused", fmt.Sprintf("Playlist %q paused — API quota exhausted while creating playlist.", newName))
			if r.otel != nil {
				r.otel.RecordJobPaused(context.Background(), 0, jp.Total)
			}
			return
		}
		setError(fmt.Sprintf("Failed to create playlist: %v", err))
		return
	}
	if _, err := r.store.AddQuota(store.QuotaCreatePlaylist); err != nil {
		r.logger.Warnc(ctx, "failed to track quota", "error", err.Error())
	}

	progress(0, len(shuffled), newPlaylistID)
	updateStatus(StatusInserting)

	// --- Phase 5: Insert items ---
	r.insertItems(ctx, jobID, newPlaylistID, jp, updateStatus, setError, progress)

	// If insertItems paused or errored, don't set done.
	jp.mu.RLock()
	finalStatus := jp.Status
	jp.mu.RUnlock()
	if finalStatus == StatusPaused || finalStatus == StatusError {
		return
	}

	// --- Phase 6: Complete ---
	r.logger.Infoc(ctx, fmt.Sprintf("Successfully inserted %d items into playlist %s", jp.Total, newPlaylistID))
	progress(jp.Total, jp.Total, "")
	updateStatus(StatusDone)
	r.store.SetJobDone(jobID)
	playlistURL := fmt.Sprintf("https://www.youtube.com/playlist?list=%s", newPlaylistID)
	r.sendNotification("✅ Shuffle Complete", fmt.Sprintf("Playlist %q randomized with %d items.\n%s", newName, jp.Total, playlistURL))
	if span != nil {
		span.SetAttributes(attribute.Int("items.total", jp.Total))
		span.SetStatus(codes.Ok, "")
	}
	if r.otel != nil {
		r.otel.RecordJobCompleted(context.Background(), jp.Total)
	}
}

// resume resumes an interrupted job, run in a goroutine.
func (r *Runner) resume(ctx context.Context, j store.Job, jp *Progress) {
	var span trace.Span
	if r.otel != nil {
		ctx, span = r.otel.Tracer.Start(ctx, "resumeJob",
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

	updateStatus := func(s Status) {
		jp.mu.Lock()
		jp.Status = s
		jp.mu.Unlock()
		r.store.UpdateJobStatus(j.ID, string(s))
	}

	setError := func(errMsg string) {
		jp.mu.Lock()
		jp.Status = StatusError
		jp.Error = errMsg
		jp.mu.Unlock()
		r.store.SetJobError(j.ID, errMsg)
		if span != nil {
			span.SetStatus(codes.Error, errMsg)
			span.RecordError(fmt.Errorf("%s", errMsg))
		}
	}

	progress := func(done, total int, newPlaylistID string) {
		jp.mu.Lock()
		jp.Done = done
		jp.Total = total
		if newPlaylistID != "" {
			jp.NewPlaylistID = newPlaylistID
		}
		jp.mu.Unlock()
		r.store.UpdateJobProgress(j.ID, done, jp.NewPlaylistID)
	}

	// Create playlist if it doesn't exist yet
	newPlaylistID := j.NewPlaylistID
	if newPlaylistID == "" {
		id, err := r.ytClient.CreatePlaylist(ctx, j.NewName)
		if err != nil {
			if youtube.IsQuotaError(err) {
				r.logger.Warnc(ctx, fmt.Sprintf("Quota error creating playlist during resume for job %s. Pausing.", j.ID))
				r.store.SetJobPaused(j.ID)
				return
			}
			setError(fmt.Sprintf("Failed to create playlist on resume: %v", err))
			return
		}
		if _, qErr := r.store.AddQuota(store.QuotaCreatePlaylist); qErr != nil {
			r.logger.Warnc(ctx, "failed to track quota", "error", qErr.Error())
		}
		r.store.UpdateJobNewPlaylistID(j.ID, id)
		newPlaylistID = id
		jp.mu.Lock()
		jp.NewPlaylistID = newPlaylistID
		jp.mu.Unlock()
	}

	updateStatus(StatusInserting)

	r.insertItems(ctx, j.ID, newPlaylistID, jp, updateStatus, setError, progress)

	jp.mu.RLock()
	finalStatus := jp.Status
	jp.mu.RUnlock()
	if finalStatus == StatusPaused || finalStatus == StatusError {
		return
	}

	r.logger.Infoc(ctx, fmt.Sprintf("Resume complete: inserted %d items into playlist %s", jp.Total, newPlaylistID))
	progress(jp.Total, jp.Total, "")
	updateStatus(StatusDone)
	r.store.SetJobDone(j.ID)
	if span != nil {
		span.SetAttributes(attribute.Int("items.total", jp.Total))
		span.SetStatus(codes.Ok, "")
	}
}

// insertItems inserts uninserted items into the target playlist. It is the
// shared core loop used by both run and resume.
func (r *Runner) insertItems(
	ctx context.Context,
	jobID, newPlaylistID string,
	jp *Progress,
	updateStatus func(Status),
	setError func(string),
	progress func(done, total int, newPlaylistID string),
) {
	uninserted, err := r.store.GetUninsertedItems(jobID)
	if err != nil {
		setError(fmt.Sprintf("Failed to get uninserted items: %v", err))
		return
	}

	for _, item := range uninserted {
		quota, err := r.store.GetQuota()
		if err != nil {
			setError(fmt.Sprintf("Failed to check quota: %v", err))
			return
		}
		if quota.Remaining < store.QuotaInsertItem {
			r.logger.Warnc(ctx, fmt.Sprintf("Quota exhausted at %d/%d. Job %s paused.", jp.Done, jp.Total, jobID))
			updateStatus(StatusPaused)
			r.store.SetJobPaused(jobID)
			if r.otel != nil {
				r.otel.RecordJobPaused(context.Background(), jp.Done, jp.Total)
			}
			return
		}

		if err := r.ytClient.InsertPlaylistItem(ctx, newPlaylistID, item.VideoID, int64(item.Position)); err != nil {
			if youtube.IsQuotaError(err) {
				r.logger.Warnc(ctx, fmt.Sprintf("Quota error during insert at %d/%d. Job %s paused.", jp.Done, jp.Total, jobID))
				updateStatus(StatusPaused)
				r.store.SetJobPaused(jobID)
				if r.otel != nil {
					r.otel.RecordJobPaused(context.Background(), jp.Done, jp.Total)
				}
				return
			}
			r.logger.Warnc(ctx, fmt.Sprintf("failed to insert item at position %d (video %s)", item.Position, item.VideoID), "error", err.Error())
			continue
		}

		if _, qErr := r.store.AddQuota(store.QuotaInsertItem); qErr != nil {
			r.logger.Warnc(ctx, "failed to track quota", "error", qErr.Error())
		}
		if err := r.store.MarkItemInserted(jobID, item.Position); err != nil {
			r.logger.Warnc(ctx, "failed to mark item inserted", "error", err.Error())
		}

		done := jp.Done + 1
		progress(done, jp.Total, "")

		time.Sleep(100 * time.Millisecond)

		if done%50 == 0 {
			r.logger.Infoc(ctx, fmt.Sprintf("Inserted %d/%d items for job %s", done, jp.Total, jobID))
			time.Sleep(1 * time.Second)
		}
	}
}

// ResumePending scans for pending/paused/interrupted jobs and resumes them
// if quota permits. Called at startup and periodically by Poller.
func (r *Runner) ResumePending(ctx context.Context) {
	jobs, err := r.store.GetPendingJobs()
	if err != nil {
		r.logger.Warnc(ctx, "failed to check for pending jobs", "error", err.Error())
		return
	}

	for _, j := range jobs {
		switch j.Status {
		case "pending":
			r.logger.Infoc(ctx, fmt.Sprintf("Found queued job: %s -> %s", j.SourcePlaylistID, j.NewName))
			r.Run(ctx, j.ID, j.SourcePlaylistID, j.NewName)
		case "paused":
			pausedAt, parseErr := time.Parse(time.RFC3339, j.PausedAt)
			if parseErr == nil && time.Since(pausedAt) < 24*time.Hour {
				waitDuration := 24*time.Hour - time.Since(pausedAt)
				r.logger.Infoc(ctx, fmt.Sprintf("Job %s paused less than 24h ago (will retry in %v)", j.ID, waitDuration.Round(time.Second)))
				continue
			}
			r.logger.Infoc(ctx, fmt.Sprintf("Resuming paused job: %s -> %s (%d/%d items)", j.SourceTitle, j.NewName, j.InsertedItems, j.TotalItems))
			quota, err := r.store.GetQuota()
			if err != nil {
				r.logger.Warnc(ctx, "skipping resume, quota check failed", "error", err.Error())
				continue
			}
			remainingItems := j.TotalItems - j.InsertedItems
			needed := r.store.EstimateQuotaNeeded(remainingItems)
			if needed > quota.Remaining {
				r.logger.Warnc(ctx, fmt.Sprintf("Insufficient quota to resume job %s: need %d, have %d", j.ID, needed, quota.Remaining))
				continue
			}
			r.Resume(ctx, j)
		case "fetching", "shuffling", "inserting":
			r.logger.Infoc(ctx, fmt.Sprintf("Resuming interrupted job: %s -> %s (%d/%d items)", j.SourceTitle, j.NewName, j.InsertedItems, j.TotalItems))
			quota, err := r.store.GetQuota()
			if err != nil {
				r.logger.Warnc(ctx, "skipping resume, quota check failed", "error", err.Error())
				continue
			}
			remainingItems := j.TotalItems - j.InsertedItems
			needed := r.store.EstimateQuotaNeeded(remainingItems)
			if needed > quota.Remaining {
				r.logger.Warnc(ctx, fmt.Sprintf("Insufficient quota to resume job %s: need %d, have %d", j.ID, needed, quota.Remaining))
				continue
			}
			r.Resume(ctx, j)
		}
	}
}

// Poller periodically checks for resumable jobs. Runs in a background
// goroutine.
func (r *Runner) Poller(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.ResumePending(ctx)
		}
	}
}

// GetProgress reads job progress from the database and returns a Progress
// snapshot. Returns nil if the job is not found.
func (r *Runner) GetProgress(jobID string) *Progress {
	j, err := r.store.GetJob(jobID)
	if err != nil {
		return nil
	}
	return &Progress{
		Status:        Status(j.Status),
		Total:         j.TotalItems,
		Done:          j.InsertedItems,
		NewPlaylistID: j.NewPlaylistID,
		Error:         j.Error,
	}
}

func (r *Runner) sendNotification(title, message string) {
	if r.gotifyClient == nil {
		return
	}
	if err := r.gotifyClient.Send(title, message); err != nil {
		r.logger.Warnc(context.Background(), "failed to send Gotify notification", "error", err.Error())
	} else {
		r.logger.Infoc(context.Background(), "Gotify notification sent", "title", title)
	}
}
