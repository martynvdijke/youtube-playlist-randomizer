package randomizer

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/models"
)

const (
	chunkSleepDuration = 25 * time.Hour  // 90000 seconds
	requestDelay       = 10 * time.Millisecond
)

// YouTubeClient defines the interface for YouTube API operations.
type YouTubeClient interface {
	GetPlaylists(ctx context.Context) ([]models.Playlist, error)
	GetPlaylistItems(ctx context.Context, playlistID string) ([]models.PlayListItem, error)
	CreatePlaylist(ctx context.Context, title string) (string, error)
	InsertPlaylistItem(ctx context.Context, playlistID, videoID string, position int64) error
}

type Randomizer struct {
	client    YouTubeClient
	chunks    int
	playlists []models.Playlist
	prompter  Prompter
}

type Prompter interface {
	Prompt(message string) (string, error)
	PromptWithDefault(message, defaultValue string) (string, error)
	ChooseFromList(message string, options []string) (string, error)
}

func New(client YouTubeClient, chunks int, prompter Prompter) *Randomizer {
	return &Randomizer{
		client:   client,
		chunks:   chunks,
		prompter: prompter,
	}
}

func (r *Randomizer) Run(ctx context.Context) error {
	var err error
	r.playlists, err = r.client.GetPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("failed to get playlists: %w", err)
	}

	playlist, err := r.choosePlaylist(ctx)
	if err != nil {
		return fmt.Errorf("failed to choose playlist: %w", err)
	}

	return r.randomizePlaylist(ctx, playlist)
}

func (r *Randomizer) choosePlaylist(ctx context.Context) (*models.Playlist, error) {
	titles := make([]string, len(r.playlists))
	for i, pl := range r.playlists {
		titles[i] = pl.Title
	}

	if len(titles) == 0 {
		return nil, fmt.Errorf("no playlists found")
	}

	title, err := r.prompter.ChooseFromList("Choose a playlist to randomize", titles)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	monthYear := now.Format("January-2006")
	defaultNewTitle := fmt.Sprintf("%s-randomized-%s", title, monthYear)

	newTitle, err := r.prompter.PromptWithDefault("Name for the randomized playlist", defaultNewTitle)
	if err != nil {
		return nil, err
	}

	playlistID, err := r.client.CreatePlaylist(ctx, newTitle)
	if err != nil {
		return nil, fmt.Errorf("failed to create new playlist: %w", err)
	}

	r.playlists = append(r.playlists, *models.NewPlaylist(playlistID, newTitle))

	pl := r.getPlaylistByTitle(title)
	if pl == nil {
		return nil, fmt.Errorf("playlist '%s' not found after creation", title)
	}
	return pl, nil
}

func (r *Randomizer) getPlaylistByTitle(title string) *models.Playlist {
	for i := range r.playlists {
		if r.playlists[i].Title == title {
			return &r.playlists[i]
		}
	}
	return nil
}

func (r *Randomizer) randomizePlaylist(ctx context.Context, playlist *models.Playlist) error {
	items, err := r.client.GetPlaylistItems(ctx, playlist.ID)
	if err != nil {
		return fmt.Errorf("failed to get playlist items: %w", err)
	}
	playlist.Items = items

	return r.populateNewPlaylist(ctx, playlist)
}

func (r *Randomizer) populateNewPlaylist(ctx context.Context, playlist *models.Playlist) error {
	if len(r.playlists) == 0 {
		return fmt.Errorf("no playlists available for insertion")
	}

	newPlaylistID := r.playlists[len(r.playlists)-1].ID

	shuffled := make([]models.PlayListItem, len(playlist.Items))
	copy(shuffled, playlist.Items)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	for i, item := range shuffled {
		if err := r.client.InsertPlaylistItem(ctx, newPlaylistID, item.VideoID, int64(i)); err != nil {
			log.Printf("warning: failed to insert item %s (video %s): %v", item.ID, item.VideoID, err)
			continue
		}
		time.Sleep(requestDelay)

		if (i+1)%r.chunks == 0 {
			log.Printf("reached %d items, sleeping for %v...", i+1, chunkSleepDuration)
			time.Sleep(chunkSleepDuration)
		}
	}

	log.Printf("successfully inserted %d items into playlist %s", len(shuffled), newPlaylistID)
	return nil
}
