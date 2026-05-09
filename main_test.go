package main

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	if err := initDB(":memory:"); err != nil {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestInitDB(t *testing.T) {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM oauth_token").Scan(&count)
	if count > 0 {
		t.Errorf("expected empty oauth_token, got %d", count)
	}

	var qcount int
	db.QueryRow("SELECT COUNT(*) FROM quota_usage").Scan(&qcount)
	if qcount > 0 {
		t.Errorf("expected empty quota_usage, got %d", qcount)
	}

	var scount int
	db.QueryRow("SELECT COUNT(*) FROM settings").Scan(&scount)
	if scount > 0 {
		t.Errorf("expected empty settings, got %d", scount)
	}
}

func TestSaveLoadToken(t *testing.T) {
	token := &oauth2.Token{
		AccessToken:  "test-access-token",
		TokenType:    "Bearer",
		RefreshToken: "test-refresh-token",
		Expiry:       time.Now().Add(time.Hour),
	}

	if err := saveToken(token); err != nil {
		t.Fatalf("saveToken: %v", err)
	}

	loaded, err := loadToken()
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}
	if loaded == nil {
		t.Fatal("loadToken returned nil")
	}
	if loaded.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "test-access-token")
	}
	if loaded.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, "test-refresh-token")
	}

	if !hasToken() {
		t.Error("hasToken should be true after save")
	}
}

func TestLoadTokenEmpty(t *testing.T) {
	db.Exec("DELETE FROM oauth_token")
	token, err := loadToken()
	if err != nil {
		t.Fatalf("loadToken on empty: %v", err)
	}
	if token != nil {
		t.Error("expected nil token on empty db")
	}
}

func TestGetTodayQuota(t *testing.T) {
	used, limit := getTodayQuota()
	if used != 0 {
		t.Errorf("expected 0 used, got %d", used)
	}
	if limit != 10000 {
		t.Errorf("expected 10000 limit, got %d", limit)
	}
}

func TestRecordQuota(t *testing.T) {
	db.Exec("DELETE FROM quota_usage")

	if err := recordQuota(5); err != nil {
		t.Fatalf("recordQuota(5): %v", err)
	}

	used, _ := getTodayQuota()
	if used != 5 {
		t.Errorf("expected 5 used, got %d", used)
	}

	if err := recordQuota(10); err != nil {
		t.Fatalf("recordQuota(10): %v", err)
	}

	used, _ = getTodayQuota()
	if used != 15 {
		t.Errorf("expected 15 used, got %d", used)
	}
}

func TestRemainingQuota(t *testing.T) {
	db.Exec("DELETE FROM quota_usage")
	db.Exec("DELETE FROM settings WHERE key = 'daily_quota_limit'")

	rem := remainingQuota()
	if rem != 10000 {
		t.Errorf("expected 10000 remaining, got %d", rem)
	}

	recordQuota(9999)
	rem = remainingQuota()
	if rem != 1 {
		t.Errorf("expected 1 remaining, got %d", rem)
	}

	if quotaExpired() {
		t.Error("quotaExpired should be false with 1 remaining")
	}

	recordQuota(1)
	if !quotaExpired() {
		t.Error("quotaExpired should be true with 0 remaining")
	}
}

func TestDailyLimit(t *testing.T) {
	db.Exec("DELETE FROM settings WHERE key = 'daily_quota_limit'")

	lim := getDailyLimit()
	if lim != 10000 {
		t.Errorf("default limit = %d, want 10000", lim)
	}

	setDailyLimit(5000)
	lim = getDailyLimit()
	if lim != 5000 {
		t.Errorf("custom limit = %d, want 5000", lim)
	}

	setDailyLimit(0)
	lim = getDailyLimit()
	if lim != 10000 {
		t.Errorf("limit after setting 0 = %d, want 10000", lim)
	}
}

