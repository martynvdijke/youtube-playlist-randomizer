package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	quotaListPlaylists      = 1
	quotaListPlaylistItems  = 1
	quotaInsertPlaylist     = 50
	quotaInsertPlaylistItem = 50
	defaultDailyLimit       = 10000
)

type PlaylistInfo struct {
	ID        string
	Title     string
	ItemCount int64
}

type ShuffleJob struct {
	ID               string
	PlaylistID       string
	PlaylistTitle    string
	Status           string
	Progress         int
	Total            int
	Done             int
	NewPlaylistID    string
	NewPlaylistTitle string
	Error            string
	QuotaUsed        int
	QuotaEstimated   int
	Cancel           chan struct{} `json:"-"`
}

type QuotaDay struct {
	Date       string `json:"date"`
	Used       int    `json:"used"`
	DailyLimit int    `json:"dailyLimit"`
}

func getYouTubeService(ctx context.Context, token *oauth2.Token, ts oauth2.TokenSource) (*youtube.Service, error) {
	client := oauth2.NewClient(ctx, ts)
	return youtube.NewService(ctx, option.WithHTTPClient(client))
}

func getTokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return oauthConfig.TokenSource(ctx, token)
}

func checkAborted(job *ShuffleJob) bool {
	select {
	case <-job.Cancel:
		job.Status = "cancelled"
		return true
	default:
		return false
	}
}

func fetchPlaylists(svc *youtube.Service) ([]PlaylistInfo, error) {
	if remainingQuota() < quotaListPlaylists {
		used, limit := getTodayQuota()
		return nil, fmt.Errorf("daily YouTube API quota exhausted (%d/%d used), try again tomorrow",
			used, limit)
	}

	var playlists []PlaylistInfo
	pageToken := ""
	for {
		call := svc.Playlists.List([]string{"snippet", "contentDetails"}).
			Mine(true).
			MaxResults(50)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list playlists: %w", err)
		}
		recordQuota(quotaListPlaylists)

		for _, item := range resp.Items {
			playlists = append(playlists, PlaylistInfo{
				ID:        item.Id,
				Title:     item.Snippet.Title,
				ItemCount: item.ContentDetails.ItemCount,
			})
		}

		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}
	return playlists, nil
}

func fetchPlaylistItems(svc *youtube.Service, playlistID string) ([]*youtube.PlaylistItem, error) {
	if remainingQuota() < quotaListPlaylistItems {
		used, limit := getTodayQuota()
		return nil, fmt.Errorf("daily YouTube API quota exhausted (%d/%d used)",
			used, limit)
	}

	var items []*youtube.PlaylistItem
	pageToken := ""
	for {
		call := svc.PlaylistItems.List([]string{"snippet"}).
			PlaylistId(playlistID).
			MaxResults(50)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list playlist items: %w", err)
		}
		recordQuota(quotaListPlaylistItems)

		items = append(items, resp.Items...)

		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}
	return items, nil
}

func createPlaylist(svc *youtube.Service, title string) (string, error) {
	if remainingQuota() < quotaInsertPlaylist {
		used, limit := getTodayQuota()
		return "", fmt.Errorf("daily YouTube API quota exhausted (%d/%d used)",
			used, limit)
	}

	playlist := &youtube.Playlist{
		Snippet: &youtube.PlaylistSnippet{
			Title: title,
		},
		Status: &youtube.PlaylistStatus{
			PrivacyStatus: "private",
		},
	}

	resp, err := svc.Playlists.Insert([]string{"snippet", "status"}, playlist).Do()
	if err != nil {
		return "", fmt.Errorf("create playlist: %w", err)
	}
	recordQuota(quotaInsertPlaylist)
	return resp.Id, nil
}

