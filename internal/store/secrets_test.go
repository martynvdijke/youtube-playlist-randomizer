package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSecretSettingEncryptedAtRest(t *testing.T) {
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "test-key")
	s := openTestStore(t)

	if err := s.SetSecretSetting("gotify_token", "super-secret"); err != nil {
		t.Fatalf("set secret: %v", err)
	}

	raw, err := s.GetSetting("gotify_token")
	if err != nil {
		t.Fatalf("get raw: %v", err)
	}
	if !strings.HasPrefix(raw, encryptedPrefix) {
		t.Fatalf("expected encrypted prefix, got %q", raw)
	}
	if strings.Contains(raw, "super-secret") {
		t.Fatal("plaintext secret leaked into database")
	}

	got, err := s.GetSecretSetting("gotify_token")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if got != "super-secret" {
		t.Fatalf("expected decrypted secret, got %q", got)
	}
}

func TestSecretSettingPlaintextBackwardCompatible(t *testing.T) {
	s := openTestStore(t)
	if err := s.SetSetting("gotify_token", "legacy-token"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.GetSecretSetting("gotify_token")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if got != "legacy-token" {
		t.Fatalf("expected legacy plaintext, got %q", got)
	}
}

func TestSecretSettingWithoutKeyStoresPlaintext(t *testing.T) {
	s := openTestStore(t)
	if err := s.SetSecretSetting("gotify_token", "no-key"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	raw, _ := s.GetSetting("gotify_token")
	if raw != "no-key" {
		t.Fatalf("expected plaintext without key, got %q", raw)
	}
}
