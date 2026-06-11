// Package gotify provides a lightweight client for sending push notifications
// via a Gotify server.
package gotify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultPriority = 5
	httpTimeout     = 10 * time.Second
)

// Client sends notifications to a Gotify server.
type Client struct {
	url      string
	token    string
	enabled  bool
	priority int
	http     *http.Client
}

// Message is the JSON payload sent to the Gotify API.
type Message struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

// New creates a new Gotify client. If url or token is empty, the client
// is effectively disabled (Send becomes a no-op).
func New(url, token string, enabled bool) *Client {
	return &Client{
		url:      url,
		token:    token,
		enabled:  enabled,
		priority: defaultPriority,
		http: &http.Client{
			Timeout: httpTimeout,
		},
	}
}

// Send sends a push notification with the given title and message.
// Returns an error if the client is misconfigured or the server responds
// with a non-2xx status.
func (c *Client) Send(title, message string) error {
	if !c.enabled || c.url == "" || c.token == "" {
		return nil // silently no-op when not configured
	}

	msg := Message{
		Title:    title,
		Message:  message,
		Priority: c.priority,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("gotify: marshal message: %w", err)
	}

	endpoint := c.url + "/message?token=" + c.token
	resp, err := c.http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gotify: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gotify: server returned status %d", resp.StatusCode)
	}

	return nil
}

// Enabled returns whether the client is configured and enabled.
func (c *Client) Enabled() bool {
	return c.enabled && c.url != "" && c.token != ""
}

// SetPriority overrides the default notification priority (default 5).
func (c *Client) SetPriority(p int) {
	c.priority = p
}
