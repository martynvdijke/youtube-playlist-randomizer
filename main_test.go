package main

import (
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
	// initDB already called in TestMain, verify tables exist
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

func TestEstimateShuffleCost(t *testing.T) {
	tests := []struct {
		count    int
		expected int
	}{
		{0, 51},   // 0/50+1=1 page(1) + create(50) + 0 items(0) = 51
		{1, 101},  // 1/50+1=1 page(1) + create(50) + 1 item(50) = 101
		{50, 2552}, // 50/50+1=2 pages(2) + create(50) + 50 items(2500) = 2552 (conservative overestimate)
		{51, 2602}, // 51/50+1=2 pages(2) + create(50) + 51 items(2550) = 2602
		{190, 9554}, // 190/50+1=4 pages(4) + create(50) + 190 items(9500) = 9554
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
		ID:             "job123",
		PlaylistID:     "PL123",
		PlaylistTitle:  "Test",
		Status:         "done",
		Progress:       100,
		Total:          10,
		Done:           10,
		NewPlaylistID:  "PLNEW",
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

func TestNewSession(t *testing.T) {
	sid1 := newSession()
	sid2 := newSession()

	if sid1 == "" {
		t.Error("session id should not be empty")
	}
	if sid1 == sid2 {
		t.Error("consecutive sessions should have different ids")
	}
}

func TestIsValidSession(t *testing.T) {
	sid := newSession()
	if !isValidSession(sid) {
		t.Error("new session should be valid")
	}

	if isValidSession("nonexistent") {
		t.Error("nonexistent session should be invalid")
	}

	sessionMu.Lock()
	sessionStore[sid] = time.Now().Add(-1 * time.Hour).Unix()
	sessionMu.Unlock()
	if isValidSession(sid) {
		t.Error("expired session should be invalid")
	}
}

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		setCookie   bool
		cookieValue string
		wantStatus  int
	}{
		{"no cookie", false, "", http.StatusFound},
		{"invalid cookie", true, "bad_session", http.StatusFound},
		{"valid cookie", true, "", http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/test", authMiddleware(), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tc.setCookie {
				val := tc.cookieValue
				if val == "" {
					val = newSession()
				}
				req.AddCookie(&http.Cookie{
					Name:  "ypr_session",
					Value: val,
				})
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

	// non-existent job
	req := httptest.NewRequest("GET", "/shuffle/nonexistent/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("non-existent job: status = %d, want %d", w.Code, http.StatusNotFound)
	}

	// existing job
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

	// set today's usage to exceed limit
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