func TestGetClientIP(t *testing.T) {
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		ip := getClientIP(c)
		c.String(http.StatusOK, ip)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Body.String() != "192.168.1.1" {
		t.Errorf("client ip = %q, want 192.168.1.1", w.Body.String())
	}

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Body.String() != "10.0.0.1" {
		t.Errorf("forwarded ip = %q, want 10.0.0.1", w.Body.String())
	}
}

func TestEstimateShuffleCost(t *testing.T) {
	tests := []struct {
		count    int
		expected int
	}{
		{0, 51},
		{1, 101},
		{50, 2552},
		{51, 2602},
		{190, 9554},
	}

	for _, tc := range tests {
		got := estimateShuffleCost(tc.count)
		if got != tc.expected {
			t.Errorf("estimateShuffleCost(%d) = %d, want %d", tc.count, got, tc.expected)
		}
	}
}

func TestPlaylistInfo(t *testing.T) {
	p := PlaylistInfo{
		ID:        "PL123",
		Title:     "Test Playlist",
		ItemCount: 10,
	}
	if p.ID != "PL123" {
		t.Errorf("ID = %q, want PL123", p.ID)
	}
	if p.Title != "Test Playlist" {
		t.Errorf("Title = %q, want Test Playlist", p.Title)
	}
	if p.ItemCount != 10 {
		t.Errorf("ItemCount = %d, want 10", p.ItemCount)
	}
}

func TestShuffleJobJSON(t *testing.T) {
	job := &ShuffleJob{
		ID:              "job123",
		PlaylistID:      "PL123",
		PlaylistTitle:   "Test",
		Status:          "done",
		Progress:        100,
		Total:           10,
		Done:            10,
		NewPlaylistID:   "PLNEW",
		NewPlaylistTitle: "Test-randomized-May-2026",
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ShuffleJob
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Status != "done" {
		t.Errorf("Status = %q, want done", decoded.Status)
	}
}

func newTestSession() (string, *gin.Context) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	// set a deterministic IP for test sessions
	c.Request.RemoteAddr = "127.0.0.1:12345"
	sid := newSession(c)
	return sid, c
}

func TestNewSession(t *testing.T) {
	_, ctx := newTestSession()
	sid1 := newSession(ctx)
	sid2 := newSession(ctx)

	if sid1 == "" {
		t.Error("session id should not be empty")
	}
	if sid1 == sid2 {
		t.Error("consecutive sessions should have different ids")
	}
}

func TestIsValidSession(t *testing.T) {
	sid, ctx := newTestSession()
	if !isValidSession(sid, ctx) {
		t.Error("new session should be valid")
	}

	if isValidSession("nonexistent", ctx) {
		t.Error("nonexistent session should be invalid")
	}

	sessionMu.Lock()
	sessionStore[sid] = sessionInfo{Expiry: time.Now().Add(-1 * time.Hour).Unix(), IP: "127.0.0.1"}
	sessionMu.Unlock()
	if isValidSession(sid, ctx) {
		t.Error("expired session should be invalid")
	}
}

func TestIPMismatchInvalidatesSession(t *testing.T) {
	sid, ctx := newTestSession()

	otherCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	otherCtx.Request = httptest.NewRequest("GET", "/", nil)
	otherCtx.Request.RemoteAddr = "192.168.1.1:54321"

	if isValidSession(sid, otherCtx) {
		t.Error("session should be invalid from different IP")
	}

	if !isValidSession(sid, ctx) {
		t.Error("session should still be valid from original IP")
	}
}

func TestNewSessionFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	req.RemoteAddr = "10.0.0.1:9999"

	sid := newSession(c)
	if sid == "" {
		t.Fatal("session id empty")
	}

	if !isValidSession(sid, c) {
		t.Error("new session from request should be valid")
	}
}

