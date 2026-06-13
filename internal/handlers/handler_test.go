package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/job"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

// --- Helper function tests ---

func TestWriteQuotaPct(t *testing.T) {
	tests := []struct {
		used       int
		limit      int
		wantPct    float64
		wantClass  string
		classCheck func(string) bool
	}{
		{0, 100, 0, "quota-fill", func(s string) bool { return s == "quota-fill" }},
		{10, 100, 10, "quota-fill", func(s string) bool { return s == "quota-fill" }},
		{60, 100, 60, "quota-fill", func(s string) bool { return strings.Contains(s, "quota-warning") }},
		{90, 100, 90, "quota-fill", func(s string) bool { return strings.Contains(s, "quota-critical") }},
		{0, 0, 0, "quota-fill", func(s string) bool { return s == "quota-fill" }},
		{100, 100, 100, "quota-fill", func(s string) bool { return strings.Contains(s, "quota-critical") }},
		{105, 100, 100, "quota-fill", func(s string) bool { return strings.Contains(s, "quota-critical") }},
		{200, 100, 100, "quota-fill", func(s string) bool { return strings.Contains(s, "quota-critical") }},
	}

	for _, tc := range tests {
		pct, class := writeQuotaPct(tc.used, tc.limit)
		if pct != tc.wantPct {
			t.Errorf("writeQuotaPct(%d, %d) pct = %f, want %f", tc.used, tc.limit, pct, tc.wantPct)
		}
		if !tc.classCheck(class) {
			t.Errorf("writeQuotaPct(%d, %d) class = %q, does not satisfy check", tc.used, tc.limit, class)
		}
	}
}

func TestWriteQuotaPct_EdgeCases(t *testing.T) {
	pct, class := writeQuotaPct(0, 0)
	if pct != 0 {
		t.Errorf("zero limit pct = %f, want 0", pct)
	}
	if class != "quota-fill" {
		t.Errorf("zero limit class = %q, want quota-fill", class)
	}

	pct, class = writeQuotaPct(10000, 10000)
	if pct != 100 {
		t.Errorf("full quota pct = %f, want 100", pct)
	}
	if !strings.Contains(class, "quota-critical") {
		t.Errorf("full quota should be critical, got %q", class)
	}
}

