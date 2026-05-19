package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestServiceNameDefault(t *testing.T) {
	os.Unsetenv("OTEL_SERVICE_NAME")
	if got := serviceName(); got != "youtube-playlist-randomizer" {
		t.Errorf("serviceName() = %q, want %q", got, "youtube-playlist-randomizer")
	}
}

func TestServiceNameEnvVar(t *testing.T) {
	os.Setenv("OTEL_SERVICE_NAME", "custom-name")
	defer os.Unsetenv("OTEL_SERVICE_NAME")
	if got := serviceName(); got != "custom-name" {
		t.Errorf("serviceName() = %q, want %q", got, "custom-name")
	}
}

func TestNewValidatesAllInstruments(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer tel.Shutdown(context.Background())

	instruments := []struct {
		name string
		val  interface{}
	}{
		{"HTTPRequestCount", tel.HTTPRequestCount},
		{"HTTPRequestDur", tel.HTTPRequestDur},
		{"HTTPRequestsInFly", tel.HTTPRequestsInFly},
		{"QuotaUsed", tel.QuotaUsed},
		{"QuotaRemaining", tel.QuotaRemaining},
		{"QuotaLimit", tel.QuotaLimit},
		{"JobsCreated", tel.JobsCreated},
		{"JobsCompleted", tel.JobsCompleted},
		{"JobsPaused", tel.JobsPaused},
		{"JobsFailed", tel.JobsFailed},
		{"ItemsInserted", tel.ItemsInserted},
		{"YouTubeAPICalls", tel.YouTubeAPICalls},
	}
	for _, inst := range instruments {
		if inst.val == nil {
			t.Errorf("instrument %q is nil", inst.name)
		}
	}
}

func TestNew(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if tel == nil {
		t.Fatal("New() returned nil")
	}
	if tel.Tracer == nil {
		t.Error("Tracer is nil")
	}
	if tel.Meter == nil {
		t.Error("Meter is nil")
	}
	if tel.HTTPRequestCount == nil {
		t.Error("HTTPRequestCount instrument is nil")
	}
	if tel.QuotaUsed == nil {
		t.Error("QuotaUsed instrument is nil")
	}
	if tel.JobsCreated == nil {
		t.Error("JobsCreated instrument is nil")
	}
	if tel.YouTubeAPICalls == nil {
		t.Error("YouTubeAPICalls instrument is nil")
	}
	tel.Shutdown(context.Background())
}

func TestMiddleware(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	handler := tel.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if rr.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", rr.Body.String())
	}
}

func TestMiddlewareTracksStatusCode(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	handler := tel.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))

	req := httptest.NewRequest("GET", "/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rr.Code)
	}
}

func TestMiddlewareWithHostPort(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	var recordedHost string
	handler := tel.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "myserver:6270"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if recordedHost != "myserver:6270" {
		t.Errorf("expected host 'myserver:6270', got %q", recordedHost)
	}
}

func TestMiddlewareWithHostOnly(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	var recordedHost string
	handler := tel.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = "myserver"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if recordedHost != "myserver" {
		t.Errorf("expected host 'myserver', got %q", recordedHost)
	}
}

func TestMiddlewareEmptyHost(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	handler := tel.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Host = ""
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestRecordQuotaMetrics(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	// Should not panic
	tel.RecordQuotaMetrics(context.Background(), 100, 10000, 9900)
}

func TestRecordYouTubeAPICall(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	tel.RecordYouTubeAPICall(context.Background(), "playlists.list")
	tel.RecordYouTubeAPICall(context.Background(), "playlistItems.insert")
}

func TestRecordJobMetrics(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	tel.RecordJobCreated(context.Background())
	tel.RecordJobCompleted(context.Background(), 50)
	tel.RecordJobPaused(context.Background(), 25, 50)
	tel.RecordJobFailed(context.Background(), "quota exhausted")
	tel.RecordItemsInserted(context.Background(), 10)
}

func TestTelemetryNilSafety(t *testing.T) {
	// The otel field in Client is nil-safe; verify the methods handle nil
	var nilTel *Telemetry = nil

	// These should not panic
	assertNotPanics(t, func() {
		nilTel.RecordQuotaMetrics(context.Background(), 0, 0, 0)
	})
	assertNotPanics(t, func() {
		nilTel.RecordYouTubeAPICall(context.Background(), "test")
	})
	assertNotPanics(t, func() {
		nilTel.RecordJobCreated(context.Background())
	})
	assertNotPanics(t, func() {
		nilTel.RecordJobCompleted(context.Background(), 0)
	})
	assertNotPanics(t, func() {
		nilTel.RecordJobPaused(context.Background(), 0, 0)
	})
	assertNotPanics(t, func() {
		nilTel.RecordJobFailed(context.Background(), "reason")
	})
	assertNotPanics(t, func() {
		nilTel.RecordItemsInserted(context.Background(), 0)
	})
	assertNotPanics(t, func() {
		nilTel.Shutdown(context.Background())
	})
}

func TestGetEnvWithDefault(t *testing.T) {
	if v := GetEnvWithDefault("NONEXISTENT_VAR_12345", "default"); v != "default" {
		t.Errorf("expected 'default', got %q", v)
	}
}

func assertNotPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()
	fn()
}

func TestMiddlewarePanicSafety(t *testing.T) {
	tel, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Shutdown(context.Background())

	// Test with nil Telemetry - should not panic
	var nilTel *Telemetry = nil
	handler := nilTel.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/nil-test", nil)
	rr := httptest.NewRecorder()

	assertNotPanics(t, func() {
		handler.ServeHTTP(rr, req)
	})

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}
