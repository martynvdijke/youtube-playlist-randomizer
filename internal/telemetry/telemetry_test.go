package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel/trace"
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

func newTestTelemetry(t *testing.T) *Telemetry {
	t.Helper()
	tel, err := New(DefaultSettings())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	return tel
}

func TestNewValidatesAllInstruments(t *testing.T) {
	tel := newTestTelemetry(t)
	defer tel.Shutdown(context.Background())

	instruments := []struct {
		name string
		val  any
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
	tel := newTestTelemetry(t)
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
	tel := newTestTelemetry(t)
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
	tel := newTestTelemetry(t)
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
	tel := newTestTelemetry(t)
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
	tel := newTestTelemetry(t)
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
	tel := newTestTelemetry(t)
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
	tel := newTestTelemetry(t)
	defer tel.Shutdown(context.Background())

	// Should not panic
	tel.RecordQuotaMetrics(context.Background(), 100, 10000, 9900)
}

func TestRecordYouTubeAPICall(t *testing.T) {
	tel := newTestTelemetry(t)
	defer tel.Shutdown(context.Background())

	tel.RecordYouTubeAPICall(context.Background(), "playlists.list")
	tel.RecordYouTubeAPICall(context.Background(), "playlistItems.insert")
}

func TestRecordJobMetrics(t *testing.T) {
	tel := newTestTelemetry(t)
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
	tel := newTestTelemetry(t)
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

func TestTraceDBQuery(t *testing.T) {
	tel := newTestTelemetry(t)
	defer tel.Shutdown(context.Background())

	called := false
	err := TraceDBQuery(context.Background(), tel.Tracer, "TestOp", func(ctx context.Context) error {
		called = true
		_ = trace.SpanFromContext(ctx)
		return nil
	})
	if err != nil {
		t.Errorf("TraceDBQuery returned error: %v", err)
	}
	if !called {
		t.Error("callback was not called")
	}
}

func TestTraceDBQuery_NilTracer(t *testing.T) {
	called := false
	err := TraceDBQuery(context.Background(), nil, "TestOp", func(ctx context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("TraceDBQuery with nil tracer returned error: %v", err)
	}
	if !called {
		t.Error("callback was not called")
	}
}

func TestTraceDBQuery_ErrorPropagation(t *testing.T) {
	tel := newTestTelemetry(t)
	defer tel.Shutdown(context.Background())

	expected := fmt.Errorf("db error")
	err := TraceDBQuery(context.Background(), tel.Tracer, "TestOp", func(ctx context.Context) error {
		return expected
	})
	if err != expected {
		t.Errorf("expected error %v, got %v", expected, err)
	}
}

func TestNewSampler_AlwaysOn(t *testing.T) {
	os.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	defer os.Unsetenv("OTEL_TRACES_SAMPLER")

	cfg := DefaultSettings()
	cfg.TracesEnabled = true
	tel, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	tel.Shutdown(context.Background())
}

func TestParseHeadersJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "valid json",
			input: `{"Authorization":"Bearer test123","X-Custom":"value"}`,
			want:  map[string]string{"Authorization": "Bearer test123", "X-Custom": "value"},
		},
		{
			name:  "invalid json",
			input: `{bad`,
			want:  nil,
		},
		{
			name:  "empty object",
			input: `{}`,
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHeadersJSON(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("ParseHeadersJSON() len = %d, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("ParseHeadersJSON()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestNewResourceWithAttributes(t *testing.T) {
	os.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment=staging,service.namespace=myapp")
	defer os.Unsetenv("OTEL_RESOURCE_ATTRIBUTES")

	res, err := newResource("test-service")
	if err != nil {
		t.Fatalf("newResource() failed: %v", err)
	}
	if res == nil {
		t.Fatal("newResource() returned nil")
	}
}

func TestNew_NoExportTarget(t *testing.T) {
	cfg := Settings{
		TracesEnabled:   true,
		MetricsEnabled:  true,
		LogsEnabled:     true,
		TraceSampleRate: 1.0,
	}
	tel, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with no endpoint failed: %v", err)
	}
	defer tel.Shutdown(context.Background())

	if tel.TracerProvider == nil {
		t.Error("TracerProvider is nil")
	}
	if tel.MeterProvider == nil {
		t.Error("MeterProvider is nil")
	}
	if tel.LoggerProvider == nil {
		t.Error("LoggerProvider is nil")
	}
	if tel.Tracer == nil {
		t.Error("Tracer is nil")
	}
	if tel.Meter == nil {
		t.Error("Meter is nil")
	}
}

func TestTraceDBQuery_SpanName(t *testing.T) {
	tel := newTestTelemetry(t)
	defer tel.Shutdown(context.Background())

	err := TraceDBQuery(context.Background(), tel.Tracer, "GetItems", func(ctx context.Context) error {
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().IsValid() {
			t.Log("span created successfully")
		}
		return nil
	})
	if err != nil {
		t.Errorf("TraceDBQuery failed: %v", err)
	}
}
