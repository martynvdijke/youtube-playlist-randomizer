package youtube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
)

func TestIsQuotaError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"random error", fmt.Errorf("network error"), false},
		{"quotaExceeded 403", &googleapi.Error{
			Code: 403,
			Errors: []googleapi.ErrorItem{
				{Reason: "quotaExceeded", Message: "Quota exceeded"},
			},
		}, true},
		{"rateLimitExceeded 403", &googleapi.Error{
			Code: 403,
			Errors: []googleapi.ErrorItem{
				{Reason: "rateLimitExceeded", Message: "Rate limit exceeded"},
			},
		}, true},
		{"dailyLimitExceeded 403", &googleapi.Error{
			Code: 403,
			Errors: []googleapi.ErrorItem{
				{Reason: "dailyLimitExceeded", Message: "Daily limit exceeded"},
			},
		}, true},
		{"quotaExceeded 429", &googleapi.Error{
			Code: 429,
			Errors: []googleapi.ErrorItem{
				{Reason: "quotaExceeded", Message: "Quota exceeded"},
			},
		}, true},
		{"notFound 404", &googleapi.Error{
			Code: 404,
			Errors: []googleapi.ErrorItem{
				{Reason: "notFound", Message: "Not found"},
			},
		}, false},
		{"badRequest 400", &googleapi.Error{
			Code: 400,
			Errors: []googleapi.ErrorItem{
				{Reason: "badRequest", Message: "Bad request"},
			},
		}, false},
		{"quotaExceeded via body", &googleapi.Error{
			Code: 403,
			Body: `{"error":{"errors":[{"reason":"quotaExceeded"}]}}`,
		}, true},
		{"dailyLimitExceeded via body", &googleapi.Error{
			Code: 403,
			Body: `{"error":{"message":"Daily Limit Exceeded"}}`,
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsQuotaError(tc.err)
			if got != tc.want {
				t.Errorf("IsQuotaError() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "client_secret.json")
	if err := os.WriteFile(secretPath, []byte(`{"web":{"client_id":"x","client_secret":"y","project_id":"p","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["https://example.com/callback"]}}`), 0600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	os.Setenv("OAUTH_CALLBACK_URL", "https://example.com/callback")
	defer os.Unsetenv("OAUTH_CALLBACK_URL")

	setup, err := LoadConfig(secretPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if setup.Config == nil {
		t.Fatal("expected non-nil config")
	}
	if setup.SecretDir != dir {
		t.Errorf("expected SecretDir %q, got %q", dir, setup.SecretDir)
	}
	if setup.Config.RedirectURL != "https://example.com/callback" {
		t.Errorf("expected RedirectURL from env, got %q", setup.Config.RedirectURL)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/client_secret.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_NoCallbackURL(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "client_secret.json")
	if err := os.WriteFile(secretPath, []byte(`{"web":{"client_id":"x","client_secret":"y","project_id":"p","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost:6270/callback"]}}`), 0600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	os.Unsetenv("OAUTH_CALLBACK_URL")
	defer os.Setenv("OAUTH_CALLBACK_URL", os.Getenv("OAUTH_CALLBACK_URL"))

	setup, err := LoadConfig(secretPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if setup.Config.RedirectURL == "" {
		t.Error("expected non-empty RedirectURL even without env")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "bad_secret.json")
	if err := os.WriteFile(secretPath, []byte(`not json`), 0600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	_, err := LoadConfig(secretPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAuthURL(t *testing.T) {
	setup := &OAuthSetup{
		Config: &oauth2.Config{
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
			RedirectURL: "https://example.com/callback",
			Scopes:      []string{"https://www.googleapis.com/auth/youtube.force-ssl"},
		},
	}

	url := AuthURL(setup)
	if !strings.Contains(url, "client_id=test-id") {
		t.Errorf("expected client_id in URL, got %s", url)
	}
	if !strings.Contains(url, "redirect_uri=https%3A%2F%2Fexample.com%2Fcallback") {
		t.Errorf("expected redirect_uri in URL, got %s", url)
	}
	if !strings.Contains(url, "access_type=offline") {
		t.Errorf("expected access_type=offline in URL, got %s", url)
	}
	if !strings.Contains(url, "prompt=consent") {
		t.Errorf("expected prompt=consent in URL, got %s", url)
	}
}

func TestTokenFromFile_Missing(t *testing.T) {
	_, err := tokenFromFile("/nonexistent/token.json")
	if err == nil {
		t.Fatal("expected error for missing token file")
	}
}

func TestSaveAndReadToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	tok := &oauth2.Token{AccessToken: "test-access", TokenType: "Bearer", RefreshToken: "test-refresh"}
	if err := saveToken(path, tok); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	read, err := tokenFromFile(path)
	if err != nil {
		t.Fatalf("tokenFromFile failed: %v", err)
	}
	if read.AccessToken != "test-access" {
		t.Errorf("expected AccessToken 'test-access', got '%s'", read.AccessToken)
	}
	if read.RefreshToken != "test-refresh" {
		t.Errorf("expected RefreshToken 'test-refresh', got '%s'", read.RefreshToken)
	}
}

func TestNewClient_MissingSecret(t *testing.T) {
	client, err := NewClient(context.Background(), "/nonexistent/secret.json", t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected error for missing secret file")
	}
	if client != nil {
		t.Fatal("expected nil client")
	}
}

func TestNewClient_NoTokenFile(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "client_secret.json")
	if err := os.WriteFile(secretPath, []byte(`{"web":{"client_id":"x","client_secret":"y","project_id":"p","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost:6270/callback"]}}`), 0600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	client, err := NewClient(context.Background(), secretPath, dir, nil)
	if err != nil {
		t.Fatalf("NewClient returned error (expected nil): %v", err)
	}
	if client != nil {
		t.Fatal("expected nil client when no token file exists")
	}
}

func TestNewClient_StaleToken(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	if err := saveToken(tokenPath, &oauth2.Token{AccessToken: "stale-token", TokenType: "Bearer"}); err != nil {
		t.Fatalf("save stale token: %v", err)
	}

	secretPath := filepath.Join(dir, "client_secret.json")
	if err := os.WriteFile(secretPath, []byte(`{"web":{"client_id":"x","client_secret":"y","project_id":"p","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost:6270/callback"]}}`), 0600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	client, err := NewClient(context.Background(), secretPath, dir, nil)
	if err != nil {
		t.Fatalf("NewClient returned error (expected nil): %v", err)
	}
	if client != nil {
		t.Fatal("expected nil client for stale token with no refresh")
	}

	if _, err := os.Stat(tokenPath); err == nil {
		t.Error("stale token file should have been deleted")
	}
}

func TestNewClient_EmptyTokenDir(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "client_secret.json")
	if err := os.WriteFile(secretPath, []byte(`{"web":{"client_id":"x","client_secret":"y","project_id":"p","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost:6270/callback"]}}`), 0600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	client, err := NewClient(context.Background(), secretPath, "", nil)
	if err != nil {
		t.Fatalf("NewClient with empty tokenDir returned error: %v", err)
	}
	if client != nil {
		t.Fatal("expected nil client (no token, empty dir should default to secret dir)")
	}
}

func TestHandleCallback_Fail(t *testing.T) {
	setup := &OAuthSetup{
		Config: &oauth2.Config{
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
			RedirectURL: "http://localhost:6270/callback",
		},
		SecretDir: t.TempDir(),
	}

	err := HandleCallback(setup, "bad-code")
	if err == nil {
		t.Fatal("expected error for bad OAuth code")
	}
}

func TestHandleCallback_NoExtraDirs(t *testing.T) {
	dir := t.TempDir()
	setup := &OAuthSetup{
		Config: &oauth2.Config{
			ClientID:     "test-id",
			ClientSecret: "test-secret",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
			RedirectURL: "http://localhost:6270/callback",
		},
		SecretDir: dir,
	}

	err := HandleCallback(setup, "bad-code")
	if err == nil {
		t.Fatal("expected error for bad code")
	}

	files, _ := filepath.Glob(filepath.Join(dir, "token*"))
	if len(files) > 0 {
		t.Error("no token file should exist after failed callback")
	}
}

func TestSaveToken_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	tok := &oauth2.Token{AccessToken: "new-token", TokenType: "Bearer"}
	if err := saveToken(path, tok); err != nil {
		t.Fatalf("saveToken failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if !strings.Contains(string(data), "new-token") {
		t.Errorf("expected token content, got %s", string(data))
	}
}

func TestTokenFromFile_Corrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	if err := os.WriteFile(path, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("write corrupted token: %v", err)
	}

	_, err := tokenFromFile(path)
	if err == nil {
		t.Fatal("expected error for corrupted token file")
	}
}
