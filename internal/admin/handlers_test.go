package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Failed to open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	logger := logging.New(logging.LogOptions{
		Store:    s,
		MinLevel: logging.WARN,
		Service:  "test",
	})

	return New(s, logger)
}

func TestHandleSettingsGotify_Get(t *testing.T) {
	h := newTestHandlers(t)

	// Set some initial settings
	if err := h.store.SetSetting("gotify_url", "http://gotify:8080"); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}
	if err := h.store.SetSetting("gotify_token", "myapptoken"); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}
	if err := h.store.SetSetting("gotify_enabled", "true"); err != nil {
		t.Fatalf("SetSetting failed: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/admin/settings/gotify", nil)
	h.HandleSettingsGotify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var result gotifySettings
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.URL != "http://gotify:8080" {
		t.Errorf("expected URL 'http://gotify:8080', got '%s'", result.URL)
	}
	if result.Token != "myapptoken" {
		t.Errorf("expected Token 'myapptoken', got '%s'", result.Token)
	}
	if result.Enabled != "true" {
		t.Errorf("expected Enabled 'true', got '%s'", result.Enabled)
	}
}

func TestHandleSettingsGotify_GetDefaults(t *testing.T) {
	h := newTestHandlers(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/admin/settings/gotify", nil)
	h.HandleSettingsGotify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var result gotifySettings
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.URL != "" {
		t.Errorf("expected empty URL, got '%s'", result.URL)
	}
	if result.Token != "" {
		t.Errorf("expected empty Token, got '%s'", result.Token)
	}
	if result.Enabled != "" {
		t.Errorf("expected empty Enabled, got '%s'", result.Enabled)
	}
}

func TestHandleSettingsGotify_Post(t *testing.T) {
	h := newTestHandlers(t)

	body := "url=http://gotify:8080&token=apptoken&enabled=true"
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/admin/settings/gotify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.HandleSettingsGotify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: body=%s", rr.Code, rr.Body.String())
	}

	// Verify settings were saved
	got, _ := h.store.GetSetting("gotify_url")
	if got != "http://gotify:8080" {
		t.Errorf("expected gotify_url 'http://gotify:8080', got '%s'", got)
	}
	got, _ = h.store.GetSetting("gotify_token")
	if got != "apptoken" {
		t.Errorf("expected gotify_token 'apptoken', got '%s'", got)
	}
	got, _ = h.store.GetSetting("gotify_enabled")
	if got != "true" {
		t.Errorf("expected gotify_enabled 'true', got '%s'", got)
	}
}

func TestHandleSettingsGotify_PostInvalidURL(t *testing.T) {
	h := newTestHandlers(t)

	body := "url=ftp://bad&token=token&enabled=true"
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/admin/settings/gotify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.HandleSettingsGotify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleSettingsGotify_PostDisabled(t *testing.T) {
	h := newTestHandlers(t)

	// When checkbox is unchecked, enabled form value is empty
	body := "url=http://gotify:8080&token=token"
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/admin/settings/gotify", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.HandleSettingsGotify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	got, _ := h.store.GetSetting("gotify_enabled")
	if got != "false" {
		t.Errorf("expected gotify_enabled 'false', got '%s'", got)
	}
}

func TestHandleSettingsGotify_MethodNotAllowed(t *testing.T) {
	h := newTestHandlers(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/admin/settings/gotify", nil)
	h.HandleSettingsGotify(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rr.Code)
	}
}

func TestHandleSettingsGotifyHTML(t *testing.T) {
	h := newTestHandlers(t)

	// Set some settings to verify they appear in the form
	h.store.SetSetting("gotify_url", "http://gotify:8080")
	h.store.SetSetting("gotify_token", "secret")
	h.store.SetSetting("gotify_enabled", "true")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/admin/settings/gotify/html", nil)
	h.HandleSettingsGotifyHTML(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Gotify Notifications") {
		t.Errorf("expected 'Gotify Notifications' in body, got: %s", body)
	}
	if !strings.Contains(body, "http://gotify:8080") {
		t.Errorf("expected URL in form, got: %s", body)
	}
	if !strings.Contains(body, `checked`) {
		t.Errorf("expected checkbox to be checked, got: %s", body)
	}
}

func TestHandleSettingsGotifyHTML_MethodNotAllowed(t *testing.T) {
	h := newTestHandlers(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/admin/settings/gotify/html", nil)
	h.HandleSettingsGotifyHTML(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rr.Code)
	}
}
