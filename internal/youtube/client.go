package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/logging"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/models"
	"github.com/martynvdijke/youtube-playlist-randomizer/internal/telemetry"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	tokenFileName = "token.json"
	scope         = youtube.YoutubeForceSslScope
)

type Client struct {
	service *youtube.Service
	otel    *telemetry.Telemetry
	logger  *logging.Logger
}

type OAuthSetup struct {
	Config    *oauth2.Config
	SecretDir string
}

var ErrNoToken = errors.New("no cached OAuth token found — authentication required")

func NewClient(ctx context.Context, clientSecretPath, tokenDir string, otel *telemetry.Telemetry, logger *logging.Logger) (*Client, error) {
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
		return nil, ErrNoToken
	}

	_, client, err := createServiceWithToken(ctx, config, token, tokenCachePath, otel, logger)
	if err == nil {
		return client, nil
	}

	if strings.Contains(err.Error(), "token expired") || strings.Contains(err.Error(), "refresh token is not set") || strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "Invalid Credentials") || strings.Contains(err.Error(), "authError") {
		log.Printf("Stored token at %s is expired and cannot be refreshed. Deleting.", tokenCachePath)
		os.Remove(tokenCachePath)
		backupPath := filepath.Join(filepath.Dir(clientSecretPath), tokenFileName)
		os.Remove(backupPath)
		return nil, ErrNoToken
	}

	log.Printf("Token validation error (unexpected): %v", err)
	return nil, ErrNoToken
}

func createServiceWithToken(ctx context.Context, config *oauth2.Config, token *oauth2.Token, tokenCachePath string, otel *telemetry.Telemetry, logger *logging.Logger) (*youtube.Service, *Client, error) {
	tokenSrc := config.TokenSource(ctx, token)
	tokenSrc = &persistTokenSource{
		inner: tokenSrc,
		path:  tokenCachePath,
	}

	service, err := youtube.NewService(ctx, option.WithTokenSource(tokenSrc))
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create YouTube service: %w", err)
	}

	client := &Client{service: service, otel: otel, logger: logger}

	if err := client.verifyToken(ctx); err != nil {
		if IsQuotaError(err) {
			log.Printf("Token valid but quota exhausted — returning client anyway.")
			return service, client, nil
		}
		return nil, nil, err
	}

	if logger != nil {
		logger.Infoc(ctx, "YouTube API: token validated successfully")
	}
	return service, client, nil
}

type persistTokenSource struct {
	inner oauth2.TokenSource
	path  string
	mu    sync.Mutex
}

func (p *persistTokenSource) Token() (*oauth2.Token, error) {
	token, err := p.inner.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := saveToken(p.path, token); err != nil {
		log.Printf("warning: failed to persist refreshed token: %v", err)
	}
	return token, nil
}

// TokenWithLogger is like Token but with structured logging.
// Deprecated: kept for backward compatibility.

func (c *Client) verifyToken(ctx context.Context) error {
	call := c.service.Playlists.List([]string{"snippet"}).Mine(true).MaxResults(1)
	_, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("YouTube API test call failed: %w", err)
	}
	if c.logger != nil {
		c.logger.Infoc(ctx, "YouTube API: token validated successfully")
	}
	return nil
}