func TestQuotaCostClass(t *testing.T) {
	sufficient := &store.QuotaInfo{Remaining: 100}
	insufficient := &store.QuotaInfo{Remaining: 5}

	tests := []struct {
		name  string
		quota *store.QuotaInfo
		cost  int
		want  string
	}{
		{"nil quota", nil, 10, "quota-cost quota-low"},
		{"sufficient", sufficient, 10, "quota-cost quota-ok"},
		{"exact sufficient", sufficient, 100, "quota-cost quota-ok"},
		{"insufficient", insufficient, 10, "quota-cost quota-warning"},
		{"zero cost nil", nil, 0, "quota-cost quota-low"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := quotaCostClass(tc.quota, tc.cost)
			if got != tc.want {
				t.Errorf("quotaCostClass() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQuotaText(t *testing.T) {
	sufficient := &store.QuotaInfo{Remaining: 100}
	insufficient := &store.QuotaInfo{Remaining: 5}

	tests := []struct {
		name  string
		quota *store.QuotaInfo
		cost  int
		want  string
	}{
		{"nil quota", nil, 10, "Unknown"},
		{"sufficient", sufficient, 10, "Sufficient"},
		{"exact sufficient", sufficient, 100, "Sufficient"},
		{"insufficient", insufficient, 10, "Low (will resume)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := quotaText(tc.quota, tc.cost)
			if got != tc.want {
				t.Errorf("quotaText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCorsMiddleware(t *testing.T) {
	h := &Handlers{}
	handler := h.CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	tests := []struct {
		name       string
		method     string
		wantStatus int
		wantBody   string
		wantOrigin string
	}{
		{"GET request", "GET", http.StatusOK, "ok", "*"},
		{"OPTIONS request", "OPTIONS", http.StatusOK, "", "*"},
		{"POST request", "POST", http.StatusOK, "ok", "*"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/test", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d", tc.wantStatus, rr.Code)
			}
			if rr.Body.String() != tc.wantBody {
				t.Errorf("expected body %q, got %q", tc.wantBody, rr.Body.String())
			}
			origin := rr.Header().Get("Access-Control-Allow-Origin")
			if origin != tc.wantOrigin {
				t.Errorf("expected Access-Control-Allow-Origin %q, got %q", tc.wantOrigin, origin)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"key": "value"})

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, `"key":"value"`) {
		t.Errorf("expected body to contain key:value, got %q", body)
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "invalid request")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, `"error":"invalid request"`) {
		t.Errorf("expected error body, got %q", body)
	}
}

// --- Response type JSON tests ---

func TestQuotaResponseJSON(t *testing.T) {
	q := QuotaResponse{Used: 50, Limit: 10000, Remaining: 9950, Date: "2026-01-01"}
	data, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"remaining":9950`) {
		t.Errorf("expected remaining 9950 in JSON, got %s", string(data))
	}
}

func TestJobResponseJSON(t *testing.T) {
	j := JobResponse{JobID: "test-123", Status: job.StatusPending}
	data, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"jobId":"test-123"`) {
		t.Errorf("expected jobId in JSON, got %s", string(data))
	}
	if !strings.Contains(string(data), `"status":"pending"`) {
		t.Errorf("expected status pending in JSON, got %s", string(data))
	}
}

func TestRandomizeRequestJSON(t *testing.T) {
	r := RandomizeRequest{PlaylistID: "PL123", NewName: "My Mix"}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"playlistId":"PL123"`) {
		t.Errorf("expected playlistId in JSON, got %s", string(data))
	}
}

// --- Handler integration tests ---

func TestHandleOAuthCallback_NoCode(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/callback", nil)
	h.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Missing") {
		t.Errorf("expected error message about missing code, got %s", rr.Body.String())
	}
}

func TestHandleAuth_NoSetup(t *testing.T) {
	h := &Handlers{oauthSetup: nil}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/auth", nil)
	h.handleAuth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "OAuth not configured") {
		t.Errorf("expected 'OAuth not configured' message, got: %s", rr.Body.String())
	}
}

func TestHandleForceResume_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/resume", nil)
	h.handleForceResume(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestHandleForceResume_NoJobID(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/resume", nil)
	h.handleForceResume(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Missing") {
		t.Errorf("expected 'Missing job ID', got %s", rr.Body.String())
	}
}

func TestHandleForceResume_YTClientNil(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	if err := s.CreateJob("test-job-1", "PL_SRC", "Title", "New Name"); err != nil {
		t.Fatalf("create job: %v", err)
	}

	h := &Handlers{store: s}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/resume", strings.NewReader("jobId=test-job-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.handleForceResume(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestHandleJobQueueHTML_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/queue/html", nil)
	h.handleJobQueueHTML(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestHandleJobQueueHTML_EmptyQueue(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	h := &Handlers{store: s}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/queue/html", nil)
	h.handleJobQueueHTML(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `class="job-queue hidden"`) {
		t.Errorf("expected hidden job-queue for empty result, got: %s", rr.Body.String())
	}
}

func TestHandleJobQueueHTML_WithPausedJob(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	if err := s.CreateJob("test-job-1", "PL_SRC", "Source Title", "My Playlist"); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := s.SetJobPaused("test-job-1"); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	h := &Handlers{store: s}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/queue/html", nil)
	h.handleJobQueueHTML(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Resume Now") {
		t.Errorf("expected Resume Now button in response, got: %s", body)
	}
	if !strings.Contains(body, `hx-post="/api/jobs/resume"`) {
		t.Errorf("expected hx-post to /api/jobs/resume, got: %s", body)
	}
	if !strings.Contains(body, `"jobId":"test-job-1"`) {
		t.Errorf("expected job ID in hx-vals, got: %s", body)
	}
	if !strings.Contains(body, `class="status-paused"`) {
		t.Errorf("expected paused status class, got: %s", body)
	}
	if !strings.Contains(body, "Source Title") {
		t.Errorf("expected source title in response, got: %s", body)
	}
}

// --- Additional handler tests ---

func TestHandleQuota_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/quota", nil)
	h.handleQuota(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleQuotaHTML_ReturnsHTML(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	h := &Handlers{store: s}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/quota/html", nil)
	h.handleQuotaHTML(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("expected text/html, got %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "quota") {
		t.Errorf("expected quota in body, got: %s", rr.Body.String())
	}
}

func TestHandlePlaylists_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/playlists", nil)
	h.handlePlaylists(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlePlaylists_NoYTClient(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/playlists", nil)
	h.handlePlaylists(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "authNeeded") {
		t.Errorf("expected authNeeded in body, got: %s", rr.Body.String())
	}
}

func TestHandlePlaylistsHTML_NoYTClient(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/playlists/html", nil)
	h.handlePlaylistsHTML(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "auth") && !strings.Contains(body, "OAuth") {
		t.Errorf("expected auth/OAuth in body, got: %s", body)
	}
}

func TestHandleModalHTML_WithParams(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	h := &Handlers{store: s}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/modal/html?id=PL123&itemCount=10&title=TestPlaylist", nil)
	h.handleModalHTML(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "TestPlaylist") {
		t.Errorf("expected TestPlaylist in body, got: %s", rr.Body.String())
	}
}

func TestHandleModalHTML_DefaultTitle(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	h := &Handlers{store: s}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/modal/html?id=PL123&itemCount=5", nil)
	h.handleModalHTML(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Selected Playlist") {
		t.Errorf("expected 'Selected Playlist' as default, got: %s", rr.Body.String())
	}
}

func TestHandleRandomize_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/randomize", nil)
	h.handleRandomize(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleRandomize_NoYTClient(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/randomize", strings.NewReader(`{"playlistId":"PL1","newName":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	h.handleRandomize(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleRandomizeHTML_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/randomize/html", nil)
	h.handleRandomizeHTML(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Method not allowed") {
		t.Errorf("expected error message, got: %s", rr.Body.String())
	}
}

func TestHandlePlaylistPreviewHTML_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/playlists/preview/html", nil)
	h.handlePlaylistPreviewHTML(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlePlaylistPreviewHTML_MissingID(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/playlists/preview/html", nil)
	h.handlePlaylistPreviewHTML(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Missing") {
		t.Errorf("expected 'Missing' in body, got: %s", rr.Body.String())
	}
}

func TestHandlePlaylistPreviewHTML_NoYTClient(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/playlists/preview/html?id=PL123", nil)
	h.handlePlaylistPreviewHTML(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleJobStatus_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/xyz", nil)
	h.handleJobStatus(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleJobStatus_EmptyID(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/", nil)
	h.handleJobStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleUndo_BadMethod(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/undo", nil)
	h.handleUndo(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleUndo_MissingJobID(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/undo", nil)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.handleUndo(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- CSRF tests ---

func TestCSRFToken_NonEmpty(t *testing.T) {
	h := &Handlers{csrfKey: make([]byte, 32)}
	for i := range h.csrfKey {
		h.csrfKey[i] = byte(i)
	}
	tok := h.csrfToken()
	if tok == "" {
		t.Fatal("expected non-empty CSRF token")
	}
	if len(tok) < 10 {
		t.Errorf("expected token length >= 10, got %d", len(tok))
	}
}

func TestCSRFValidate_ValidToken(t *testing.T) {
	h := &Handlers{csrfKey: make([]byte, 32)}
	for i := range h.csrfKey {
		h.csrfKey[i] = byte(i)
	}
	tok := h.csrfToken()
	req := httptest.NewRequest("POST", "/api/randomize", nil)
	req.Header.Set("X-CSRF-Token", tok)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: tok})
	if !h.validateCSRF(req) {
		t.Error("expected valid CSRF token to pass validation")
	}
}

func TestCSRFValidate_NoCookie(t *testing.T) {
	h := &Handlers{csrfKey: make([]byte, 32)}
	req := httptest.NewRequest("POST", "/api/randomize", nil)
	req.Header.Set("X-CSRF-Token", "anything")
	if h.validateCSRF(req) {
		t.Error("expected validation to fail without cookie")
	}
}

func TestCSRFValidate_Mismatch(t *testing.T) {
	h := &Handlers{csrfKey: make([]byte, 32)}
	req := httptest.NewRequest("POST", "/api/randomize", nil)
	req.Header.Set("X-CSRF-Token", "token-a")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token-b"})
	if h.validateCSRF(req) {
		t.Error("expected validation to fail with mismatched tokens")
	}
}

func TestCSRFValidate_SkipsGET(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest("GET", "/api/playlists", nil)
	if !h.validateCSRF(req) {
		t.Error("expected GET to skip CSRF validation")
	}
}

func TestCSRFValidate_SkipsCallback(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest("POST", "/callback?code=abc", nil)
	if !h.validateCSRF(req) {
		t.Error("expected OAuth callback to skip CSRF validation")
	}
}

func TestCSRFMiddleware_SetsCookieOnGET(t *testing.T) {
	h := New(&Config{
		Store:   &store.Store{},
		Logger:  nil,
		Version: "test",
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	handler := h.CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(rr, req)
	resp := rr.Result()
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == "csrf_token" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected csrf_token cookie to be set")
	}
}

func TestCSRFMiddleware_RejectsPOSTWithoutToken(t *testing.T) {
	h := New(&Config{
		Store:   &store.Store{},
		Logger:  nil,
		Version: "test",
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/randomize", nil)
	handler := h.CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_AcceptsPOSTWithValidToken(t *testing.T) {
	h := New(&Config{
		Store:   &store.Store{},
		Logger:  nil,
		Version: "test",
	})
	// First GET to get CSRF cookie
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	handler := h.CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(rr, req)
	resp := rr.Result()
	var csrfCookie string
	for _, c := range resp.Cookies() {
		if c.Name == "csrf_token" {
			csrfCookie = c.Value
			break
		}
	}
	if csrfCookie == "" {
		t.Fatal("no CSRF cookie in response")
	}

	// Now POST with the token
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/randomize", nil)
	req2.Header.Set("X-CSRF-Token", csrfCookie)
	req2.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrfCookie})
	handler.ServeHTTP(rr2, req2)
	if rr2.Code == http.StatusForbidden {
		t.Errorf("expected POST with valid token to succeed, got 403: %s", rr2.Body.String())
	}
}

func TestCSRFValidate_FormField(t *testing.T) {
	h := &Handlers{csrfKey: make([]byte, 32)}
	for i := range h.csrfKey {
		h.csrfKey[i] = byte(i)
	}
	tok := h.csrfToken()
	body := "csrf_token=" + tok
	req := httptest.NewRequest("POST", "/api/randomize", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: tok})
	if !h.validateCSRF(req) {
		t.Error("expected form field CSRF token to pass validation")
	}
}
