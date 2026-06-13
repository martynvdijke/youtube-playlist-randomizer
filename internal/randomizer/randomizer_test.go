package randomizer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/models"
)

type mockYouTubeClient struct {
	playlistsFn func(ctx context.Context) ([]models.Playlist, error)
	itemsFn     func(ctx context.Context, playlistID string) ([]models.PlayListItem, error)
	createFn    func(ctx context.Context, title string) (string, error)
	insertFn    func(ctx context.Context, playlistID, videoID string, position int64) error
}

func (m *mockYouTubeClient) GetPlaylists(ctx context.Context) ([]models.Playlist, error) {
	if m.playlistsFn != nil {
		return m.playlistsFn(ctx)
	}
	return nil, nil
}

func (m *mockYouTubeClient) GetPlaylistItems(ctx context.Context, playlistID string) ([]models.PlayListItem, error) {
	if m.itemsFn != nil {
		return m.itemsFn(ctx, playlistID)
	}
	return nil, nil
}

func (m *mockYouTubeClient) CreatePlaylist(ctx context.Context, title string) (string, error) {
	if m.createFn != nil {
		return m.createFn(ctx, title)
	}
	return "new-pl-id", nil
}

func (m *mockYouTubeClient) InsertPlaylistItem(ctx context.Context, playlistID, videoID string, position int64) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, playlistID, videoID, position)
	}
	return nil
}

type mockPrompter struct {
	promptFn            func(message string) (string, error)
	promptWithDefaultFn func(message, defaultValue string) (string, error)
	chooseFromListFn    func(message string, options []string) (string, error)
}

func (m *mockPrompter) Prompt(message string) (string, error) {
	if m.promptFn != nil {
		return m.promptFn(message)
	}
	return "", nil
}

func (m *mockPrompter) PromptWithDefault(message, defaultValue string) (string, error) {
	if m.promptWithDefaultFn != nil {
		return m.promptWithDefaultFn(message, defaultValue)
	}
	return defaultValue, nil
}

func (m *mockPrompter) ChooseFromList(message string, options []string) (string, error) {
	if m.chooseFromListFn != nil {
		return m.chooseFromListFn(message, options)
	}
	if len(options) > 0 {
		return options[0], nil
	}
	return "", nil
}

func TestRandomizerInitialization(t *testing.T) {
	client := &mockYouTubeClient{}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	if r.client != client {
		t.Error("client not set correctly")
	}
	if r.chunks != 190 {
		t.Errorf("expected chunks 190, got %d", r.chunks)
	}
	if r.prompter != prompter {
		t.Error("prompter not set correctly")
	}
}

func TestGetPlaylistByTitle(t *testing.T) {
	client := &mockYouTubeClient{}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	r.playlists = []models.Playlist{
		*models.NewPlaylist("pl1", "Playlist One"),
		*models.NewPlaylist("pl2", "Playlist Two"),
		*models.NewPlaylist("pl3", "Playlist Three"),
	}

	tests := []struct {
		title    string
		expected string
		found    bool
	}{
		{"Playlist One", "pl1", true},
		{"Playlist Two", "pl2", true},
		{"Playlist Three", "pl3", true},
		{"Nonexistent", "", false},
	}

	for _, tc := range tests {
		pl := r.getPlaylistByTitle(tc.title)
		if tc.found {
			if pl == nil {
				t.Errorf("expected to find playlist '%s', got nil", tc.title)
			} else if pl.ID != tc.expected {
				t.Errorf("expected playlist ID '%s', got '%s'", tc.expected, pl.ID)
			}
		} else {
			if pl != nil {
				t.Errorf("expected not to find playlist '%s', got %v", tc.title, pl.ID)
			}
		}
	}
}

func TestRun_FetchPlaylistsError(t *testing.T) {
	client := &mockYouTubeClient{
		playlistsFn: func(ctx context.Context) ([]models.Playlist, error) {
			return nil, errors.New("api error")
		},
	}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to get playlists") {
		t.Errorf("expected playlist fetch error, got: %v", err)
	}
}

func TestRun_EmptyPlaylistsError(t *testing.T) {
	client := &mockYouTubeClient{
		playlistsFn: func(ctx context.Context) ([]models.Playlist, error) {
			return []models.Playlist{}, nil
		},
	}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	err := r.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no playlists found") {
		t.Errorf("expected empty playlists error, got: %v", err)
	}
}

