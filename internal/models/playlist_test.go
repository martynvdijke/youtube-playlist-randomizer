package models

import "testing"

func TestPlaylistInitialization(t *testing.T) {
	pl := NewPlaylist("PL123", "My Playlist")

	if pl.ID != "PL123" {
		t.Errorf("expected ID 'PL123', got '%s'", pl.ID)
	}
	if pl.Title != "My Playlist" {
		t.Errorf("expected Title 'My Playlist', got '%s'", pl.Title)
	}
	if pl.Items == nil {
		t.Error("expected Items to be initialized, got nil")
	}
	if len(pl.Items) != 0 {
		t.Errorf("expected empty Items, got %d items", len(pl.Items))
	}
}

func TestPlaylistAddItem(t *testing.T) {
	pl := NewPlaylist("PL123", "My Playlist")
	item := NewPlayListItem("item1", "Video 1", "2024-01-15T10:00:00Z", "UC123", "Desc", "vid1")

	pl.AddItem(item)

	if len(pl.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(pl.Items))
	}
	if pl.Items[0].ID != "item1" {
		t.Errorf("expected item ID 'item1', got '%s'", pl.Items[0].ID)
	}
}

func TestPlaylistAddMultipleItems(t *testing.T) {
	pl := NewPlaylist("PL123", "My Playlist")

	items := []PlayListItem{
		NewPlayListItem("item1", "Video 1", "", "", "", "vid1"),
		NewPlayListItem("item2", "Video 2", "", "", "", "vid2"),
		NewPlayListItem("item3", "Video 3", "", "", "", "vid3"),
	}

	for _, item := range items {
		pl.AddItem(item)
	}

	if len(pl.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(pl.Items))
	}

	for i, expected := range items {
		if pl.Items[i].ID != expected.ID {
			t.Errorf("item %d: expected ID '%s', got '%s'", i, expected.ID, pl.Items[i].ID)
		}
	}
}
