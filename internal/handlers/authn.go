package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName    = "ypr_session"
	oauthStateCookieName = "ypr_oauth_state"
	sessionMaxAge        = 12 * time.Hour
	oauthStateTTL        = 10 * time.Minute
)

// LoginData is passed to the login template.
type LoginData struct {
	Error     string
	Next      string
	CSRFToken string
}

// requiresAuth reports whether a request targets an admin or mutating route.
// Read-only endpoints (quota, playlists, job status) stay public so the
// dashboard keeps working before login; anything that changes state or
// exposes admin configuration is protected.
func (h *Handlers) requiresAuth(r *http.Request) bool {
	p := r.URL.Path
	switch p {
	case "/login", "/logout", "/callback":
		return false
	}
	if strings.HasPrefix(p, "/static/") {
		return false
	}
	if strings.HasPrefix(p, "/api/admin/") {
		return true
	}
	switch p {
	case "/api/randomize", "/api/randomize/html",
		"/api/jobs/resume", "/api/jobs/undo", "/api/jobs/archive", "/api/jobs/delete",
		"/api/auth":
		return true
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// AuthMiddleware enforces authentication for admin and mutating routes.
//
// When ADMIN_PASSWORD is configured, a valid signed session cookie is
// required. When it is unset, protected routes are only reachable from
// loopback so local single-user installs keep working while network exposure
// is denied.
func (h *Handlers) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.requiresAuth(r) {
			next.ServeHTTP(w, r)
			return
		}

		if h.adminPassword == "" {
			if isLoopbackRequest(r) {
				next.ServeHTTP(w, r)
				return
			}
			h.denyUnauthenticated(w, r, "admin authentication is not configured; set ADMIN_PASSWORD to enable remote access")
			return
		}

		if h.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		h.denyUnauthenticated(w, r, "authentication required")
	})
}

func (h *Handlers) denyUnauthenticated(w http.ResponseWriter, r *http.Request, msg string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		// HTMX follows redirects transparently, which would swap the login
		// page into the current fragment. Use HX-Redirect instead.
		if r.Header.Get("HX-Request") == "true" {
			next := url.QueryEscape(r.URL.RequestURI())
			w.Header().Set("HX-Redirect", "/login?next="+next)
		}
		writeError(w, http.StatusUnauthorized, msg)
		return
	}
	next := url.QueryEscape(r.URL.RequestURI())
	http.Redirect(w, r, "/login?next="+next, http.StatusSeeOther)
}

func isLoopbackRequest(r *http.Request) bool {
	// A reverse proxy makes every request look loopback. Treat forwarding
	// headers as remote so a proxied deployment cannot bypass auth by leaving
	// ADMIN_PASSWORD unset.
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newOAuthState creates a random, single-use OAuth state value.
func (h *Handlers) newOAuthState() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is unrecoverable for security-sensitive state.
		panic("crypto/rand unavailable: " + err.Error())
	}
	state := hex.EncodeToString(buf)

	h.oauthMu.Lock()
	defer h.oauthMu.Unlock()
	if h.oauthStates == nil {
		h.oauthStates = make(map[string]time.Time)
	}
	now := time.Now()
	for s, expiry := range h.oauthStates {
		if now.After(expiry) {
			delete(h.oauthStates, s)
		}
	}
	h.oauthStates[state] = now.Add(oauthStateTTL)
	return state
}

// issueOAuthState creates a one-time state, remembers it server-side and binds
// it to this browser with an HttpOnly cookie. Binding the state to the browser
// prevents an attacker from completing a login flow they started in a victim's
// browser (login CSRF).
func (h *Handlers) issueOAuthState(w http.ResponseWriter) string {
	state := h.newOAuthState()
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthStateTTL.Seconds()),
	})
	return state
}

// verifyOAuthState checks the callback state against both the browser-bound
// cookie and the server-side one-time store, consuming it on success.
func (h *Handlers) verifyOAuthState(r *http.Request, state string) bool {
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || state == "" || !constantTimeEqual(cookie.Value, state) {
		return false
	}
	return h.consumeOAuthState(state)
}

// clearOAuthStateCookie removes the browser-bound state after a callback.
func (h *Handlers) clearOAuthStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// consumeOAuthState verifies and deletes an OAuth state value. A state can
// only be used once, even if the callback is replayed.
func (h *Handlers) consumeOAuthState(state string) bool {
	if state == "" {
		return false
	}
	h.oauthMu.Lock()
	defer h.oauthMu.Unlock()
	expiry, ok := h.oauthStates[state]
	if !ok {
		return false
	}
	delete(h.oauthStates, state)
	return time.Now().Before(expiry)
}

// constantTimeEqual compares two strings without leaking length via timing.
func constantTimeEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func (h *Handlers) newSessionToken() string {
	expiry := time.Now().Add(sessionMaxAge).Unix()
	payload := strconv.FormatInt(expiry, 10)
	mac := hmac.New(sha256.New, h.sessionKey)
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func (h *Handlers) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	mac := hmac.New(sha256.New, h.sessionKey)
	mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

func (h *Handlers) setSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    h.newSessionToken(),
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookies || r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionMaxAge.Seconds()),
	})
}

func (h *Handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// safeNext only permits same-origin absolute paths, preventing open redirects.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if u, err := url.Parse(next); err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return next
}

func (h *Handlers) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if h.adminPassword == "" {
			// Loopback-only mode: there is nothing to log in to.
			http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
			return
		}
		if h.validSession(r) {
			http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
			return
		}
		h.renderLogin(w, http.StatusOK, "", safeNext(r.URL.Query().Get("next")))
	case http.MethodPost:
		if h.adminPassword == "" {
			// Loopback-only mode: no password is configured.
			http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
			return
		}
		next := safeNext(r.FormValue("next"))
		if !constantTimeEqual(r.FormValue("password"), h.adminPassword) {
			if h.logger != nil {
				h.logger.Warnc(r.Context(), "Failed admin login attempt", "remote", r.RemoteAddr)
			}
			h.renderLogin(w, http.StatusUnauthorized, "Invalid password", next)
			return
		}
		h.setSessionCookie(w, r)
		if h.logger != nil {
			h.logger.Infoc(r.Context(), "Admin login successful", "remote", r.RemoteAddr)
		}
		http.Redirect(w, r, next, http.StatusSeeOther)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) renderLogin(w http.ResponseWriter, status int, errMsg, next string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	tmpl.ExecuteTemplate(w, "login", LoginData{
		Error:     errMsg,
		Next:      next,
		CSRFToken: h.csrfToken(),
	})
}

func (h *Handlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.clearSessionCookie(w)
	if h.adminPassword != "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
