package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

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

func TestQuotaCostClass(t *testing.T) {
	sufficient := &store.QuotaInfo{Remaining: 100}
	insufficient := &store.QuotaInfo{Remaining: 5}

	tests := []struct {
		name   string
		quota  *store.QuotaInfo
		cost   int
		want   string
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
		name   string
		quota  *store.QuotaInfo
		cost   int
		want   string
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
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestFindClientSecret(t *testing.T) {
	t.Run("finds existing file", func(t *testing.T) {
		path := findClientSecret()
		if path == "" {
			t.Error("expected non-empty path")
		}
	})
}

func TestJobStatusConstants(t *testing.T) {
	expected := map[JobStatus]bool{
		JobPending:   true,
		JobFetching:  true,
		JobShuffling: true,
		JobInserting: true,
		JobDone:      true,
		JobError:     true,
		JobPaused:    true,
	}
	for status := range expected {
		if _, ok := expected[status]; !ok {
			t.Errorf("unexpected job status: %s", status)
		}
	}
}

func TestHandleOAuthCallback_NoCode(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/callback", nil)
	handleOAuthCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Missing") {
		t.Errorf("expected error message about missing code, got %s", rr.Body.String())
	}
}

func TestHandleOAuthCallback_NoSetup(t *testing.T) {
	oldSetup := oauthSetup
	oauthSetup = nil
	defer func() { oauthSetup = oldSetup }()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/callback?code=somecode", nil)
	handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestHandleAuth_NoSetup(t *testing.T) {
	oldSetup := oauthSetup
	oauthSetup = nil
	defer func() { oauthSetup = oldSetup }()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/auth", nil)
	handleAuth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "OAuth not configured") {
		t.Errorf("expected 'OAuth not configured' message, got: %s", rr.Body.String())
	}
}

func TestHandleForceResume_BadMethod(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/resume", nil)
	handleForceResume(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestHandleForceResume_NoJobID(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/resume", nil)
	handleForceResume(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Missing") {
		t.Errorf("expected 'Missing job ID', got %s", rr.Body.String())
	}
}

func TestHandleForceResume_YTClientNil(t *testing.T) {
	oldClient := ytClient
	ytClient = nil
	defer func() { ytClient = oldClient }()

	if db == nil {
		t.Skip("no database available for this test")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/resume", strings.NewReader("jobId=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleForceResume(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

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
	j := JobResponse{JobID: "test-123", Status: JobPending}
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

func TestHandleJobQueueHTML_BadMethod(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/jobs/queue/html", nil)
	handleJobQueueHTML(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

func TestHandleJobQueueHTML_EmptyQueue(t *testing.T) {
	oldDB := db
	defer func() { db = oldDB }()

	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	db = s

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/queue/html", nil)
	handleJobQueueHTML(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `class="job-queue hidden"`) {
		t.Errorf("expected hidden job-queue for empty result, got: %s", rr.Body.String())
	}
}

func TestHandleJobQueueHTML_WithPausedJob(t *testing.T) {
	oldDB := db
	defer func() { db = oldDB }()

	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	db = s

	if err := db.CreateJob("test-job-1", "PL_SRC", "Source Title", "My Playlist"); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := db.SetJobPaused("test-job-1"); err != nil {
		t.Fatalf("pause job: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/jobs/queue/html", nil)
	handleJobQueueHTML(rr, req)

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


