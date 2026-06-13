package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/job"
)

func (h *Handlers) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
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

	jp := h.jobRunner.GetProgress(jobID)
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

func (h *Handlers) handleForceResume(w http.ResponseWriter, r *http.Request) {
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

	j, err := h.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	ctx := r.Context()
	if h.ytClient == nil {
		writeError(w, http.StatusBadRequest, "YouTube API not available")
		return
	}

	w.Header().Set("Content-Type", "text/html")

	switch j.Status {
	case "pending":
		jp := h.jobRunner.Run(ctx, j.ID, j.NewName, j.SourcePlaylistIDs...)
		tmpl.ExecuteTemplate(w, "forceResumePending", ForceResumeData{JobID: jobID})
		_ = jp

	case "paused", "fetching", "shuffling", "inserting":
		jp := h.jobRunner.Resume(ctx, *j)
		tmpl.ExecuteTemplate(w, "forceResumeContinue", ForceResumeData{JobID: jobID})
		_ = jp

	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Job is in state %s and cannot be resumed", j.Status))
	}
}

func (h *Handlers) handleJobQueueHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs, err := h.store.GetAllJobs(50)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if len(jobs) == 0 {
		tmpl.ExecuteTemplate(w, "noJobs", nil)
		return
	}

	rows := make([]JobQueueRowData, 0, len(jobs))
	for _, j := range jobs {
		label := j.SourceTitle
		if label == "" {
			label = j.SourcePlaylistID
		}

		progress := "-"
		if j.TotalItems > 0 {
			progress = fmt.Sprintf("%d / %d", j.InsertedItems, j.TotalItems)
		}

		created := j.CreatedAt
		if len(created) > 19 {
			created = created[:19]
		}
		created = strings.Replace(created, "T", " ", 1)

		var actionHTML template.HTML
		var undoHTML template.HTML
		var archiveHTML template.HTML
		vals := template.HTML(fmt.Sprintf(`hx-vals='{"jobId":%q}'`, j.ID))
		switch j.Status {
		case "paused", "pending", "fetching", "shuffling", "inserting":
			actionHTML = template.HTML(fmt.Sprintf(
				`<button class="btn btn-warning btn-sm" hx-post="/api/jobs/resume" %s hx-target="closest tr" hx-swap="outerHTML" hx-confirm="Resume this job now?">Resume Now</button>`,
				vals))
		case "done":
			actionHTML = `<span class="status-done">Done</span>`
			if j.NewPlaylistID != "" {
				undoHTML = template.HTML(fmt.Sprintf(
					`<button class="btn btn-danger btn-sm" hx-post="/api/jobs/undo" %s hx-confirm="Delete the created playlist and undo this randomization?" hx-target="#undo-result" hx-swap="innerHTML">Undo</button>`,
					vals))
			}
			archiveHTML = template.HTML(fmt.Sprintf(
				`<button class="btn btn-ghost btn-sm" hx-post="/api/jobs/archive" %s hx-target="closest tr" hx-swap="outerHTML" hx-confirm="Archive this job?">Archive</button>`,
				vals))
		case "error":
			actionHTML = `<span class="status-error">Error</span>`
			archiveHTML = template.HTML(fmt.Sprintf(
				`<button class="btn btn-ghost btn-sm" hx-post="/api/jobs/archive" %s hx-target="closest tr" hx-swap="outerHTML" hx-confirm="Archive this job?">Archive</button>`,
				vals))
		case "undone":
			actionHTML = `<span class="status-undone">Undone</span>`
			archiveHTML = template.HTML(fmt.Sprintf(
				`<button class="btn btn-ghost btn-sm" hx-post="/api/jobs/archive" %s hx-target="closest tr" hx-swap="outerHTML" hx-confirm="Archive this job?">Archive</button>`,
				vals))
		}

		rows = append(rows, JobQueueRowData{
			Status:      j.Status,
			StatusClass: j.Status,
			Title:       label,
			NewName:     j.NewName,
			Progress:    progress,
			Created:     created,
			ActionHTML:  actionHTML,
			UndoHTML:    undoHTML,
			ArchiveHTML: archiveHTML,
		})
	}

	tmpl.ExecuteTemplate(w, "jobQueue", rows)
}

func (h *Handlers) handleUndo(w http.ResponseWriter, r *http.Request) {
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

	j, err := h.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	if j.Status != "done" {
		writeError(w, http.StatusBadRequest, "Only completed jobs can be undone")
		return
	}

	if j.NewPlaylistID == "" {
		writeError(w, http.StatusBadRequest, "No playlist to undo")
		return
	}

	if h.ytClient == nil {
		writeError(w, http.StatusBadRequest, "YouTube API not available")
		return
	}

	// Delete the created playlist from YouTube
	if err := h.ytClient.DeletePlaylist(r.Context(), j.NewPlaylistID); err != nil {
		h.logger.Errorc(r.Context(), "failed to delete playlist for undo", "jobId", jobID, "playlistId", j.NewPlaylistID, "error", err.Error())
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete playlist: %v", err))
		return
	}

	if err := h.store.SetJobUndone(jobID); err != nil {
		h.logger.Errorc(r.Context(), "failed to mark job as undone", "jobId", jobID, "error", err.Error())
		writeError(w, http.StatusInternalServerError, "Failed to update job status")
		return
	}

	h.logger.Infoc(r.Context(), "Job undone", "jobId", jobID, "playlistId", j.NewPlaylistID)

	// Redirect back to the job queue HTML
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "<div class='undo-success'>Playlist deleted. <button class='btn' hx-get='/api/jobs/queue/html' hx-target='#job-queue' hx-swap='innerHTML'>Refresh</button></div>")
}

