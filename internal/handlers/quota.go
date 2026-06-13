package handlers

import (
	"net/http"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

func (h *Handlers) handleQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	q, err := h.store.GetQuota()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.otel != nil {
		h.otel.RecordQuotaMetrics(r.Context(), q.Used, q.Limit, q.Remaining)
	}
	writeJSON(w, http.StatusOK, QuotaResponse{
		Used:      q.Used,
		Limit:     q.Limit,
		Remaining: q.Remaining,
		Date:      q.Date,
	})
}

func (h *Handlers) handleQuotaHTML(w http.ResponseWriter, r *http.Request) {
	q, err := h.store.GetQuota()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	pct, fillClass := writeQuotaPct(q.Used, q.Limit)
	w.Header().Set("Content-Type", "text/html")
	tmpl.ExecuteTemplate(w, "quota", QuotaData{
		Used:      q.Used,
		Limit:     q.Limit,
		Remaining: q.Remaining,
		FillClass: fillClass,
		Pct:       pct,
	})
}

func writeQuotaPct(used, limit int) (float64, string) {
	pct := 0.0
	if limit > 0 {
		pct = float64(used) / float64(limit) * 100
	}
	displayPct := pct
	if displayPct > 100 {
		displayPct = 100
	}
	fillClass := "quota-fill"
	if pct > 80 {
		fillClass += " quota-critical"
	} else if pct > 50 {
		fillClass += " quota-warning"
	}
	return displayPct, fillClass
}

func quotaCostClass(quota *store.QuotaInfo, cost int) string {
	if quota == nil {
		return "quota-cost quota-low"
	}
	if quota.Remaining >= cost {
		return "quota-cost quota-ok"
	}
	return "quota-cost quota-warning"
}

func quotaText(quota *store.QuotaInfo, cost int) string {
	if quota == nil {
		return "Unknown"
	}
	if quota.Remaining >= cost {
		return "Sufficient"
	}
	return "Low (will resume)"
}
