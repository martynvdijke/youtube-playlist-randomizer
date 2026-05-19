package youtube

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGetTokenViaLocalServer_UsesCallbackURLEnv(t *testing.T) {
	os.Setenv("OAUTH_CALLBACK_URL", "http://test.example.com:9999/callback")
	defer os.Unsetenv("OAUTH_CALLBACK_URL")

	config := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL: "https://example.com/token",
		},
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := getTokenViaLocalServer(config, 9999)
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:9999/callback?code=test")
	if err != nil {
		t.Fatalf("callback server not reachable: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected OAuth exchange error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for getTokenViaLocalServer to return")
	}

	if config.RedirectURL != "http://test.example.com:9999/callback" {
		t.Errorf("RedirectURL = %q, want %q", config.RedirectURL, "http://test.example.com:9999/callback")
	}
}

func TestGetTokenViaLocalServer_DefaultsToLocalhost(t *testing.T) {
	os.Unsetenv("OAUTH_CALLBACK_URL")

	config := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL: "https://example.com/token",
		},
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := getTokenViaLocalServer(config, 9998)
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:9998/callback?code=test")
	if err != nil {
		t.Fatalf("callback server not reachable: %v", err)
	}
	resp.Body.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected OAuth exchange error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for getTokenViaLocalServer to return")
	}

	if !strings.Contains(config.RedirectURL, "localhost") {
		t.Errorf("RedirectURL = %q, want a localhost-based URL", config.RedirectURL)
	}
}
