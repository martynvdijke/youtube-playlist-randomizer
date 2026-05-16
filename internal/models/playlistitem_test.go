package models

import "testing"

func TestPlayListItemInitialization(t *testing.T) {
	item := NewPlayListItem("id1", "Test Video", "2024-01-15T10:00:00Z", "UC123", "A test description", "vid123")

	if item.ID != "id1" {
		t.Errorf("expected ID 'id1', got '%s'", item.ID)
	}
	if item.Title != "Test Video" {
		t.Errorf("expected Title 'Test Video', got '%s'", item.Title)
	}
	if item.PublishedAt != "2024-01-15T10:00:00Z" {
		t.Errorf("expected PublishedAt '2024-01-15T10:00:00Z', got '%s'", item.PublishedAt)
	}
	if item.ChannelID != "UC123" {
		t.Errorf("expected ChannelID 'UC123', got '%s'", item.ChannelID)
	}
	if item.Description != "A test description" {
		t.Errorf("expected Description 'A test description', got '%s'", item.Description)
	}
	if item.VideoID != "vid123" {
		t.Errorf("expected VideoID 'vid123', got '%s'", item.VideoID)
	}
}

func TestPlayListItemEmptyInitialization(t *testing.T) {
	item := NewPlayListItem("", "", "", "", "", "")

	if item.ID != "" || item.Title != "" || item.PublishedAt != "" ||
		item.ChannelID != "" || item.Description != "" || item.VideoID != "" {
		t.Error("expected all fields to be empty")
	}
}

func TestPlayListItemPartialInitialization(t *testing.T) {
	item := NewPlayListItem("id1", "Test Video", "", "", "", "")

	if item.ID != "id1" {
		t.Errorf("expected ID 'id1', got '%s'", item.ID)
	}
	if item.Title != "Test Video" {
		t.Errorf("expected Title 'Test Video', got '%s'", item.Title)
	}
	if item.PublishedAt != "" {
		t.Errorf("expected empty PublishedAt, got '%s'", item.PublishedAt)
	}
}
