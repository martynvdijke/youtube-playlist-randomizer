package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	tokenFileName = "token.json"
	scope         = youtube.YoutubeForceSslScope
)

type Client struct {
	service *youtube.Service
}

func NewClient(ctx context.Context, clientSecretPath, tokenDir string) (*Client, error) {
	data, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %w", err)
	}

	config, err := google.ConfigFromJSON(data, scope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file: %w", err)
	}

	if tokenDir == "" {
		tokenDir = filepath.Dir(clientSecretPath)
	}
	tokenCachePath := filepath.Join(tokenDir, tokenFileName)
	token, err := tokenFromFile(tokenCachePath)
	if err != nil {
		token, err = getTokenFromWeb(config)
		if err != nil {
			return nil, fmt.Errorf("unable to get token from web: %w", err)
		}
		if err := saveToken(tokenCachePath, token); err != nil {
			log.Printf("warning: unable to cache token: %v", err)
		}
	}

	service, err := youtube.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		return nil, fmt.Errorf("unable to create YouTube service: %w", err)
	}

	return &Client{service: service}, nil
}

func (c *Client) GetPlaylists(ctx context.Context) ([]models.Playlist, error) {
	var allItems []*youtube.Playlist
	nextPageToken := ""

	for {
		call := c.service.Playlists.List([]string{"snippet", "contentDetails"}).Mine(true)
		if nextPageToken != "" {
			call = call.PageToken(nextPageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch playlists: %w", err)
		}
		allItems = append(allItems, response.Items...)
		if response.NextPageToken == "" {
			break
		}
		nextPageToken = response.NextPageToken
	}

	playlists := make([]models.Playlist, 0, len(allItems))
	for _, item := range allItems {
		pl := models.NewPlaylist(item.Id, "")
		if item.Snippet != nil {
			if item.Snippet.Localized != nil {
				pl.Title = item.Snippet.Localized.Title
			} else {
				pl.Title = item.Snippet.Title
			}
		}
		if item.ContentDetails != nil {
			pl.ItemCount = int(item.ContentDetails.ItemCount)
		}
		playlists = append(playlists, *pl)
	}
	return playlists, nil
}

func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]models.PlayListItem, error) {
	var allItems []*youtube.PlaylistItem
	nextPageToken := ""

	for {
		call := c.service.PlaylistItems.List([]string{"snippet"}).PlaylistId(playlistID)
		if nextPageToken != "" {
			call = call.PageToken(nextPageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch playlist items: %w", err)
		}
		allItems = append(allItems, response.Items...)
		if response.NextPageToken == "" {
			break
		}
		nextPageToken = response.NextPageToken
	}

	items := make([]models.PlayListItem, 0, len(allItems))
	for _, item := range allItems {
		items = append(items, convertToPlayListItem(item))
	}
	return items, nil
}

func (c *Client) CreatePlaylist(ctx context.Context, title string) (string, error) {
	playlist := &youtube.Playlist{
		Snippet: &youtube.PlaylistSnippet{Title: title},
		Status:  &youtube.PlaylistStatus{PrivacyStatus: "public"},
	}

	response, err := c.service.Playlists.Insert([]string{"snippet", "status"}, playlist).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to create playlist: %w", err)
	}
	return response.Id, nil
}

func (c *Client) InsertPlaylistItem(ctx context.Context, playlistID, videoID string, position int64) error {
	item := &youtube.PlaylistItem{
		Snippet: &youtube.PlaylistItemSnippet{
			PlaylistId: playlistID,
			Position:   position,
			ResourceId: &youtube.ResourceId{
				Kind:    "youtube#video",
				VideoId: videoID,
			},
		},
	}

	_, err := c.service.PlaylistItems.Insert([]string{"snippet"}, item).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to insert playlist item: %w", err)
	}
	return nil
}

func convertToPlayListItem(item *youtube.PlaylistItem) models.PlayListItem {
	snippet := item.Snippet
	return models.NewPlayListItem(
		item.Id,
		snippet.Title,
		snippet.PublishedAt,
		snippet.ChannelId,
		snippet.Description,
		snippet.ResourceId.VideoId,
	)
}

func tokenFromFile(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	token := &oauth2.Token{}
	if err := json.Unmarshal(data, token); err != nil {
		return nil, err
	}
	return token, nil
}

func saveToken(path string, token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}
	return nil
}

func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	// In Docker, use port 6270 (same as app, acquired before ListenAndServe)
	// so docker -p 6270:6270 covers both the app and OAuth callback.
	mainPortStr := os.Getenv("PORT")
	mainPort := 6270
	if p, err := strconv.Atoi(mainPortStr); err == nil && p > 0 {
		mainPort = p
	}

	ports := []int{8080, 0}
	if os.Getenv("DOCKER") == "true" {
		ports = []int{mainPort, 0}
	}

	var lastErr error

	for _, port := range ports {
		token, err := getTokenViaLocalServer(config, port)
		if err == nil {
			return token, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed to get token via web: %w", lastErr)
}

func getTokenViaLocalServer(config *oauth2.Config, preferredPort int) (*oauth2.Token, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", preferredPort))
	if err != nil {
		return nil, fmt.Errorf("unable to listen on port %d: %w", preferredPort, err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://localhost:%d/callback", port)
	config.RedirectURL = redirectURL

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Printf("Open the following link in your browser:\n\n%s\n\n", authURL)
	fmt.Printf("The server is listening on %s\n", redirectURL)

	tokenChan := make(chan *oauth2.Token, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in request")
			http.Error(w, "No code in request", http.StatusBadRequest)
			return
		}

		token, err := config.Exchange(context.Background(), code)
		if err != nil {
			errChan <- fmt.Errorf("unable to exchange code for token: %w", err)
			http.Error(w, "Unable to exchange code for token", http.StatusInternalServerError)
			return
		}

		fmt.Fprintln(w, "Authentication successful! You may close this window.")
		tokenChan <- token
	})

	server := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      mux,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case token := <-tokenChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
		return token, nil
	case err := <-errChan:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
		return nil, err
	}
}
