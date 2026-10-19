package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
)

func testAuthHandlers(password string) *Handlers {
	return &Handlers{
		adminPassword: password,
		sessionKey:    []byte("01234567890123456789012345678901"),
		oauthStates:   make(map[string]time.Time),
		logger:        logging.New(logging.LogOptions{MinLevel: logging.ERROR}),
	}
}

func TestAuthMiddleware_RequiresAuthForMutatingRoutes(t *testing.T) {
	h := testAuthHandlers("correct horse battery staple")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := h.AuthMiddleware(next)

	req := httptest.NewRequest(http.MethodPost, "/api/randomize", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_RequiresAuthForAdminRoutes(t *testing.T) {
	h := testAuthHandlers("secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := h.AuthMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/settings/gotify", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for admin route, got %d", rr.Code)
	}
}

func TestAuthMiddleware_PublicReadOnlyRoutesStayOpen(t *testing.T) {
	h := testAuthHandlers("secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := h.AuthMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected public read route to stay open, got %d", rr.Code)
	}
}

func TestAuthMiddleware_BrowserRedirectsToLogin(t *testing.T) {
	h := testAuthHandlers("secret")
	handler := h.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/logs/html", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	req.Header.Set("HX-Request", "true")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for API path, got %d", rr.Code)
	}
	if got := rr.Header().Get("HX-Redirect"); got == "" {
		t.Fatalf("expected HX-Redirect header, got none")
	}
	if !strings.Contains(rr.Header().Get("HX-Redirect"), "/login?next=") {
		t.Errorf("expected login redirect, got %q", rr.Header().Get("HX-Redirect"))
	}
}

func TestAuthMiddleware_NoPasswordAllowsLoopbackOnly(t *testing.T) {
	h := testAuthHandlers("")
	handler := h.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	loop := httptest.NewRequest(http.MethodPost, "/api/randomize", nil)
	loop.RemoteAddr = "127.0.0.1:5000"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, loop)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected loopback request allowed, got %d", rr.Code)
	}

	remote := httptest.NewRequest(http.MethodPost, "/api/randomize", nil)
	remote.RemoteAddr = "198.51.100.7:5000"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, remote)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected remote request denied, got %d", rr.Code)
	}

	proxied := httptest.NewRequest(http.MethodPost, "/api/randomize", nil)
	proxied.RemoteAddr = "127.0.0.1:5000"
	proxied.Header.Set("X-Forwarded-For", "198.51.100.7")
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, proxied)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected proxied request denied, got %d", rr.Code)
	}
}

func TestLoginFlowSetsSessionAndAuthorizes(t *testing.T) {
	h := testAuthHandlers("s3cret-password")
	h.secureCookies = true

	form := url.Values{"password": {"s3cret-password"}, "next": {"/api/admin/logs/html"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.handleLogin(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d: %s", rr.Code, rr.Body.String())
	}
	resp := rr.Result()
	var session *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatal("expected session cookie")
	}
	if !session.HttpOnly {
		t.Error("expected HttpOnly session cookie")
	}
	if !session.Secure {
		t.Error("expected Secure session cookie")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax, got %v", session.SameSite)
	}

	auth := h.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	authed := httptest.NewRequest(http.MethodPost, "/api/randomize", nil)
	authed.RemoteAddr = "203.0.113.5:1234"
	authed.AddCookie(session)
	rr = httptest.NewRecorder()
	auth.ServeHTTP(rr, authed)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected authenticated request allowed, got %d", rr.Code)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	h := testAuthHandlers("right")
	form := url.Values{"password": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.handleLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			t.Fatal("should not issue a session for a failed login")
		}
	}
}

func TestValidSessionRejectsTamperedToken(t *testing.T) {
	h := testAuthHandlers("pw")
	valid := h.newSessionToken()
	req := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: valid})
	if !h.validSession(req) {
		t.Fatal("expected freshly issued session to be valid")
	}

	tampered := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
	tampered.AddCookie(&http.Cookie{Name: sessionCookieName, Value: valid + "tampered"})
	if h.validSession(tampered) {
		t.Fatal("expected tampered session to be rejected")
	}
}

func TestOAuthStateIsRandomAndSingleUse(t *testing.T) {
	h := testAuthHandlers("pw")
	a := h.newOAuthState()
	b := h.newOAuthState()
	if a == "" || b == "" || a == b {
		t.Fatalf("expected unique non-empty states, got %q and %q", a, b)
	}
	if len(a) < 32 {
		t.Errorf("expected at least 128 bits of state, got %d chars", len(a))
	}
	if !h.consumeOAuthState(a) {
		t.Fatal("expected fresh state to be accepted")
	}
	if h.consumeOAuthState(a) {
		t.Fatal("expected state to be single-use")
	}
	if h.consumeOAuthState("not-a-real-state") {
		t.Fatal("expected unknown state to be rejected")
	}
}

func TestOAuthStateExpires(t *testing.T) {
	h := testAuthHandlers("pw")
	state := h.newOAuthState()
	h.oauthMu.Lock()
	h.oauthStates[state] = time.Now().Add(-time.Second)
	h.oauthMu.Unlock()
	if h.consumeOAuthState(state) {
		t.Fatal("expected expired state to be rejected")
	}
}

func TestOAuthCallbackRequiresBrowserBoundState(t *testing.T) {
	h := testAuthHandlers("pw")
	state := h.newOAuthState()

	// A valid server-side state is not enough: it must also match the
	// HttpOnly cookie set when the flow started.
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+state, nil)
	rr := httptest.NewRecorder()
	h.handleOAuthCallback(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without bound cookie, got %d", rr.Code)
	}

	// The failed attempt must not consume the state.
	if !h.consumeOAuthState(state) {
		t.Fatal("expected state to remain usable after an unbound attempt")
	}
}

func TestOAuthCallbackRejectsMismatchedStateCookie(t *testing.T) {
	h := testAuthHandlers("pw")
	state := h.newOAuthState()

	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "different"})
	rr := httptest.NewRecorder()
	h.handleOAuthCallback(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for mismatched cookie, got %d", rr.Code)
	}
	if !h.consumeOAuthState(state) {
		t.Fatal("expected state not to be consumed on mismatch")
	}
}

func TestOAuthCallbackRejectsBadState(t *testing.T) {
	h := testAuthHandlers("pw")
	req := httptest.NewRequest(http.MethodGet, "/callback?code=abc&state=wrong", nil)
	rr := httptest.NewRecorder()
	h.handleOAuthCallback(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad state, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "state") {
		t.Errorf("expected state error, got %q", rr.Body.String())
	}
}

func TestSafeNextRejectsExternalURLs(t *testing.T) {
	for _, in := range []string{"https://evil.example.com", "//evil.example.com", "javascript:alert(1)", ""} {
		if got := safeNext(in); got != "/" {
			t.Errorf("safeNext(%q) = %q, want /", in, got)
		}
	}
	if got := safeNext("/api/admin/logs/html?x=1"); got != "/api/admin/logs/html?x=1" {
		t.Errorf("safeNext kept valid path incorrectly: %q", got)
	}
}

func TestYTClientAccessIsRaceFree(t *testing.T) {
	h := testAuthHandlers("pw")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			h.setYTClient(nil)
		}()
		go func() {
			defer wg.Done()
			_ = h.getYTClient()
		}()
	}
	wg.Wait()
}
