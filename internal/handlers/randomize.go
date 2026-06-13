package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (h *Handlers) handleRandomize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if h.ytClient == nil {
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

	if err := h.store.CreateJob(jobID, req.PlaylistID, "", req.NewName); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create job")
		return
	}

	if h.otel != nil {
		h.otel.RecordJobCreated(r.Context())
	}

	jp := h.jobRunner.Run(r.Context(), jobID, req.PlaylistID, req.NewName)

	writeJSON(w, http.StatusAccepted, JobResponse{JobID: jobID, Status: jp.Status})
}

func (h *Handlers) handleRandomizeHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		tmpl.ExecuteTemplate(w, "randomizeError", RandomizeErrorData{Error: "Method not allowed"})
		return
	}

	if h.ytClient == nil {
		w.WriteHeader(http.StatusBadRequest)
		tmpl.ExecuteTemplate(w, "randomizeError", RandomizeErrorData{Error: "YouTube API not available in mock mode"})
		return
	}

	playlistID := r.FormValue("playlistId")
	newName := r.FormValue("newName")

	if playlistID == "" || newName == "" {
		w.WriteHeader(http.StatusBadRequest)
		tmpl.ExecuteTemplate(w, "randomizeError", RandomizeErrorData{Error: "playlistId and newName are required"})
		return
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())

	if err := h.store.CreateJob(jobID, playlistID, "", newName); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		tmpl.ExecuteTemplate(w, "randomizeError", RandomizeErrorData{Error: fmt.Sprintf("Failed to create job: %s", err.Error())})
		return
	}

	jp := h.jobRunner.Run(r.Context(), jobID, playlistID, newName)

	writeJobProgressHTML(w, jobID, jp)
}