func (c *Client) GetPlaylists(ctx context.Context) ([]models.Playlist, error) {
	var allItems []*youtube.Playlist
	nextPageToken := ""

	for {
		call := c.service.Playlists.List([]string{"snippet", "contentDetails"}).Mine(true).MaxResults(50)
		if nextPageToken != "" {
			call = call.PageToken(nextPageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			if c.logger != nil {
				c.logger.Errorc(ctx, "YouTube API: failed to fetch playlists", "error", err.Error())
			}
			return nil, fmt.Errorf("failed to fetch playlists: %w", err)
		}
		if c.otel != nil {
			c.otel.RecordYouTubeAPICall(ctx, "playlists.list")
		}
		if c.logger != nil {
			c.logger.Debugc(ctx, "YouTube API: fetched playlist page", "items", fmt.Sprintf("%d", len(response.Items)), "pageToken", nextPageToken)
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
	if c.logger != nil {
		c.logger.Infoc(ctx, "YouTube API: playlists fetched successfully", "count", fmt.Sprintf("%d", len(playlists)))
	}
	return playlists, nil
}

func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]models.PlayListItem, error) {
	var allItems []*youtube.PlaylistItem
	nextPageToken := ""

	for {
		call := c.service.PlaylistItems.List([]string{"snippet"}).PlaylistId(playlistID).MaxResults(50)
		if nextPageToken != "" {
			call = call.PageToken(nextPageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			if c.logger != nil {
				c.logger.Errorc(ctx, "YouTube API: failed to fetch playlist items", "playlistId", playlistID, "error", err.Error())
			}
			return nil, fmt.Errorf("failed to fetch playlist items: %w", err)
		}
		if c.otel != nil {
			c.otel.RecordYouTubeAPICall(ctx, "playlistItems.list")
		}
		if c.logger != nil {
			c.logger.Debugc(ctx, "YouTube API: fetched item page", "playlistId", playlistID, "items", fmt.Sprintf("%d", len(response.Items)))
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
	if c.logger != nil {
		c.logger.Infoc(ctx, "YouTube API: playlist items fetched", "playlistId", playlistID, "count", fmt.Sprintf("%d", len(items)))
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
		if c.logger != nil {
			c.logger.Errorc(ctx, "YouTube API: failed to create playlist", "title", title, "error", err.Error())
		}
		return "", fmt.Errorf("failed to create playlist: %w", err)
	}
	if c.otel != nil {
		c.otel.RecordYouTubeAPICall(ctx, "playlists.insert")
	}
	if c.logger != nil {
		c.logger.Infoc(ctx, "YouTube API: playlist created", "title", title, "playlistId", response.Id)
	}
	return response.Id, nil
}

func (c *Client) InsertPlaylistItem(ctx context.Context, playlistID, videoID string, position int64) error {
	item := &youtube.PlaylistItem{
		Snippet: &youtube.PlaylistItemSnippet{
			PlaylistId: playlistID,
			ResourceId: &youtube.ResourceId{
				Kind:    "youtube#video",
				VideoId: videoID,
			},
		},
	}

	_, err := c.service.PlaylistItems.Insert([]string{"snippet"}, item).Context(ctx).Do()
	if err != nil {
		if c.logger != nil {
			c.logger.Warnc(ctx, "YouTube API: failed to insert playlist item", "playlistId", playlistID, "videoId", videoID, "error", err.Error())
		}
		return fmt.Errorf("failed to insert playlist item: %w", err)
	}
	if c.otel != nil {
		c.otel.RecordYouTubeAPICall(ctx, "playlistItems.insert")
	}
	if c.logger != nil {
		c.logger.Debugc(ctx, "YouTube API: playlist item inserted", "playlistId", playlistID, "videoId", videoID)
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

func IsQuotaError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.Code != 403 && apiErr.Code != 429 {
		return false
	}
	for _, e := range apiErr.Errors {
		reason := strings.ToLower(e.Reason)
		if reason == "quotaexceeded" || reason == "ratelimitexceeded" || reason == "dailylimitexceeded" {
			return true
		}
	}
	bodyLower := strings.ToLower(apiErr.Body)
	return strings.Contains(bodyLower, "quotaexceeded") ||
		strings.Contains(bodyLower, "dailylimitexceeded") ||
		strings.Contains(bodyLower, "daily limit exceeded")
}

func LoadConfig(clientSecretPath string) (*OAuthSetup, error) {
	data, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secret file: %w", err)
	}

	config, err := google.ConfigFromJSON(data, scope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secret file: %w", err)
	}

	callbackURL := os.Getenv("OAUTH_CALLBACK_URL")
	if callbackURL != "" {
		config.RedirectURL = callbackURL
	} else {
		config.RedirectURL = fmt.Sprintf("http://localhost:%s/callback", os.Getenv("PORT"))
	}

	return &OAuthSetup{Config: config, SecretDir: filepath.Dir(clientSecretPath)}, nil
}

func AuthURL(setup *OAuthSetup) string {
	return setup.Config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

func HandleCallback(setup *OAuthSetup, code string, extraDirs ...string) error {
	token, err := setup.Config.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("unable to exchange code for token: %w", err)
	}

	dirs := append([]string{setup.SecretDir}, extraDirs...)
	for _, dir := range dirs {
		path := filepath.Join(dir, tokenFileName)
		if err := saveToken(path, token); err != nil {
			log.Printf("warning: failed to save token to %s: %v", path, err)
		} else {
			log.Printf("Token saved to %s", path)
		}
	}
	return nil
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
