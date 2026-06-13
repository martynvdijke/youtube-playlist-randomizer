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

	ids := req.PlaylistIDs
	if len(ids) == 0 && req.PlaylistID != "" {
		ids = []string{req.PlaylistID}
	}
	if len(ids) == 0 || req.NewName == "" {
		writeError(w, http.StatusBadRequest, "playlistId(s) and newName are required")
		return
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())

	if err := h.store.CreateJob(jobID, ids[0], "", req.NewName, ids[1:]...); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create job")
		return
	}

	if h.otel != nil {
		h.otel.RecordJobCreated(r.Context())
	}

	jp := h.jobRunner.Run(r.Context(), jobID, req.NewName, ids...)

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

	playlistIDs := r.Form["playlistId"]
	newName := r.FormValue("newName")

	if len(playlistIDs) == 0 || newName == "" {
		w.WriteHeader(http.StatusBadRequest)
		tmpl.ExecuteTemplate(w, "randomizeError", RandomizeErrorData{Error: "playlistId(s) and newName are required"})
		return
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())

	if err := h.store.CreateJob(jobID, playlistIDs[0], "", newName, playlistIDs[1:]...); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		tmpl.ExecuteTemplate(w, "randomizeError", RandomizeErrorData{Error: fmt.Sprintf("Failed to create job: %s", err.Error())})
		return
	}

	jp := h.jobRunner.Run(r.Context(), jobID, newName, playlistIDs...)

	writeJobProgressHTML(w, jobID, jp)
}
