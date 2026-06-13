package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

func (h *Handlers) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if h.ytClient == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"playlists":  []PlaylistResponse{},
			"authNeeded": true,
			"authURL":    h.oauthURL(),
		})
		return
	}

	playlists, err := h.ytClient.GetPlaylists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.store.AddQuota(store.QuotaListPlaylists); err != nil {
		h.logger.Warnc(r.Context(), "failed to track quota", "error", err.Error())
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

func (h *Handlers) handlePlaylistsHTML(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))

	if h.ytClient == nil {
		h.renderAuthRequired(w, r)
		return
	}

	playlists, err := h.ytClient.GetPlaylists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := h.store.AddQuota(store.QuotaListPlaylists); err != nil {
		h.logger.Warnc(r.Context(), "failed to track quota", "error", err.Error())
	}

	quota, _ := h.store.GetQuota()

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
			tmpl.ExecuteTemplate(w, "noResults", nil)
		} else {
			tmpl.ExecuteTemplate(w, "emptyPlaylists", nil)
		}
		return
	}

	cards := make([]PlaylistCardData, 0, len(filtered))
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

		modalURL := template.URL(fmt.Sprintf("/api/modal/html?id=%s&itemCount=%d&title=%s",
			pl.ID, pl.ItemCount, url.QueryEscape(pl.Title)))

		cards = append(cards, PlaylistCardData{
			ID:           pl.ID,
			Title:        pl.Title,
			ItemCount:    pl.ItemCount,
			ItemCountStr: itemCountStr,
			Cost:         cost,
			ButtonClass:  btnClass,
			ButtonText:   btnText,
			ModalURL:     modalURL,
		})
	}

	tmpl.ExecuteTemplate(w, "playlistCards", cards)
}

func (h *Handlers) handleModalHTML(w http.ResponseWriter, r *http.Request) {
	playlistID := r.URL.Query().Get("id")
	itemCountStr := r.URL.Query().Get("itemCount")
	itemCount, _ := strconv.Atoi(itemCountStr)
	title := r.URL.Query().Get("title")
	if title == "" {
		title = "Selected Playlist"
	}

	quota, _ := h.store.GetQuota()
	cost := store.QuotaCreatePlaylist + itemCount*store.QuotaInsertItem

	now := time.Now()
	monthYear := now.Format("January 2006")
	defaultName := fmt.Sprintf("%s-randomized-%s", title, monthYear)

	w.Header().Set("Content-Type", "text/html")
	lowQuota := quota != nil && quota.Remaining < cost
	var warningHTML template.HTML
	if lowQuota {
		warningHTML = `<div class="quota-warning-banner"><p>⚠️ Insufficient quota for one session. The job will save progress and auto-resume when quota is available (within ~24h).</p></div>`
	}

	tmpl.ExecuteTemplate(w, "modal", ModalData{
		PlaylistID:     playlistID,
		Title:          title,
		DefaultName:    defaultName,
		Cost:           cost,
		QuotaCostClass: quotaCostClass(quota, cost),
		QuotaText:      quotaText(quota, cost),
		WarningHTML:    warningHTML,
	})
}