func insertPlaylistItem(svc *youtube.Service, playlistID string, videoID string, position int64) error {
	if remainingQuota() < quotaInsertPlaylistItem {
		return fmt.Errorf("quota exceeded")
	}

	item := &youtube.PlaylistItem{
		Snippet: &youtube.PlaylistItemSnippet{
			PlaylistId: playlistID,
			ResourceId: &youtube.ResourceId{
				Kind:    "youtube#video",
				VideoId: videoID,
			},
			Position: position,
		},
	}

	_, err := svc.PlaylistItems.Insert([]string{"snippet"}, item).Do()
	if err != nil {
		return err
	}
	recordQuota(quotaInsertPlaylistItem)
	return nil
}

func estimateShuffleCost(itemCount int) int {
	return quotaListPlaylistItems*(itemCount/50+1) +
		quotaInsertPlaylist +
		quotaInsertPlaylistItem*itemCount
}

func runShuffleJob(job *ShuffleJob, svc *youtube.Service) {
	if checkAborted(job) {
		return
	}
	job.Status = "fetching"
	log.Printf("shuffle job %s: fetching items from playlist %s", job.ID, job.PlaylistID)

	items, err := fetchPlaylistItems(svc, job.PlaylistID)
	if err != nil {
		job.Status = "error"
		job.Error = err.Error()
		log.Printf("shuffle job %s: fetch error: %v", job.ID, err)
		return
	}
	if checkAborted(job) {
		return
	}

	job.Total = len(items)
	job.QuotaEstimated = estimateShuffleCost(job.Total)

	remaining := remainingQuota()
	if remaining < job.QuotaEstimated {
		job.Status = "error"
		job.Error = fmt.Sprintf(
			"Not enough daily quota. Need ~%d units for %d items, but only %d remaining. "+
				"Wait until tomorrow (quota resets at midnight PST) or reduce the playlist size.",
			job.QuotaEstimated, job.Total, remaining,
		)
		log.Printf("shuffle job %s: quota error: %s", job.ID, job.Error)
		return
	}

	if job.Total == 0 {
		job.Status = "error"
		job.Error = "playlist has no items"
		return
	}

	rand.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})

	now := time.Now()
	job.NewPlaylistTitle = fmt.Sprintf("%s-randomized-%s",
		job.PlaylistTitle, now.Format("January-2006"))

	if checkAborted(job) {
		return
	}
	job.Status = "creating"
	log.Printf("shuffle job %s: creating new playlist %q", job.ID, job.NewPlaylistTitle)
	newID, err := createPlaylist(svc, job.NewPlaylistTitle)
	if err != nil {
		job.Status = "error"
		job.Error = fmt.Sprintf("failed to create new playlist: %v", err)
		log.Printf("shuffle job %s: create playlist error: %v", job.ID, err)
		return
	}
	job.NewPlaylistID = newID

	if checkAborted(job) {
		return
	}
	job.Status = "inserting"
	log.Printf("shuffle job %s: inserting %d items into %s", job.ID, job.Total, newID)

	for i, item := range items {
		if checkAborted(job) {
			return
		}
		if remainingQuota() < quotaInsertPlaylistItem {
			job.Status = "error"
			job.Error = fmt.Sprintf(
				"Quota exhausted after inserting %d/%d items. %d items were not added. "+
					"Wait until tomorrow to finish.",
				i, job.Total, job.Total-i,
			)
			log.Printf("shuffle job %s: quota exhausted at item %d/%d", job.ID, i, job.Total)
			return
		}

		err := insertPlaylistItem(svc, newID, item.Snippet.ResourceId.VideoId, int64(i))
		if err != nil {
			log.Printf("shuffle job %s: insert item %d/%d (%s) error: %v",
				job.ID, i+1, job.Total, item.Snippet.Title, err)
			continue
		}

		job.Done = i + 1
		job.Progress = (job.Done * 100) / job.Total
		time.Sleep(50 * time.Millisecond)
	}

	job.Status = "done"
	job.Progress = 100
	log.Printf("shuffle job %s: done - %d items in %s", job.ID, job.Total, newID)
}
