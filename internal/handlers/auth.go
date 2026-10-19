package handlers

import (
	"context"
	"net/http"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/youtube"
)

func (h *Handlers) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
		h.logger.Warnc(r.Context(), "OAuth provider returned an error", "error", oauthErr)
		http.Error(w, "Authentication was cancelled or rejected", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Verify the browser-bound state and consume it before exchanging the code
	// or doing any other work.
	state := r.URL.Query().Get("state")
	if !h.verifyOAuthState(r, state) {
		h.clearOAuthStateCookie(w)
		h.logger.Warnc(r.Context(), "OAuth callback rejected: invalid or replayed state")
		http.Error(w, "Invalid or expired OAuth state", http.StatusBadRequest)
		return
	}
	h.clearOAuthStateCookie(w)

	if h.oauthSetup == nil {
		http.Error(w, "OAuth not configured", http.StatusInternalServerError)
		return
	}

	if err := youtube.HandleCallback(h.oauthSetup, code, h.dataDir); err != nil {
		h.logger.Errorc(r.Context(), "OAuth callback error", "error", err.Error())
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	h.logger.Infoc(r.Context(), "OAuth authentication successful! Token saved. Recreating YouTube client...")

	newClient, err := youtube.NewClient(context.Background(), h.clientSecretPath, h.dataDir, h.otel, h.logger)
	if err == nil && newClient != nil {
		h.setYTClient(newClient)
		if h.jobRunner != nil {
			h.jobRunner.SetYTClient(newClient)
		}
		h.logger.Infoc(r.Context(), "YouTube client recreated successfully!")
	} else if err == nil && newClient == nil {
		h.logger.Warnc(r.Context(), "Token still invalid after callback (unexpected)")
	} else {
		h.logger.Warnc(r.Context(), "New client error (non-critical)", "error", err.Error())
		if newClient != nil {
			h.setYTClient(newClient)
			if h.jobRunner != nil {
				h.jobRunner.SetYTClient(newClient)
			}
		}
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.ExecuteTemplate(w, "authSuccess", nil)
}

func (h *Handlers) handleAuth(w http.ResponseWriter, r *http.Request) {
	h.renderAuthRequired(w, r)
}

func (h *Handlers) renderAuthRequired(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if h.oauthSetup == nil {
		tmpl.ExecuteTemplate(w, "authNotConfigured", nil)
		return
	}
	tmpl.ExecuteTemplate(w, "authRequired", AuthRequiredData{
		AuthURL: h.oauthURLWithState(h.issueOAuthState(w)),
	})
}
