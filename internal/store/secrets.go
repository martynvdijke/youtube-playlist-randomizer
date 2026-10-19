package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
)

// encryptedPrefix marks values encrypted with SetSecretSetting. Values without
// the prefix are treated as legacy plaintext and are returned unchanged so
// existing installations keep working.
const encryptedPrefix = "enc:v1:"

var errSecretKeyMissing = errors.New("SETTINGS_ENCRYPTION_KEY is not set")

func secretKey() ([]byte, bool) {
	raw := os.Getenv("SETTINGS_ENCRYPTION_KEY")
	if raw == "" {
		return nil, false
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], true
}

func encryptSecret(plaintext string) (string, error) {
	key, ok := secretKey()
	if !ok {
		return "", errSecretKeyMissing
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encryptedPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptSecret(value string) (string, error) {
	if !strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}
	key, ok := secretKey()
	if !ok {
		return "", errSecretKeyMissing
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encryptedPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("encrypted setting is truncated")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// SetSecretSetting stores a secret encrypted at rest when
// SETTINGS_ENCRYPTION_KEY is configured. Without a key the value is stored as
// plaintext (backward compatible) and callers should mask it when returning it
// to clients.
func (s *Store) SetSecretSetting(key, value string) error {
	if value == "" {
		return s.SetSetting(key, "")
	}
	if _, ok := secretKey(); !ok {
		return s.SetSetting(key, value)
	}
	encrypted, err := encryptSecret(value)
	if err != nil {
		return err
	}
	return s.SetSetting(key, encrypted)
}

// GetSecretSetting reads a setting written by SetSecretSetting. Legacy
// plaintext values are returned as-is. If the value is encrypted but the key is
// missing or wrong, an empty string and an error are returned; callers must not
// log the underlying value.
func (s *Store) GetSecretSetting(key string) (string, error) {
	value, err := s.GetSetting(key)
	if err != nil || value == "" {
		return value, err
	}
	if !strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}
	return decryptSecret(value)
}