func TestAuthMiddleware(t *testing.T) {
	sid, _ := newTestSession()

	tests := []struct {
		name       string
		addCookie  bool
		cookieVal  string
		wantStatus int
	}{
		{"no cookie", false, "", http.StatusFound},
		{"invalid cookie", true, "bad_session", http.StatusFound},
		{"valid cookie", true, sid, http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/test", authMiddleware(), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tc.addCookie {
				val := tc.cookieVal
				req.AddCookie(&http.Cookie{Name: "ypr_session", Value: val})
			}
			req.RemoteAddr = "127.0.0.1:12345"

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

func TestCSRFMiddleware(t *testing.T) {
	csrfTests := []struct {
		name       string
		method     string
		origin     string
		referer    string
		wantStatus int
	}{
		{"GET passes", "GET", "", "", http.StatusOK},
		{"HEAD passes", "HEAD", "", "", http.StatusOK},
		{"OPTIONS passes", "OPTIONS", "", "", http.StatusOK},
		{"POST without origin or referer blocked", "POST", "", "", http.StatusForbidden},
		{"POST with origin passes", "POST", "http://localhost", "", http.StatusOK},
		{"POST with referer passes", "POST", "", "http://localhost/test", http.StatusOK},
	}

	for _, tc := range csrfTests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Any("/test", csrfMiddleware(), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(tc.method, "/test", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

func TestHandleShuffleStatus(t *testing.T) {
	r := gin.New()
	r.GET("/shuffle/:jobId/status", handleShuffleStatus)

	req := httptest.NewRequest("GET", "/shuffle/nonexistent/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("non-existent job: status = %d, want %d", w.Code, http.StatusNotFound)
	}

	job := &ShuffleJob{ID: "testjob", Status: "running", Progress: 50}
	jobsMu.Lock()
	jobs["testjob"] = job
	jobsMu.Unlock()

	req = httptest.NewRequest("GET", "/shuffle/testjob/status", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("existing job: status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "running" {
		t.Errorf("status = %v, want running", resp["status"])
	}
}

func TestHandleCancelShuffle(t *testing.T) {
	r := gin.New()
	r.DELETE("/shuffle/:jobId", handleCancelShuffle)

	req := httptest.NewRequest("DELETE", "/shuffle/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("non-existent job: status = %d, want %d", w.Code, http.StatusNotFound)
	}

	job := &ShuffleJob{
		ID:     "canceljob",
		Status: "inserting",
		Cancel: make(chan struct{}),
	}
	jobsMu.Lock()
	jobs["canceljob"] = job
	jobsMu.Unlock()

	req = httptest.NewRequest("DELETE", "/shuffle/canceljob", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("cancel job: status = %d, want %d", w.Code, http.StatusOK)
	}

	select {
	case <-job.Cancel:
	default:
		t.Error("Cancel channel should be closed after cancel")
	}

	// can't cancel an already cancelled job
	req = httptest.NewRequest("DELETE", "/shuffle/canceljob", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("double cancel: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleShuffleNoID(t *testing.T) {
	r := gin.New()
	r.POST("/playlist/:id/shuffle", handleShuffle)

	req := httptest.NewRequest("POST", "/playlist//shuffle", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleShuffleNoAuth(t *testing.T) {
	r := gin.New()
	r.POST("/playlist/:id/shuffle", handleShuffle)

	req := httptest.NewRequest("POST", "/playlist/PL123/shuffle",
		strings.NewReader(`{"title":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestGetEnv(t *testing.T) {
	os.Setenv("YPR_TEST", "hello")
	defer os.Unsetenv("YPR_TEST")

	if got := getEnv("YPR_TEST", "default"); got != "hello" {
		t.Errorf("getEnv with env = %q, want hello", got)
	}
	if got := getEnv("YPR_NONEXISTENT", "default"); got != "default" {
		t.Errorf("getEnv without env = %q, want default", got)
	}
}

func TestQuotaExpired(t *testing.T) {
	db.Exec("DELETE FROM quota_usage")
	db.Exec("DELETE FROM settings WHERE key = 'daily_quota_limit'")

	recordQuota(10001)
	if !quotaExpired() {
		t.Error("quotaExpired should be true after 10001 units used")
	}
}

func TestSetDailyLimitTwice(t *testing.T) {
	db.Exec("DELETE FROM settings WHERE key = 'daily_quota_limit'")

	setDailyLimit(3000)
	if getDailyLimit() != 3000 {
		t.Errorf("first set: got %d, want 3000", getDailyLimit())
	}

	setDailyLimit(7500)
	if getDailyLimit() != 7500 {
		t.Errorf("second set: got %d, want 7500", getDailyLimit())
	}
}

func TestGetQuotaHistory(t *testing.T) {
	db.Exec("DELETE FROM quota_usage")
	db.Exec("DELETE FROM settings WHERE key = 'daily_quota_limit'")

	// seed some quota records
	for i := 1; i <= 3; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		db.Exec("INSERT INTO quota_usage (date, used, daily_limit) VALUES (?, ?, ?)",
			date, i*100, 10000)
	}

	history, err := getQuotaHistory(7)
	if err != nil {
		t.Fatalf("getQuotaHistory: %v", err)
	}

	if len(history) < 1 {
		t.Fatal("expected at least 1 history entry")
	}
}

func TestOAuthStateFlow(t *testing.T) {
	oauthStatesMu.Lock()
	for k := range oauthStates {
		delete(oauthStates, k)
	}
	oauthStatesMu.Unlock()

	stateBytes := make([]byte, 32)
	state := hex.EncodeToString(stateBytes)

	oauthStatesMu.Lock()
	oauthStates[state] = true
	oauthStatesMu.Unlock()

	oauthStatesMu.Lock()
	pending := oauthStates[state]
	oauthStatesMu.Unlock()
	if !pending {
		t.Error("state should be pending after adding")
	}

	oauthStatesMu.Lock()
	pending = oauthStates[state]
	delete(oauthStates, state)
	oauthStatesMu.Unlock()
	if !pending {
		t.Error("state should be pending before deletion")
	}

	if oauthStates[state] {
		t.Error("state should be removed after consuming")
	}
}

func TestCheckAborted(t *testing.T) {
	job := &ShuffleJob{
		ID:     "aborttest",
		Status: "running",
		Cancel: make(chan struct{}),
	}

	if checkAborted(job) {
		t.Error("should not be aborted when Cancel is open")
	}

	close(job.Cancel)

	if !checkAborted(job) {
		t.Error("should be aborted when Cancel is closed")
	}

	if job.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", job.Status)
	}
}

func TestSecureCookiesDefault(t *testing.T) {
	if secureCookies {
		t.Error("secureCookies should be false by default in tests")
	}
}

func TestSessionInfo(t *testing.T) {
	info := sessionInfo{
		Expiry: 1234567890,
		IP:     "10.0.0.1",
	}

	if info.Expiry != 1234567890 {
		t.Errorf("Expiry = %d, want 1234567890", info.Expiry)
	}
	if info.IP != "10.0.0.1" {
		t.Errorf("IP = %s, want 10.0.0.1", info.IP)
	}
}

func TestQuotaDay(t *testing.T) {
	d := QuotaDay{
		Date:       "2026-05-09",
		Used:       500,
		DailyLimit: 10000,
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal QuotaDay: %v", err)
	}

	var decoded QuotaDay
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal QuotaDay: %v", err)
	}
	if decoded.Used != 500 {
		t.Errorf("Used = %d, want 500", decoded.Used)
	}
}

func TestHandleQuotaHistory(t *testing.T) {
	r := gin.New()
	r.GET("/quota/history", handleQuotaHistory)

	req := httptest.NewRequest("GET", "/quota/history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp []QuotaDay
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestCompleteShuffleJobFlow(t *testing.T) {
	job := &ShuffleJob{
		ID:            "completeflow",
		PlaylistID:    "PLTEST",
		PlaylistTitle: "Test",
		Status:        "pending",
		Cancel:        make(chan struct{}),
	}

	if job.Status != "pending" {
		t.Errorf("initial status = %q, want pending", job.Status)
	}

	job.Status = "fetching"
	if job.Status != "fetching" {
		t.Errorf("status = %q, want fetching", job.Status)
	}

	// simulate all required fields set
	_ = job.ID
	_ = job.PlaylistID
	_ = job.PlaylistTitle
	_ = job.Cancel
}