func (h *Handlers) handleArchiveJob(w http.ResponseWriter, r *http.Request) {
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

	if err := h.store.ArchiveJob(jobID); err != nil {
		h.logger.Errorc(r.Context(), "failed to archive job", "jobId", jobID, "error", err.Error())
		writeError(w, http.StatusInternalServerError, "Failed to archive job")
		return
	}

	h.logger.Infoc(r.Context(), "Job archived", "jobId", jobID)

	// Return an empty response to trigger HTMX removal of the row
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<tr id="archived-`+jobID+`" hx-swap-oob="true"></tr>`)
}

func (h *Handlers) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
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

	if err := h.store.DeleteJob(jobID); err != nil {
		h.logger.Errorc(r.Context(), "failed to delete job", "jobId", jobID, "error", err.Error())
		writeError(w, http.StatusInternalServerError, "Failed to delete job")
		return
	}

	h.logger.Infoc(r.Context(), "Job deleted", "jobId", jobID)

	// Return success — HTMX removes the row
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<tr id="deleted-`+jobID+`" hx-swap-oob="true"></tr>`)
}

func (h *Handlers) handleArchivedJobsHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs, err := h.store.GetArchivedJobs(50)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if len(jobs) == 0 {
		tmpl.ExecuteTemplate(w, "noArchivedJobs", nil)
		return
	}

	rows := make([]ArchivedJobRowData, 0, len(jobs))
	for _, j := range jobs {
		label := j.SourceTitle
		if label == "" {
			label = j.SourcePlaylistID
		}

		progress := "-"
		if j.TotalItems > 0 {
			progress = fmt.Sprintf("%d / %d", j.InsertedItems, j.TotalItems)
		}

		created := j.CreatedAt
		if len(created) > 19 {
			created = created[:19]
		}
		created = strings.Replace(created, "T", " ", 1)

		rows = append(rows, ArchivedJobRowData{
			ID:       j.ID,
			Status:   j.Status,
			Title:    label,
			NewName:  j.NewName,
			Progress: progress,
			Created:  created,
		})
	}

	tmpl.ExecuteTemplate(w, "archivedJobs", rows)
}

// writeJobProgressHTML renders the job progress modal content based on status.
func writeJobProgressHTML(w http.ResponseWriter, jobID string, jp *job.Progress) {
	snap := jp.Snapshot()

	w.Header().Set("Content-Type", "text/html")

	switch snap.Status {
	case job.StatusPending:
		tmpl.ExecuteTemplate(w, "jobProgressPending", JobProgressData{
			JobID: jobID,
		})

	case job.StatusFetching, job.StatusShuffling:
		label := "Starting..."
		pct := 0
		switch snap.Status {
		case job.StatusFetching:
			label = "Fetching playlist items..."
			pct = 25
		case job.StatusShuffling:
			label = "Shuffling items..."
			pct = 50
		}
		tmpl.ExecuteTemplate(w, "jobProgressWorking", JobProgressData{
			JobID: jobID,
			Pct:   pct,
			Label: label,
		})

	case job.StatusInserting:
		pct := 50
		if snap.Total > 0 {
			pct = int(float64(snap.Done)/float64(snap.Total)*50) + 50
			if pct > 99 {
				pct = 99
			}
		}
		tmpl.ExecuteTemplate(w, "jobProgressWorking", JobProgressData{
			JobID: jobID,
			Pct:   pct,
			Label: "Inserting items...",
			Done:  snap.Done,
			Total: snap.Total,
		})

	case job.StatusDone:
		var playlistURL template.URL
		if snap.NewPlaylistID != "" {
			playlistURL = template.URL("https://www.youtube.com/playlist?list=" + snap.NewPlaylistID)
		}
		tmpl.ExecuteTemplate(w, "jobProgressDone", JobProgressData{
			JobID:          jobID,
			NewPlaylistURL: playlistURL,
		})

	case job.StatusPaused:
		pct := 0
		if snap.Total > 0 {
			pct = int(float64(snap.Done)/float64(snap.Total) * 100)
		}
		resumeAttr := template.HTML(fmt.Sprintf(`hx-vals='{"jobId":%q}'`, jobID))
		tmpl.ExecuteTemplate(w, "jobProgressPaused", JobProgressData{
			JobID:         jobID,
			Pct:           pct,
			Done:          snap.Done,
			Total:         snap.Total,
			ResumeBtnAttr: resumeAttr,
		})

	case job.StatusError:
		tmpl.ExecuteTemplate(w, "jobProgressError", JobProgressData{
			JobID: jobID,
			Error: snap.Error,
		})

	default:
		tmpl.ExecuteTemplate(w, "jobProgressDefault", JobProgressData{
			JobID: jobID,
		})
	}
}