func TestRun_SuccessfulFlow(t *testing.T) {
	playlists := []models.Playlist{
		*models.NewPlaylist("pl1", "Music"),
	}
	items := []models.PlayListItem{
		models.NewPlayListItem("item1", "Song 1", "2024-01-01", "UC1", "", "Desc", "vid1", ""),
		models.NewPlayListItem("item2", "Song 2", "2024-01-02", "UC1", "", "Desc", "vid2", ""),
		models.NewPlayListItem("item3", "Song 3", "2024-01-03", "UC1", "", "Desc", "vid3", ""),
	}

	client := &mockYouTubeClient{
		playlistsFn: func(ctx context.Context) ([]models.Playlist, error) {
			return playlists, nil
		},
		itemsFn: func(ctx context.Context, playlistID string) ([]models.PlayListItem, error) {
			return items, nil
		},
		createFn: func(ctx context.Context, title string) (string, error) {
			return "new-pl", nil
		},
		insertFn: func(ctx context.Context, playlistID, videoID string, position int64) error {
			return nil
		},
	}

	inserted := make(map[int64]string)
	client.insertFn = func(ctx context.Context, playlistID, videoID string, position int64) error {
		if playlistID != "new-pl" {
			t.Errorf("expected playlistID 'new-pl', got '%s'", playlistID)
		}
		inserted[position] = videoID
		return nil
	}

	prompter := &mockPrompter{
		chooseFromListFn: func(message string, options []string) (string, error) {
			return "Music", nil
		},
		promptWithDefaultFn: func(message, defaultValue string) (string, error) {
			return defaultValue, nil
		},
	}

	r := New(client, 190, prompter)
	err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(inserted) != 3 {
		t.Errorf("expected 3 items inserted, got %d", len(inserted))
	}

	videoIDs := make(map[string]bool)
	for _, vid := range inserted {
		videoIDs[vid] = true
	}
	for _, item := range items {
		if !videoIDs[item.VideoID] {
			t.Errorf("expected video %s to be in inserted items", item.VideoID)
		}
	}
}

func TestPopulateNewPlaylist_NoPlaylists(t *testing.T) {
	client := &mockYouTubeClient{}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	pl := models.NewPlaylist("pl1", "Music")
	err := r.populateNewPlaylist(context.Background(), pl)
	if err == nil || !strings.Contains(err.Error(), "no playlists available") {
		t.Errorf("expected no playlists error, got: %v", err)
	}
}

func TestPopulateNewPlaylist_InsertError(t *testing.T) {
	client := &mockYouTubeClient{
		insertFn: func(ctx context.Context, playlistID, videoID string, position int64) error {
			return errors.New("insert error")
		},
	}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	r.playlists = []models.Playlist{
		*models.NewPlaylist("original", "Music"),
		*models.NewPlaylist("new-pl", "Music-randomized"),
	}

	pl := models.NewPlaylist("original", "Music")
	pl.AddItem(models.NewPlayListItem("item1", "Song 1", "", "", "", "", "vid1", ""))
	pl.AddItem(models.NewPlayListItem("item2", "Song 2", "", "", "", "", "vid2", ""))

	err := r.populateNewPlaylist(context.Background(), pl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChoosePlaylist(t *testing.T) {
	client := &mockYouTubeClient{
		createFn: func(ctx context.Context, title string) (string, error) {
			return "new-pl", nil
		},
	}
	prompter := &mockPrompter{
		chooseFromListFn: func(message string, options []string) (string, error) {
			return "Music", nil
		},
		promptWithDefaultFn: func(message, defaultValue string) (string, error) {
			return defaultValue, nil
		},
	}

	r := New(client, 190, prompter)
	r.playlists = []models.Playlist{
		*models.NewPlaylist("pl1", "Music"),
		*models.NewPlaylist("pl2", "Videos"),
	}

	pl, err := r.choosePlaylist(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pl == nil {
		t.Fatal("expected non-nil playlist")
	}
	if pl.ID != "pl1" {
		t.Errorf("expected playlist ID 'pl1', got '%s'", pl.ID)
	}
	if pl.Title != "Music" {
		t.Errorf("expected playlist title 'Music', got '%s'", pl.Title)
	}

	if len(r.playlists) != 3 {
		t.Errorf("expected 3 playlists, got %d", len(r.playlists))
	}
	if r.playlists[2].ID != "new-pl" {
		t.Errorf("expected new playlist ID 'new-pl', got '%s'", r.playlists[2].ID)
	}
}

func TestChoosePlaylist_Empty(t *testing.T) {
	client := &mockYouTubeClient{}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	_, err := r.choosePlaylist(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no playlists found") {
		t.Errorf("expected no playlists error, got: %v", err)
	}
}

func TestRandomizePlaylist(t *testing.T) {
	items := []models.PlayListItem{
		models.NewPlayListItem("item1", "Video 1", "", "", "", "", "vid1", ""),
		models.NewPlayListItem("item2", "Video 2", "", "", "", "", "vid2", ""),
	}

	client := &mockYouTubeClient{
		itemsFn: func(ctx context.Context, playlistID string) ([]models.PlayListItem, error) {
			return items, nil
		},
		insertFn: func(ctx context.Context, playlistID, videoID string, position int64) error {
			return nil
		},
	}
	prompter := &mockPrompter{}
	r := New(client, 190, prompter)

	r.playlists = []models.Playlist{
		*models.NewPlaylist("original", "Music"),
		*models.NewPlaylist("new-pl", "Music-randomized"),
	}

	pl := models.NewPlaylist("original", "Music")
	err := r.randomizePlaylist(context.Background(), pl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pl.Items) != 2 {
		t.Errorf("expected 2 items in playlist, got %d", len(pl.Items))
	}
}
