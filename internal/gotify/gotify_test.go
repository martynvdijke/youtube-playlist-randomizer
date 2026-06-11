package gotify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := New("http://gotify:8080", "mytoken", true)
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.url != "http://gotify:8080" {
		t.Errorf("expected url 'http://gotify:8080', got '%s'", c.url)
	}
	if c.token != "mytoken" {
		t.Errorf("expected token 'mytoken', got '%s'", c.token)
	}
	if !c.enabled {
		t.Error("expected enabled=true")
	}
}

func TestNewClient_Disabled(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		token   string
		enabled bool
	}{
		{"no url", "", "token", true},
		{"no token", "http://gotify:8080", "", true},
		{"not enabled", "http://gotify:8080", "token", false},
		{"all empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.url, tt.token, tt.enabled)
			if c.Enabled() {
				t.Error("expected client to be disabled")
			}
			// Send should be a no-op
			if err := c.Send("title", "message"); err != nil {
				t.Errorf("expected no error when disabled, got: %v", err)
			}
		})
	}
}

func TestSend_Success(t *testing.T) {
	var received struct {
		Title   string `json:"title"`
		Message string `json:"message"`
		Priority int   `json:"priority"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify token in query string
		if r.URL.Query().Get("token") != "testtoken" {
			t.Errorf("expected token 'testtoken', got '%s'", r.URL.Query().Get("token"))
		}
		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}
		// Decode body
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, "testtoken", true)
	if err := c.Send("Test Title", "Test Message"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if received.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got '%s'", received.Title)
	}
	if received.Message != "Test Message" {
		t.Errorf("expected message 'Test Message', got '%s'", received.Message)
	}
	if received.Priority != defaultPriority {
		t.Errorf("expected priority %d, got %d", defaultPriority, received.Priority)
	}
}

func TestSend_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := New(server.URL, "token", true)
	err := c.Send("title", "msg")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSetPriority(t *testing.T) {
	var received struct {
		Priority int `json:"priority"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(server.URL, "token", true)
	c.SetPriority(10)
	if err := c.Send("title", "msg"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if received.Priority != 10 {
		t.Errorf("expected priority 10, got %d", received.Priority)
	}
}

func TestEnabled_WhenConfigured(t *testing.T) {
	c := New("http://gotify:8080", "token", true)
	if !c.Enabled() {
		t.Error("expected client to be enabled when url, token, and enabled are all set")
	}

	c2 := New("http://gotify:8080", "token", false)
	if c2.Enabled() {
		t.Error("expected client to be disabled when enabled=false")
	}

	c3 := New("http://gotify:8080", "", true)
	if c3.Enabled() {
		t.Error("expected client to be disabled when token is empty")
	}
}
